package bridge

import (
	"context"
	"encoding/json"
	"testing"
)

func TestThreadPermissionModes(t *testing.T) {
	for _, tc := range []struct{ mode, sandbox, approval string }{
		{"host-default", "", ""}, {"read-only", "read-only", "on-request"},
		{"workspace-write", "workspace-write", "on-request"}, {"full-access", "danger-full-access", "never"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			call := func(_ context.Context, method string, value any) (json.RawMessage, error) {
				p := value.(map[string]any)
				if method != "thread/start" || p["cwd"] != "/work" {
					t.Fatalf("unexpected request %s %v", method, p)
				}
				if tc.sandbox == "" {
					if len(p) != 1 {
						t.Fatalf("default changed: %v", p)
					}
				} else {
					if p["sandbox"] != tc.sandbox || p["approvalPolicy"] != tc.approval || p["approvalsReviewer"] != "user" {
						t.Fatalf("permissions %v", p)
					}
				}
				return json.RawMessage(`{"thread":{"id":"one","cwd":"/work","modelProvider":"custom"},"model":"test","approvalPolicy":"on-request","sandbox":{"type":"readOnly","extra":"private"},"secret":"private"}`), nil
			}
			result, err := Handle(context.Background(), "/work", Command{Method: "threads/start", Params: json.RawMessage(`{"permission_mode":"` + tc.mode + `"}`)}, call)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(result)
			var projection map[string]any
			_ = json.Unmarshal(raw, &projection)
			if projection["approval_policy"] != "on-request" || projection["sandbox"] != "readOnly" {
				t.Fatalf("effective permissions missing: %s", raw)
			}
			if _, ok := projection["secret"]; ok {
				t.Fatal("secret exposed")
			}
		})
	}
}

func TestInvalidThreadPermissionModeNeverCallsUpstream(t *testing.T) {
	for _, value := range []string{`null`, `42`, `""`, `"danger-full-access"`, `{}`, `"unknown"`} {
		_, err := Handle(context.Background(), "/work", Command{Method: "threads/start", Params: json.RawMessage(`{"permission_mode":` + value + `}`)}, func(context.Context, string, any) (json.RawMessage, error) {
			t.Fatal("upstream called")
			return nil, nil
		})
		if err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}

func TestResumeProjectsPermissionsWithoutOverridingThem(t *testing.T) {
	calls := 0
	result, err := Handle(context.Background(), "/work", Command{Method: "threads/resume", Params: json.RawMessage(`{"thread_id":"one"}`)}, func(_ context.Context, method string, params any) (json.RawMessage, error) {
		calls++
		p := params.(map[string]any)
		if len(p) != 2 || p["threadId"] != "one" {
			t.Fatalf("unexpected overrides: %v", p)
		}
		if calls == 1 && method != "thread/read" || calls == 2 && method != "thread/resume" {
			t.Fatal(method)
		}
		return json.RawMessage(`{"thread":{"id":"one","cwd":"/work"},"model":"m","approvalPolicy":"never","approvalsReviewer":"user","sandbox":{"type":"dangerFullAccess"}}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(result)
	var fields map[string]any
	_ = json.Unmarshal(b, &fields)
	if calls != 2 || fields["sandbox"] != "dangerFullAccess" || fields["approvals_reviewer"] != "user" {
		t.Fatalf("result %s calls %d", b, calls)
	}
}

func TestProjectSessionReasoning(t *testing.T) {
	for _, tc := range []struct{ raw, effort string }{
		{`{"thread":{"id":"one","reasoningEffort":"low"},"model":"custom","reasoningEffort":"high"}`, "high"},
		{`{"thread":{"id":"one","reasoningEffort":"low"},"model":"custom"}`, "low"},
		{`{"thread":{"id":"one"},"model":"custom"}`, ""},
	} {
		result, err := projectSession(json.RawMessage(tc.raw))
		if err != nil || result.Model != "custom" || result.Effort != tc.effort {
			t.Fatalf("unexpected session settings: %+v, %v", result, err)
		}
	}
}
