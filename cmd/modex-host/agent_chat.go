package main

import (
	"context"
	"encoding/json"
	"io"
	"os"

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
