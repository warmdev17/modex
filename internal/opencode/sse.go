package opencode

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	sseRetryMin  = 500 * time.Millisecond
	sseRetryMax  = 8 * time.Second
	sseLineLimit = 4 << 20
	pollActive   = time.Second
	pollMessages = 20
)

// Stream subscribes to OpenCode's /api/event SSE feed and reconciles missed
// terminal states by polling the active session. It reconnects with bounded
// backoff and dedupes replayed event ids.
type Stream struct {
	client    *Client
	synth     *Synthesizer
	observers []func(typ string, properties json.RawMessage, directory string)

	// OnPermissions receives a session's pending-permission snapshot from the
	// polling reconciler (nil disables the M4 reconciliation).
	OnPermissions       func(sessionID, directory string, raw json.RawMessage)
	OnScopedPermissions func(sessionID, directory string, raw json.RawMessage, turnID string) bool
	Forms               *Decisions
	Filter              func(typ string, properties json.RawMessage, directory string) bool
	ScopedPoll          bool

	seenMu    sync.Mutex
	seen      map[string]bool
	seenOrder []string
}

func NewStream(client *Client, synth *Synthesizer, observers ...func(typ string, properties json.RawMessage, directory string)) *Stream {
	return &Stream{client: client, synth: synth, observers: observers, seen: map[string]bool{}}
}

// Run blocks until ctx is canceled. The polling reconciler runs alongside the
// SSE reader; neither goroutine outlives Run.
func (s *Stream) Run(ctx context.Context) error {
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		s.poll(ctx)
	}()
	defer workers.Wait()

	backoff := sseRetryMin
	for ctx.Err() == nil {
		started := time.Now()
		connected, _ := s.consume(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if connected && time.Since(started) > 5*time.Second {
			backoff = sseRetryMin
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if connected && time.Since(started) > 5*time.Second {
			continue
		}
		backoff *= 2
		if backoff > sseRetryMax {
			backoff = sseRetryMax
		}
	}
	return nil
}

func (s *Stream) consume(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.client.baseURL+"/api/event", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := s.client.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return false, errors.New("opencode event stream failed")
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), sseLineLimit)
	var data strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if data.Len() > 0 {
				s.dispatch(data.String())
				data.Reset()
			}
		case strings.HasPrefix(line, ":"):
			// SSE comment / keepalive.
		case strings.HasPrefix(line, "data:"):
			chunk := strings.TrimPrefix(line, "data:")
			chunk = strings.TrimPrefix(chunk, " ")
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(chunk)
		default:
			// event:, id:, retry: are not needed; the payload carries type/id.
		}
	}
	if data.Len() > 0 {
		s.dispatch(data.String())
	}
	return true, scanner.Err()
}

func (s *Stream) dispatch(payload string) {
	var envelope struct {
		ID         string          `json:"id"`
		Type       string          `json:"type"`
		Data       json.RawMessage `json:"data"`
		Properties json.RawMessage `json:"properties"`
		Location   struct {
			Directory string `json:"directory"`
		} `json:"location"`
	}
	if json.Unmarshal([]byte(payload), &envelope) != nil || envelope.Type == "" {
		return
	}
	properties := envelope.Data
	if len(properties) == 0 {
		properties = envelope.Properties
	}
	if len(properties) == 0 {
		properties = json.RawMessage("null")
	}
	if s.Filter != nil && !s.Filter(envelope.Type, properties, envelope.Location.Directory) {
		return
	}
	if envelope.ID != "" && s.alreadySeen(envelope.ID) {
		return
	}
	switch envelope.Type {
	case "session.execution.succeeded", "session.execution.failed", "session.execution.interrupted", "session.idle", "session.error":
		// These events carry only the session id. Polling requires an idle
		// marker after the accepted user message before closing its turn.
	default:
		s.synth.Handle(envelope.Type, properties)
	}
	for _, observer := range s.observers {
		observer(envelope.Type, properties, envelope.Location.Directory)
	}
}

func (s *Stream) alreadySeen(id string) bool {
	s.seenMu.Lock()
	defer s.seenMu.Unlock()
	if s.seen[id] {
		return true
	}
	s.seen[id] = true
	s.seenOrder = append(s.seenOrder, id)
	if len(s.seenOrder) > 4096 {
		delete(s.seen, s.seenOrder[0])
		s.seenOrder = s.seenOrder[1:]
	}
	return false
}

func (s *Stream) poll(ctx context.Context) {
	for {
		for _, sessionID := range s.synth.ActiveSessions() {
			if ctx.Err() != nil {
				return
			}
			s.reconcile(ctx, sessionID, true)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollActive):
		}
	}
}

func (s *Stream) reconcile(ctx context.Context, sessionID string, inFlight bool) {
	s.synth.mu.Lock()
	turn := s.synth.turns[sessionID]
	if turn == nil || turn.completed {
		s.synth.mu.Unlock()
		return
	}
	anchor := turn.historyAnchor
	s.synth.mu.Unlock()
	if !s.ScopedPoll {
		s.reconcileForms(ctx, sessionID, turn, inFlight)
	}
	session, err := s.client.ReadSession(ctx, sessionID)
	if err != nil || s.ScopedPoll && (session.ID != sessionID || session.Location.Directory != turn.dir) {
		return
	}
	var messages []message
	cursor := ""
	seen := map[string]bool{}
	limit := pollMessages
	anchored := false
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	// shortcut: recover at most 100 messages per poll from the last verified
	// history position; widen this window for larger gaps during outages.
	for len(messages) < 100 {
		page, err := s.client.ListMessagesPage(ctx, sessionID, min(limit, 100-len(messages)), "desc", cursor)
		if err != nil {
			return
		}
		messages = append(messages, page.Data...)
		for _, item := range page.Data {
			if item.ID == anchor {
				anchored = true
				break
			}
		}
		if anchored || len(page.Data) == 0 || page.Cursor.Next == nil || *page.Cursor.Next == "" {
			break
		}
		cursor = *page.Cursor.Next
		if seen[cursor] {
			return
		}
		seen[cursor] = true
		limit = min(limit, len(page.Data))
	}
	if s.ScopedPoll && !anchored {
		return
	}
	reverseMessages(messages)
	if s.ScopedPoll {
		s.reconcileForms(ctx, sessionID, turn, inFlight)
	}
	if inFlight {
		if permissions, err := s.client.ListPermissions(ctx, sessionID); err == nil {
			if s.ScopedPoll && s.OnScopedPermissions != nil {
				s.synth.mu.Lock()
				if s.synth.turns[sessionID] == turn && !turn.completed &&
					s.OnScopedPermissions(sessionID, session.Location.Directory, permissions, turn.userID) {
					s.synth.completeWithNotice(turn, "failed", "unsupported_input", pendingCapNotice)
				}
				s.synth.mu.Unlock()
			} else if !s.ScopedPoll && s.OnPermissions != nil {
				s.OnPermissions(sessionID, session.Location.Directory, permissions)
			}
		}
	}
	s.synth.reconcile(session, messages, turn)
}

func (s *Stream) reconcileForms(ctx context.Context, sessionID string, turn *turnState, inFlight bool) {
	if inFlight && s.Forms != nil {
		epoch := s.Forms.formEpoch()
		formCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		forms, err := s.client.listForms(formCtx, sessionID)
		cancel()
		if err == nil {
			s.synth.mu.Lock()
			if s.synth.turns[sessionID] == turn && !turn.completed {
				if notice := s.Forms.reconcileForms(sessionID, turn.userID, epoch, forms); notice != "" {
					s.synth.completeWithNotice(turn, "failed", "unsupported_input", notice)
				}
			}
			s.synth.mu.Unlock()
		}
	}
}
