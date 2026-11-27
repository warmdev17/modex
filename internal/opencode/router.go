package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/warmdev/modex/internal/bridge"
)

type managedWorkspace struct {
	client    *Client
	synth     *Synthesizer
	decisions *Decisions
}

var versionPattern = regexp.MustCompile(`^v?2\.([0-9]+)\.[0-9]+(?:\+[0-9A-Za-z.-]+)?$`)

func compatibleVersion(version string) bool {
	match := versionPattern.FindStringSubmatch(version)
	if match == nil {
		return false
	}
	minor, err := strconv.Atoi(match[1])
	return err == nil && minor >= 4
}

type Router struct {
	ctx              context.Context
	cancel           context.CancelFunc
	binary, cwd, dir string
	registry         *registry
	loadErr          error
	mu               sync.Mutex
	process          *Process
	workspaces       map[string]*managedWorkspace
	emit             func(string, json.RawMessage)
}

func NewRouter(ctx context.Context, binary, cwd, dir string) *Router {
	c, cancel := context.WithCancel(ctx)
	r := &Router{ctx: c, cancel: cancel, binary: binary, cwd: cwd, dir: filepath.Join(dir, "opencode"), workspaces: map[string]*managedWorkspace{}}
	if err := os.Mkdir(r.dir, 0700); err != nil && !os.IsExist(err) {
		r.loadErr = errors.New("cannot create private opencode state")
		return r
	}
	r.registry, r.loadErr = openRegistry(r.dir)
	return r
}
func (r *Router) Close() {
	r.cancel()
	r.mu.Lock()
	p := r.process
	r.mu.Unlock()
	if p != nil {
		p.Close()
	}
}

func (r *Router) workspace(path string) (*managedWorkspace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loadErr != nil {
		return nil, r.loadErr
	}
	if w := r.workspaces[path]; w != nil {
		return w, nil
	}
	if r.process == nil {
		p, err := StartPersistentProcess(r.ctx, r.binary, r.cwd, os.Environ(), r.dir)
		if err != nil {
			return nil, err
		}
		r.process = p
	}
	client := NewClient(r.process.BaseURL(), r.process.Client(), path)
	emit := func(method string, raw json.RawMessage) { r.emitEvent(path, method, raw) }
	synth := NewSynthesizer(path, emit)
	decisions := NewDecisions(path, emit, synth.TurnID, func(ctx context.Context, sid, rid, answer string) error {
		return client.ReplyPermission(ctx, sid, rid, answer)
	})
	decisions.ReplyForm = client.ReplyForm
	synth.OnTurnComplete = decisions.Resolve
	decisions.OnUnsupportedInput = synth.UnsupportedInput
	stream := NewStream(client, synth, decisions.Handle)
	stream.ScopedPoll = true
	stream.OnScopedPermissions = decisions.reconcileTurn
	stream.Forms = decisions
	// shortcut: poll anchored history until native events reliably identify their turn.
	stream.Filter = func(string, json.RawMessage, string) bool { return false }

	w := &managedWorkspace{client: client, synth: synth, decisions: decisions}
	r.workspaces[path] = w
	go func() { _ = stream.Run(r.ctx) }()
	return w, nil
}
func (r *Router) findNative(workspace, native string) (registeredChat, bool) {
	if native == "" || r.registry == nil {
		return registeredChat{}, false
	}
	r.registry.mu.Lock()
	defer r.registry.mu.Unlock()
	for _, v := range r.registry.chats {
		if v.Workspace == workspace && v.Native == native {
			return v, true
		}
	}
	return registeredChat{}, false
}
func (r *Router) emitEvent(workspace, method string, raw json.RawMessage) {
	var p map[string]any
	if json.Unmarshal(raw, &p) != nil {
		return
	}
	native, _ := p["threadId"].(string)
	if native == "" {
		native, _ = p["thread_id"].(string)
	}
	if native == "" {
		if id, ok := p["decision_id"].(string); ok {
			native = strings.SplitN(id, "/", 2)[0]
		}
	}
	chat, ok := r.findNative(workspace, native)
	if !ok {
		return
	}
	if _, ok := p["threadId"]; ok {
		p["threadId"] = chat.ID
	}
	if _, ok := p["thread_id"]; ok {
		p["thread_id"] = chat.ID
	}
	if id, ok := p["decision_id"].(string); ok && strings.HasPrefix(id, native+"/") {
		p["decision_id"] = chat.ID + strings.TrimPrefix(id, native)
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return
	}
	r.mu.Lock()
	emit := r.emit
	r.mu.Unlock()
	if emit != nil {
		emit(method, encoded)
	}
}
func canonicalWorkspace(path string) (string, error) {
	if !filepath.IsAbs(path) || strings.ContainsAny(path, "\r\n\x00") || len(path) > 32768 {
		return "", errors.New("invalid workspace")
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.New("workspace is unavailable")
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", errors.New("workspace is unavailable")
	}
	return path, nil
}
func (r *Router) Readiness(ctx context.Context, workspace string) map[string]any {
	entry := map[string]any{"id": "opencode", "label": "OpenCode", "ready": false}
	if workspace == "" {
		workspace = r.cwd
	}
	path, err := canonicalWorkspace(workspace)
	if err != nil {
		entry["reason"] = "Workspace is unavailable"
		return entry
	}
	w, err := r.workspace(path)
	if err != nil {
		entry["reason"] = "OpenCode runtime is unavailable"
		return entry
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	info, err := w.client.Info(ctx)
	if err != nil {
		entry["reason"] = "OpenCode API is unavailable or incompatible"
		return entry
	}
	if !compatibleVersion(info.Version) {
		entry["reason"] = "OpenCode 2.4 or newer in the 2.x series is required"
		return entry
	}
	models, err := w.client.ListModels(ctx)
	if err != nil {
		entry["reason"] = "OpenCode model API is unavailable or incompatible"
		return entry
	}
	for _, m := range models {
		if m.ProviderID != "" && m.ModelID != "" {
			entry["ready"] = true
			return entry
		}
	}
	entry["reason"] = "Configure an OpenCode provider and model on the host"
	return entry
}
func (r *Router) Threads() []map[string]any {
	out := []map[string]any{}
	if r.registry == nil {
		return out
	}
	r.registry.mu.Lock()
	defer r.registry.mu.Unlock()
	for _, v := range r.registry.chats {
		m := map[string]any{"id": v.ID, "cwd": v.Workspace, "agent": "opencode"}
		if v.Project != "" {
			m["projectId"] = v.Project
		}
		if v.Projectless {
			m["projectless"] = true
		}
		out = append(out, m)
	}
	return out
}
func (r *Router) Handle(ctx context.Context, cmd bridge.Command, emit func(string, json.RawMessage)) (any, bool, error) {
	p := map[string]json.RawMessage{}
	if len(cmd.Params) > 0 && string(cmd.Params) != "null" && (json.Unmarshal(cmd.Params, &p) != nil || p == nil) {
		return nil, true, errors.New("params must be an object")
	}
	str := func(k string) string { var s string; _ = json.Unmarshal(p[k], &s); return s }
	agent := str("agent")
	id := str("thread_id")
	decision := str("decision_id")
	if cmd.Method == "client/attached" {
		r.mu.Lock()
		r.emit = emit
		ws := make([]*managedWorkspace, 0, len(r.workspaces))
		for _, w := range r.workspaces {
			ws = append(ws, w)
		}
		r.mu.Unlock()
		if string(p["attached"]) == "true" {
			for _, w := range ws {
				w.decisions.Replay(true)
			}
		}
		return nil, false, nil
	}
	if !(cmd.Method == "threads/start" && agent == "opencode") && !(cmd.Method == "models/list" && agent == "opencode") && !strings.HasPrefix(id, "oc:") && !strings.HasPrefix(decision, "oc:") {
		return nil, false, nil
	}
	r.mu.Lock()
	r.emit = emit
	r.mu.Unlock()
	if r.loadErr != nil {
		return nil, true, r.loadErr
	}
	workspace := r.cwd
	project := ""
	standalone := false
	var chat registeredChat
	if id != "" || decision != "" {
		key := id
		if key == "" {
			key = strings.SplitN(decision, "/", 2)[0]
		}
		var ok bool
		chat, ok = r.registry.get(key)
		if !ok {
			return nil, true, errors.New("OpenCode session not found")
		}
		workspace = chat.Workspace
		if raw, ok := p["workspace"]; ok {
			var supplied string
			if json.Unmarshal(raw, &supplied) != nil {
				return nil, true, errors.New("invalid workspace")
			}
			path, err := canonicalWorkspace(supplied)
			if err != nil || path != workspace {
				return nil, true, errors.New("thread outside workspace")
			}
		}
		if id != "" {
			p["thread_id"], _ = json.Marshal(chat.Native)
		}
		if decision != "" {
			p["decision_id"], _ = json.Marshal(chat.Native + strings.TrimPrefix(decision, chat.ID))
		}
	} else {
		if raw, ok := p["workspace"]; ok {
			if json.Unmarshal(raw, &workspace) != nil {
				return nil, true, errors.New("invalid workspace")
			}
		}
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
				return nil, true, err
			}
			workspace = filepath.Join(home, "Documents", "Codex")
			if err := os.MkdirAll(workspace, 0700); err != nil {
				return nil, true, err
			}
			delete(p, "standalone")
		}
		var err error
		workspace, err = canonicalWorkspace(workspace)
		if err != nil {
			return nil, true, err
		}
		project = str("project_id")
	}
	delete(p, "agent")
	delete(p, "project_id")
	if cmd.Method != "decision/respond" {
		p["workspace"], _ = json.Marshal(workspace)
	}
	if cmd.Method == "models/list" {
		delete(p, "workspace")
	}
	encoded, _ := json.Marshal(p)
	cmd.Params = encoded
	w, err := r.workspace(workspace)
	if err != nil {
		return nil, true, err
	}
	call := func(ctx context.Context, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error) {
		values, _ := params.(map[string]any)
		sid, _ := values["sessionID"].(string)
		if method == "decision/respond" {
			if err := w.decisions.Answer(ctx, stringParam(values, "decisionID"), values["result"].(json.RawMessage)); err != nil {
				return nil, err
			}
			return json.RawMessage(`{"delivered":true}`), nil
		}
		if method == "turn/start" {
			if _, active := w.synth.ActiveTurn(sid); active {
				return nil, errors.New("turn is already active")
			}
			permissions, err := w.client.ListPermissions(ctx, sid)
			if err != nil {
				return nil, errors.New("cannot verify pending OpenCode requests")
			}
			forms, err := w.client.listForms(ctx, sid)
			if err != nil {
				return nil, errors.New("cannot verify pending OpenCode requests")
			}
			if err = w.decisions.excludeExisting(sid, permissions, forms); err != nil {
				return nil, err
			}
		}
		if method == "turn/steer" || method == "turn/interrupt" {
			active, ok := w.synth.ActiveTurn(sid)
			if !ok || active != stringParam(values, "turnID") {
				return nil, errors.New("turn is no longer active")
			}
		}
		raw, err := w.client.Call(ctx, method, params, emit)
		if err != nil {
			return nil, err
		}
		if method == "turn/start" {
			var v struct {
				Turn struct {
					ID string `json:"id"`
				} `json:"turn"`
			}
			if json.Unmarshal(raw, &v) == nil && v.Turn.ID != "" {
				w.synth.StartTurn(sid, v.Turn.ID, workspace)
			}
		}
		if method == "turn/interrupt" {
			var v struct {
				Interrupted bool `json:"interrupted"`
			}
			if json.Unmarshal(raw, &v) == nil && v.Interrupted {
				w.synth.MarkInterrupted(sid, stringParam(values, "turnID"))
			}
		}
		return raw, nil
	}
	forward := func(method string, raw json.RawMessage) { r.emitEvent(workspace, method, raw) }
	value, err := handleEvents(ctx, workspace, cmd, call, forward, w.synth)
	if err != nil {
		return nil, true, err
	}
	if cmd.Method == "threads/start" || cmd.Method == "threads/read" || cmd.Method == "threads/resume" {
		result, ok := value.(map[string]any)
		if !ok {
			return nil, true, errors.New("invalid thread response")
		}
		thread, ok := result["thread"].(map[string]any)
		if !ok {
			return nil, true, errors.New("invalid thread response")
		}
		if cmd.Method == "threads/start" {
			native, _ := thread["id"].(string)
			chat, err = r.registry.add(native, workspace, project, standalone)
			if err != nil {
				return nil, true, err
			}
		}
		thread["id"] = chat.ID
		thread["agent"] = "opencode"
		delete(thread, "projectId")
		if chat.Project != "" {
			thread["projectId"] = chat.Project
		}
		if chat.Projectless {
			thread["projectless"] = true
		}
		return result, true, nil
	}
	return value, true, nil
}
