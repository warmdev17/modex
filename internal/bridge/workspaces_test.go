package bridge

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAllWorkspaceList(t *testing.T) {
	_, err := Handle(context.Background(), "/work", Command{Method: "threads/list", Params: json.RawMessage(`{"all_workspaces":true}`)}, func(_ context.Context, m string, v any) (json.RawMessage, error) {
		p := v.(map[string]any)
		if _, ok := p["cwd"]; ok {
			t.Fatal("still filtered by cwd")
		}
		return json.RawMessage(`{"data":[{"id":"other","cwd":"/other"}],"nextCursor":null}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestExplicitWorkspace(t *testing.T) {
	for _, method := range []string{"threads/read", "threads/resume", "turn/start"} {
		calls := 0
		_, err := Handle(context.Background(), "/work", Command{Method: method, Params: json.RawMessage(`{"workspace":"/other","thread_id":"one"` + map[bool]string{true: `,"text":"hello"`, false: ""}[method == "turn/start"] + `}`)}, func(_ context.Context, m string, v any) (json.RawMessage, error) {
			calls++
			return json.RawMessage(`{"thread":{"id":"one","cwd":"/other"}}`), nil
		})
		if err != nil || calls == 0 {
			t.Fatalf("%s %v", method, err)
		}
	}
	for _, path := range []string{`"relative"`, `null`, `"/bad\npath"`} {
		_, err := Handle(context.Background(), "/work", Command{Method: "threads/read", Params: json.RawMessage(`{"workspace":` + path + `,"thread_id":"one"}`)}, func(context.Context, string, any) (json.RawMessage, error) {
			t.Fatal("called upstream")
			return nil, nil
		})
		if err == nil {
			t.Fatal(path)
		}
	}
}

func TestExplicitWorkspaceStillRejectsMismatch(t *testing.T) {
	for _, method := range []string{"threads/read", "threads/resume", "turn/start"} {
		_, err := Handle(context.Background(), "/work", Command{Method: method, Params: json.RawMessage(`{"workspace":"/other","thread_id":"one"` + map[bool]string{true: `,"text":"hello"`, false: ``}[method == "turn/start"] + `}`)}, func(context.Context, string, any) (json.RawMessage, error) {
			return json.RawMessage(`{"thread":{"id":"one","cwd":"/wrong"}}`), nil
		})
		if err == nil {
			t.Fatalf("%s accepted mismatched project", method)
		}
	}
}
