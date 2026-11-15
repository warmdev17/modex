package bridge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateRemoteProject(t *testing.T) {
	root := t.TempDir()
	params, _ := json.Marshal(map[string]string{"workspace": root, "name": "Mobile project", "idempotency_key": "request-1"})
	cmd, err := DecodeCommand(append(append([]byte(`{"id":1,"method":"projects/create","params":`), params...), '}'))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Handle(context.Background(), "/default", cmd, func(_ context.Context, method string, value any) (json.RawMessage, error) {
		p := value.(map[string]any)
		roots := p["roots"].([]map[string]string)
		if method != "project/create" || p["name"] != "Mobile project" || p["idempotencyKey"] != "request-1" || len(roots) != 1 || roots[0]["path"] != root {
			t.Fatal(method, p)
		}
		return json.Marshal(map[string]any{"project": map[string]any{"id": "p1", "name": "Mobile project", "roots": roots, "metadata": map[string]string{"secret": "hidden"}}})
	})
	raw, _ := json.Marshal(result)
	if err != nil || !strings.Contains(string(raw), `"id":"p1"`) || strings.Contains(string(raw), "hidden") {
		t.Fatal(string(raw), err)
	}
}

func TestCreateRemoteProjectRejectsInvalidInput(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	cases := []map[string]string{
		{"name": "ok", "idempotency_key": "k"},
		{"workspace": file, "name": "ok", "idempotency_key": "k"},
		{"workspace": filepath.Join(root, "absent"), "name": "ok", "idempotency_key": "k"},
		{"workspace": root, "name": "  ", "idempotency_key": "k"},
		{"workspace": root, "name": "bad\nname", "idempotency_key": "k"},
		{"workspace": root, "name": "ok"},
	}
	for _, params := range cases {
		b, _ := json.Marshal(params)
		_, err := Handle(context.Background(), "/default", Command{Method: "projects/create", Params: b}, func(context.Context, string, any) (json.RawMessage, error) {
			t.Fatal("invalid project reached Codex")
			return nil, nil
		})
		if err == nil {
			t.Fatal(params)
		}
	}
	if data, _ := os.ReadFile(file); string(data) != "keep" {
		t.Fatal("file modified")
	}
}

func TestStartThreadInSelectedProject(t *testing.T) {
	_, err := Handle(context.Background(), "/default", Command{Method: "threads/start", Params: json.RawMessage(`{"workspace":"/other","project_id":"p1"}`)}, func(_ context.Context, method string, value any) (json.RawMessage, error) {
		p := value.(map[string]any)
		if method != "thread/start" || p["cwd"] != "/other" || p["projectId"] != "p1" {
			t.Fatal(method, p)
		}
		return json.RawMessage(`{"thread":{"id":"t1","cwd":"/other"}}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStartThreadRejectsInvalidProjectIdentity(t *testing.T) {
	for _, value := range []string{`null`, `42`, `""`, `"bad\nproject"`, `[]`} {
		_, err := Handle(context.Background(), "/default", Command{Method: "threads/start", Params: json.RawMessage(`{"project_id":` + value + `}`)}, func(context.Context, string, any) (json.RawMessage, error) {
			t.Fatal("invalid project reached Codex")
			return nil, nil
		})
		if err == nil {
			t.Fatal(value)
		}
	}
}
