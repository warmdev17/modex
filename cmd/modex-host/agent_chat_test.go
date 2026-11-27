package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/warmdev/modex/internal/bridge"
)

func TestAgentChatListPreservesCodexPagination(t *testing.T) {
	router := &fixtureAgentRouter{listed: []map[string]any{
		{"id": "agy:in", "cwd": "/work", "agent": "antigravity"},
		{"id": "agy:out", "cwd": "/other", "agent": "antigravity"},
	}}
	codex := func(_ context.Context, method string, _ any, _ func(string, json.RawMessage)) (json.RawMessage, error) {
		if method != "thread/list" {
			t.Fatalf("unexpected native RPC %s", method)
		}
		return json.RawMessage(`{"data":[{"id":"codex","cwd":"/work"}],"nextCursor":"native-next"}`), nil
	}
	run := agentCommands("/work", codex, router)
	for _, tc := range []struct {
		params string
		count  int
	}{
		{`{}`, 2},
		{`{"all_workspaces": true}`, 3},
		{`{"cursor":"native-next"}`, 1},
		{`{"archived":true}`, 1},
	} {
		result, err := run(context.Background(), bridge.Command{Method: "threads/list", Params: json.RawMessage(tc.params)}, nil)
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Data []json.RawMessage
			Next string `json:"next_cursor"`
		}
		if err := json.Unmarshal(mustJSON(result), &page); err != nil || len(page.Data) != tc.count || page.Next != "native-next" {
			t.Fatalf("params=%s, result=%s, error=%v", tc.params, mustJSON(result), err)
		}
	}
}

type fixtureAgentRouter struct {
	listed []map[string]any
}

func (r *fixtureAgentRouter) Handle(context.Context, bridge.Command, func(string, json.RawMessage)) (any, bool, error) {
	return nil, false, nil
}

func (r *fixtureAgentRouter) Threads() []map[string]any { return r.listed }
