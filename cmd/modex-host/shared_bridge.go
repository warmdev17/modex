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
type sharedSession struct {
	settings map[string]json.RawMessage
	usage    json.RawMessage
	busy     bool
}

type sharedBackend struct {
	dispatchMu     sync.Mutex
	sessions       map[string]*sharedSession
	plans          *bridge.PlanActions
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
	s := &sharedBackend{ctx: ctx, p: p, clients: map[*nativeClient]bool{}, pending: map[string]bridge.Decision{}, sessions: map[string]*sharedSession{}, nativeParams: map[string]json.RawMessage{}, emit: emit}
	s.plans = bridge.NewPlanActions(os.Getenv("MODEX_PLAN_STATE_DIR"), func(ctx context.Context, method string, params any) (json.RawMessage, error) {
		return s.call(ctx, method, params, nil)
	})
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
		if d.ID != "" && s.emit != nil {
			s.emit("decision/required", mustJSON(d))
		}
		s.mu.Unlock()
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
		if invalidatesSession(method, params) {
			delete(s.sessions, meta.ThreadID)
		}
		if state := s.sessions[meta.ThreadID]; state != nil {
			switch method {
			case "turn/started":
				state.busy = true
			case "turn/completed":
				state.busy = false
			case "thread/tokenUsage/updated":
				var v struct {
					Usage json.RawMessage `json:"tokenUsage"`
				}
				if json.Unmarshal(params, &v) == nil {
					state.usage = bridge.SafeUsage(v.Usage)
				}
			case "thread/settings/updated":
				updateSharedSettings(state, params)
			}
		}
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
	id, _ := values["threadId"].(string)
	if method == "modex/thread/permissions" {
		s.dispatchMu.Lock()
		defer s.dispatchMu.Unlock()
		s.mu.Lock()
		state := s.sessions[id]
		if state == nil {
			s.mu.Unlock()
			return nil, bridge.ErrSessionNotOwned
		}
		busy := state.busy
		for _, d := range s.pending {
			busy = busy || d.ThreadID == id
		}
		s.mu.Unlock()
		if busy {
			return nil, bridge.ErrSessionBusy
		}
		requestCtx, cancel := modeRequestContext(ctx)
		defer cancel()
		return s.p.CallPersistent(requestCtx, "thread/settings/update", params)
	}
	if method == "modex/thread/mode" || method == "turn/start" || method == "thread/settings/update" || method == "thread/start" || method == "thread/resume" || method == "thread/fork" {
		s.dispatchMu.Lock()
		defer s.dispatchMu.Unlock()
	}
	if method == "modex/thread/status" || method == "modex/thread/mode" {
		s.mu.Lock()
		state := s.sessions[id]
		if state == nil {
			s.mu.Unlock()
			return nil, bridge.ErrSessionNotOwned
		}
		if method == "modex/thread/mode" {
			pending := false
			for _, d := range s.pending {
				if d.ThreadID == id {
					pending = true
					break
				}
			}
			if state.busy || pending {
				s.mu.Unlock()
				return nil, bridge.ErrSessionBusy
			}
			mode, _ := values["mode"].(string)
			model, _ := values["model"].(string)
			effort, _ := values["effort"].(string)
			s.mu.Unlock()
			var nativeEffort any
			if effort != "" {
				nativeEffort = effort
			}
			requestCtx, cancel := modeRequestContext(ctx)
			_, err := s.p.CallPersistent(requestCtx, "thread/settings/update", map[string]any{"threadId": id, "collaborationMode": map[string]any{"mode": mode, "settings": map[string]any{"model": model, "reasoning_effort": nativeEffort, "developer_instructions": nil}}})
			cancel()
			s.mu.Lock()
			state = s.sessions[id]
			if state == nil {
				s.mu.Unlock()
				return nil, bridge.ErrSessionNotOwned
			}
			if err != nil {
				delete(state.settings, "collaboration_mode")
				s.mu.Unlock()
				return nil, err
			}
			state.settings["collaboration_mode"], _ = json.Marshal(mode)
			state.settings["model"], _ = json.Marshal(model)
			if effort != "" {
				state.settings["effort"], _ = json.Marshal(effort)
			} else {
				delete(state.settings, "effort")
			}
		}
		raw, err := json.Marshal(map[string]any{"session": state.settings, "token_usage": state.usage})
		s.mu.Unlock()
		return raw, err
	}
	if method == "turn/start" {
		s.mu.Lock()
		if state := s.sessions[id]; state != nil {
			state.busy = true
		}
		s.mu.Unlock()
	}
	if method == "modex/plan/state" {
		handled, err := s.plans.Handled(stringValue(values, "threadId"), stringValue(values, "turnId"), stringValue(values, "itemId"))
		if err != nil {
			return nil, err
		}
		return mustJSON(map[string]bool{"handled": handled}), nil
	}
	if method == "modex/plan/respond" {
		raw, err := s.plans.Respond(ctx, values)
		if err != nil {
			return nil, err
		}
		s.event("plan/resolved", raw)
		return raw, nil
	}
	if method == "client/takeover" {
		id, _ := values["threadId"].(string)
		return requestTakeover(ctx, s.home, id)
	}
	if method == "client/attached" {
		if values["attached"] == true {
			s.mu.Lock()
			if s.emit != nil {
				// Serialize the reset and replay with pending-map changes.
				s.emit("decision/reset", json.RawMessage(`{}`))
				for _, d := range s.pending {
					if d.ID != "" {
						s.emit("decision/required", mustJSON(d))
					}
				}
			}
			s.mu.Unlock()
		}
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
	if method == "turn/start" && err != nil {
		var rejected *diagnostic.RPCError
		if errors.As(err, &rejected) {
			s.mu.Lock()
			if state := s.sessions[id]; state != nil {
				state.busy = false
			}
			s.mu.Unlock()
		}
	}

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
			raw, err = s.p.CallPersistent(requestCtx, method, copy)
		}
		if method == "thread/turns/list" {
			return s.legacyHistory(requestCtx, values)
		}
	}
	if err == nil && (method == "thread/start" || method == "thread/resume" || method == "thread/fork") {
		safe, e := bridge.SafeSession(raw)
		var session map[string]json.RawMessage
		var meta struct {
			Thread struct {
				ID     string `json:"id"`
				Status struct {
					Type string `json:"type"`
				} `json:"status"`
			} `json:"thread"`
		}
		if e == nil && json.Unmarshal(safe, &session) == nil && json.Unmarshal(raw, &meta) == nil && meta.Thread.ID != "" {
			s.mu.Lock()
			state := s.sessions[meta.Thread.ID]
			if state == nil {
				state = &sharedSession{}
				s.sessions[meta.Thread.ID] = state
			}
			state.settings = session
			state.busy = meta.Thread.Status.Type != "idle"
			s.mu.Unlock()
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

func (s *sharedBackend) answerNative(ctx context.Context, upstream json.RawMessage, result json.RawMessage, failed bool) error {
	var object map[string]json.RawMessage
	if json.Unmarshal(result, &object) != nil || object == nil {
		return errors.New("native result must be an object")
	}
	respond := s.p.Respond
	if failed {
		var code int
		var message string
		if json.Unmarshal(object["code"], &code) != nil || json.Unmarshal(object["message"], &message) != nil {
			return errors.New("invalid native error")
		}
		respond = s.p.RespondError
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
		if err := s.decisions.AnswerNativeWith(ctx, d.ID, result, respond); err != nil {
			return err
		}
		s.mu.Lock()
		delete(s.pending, string(upstream))
		delete(s.nativeParams, string(upstream))
		s.mu.Unlock()
		s.event("decision/resolved", mustJSON(map[string]string{"decision_id": d.ID}))
	} else if err := respond(ctx, upstream, result); err != nil {
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
			Error  json.RawMessage `json:"error"`
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
				if len(m.Error) > 0 && string(m.Error) != "null" {
					if len(m.Result) == 0 {
						_ = s.answerNative(s.ctx, m.ID, m.Error, true)
					}
				} else {
					_ = s.answerNative(s.ctx, m.ID, m.Result, false)
				}
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
		category, message := "request_rejected", "Modex backend request failed; refresh session before retrying"
		var rejection *diagnostic.RPCError
		if errors.As(err, &rejection) {
			category = rejection.Category()
			switch category {
			case "active_writer":
				message = "This session is open in another Codex backend (active writer). Close it in Codex Desktop or the standalone CLI, then reopen with modex; restarting the Modex service will not release that writer."
			case "unauthorized":
				message = "Codex backend authentication failed. Check credentials available to the host service; uncertain commands must not be replayed."
			case "model_provider_mismatch":
				message = "Codex backend rejected the model or provider. Check the host service configuration and session provider before retrying."
			}
		}
		c.send(mustJSON(map[string]any{"id": id, "error": map[string]any{"code": -32000, "message": message, "data": map[string]string{"category": category}}}))
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

func stringValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

// updateSharedSettings retains only the existing public permission and mode labels.
func updateSharedSettings(state *sharedSession, raw json.RawMessage) {
	var event struct {
		Settings struct {
			Mode struct {
				Mode string `json:"mode"`
			} `json:"collaborationMode"`
			Model    string          `json:"model"`
			Effort   string          `json:"effort"`
			Approval json.RawMessage `json:"approvalPolicy"`
			Reviewer string          `json:"approvalsReviewer"`
			Sandbox  json.RawMessage `json:"sandboxPolicy"`
			Profile  json.RawMessage `json:"activePermissionProfile"`
		} `json:"threadSettings"`
	}
	if state.settings == nil || json.Unmarshal(raw, &event) != nil {
		return
	}
	native, _ := json.Marshal(map[string]any{"collaborationMode": event.Settings.Mode, "model": event.Settings.Model, "reasoningEffort": event.Settings.Effort, "approvalPolicy": event.Settings.Approval, "approvalsReviewer": event.Settings.Reviewer, "sandbox": event.Settings.Sandbox, "activePermissionProfile": event.Settings.Profile})
	safe, err := bridge.SafeSession(native)
	if err != nil {
		return
	}
	var update map[string]json.RawMessage
	if json.Unmarshal(safe, &update) != nil {
		return
	}
	for _, key := range []string{"collaboration_mode", "effort", "approval_policy", "approvals_reviewer", "sandbox", "permission_profile"} {
		delete(state.settings, key)
	}
	for k, v := range update {
		if k != "thread" {
			state.settings[k] = v
		}
	}
}

func invalidatesSession(method string, raw json.RawMessage) bool {
	if method == "thread/closed" || method == "thread/archived" {
		return true
	}
	if method != "thread/status/changed" {
		return false
	}
	var v struct {
		Status struct {
			Type string `json:"type"`
		} `json:"status"`
	}
	return json.Unmarshal(raw, &v) == nil && v.Status.Type == "notLoaded"
}
