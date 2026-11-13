package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/warmdev/modex/internal/diagnostic"
)

type Caller func(context.Context, string, any) (json.RawMessage, error)
type EventCaller func(context.Context, string, any, func(string, json.RawMessage)) (json.RawMessage, error)

type Thread struct {
	ID       string `json:"id"`
	Cwd      string `json:"cwd"`
	Provider string `json:"modelProvider"`
	Model    string `json:"model,omitempty"`
	Effort   string `json:"reasoningEffort,omitempty"`
}

type threadListItem struct {
	Thread
	ProjectID   *string `json:"projectId"`
	Projectless bool    `json:"projectless,omitempty"`
	Preview     string  `json:"preview,omitempty"`
	Name        string  `json:"name,omitempty"`
}

func safePreview(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 160 {
		return string(runes[:160]) + "…"
	}
	return value
}

func Handle(ctx context.Context, cwd string, cmd Command, call Caller) (any, error) {
	return HandleEvents(ctx, cwd, cmd, func(ctx context.Context, method string, params any, _ func(string, json.RawMessage)) (json.RawMessage, error) {
		return call(ctx, method, params)
	}, nil)
}

func HandleEvents(ctx context.Context, cwd string, cmd Command, call EventCaller, emit func(string, json.RawMessage)) (any, error) {
	p := map[string]json.RawMessage{}
	if len(cmd.Params) > 0 && string(cmd.Params) != "null" {
		if json.Unmarshal(cmd.Params, &p) != nil || p == nil {
			return nil, errors.New("params must be an object")
		}
	}
	allowed := map[string]bool{}
	switch cmd.Method {
	case "client/attached":
		allowed["attached"] = true
	case "status":
	case "threads/start":
		allowed["model"] = true
		allowed["permission_mode"] = true
		allowed["workspace"] = true
	case "threads/list", "models/list", "projects/list":
		if cmd.Method == "threads/list" {
			allowed["all_workspaces"] = true
		}
		allowed["limit"] = true
		allowed["cursor"] = true
	case "threads/read", "threads/resume", "threads/takeover":
		if cmd.Method == "threads/takeover" {
			allowed["confirm"] = true
		}
		allowed["workspace"] = true
		allowed["thread_id"] = true
		if cmd.Method == "threads/read" {
			allowed["include_turns"] = true
		}
	case "turn/interrupt":
		allowed["thread_id"] = true
		allowed["turn_id"] = true
		allowed["workspace"] = true
	case "turn/start":
		allowed["attachments"] = true
		allowed["thread_id"] = true
		allowed["text"] = true
		allowed["model"] = true
		allowed["effort"] = true
		allowed["workspace"] = true
	case "decision/respond":
		allowed["decision_id"] = true
		allowed["result"] = true
	default:
		return nil, errors.New("unsupported Modex command")
	}
	for k := range p {
		if !allowed[k] {
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
	fetch := func(method string, params any) (json.RawMessage, error) {
		r, err := call(ctx, method, params, emit)
		if err != nil {
			var rejection *diagnostic.RPCError
			if errors.As(err, &rejection) {
				return nil, errors.New("Codex request failed: " + rejection.Category())
			}
			return nil, errors.New("Codex request failed; raw error suppressed")
		}
		return r, nil
	}
	if cmd.Method == "threads/takeover" {
		var confirm bool
		if json.Unmarshal(p["confirm"], &confirm) != nil || !confirm {
			return nil, errors.New("takeover requires explicit confirmation")
		}
	}
	switch cmd.Method {
	case "client/attached":
		var attached bool
		if raw, ok := p["attached"]; !ok || string(raw) == "null" || json.Unmarshal(raw, &attached) != nil {
			return nil, errors.New("attached must be boolean")
		}
		return fetch("client/attached", map[string]any{"attached": attached})
	case "threads/start":
		params := map[string]any{"cwd": cwd}
		if raw, ok := p["permission_mode"]; ok {
			var mode string
			if json.Unmarshal(raw, &mode) != nil {
				return nil, errors.New("invalid permission_mode")
			}
			switch mode {
			case "host-default":
			case "read-only", "workspace-write", "full-access":
				params["sandbox"] = mode
				params["approvalPolicy"] = "on-request"
				params["approvalsReviewer"] = "user"
				if mode == "full-access" {
					params["sandbox"] = "danger-full-access"
					params["approvalPolicy"] = "never"
				}
			default:
				return nil, errors.New("invalid permission_mode")
			}
		}
		if raw, ok := p["model"]; ok {
			var model string
			if json.Unmarshal(raw, &model) != nil || strings.TrimSpace(model) == "" || len(model) > 256 || strings.ContainsAny(model, "\r\n\x00") {
				return nil, errors.New("model must be a nonempty single-line string up to 256 bytes")
			}
			params["model"] = model
		}
		raw, err := fetch("thread/start", params)
		if err != nil {
			return nil, err
		}
		result, err := projectSession(raw)
		if err != nil || result.Thread.ID == "" {
			return nil, errors.New("invalid thread response")
		}
		if result.Thread.Cwd != cwd {
			return nil, errors.New("thread outside workspace")
		}
		return result, nil
	case "status":
		raw, err := fetch("config/read", map[string]any{"cwd": cwd, "includeLayers": false})
		if err != nil {
			return nil, err
		}
		summary, err := diagnostic.Summarize(raw)
		if err != nil {
			return nil, err
		}
		return map[string]any{"workspace": cwd, "config": summary}, nil
	case "models/list", "projects/list":
		limit := 100
		if raw, ok := p["limit"]; ok {
			if json.Unmarshal(raw, &limit) != nil || limit < 1 || limit > 100 {
				return nil, errors.New("invalid model limit")
			}
		}
		params := map[string]any{"limit": limit, "includeHidden": false}
		if raw, ok := p["cursor"]; ok {
			var cursor string
			if string(raw) == "null" || json.Unmarshal(raw, &cursor) != nil || len(cursor) > 4096 {
				return nil, errors.New("invalid model cursor")
			}
			if cursor != "" {
				params["cursor"] = cursor
			}
		}
		if cmd.Method == "projects/list" {
			delete(params, "includeHidden")
			raw, err := fetch("project/list", params)
			if err != nil {
				return nil, err
			}
			var page struct {
				Data []projectEntry `json:"data"`
				Next *string        `json:"nextCursor"`
			}
			if json.Unmarshal(raw, &page) != nil || page.Data == nil {
				return nil, errors.New("invalid project list response")
			}
			return struct {
				Data []projectEntry `json:"data"`
				Next *string        `json:"next_cursor"`
			}{page.Data, page.Next}, nil
		}
		raw, err := fetch("model/list", params)
		if err != nil {
			return nil, err
		}
		var page struct {
			Data []modelEntry `json:"data"`
			Next *string      `json:"nextCursor"`
		}
		if json.Unmarshal(raw, &page) != nil || page.Data == nil {
			return nil, errors.New("invalid model list response")
		}
		return struct {
			Data []modelEntry `json:"data"`
			Next *string      `json:"next_cursor"`
		}{page.Data, page.Next}, nil
	case "threads/list":
		limit := 20
		cursor := ""
		if v, ok := p["limit"]; ok {
			if json.Unmarshal(v, &limit) != nil || limit < 1 || limit > 100 {
				return nil, errors.New("limit must be an integer from 1 to 100")
			}
		}
		if v, ok := p["cursor"]; ok {
			if string(v) == "null" || json.Unmarshal(v, &cursor) != nil {
				return nil, errors.New("cursor must be a string")
			}
		}
		all := false
		if raw, ok := p["all_workspaces"]; ok {
			if string(raw) == "null" || json.Unmarshal(raw, &all) != nil {
				return nil, errors.New("all_workspaces must be boolean")
			}
		}
		params := map[string]any{"limit": limit, "modelProviders": []string{}, "sourceKinds": []string{"cli", "vscode", "appServer"}, "sortKey": "updated_at"}
		if !all {
			params["cwd"] = cwd
		}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := fetch("thread/list", params)
		if err != nil {
			return nil, err
		}
		var page struct {
			Data []threadListItem `json:"data"`
			Next *string          `json:"nextCursor"`
		}
		if json.Unmarshal(raw, &page) != nil || page.Data == nil {
			return nil, errors.New("invalid thread list response")
		}
		for _, thread := range page.Data {
			if !all && thread.Cwd != cwd {
				return nil, errors.New("thread outside workspace")
			}
		}
		for i := range page.Data {
			page.Data[i].Preview = safePreview(page.Data[i].Preview)
			page.Data[i].Name = safePreview(page.Data[i].Name)
		}
		return struct {
			Data []threadListItem `json:"data"`
			Next *string          `json:"next_cursor"`
		}{page.Data, page.Next}, nil
	case "turn/interrupt":
		var id, turn string
		for key, dest := range map[string]*string{"thread_id": &id, "turn_id": &turn} {
			if json.Unmarshal(p[key], dest) != nil || strings.TrimSpace(*dest) == "" || len(*dest) > 256 || strings.ContainsAny(*dest, "\r\n\x00") {
				return nil, errors.New("thread_id and turn_id must be nonempty single-line strings")
			}
		}
		raw, err := fetch("thread/read", map[string]any{"threadId": id, "includeTurns": false})
		if err != nil {
			return nil, err
		}
		var metadata struct {
			Thread Thread `json:"thread"`
		}
		if json.Unmarshal(raw, &metadata) != nil || metadata.Thread.ID != id {
			return nil, errors.New("invalid thread response")
		}
		if metadata.Thread.Cwd != cwd {
			return nil, errors.New("thread outside workspace")
		}
		return fetch("turn/interrupt", map[string]any{"threadId": id, "turnId": turn})
	case "turn/start":
		var id, text string
		if json.Unmarshal(p["thread_id"], &id) != nil || id == "" {
			return nil, errors.New("thread_id is required")
		}
		if string(p["text"]) == "null" || json.Unmarshal(p["text"], &text) != nil {
			return nil, errors.New("text must be a string")
		}
		input, err := turnInput(text, p["attachments"])
		if err != nil {
			return nil, err
		}
		var model string
		if raw, ok := p["model"]; ok {
			if json.Unmarshal(raw, &model) != nil || strings.TrimSpace(model) == "" || len(model) > 256 || strings.ContainsAny(model, "\r\n\x00") {
				return nil, errors.New("model must be a nonempty single-line string up to 256 bytes")
			}
		}
		var effort string
		if raw, ok := p["effort"]; ok {
			if json.Unmarshal(raw, &effort) != nil || strings.TrimSpace(effort) == "" || len(effort) > 64 || strings.ContainsAny(effort, "\r\n\x00") {
				return nil, errors.New("invalid reasoning effort")
			}
		}
		raw, err := fetch("thread/read", map[string]any{"threadId": id, "includeTurns": false})
		if err != nil {
			return nil, err
		}
		var metadata struct {
			Thread Thread `json:"thread"`
		}
		if json.Unmarshal(raw, &metadata) != nil || metadata.Thread.ID != id {
			return nil, errors.New("invalid thread response")
		}
		if metadata.Thread.Cwd != cwd {
			return nil, errors.New("thread outside workspace")
		}
		params := map[string]any{"threadId": id, "input": input}
		if model != "" {
			params["model"] = model
		}
		if effort != "" {
			params["effort"] = effort
		}
		return fetch("turn/start", params)
	case "decision/respond":
		var id string
		if json.Unmarshal(p["decision_id"], &id) != nil || id == "" {
			return nil, errors.New("decision_id is required")
		}
		if len(p["result"]) == 0 || string(p["result"]) == "null" {
			return nil, errors.New("result is required")
		}
		return fetch("decision/respond", map[string]any{"decisionId": id, "result": json.RawMessage(p["result"])})
	default:
		var id string
		if json.Unmarshal(p["thread_id"], &id) != nil || id == "" {
			return nil, errors.New("thread_id is required")
		}

		include := false
		if raw, ok := p["include_turns"]; ok {
			if string(raw) == "null" || json.Unmarshal(raw, &include) != nil {
				return nil, errors.New("include_turns must be boolean")
			}
		}
		if include {
			raw, err := fetch("thread/read", map[string]any{"threadId": id, "includeTurns": true})
			if err != nil {
				return nil, err
			}
			var response struct {
				Thread struct {
					Thread
					Turns []struct {
						turnMetadata
						Items []struct {
							ID       string          `json:"id"`
							Command  string          `json:"command"`
							Cwd      string          `json:"cwd"`
							Status   string          `json:"status"`
							Output   string          `json:"aggregatedOutput"`
							ExitCode *int            `json:"exitCode"`
							Type     string          `json:"type"`
							Text     string          `json:"text"`
							Summary  []string        `json:"summary"`
							Content  json.RawMessage `json:"content"`
						} `json:"items"`
					} `json:"turns"`
				} `json:"thread"`
			}
			if json.Unmarshal(raw, &response) != nil || response.Thread.ID != id {
				return nil, errors.New("invalid thread response")
			}
			if response.Thread.Cwd != cwd {
				return nil, errors.New("thread outside workspace")
			}
			messages := []map[string]any{}
			active := false
			outputBudget := 1024 * 1024
			for _, turn := range response.Thread.Turns {
				first := len(messages)
				if turn.Status == "inProgress" {
					active = true
				}
				for _, item := range turn.Items {
					if item.Type == "commandExecution" {
						output := item.Output
						limit := min(65536, outputBudget)
						if len(output) > limit {
							output = strings.ToValidUTF8(output[:limit], "") + "\n[Output truncated; view full log on host]"
						}
						outputBudget -= min(len(item.Output), limit)
						messages = append(messages, map[string]any{"id": item.ID, "role": "command", "text": output, "command": item.Command, "cwd": item.Cwd, "status": item.Status, "exit_code": item.ExitCode})
						continue
					}
					role, text, displayText := "", "", ""
					attachments := []map[string]string{}
					switch item.Type {
					case "userMessage":
						role = "user"
						var content []struct {
							Type string `json:"type"`
							Text string `json:"text"`
							Path string `json:"path"`
						}
						if json.Unmarshal(item.Content, &content) != nil {
							return nil, errors.New("invalid user message content")
						}
						for _, part := range content {
							if part.Type == "text" {
								if index := strings.LastIndex(part.Text, "Attached files selected by the user (JSON paths):\n"); index >= 0 && len(historyFileReferences(part.Text[index:])) > 0 {
									text += part.Text[:index]
									displayText += part.Text[:index]
									files := historyFileReferences(part.Text[index:])
									for _, file := range files {
										attachments = append(attachments, file)
										text += "\n[File: " + file["name"] + "]"
									}
								} else {
									text += part.Text
									displayText += part.Text
								}
							} else if part.Type == "localImage" {
								text += "\n[Image: " + filepath.Base(part.Path) + "]"
								if filepath.IsAbs(part.Path) {
									attachments = append(attachments, map[string]string{"path": part.Path, "name": filepath.Base(part.Path), "kind": "image"})
								} else {
									displayText += "\n[Image: " + filepath.Base(part.Path) + "]"
								}
							} else if part.Type == "image" {
								text += "\n[Image]"
								displayText += "\n[Image]"
							}
						}
					case "reasoning":
						role = "reasoning"
						text = strings.Join(item.Summary, "\n\n")
						if len(text) > 65536 {
							text = strings.ToValidUTF8(text[:65536], "") + "\n[Summary truncated]"
						}
						displayText = text
					case "agentMessage":
						role = "assistant"
						text = item.Text
						displayText = item.Text
					}
					if role != "" && text != "" {
						messages = append(messages, map[string]any{"id": item.ID, "role": role, "text": text, "display_text": displayText, "attachments": attachments})
					}
				}
				for _, message := range messages[first:] {
					turn.project(message)
				}
			}
			activeID := ""
			for _, turn := range response.Thread.Turns {
				if turn.Status == "inProgress" {
					activeID = turn.ID
				}
			}
			return map[string]any{"thread": response.Thread.Thread, "messages": messages, "active_turn": active, "active_turn_id": activeID}, nil
		}
		read := func(method string, params any) (Thread, error) {
			raw, err := fetch(method, params)
			if err != nil {
				return Thread{}, err
			}
			var r struct {
				Thread Thread `json:"thread"`
			}
			if json.Unmarshal(raw, &r) != nil || r.Thread.ID != id {
				return Thread{}, errors.New("invalid thread response")
			}
			if r.Thread.Cwd != cwd {
				return Thread{}, errors.New("thread outside workspace")
			}
			return r.Thread, nil
		}
		thread, err := read("thread/read", map[string]any{"threadId": id, "includeTurns": false})
		if err != nil {
			return nil, err
		}
		if cmd.Method == "threads/takeover" {
			if _, err := fetch("client/takeover", map[string]any{"threadId": id}); err != nil {
				return nil, err
			}
		}
		if cmd.Method == "threads/resume" || cmd.Method == "threads/takeover" {
			params := map[string]any{"threadId": id, "excludeTurns": true}
			if _, explicit := p["workspace"]; explicit {
				params["cwd"] = cwd
			}
			raw, err := fetch("thread/resume", params)
			if err != nil {
				return nil, err
			}
			result, err := projectSession(raw)
			if err != nil || result.Thread.ID != id {
				return nil, errors.New("invalid thread response")
			}
			if result.Thread.Cwd != cwd {
				return nil, errors.New("thread outside workspace")
			}
			return result, nil
		}
		return map[string]any{"thread": thread}, nil
	}
}

func Serve(ctx context.Context, in io.Reader, out io.Writer, cwd string, call Caller) error {
	return ServeEvents(ctx, in, out, cwd, func(ctx context.Context, method string, params any, _ func(string, json.RawMessage)) (json.RawMessage, error) {
		return call(ctx, method, params)
	})
}

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
		if response, err := EncodeEvent(method, params); err == nil {
			write(response)
		}
	}
	respond := func(cmd Command, result any, err error) {
		var response []byte
		if err == nil {
			response, err = EncodeResult(cmd.ID, result)
		}
		if err != nil {
			response, _ = EncodeError(cmd.ID, err.Error())
		}
		write(response)
	}
	handle := func(cmd Command) {
		result, err := HandleEvents(ctx, cwd, cmd, call, emit)
		respond(cmd, result, err)
	}
	// Keep ordinary commands ordered while accepting decisions during a pending RPC.
	commands := make(chan Command, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for cmd := range commands {
			handle(cmd)
		}
	}()
	for scanner.Scan() {
		cmd, err := DecodeCommand(scanner.Bytes())
		if err != nil {
			respond(cmd, nil, err)
			continue
		}
		if cmd.Method == "decision/respond" {
			handle(cmd)
			continue
		}
		select {
		case commands <- cmd:
		default:
			respond(cmd, nil, errors.New("command queue full; request not dispatched"))
		}
	}
	close(commands)
	<-done
	if err := scanner.Err(); err != nil {
		return err
	}
	outputMu.Lock()
	defer outputMu.Unlock()
	return outputErr
}
