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
	"unicode"
	"unicode/utf8"

	"github.com/warmdev/modex/internal/acp"
	"github.com/warmdev/modex/internal/bridge"
	"github.com/warmdev/modex/internal/diagnostic"
)

const prefix = "agy:"
const maxState = 4 << 20

type record struct {
	ID          string `json:"id"`
	Native      string `json:"native_id"`
	Workspace   string `json:"workspace"`
	Project     string `json:"project_id,omitempty"`
	Projectless bool   `json:"projectless,omitempty"`
	Model       string `json:"model,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	Name        string `json:"name,omitempty"`
}
type message struct {
	ID          string                 `json:"id"`
	Role        string                 `json:"role"`
	Text        string                 `json:"text"`
	Turn        string                 `json:"turn_id,omitempty"`
	Status      string                 `json:"status,omitempty"`
	Notice      *diagnostic.TurnNotice `json:"turn_notice,omitempty"`
	Activity    *invocationItem        `json:"activity_item,omitempty"`
	Attachments []attachment           `json:"attachments,omitempty"`
}
type contextUsage struct {
	Last struct {
		TotalTokens int64 `json:"totalTokens"`
	} `json:"last"`
	ModelContextWindow int64 `json:"modelContextWindow"`
}
type chat struct {
	usage      *contextUsage
	usageModel string
	record
	loaded                bool
	loading               bool
	active                string
	messages              []message
	calls                 map[string]invocationTarget
	replayCalls           map[string]int
	setting, optionsDirty bool
	titleDirty            bool
	emit                  func(string, json.RawMessage)
	admitting             bool
	queued                []acp.Update
	segment               int
	finishPending         *turnOutcome
	queuedBytes           int
	queueOverflow         bool
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
func title(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 512 || !utf8.ValidString(s) {
		return ""
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return s
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
	for _, c := range r.chats {
		c.optionsDirty = false
		c.titleDirty = false
	}
	return nil
}
func thread(c *chat) map[string]any {
	m := map[string]any{"id": c.ID, "cwd": c.Workspace, "agent": "antigravity", "modelProvider": "antigravity", "status": map[string]any{"type": "idle"}}
	if name := title(c.Name); name != "" {
		m["name"] = name
	}
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
	var c *acp.Client
	var e error
	c, e = acp.StartWithPermissions(r.ctx, r.binary, workspace, func(u acp.Update) { r.update(workspace, &c, u) }, func(p acp.PermissionRequest) bool { return r.permission(workspace, p) })
	if e != nil {
		return nil, e
	}
	r.clients[workspace] = c
	return c, nil
}
func (r *Router) update(workspace string, source **acp.Client, u acp.Update) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var client *acp.Client
	if source != nil {
		client = *source
	}
	if r.clients[workspace] != client {
		return
	}
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
	if c.admitting {
		c.queuedBytes += len(u.Text) + len(u.URI) + len(u.ToolCallID) + 256
		if c.queuedBytes > 4<<20 || len(c.queued) >= 1024 {
			if !c.queueOverflow {
				c.queueOverflow = true
				if client := r.clients[workspace]; client != nil {
					go client.Close()
				}
			}
			return
		}
		c.queued = append(c.queued, u)
		return
	}
	r.updateLocked(c, u)
}

func (r *Router) setTitleLocked(c *chat, value string) {
	name := title(value)
	if name == "" {
		return
	}
	if name == c.Name {
		if c.titleDirty {
			_ = r.saveLocked()
		}
		return
	}
	c.Name = name
	c.titleDirty = true
	_ = r.saveLocked()
	emitEvent(c.emit, "thread/name/updated", map[string]any{"threadId": c.ID, "threadName": name})
}

func (r *Router) updateLocked(c *chat, u acp.Update) {
	if u.Kind == "model_update" {
		if c.usageModel != "" && c.usageModel != u.Model {
			r.clearUsageLocked(c)
		}
		c.usageModel = u.Model
		return
	}
	if u.Kind == "usage_update" {
		if u.Model != "" {
			if c.usageModel != "" && c.usageModel != u.Model {
				r.clearUsageLocked(c)
			}
			c.usageModel = u.Model
		}
		if u.Used < 0 || u.Size <= 0 {
			return
		}
		if c.usage != nil && c.usage.Last.TotalTokens == u.Used && c.usage.ModelContextWindow == u.Size {
			return
		}
		usage := &contextUsage{ModelContextWindow: u.Size}
		usage.Last.TotalTokens = u.Used
		c.usage = usage
		params := map[string]any{"threadId": c.ID, "tokenUsage": usage}
		if u.TurnID != "" {
			params["turnId"] = u.TurnID
		}
		emitEvent(c.emit, "thread/tokenUsage/updated", params)
		return
	}
	if u.Kind == "session_info_update" {
		r.setTitleLocked(c, u.Text)
		return
	}
	if u.ToolCallID != "" {
		c.updateInvocation(u)
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
		if role == "user" && u.URI != "" {
			a := replayAttachment(u.URI, u.MIME)
			if a != nil {
				if len(c.messages) == 0 || c.messages[len(c.messages)-1].Role != "user" {
					c.messages = append(c.messages, message{ID: fmt.Sprintf("history-%d", len(c.messages)), Role: "user"})
				}
				c.messages[len(c.messages)-1].Attachments = append(c.messages[len(c.messages)-1].Attachments, *a)
			}
			return
		}
		if role != "" {
			if len(c.messages) > 0 && c.messages[len(c.messages)-1].Role == role {
				c.messages[len(c.messages)-1].Text += u.Text
			} else {
				if role == "user" {
					clear(c.replayCalls)
				}
				c.messages = append(c.messages, message{ID: fmt.Sprintf("history-%d", len(c.messages)), Role: role, Text: u.Text})
			}
		}
		return
	}
	if c.active == "" || c.active != u.TurnID || u.Kind != "agent_message_chunk" {
		return
	}
	item := fmt.Sprintf("assistant-%s-%d", u.TurnID, c.segment)
	index := len(c.messages) - 1
	for index >= 0 && c.messages[index].Role == "agent_activity" && c.messages[index].Turn == u.TurnID {
		index--
	}
	if index >= 0 && c.messages[index].ID == item {
		c.messages[index].Text += u.Text
	} else {
		c.messages = append(c.messages, message{ID: item, Role: "assistant", Turn: u.TurnID, Text: u.Text})
	}
	emitEvent(c.emit, "item/agentMessage/delta", map[string]any{"threadId": c.ID, "turnId": u.TurnID, "itemId": item, "delta": u.Text})
}
func (r *Router) clearUsageLocked(c *chat) {
	if c.usage == nil {
		return
	}
	c.usage = nil
	emitEvent(c.emit, "thread/tokenUsage/updated", map[string]any{"threadId": c.ID, "tokenUsage": nil})
}
func (r *Router) flushUpdates(c *chat) {
	c.admitting = false
	queued := c.queued
	c.queued = nil
	c.queuedBytes = 0
	c.queueOverflow = false
	for _, u := range queued {
		r.updateLocked(c, u)
	}
	if c.finishPending != nil {
		outcome := *c.finishPending
		c.finishPending = nil
		r.finishLocked(c, c.active, outcome)
	}
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
	if c.titleDirty {
		_ = r.saveLocked()
	}
	if c.setting {
		r.mu.Unlock()
		return nil, errors.New("session options are being updated")
	}
	if c.optionsDirty {
		if err := r.saveLocked(); err != nil {
			r.mu.Unlock()
			return nil, err
		}
	}
	if c.loaded && client.SessionLoaded(c.Native) {
		r.mu.Unlock()
		return client, nil
	}
	if c.loading {
		r.mu.Unlock()
		return nil, errors.New("session is loading")
	}
	r.clearUsageLocked(c)
	c.usageModel = ""
	c.loading = true
	c.messages = nil
	c.calls = nil
	saved := c.record
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		c.loading = false
		c.replayCalls = nil
		r.mu.Unlock()
	}()
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
				r.mu.Lock()
				if c.Name == saved.Name {
					r.setTitleLocked(c, s.Title)
				}
				r.mu.Unlock()
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
	if e = restoreOptions(ctx, client, saved); e != nil {
		r.retire(c.Workspace, client)
		return nil, errors.New("Antigravity could not restore chat options; refresh before sending")
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
	case "threads/options", "threads/options/set":
		return r.handleOptions(ctx, c, cmd.Method, p)
	case "threads/resume":
		r.mu.Lock()
		if emit != nil {
			c.emit = emit
		}
		r.mu.Unlock()
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
		result := map[string]any{"session": session(c), "token_usage": c.usage, "rate_limits": nil}
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
		for _, key := range []string{"references", "model", "effort"} {
			if _, ok := p[key]; ok {
				return nil, true, errors.New("unsupported Antigravity turn option")
			}
		}
		text := str("text")
		if len(text) > 1024*1024 {
			return nil, true, errors.New("invalid text prompt")
		}
		client, err := r.load(ctx, c)
		if err != nil {
			return nil, true, err
		}
		blocks, attachments, err := prepareAttachments(p["attachments"], client.Info())
		if err != nil {
			return nil, true, err
		}
		if strings.TrimSpace(text) != "" {
			blocks = append([]acp.PromptBlock{{Type: "text", Text: text}}, blocks...)
		}
		if len(blocks) == 0 {
			return nil, true, errors.New("text or attachments are required")
		}
		turn, err := uid()
		if err != nil {
			return nil, true, err
		}
		r.mu.Lock()
		if c.active != "" || c.setting || c.optionsDirty || !c.loaded || !client.SessionLoaded(c.Native) {
			r.mu.Unlock()
			return nil, true, errors.New("session busy")
		}
		c.active = turn
		c.emit = emit
		c.calls = nil
		c.admitting = true
		c.segment = 0
		r.mu.Unlock()
		sent := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			promptCtx, cancelPrompt := context.WithCancel(r.ctx)
			defer cancelPrompt()
			timer := time.AfterFunc(20*time.Second, cancelPrompt)
			defer timer.Stop()
			stop, e := client.PromptBlocksWithSent(promptCtx, c.Native, turn, blocks, func() {
				timer.Stop()
				r.mu.Lock()
				c.messages = append(c.messages, message{ID: "user-" + turn, Role: "user", Text: text, Turn: turn, Attachments: attachments})
				emitEvent(emit, "turn/started", map[string]any{"threadId": id, "turn": map[string]any{"id": turn, "status": "inProgress"}})
				item := map[string]any{"id": "user-" + turn, "type": "userMessage", "content": []any{map[string]any{"type": "text", "text": text}}}
				if len(attachments) > 0 {
					item["attachments"] = attachments
				}
				emitEvent(emit, "item/completed", map[string]any{"threadId": id, "turnId": turn, "item": item})
				r.flushUpdates(c)
				r.mu.Unlock()
				close(sent)
			})
			if e != nil {
				r.mu.Lock()
				if c.admitting {
					r.flushUpdates(c)
				}
				r.mu.Unlock()
				select {
				case <-sent:
				default:
					done <- e
				}
				if !errors.Is(e, acp.ErrRequestRejected) && !errors.Is(e, acp.ErrAuthenticationRequired) {
					r.retire(c.Workspace, client)
				}
				r.finish(c, turn, antigravityOutcome("", e))
				return
			}
			r.finish(c, turn, antigravityOutcome(stop, nil))
		}()
		select {
		case <-sent:
			return map[string]any{"turn": map[string]any{"id": turn, "status": "inProgress"}}, true, nil
		case e := <-done:
			return nil, true, e
		case <-ctx.Done():
			return nil, true, errors.New("Antigravity turn delivery is uncertain; refresh before retrying")
		}
	case "turn/steer":
		for _, key := range []string{"references", "model", "effort", "reasoning", "attachments"} {
			if _, ok := p[key]; ok {
				return nil, true, errors.New("unsupported Antigravity steer option")
			}
		}
		text, turn := str("text"), str("turn_id")
		if strings.TrimSpace(text) == "" || len(text) > 1024*1024 || turn == "" {
			return nil, true, errors.New("invalid Antigravity steer")
		}
		r.mu.Lock()
		client := r.clients[c.Workspace]
		if client == nil || c.active != turn || c.admitting {
			r.mu.Unlock()
			return nil, true, errors.New("Antigravity turn is unavailable for steering")
		}
		c.admitting = true
		r.mu.Unlock()
		sent := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			promptCtx, cancelPrompt := context.WithCancel(r.ctx)
			defer cancelPrompt()
			timer := time.AfterFunc(20*time.Second, cancelPrompt)
			defer timer.Stop()
			e := client.SteerWithSent(promptCtx, c.Native, turn, text, func() {
				timer.Stop()
				r.mu.Lock()
				c.segment++
				itemID := fmt.Sprintf("user-%s-%d", turn, c.segment)
				c.messages = append(c.messages, message{ID: itemID, Role: "user", Text: text, Turn: turn})
				emitEvent(c.emit, "item/completed", map[string]any{"threadId": id, "turnId": turn, "item": map[string]any{"id": itemID, "type": "userMessage", "content": []any{map[string]any{"type": "text", "text": text}}}})
				r.flushUpdates(c)
				r.mu.Unlock()
				close(sent)
			})
			if e != nil {
				select {
				case <-sent:
				default:
					r.mu.Lock()
					if c.admitting {
						r.flushUpdates(c)
					}
					r.mu.Unlock()
					done <- e
				}
			}
		}()
		select {
		case <-sent:
			return map[string]any{"turnId": turn}, true, nil
		case e := <-done:
			return nil, true, e
		case <-ctx.Done():
			return nil, true, errors.New("Antigravity steer delivery is uncertain; refresh before retrying")
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

type turnOutcome struct {
	status string
	notice *diagnostic.TurnNotice
}

func antigravityOutcome(stop string, err error) turnOutcome {
	notice := diagnostic.TurnNotice{
		Category: "turn_failed",
		Message:  "Antigravity could not complete this turn. Review the conversation before trying again.",
	}
	if errors.Is(err, acp.ErrAuthenticationRequired) {
		notice.Category = "unauthorized"
		notice.Message = "Antigravity requires sign-in on the host. Sign in there and try again."
	}
	if err == nil {
		switch stop {
		case "end_turn":
			return turnOutcome{status: "completed"}
		case "cancelled":
			return turnOutcome{status: "interrupted"}
		case "max_tokens":
			notice.Category = "response_token_limit"
			notice.Message = "Antigravity reached the token limit before finishing. You can ask it to continue."
		case "max_turn_requests":
			notice.Category = "turn_request_limit"
			notice.Message = "Antigravity reached the request limit for this turn. You can continue in a new message."
		case "refusal":
			notice.Category = "agent_refusal"
			notice.Message = "Antigravity declined to continue this turn."
		}
	}
	return turnOutcome{status: "failed", notice: &notice}
}

func (r *Router) finish(c *chat, turn string, outcome turnOutcome) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.active != turn {
		return
	}
	if c.admitting {
		c.finishPending = &outcome
		return
	}
	r.finishLocked(c, turn, outcome)
}

func (r *Router) finishLocked(c *chat, turn string, outcome turnOutcome) {
	for _, target := range c.calls {
		if target.Status == "running" || target.Status == "pendingInit" {
			target.Status = "unknown"
			if outcome.status == "interrupted" {
				target.Status = "interrupted"
			}
			c.emitInvocation(turn, "completed", target)
		}
	}
	c.calls = nil
	for id, p := range r.pending {
		if p.thread == c.ID && p.request.TurnID == turn {
			delete(r.pending, id)
			emitEvent(c.emit, "decision/resolved", map[string]any{"decision_id": id})
		}
	}
	for i := len(c.messages) - 1; i >= 0 && c.messages[i].Turn == turn; i-- {
		if c.messages[i].Role == "user" {
			c.messages[i].Status = outcome.status
			c.messages[i].Notice = outcome.notice
		}
	}
	params := map[string]any{"threadId": c.ID, "turn": map[string]any{"id": turn, "status": outcome.status}}
	if outcome.notice != nil {
		params["notice"] = outcome.notice
	}
	emitEvent(c.emit, "turn/completed", params)
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
			r.clearUsageLocked(c)
			c.usageModel = ""
		}
	}
	r.mu.Unlock()
	client.Close()
}
