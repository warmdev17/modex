package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	requestTimeout = 20 * time.Second
	maxResponse    = 8 << 20
)

var (
	errRequestFailed   = errors.New("opencode request failed")
	errInvalidResponse = errors.New("invalid opencode response")
)

// Client is the typed OpenCode HTTP surface. It always talks to the loopback
// child through the authenticated client owned by Process.
type Client struct {
	baseURL string
	http    *http.Client
	cwd     string
}

func NewClient(baseURL string, httpClient *http.Client, cwd string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient, cwd: cwd}
}

type info struct {
	Version string `json:"version"`
}

type configDocument struct {
	Type string     `json:"type"`
	Path string     `json:"path"`
	Info configInfo `json:"info"`
}

type configInfo struct {
	DefaultAgent string                 `json:"default_agent"`
	Agents       map[string]configAgent `json:"agents"`
}

type configAgent struct {
	Model json.RawMessage `json:"model"`
}

type sessionsPage struct {
	Data   []session `json:"data"`
	Cursor struct {
		Previous *string `json:"previous"`
		Next     *string `json:"next"`
	} `json:"cursor"`
}

type messagePage struct {
	Data   []message `json:"data"`
	Cursor struct {
		Previous *string `json:"previous"`
		Next     *string `json:"next"`
	} `json:"cursor"`
}

// Call names are the internal method seam between server.go and the HTTP
// client. They are not Modex method names.
const (
	callInfo             = "info"
	callConfig           = "config"
	callSessionList      = "session/list"
	callSessionCreate    = "session/create"
	callSessionRead      = "session/read"
	callSessionMessages  = "session/messages"
	callSessionInterrupt = "session/interrupt"
	callModels           = "models"
	callClientAttached   = "client/attached"
	callTurnStart        = "turn/start"
	callTurnInterrupt    = "turn/interrupt"
	callDecisionRespond  = "decision/respond"
)

// promptFile is the v2 prompt attachment input. OpenCode 2.4.x takes the file
// URI directly; MIME is inferred server-side and is not accepted in this body.
type promptFile struct {
	URI  string `json:"uri"`
	Name string `json:"name,omitempty"`
}

// Call adapts the typed client to the bridge EventCaller seam. It returns
// native OpenCode shapes as JSON; server.go performs the Modex projection.
func (c *Client) Call(ctx context.Context, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error) {
	values, _ := params.(map[string]any)
	var value any
	var err error
	switch method {
	case callInfo:
		value, err = c.Info(ctx)
	case callConfig:
		value, err = c.Config(ctx)
	case callSessionList:
		value, err = c.ListSessions(ctx, intParam(values, "limit", 20))
	case callSessionCreate:
		var model *modelRef
		providerID := stringParam(values, "providerID")
		modelID := stringParam(values, "modelID")
		if providerID != "" && modelID != "" {
			model = &modelRef{ProviderID: providerID, ModelID: modelID}
		}
		value, err = c.CreateSession(ctx, stringParam(values, "directory"), model, stringParam(values, "agent"))
	case callSessionRead:
		value, err = c.ReadSession(ctx, stringParam(values, "id"))
	case callSessionMessages:
		order := stringParam(values, "order")
		if order == "" {
			order = "desc"
		}
		value, err = c.ListMessagesPage(ctx, stringParam(values, "id"), intParam(values, "limit", 0), order, stringParam(values, "cursor"))
	case callSessionInterrupt:
		err = c.Abort(ctx, stringParam(values, "id"))
		value = map[string]any{}
	case callModels:
		value, err = c.ListModels(ctx)
	case callTurnStart, "turn/steer":
		rawParams, _ := json.Marshal(params)
		var input struct {
			SessionID  string       `json:"sessionID"`
			Text       string       `json:"text"`
			Files      []promptFile `json:"files"`
			ProviderID string       `json:"providerID"`
			ModelID    string       `json:"modelID"`
			Agent      string       `json:"agent"`
		}
		if json.Unmarshal(rawParams, &input) != nil {
			return nil, errors.New("invalid turn request")
		}
		var model *modelRef
		if input.ProviderID != "" && input.ModelID != "" {
			model = &modelRef{ProviderID: input.ProviderID, ModelID: input.ModelID}
		}
		expectedTurn := ""
		if method == "turn/steer" {
			expectedTurn = stringParam(values, "turnID")
		}
		userID, err := c.prompt(ctx, input.SessionID, input.Text, input.Files, model, input.Agent, expectedTurn)
		if err != nil {
			return nil, err
		}
		if method == "turn/steer" {
			value = map[string]any{"turn": map[string]any{"id": expectedTurn, "status": "inProgress"}, "userMessageId": userID}
		} else {
			value = map[string]any{"turn": map[string]any{"id": userID, "status": "inProgress"}, "userMessageId": userID}
		}
	case callTurnInterrupt:
		interrupted, err := c.Interrupt(ctx, stringParam(values, "sessionID"))
		if err != nil {
			return nil, err
		}
		value = map[string]any{"interrupted": interrupted}
	case callClientAttached:
		return json.RawMessage(`{"updated":true}`), nil
	default:
		return nil, fmt.Errorf("unsupported opencode call %q", method)
	}
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, errInvalidResponse
	}
	return raw, nil
}

func (c *Client) Info(ctx context.Context) (info, error) {
	var out info
	err := c.request(ctx, http.MethodGet, "/api/info", nil, nil, &out)
	return out, err
}

func (c *Client) Config(ctx context.Context) ([]configDocument, error) {
	var out []configDocument
	err := c.request(ctx, http.MethodGet, "/api/config", c.locationQuery(), nil, &out)
	return out, err
}

// ListSessions collects newest sessions first and slices to limit. OpenCode
// exposes cursor pagination, but the Modex list contract returns a null cursor,
// so the client walks every page (bounded) and slices afterwards rather than
// hiding older sessions in an undelivered cursor.
func (c *Client) ListSessions(ctx context.Context, limit int) ([]session, error) {
	if limit <= 0 {
		limit = 20
	}
	var all []session
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 20; page++ {
		query := url.Values{}
		query.Set("limit", "100")
		query.Set("order", "desc")
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var response sessionsPage
		if err := c.request(ctx, http.MethodGet, "/api/session", query, nil, &response); err != nil {
			return nil, err
		}
		for _, item := range response.Data {
			if item.ID == "" || seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			all = append(all, item)
		}
		if response.Cursor.Next == nil || *response.Cursor.Next == "" {
			break
		}
		cursor = *response.Cursor.Next
	}
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (c *Client) CreateSession(ctx context.Context, cwd string, model *modelRef, agent string) (session, error) {
	body := map[string]any{"location": map[string]string{"directory": cwd}}
	if model != nil && model.ProviderID != "" && model.ModelID != "" {
		// OpenCode's Model.Ref uses id/providerID; the task's modelID maps to id.
		body["model"] = map[string]string{"id": model.ModelID, "providerID": model.ProviderID}
	}
	if agent != "" {
		body["agent"] = agent
	}
	var response struct {
		Data session `json:"data"`
	}
	err := c.request(ctx, http.MethodPost, "/api/session", nil, body, &response)
	return response.Data, err
}

func (c *Client) ReadSession(ctx context.Context, id string) (session, error) {
	var response struct {
		Data session `json:"data"`
	}
	err := c.request(ctx, http.MethodGet, "/api/session/"+url.PathEscape(id), nil, nil, &response)
	return response.Data, err
}

func (c *Client) ListMessages(ctx context.Context, id string, limit int, order string) ([]message, error) {
	page, err := c.ListMessagesPage(ctx, id, limit, order, "")
	return page.Data, err
}

func (c *Client) ListMessagesPage(ctx context.Context, id string, limit int, order, cursor string) (messagePage, error) {
	query := url.Values{}
	// OpenCode encodes the order in the cursor and rejects a cursor combined
	// with order, so only the first page may specify one.
	if order != "" && cursor == "" {
		query.Set("order", order)
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	var page messagePage
	err := c.request(ctx, http.MethodGet, "/api/session/"+url.PathEscape(id)+"/message", query, nil, &page)
	return page, err
}

// Interrupt stops the active execution and reports whether one was running.
// OpenCode v2 exposes /interrupt; the planned /abort path does not exist in
// 2.4.x.
func (c *Client) Interrupt(ctx context.Context, id string) (bool, error) {
	var response struct {
		Interrupted bool `json:"interrupted"`
	}
	if err := c.request(ctx, http.MethodPost, "/api/session/"+url.PathEscape(id)+"/interrupt", nil, nil, &response); err != nil {
		return false, err
	}
	return response.Interrupted, nil
}

// Abort keeps the M2 API; callers that need the result use Interrupt.
func (c *Client) Abort(ctx context.Context, id string) error {
	_, err := c.Interrupt(ctx, id)
	return err
}

// Prompt sends a user prompt. The live v2 body is text/files (not the v1
// parts/model/agent shape); a requested model or agent is applied to the
// session first. It returns the accepted user message id used as the Modex
// turn id.
func (c *Client) Prompt(ctx context.Context, sessionID, text string, files []promptFile, model *modelRef, agent string) (string, error) {
	return c.prompt(ctx, sessionID, text, files, model, agent, "")
}

func (c *Client) prompt(ctx context.Context, sessionID, text string, files []promptFile, model *modelRef, agent, expectedTurn string) (string, error) {
	path := "/api/session/" + url.PathEscape(sessionID)
	if model != nil && model.ProviderID != "" && model.ModelID != "" {
		body := map[string]any{"model": map[string]string{"id": model.ModelID, "providerID": model.ProviderID}}
		if err := c.request(ctx, http.MethodPost, path+"/model", nil, body, nil); err != nil {
			return "", err
		}
	}
	if agent != "" {
		if err := c.request(ctx, http.MethodPost, path+"/agent", nil, map[string]string{"agent": agent}, nil); err != nil {
			return "", err
		}
	}
	body := map[string]any{"text": text}
	if expectedTurn != "" {
		body["delivery"] = "steer"
		body["metadata"] = map[string]string{"modex_turn_id": expectedTurn}
	}
	if len(files) > 0 {
		body["files"] = files
	}
	var response struct {
		Data struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"data"`
	}
	if err := c.request(ctx, http.MethodPost, path+"/prompt", nil, body, &response); err != nil {
		return "", err
	}
	if response.Data.ID != "" {
		return response.Data.ID, nil
	}
	if expectedTurn != "" {
		return "", errors.New("opencode did not confirm the follow-up; check the conversation before resending")
	}
	// Older or asynchronous prompt responses may omit the message; resolve the
	// newest user message instead.
	page, err := c.ListMessagesPage(ctx, sessionID, 5, "desc", "")
	if err != nil {
		return "", err
	}
	for _, item := range page.Data {
		if item.Type == "user" && item.ID != "" {
			return item.ID, nil
		}
	}
	return "", errors.New("opencode did not return the user message")
}

// ListPermissions returns the pending permission requests for a session.
func (c *Client) ListPermissions(ctx context.Context, sessionID string) (json.RawMessage, error) {
	var response json.RawMessage
	err := c.request(ctx, http.MethodGet, "/api/session/"+url.PathEscape(sessionID)+"/permission", nil, nil, &response)
	return response, err
}

// ReplyPermission answers a pending permission. OpenCode v2 accepts
// once/always/reject; Modex only ever sends once or reject.
func (c *Client) ReplyPermission(ctx context.Context, sessionID, requestID, decision string) error {
	switch decision {
	case "once", "always", "reject":
	default:
		return errors.New("invalid opencode permission decision")
	}
	body := map[string]string{"decision": decision}
	return c.request(ctx, http.MethodPost, "/api/session/"+url.PathEscape(sessionID)+"/permission/"+url.PathEscape(requestID)+"/reply", nil, body, nil)
}

func (c *Client) ListModels(ctx context.Context) ([]modelRef, error) {
	// OpenCode discovers provider models lazily: the first /api/model call for
	// a directory can return an empty catalogue while discovery runs. Retry
	// briefly so a cold session does not look model-less.
	var models []modelRef
	for attempt := 0; attempt < 4; attempt++ {
		var err error
		models, err = c.listModelsOnce(ctx)
		if err != nil {
			return nil, err
		}
		if len(models) > 0 || attempt == 3 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, errRequestFailed
		case <-time.After(250 * time.Millisecond):
		}
	}
	return models, nil
}

func (c *Client) listModelsOnce(ctx context.Context) ([]modelRef, error) {
	var response struct {
		Data []struct {
			ID         string `json:"id"`
			ModelID    string `json:"modelID"`
			ProviderID string `json:"providerID"`
			Name       string `json:"name"`
		} `json:"data"`
	}
	if err := c.request(ctx, http.MethodGet, "/api/model", c.locationQuery(), nil, &response); err != nil {
		return nil, err
	}
	models := make([]modelRef, 0, len(response.Data))
	for _, item := range response.Data {
		modelID := item.ModelID
		if modelID == "" {
			modelID = item.ID
		}
		if modelID == "" || item.ProviderID == "" {
			continue
		}
		models = append(models, modelRef{ProviderID: item.ProviderID, ModelID: modelID})
	}
	return models, nil
}

func (c *Client) locationQuery() url.Values {
	query := url.Values{}
	if c.cwd != "" {
		query.Set("directory", c.cwd)
	}
	return query
}

func (c *Client) request(ctx context.Context, method, path string, query url.Values, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return errInvalidResponse
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return errRequestFailed
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return errRequestFailed
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// Never echo the upstream body: it may contain credentials or prompts.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%w (status %d)", errRequestFailed, resp.StatusCode)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponse))
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return errRequestFailed
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return errInvalidResponse
	}
	if json.Unmarshal(raw, out) != nil {
		return errInvalidResponse
	}
	return nil
}

func stringParam(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func intParam(values map[string]any, key string, fallback int) int {
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return int(parsed)
		}
	}
	return fallback
}

// resolveDefaultModel finds default_agent in the layered config documents and
// returns that agent's configured model. Later documents win. If no agent or
// model is configured the result is empty, matching the documented fallback.
func resolveDefaultModel(docs []configDocument) (provider, model string) {
	agent := ""
	agents := map[string]json.RawMessage{}
	for _, doc := range docs {
		if doc.Type != "" && doc.Type != "document" {
			continue
		}
		if doc.Info.DefaultAgent != "" {
			agent = doc.Info.DefaultAgent
		}
		for name, configured := range doc.Info.Agents {
			agents[name] = configured.Model
		}
	}
	if agent == "" {
		return "", ""
	}
	return resolveModelRef(agents[agent])
}

func resolveModelRef(raw json.RawMessage) (provider, model string) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", ""
	}
	var ref struct {
		ProviderID string `json:"providerID"`
		Model      string `json:"model"`
		ModelID    string `json:"modelID"`
	}
	if json.Unmarshal(raw, &ref) == nil && ref.ProviderID != "" {
		model = ref.Model
		if model == "" {
			model = ref.ModelID
		}
		return ref.ProviderID, model
	}
	var compact string
	if json.Unmarshal(raw, &compact) == nil {
		if slash := strings.Index(compact, "/"); slash > 0 {
			provider = compact[:slash]
			model = compact[slash+1:]
			if hash := strings.Index(model, "#"); hash >= 0 {
				model = model[:hash]
			}
			return provider, model
		}
	}
	return "", ""
}
