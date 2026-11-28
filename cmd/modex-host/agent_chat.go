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
	"github.com/warmdev/modex/internal/opencode"
	"github.com/warmdev/modex/internal/supervisor"
)

type agentChatRouter interface {
	Handle(context.Context, bridge.Command, func(string, json.RawMessage)) (any, bool, error)
	Threads() []map[string]any
}

func serveAgentChats(ctx context.Context, in io.Reader, out io.Writer, cwd, agy, opencodeBinary string, codex bridge.EventCaller) error {
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
	oc := opencode.NewRouter(ctx, opencodeBinary, cwd, stateDir)
	defer oc.Close()
	handler := agentCommandsWithOpenCode(cwd, codex, router, oc)
	handler = attachmentContext(router.SetSSHConnection, handler)
	return bridge.ServeCommands(ctx, in, out, func(ctx context.Context, cmd bridge.Command, emit func(string, json.RawMessage)) (any, error) {
		if cmd.ID == supervisor.AttachmentCommandID && cmd.Method == "client/attached" {
			var p struct {
				CLISSH bool `json:"cli_ssh"`
			}
			if json.Unmarshal(cmd.Params, &p) == nil && p.CLISSH {
				oc.ReleaseNativeCLIs()
				router.ReleaseNativeCLIs()
			}
		}
		return handler(ctx, cmd, emit)
	})
}

func attachmentContext(set func(string), next bridge.CommandHandler) bridge.CommandHandler {
	// The bridge serializes lifecycle commands before normal client requests.
	var phone, terminal string
	return func(ctx context.Context, cmd bridge.Command, emit func(string, json.RawMessage)) (any, error) {
		if cmd.ID == supervisor.AttachmentCommandID && cmd.Method == "client/attached" {
			var p struct {
				Attached      bool   `json:"attached"`
				SSHConnection string `json:"ssh_connection"`
				CLISSH        bool   `json:"cli_ssh"`
			}
			if json.Unmarshal(cmd.Params, &p) != nil {
				set("")
				return nil, errors.New("invalid attachment context")
			}
			if p.CLISSH {
				terminal = p.SSHConnection
			} else {
				phone = ""
				if p.Attached {
					phone = p.SSHConnection
				}
			}
			selected := phone
			if selected == "" {
				selected = terminal
			}
			set(selected)
			if p.CLISSH {
				return map[string]bool{"updated": true}, nil
			}
			cmd.Params, _ = json.Marshal(struct {
				Attached bool `json:"attached"`
			}{p.Attached})
		}
		return next(ctx, cmd, emit)
	}
}

func agentCommands(cwd string, codex bridge.EventCaller, router agentChatRouter) bridge.CommandHandler {
	return agentCommandsWithOpenCode(cwd, codex, router, nil)
}

func agentCommandsWithOpenCode(cwd string, codex bridge.EventCaller, router agentChatRouter, oc *opencode.Router) bridge.CommandHandler {
	return func(ctx context.Context, cmd bridge.Command, emit func(string, json.RawMessage)) (any, error) {
		if cmd.Method == "cli/native" || cmd.Method == "cli/native/end" {
			if !supervisor.CLIRequest(cmd.ID) {
				return nil, errors.New("native terminal access requires the local CLI")
			}
			var p struct {
				ThreadID  string `json:"thread_id"`
				Workspace string `json:"workspace"`
			}
			if json.Unmarshal(cmd.Params, &p) != nil || !validCLIWorkspace(p.Workspace) {
				return nil, errors.New("invalid native terminal request")
			}
			if strings.HasPrefix(p.ThreadID, "oc:") && oc != nil {
				if cmd.Method == "cli/native/end" {
					oc.ReleaseNativeCLI(p.ThreadID)
					return map[string]bool{"released": true}, nil
				}
				return oc.NativeCLI(ctx, p.Workspace, p.ThreadID)
			}
			if strings.HasPrefix(p.ThreadID, "agy:") {
				if cmd.Method == "cli/native/end" {
					if native, ok := router.(interface{ ReleaseNativeCLI(string, string) }); ok {
						native.ReleaseNativeCLI(p.Workspace, p.ThreadID)
					}
					return map[string]bool{"released": true}, nil
				}
				native, ok := router.(interface {
					NativeCLI(context.Context, string, string) (map[string]string, error)
				})
				if ok {
					return native.NativeCLI(ctx, p.Workspace, p.ThreadID)
				}
			}
			return nil, errors.New("native terminal runtime is unavailable")
		}
		if oc != nil && cmd.Method == "agents/list" {
			var p struct {
				Workspace string `json:"workspace"`
			}
			if len(cmd.Params) > 0 && json.Unmarshal(cmd.Params, &p) != nil {
				return nil, errors.New("invalid workspace")
			}
			value, _, err := router.Handle(ctx, cmd, emit)
			if err != nil {
				return nil, err
			}
			var result struct {
				Data []any `json:"data"`
			}
			if json.Unmarshal(mustJSON(value), &result) != nil {
				return nil, errors.New("invalid agents response")
			}
			result.Data = append(result.Data, oc.Readiness(ctx, p.Workspace))
			return result, nil
		}
		if strings.HasPrefix(cmd.Method, "threads/goal/") {
			var p struct {
				ThreadID string `json:"thread_id"`
			}
			if json.Unmarshal(cmd.Params, &p) == nil && (strings.HasPrefix(p.ThreadID, "agy:") || strings.HasPrefix(p.ThreadID, "oc:")) {
				return nil, errors.New("goals are unsupported by this agent")
			}
		}
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
				if json.Unmarshal(raw, &agent) != nil {
					return nil, errors.New("unknown agent")
				}
				if agent == "opencode" && oc != nil {
					value, _, err := oc.Handle(ctx, cmd, emit)
					return value, err
				}
				if agent != "codex" && agent != "antigravity" {
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
				if agent == "antigravity" {
					return bridge.HandleEvents(ctx, cwd, cmd, func(context.Context, string, any, func(string, json.RawMessage)) (json.RawMessage, error) {
						return json.RawMessage(`{"data":[]}`), nil
					}, emit)
				}
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
				if agent == "opencode" && oc != nil {
					return agentChatPage(p, cwd, oc)
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
			if oc != nil {
				if value, handled, err := oc.Handle(ctx, cmd, emit); handled || err != nil {
					return value, err
				}
			}
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
			if err == nil && oc != nil {
				_, _, err = oc.Handle(ctx, cmd, emit)
			}
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
