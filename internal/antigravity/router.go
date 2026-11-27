// Package antigravity adapts ACP sessions to the Modex chat protocol.
package antigravity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/warmdev/modex/internal/acp"
	"github.com/warmdev/modex/internal/bridge"
)

const prefix = "agy:"
const maxState = 4 << 20

type record struct {
	ID          string `json:"id"`
	Native      string `json:"native_id"`
	Workspace   string `json:"workspace"`
	Project     string `json:"project_id,omitempty"`
	Projectless bool   `json:"projectless,omitempty"`
}
type message struct {
	ID   string `json:"id"`
	Role string `json:"role"`
	Text string `json:"text"`
	Turn string `json:"turn_id,omitempty"`
}
type chat struct {
	record
	loaded   bool
	loading  bool
	active   string
	messages []message
	emit     func(string, json.RawMessage)
}
type pending struct {
	request acp.PermissionRequest
	thread  string
	client  *acp.Client
	event   map[string]any
}
type Router struct {
	ctx                      context.Context
	cancel                   context.CancelFunc
	binary, workspace, state string
	mu                       sync.Mutex
	chats                    map[string]*chat
	clients                  map[string]*acp.Client
	pending                  map[string]pending
	loadErr                  error
}

func New(ctx context.Context, binary, defaultWorkspace, stateDir string) *Router {
	c, cancel := context.WithCancel(ctx)
	r := &Router{ctx: c, cancel: cancel, binary: binary, workspace: defaultWorkspace, state: filepath.Join(stateDir, "antigravity-threads.json"), chats: map[string]*chat{}, clients: map[string]*acp.Client{}, pending: map[string]pending{}}
	data, err := os.ReadFile(r.state)
	if err != nil && !os.IsNotExist(err) {
		r.loadErr = errors.New("cannot read Antigravity session state")
	}
	if err == nil {
		var records []record
		if len(data) > maxState || json.Unmarshal(data, &records) != nil {
			r.loadErr = errors.New("invalid Antigravity session state")
		} else {
			for _, v := range records {
				if !strings.HasPrefix(v.ID, prefix) || !valid(v.ID) || !valid(v.Native) || !filepath.IsAbs(v.Workspace) || r.chats[v.ID] != nil {
					r.loadErr = errors.New("invalid Antigravity session state")
					break
				}
				r.chats[v.ID] = &chat{record: v}
			}
		}
	}
	return r
}
func (r *Router) Close() {
	r.cancel()
	r.mu.Lock()
	clients := r.clients
	r.clients = map[string]*acp.Client{}
	r.mu.Unlock()
	for _, c := range clients {
		c.Close()
	}
}
func valid(s string) bool {
	return strings.TrimSpace(s) != "" && len(s) <= 4096 && !strings.ContainsAny(s, "\r\n\x00")
}
func uid() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func canonical(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("workspace must be absolute")
	}
	p, e := filepath.EvalSymlinks(path)
	if e != nil {
		return "", errors.New("workspace is unavailable")
	}
	info, e := os.Stat(p)
	if e != nil || !info.IsDir() {
		return "", errors.New("workspace is unavailable")
	}
	return p, nil
}
func (r *Router) saveLocked() error {
	if r.loadErr != nil {
		return r.loadErr
	}
	records := make([]record, 0, len(r.chats))
	for _, c := range r.chats {
		records = append(records, c.record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	data, e := json.Marshal(records)
	if e != nil || len(data) > maxState {
		return errors.New("Antigravity session state exceeds limit")
	}
	f, e := os.CreateTemp(filepath.Dir(r.state), ".antigravity-*")
	if e != nil {
		return errors.New("cannot save Antigravity session")
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	if e2 := f.Close(); e == nil {
		e = e2
	}
	if e == nil {
		e = os.Rename(name, r.state)
	}
	if e != nil {
		return errors.New("cannot save Antigravity session")
	}
	return nil
}
func thread(c *chat) map[string]any {
	m := map[string]any{"id": c.ID, "cwd": c.Workspace, "agent": "antigravity", "modelProvider": "antigravity", "status": map[string]any{"type": "idle"}}
	if c.Project != "" {
		m["projectId"] = c.Project
	} else if c.Projectless {
		m["projectless"] = true
	}
	if c.active != "" {
		m["status"] = map[string]any{"type": "active"}
	}
	return m
}
func session(c *chat) map[string]any {
	return map[string]any{"thread": thread(c), "model": "", "collaboration_mode": "default"}
}
func (r *Router) Threads() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]map[string]any, 0, len(r.chats))
	for _, c := range r.chats {
		out = append(out, thread(c))
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["id"].(string) < out[j]["id"].(string) })
	return out
}
func emitEvent(emit func(string, json.RawMessage), method string, params any) {
	if emit == nil {
		return
	}
	b, e := json.Marshal(params)
	if e == nil {
		emit(method, b)
	}
}
func (r *Router) client(workspace string) (*acp.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c := r.clients[workspace]; c != nil {
		return c, nil
	}
	c, e := acp.StartWithPermissions(r.ctx, r.binary, workspace, func(u acp.Update) { r.update(workspace, u) }, func(p acp.PermissionRequest) bool { return r.permission(workspace, p) })
	if e != nil {
		return nil, e
	}
	r.clients[workspace] = c
	return c, nil
}
func (r *Router) update(workspace string, u acp.Update) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var c *chat
	for _, candidate := range r.chats {
		if candidate.Workspace == workspace && candidate.Native == u.SessionID {
			c = candidate
			break
		}
	}
	if c == nil {
		return
	}
	if c.loading {
		role := ""
		switch u.Kind {
		case "user_message_chunk":
			role = "user"
		case "agent_message_chunk":
			role = "assistant"
		}
		if role != "" {
			if len(c.messages) > 0 && c.messages[len(c.messages)-1].Role == role {
				c.messages[len(c.messages)-1].Text += u.Text
			} else {
				c.messages = append(c.messages, message{ID: fmt.Sprintf("history-%d", len(c.messages)), Role: role, Text: u.Text})
			}
		}
		return
	}
	if c.active == "" || c.active != u.TurnID || u.Kind != "agent_message_chunk" {
		return
	}
	item := "assistant-" + u.TurnID
	if len(c.messages) > 0 && c.messages[len(c.messages)-1].ID == item {
		c.messages[len(c.messages)-1].Text += u.Text
	} else {
		c.messages = append(c.messages, message{ID: item, Role: "assistant", Turn: u.TurnID, Text: u.Text})
	}
	emitEvent(c.emit, "item/agentMessage/delta", map[string]any{"threadId": c.ID, "turnId": u.TurnID, "itemId": item, "delta": u.Text})
}
func (r *Router) permission(workspace string, p acp.PermissionRequest) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.chats {
		if c.Workspace == workspace && c.Native == p.SessionID && c.active == p.TurnID && c.active != "" && len(r.pending) < 128 {
			id, e := uid()
			if e != nil {
				return false
			}
			id = prefix + "decision:" + id
			choices := []string{}
			if p.AllowOption != "" {
				choices = append(choices, "accept")
			}
			if p.DenyOption != "" {
				choices = append(choices, "decline")
			}
			if len(choices) == 0 {
				return false
			}
			event := map[string]any{"decision_id": id, "method": "item/commandExecution/requestApproval", "thread_id": c.ID, "turn_id": c.active, "item_id": id, "context": map[string]any{"reason": p.Title, "availableDecisions": choices}}
			r.pending[id] = pending{request: p, thread: c.ID, client: r.clients[workspace], event: event}
			emitEvent(c.emit, "decision/required", event)
			return true
		}
	}
	return false
}
func (r *Router) load(ctx context.Context, c *chat) (*acp.Client, error) {
	client, e := r.client(c.Workspace)
	if e != nil {
		return nil, e
	}
	r.mu.Lock()
	if c.loaded {
		r.mu.Unlock()
		return client, nil
	}
	if c.loading {
		r.mu.Unlock()
		return nil, errors.New("session is loading")
	}
	c.loading = true
	c.messages = nil
	r.mu.Unlock()
	defer func() { r.mu.Lock(); c.loading = false; r.mu.Unlock() }()
	cursor := ""
	found := false
	for i := 0; i < 100; i++ {
		page, err := client.List(ctx, cursor)
		if err != nil {
			r.retire(c.Workspace, client)
			return nil, err
		}
		for _, s := range page.Sessions {
			if s.ID == c.Native {
				found = true
				break
			}
		}
		if found || page.NextCursor == "" {
			break
		}
		if page.NextCursor == cursor {
			return nil, errors.New("invalid Antigravity session cursor")
		}
		cursor = page.NextCursor
	}
	if !found {
		return nil, errors.New("Antigravity session was not found in workspace")
	}
	if e = client.Load(ctx, c.Native); e != nil {
		r.retire(c.Workspace, client)
		return nil, e
	}
	r.mu.Lock()
	c.loaded = true
	r.mu.Unlock()
	return client, nil
}
func (r *Router) readiness() map[string]any {
	entry := map[string]any{"id": "antigravity", "label": "Antigravity", "ready": false}
	ctx, cancel := context.WithTimeout(r.ctx, 8*time.Second)
	defer cancel()
	path, e := exec.LookPath(r.binary)
	if e != nil {
		entry["reason"] = "Install the agy CLI on the host"
		return entry
	}
	workspace, e := canonical(r.workspace)
	if e != nil {
		entry["reason"] = "Host workspace is unavailable"
		return entry
	}
	cmd := exec.CommandContext(ctx, path, "-p", "/model", "--output-format", "json")
	cmd.Dir = workspace
	var output bytes.Buffer
	cmd.Stdout = &limitedWriter{dst: &output, remaining: 65536}
	if e = cmd.Run(); e != nil {
		entry["reason"] = "Antigravity is unavailable or requires sign-in on the host"
		return entry
	}
	var probe struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(output.Bytes(), &probe) != nil || probe.Status != "SUCCESS" {
		entry["reason"] = "Antigravity is unavailable or requires sign-in on the host"
		return entry
	}
	c, e := acp.Start(ctx, path, workspace, nil)
	if e != nil {
		entry["reason"] = "Antigravity ACP is unavailable"
		return entry
	}
	info := c.Info()
	c.Close()
	if !info.LoadSession || !info.ListSessions {
		entry["reason"] = "Antigravity ACP does not support session recovery"
		return entry
	}
	entry["ready"] = true
	return entry
}

func (r *Router) Handle(ctx context.Context, cmd bridge.Command, emit func(string, json.RawMessage)) (any, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	p := map[string]json.RawMessage{}
	if len(cmd.Params) > 0 && string(cmd.Params) != "null" {
		if json.Unmarshal(cmd.Params, &p) != nil || p == nil {
			return nil, true, errors.New("params must be an object")
		}
	}
	str := func(key string) string { var s string; _ = json.Unmarshal(p[key], &s); return s }
	if cmd.Method == "agents/list" {
		return map[string]any{"data": []any{map[string]any{"id": "codex", "label": "Codex", "ready": true}, r.readiness()}}, true, nil
	}
	if cmd.Method == "client/attached" {
		if string(p["attached"]) == "true" {
			r.mu.Lock()
			for _, v := range r.pending {
				emitEvent(emit, "decision/required", v.event)
			}
			r.mu.Unlock()
		}
		return nil, false, nil
	}
	if cmd.Method == "decision/respond" {
		id := str("decision_id")
		if !strings.HasPrefix(id, prefix) {
			return nil, false, nil
		}
		r.mu.Lock()
		v, ok := r.pending[id]
		if !ok {
			r.mu.Unlock()
			return nil, true, errors.New("decision is missing or already resolved")
		}
		var answer struct {
			Decision string `json:"decision"`
		}
		decoder := json.NewDecoder(bytes.NewReader(p["result"]))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&answer) != nil || decoder.Decode(new(any)) != io.EOF {
			r.mu.Unlock()
			return nil, true, errors.New("invalid decision response")
		}
		option := ""
		switch answer.Decision {
		case "accept":
			option = v.request.AllowOption
		case "decline":
			option = v.request.DenyOption
		}
		if option == "" {
			r.mu.Unlock()
			return nil, true, errors.New("decision was not offered")
		}
		delete(r.pending, id)
		r.mu.Unlock()
		if v.client == nil {
			return nil, true, errors.New("decision delivery could not be confirmed")
		}
		if err := v.client.ReplyPermission(ctx, v.request.ID, option); err != nil {
			return nil, true, err
		}
		emitEvent(emit, "decision/resolved", map[string]any{"decision_id": id})
		return map[string]any{}, true, nil
	}
	if cmd.Method == "threads/start" && str("agent") == "antigravity" {
		for _, key := range []string{"model", "permission_mode"} {
			if _, ok := p[key]; ok {
				return nil, true, errors.New("unsupported Antigravity session option")
			}
		}
		workspace := r.workspace
		standalone := false
		if raw, ok := p["standalone"]; ok && (string(raw) == "null" || json.Unmarshal(raw, &standalone) != nil) {
			return nil, true, errors.New("standalone must be boolean")
		}
		if standalone {
			if _, ok := p["workspace"]; ok {
				return nil, true, errors.New("standalone cannot specify workspace")
			}
			if _, ok := p["project_id"]; ok {
				return nil, true, errors.New("standalone cannot specify project_id")
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, true, errors.New("home directory unavailable")
			}
			workspace = filepath.Join(home, "Documents", "Codex")
			if err := os.MkdirAll(workspace, 0700); err != nil {
				return nil, true, errors.New("standalone workspace unavailable")
			}
		} else if _, ok := p["workspace"]; ok {
			workspace = str("workspace")
		}
		workspace, err := canonical(workspace)
		if err != nil {
			return nil, true, err
		}
		project := str("project_id")
		if _, ok := p["project_id"]; ok && !valid(project) {
			return nil, true, errors.New("invalid project_id")
		}
		r.mu.Lock()
		loadErr := r.loadErr
		r.mu.Unlock()
		if loadErr != nil {
			return nil, true, loadErr
		}
		client, err := r.client(workspace)
		if err != nil {
			return nil, true, err
		}
		native, err := client.NewSession(ctx)
		if err != nil {
			r.retire(workspace, client)
			return nil, true, err
		}
		id, err := uid()
		if err != nil {
			return nil, true, err
		}
		c := &chat{record: record{ID: prefix + id, Native: native, Workspace: workspace, Project: project, Projectless: standalone}, loaded: true}
		r.mu.Lock()
		r.chats[c.ID] = c
		err = r.saveLocked()
		if err != nil {
			delete(r.chats, c.ID)
		}
		result := session(c)
		r.mu.Unlock()
		if err != nil {
			return nil, true, err
		}
		return result, true, nil
	}
	id := str("thread_id")
	if !strings.HasPrefix(id, prefix) {
		return nil, false, nil
	}
	r.mu.Lock()
	c := r.chats[id]
	r.mu.Unlock()
	if c == nil {
		return nil, true, errors.New("Antigravity session not found")
	}
	workspace := r.workspace
	if _, ok := p["workspace"]; ok {
		workspace = str("workspace")
	}
	workspace, err := canonical(workspace)
	if err != nil {
		return nil, true, err
	}
	if workspace != c.Workspace {
		return nil, true, errors.New("thread outside workspace")
	}
	switch cmd.Method {
	case "threads/resume":
		_, err := r.load(ctx, c)
		if err != nil {
			return nil, true, err
		}
		r.mu.Lock()
		result := session(c)
		r.mu.Unlock()
		return result, true, nil
	case "threads/status":
		r.mu.Lock()
		result := map[string]any{"session": session(c), "token_usage": nil, "rate_limits": nil}
		r.mu.Unlock()
		return result, true, nil
	case "threads/read":
		include := false
		if raw, ok := p["include_turns"]; ok && (string(raw) == "null" || json.Unmarshal(raw, &include) != nil) {
			return nil, true, errors.New("include_turns must be boolean")
		}
		if !include {
			r.mu.Lock()
			result := map[string]any{"thread": thread(c)}
			r.mu.Unlock()
			return result, true, nil
		}
		if _, err := r.load(ctx, c); err != nil {
			return nil, true, err
		}
		limit := 0
		if raw, ok := p["limit"]; ok {
			if json.Unmarshal(raw, &limit) != nil || limit < 1 || limit > 50 {
				return nil, true, errors.New("history limit must be 1..50")
			}
		} else if _, ok := p["cursor"]; ok {
			return nil, true, errors.New("history cursor requires limit")
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		msgs := append([]message{}, c.messages...)
		end := len(msgs)
		if cursor := str("cursor"); cursor != "" {
			n, e := strconv.Atoi(cursor)
			if e != nil || n < 0 || n > end {
				return nil, true, errors.New("invalid history cursor")
			}
			end = n
		}
		start := 0
		if limit > 0 && end > limit {
			start = end - limit
		}
		result := map[string]any{"thread": thread(c), "messages": msgs[start:end], "active_turn": c.active != "", "active_turn_id": c.active}
		if limit > 0 {
			b, _ := json.Marshal(msgs)
			sum := sha256.Sum256(b)
			result["history_revision"] = fmt.Sprintf("%x", sum)
			result["next_cursor"] = nil
			if start > 0 {
				result["next_cursor"] = strconv.Itoa(start)
			}
		}
		return result, true, nil
	case "turn/start":
		for _, key := range []string{"attachments", "references", "model", "effort"} {
			if _, ok := p[key]; ok {
				return nil, true, errors.New("unsupported Antigravity turn option")
			}
		}
		text := str("text")
		if strings.TrimSpace(text) == "" || len(text) > 1024*1024 {
			return nil, true, errors.New("invalid text prompt")
		}
		client, err := r.load(ctx, c)
		if err != nil {
			return nil, true, err
		}
		turn, err := uid()
		if err != nil {
			return nil, true, err
		}
		r.mu.Lock()
		if c.active != "" {
			r.mu.Unlock()
			return nil, true, errors.New("session busy")
		}
		c.active = turn
		c.emit = emit
		sent := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			promptCtx, cancelPrompt := context.WithCancel(r.ctx)
			defer cancelPrompt()
			timer := time.AfterFunc(20*time.Second, cancelPrompt)
			defer timer.Stop()
			stop, e := client.PromptWithSent(promptCtx, c.Native, turn, text, func() { timer.Stop(); close(sent) })
			if e != nil {
				select {
				case done <- e:
				default:
				}
				r.retire(c.Workspace, client)
				r.finish(c, turn, "failed")
				return
			}
			status := "completed"
			if stop == "cancelled" {
				status = "interrupted"
			}
			r.finish(c, turn, status)
		}()
		announce := func() {
			c.messages = append(c.messages, message{ID: "user-" + turn, Role: "user", Text: text, Turn: turn})
			emitEvent(emit, "turn/started", map[string]any{"threadId": id, "turn": map[string]any{"id": turn, "status": "inProgress"}})
			emitEvent(emit, "item/completed", map[string]any{"threadId": id, "turnId": turn, "item": map[string]any{"id": "user-" + turn, "type": "userMessage", "content": []any{map[string]any{"type": "text", "text": text}}}})
		}
		select {
		case <-sent:
			announce()
			r.mu.Unlock()
			return map[string]any{"turn": map[string]any{"id": turn, "status": "inProgress"}}, true, nil
		case e := <-done:
			r.mu.Unlock()
			return nil, true, e
		case <-ctx.Done():
			// Preserve event order if the write wins the race with the caller's deadline.
			go func() {
				select {
				case <-sent:
					announce()
				case <-done:
				}
				r.mu.Unlock()
			}()
			return nil, true, errors.New("Antigravity turn delivery is uncertain; refresh before retrying")
		}
	case "turn/interrupt":
		turn := str("turn_id")
		r.mu.Lock()
		active := c.active
		r.mu.Unlock()
		if turn == "" || turn != active {
			return nil, true, errors.New("Antigravity turn is no longer active")
		}
		client, err := r.client(c.Workspace)
		if err != nil {
			return nil, true, err
		}
		if err = client.Cancel(ctx, c.Native, turn); err != nil {
			return nil, true, err
		}
		return map[string]any{}, true, nil
	default:
		return nil, true, errors.New("operation is not supported for Antigravity sessions")
	}
}
func (r *Router) finish(c *chat, turn, status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.active != turn {
		return
	}
	for id, p := range r.pending {
		if p.thread == c.ID && p.request.TurnID == turn {
			delete(r.pending, id)
			emitEvent(c.emit, "decision/resolved", map[string]any{"decision_id": id})
		}
	}
	emitEvent(c.emit, "turn/completed", map[string]any{"threadId": c.ID, "turn": map[string]any{"id": turn, "status": status}})
	c.active = ""
}

// limitedWriter bounds probe output without retaining provider details beyond the check.
type limitedWriter struct {
	dst       io.Writer
	remaining int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if n > w.remaining {
		return 0, errors.New("probe output exceeds limit")
	}
	written, err := w.dst.Write(p)
	w.remaining -= written
	return written, err
}

func (r *Router) retire(workspace string, client *acp.Client) {
	r.mu.Lock()
	if r.clients[workspace] != client {
		r.mu.Unlock()
		return
	}
	delete(r.clients, workspace)
	for _, c := range r.chats {
		if c.Workspace == workspace {
			c.loaded = false
		}
	}
	r.mu.Unlock()
	client.Close()
}
