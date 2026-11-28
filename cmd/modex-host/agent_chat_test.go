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

func TestExplicitAntigravityListPaginatesRegisteredChats(t *testing.T) {
	router := &fixtureAgentRouter{listed: []map[string]any{
		{"id": "agy:c", "cwd": "/work", "agent": "antigravity"},
		{"id": "agy:a", "cwd": "/work", "agent": "antigravity"},
		{"id": "agy:b", "cwd": "/work", "agent": "antigravity"},
	}}
	codex := func(context.Context, string, any, func(string, json.RawMessage)) (json.RawMessage, error) {
		t.Fatal("explicit Antigravity list called Codex")
		return nil, nil
	}
	run := agentCommands("/work", codex, router)
	first, err := run(context.Background(), bridge.Command{Method: "threads/list", Params: json.RawMessage(`{"agent":"antigravity","limit":2}`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Next *string `json:"next_cursor"`
	}
	if err := json.Unmarshal(mustJSON(first), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 2 || page.Data[0].ID != "agy:a" || page.Data[1].ID != "agy:b" || page.Next == nil {
		t.Fatalf("first page %s", mustJSON(first))
	}
	params, _ := json.Marshal(map[string]any{"agent": "antigravity", "limit": 2, "cursor": *page.Next})
	second, err := run(context.Background(), bridge.Command{Method: "threads/list", Params: params}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mustJSON(second), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != "agy:c" || page.Next != nil {
		t.Fatalf("second page %s", mustJSON(second))
	}
}

func TestExplicitCodexListDoesNotIncludeAntigravity(t *testing.T) {
	router := &fixtureAgentRouter{listed: []map[string]any{{"id": "agy:a", "cwd": "/work"}}}
	codex := func(context.Context, string, any, func(string, json.RawMessage)) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[{"id":"codex-1","cwd":"/work"}],"nextCursor":null}`), nil
	}
	result, err := agentCommands("/work", codex, router)(context.Background(), bridge.Command{Method: "threads/list", Params: json.RawMessage(`{"agent":"codex"}`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(mustJSON(result), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || page.Data[0]["id"] != "codex-1" || page.Data[0]["agent"] != "codex" {
		t.Fatalf("page: %s", mustJSON(result))
	}
}

type acceptingAgentRouter struct{ fixtureAgentRouter }

func (*acceptingAgentRouter) Handle(context.Context, bridge.Command, func(string, json.RawMessage)) (any, bool, error) {
	return map[string]any{"thread": map[string]any{"id": "agy:test"}}, true, nil
}

func TestAgentStartRejectsProjectOutsideSharedRegistry(t *testing.T) {
	router := &acceptingAgentRouter{}
	workspace := t.TempDir()
	other := t.TempDir()
	codex := func(_ context.Context, method string, _ any, _ func(string, json.RawMessage)) (json.RawMessage, error) {
		if method != "project/list" {
			t.Fatalf("unexpected method %s", method)
		}
		raw, _ := json.Marshal(map[string]any{"data": []any{map[string]any{"id": "shared", "roots": []any{map[string]any{"path": other}}}}})
		return raw, nil
	}
	params, _ := json.Marshal(map[string]any{"agent": "antigravity", "project_id": "shared", "workspace": workspace})
	_, err := agentCommands(workspace, codex, router)(context.Background(), bridge.Command{Method: "threads/start", Params: params}, nil)
	if err == nil {
		t.Fatal("accepted workspace outside selected project")
	}
}

func TestExplicitCodexStartLabelsThread(t *testing.T) {
	codex := func(_ context.Context, method string, _ any, _ func(string, json.RawMessage)) (json.RawMessage, error) {
		if method != "thread/start" {
			t.Fatalf("unexpected method %s", method)
		}
		return json.RawMessage(`{"thread":{"id":"codex-id","cwd":"/work"}}`), nil
	}
	result, err := agentCommands("/work", codex, &fixtureAgentRouter{})(context.Background(), bridge.Command{Method: "threads/start", Params: json.RawMessage(`{"agent":"codex"}`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Thread map[string]any `json:"thread"`
	}
	if json.Unmarshal(mustJSON(result), &value) != nil || value.Thread["agent"] != "codex" {
		t.Fatalf("result %s", mustJSON(result))
	}
}

func TestAgentStartAcceptsSelectedSharedProjectRoot(t *testing.T) {
	workspace := t.TempDir()
	codex := func(_ context.Context, method string, _ any, _ func(string, json.RawMessage)) (json.RawMessage, error) {
		if method != "project/list" {
			t.Fatalf("unexpected method %s", method)
		}
		raw, _ := json.Marshal(map[string]any{"data": []any{map[string]any{"id": "shared", "roots": []any{map[string]any{"path": workspace}}}}})
		return raw, nil
	}
	params, _ := json.Marshal(map[string]any{"agent": "antigravity", "project_id": "shared", "workspace": workspace})
	result, err := agentCommands(workspace, codex, &acceptingAgentRouter{})(context.Background(), bridge.Command{Method: "threads/start", Params: params}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(mustJSON(result)) != `{"thread":{"id":"agy:test"}}` {
		t.Fatalf("unexpected response %s", mustJSON(result))
	}
}

func TestExplicitCodexModelsUsesSelectedWorkspace(t *testing.T) {
	workspace := t.TempDir()
	codex := func(_ context.Context, method string, params any, _ func(string, json.RawMessage)) (json.RawMessage, error) {
		if method != "model/list" {
			t.Fatalf("unexpected method %s", method)
		}
		return json.RawMessage(`{"data":[],"nextCursor":null}`), nil
	}
	params, _ := json.Marshal(map[string]any{"agent": "codex", "workspace": workspace})
	_, err := agentCommands(workspace, codex, &fixtureAgentRouter{})(context.Background(), bridge.Command{Method: "models/list", Params: params}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAntigravityModelsReturnsEmptyCatalogueForValidWorkspace(t *testing.T) {
	workspace := t.TempDir()
	params, _ := json.Marshal(map[string]any{"agent": "antigravity", "workspace": workspace})
	codex := func(context.Context, string, any, func(string, json.RawMessage)) (json.RawMessage, error) {
		t.Fatal("called Codex for Antigravity models")
		return nil, nil
	}
	result, err := agentCommands(workspace, codex, &fixtureAgentRouter{})(context.Background(), bridge.Command{Method: "models/list", Params: params}, nil)
	if err != nil || string(mustJSON(result)) != `{"data":[],"next_cursor":null}` {
		t.Fatalf("result=%s err=%v", mustJSON(result), err)
	}
	params, _ = json.Marshal(map[string]any{"agent": "antigravity", "workspace": "relative"})
	if _, err = agentCommands(workspace, codex, &fixtureAgentRouter{})(context.Background(), bridge.Command{Method: "models/list", Params: params}, nil); err == nil {
		t.Fatal("accepted relative workspace")
	}
}

func TestGoalRejectsOtherAgentThreads(t *testing.T) {
	codex := func(context.Context, string, any, func(string, json.RawMessage)) (json.RawMessage, error) {
		t.Fatal("sent another agent goal to Codex")
		return nil, nil
	}
	run := agentCommands("/work", codex, &fixtureAgentRouter{})
	for _, id := range []string{"agy:one", "oc:one"} {
		for _, method := range []string{"threads/goal/get", "threads/goal/set", "threads/goal/clear"} {
			params, _ := json.Marshal(map[string]string{"thread_id": id})
			if _, err := run(context.Background(), bridge.Command{Method: method, Params: params}, nil); err == nil {
				t.Fatalf("accepted %s %s", method, id)
			}
		}
	}
}
