package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/warmdev/modex/internal/antigravity"
	"github.com/warmdev/modex/internal/bridge"
)

type agentChatRouter interface {
	Handle(context.Context, bridge.Command, func(string, json.RawMessage)) (any, bool, error)
	Threads() []map[string]any
}

func serveAgentChats(ctx context.Context, in io.Reader, out io.Writer, cwd, agy string, codex bridge.EventCaller) error {
	stateDir := os.Getenv("MODEX_AGENT_STATE_DIR")
	if stateDir == "" {
		return bridge.ServeEvents(ctx, in, out, cwd, codex)
	}
	if err := os.Mkdir(stateDir, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	if err := checkPrivatePath(stateDir, true); err != nil {
		return err
	}
	router := antigravity.New(ctx, agy, cwd, stateDir)
	defer router.Close()
	return bridge.ServeCommands(ctx, in, out, agentCommands(cwd, codex, router))
}

func agentCommands(cwd string, codex bridge.EventCaller, router agentChatRouter) bridge.CommandHandler {
	return func(ctx context.Context, cmd bridge.Command, emit func(string, json.RawMessage)) (any, error) {
		explicitCodexStart := false
		if cmd.Method == "threads/start" {
			var p map[string]json.RawMessage
			if len(cmd.Params) > 0 && json.Unmarshal(cmd.Params, &p) != nil {
				return nil, errors.New("params must be an object")
			}
			var agent string
			if raw, ok := p["agent"]; ok {
				if json.Unmarshal(raw, &agent) != nil {
					return nil, errors.New("invalid agent")
				}
				if agent == "codex" {
					explicitCodexStart = true
					delete(p, "agent")
					cmd.Params, _ = json.Marshal(p)
				}
			}
			if agent == "antigravity" || agent == "opencode" {
				if err := validateAgentProject(ctx, cwd, p, codex, emit); err != nil {
					return nil, err
				}
			}
		}
		if cmd.Method == "models/list" {
			var p map[string]json.RawMessage
			if len(cmd.Params) > 0 && json.Unmarshal(cmd.Params, &p) != nil {
				return nil, errors.New("params must be an object")
			}
			if raw, ok := p["agent"]; ok {
				var agent string
				if json.Unmarshal(raw, &agent) != nil || agent != "codex" {
					return nil, errors.New("unknown agent")
				}
				if raw, ok := p["workspace"]; ok {
					var workspace string
					if json.Unmarshal(raw, &workspace) != nil || !filepath.IsAbs(workspace) || strings.ContainsAny(workspace, "\r\n\x00") || len(workspace) > 32768 {
						return nil, errors.New("invalid workspace")
					}
					info, err := os.Stat(workspace)
					if err != nil || !info.IsDir() {
						return nil, errors.New("workspace is unavailable")
					}
					delete(p, "workspace")
				}
				delete(p, "agent")
				cmd.Params, _ = json.Marshal(p)
				return bridge.HandleEvents(ctx, cwd, cmd, codex, emit)
			}
		}
		if cmd.Method == "threads/list" {
			var p map[string]json.RawMessage
			if len(cmd.Params) > 0 && json.Unmarshal(cmd.Params, &p) != nil {
				return nil, errors.New("params must be an object")
			}
			if raw, ok := p["agent"]; ok {
				var agent string
				if json.Unmarshal(raw, &agent) != nil {
					return nil, errors.New("invalid agent")
				}
				if agent == "antigravity" {
					return agentChatPage(p, cwd, router)
				}
				if agent != "codex" {
					return nil, errors.New("unknown agent")
				}
				delete(p, "agent")
				cmd.Params, _ = json.Marshal(p)
				value, err := bridge.HandleEvents(ctx, cwd, cmd, codex, emit)
				if err != nil {
					return nil, err
				}
				var page struct {
					Data []map[string]any `json:"data"`
					Next any              `json:"next_cursor"`
				}
				if err := json.Unmarshal(mustJSON(value), &page); err != nil {
					return nil, err
				}
				for _, thread := range page.Data {
					thread["agent"] = "codex"
				}
				return page, nil
			}
		}
		// Codex resets decisions on attach, so replay AGY decisions after that reset.
		if cmd.Method != "client/attached" {
			if value, handled, err := router.Handle(ctx, cmd, emit); handled || err != nil {
				return value, err
			}
		}
		value, err := bridge.HandleEvents(ctx, cwd, cmd, codex, emit)
		if err != nil {
			return nil, err
		}
		if explicitCodexStart {
			var result map[string]any
			if json.Unmarshal(mustJSON(value), &result) != nil {
				return nil, errors.New("invalid thread response")
			}
			thread, ok := result["thread"].(map[string]any)
			if !ok {
				return nil, errors.New("invalid thread response")
			}
			thread["agent"] = "codex"
			return result, nil
		}
		switch cmd.Method {
		case "client/attached":
			_, _, err := router.Handle(ctx, cmd, emit)
			return value, err
		case "threads/list":
			return appendAgentChats(value, cmd.Params, cwd, router)
		default:
			return value, nil
		}
	}
}

func appendAgentChats(value any, params json.RawMessage, cwd string, router agentChatRouter) (any, error) {
	var options struct {
		Cursor   string `json:"cursor"`
		All      bool   `json:"all_workspaces"`
		Archived bool   `json:"archived"`
	}
	if len(params) > 0 && json.Unmarshal(params, &options) != nil {
		return value, nil
	}
	if options.Cursor != "" || options.Archived {
		return value, nil
	}
	threads := router.Threads()
	if len(threads) == 0 {
		return value, nil
	}
	var page struct {
		Data []json.RawMessage `json:"data"`
		Next *string           `json:"next_cursor"`
	}
	if err := json.Unmarshal(mustJSON(value), &page); err != nil {
		return nil, err
	}
	for _, thread := range threads {
		if options.All || thread["cwd"] == cwd {
			page.Data = append(page.Data, mustJSON(thread))
		}
	}
	return page, nil
}

func agentChatPage(p map[string]json.RawMessage, cwd string, router agentChatRouter) (any, error) {
	limit := 20
	cursor := ""
	all := false
	for key, raw := range p {
		switch key {
		case "agent":
		case "limit":
			if json.Unmarshal(raw, &limit) != nil || limit < 1 || limit > 100 {
				return nil, errors.New("limit must be 1..100")
			}
		case "cursor":
			if json.Unmarshal(raw, &cursor) != nil || cursor == "" || len(cursor) > 4096 || strings.ContainsAny(cursor, "\r\n\x00") {
				return nil, errors.New("invalid cursor")
			}
		case "all_workspaces":
			if string(raw) == "null" || json.Unmarshal(raw, &all) != nil {
				return nil, errors.New("all_workspaces must be boolean")
			}
		case "archived":
			var archived bool
			if string(raw) == "null" || json.Unmarshal(raw, &archived) != nil || archived {
				return nil, errors.New("archived is unsupported")
			}
		default:
			return nil, errors.New("unknown parameter")
		}
	}
	threads := router.Threads()
	sort.Slice(threads, func(i, j int) bool {
		a, _ := threads[i]["id"].(string)
		b, _ := threads[j]["id"].(string)
		return a < b
	})
	data := make([]map[string]any, 0, limit)
	var next any
	for _, thread := range threads {
		id, _ := thread["id"].(string)
		if id <= cursor || (!all && thread["cwd"] != cwd) {
			continue
		}
		if len(data) == limit {
			next = data[len(data)-1]["id"]
			break
		}
		data = append(data, thread)
	}
	return map[string]any{"data": data, "next_cursor": next}, nil
}

func validateAgentProject(ctx context.Context, cwd string, p map[string]json.RawMessage, codex bridge.EventCaller, emit func(string, json.RawMessage)) error {
	raw, ok := p["project_id"]
	if !ok {
		return nil
	}
	var project, workspace string
	if json.Unmarshal(raw, &project) != nil || project == "" || len(project) > 256 || strings.ContainsAny(project, "\r\n\x00") {
		return errors.New("invalid project_id")
	}
	if json.Unmarshal(p["workspace"], &workspace) != nil || !filepath.IsAbs(workspace) {
		return errors.New("project_id requires an explicit project workspace")
	}
	selected, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return errors.New("workspace is unavailable")
	}
	cursor := ""
	for i := 0; i < 100; i++ {
		params := map[string]any{"limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		encoded, _ := json.Marshal(params)
		value, err := bridge.HandleEvents(ctx, cwd, bridge.Command{Method: "projects/list", Params: encoded}, codex, emit)
		if err != nil {
			return err
		}
		var page struct {
			Data []struct {
				ID    string `json:"id"`
				Roots []struct {
					Path string `json:"path"`
				} `json:"roots"`
			} `json:"data"`
			Next *string `json:"next_cursor"`
		}
		if json.Unmarshal(mustJSON(value), &page) != nil || page.Data == nil {
			return errors.New("invalid project list response")
		}
		for _, entry := range page.Data {
			if entry.ID == project {
				for _, root := range entry.Roots {
					p, err := filepath.EvalSymlinks(root.Path)
					if err == nil && p == selected {
						return nil
					}
				}
			}
		}
		if page.Next == nil || *page.Next == "" {
			break
		}
		if *page.Next == cursor {
			return errors.New("invalid project cursor")
		}
		cursor = *page.Next
	}
	return errors.New("workspace does not belong to selected project")
}
