package supervisor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"

	"github.com/warmdev/modex/internal/bridge"
)

type cliLease struct {
	pending map[int]int
	replies chan json.RawMessage
	cancel  context.CancelFunc
}

// Publish sends private CLI responses to their owner and journals all other frames.
func (s *Server) Publish(raw json.RawMessage) error {
	var frame struct {
		ID     int             `json:"id"`
		Event  string          `json:"event"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(raw, &frame); err != nil {
		return err
	}
	if frame.ID >= cliIDStart && frame.ID <= cliIDEnd {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.cli != nil {
			if id, ok := s.cli.pending[frame.ID]; ok {
				delete(s.cli.pending, frame.ID)
				var result map[string]json.RawMessage
				if json.Unmarshal(raw, &result) != nil {
					return errors.New("invalid response")
				}
				result["id"], _ = json.Marshal(id)
				data, err := json.Marshal(result)
				if err != nil {
					return err
				}
				select {
				case s.cli.replies <- data:
				default:
					s.cli.cancel()
				}
			}
		}
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Journal.Append(raw); err != nil {
		return err
	}
	var decision struct {
		ID string `json:"decision_id"`
	}
	if frame.Event == "decision/required" || frame.Event == "decision/resolved" {
		_ = json.Unmarshal(frame.Params, &decision)
	}
	switch frame.Event {
	case "decision/reset":
		s.decisions = nil
		s.decisionBytes = 0
		s.decisionOverflow = false
	case "decision/required":
		if decision.ID != "" {
			if s.decisions == nil {
				s.decisions = make(map[string]json.RawMessage)
			}
			old := s.decisions[decision.ID]
			if (old == nil && len(s.decisions) >= 128) || s.decisionBytes-len(old)+len(raw) > 8*1024*1024 {
				s.decisionOverflow = true
			} else {
				s.decisionBytes += len(raw) - len(old)
				s.decisions[decision.ID] = append(json.RawMessage(nil), raw...)
			}
		}
	case "decision/resolved":
		if old := s.decisions[decision.ID]; old != nil {
			s.decisionBytes -= len(old)
			delete(s.decisions, decision.ID)
		}
	}
	return nil
}

func (s *Server) handleCLI(ctx context.Context, cancel context.CancelFunc, conn net.Conn, scanner *bufio.Scanner, hello Hello, send func(any) error) {
	lease := &cliLease{pending: make(map[int]int), replies: make(chan json.RawMessage, 4), cancel: cancel}
	s.mu.Lock()
	if s.cli != nil {
		s.mu.Unlock()
		_ = send(map[string]string{"error": "another CLI is attached"})
		return
	}
	if s.decisionOverflow {
		s.mu.Unlock()
		_ = send(map[string]string{"error": "pending decisions exceed relay limit"})
		return
	}
	s.cli = lease
	cursor := s.Journal.Checkpoint()
	pending := make([]json.RawMessage, 0, len(s.decisions))
	for _, raw := range s.decisions {
		pending = append(pending, raw)
	}
	s.mu.Unlock()
	setContext := func(value string) error {
		if s.OnCLIContext == nil {
			return nil
		}
		s.childMu.Lock()
		defer s.childMu.Unlock()
		return s.OnCLIContext(value)
	}
	defer func() {
		cancel()
		if setContext("") != nil {
			s.stop()
		}
		s.mu.Lock()
		s.cli = nil
		s.mu.Unlock()
	}()
	if setContext(validSSHConnection(hello.SSHConnection)) != nil {
		s.stop()
		return
	}
	if send(map[string]any{"ready": true, "instance": s.instance}) != nil {
		return
	}
	for _, raw := range pending {
		if send(raw) != nil {
			return
		}
	}
	_ = conn.SetReadDeadline(time.Time{})
	go func() {
		defer cancel()
		for scanner.Scan() {
			command, err := bridge.DecodeCommand(scanner.Bytes())
			if err != nil || !cliAllowed(command.Method) {
				return
			}
			s.mu.Lock()
			if len(lease.pending) >= 128 || s.nextCLIID >= cliIDEnd {
				s.mu.Unlock()
				return
			}
			if s.nextCLIID < cliIDStart {
				s.nextCLIID = cliIDStart
			} else {
				s.nextCLIID++
			}
			id := s.nextCLIID
			lease.pending[id] = command.ID
			s.mu.Unlock()
			command.ID = id
			raw, err := json.Marshal(command)
			if err != nil {
				return
			}
			s.childMu.Lock()
			if ctx.Err() != nil {
				s.childMu.Unlock()
				return
			}
			_, err = s.child.Write(append(raw, '\n'))
			s.childMu.Unlock()
			if err != nil {
				s.stop()
				return
			}
		}
	}()
	for {
		entries, wake, err := s.Journal.After(cursor)
		if err != nil {
			return
		}
		for _, entry := range entries {
			var frame struct {
				Event string `json:"event"`
			}
			if json.Unmarshal(entry.Message, &frame) == nil && frame.Event != "" {
				if send(entry.Message) != nil {
					return
				}
			}
			cursor = entry.Seq
		}
		select {
		case <-ctx.Done():
			return
		case raw := <-lease.replies:
			if send(raw) != nil {
				return
			}
		case <-wake:
		}
	}
}

func cliAllowed(method string) bool {
	switch method {
	case "cli/native", "cli/native/end", "cli/agents/check", "agents/list", "models/list", "threads/list", "threads/start", "threads/resume", "threads/read", "threads/status", "threads/options", "threads/options/set", "turn/start", "turn/interrupt", "decision/respond":
		return true
	}
	return false
}

// CLIRequest identifies commands assigned by the authenticated local CLI relay.
func CLIRequest(id int) bool { return id >= cliIDStart && id <= cliIDEnd }
