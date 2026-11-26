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
	"net/url"
	"os"
	"path/filepath"
	"strings"

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
}

func Handle(ctx context.Context, cwd string, cmd bridge.Command, call Caller) (any, error) {
	return HandleEvents(ctx, cwd, cmd, func(ctx context.Context, method string, params any, _ func(string, json.RawMessage)) (json.RawMessage, error) {
		return call(ctx, method, params)
	}, nil)
}

// HandleEvents validates a Modex command and projects the OpenCode read
// surface into the same result shapes the Codex bridge returns.
func HandleEvents(ctx context.Context, cwd string, cmd bridge.Command, call EventCaller, emit func(string, json.RawMessage)) (any, error) {
	return handleEvents(ctx, cwd, cmd, call, emit, nil)
}

func handleEvents(ctx context.Context, cwd string, cmd bridge.Command, call EventCaller, emit func(string, json.RawMessage), synth *Synthesizer) (any, error) {
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
	case "turn/start", "turn/steer":
		if cmd.Method == "turn/steer" {
			allowed["turn_id"] = true
		}
		allowed["references"] = true
		allowed["attachments"] = true
		allowed["thread_id"] = true
		allowed["text"] = true
		allowed["model"] = true
		allowed["effort"] = true
		allowed["workspace"] = true
	case "turn/interrupt":
		allowed["thread_id"] = true
		allowed["turn_id"] = true
		allowed["workspace"] = true
	case "decision/respond":
		allowed["decision_id"] = true
		allowed["result"] = true
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
	standalone := false
	if raw, ok := p["standalone"]; ok {
		var err error
		standalone, err = boolParam(raw, "standalone")
		if err != nil {
			return nil, err
		}
	}
	if raw, ok := p["workspace"]; ok {
		var workspace string
		if json.Unmarshal(raw, &workspace) != nil || (!(standalone && workspace == "") && !filepath.IsAbs(workspace)) || strings.ContainsAny(workspace, "\r\n\x00") || len(workspace) > 32768 {
			return nil, errors.New("workspace must be an absolute path")
		}
		if standalone && workspace != "" {
			return nil, errors.New("standalone cannot be combined with workspace")
		}
		if workspace != "" {
			cwd = workspace
		}
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
			"config":    map[string]any{"provider": provider, "model": modelChoiceID(provider, model)},
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
		if standalone {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			cwd = filepath.Join(home, "Documents", "Codex")
			if err := os.MkdirAll(cwd, 0700); err != nil {
				return nil, err
			}
		}
		model, err := resolveModelChoice(ctx, call, modelID, emit)
		if err != nil {
			return nil, err
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
		return map[string]any{"thread": projectThread(created), "model": modelChoiceID(created.Model.ProviderID, created.Model.ID)}, nil
	case "turn/start", "turn/steer":
		id, err := requiredThreadID(p)
		if err != nil {
			return nil, err
		}
		var turn string
		if cmd.Method == "turn/steer" {
			if raw, ok := p["turn_id"]; !ok || json.Unmarshal(raw, &turn) != nil || strings.TrimSpace(turn) == "" || len(turn) > 256 || strings.ContainsAny(turn, "\r\n\x00") {
				return nil, errors.New("turn_id must be a nonempty single-line string up to 256 bytes")
			}
		}
		var text string
		rawText, ok := p["text"]
		if !ok || string(rawText) == "null" || json.Unmarshal(rawText, &text) != nil {
			return nil, errors.New("text must be a string")
		}
		if raw, ok := p["references"]; ok && string(raw) != "null" {
			var references []json.RawMessage
			if json.Unmarshal(raw, &references) != nil {
				return nil, errors.New("invalid references")
			}
			if len(references) > 0 {
				return nil, errors.New("references are unsupported by the opencode backend")
			}
		}
		var files []promptFile
		var confirmedAttachments []chatAttachment
		content := []map[string]string{{"type": "text", "text": text}}
		if raw, ok := p["attachments"]; ok && string(raw) != "null" {
			var attachments []struct {
				Path string `json:"path"`
				Kind string `json:"kind"`
			}
			if json.Unmarshal(raw, &attachments) != nil || len(attachments) > 32 {
				return nil, errors.New("invalid attachments")
			}
			for _, attachment := range attachments {
				attachment.Path = attachmentPath(attachment.Path)
				if !validAttachmentPath(attachment.Path) {
					return nil, errors.New("attachment path must be an absolute path")
				}
				if attachment.Kind != "" && attachment.Kind != "image" && attachment.Kind != "file" {
					return nil, errors.New("invalid attachment kind")
				}
				// Live v2 PromptInput.FileAttachment takes uri/name; MIME is
				// inferred by OpenCode and is not accepted in this body.
				path := filepath.ToSlash(attachment.Path)
				uri := url.URL{Scheme: "file", Path: path}
				if filepath.VolumeName(attachment.Path) != "" && !strings.HasPrefix(path, "/") {
					uri.Path = "/" + path
				}
				files = append(files, promptFile{URI: uri.String(), Name: filepath.Base(attachment.Path)})
				kind := attachment.Kind
				if kind == "" {
					kind = "file"
				}
				confirmedAttachments = append(confirmedAttachments, chatAttachment{Path: attachment.Path, Name: filepath.Base(attachment.Path), Kind: kind})
				if attachment.Kind == "image" {
					content = append(content, map[string]string{"type": "localImage", "path": attachment.Path})
				}
			}
		}
		modelID := ""
		if raw, ok := p["model"]; ok {
			parsed, err := modelParam(raw)
			if err != nil {
				return nil, err
			}
			modelID = parsed
		}
		if raw, ok := p["effort"]; ok && string(raw) != "null" {
			var effort string
			if json.Unmarshal(raw, &effort) != nil || strings.TrimSpace(effort) == "" || len(effort) > 64 || strings.ContainsAny(effort, "\r\n\x00") {
				return nil, errors.New("invalid reasoning effort")
			}
		}
		if _, err := readSession(ctx, call, id, cwd, emit); err != nil {
			return nil, err
		}
		var model *modelRef
		if cmd.Method == "turn/start" {
			model, err = resolveModelChoice(ctx, call, modelID, emit)
			if err != nil {
				return nil, err
			}
		}
		params := map[string]any{"sessionID": id, "text": text, "directory": cwd}
		if turn != "" {
			params["turnID"] = turn
		}
		if len(files) > 0 {
			params["files"] = files
		}
		if model != nil {
			params["providerID"] = model.ProviderID
			params["modelID"] = model.ModelID
		}
		raw, err := call(ctx, cmd.Method, params, emit)
		if err != nil {
			return nil, err
		}
		var result struct {
			UserMessageID string `json:"userMessageId"`
			Turn          struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if json.Unmarshal(raw, &result) != nil || result.Turn.ID == "" {
			return nil, errors.New("invalid turn response")
		}
		if emit != nil && result.UserMessageID != "" {
			item := map[string]any{"id": result.UserMessageID, "type": "userMessage", "content": content}
			if len(confirmedAttachments) != 0 {
				item["attachments"] = confirmedAttachments
			}
			event, err := json.Marshal(map[string]any{
				"threadId": id, "turnId": result.Turn.ID,
				"item": item,
			})
			if err != nil {
				return nil, err
			}
			emit("item/completed", event)
		}
		return map[string]any{"turn": map[string]any{"id": result.Turn.ID, "status": "inProgress"}}, nil
	case "turn/interrupt":
		id, err := requiredThreadID(p)
		if err != nil {
			return nil, err
		}
		var turn string
		if raw, ok := p["turn_id"]; !ok || json.Unmarshal(raw, &turn) != nil || strings.TrimSpace(turn) == "" || len(turn) > 256 || strings.ContainsAny(turn, "\r\n\x00") {
			return nil, errors.New("turn_id must be a nonempty single-line string up to 256 bytes")
		}
		if _, err := readSession(ctx, call, id, cwd, emit); err != nil {
			return nil, err
		}
		if _, err := call(ctx, callTurnInterrupt, map[string]any{"sessionID": id, "turnID": turn}, emit); err != nil {
			return nil, err
		}
		return map[string]any{}, nil
	case "decision/respond":
		var id string
		if raw, ok := p["decision_id"]; !ok || json.Unmarshal(raw, &id) != nil || id == "" || len(id) > 256 || strings.ContainsAny(id, "\r\n\x00") {
			return nil, errors.New("decision_id is required")
		}
		if len(p["result"]) == 0 || string(p["result"]) == "null" {
			return nil, errors.New("result is required")
		}
		raw, err := call(ctx, callDecisionRespond, map[string]any{"decisionID": id, "result": json.RawMessage(p["result"])}, emit)
		if err != nil {
			return nil, err
		}
		var result map[string]any
		if json.Unmarshal(raw, &result) != nil {
			return nil, errors.New("invalid decision response")
		}
		return result, nil
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
			limit, err = historyLimitParam(raw)
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
		if cursor != "" && !hasLimit {
			return nil, errors.New("history cursor requires limit")
		}
		if hasLimit && !include {
			return nil, errors.New("history limit must be 1..50 with include_turns")
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
		if cursor == "" && synth != nil {
			if turnID, running := synth.ActiveTurn(id); running {
				active, activeID = true, turnID
			} else if turnID != "" {
				active, activeID = false, ""
			}
		}
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
		return map[string]any{"thread": projectThread(thread), "model": modelChoiceID(thread.Model.ProviderID, thread.Model.ID)}, nil
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
		attached, err := boolParam(p["attached"], "attached")
		if err != nil {
			return nil, err
		}
		// Route through the seam so the host can reset/replay pending decisions.
		raw, err := call(ctx, callClientAttached, map[string]any{"attached": attached}, emit)
		if err != nil {
			return nil, err
		}
		var result map[string]any
		if json.Unmarshal(raw, &result) != nil {
			return nil, errors.New("invalid attach response")
		}
		return result, nil
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
	// Bump the prefix when the projection changes so Android invalidates its
	// cached pages even if the upstream content is identical.
	hash.Write([]byte("opencode-history-v2\x00"))
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

// historyLimitParam mirrors bridge history paging (1..50).
func historyLimitParam(raw json.RawMessage) (int, error) {
	var limit int
	if json.Unmarshal(raw, &limit) != nil || limit < 1 || limit > 50 {
		return 0, errors.New("history limit must be an integer from 1 to 50")
	}
	return limit, nil
}

func cursorParam(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var cursor string
	if json.Unmarshal(raw, &cursor) != nil || len(cursor) > 8192 || strings.ContainsAny(cursor, "\r\n\x00") {
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

// resolveModelChoice accepts either provider/model or a bare model id from the
// catalogue. An empty value means "use the session/default model".
func resolveModelChoice(ctx context.Context, call EventCaller, model string, emit func(string, json.RawMessage)) (*modelRef, error) {
	if model == "" {
		return nil, nil
	}
	provider, id := splitModel(model)
	if provider == "" {
		models, err := callInto[[]modelRef](ctx, call, callModels, nil, emit)
		if err != nil {
			return nil, err
		}
		for _, candidate := range models {
			if candidate.ModelID != id || candidate.ProviderID == "" {
				continue
			}
			if provider != "" && provider != candidate.ProviderID {
				return nil, errors.New("model is offered by multiple OpenCode providers; select a provider-qualified model")
			}
			provider = candidate.ProviderID
		}
	}
	if provider == "" {
		return nil, errors.New("model is not available for the opencode backend")
	}
	return &modelRef{ProviderID: provider, ModelID: id}, nil
}

func Serve(ctx context.Context, in io.Reader, out io.Writer, cwd string, call Caller) error {
	return ServeEvents(ctx, in, out, cwd, func(ctx context.Context, method string, params any, _ func(string, json.RawMessage)) (json.RawMessage, error) {
		return call(ctx, method, params)
	})
}

// ServeEvents reads Modex commands from in and writes framed replies to out.
func ServeEvents(ctx context.Context, in io.Reader, out io.Writer, cwd string, call EventCaller) error {
	return ServeEventsWithWriter(ctx, in, cwd, call, NewWriter(out), nil)
}

// ServeEventsWithWriter lets the host share one framed writer between command
// replies and the SSE event stream. turn/interrupt is handled out-of-band so a
// long turn/start cannot starve it, mirroring the Codex bridge.
func ServeEventsWithWriter(ctx context.Context, in io.Reader, cwd string, call EventCaller, writer *Writer, synth *Synthesizer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	handle := func(cmd bridge.Command) {
		result, err := handleEvents(ctx, cwd, cmd, call, writer.Emit, synth)
		if err != nil {
			writer.Error(cmd.ID, err)
			return
		}
		writer.Result(cmd.ID, result)
	}
	commands := make(chan bridge.Command, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for cmd := range commands {
			handle(cmd)
		}
	}()
	for scanner.Scan() {
		cmd, err := bridge.DecodeCommand(scanner.Bytes())
		if err != nil {
			writer.Error(cmd.ID, err)
			continue
		}
		if cmd.Method == "turn/interrupt" || cmd.Method == "decision/respond" {
			handle(cmd)
			continue
		}
		select {
		case commands <- cmd:
		default:
			writer.Error(cmd.ID, errors.New("command queue full; request not dispatched"))
		}
	}
	close(commands)
	<-done
	if err := scanner.Err(); err != nil {
		return err
	}
	return writer.Err()
}
