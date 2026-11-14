package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/warmdev/modex/internal/bridge"
	"github.com/warmdev/modex/internal/diagnostic"
)

// One backend owns all managed sessions. Native TUI connections are views of it,
// not process owners; losing a view must never cancel a dispatched turn.
type sharedBackend struct {
	home           string
	ctx            context.Context
	p              *bridge.Process
	decisions      *bridge.Decisions
	initialization json.RawMessage
	mu             sync.Mutex
	clients        map[*nativeClient]bool
	pending        map[string]bridge.Decision
	nativeParams   map[string]json.RawMessage
	emit           func(string, json.RawMessage)
}

type nativeClient struct {
	conn      *websocket.Conn
	out       chan []byte
	slots     chan struct{}
	mu        sync.Mutex
	threads   map[string]bool
	delivered map[string]bool
	queued    int
}

func newSharedBackend(ctx context.Context, p *bridge.Process, emit func(string, json.RawMessage)) (*sharedBackend, error) {
	s := &sharedBackend{ctx: ctx, p: p, clients: map[*nativeClient]bool{}, pending: map[string]bridge.Decision{}, nativeParams: map[string]json.RawMessage{}, emit: emit}
	s.decisions = bridge.NewDecisions(p.Respond)
	p.SetRequestHandler(func(id json.RawMessage, method string, params json.RawMessage) {
		d, err := s.decisions.Add(id, method, params)
		if err != nil {
			var meta struct {
				ThreadID string `json:"threadId"`
				TurnID   string `json:"turnId"`
			}
			_ = json.Unmarshal(params, &meta)
			s.mu.Lock()
			clients := make([]*nativeClient, 0, len(s.clients))
			for c := range s.clients {
				clients = append(clients, c)
			}
			s.mu.Unlock()
			available := false
			for _, c := range clients {
				if c.accepts(params) {
					available = true
					break
				}
			}
			if !available || !validNativeID(id) {
				_ = p.Reject(id)
				return
			}
			d = bridge.Decision{Method: method, ThreadID: meta.ThreadID, TurnID: meta.TurnID}
		}
		s.mu.Lock()
		if len(s.pending) >= 128 {
			s.mu.Unlock()
			_ = p.Reject(id)
			return
		}
		s.pending[string(id)] = d
		s.nativeParams[string(id)] = append(json.RawMessage(nil), params...)
		s.mu.Unlock()
		if d.ID != "" {
			raw, _ := json.Marshal(d)
			s.event("decision/required", raw)
		}
		s.broadcast(method, params, id)
	})
	p.SetNotificationHandler(func(method string, params json.RawMessage) {
		s.decisions.Resolve(method, params)
		var meta struct {
			ThreadID  string          `json:"threadId"`
			RequestID json.RawMessage `json:"requestId"`
			Turn      struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		_ = json.Unmarshal(params, &meta)
		s.mu.Lock()
		resolved := []string{}
		for key, d := range s.pending {
			if d.ThreadID == meta.ThreadID && ((method == "turn/completed" && d.TurnID == meta.Turn.ID) || (method == "serverRequest/resolved" && key == string(meta.RequestID))) {
				delete(s.pending, key)
				delete(s.nativeParams, key)
				if d.ID != "" {
					resolved = append(resolved, d.ID)
				}
			}
		}
		s.mu.Unlock()
		for _, id := range resolved {
			s.event("decision/resolved", mustJSON(map[string]string{"decision_id": id}))
		}
		s.event(method, params)
		s.broadcast(method, params, nil)
	})
	initCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	raw, err := p.Call(initCtx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "modex_host", "version": "0.0.2"}, "capabilities": map[string]bool{"experimentalApi": true}})
	if err != nil {
		return nil, err
	}
	s.initialization = raw
	if err = p.Notify("initialized", nil); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *sharedBackend) event(method string, params json.RawMessage) {
	s.mu.Lock()
	emit := s.emit
	s.mu.Unlock()
	if emit != nil {
		emit(method, params)
	}
}

func (s *sharedBackend) call(ctx context.Context, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error) {
	if emit != nil {
		s.mu.Lock()
		s.emit = emit
		s.mu.Unlock()
	}
	values, _ := params.(map[string]any)
	if method == "client/takeover" {
		id, _ := values["threadId"].(string)
		return requestTakeover(ctx, s.home, id)
	}
	if method == "client/attached" {
		return json.RawMessage(`{"updated":true}`), nil
	}
	if method == "decision/respond" {
		id, _ := values["decisionId"].(string)
		result, _ := values["result"].(json.RawMessage)
		if err := s.answer(ctx, id, result); err != nil {
			return nil, err
		}
		return json.RawMessage(`{"delivered":true}`), nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	raw, err := s.p.CallPersistent(requestCtx, method, params)
	var rejection *diagnostic.RPCError
	if errors.As(err, &rejection) && rejection.Category() == "history_pagination_unsupported" {
		if method == "thread/resume" {
			// Legacy JSONL sessions cannot fulfill excludeTurns. Only retry this
			// explicit metadata rejection, never an uncertain turn dispatch.
			copy := make(map[string]any, len(values))
			for k, v := range values {
				if k != "excludeTurns" {
					copy[k] = v
				}
			}
			return s.p.CallPersistent(requestCtx, method, copy)
		}
		if method == "thread/turns/list" {
			return s.legacyHistory(requestCtx, values)
		}
	}
	return raw, err
}

func (s *sharedBackend) answer(ctx context.Context, id string, result json.RawMessage) error {
	if err := s.decisions.Answer(ctx, id, result); err != nil {
		return err
	}
	s.mu.Lock()
	var upstream json.RawMessage
	for key, d := range s.pending {
		if d.ID == id {
			upstream = json.RawMessage(key)
			delete(s.pending, key)
			delete(s.nativeParams, key)
			break
		}
	}
	s.mu.Unlock()
	if upstream != nil {
		params, _ := json.Marshal(map[string]any{"requestId": upstream, "decisionId": id})
		s.broadcast("serverRequest/resolved", params, nil)
		s.event("serverRequest/resolved", params)
		s.event("decision/resolved", mustJSON(map[string]string{"decision_id": id}))
	}
	return nil
}

func validNativeID(id json.RawMessage) bool {
	var text string
	var number int64
	return (json.Unmarshal(id, &text) == nil && text != "") || (string(id) != "null" && json.Unmarshal(id, &number) == nil)
}

func (s *sharedBackend) answerNative(ctx context.Context, upstream json.RawMessage, result json.RawMessage) error {
	var object map[string]json.RawMessage
	if json.Unmarshal(result, &object) != nil || object == nil {
		return errors.New("native result must be an object")
	}
	s.mu.Lock()
	d, ok := s.pending[string(upstream)]
	if !ok {
		s.mu.Unlock()
		return errors.New("request already resolved")
	}
	if d.ID == "" {
		delete(s.pending, string(upstream))
		delete(s.nativeParams, string(upstream))
	}
	s.mu.Unlock()
	if d.ID != "" {
		if err := s.decisions.AnswerNative(ctx, d.ID, result); err != nil {
			return err
		}
		s.mu.Lock()
		delete(s.pending, string(upstream))
		delete(s.nativeParams, string(upstream))
		s.mu.Unlock()
		s.event("decision/resolved", mustJSON(map[string]string{"decision_id": d.ID}))
	} else if err := s.p.Respond(ctx, upstream, result); err != nil {
		return errors.New("native decision delivery failed; do not replay")
	}
	s.broadcast("serverRequest/resolved", mustJSON(map[string]any{"threadId": d.ThreadID, "requestId": upstream}), nil)
	return nil
}

func (c *nativeClient) accepts(params json.RawMessage) bool {
	var meta struct {
		ThreadID string `json:"threadId"`
		Thread   struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	_ = json.Unmarshal(params, &meta)
	if meta.ThreadID == "" {
		meta.ThreadID = meta.Thread.ID
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return meta.ThreadID == "" || c.threads[meta.ThreadID]
}

func (c *nativeClient) send(raw []byte) {
	c.mu.Lock()
	if c.queued+len(raw) > 2*bridge.MaxResponseFrame {
		c.mu.Unlock()
		_ = c.conn.CloseNow()
		return
	}
	c.queued += len(raw)
	c.mu.Unlock()
	select {
	case c.out <- raw:
	default:
		c.mu.Lock()
		c.queued -= len(raw)
		c.mu.Unlock()
		_ = c.conn.CloseNow()
	}
}

func (s *sharedBackend) broadcast(method string, params, id json.RawMessage) {
	raw, _ := json.Marshal(struct {
		ID     json.RawMessage `json:"id,omitempty"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}{id, method, params})
	s.mu.Lock()
	clients := make([]*nativeClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.Unlock()
	for _, c := range clients {
		if c.accepts(params) {
			if len(id) > 0 {
				c.mu.Lock()
				seen := c.delivered[string(id)]
				c.delivered[string(id)] = true
				c.mu.Unlock()
				if seen {
					continue
				}
			}
			if method == "serverRequest/resolved" {
				var resolved struct {
					Request json.RawMessage `json:"requestId"`
				}
				_ = json.Unmarshal(params, &resolved)
				c.mu.Lock()
				delete(c.delivered, string(resolved.Request))
				c.mu.Unlock()
			}
			c.send(raw)
		}
	}
}

func (s *sharedBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4 * 1024 * 1024)
	c := &nativeClient{conn: conn, out: make(chan []byte, 64), slots: make(chan struct{}, 32), threads: map[string]bool{}, delivered: map[string]bool{}}
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				_ = conn.CloseNow()
				return
			case raw := <-c.out:
				c.mu.Lock()
				c.queued -= len(raw)
				c.mu.Unlock()
				writeCtx, stop := context.WithTimeout(ctx, 5*time.Second)
				err := conn.Write(writeCtx, websocket.MessageText, raw)
				stop()
				if err != nil {
					_ = conn.CloseNow()
					return
				}
			}
		}
	}()
	defer func() { s.mu.Lock(); delete(s.clients, c); s.mu.Unlock() }()
	initialized := false
	for {
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
		}
		if wsjson.Read(ctx, conn, &m) != nil {
			return
		}
		if m.Method == "initialized" {
			continue
		}
		if len(m.ID) == 0 || string(m.ID) == "null" {
			continue
		}
		if !validNativeID(m.ID) {
			return
		}
		if !initialized {
			if m.Method != "initialize" {
				c.reply(m.ID, nil, errors.New("initialize is required"))
				continue
			}
			initialized = true
			c.reply(m.ID, s.initialization, nil)
			s.mu.Lock()
			s.clients[c] = true
			s.mu.Unlock()
			continue
		}
		if m.Method == "" {
			s.mu.Lock()
			d, ok := s.pending[string(m.ID)]
			s.mu.Unlock()
			c.mu.Lock()
			delivered := c.delivered[string(m.ID)]
			c.mu.Unlock()
			if ok && delivered && c.accepts(mustJSON(map[string]string{"threadId": d.ThreadID})) {
				_ = s.answerNative(s.ctx, m.ID, m.Result)
			}
			continue
		}
		if m.Method == "initialize" {
			c.reply(m.ID, nil, errors.New("already initialized"))
			continue
		}
		var params map[string]any
		if len(m.Params) > 0 && string(m.Params) != "null" && json.Unmarshal(m.Params, &params) != nil {
			c.reply(m.ID, nil, errors.New("invalid params"))
			continue
		}
		if params == nil {
			params = map[string]any{}
		}
		id, _ := params["threadId"].(string)
		// Subscriptions belong to each view. Unsubscribing the shared upstream
		// would unload the session for every other client.
		if m.Method == "thread/unsubscribe" {
			c.mu.Lock()
			delete(c.threads, id)
			c.mu.Unlock()
			c.reply(m.ID, json.RawMessage(`{}`), nil)
			continue
		}
		select {
		case c.slots <- struct{}{}:
		default:
			c.reply(m.ID, nil, errors.New("too many pending requests"))
			continue
		}
		if m.Method == "thread/resume" || m.Method == "thread/start" || m.Method == "thread/fork" {
			c.mu.Lock()
			if id != "" {
				c.threads[id] = true
			}
			c.mu.Unlock()
		}
		go func(id json.RawMessage, method string, params map[string]any) {
			defer func() { <-c.slots }()
			// A disconnected TUI cannot cancel a request already dispatched to Codex.
			raw, err := s.call(s.ctx, method, params, nil)
			if method == "thread/resume" || method == "thread/start" || method == "thread/fork" {
				var result struct {
					Thread struct {
						ID string `json:"id"`
					} `json:"thread"`
				}
				_ = json.Unmarshal(raw, &result)
				c.mu.Lock()
				if result.Thread.ID != "" {
					c.threads[result.Thread.ID] = true
				}
				c.mu.Unlock()
				if err == nil {
					if method == "thread/start" || method == "thread/fork" {
						s.broadcast("thread/started", mustJSON(map[string]any{"thread": json.RawMessage(extractThread(raw))}), nil)
					}
					s.replayDecisions(c)
				}
			}
			c.reply(id, raw, err)
		}(append(json.RawMessage(nil), m.ID...), m.Method, params)
	}
}

func mustJSON(value any) json.RawMessage { raw, _ := json.Marshal(value); return raw }

func extractThread(raw json.RawMessage) json.RawMessage {
	var r struct {
		Thread json.RawMessage `json:"thread"`
	}
	_ = json.Unmarshal(raw, &r)
	return r.Thread
}

func (s *sharedBackend) replayDecisions(c *nativeClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, d := range s.pending {
		params := s.nativeParams[id]
		if !c.accepts(params) {
			continue
		}
		c.mu.Lock()
		seen := c.delivered[id]
		c.delivered[id] = true
		c.mu.Unlock()
		if !seen {
			c.send(mustJSON(map[string]any{"id": json.RawMessage(id), "method": d.Method, "params": params}))
		}
	}
}

func (c *nativeClient) reply(id, result json.RawMessage, err error) {
	if err != nil {
		c.send(mustJSON(map[string]any{"id": id, "error": map[string]any{"code": -32000, "message": "Modex backend request failed; refresh session before retrying"}}))
		return
	}
	c.send(mustJSON(map[string]any{"id": id, "result": result}))
}

func (s *sharedBackend) listen(socket string) (func(), error) {
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(socket, 0600); err != nil {
		_ = listener.Close()
		return nil, err
	}
	server := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	return func() {
		_ = server.Close()
		s.mu.Lock()
		for c := range s.clients {
			_ = c.conn.CloseNow()
		}
		s.mu.Unlock()
	}, nil
}
