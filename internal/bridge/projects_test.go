package bridge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDesktopProjectsProjection(t *testing.T) {
	if _, err := DecodeCommand([]byte(`{"id":1,"method":"projects/list"}`)); err != nil {
		t.Fatal(err)
	}
	r, err := Handle(context.Background(), "/work", Command{Method: "projects/list", Params: json.RawMessage(`{"cursor":"next","limit":10}`)}, func(_ context.Context, m string, p any) (json.RawMessage, error) {
		if m != "project/list" || p.(map[string]any)["cursor"] != "next" {
			t.Fatal(m, p)
		}
		return json.RawMessage(`{"data":[{"id":"p","name":"Real project","roots":[{"path":"/work"}],"metadata":{"secret":"private"}}],"nextCursor":"after"}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "private") || !strings.Contains(string(b), `"next_cursor":"after"`) {
		t.Fatal(string(b))
	}
}
func TestThreadListPreservesProjectMembership(t *testing.T) {
	r, err := Handle(context.Background(), "/work", Command{Method: "threads/list"}, func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[{"id":"a","cwd":"/work","projectId":"p"},{"id":"b","cwd":"/work","projectId":null}],"nextCursor":null}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r)
	if !strings.Contains(string(b), `"projectId":"p"`) || !strings.Contains(string(b), `"projectId":null`) {
		t.Fatal(string(b))
	}
}
