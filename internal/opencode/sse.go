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
	pollIdle     = 10 * time.Second
	pollMessages = 20
)

// Stream subscribes to OpenCode's /api/event SSE feed and reconciles missed
// terminal states by polling the active session. It reconnects with bounded
// backoff and dedupes replayed event ids.
type Stream struct {
	client *Client
	synth  *Synthesizer

	seenMu    sync.Mutex
	seen      map[string]bool
	seenOrder []string
}

func NewStream(client *Client, synth *Synthesizer) *Stream {
	return &Stream{client: client, synth: synth, seen: map[string]bool{}}
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
	}
	if json.Unmarshal([]byte(payload), &envelope) != nil || envelope.Type == "" {
		return
	}
	if envelope.ID != "" {
		if s.alreadySeen(envelope.ID) {
			return
		}
	}
	properties := envelope.Data
	if len(properties) == 0 {
		properties = envelope.Properties
	}
	if len(properties) == 0 {
		properties = json.RawMessage("null")
	}
	s.synth.Handle(envelope.Type, properties)
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
	var lastIdle time.Time
	for {
		sessionID, inFlight := s.synth.ActiveSession()
		if sessionID != "" && (inFlight || time.Since(lastIdle) >= pollIdle) {
			s.reconcile(ctx, sessionID, inFlight)
			if !inFlight {
				lastIdle = time.Now()
			}
		}
		// Wake every second so a newly started turn is reconciled promptly;
		// idle sessions only issue requests once per pollIdle.
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollActive):
		}
	}
}

func (s *Stream) reconcile(ctx context.Context, sessionID string, inFlight bool) {
	session, err := s.client.ReadSession(ctx, sessionID)
	if err != nil {
		return
	}
	page, err := s.client.ListMessagesPage(ctx, sessionID, pollMessages, "desc", "")
	if err != nil {
		return
	}
	reverseMessages(page.Data)
	if inFlight {
		// M4 projects permission requests; M3 only keeps the seam warm.
		_, _ = s.client.ListPermissions(ctx, sessionID)
	}
	s.synth.Reconcile(session, page.Data)
}
