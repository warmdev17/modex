package opencode

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/warmdev/modex/internal/bridge"
)

// EventCaller mirrors bridge.EventCaller but speaks the OpenCode call seam.
type EventCaller func(ctx context.Context, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error)

// Caller is the event-free form used by Serve.
type Caller func(ctx context.Context, method string, params any) (json.RawMessage, error)

var unsupportedMethods = map[string]bool{
	"threads/archive":     true,
	"threads/unarchive":   true,
	"threads/delete":      true,
	"threads/fork":        true,
	"threads/takeover":    true,
	"threads/mode":        true,
	"threads/status":      true,
	"threads/permissions": true,
	"plans/respond":       true,
	"projects/list":       true,
	"projects/create":     true,
	"references/list":     true,
	"turn/start":          true,
	"turn/interrupt":      true,
	"decision/respond":    true,
}

func Handle(ctx context.Context, cwd string, cmd bridge.Command, call Caller) (any, error) {
	return HandleEvents(ctx, cwd, cmd, func(ctx context.Context, method string, params any, _ func(string, json.RawMessage)) (json.RawMessage, error) {
		return call(ctx, method, params)
	}, nil)
}

// HandleEvents validates a Modex command and projects the OpenCode read
// surface into the same result shapes the Codex bridge returns.
func HandleEvents(ctx context.Context, cwd string, cmd bridge.Command, call EventCaller, emit func(string, json.RawMessage)) (any, error) {
	if unsupportedMethods[cmd.Method] {
		return nil, fmt.Errorf("%s is unsupported by the opencode backend", cmd.Method)
	}
	p := map[string]json.RawMessage{}
	if len(cmd.Params) > 0 && string(cmd.Params) != "null" {
		if json.Unmarshal(cmd.Params, &p) != nil || p == nil {
			return nil, errors.New("params must be an object")
		}
	}
	allowed := map[string]bool{}
	switch cmd.Method {
	case "status":
	case "threads/list":
		allowed["all_workspaces"] = true
		allowed["archived"] = true
		allowed["limit"] = true
		allowed["cursor"] = true
	case "threads/start":
		allowed["standalone"] = true
		allowed["project_id"] = true
		allowed["model"] = true
		allowed["permission_mode"] = true
		allowed["workspace"] = true
	case "threads/read":
		allowed["workspace"] = true
		allowed["thread_id"] = true
		allowed["include_turns"] = true
		allowed["limit"] = true
		allowed["cursor"] = true
	case "threads/resume":
		allowed["workspace"] = true
		allowed["thread_id"] = true
	case "models/list":
		allowed["limit"] = true
		allowed["cursor"] = true
	case "client/attached":
		allowed["attached"] = true
	default:
		return nil, fmt.Errorf("%s is unsupported by the opencode backend", cmd.Method)
	}
	for key := range p {
		if !allowed[key] {
			return nil, errors.New("unknown parameter")
		}
	}
	if raw, ok := p["workspace"]; ok {
		var workspace string
		if json.Unmarshal(raw, &workspace) != nil || !filepath.IsAbs(workspace) || strings.ContainsAny(workspace, "\r\n\x00") || len(workspace) > 32768 {
			return nil, errors.New("workspace must be an absolute path")
		}
		cwd = workspace
	}

	switch cmd.Method {
	case "status":
		// Info() proves the server is reachable; the result itself is not
		// forwarded, and raw config documents never leave the host.
		if _, err := call(ctx, callInfo, nil, emit); err != nil {
			return nil, err
		}
		docs, err := callInto[[]configDocument](ctx, call, callConfig, nil, emit)
		if err != nil {
			return nil, err
		}
		provider, model := resolveDefaultModel(docs)
		return map[string]any{
			"workspace": cwd,
			"config":    map[string]any{"provider": provider, "model": model},
		}, nil
	case "threads/list":
		limit := 20
		if raw, ok := p["limit"]; ok {
			parsed, err := limitParam(raw)
			if err != nil {
				return nil, err
			}
			limit = parsed
		}
		if raw, ok := p["cursor"]; ok {
			if _, err := cursorParam(raw); err != nil {
				return nil, err
			}
		}
		if raw, ok := p["all_workspaces"]; ok {
			if _, err := boolParam(raw, "all_workspaces"); err != nil {
				return nil, err
			}
		}
		if raw, ok := p["archived"]; ok {
			archived, err := boolParam(raw, "archived")
			if err != nil {
				return nil, err
			}
			if archived {
				return nil, errors.New("archived is unsupported by the opencode backend")
			}
		}
		sessions, err := callInto[[]session](ctx, call, callSessionList, map[string]any{"limit": limit}, emit)
		if err != nil {
			return nil, err
		}
		return projectSessionList(sessions), nil
	case "threads/start":
		if raw, ok := p["standalone"]; ok {
			standalone, err := boolParam(raw, "standalone")
			if err != nil {
				return nil, err
			}
			if standalone {
				return nil, errors.New("standalone is unsupported by the opencode backend")
			}
		}
		if raw, ok := p["project_id"]; ok && string(raw) != "null" {
			return nil, errors.New("project_id is unsupported by the opencode backend")
		}
		if raw, ok := p["permission_mode"]; ok && string(raw) != "null" {
			return nil, errors.New("permission_mode is unsupported by the opencode backend")
		}
		modelID := ""
		if raw, ok := p["model"]; ok {
			parsed, err := modelParam(raw)
			if err != nil {
				return nil, err
			}
			modelID = parsed
		}
		var model *modelRef
		if modelID != "" {
			provider, id := splitModel(modelID)
			if provider == "" {
				models, err := callInto[[]modelRef](ctx, call, callModels, nil, emit)
				if err != nil {
					return nil, err
				}
				for _, candidate := range models {
					if candidate.ModelID == id {
						provider = candidate.ProviderID
						break
					}
				}
			}
			if provider == "" {
				return nil, errors.New("model is not available for the opencode backend")
			}
			model = &modelRef{ProviderID: provider, ModelID: id}
		}
		params := map[string]any{"directory": cwd, "agent": ""}
		if model != nil {
			params["providerID"] = model.ProviderID
			params["modelID"] = model.ModelID
		}
		created, err := callInto[session](ctx, call, callSessionCreate, params, emit)
		if err != nil {
			return nil, err
		}
		if created.ID == "" || created.Location.Directory != cwd {
			return nil, errors.New("invalid thread response")
		}
		return map[string]any{"thread": projectThread(created), "model": created.Model.ID}, nil
	case "threads/read":
		id, err := requiredThreadID(p)
		if err != nil {
			return nil, err
		}
		include := false
		if raw, ok := p["include_turns"]; ok {
			include, err = boolParam(raw, "include_turns")
			if err != nil {
				return nil, err
			}
		}
		limit := 0
		hasLimit := false
		if raw, ok := p["limit"]; ok {
			limit, err = limitParam(raw)
			if err != nil {
				return nil, err
			}
			hasLimit = true
		}
		cursor := ""
		if raw, ok := p["cursor"]; ok {
			cursor, err = cursorParam(raw)
			if err != nil {
				return nil, err
			}
		}
		if !include && (hasLimit || cursor != "") {
			return nil, errors.New("history limit and cursor require include_turns")
		}
		thread, err := readSession(ctx, call, id, cwd, emit)
		if err != nil {
			return nil, err
		}
		if !include {
			return map[string]any{"thread": projectThread(thread)}, nil
		}
		messages, next, err := fetchMessages(ctx, call, id, limit, hasLimit, cursor, emit)
		if err != nil {
			return nil, err
		}
		entries, active, activeID := projectMessages(id, cwd, messages)
		result := map[string]any{
			"thread":         projectThread(thread),
			"messages":       entries,
			"active_turn":    active,
			"active_turn_id": activeID,
		}
		if hasLimit {
			// Android requires an opaque revision on every paged history read.
			result["history_revision"] = historyRevision(id, messages)
		}
		if next != "" {
			result["next_cursor"] = next
		} else if hasLimit || cursor != "" {
			result["next_cursor"] = nil
		}
		return result, nil
	case "threads/resume":
		id, err := requiredThreadID(p)
		if err != nil {
			return nil, err
		}
		thread, err := readSession(ctx, call, id, cwd, emit)
		if err != nil {
			return nil, err
		}
		return map[string]any{"thread": projectThread(thread), "model": thread.Model.ID}, nil
	case "models/list":
		if raw, ok := p["limit"]; ok {
			if _, err := limitParam(raw); err != nil {
				return nil, err
			}
		}
		if raw, ok := p["cursor"]; ok {
			if _, err := cursorParam(raw); err != nil {
				return nil, err
			}
		}
		models, err := callInto[[]modelRef](ctx, call, callModels, nil, emit)
		if err != nil {
			return nil, err
		}
		return projectModelList(models), nil
	case "client/attached":
		if _, err := boolParam(p["attached"], "attached"); err != nil {
			return nil, err
		}
		return map[string]any{"updated": true}, nil
	}
	return nil, fmt.Errorf("%s is unsupported by the opencode backend", cmd.Method)
}

func callInto[T any](ctx context.Context, call EventCaller, method string, params any, emit func(string, json.RawMessage)) (T, error) {
	var value T
	raw, err := call(ctx, method, params, emit)
	if err != nil {
		return value, err
	}
	if len(raw) == 0 {
		return value, errors.New("empty opencode response")
	}
	if json.Unmarshal(raw, &value) != nil {
		return value, errors.New("invalid opencode response")
	}
	return value, nil
}

func readSession(ctx context.Context, call EventCaller, id, cwd string, emit func(string, json.RawMessage)) (session, error) {
	thread, err := callInto[session](ctx, call, callSessionRead, map[string]any{"id": id}, emit)
	if err != nil {
		return session{}, err
	}
	if thread.ID != id {
		return session{}, errors.New("invalid thread response")
	}
	if thread.Location.Directory != cwd {
		return session{}, errors.New("thread outside workspace")
	}
	return thread, nil
}

// fetchMessages mirrors the Codex history page: OpenCode is queried newest
// first, each page is reversed to chronological order, and cursor.next (which
// points toward older messages in desc order) becomes the Modex next_cursor.
func fetchMessages(ctx context.Context, call EventCaller, id string, limit int, hasLimit bool, cursor string, emit func(string, json.RawMessage)) ([]message, string, error) {
	const maxPages = 32
	const maxMessages = 2000
	if hasLimit {
		page, err := callInto[messagePage](ctx, call, callSessionMessages, map[string]any{"id": id, "limit": limit, "order": "desc", "cursor": cursor}, emit)
		if err != nil {
			return nil, "", err
		}
		reverseMessages(page.Data)
		next := ""
		if page.Cursor.Next != nil {
			next = *page.Cursor.Next
		}
		return page.Data, next, nil
	}
	var all []message
	seen := map[string]bool{}
	next := ""
	for page := 0; page < maxPages; page++ {
		result, err := callInto[messagePage](ctx, call, callSessionMessages, map[string]any{"id": id, "order": "desc", "cursor": next}, emit)
		if err != nil {
			return nil, "", err
		}
		all = append(all, result.Data...)
		if result.Cursor.Next == nil || *result.Cursor.Next == "" {
			reverseMessages(all)
			return all, "", nil
		}
		if seen[*result.Cursor.Next] {
			return nil, "", errors.New("opencode message cursor did not advance")
		}
		seen[*result.Cursor.Next] = true
		next = *result.Cursor.Next
		if len(all) >= maxMessages {
			reverseMessages(all)
			return all, next, nil
		}
	}
	reverseMessages(all)
	return all, next, nil
}

func reverseMessages(messages []message) {
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
}

// historyRevision is an opaque, content-addressed cache token. It is stable
// for identical pages and changes whenever the thread id or any returned
// message (text, tool output, outcome, timing) changes.
func historyRevision(threadID string, messages []message) string {
	hash := sha256.New()
	hash.Write([]byte(threadID))
	hash.Write([]byte{0})
	for _, item := range messages {
		if raw, err := json.Marshal(item); err == nil {
			hash.Write(raw)
		}
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func requiredThreadID(p map[string]json.RawMessage) (string, error) {
	var id string
	raw, ok := p["thread_id"]
	if !ok || json.Unmarshal(raw, &id) != nil || strings.TrimSpace(id) == "" || len(id) > 256 || strings.ContainsAny(id, "\r\n\x00") {
		return "", errors.New("thread_id must be a nonempty single-line string up to 256 bytes")
	}
	return id, nil
}

func boolParam(raw json.RawMessage, name string) (bool, error) {
	var value bool
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
		return false, fmt.Errorf("%s must be boolean", name)
	}
	return value, nil
}

func limitParam(raw json.RawMessage) (int, error) {
	var limit int
	if json.Unmarshal(raw, &limit) != nil || limit < 1 || limit > 100 {
		return 0, errors.New("limit must be an integer from 1 to 100")
	}
	return limit, nil
}

func cursorParam(raw json.RawMessage) (string, error) {
	var cursor string
	if string(raw) == "null" || json.Unmarshal(raw, &cursor) != nil || len(cursor) > 8192 || strings.ContainsAny(cursor, "\r\n\x00") {
		return "", errors.New("cursor must be a string")
	}
	return cursor, nil
}

func modelParam(raw json.RawMessage) (string, error) {
	var model string
	if json.Unmarshal(raw, &model) != nil || strings.TrimSpace(model) == "" || len(model) > 256 || strings.ContainsAny(model, "\r\n\x00") {
		return "", errors.New("model must be a nonempty single-line string up to 256 bytes")
	}
	return model, nil
}

func splitModel(model string) (provider, id string) {
	if slash := strings.Index(model, "/"); slash > 0 && slash < len(model)-1 {
		return model[:slash], model[slash+1:]
	}
	return "", model
}

func Serve(ctx context.Context, in io.Reader, out io.Writer, cwd string, call Caller) error {
	return ServeEvents(ctx, in, out, cwd, func(ctx context.Context, method string, params any, _ func(string, json.RawMessage)) (json.RawMessage, error) {
		return call(ctx, method, params)
	})
}

// ServeEvents reads Modex commands from in and writes framed replies to out.
// M2 emits no events; emit is threaded through so M3 can stream without
// changing the dispatch signature.
func ServeEvents(ctx context.Context, in io.Reader, out io.Writer, cwd string, call EventCaller) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	var outputMu sync.Mutex
	var outputErr error
	write := func(response []byte) {
		outputMu.Lock()
		defer outputMu.Unlock()
		if outputErr == nil {
			_, outputErr = out.Write(append(response, '\n'))
		}
	}
	emit := func(method string, params json.RawMessage) {
		if params == nil {
			params = json.RawMessage("null")
		}
		response, err := json.Marshal(struct {
			Event  string          `json:"event"`
			Params json.RawMessage `json:"params"`
		}{method, params})
		if err == nil {
			write(response)
		}
	}
	respond := func(cmd bridge.Command, result any, err error) {
		var response []byte
		if err == nil {
			response, err = bridge.EncodeResult(cmd.ID, result)
		}
		if err != nil {
			response, _ = bridge.EncodeError(cmd.ID, err.Error())
		}
		write(response)
	}
	for scanner.Scan() {
		cmd, err := bridge.DecodeCommand(scanner.Bytes())
		if err != nil {
			respond(cmd, nil, err)
			continue
		}
		result, err := HandleEvents(ctx, cwd, cmd, call, emit)
		respond(cmd, result, err)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	outputMu.Lock()
	defer outputMu.Unlock()
	return outputErr
}
