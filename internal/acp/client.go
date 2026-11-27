// Package acp connects Modex Host to the Antigravity ACP subprocess.
package acp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/warmdev/modex/internal/bridge"
	"github.com/warmdev/modex/internal/diagnostic"
)

var ErrAuthenticationRequired = errors.New("Antigravity requires sign-in on the host with agy acp --login")

type Info struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	ProtocolVersion int    `json:"protocolVersion"`
	ListSessions    bool   `json:"listSessions"`
	LoadSession     bool   `json:"loadSession"`
	Image           bool   `json:"image"`
	EmbeddedContext bool   `json:"embeddedContext"`
}

type Session struct {
	ID    string `json:"sessionId"`
	CWD   string `json:"cwd"`
	Title string `json:"title,omitempty"`
}

type Page struct {
	Sessions   []Session `json:"sessions"`
	NextCursor string    `json:"nextCursor,omitempty"`
}

type OptionValue struct {
	Value, Name, Group string
}

type Option struct {
	ID, Name, Category, CurrentValue string
	Values                           []OptionValue
}

type nativeOption struct {
	ID, Name, Category, Type, CurrentValue string
	Options                                []struct {
		Value, Name, Group string
		Options            []struct{ Value, Name string }
	}
}

type configResult struct {
	ConfigOptions []nativeOption `json:"configOptions"`
}

type Update struct {
	SessionID  string
	TurnID     string
	Kind       string
	Text       string
	ToolCallID string
	Status     string
	Subagent   bool
	Role       string
	URI        string
	MIME       string
}

type PermissionRequest struct {
	ID, SessionID, TurnID, Title, AllowOption, DenyOption string
}

type pendingPermission struct {
	nativeID json.RawMessage
	request  PermissionRequest
	turn     *promptTurn
}

type Client struct {
	process *bridge.Process
	cwd     string
	info    Info

	mu             sync.Mutex
	sessions       map[string]*sessionState
	permissions    map[string]pendingPermission
	nextPermission uint64
	closed         bool
}

type sessionState struct {
	loaded, loading bool
	turn            *promptTurn
	setting         bool
	options         []Option
	optionsVersion  uint64
}

type promptTurn struct {
	id            string
	sent          chan struct{}
	done          chan struct{}
	cancelling    bool
	stopRequested bool
}

// Start launches Antigravity using the host user's credentials. onUpdate runs on
// the RPC reader and must return promptly without calling back into the client.
func Start(ctx context.Context, binary, cwd string, onUpdate func(Update)) (*Client, error) {
	return StartWithPermissions(ctx, binary, cwd, onUpdate, nil)
}

// StartWithPermissions delivers permission requests on the RPC reader; callbacks must return promptly and must not call the client.
func StartWithPermissions(ctx context.Context, binary, cwd string, onUpdate func(Update), onPermission func(PermissionRequest) bool) (*Client, error) {
	return startWithPermissions(ctx, binary, []string{"acp", "--disable-telemetry"}, cwd, os.Environ(), onUpdate, onPermission)
}

func start(ctx context.Context, binary string, args []string, cwd string, env []string, onUpdate func(Update)) (*Client, error) {
	return startWithPermissions(ctx, binary, args, cwd, env, onUpdate, nil)
}

func startWithPermissions(ctx context.Context, binary string, args []string, cwd string, env []string, onUpdate func(Update), onPermission func(PermissionRequest) bool) (*Client, error) {
	canonical, err := filepath.EvalSymlinks(cwd)
	if err != nil || !filepath.IsAbs(canonical) {
		return nil, errors.New("Antigravity requires an existing absolute workspace")
	}
	process, err := bridge.StartJSONRPCProcess(ctx, binary, args, canonical, env)
	if err != nil {
		return nil, errors.New("cannot start Antigravity CLI; check the agy executable on the host")
	}
	c := &Client{process: process, cwd: canonical, sessions: map[string]*sessionState{}, permissions: map[string]pendingPermission{}}
	process.SetRequestHandler(func(id json.RawMessage, method string, params json.RawMessage) {
		if method == "session/request_permission" {
			c.handlePermission(ctx, id, params, onPermission)
			return
		}
		_ = process.Reject(id)
	})
	process.SetNotificationHandler(func(method string, raw json.RawMessage) {
		if method == "session/update" {
			c.handleUpdate(raw, onUpdate)
		}
	})
	initCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var initialized struct {
		Version int `json:"protocolVersion"`
		Agent   struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"agentInfo"`
		Capabilities struct {
			LoadSession bool `json:"loadSession"`
			Prompt      struct {
				Image           bool `json:"image"`
				EmbeddedContext bool `json:"embeddedContext"`
			} `json:"promptCapabilities"`
			Sessions struct {
				List *struct{} `json:"list"`
			} `json:"sessionCapabilities"`
		} `json:"agentCapabilities"`
	}
	if err := c.call(initCtx, "initialize", map[string]any{"protocolVersion": 1, "clientCapabilities": map[string]any{}}, &initialized); err != nil {
		c.Close()
		return nil, err
	}
	if initialized.Version != 1 || initialized.Agent.Name != "Antigravity ACP Server" || initialized.Agent.Version == "" {
		c.Close()
		return nil, errors.New("host executable did not negotiate Antigravity ACP v1")
	}
	c.info = Info{
		Name: initialized.Agent.Name, Version: initialized.Agent.Version, ProtocolVersion: initialized.Version,
		ListSessions: initialized.Capabilities.Sessions.List != nil,
		LoadSession:  initialized.Capabilities.LoadSession,
		Image:        initialized.Capabilities.Prompt.Image, EmbeddedContext: initialized.Capabilities.Prompt.EmbeddedContext,
	}
	return c, nil
}

func (c *Client) Close() {
	c.mu.Lock()
	c.closed = true
	clear(c.permissions)
	c.mu.Unlock()
	c.process.CloseIdle()
}
func (c *Client) Info() Info { return c.info }

func (c *Client) List(ctx context.Context, cursor string) (Page, error) {
	if !c.info.ListSessions {
		return Page{}, errors.New("Antigravity ACP does not advertise session listing")
	}
	params := map[string]string{"cwd": c.cwd}
	if cursor != "" {
		params["cursor"] = cursor
	}
	var page Page
	if err := c.call(ctx, "session/list", params, &page); err != nil {
		return Page{}, err
	}
	if page.Sessions == nil {
		return Page{}, errors.New("invalid Antigravity session page")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	sessions := page.Sessions[:0]
	for _, session := range page.Sessions {
		if !validID(session.ID) || !filepath.IsAbs(session.CWD) || filepath.Clean(session.CWD) != c.cwd {
			continue
		}
		if c.sessions[session.ID] == nil {
			c.sessions[session.ID] = &sessionState{}
		}
		sessions = append(sessions, session)
	}
	page.Sessions = sessions
	return page, nil
}

func (c *Client) NewSession(ctx context.Context) (string, error) {
	var session struct {
		Session
		configResult
	}
	params := map[string]any{"cwd": c.cwd, "mcpServers": []any{}}
	if err := c.call(ctx, "session/new", params, &session); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !validID(session.ID) || c.sessions[session.ID] != nil {
		return "", errors.New("invalid or duplicate Antigravity session ID")
	}
	c.sessions[session.ID] = &sessionState{loaded: true, options: parseOptions(session.ConfigOptions)}
	return session.ID, nil
}

// Load replays history through onUpdate for a session discovered in this workspace.
func (c *Client) Load(ctx context.Context, sessionID string) error {
	if !c.info.LoadSession {
		return errors.New("Antigravity ACP does not advertise session loading")
	}
	c.mu.Lock()
	session := c.sessions[sessionID]
	if session == nil || session.loading || session.setting || session.turn != nil {
		c.mu.Unlock()
		return errors.New("Antigravity session is unavailable to load")
	}
	session.loading = true
	c.mu.Unlock()
	params := map[string]any{"sessionId": sessionID, "cwd": c.cwd, "mcpServers": []any{}}
	var result configResult
	err := c.call(ctx, "session/load", params, &result)
	c.mu.Lock()
	session.loading = false
	session.loaded = err == nil
	if err == nil {
		session.options = parseOptions(result.ConfigOptions)
	}
	c.mu.Unlock()
	return err
}

// SessionLoaded reports whether the session is usable on this connection.
func (c *Client) SessionLoaded(sessionID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	session := c.sessions[sessionID]
	return !c.closed && session != nil && session.loaded
}

// Options returns a snapshot of the latest supported session selectors.
func (c *Client) Options(sessionID string) []Option {
	c.mu.Lock()
	defer c.mu.Unlock()
	session := c.sessions[sessionID]
	if session == nil || !session.loaded {
		return nil
	}
	result := append([]Option(nil), session.options...)
	for i := range result {
		result[i].Values = append([]OptionValue(nil), result[i].Values...)
	}
	return result
}

func parseOptions(native []nativeOption) []Option {
	var result []Option
	for _, n := range native {
		if n.Type != "select" || (n.ID != "model" && n.ID != "thought_level" && n.Category != "model" && n.Category != "thought_level") || !validID(n.ID) {
			continue
		}
		option := Option{ID: n.ID, Name: n.Name, Category: n.Category, CurrentValue: n.CurrentValue}
		for _, v := range n.Options {
			if validID(v.Value) {
				option.Values = append(option.Values, OptionValue{Value: v.Value, Name: v.Name, Group: v.Group})
			}
			for _, item := range v.Options {
				if validID(item.Value) {
					option.Values = append(option.Values, OptionValue{Value: item.Value, Name: item.Name, Group: v.Name})
				}
			}
		}
		if len(option.Values) > 0 {
			result = append(result, option)
		}
	}
	return result
}

// SetOption only accepts values currently offered for an idle loaded session.
func (c *Client) SetOption(ctx context.Context, sessionID, configID, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	session := c.sessions[sessionID]
	allowed := false
	if session != nil && session.loaded && !session.loading && !session.setting && session.turn == nil {
		for _, option := range session.options {
			if option.ID == configID {
				for _, v := range option.Values {
					if v.Value == value {
						allowed = true
					}
				}
			}
		}
	}
	if !allowed {
		c.mu.Unlock()
		return errors.New("Antigravity session or configuration option is unavailable")
	}
	session.setting = true
	version := session.optionsVersion
	c.mu.Unlock()
	raw, err := c.request(ctx, "session/set_config_option", map[string]string{"sessionId": sessionID, "configId": configID, "value": value}, nil)
	c.mu.Lock()
	defer c.mu.Unlock()
	session.setting = false
	if err != nil {
		return err
	}
	var result configResult
	validResponse := json.Unmarshal(raw, &result) == nil
	acknowledged := parseOptions(result.ConfigOptions)
	confirmed := false
	for _, option := range acknowledged {
		if option.ID == configID && option.CurrentValue == value {
			confirmed = true
		}
	}
	if !validResponse || !confirmed {
		session.options = nil
		session.loaded = false
		return errors.New("invalid Antigravity configuration response; reload the session")
	}
	if session.optionsVersion != version && !reflect.DeepEqual(session.options, acknowledged) {
		session.options = nil
		session.loaded = false
		return errors.New("conflicting Antigravity configuration updates; reload the session")
	}
	session.options = acknowledged
	return nil
}

// Prompt runs one turn. The caller assigns a fresh turnID for each submission.
func (c *Client) Prompt(ctx context.Context, sessionID, turnID, text string) (string, error) {
	return c.PromptWithSent(ctx, sessionID, turnID, text, nil)
}

type PromptBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	Data     string          `json:"data,omitempty"`
	MimeType string          `json:"mimeType,omitempty"`
	URI      string          `json:"uri,omitempty"`
	Resource *PromptResource `json:"resource,omitempty"`
}
type PromptResource struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

func (c *Client) PromptWithSent(ctx context.Context, sessionID, turnID, text string, sent func()) (string, error) {
	return c.PromptBlocksWithSent(ctx, sessionID, turnID, []PromptBlock{{Type: "text", Text: text}}, sent)
}

func (c *Client) PromptBlocksWithSent(ctx context.Context, sessionID, turnID string, blocks []PromptBlock, sent func()) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !validID(turnID) || len(blocks) == 0 {
		return "", errors.New("invalid Antigravity turn or text prompt")
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) == "" || len(b.Text) > 1024*1024 {
				return "", errors.New("invalid Antigravity turn or text prompt")
			}
		case "image":
			if !c.info.Image {
				return "", errors.New("Antigravity does not support image attachments")
			}
		case "resource":
			if !c.info.EmbeddedContext {
				return "", errors.New("Antigravity does not support text attachments")
			}
		default:
			return "", errors.New("invalid Antigravity prompt block")
		}
	}
	c.mu.Lock()
	session := c.sessions[sessionID]
	if session == nil || !session.loaded || session.loading || session.setting || session.turn != nil {
		c.mu.Unlock()
		return "", errors.New("Antigravity session is unavailable for a new prompt")
	}
	turn := &promptTurn{id: turnID, sent: make(chan struct{}), done: make(chan struct{})}
	session.turn = turn
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.clearTurnPermissions(turn)
		if !turn.cancelling {
			session.turn = nil
		}
		close(turn.done)
		c.mu.Unlock()
	}()
	params := map[string]any{"sessionId": sessionID, "prompt": blocks}
	raw, err := c.request(ctx, "session/prompt", params, func() {
		close(turn.sent)
		if sent != nil {
			sent()
		}
	})
	if err != nil {
		return "", err
	}
	var result struct {
		Stop string `json:"stopReason"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Stop == "" {
		c.process.Close()
		return "", errors.New("invalid Antigravity turn result")
	}
	return result.Stop, nil
}

func (c *Client) Cancel(ctx context.Context, sessionID, turnID string) error {
	c.mu.Lock()
	session := c.sessions[sessionID]
	if session == nil || session.turn == nil || session.turn.id != turnID {
		c.mu.Unlock()
		return errors.New("Antigravity turn is no longer active")
	}
	turn := session.turn
	c.mu.Unlock()
	select {
	case <-turn.sent:
	case <-turn.done:
		return errors.New("Antigravity turn is no longer active")
	case <-ctx.Done():
		return ctx.Err()
	}
	c.mu.Lock()
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return err
	}
	if session.turn != turn || turn.stopRequested {
		c.mu.Unlock()
		return errors.New("Antigravity turn is no longer active")
	}
	// Keep the session reserved until cancel is written, even if prompt finishes.
	turn.cancelling = true
	turn.stopRequested = true
	c.clearTurnPermissions(turn)
	c.mu.Unlock()
	stop := context.AfterFunc(ctx, c.process.Close)
	err := c.process.Notify("session/cancel", map[string]string{"sessionId": sessionID})
	stop()
	if err != nil {
		c.process.Close()
	}
	c.mu.Lock()
	turn.cancelling = false
	select {
	case <-turn.done:
		session.turn = nil
	default:
	}
	c.mu.Unlock()
	if err != nil {
		return errors.New("Antigravity cancellation delivery could not be confirmed")
	}
	return nil
}

func (c *Client) handleUpdate(raw json.RawMessage, emit func(Update)) {
	var event struct {
		ID     string `json:"sessionId"`
		Update struct {
			Kind    string `json:"sessionUpdate"`
			Content struct {
				Type, Text, URI, MimeType string
				Resource                  struct{ URI, MimeType string }
			} `json:"content"`
			ConfigOptions []nativeOption `json:"configOptions"`
			Name          string         `json:"name"`
			ToolCallID    string         `json:"toolCallId"`
			Status        string         `json:"status"`
			RawInput      struct {
				Role string `json:"Role"`
			} `json:"rawInput"`
		} `json:"update"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return
	}
	if event.Update.Kind == "config_option_update" {
		c.mu.Lock()
		session := c.sessions[event.ID]
		if session != nil && event.Update.ConfigOptions != nil {
			session.options = parseOptions(event.Update.ConfigOptions)
			session.optionsVersion++
		}
		c.mu.Unlock()
		return
	}
	update := Update{SessionID: event.ID, Kind: event.Update.Kind}
	switch event.Update.Kind {
	case "agent_message_chunk", "agent_thought_chunk", "user_message_chunk":
		switch event.Update.Content.Type {
		case "text":
			update.Text = event.Update.Content.Text
		case "image", "resource_link":
			if event.Update.Kind != "user_message_chunk" {
				return
			}
			update.URI = event.Update.Content.URI
			update.MIME = event.Update.Content.MimeType
		case "resource":
			if event.Update.Kind != "user_message_chunk" {
				return
			}
			update.URI = event.Update.Content.Resource.URI
			update.MIME = event.Update.Content.Resource.MimeType
		default:
			return
		}
		if event.Update.Content.Type != "text" && update.URI == "" {
			return
		}
	case "tool_call", "tool_call_update":
		if !safeScalar(event.Update.ToolCallID, 256) {
			return
		}
		update.ToolCallID = event.Update.ToolCallID
		switch event.Update.Status {
		case "pending", "in_progress", "completed", "failed":
			update.Status = event.Update.Status
		}
		update.Subagent = event.Update.Name == "invoke_subagent"
		if update.Subagent && safeScalar(event.Update.RawInput.Role, 64) {
			update.Role = event.Update.RawInput.Role
		}
	default:
		return
	}
	c.mu.Lock()
	session := c.sessions[event.ID]
	if session == nil || (!session.loading && session.turn == nil) {
		c.mu.Unlock()
		return
	}
	if session.turn != nil {
		update.TurnID = session.turn.id
	}
	c.mu.Unlock()
	if emit != nil {
		emit(update)
	}
}

func safeScalar(value string, max int) bool {
	if value == "" || len(value) > max {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_-.", r)) {
			return false
		}
	}
	return true
}

func (c *Client) call(ctx context.Context, method string, params, dst any) error {
	raw, err := c.request(ctx, method, params, nil)
	if err != nil {
		return err
	}
	if string(raw) == "null" || json.Unmarshal(raw, dst) != nil {
		return errors.New("invalid Antigravity ACP response")
	}
	return nil
}

func (c *Client) request(ctx context.Context, method string, params any, sent func()) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// The RPC deadline must also interrupt a pipe write to a stalled child.
	stop := context.AfterFunc(ctx, c.process.Close)
	defer stop()
	raw, err := c.process.CallWithSent(ctx, method, params, sent)
	if err != nil {
		return nil, c.exchangeError(err)
	}
	return raw, nil
}

func (c *Client) exchangeError(err error) error {
	var rejected *diagnostic.RPCError
	if errors.As(err, &rejected) {
		if rejected.Code() == -32000 {
			return ErrAuthenticationRequired
		}
		return errors.New("Antigravity rejected the ACP request")
	}
	// The native request may still be running. Retire this connection before
	// admitting another request that could receive the first request's updates.
	c.process.Close()
	c.mu.Lock()
	c.closed = true
	clear(c.permissions)
	c.mu.Unlock()
	return errors.New("Antigravity ACP exchange failed; refresh the session before retrying")
}

func validID(id string) bool {
	return id != "" && len(id) <= 256 && !strings.ContainsAny(id, "\r\n\x00")
}

// clearTurnPermissions requires c.mu.
func (c *Client) clearTurnPermissions(turn *promptTurn) {
	for id, p := range c.permissions {
		if p.turn == turn {
			delete(c.permissions, id)
		}
	}
}

func (c *Client) handlePermission(ctx context.Context, nativeID, raw json.RawMessage, callback func(PermissionRequest) bool) {
	var input struct {
		SessionID string `json:"sessionId"`
		ToolCall  struct {
			Title string `json:"title"`
		} `json:"toolCall"`
		Options []struct {
			ID   string `json:"optionId"`
			Kind string `json:"kind"`
		} `json:"options"`
	}
	if json.Unmarshal(raw, &input) == nil && callback != nil {
		c.mu.Lock()
		session := c.sessions[input.SessionID]
		if !c.closed && session != nil && session.turn != nil && !session.turn.stopRequested && len(c.permissions) < 128 {
			for _, pending := range c.permissions {
				if string(pending.nativeID) == string(nativeID) {
					clear(c.permissions)
					c.mu.Unlock()
					c.process.Close()
					return
				}
			}
			p := PermissionRequest{SessionID: input.SessionID, TurnID: session.turn.id, Title: input.ToolCall.Title}
			var allows, denies int
			ids := make(map[string]int)
			for _, option := range input.Options {
				ids[option.ID]++
			}
			for _, option := range input.Options {
				switch option.Kind {
				case "allow_once":
					allows++
					if validID(option.ID) && ids[option.ID] == 1 {
						p.AllowOption = option.ID
					}
				case "reject_once":
					denies++
					if validID(option.ID) && ids[option.ID] == 1 {
						p.DenyOption = option.ID
					}
				}
			}
			if allows != 1 {
				p.AllowOption = ""
			}
			if denies != 1 {
				p.DenyOption = ""
			}
			c.nextPermission++
			p.ID = strconv.FormatUint(c.nextPermission, 10)
			c.permissions[p.ID] = pendingPermission{nativeID: append(json.RawMessage(nil), nativeID...), request: p, turn: session.turn}
			c.mu.Unlock()
			if callback(p) {
				return
			}
			c.mu.Lock()
			_, pending := c.permissions[p.ID]
			delete(c.permissions, p.ID)
			c.mu.Unlock()
			if !pending {
				return
			}
			_ = c.process.Respond(ctx, nativeID, json.RawMessage(`{"outcome":{"outcome":"cancelled"}}`))
			return
		}
		c.mu.Unlock()
	}
	_ = c.process.Respond(ctx, nativeID, json.RawMessage(`{"outcome":{"outcome":"cancelled"}}`))
}

// ReplyPermission sends at most one response for an active native request.
func (c *Client) ReplyPermission(ctx context.Context, requestID, optionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.permissions[requestID]
	if !ok || p.turn.stopRequested || (optionID != "" && optionID != p.request.AllowOption && optionID != p.request.DenyOption) {
		return errors.New("Antigravity permission is unavailable or option is invalid")
	}
	delete(c.permissions, requestID)
	outcome := map[string]string{"outcome": "cancelled"}
	if optionID != "" {
		outcome = map[string]string{"outcome": "selected", "optionId": optionID}
	}
	result, _ := json.Marshal(map[string]any{"outcome": outcome})
	stop := context.AfterFunc(ctx, c.process.Close)
	err := c.process.Respond(ctx, p.nativeID, result)
	stop()
	if err != nil {
		c.process.Close()
		clear(c.permissions)
		return errors.New("Antigravity permission delivery could not be confirmed")
	}
	return nil
}
