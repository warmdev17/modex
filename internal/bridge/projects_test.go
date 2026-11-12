package bridge

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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

func TestLegacyDesktopMembershipOnlyExplicitAssignments(t *testing.T) {
	raw := json.RawMessage(`{"data":[{"id":"member","projectId":null},{"id":"standalone","projectId":null},{"id":"canonical","projectId":"keep"}]}`)
	state := []byte(`{"thread-project-assignments":{"member":{"projectKind":"local","projectId":"old"},"standalone":{"projectKind":"local","projectId":"old"},"canonical":{"projectKind":"local","projectId":"old"}},"projectless-thread-ids":["standalone"],"app-server-project-id-by-legacy-project-id-by-host":{"local:/custom":{"old":"new"}}}`)
	b := desktopMembership(raw, state, "/custom")
	var r struct {
		Data []struct {
			ID      string  `json:"id"`
			Project *string `json:"projectId"`
		} `json:"data"`
	}
	_ = json.Unmarshal(b, &r)
	if len(r.Data) != 3 || r.Data[0].Project == nil || *r.Data[0].Project != "new" || r.Data[1].Project != nil || r.Data[2].Project == nil || *r.Data[2].Project != "keep" {
		t.Fatal(string(b))
	}
}

func TestDesktopMembershipUsesChildHomeAndPreservesUnknownFields(t *testing.T) {
	home := t.TempDir()
	state, _ := json.Marshal(map[string]any{
		"thread-project-assignments":                         map[string]any{"member": map[string]string{"projectKind": "local", "projectId": "old"}},
		"app-server-project-id-by-legacy-project-id-by-host": map[string]any{"local:" + home: map[string]string{"old": "new"}},
	})
	if err := os.WriteFile(filepath.Join(home, ".codex-global-state.json"), state, 0600); err != nil {
		t.Fatal(err)
	}
	p := &Process{cmd: &exec.Cmd{Env: []string{"CODEX_HOME=" + home}}}
	raw := json.RawMessage(`{"data":[{"id":"member","extra":42}],"nextCursor":"next"}`)
	got := p.desktopMembership(raw)
	if !strings.Contains(string(got), `"projectId":"new"`) || !strings.Contains(string(got), `"extra":42`) || !strings.Contains(string(got), `"nextCursor":"next"`) {
		t.Fatal(string(got))
	}
	for _, bad := range []string{`invalid`, `{}`, `{"thread-project-assignments":[]}`} {
		if got := desktopMembership(raw, []byte(bad), home); string(got) != string(raw) {
			t.Fatal(string(got))
		}
	}
}

func TestExplicitProjectlessMarkerSurvivesProjection(t *testing.T) {
	raw := json.RawMessage(`{"data":[{"id":"standalone","cwd":"/work"},{"id":"native","cwd":"/work"}],"nextCursor":"next"}`)
	enriched := desktopMembership(raw, []byte(`{"projectless-thread-ids":["standalone"]}`), "/custom")
	r, err := Handle(context.Background(), "/work", Command{Method: "threads/list"}, func(context.Context, string, any) (json.RawMessage, error) { return enriched, nil })
	b, _ := json.Marshal(r)
	var page struct {
		Data []struct {
			ID          string `json:"id"`
			Projectless bool   `json:"projectless"`
		} `json:"data"`
	}
	if err != nil || json.Unmarshal(b, &page) != nil || len(page.Data) != 2 || !page.Data[0].Projectless || page.Data[1].Projectless {
		t.Fatalf("projectless distinction lost: %s %v", b, err)
	}
}
