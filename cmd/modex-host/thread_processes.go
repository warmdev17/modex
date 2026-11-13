package main

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/warmdev/modex/internal/bridge"
)

type threadWorker struct {
	process   *bridge.Process
	decisions *bridge.Decisions
	key, turn string
	fresh     bool
	busy      bool
}

// Each writer has its own process: closing an idle writer must not interrupt another turn.
type threadProcesses struct {
	ctx        context.Context
	root       *bridge.Process
	emit       func(string, json.RawMessage)
	mu         sync.Mutex
	workers    map[string]*threadWorker
	sequence   uint64
	attached   bool
	responseMu sync.RWMutex
	responders map[string]*bridge.Decisions
}

func newThreadProcesses(ctx context.Context, root *bridge.Process, emit func(string, json.RawMessage)) *threadProcesses {
	return &threadProcesses{ctx: ctx, root: root, emit: emit, workers: map[string]*threadWorker{}, responders: map[string]*bridge.Decisions{}, attached: true}
}

func (s *threadProcesses) start(ctx context.Context) (*threadWorker, error) {
	p, err := s.root.Fork(s.ctx)
	if err != nil {
		return nil, err
	}
	s.sequence++
	w := &threadWorker{process: p, key: strconv.FormatUint(s.sequence, 10)}
	w.decisions = bridge.NewDecisions(p.Respond)
	s.responseMu.Lock()
	s.responders[w.key] = w.decisions
	s.responseMu.Unlock()
	p.SetRequestHandler(func(id json.RawMessage, method string, params json.RawMessage) {
		decision, err := w.decisions.Add(id, method, params)
		if err != nil {
			if p.Reject(id) != nil {
				p.Close()
			}
			return
		}
		decision.ID = w.key + "/" + decision.ID
		raw, _ := json.Marshal(decision)
		s.emit("decision/required", raw)
	})
	p.SetNotificationHandler(func(method string, params json.RawMessage) {
		w.decisions.Resolve(method, params)
		s.emit(method, params)
		if method == "turn/completed" {
			go s.complete(w, params)
		}
	})
	if err = p.Initialize(ctx); err != nil {
		s.closeWorker(w, false)
		return nil, err
	}
	return w, nil
}

func (s *threadProcesses) closeWorker(w *threadWorker, idle bool) {
	if idle {
		w.process.CloseIdle()
	} else {
		w.process.Close()
	}
	s.responseMu.Lock()
	delete(s.responders, w.key)
	s.responseMu.Unlock()
}

func (s *threadProcesses) Answer(ctx context.Context, id string, result json.RawMessage) error {
	key, local, ok := strings.Cut(id, "/")
	if !ok {
		return errors.New("invalid worker decision")
	}
	s.responseMu.RLock()
	decisions := s.responders[key]
	s.responseMu.RUnlock()
	if decisions == nil {
		return errors.New("session decision is no longer available")
	}
	return decisions.Answer(ctx, local, result)
}

func (s *threadProcesses) complete(w *threadWorker, params json.RawMessage) {
	var event struct {
		ThreadID string `json:"threadId"`
		Turn     struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if json.Unmarshal(params, &event) != nil || event.ThreadID == "" || event.Turn.ID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// A stale completion must never close the worker of a newer turn.
	if s.workers[event.ThreadID] != w || w.turn != event.Turn.ID {
		return
	}
	w.busy = false
	w.fresh = false
	if !s.attached {
		delete(s.workers, event.ThreadID)
		s.closeWorker(w, true)
	}
}

func (s *threadProcesses) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, _ := params.(map[string]any)
	id, _ := p["threadId"].(string)
	if method == "client/attached" {
		attached, ok := p["attached"].(bool)
		if !ok {
			return nil, errors.New("attached must be boolean")
		}
		s.attached = attached
		if !attached {
			for id, worker := range s.workers {
				if !worker.busy && !worker.fresh {
					delete(s.workers, id)
					s.closeWorker(worker, true)
				}
			}
		}
		return json.RawMessage(`{"updated":true}`), nil
	}
	if method == "turn/interrupt" {
		turn, _ := p["turnId"].(string)
		w := s.workers[id]
		if w == nil || !w.busy || turn == "" || (w.turn != "" && w.turn != turn) {
			return nil, errors.New("active turn is not owned by this supervisor; refresh session state")
		}
		return w.process.Call(ctx, method, params)
	}
	if method != "thread/start" && method != "thread/resume" && method != "turn/start" && method != "thread/read" {
		return s.root.Call(ctx, method, params)
	}
	w := s.workers[id]
	if method == "thread/read" {
		if w == nil {
			return s.root.Call(ctx, method, params)
		}
		return w.process.Call(ctx, method, params)
	}
	created := w == nil
	if created {
		var err error
		w, err = s.start(ctx)
		if err != nil {
			return nil, err
		}
		if method == "turn/start" {
			raw, err := w.process.Call(ctx, "thread/resume", map[string]any{"threadId": id, "excludeTurns": true})
			var result struct {
				Thread struct {
					ID string `json:"id"`
				} `json:"thread"`
			}
			if err != nil {
				s.closeWorker(w, false)
				return nil, err
			}
			if json.Unmarshal(raw, &result) != nil || result.Thread.ID != id {
				s.closeWorker(w, false)
				return nil, errors.New("invalid reacquired session")
			}
			s.workers[id] = w
		}
	}
	if method == "turn/start" {
		w.busy = true
	}
	raw, err := w.process.Call(ctx, method, params)
	if err != nil {
		// A turn/start error can be an uncertain dispatch; preserve its worker until shutdown.
		if created && method != "turn/start" {
			s.closeWorker(w, false)
		}
		return nil, err
	}
	if method == "turn/start" {
		var r struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if json.Unmarshal(raw, &r) == nil {
			w.turn = r.Turn.ID
			w.busy = true
		}
		return raw, nil
	}
	var r struct {
		Thread struct {
			ID     string `json:"id"`
			Status struct {
				Type string `json:"type"`
			} `json:"status"`
		} `json:"thread"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Thread.ID == "" {
		if created {
			s.closeWorker(w, false)
		}
		return nil, errors.New("invalid session response")
	}
	s.workers[r.Thread.ID] = w
	if method == "thread/start" {
		w.fresh = true
	}
	if method == "thread/resume" {
		w.busy = r.Thread.Status.Type != "idle"
	}
	if !s.attached && !w.busy && !w.fresh {
		delete(s.workers, r.Thread.ID)
		s.closeWorker(w, true)
	}
	return raw, nil
}

func (s *threadProcesses) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, w := range s.workers {
		s.closeWorker(w, false)
		delete(s.workers, id)
	}
}
