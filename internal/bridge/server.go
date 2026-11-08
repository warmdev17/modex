package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
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
	case "status":
	case "threads/start":
		allowed["model"] = true
	case "threads/list":
		allowed["limit"] = true
		allowed["cursor"] = true
	case "threads/read", "threads/resume":
		allowed["thread_id"] = true
	case "turn/start":
		allowed["thread_id"] = true
		allowed["text"] = true
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
	switch cmd.Method {
	case "threads/start":
		params := map[string]any{"cwd": cwd}
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
		var result struct {
			Thread Thread `json:"thread"`
			Model  string `json:"model"`
		}
		if json.Unmarshal(raw, &result) != nil || result.Thread.ID == "" {
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
		params := map[string]any{"cwd": cwd, "limit": limit, "modelProviders": []string{}}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := fetch("thread/list", params)
		if err != nil {
			return nil, err
		}
		var page struct {
			Data []Thread `json:"data"`
			Next *string  `json:"nextCursor"`
		}
		if json.Unmarshal(raw, &page) != nil || page.Data == nil {
			return nil, errors.New("invalid thread list response")
		}
		for _, thread := range page.Data {
			if thread.Cwd != cwd {
				return nil, errors.New("thread outside workspace")
			}
		}
		return struct {
			Data []Thread `json:"data"`
			Next *string  `json:"next_cursor"`
		}{page.Data, page.Next}, nil
	case "turn/start":
		var id, text string
		if json.Unmarshal(p["thread_id"], &id) != nil || id == "" {
			return nil, errors.New("thread_id is required")
		}
		if json.Unmarshal(p["text"], &text) != nil || strings.TrimSpace(text) == "" {
			return nil, errors.New("text is required")
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
		return fetch("turn/start", map[string]any{"threadId": id, "input": []map[string]string{{"type": "text", "text": text}}})
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
		if cmd.Method == "threads/resume" {
			thread, err = read("thread/resume", map[string]any{"threadId": id, "excludeTurns": true})
			if err != nil {
				return nil, err
			}
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
