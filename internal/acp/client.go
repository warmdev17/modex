// Package acp connects Modex Host to the Antigravity ACP subprocess.
package acp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

type Update struct {
	SessionID string
	TurnID    string
	Kind      string
	Text      string
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
	if onUpdate != nil {
		process.SetNotificationHandler(func(method string, raw json.RawMessage) {
			if method == "session/update" {
				c.handleUpdate(raw, onUpdate)
			}
		})
	}
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
			Sessions    struct {
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
	var session Session
	params := map[string]any{"cwd": c.cwd, "mcpServers": []any{}}
	if err := c.call(ctx, "session/new", params, &session); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !validID(session.ID) || c.sessions[session.ID] != nil {
		return "", errors.New("invalid or duplicate Antigravity session ID")
	}
	c.sessions[session.ID] = &sessionState{loaded: true}
	return session.ID, nil
}

// Load replays history through onUpdate for a session discovered in this workspace.
func (c *Client) Load(ctx context.Context, sessionID string) error {
	if !c.info.LoadSession {
		return errors.New("Antigravity ACP does not advertise session loading")
	}
	c.mu.Lock()
	session := c.sessions[sessionID]
	if session == nil || session.loading || session.turn != nil {
		c.mu.Unlock()
		return errors.New("Antigravity session is unavailable to load")
	}
	session.loading = true
	c.mu.Unlock()
	params := map[string]any{"sessionId": sessionID, "cwd": c.cwd, "mcpServers": []any{}}
	err := c.call(ctx, "session/load", params, &struct{}{})
	c.mu.Lock()
	session.loading = false
	session.loaded = err == nil
	c.mu.Unlock()
	return err
}

// Prompt runs one turn. The caller assigns a fresh turnID for each submission.
func (c *Client) Prompt(ctx context.Context, sessionID, turnID, text string) (string, error) {
	return c.PromptWithSent(ctx, sessionID, turnID, text, nil)
}

func (c *Client) PromptWithSent(ctx context.Context, sessionID, turnID, text string, sent func()) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !validID(turnID) || strings.TrimSpace(text) == "" || len(text) > 1024*1024 {
		return "", errors.New("invalid Antigravity turn or text prompt")
	}
	c.mu.Lock()
	session := c.sessions[sessionID]
	if session == nil || !session.loaded || session.loading || session.turn != nil {
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
	params := map[string]any{"sessionId": sessionID, "prompt": []map[string]string{{"type": "text", "text": text}}}
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
			Kind    string                      `json:"sessionUpdate"`
			Content struct{ Type, Text string } `json:"content"`
		} `json:"update"`
	}
	if json.Unmarshal(raw, &event) != nil || event.Update.Content.Type != "text" {
		return
	}
	switch event.Update.Kind {
	case "agent_message_chunk", "agent_thought_chunk", "user_message_chunk":
	default:
		return
	}
	update := Update{SessionID: event.ID, Kind: event.Update.Kind, Text: event.Update.Content.Text}
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
	emit(update)
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
