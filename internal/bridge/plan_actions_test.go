package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func planFixture(t *testing.T, fail bool) (Caller, *[]string) {
	t.Helper()
	calls := []string{}
	return func(_ context.Context, method string, value any) (json.RawMessage, error) {
		calls = append(calls, method)
		encoded, _ := json.Marshal(value)
		p := map[string]any{}
		_ = json.Unmarshal(encoded, &p)
		switch method {
		case "thread/read":
			return json.RawMessage(`{"thread":{"id":"source","cwd":"/work","status":{"type":"idle"}}}`), nil
		case "thread/goal/get":
			return json.RawMessage(`{"goal":null}`), nil
		case "thread/resume":
			return json.RawMessage(`{"thread":{"id":"source","cwd":"/work","modelProvider":"third-party","projectId":"project"},"model":"custom-model","modelProvider":"third-party","reasoningEffort":"high","approvalPolicy":"on-request","approvalsReviewer":"user","activePermissionProfile":{"id":":workspace"},"sandbox":{"type":"workspaceWrite","networkAccess":false,"writableRoots":[]}}`), nil
		case "thread/turns/list":
			return json.RawMessage(`{"data":[{"id":"turn","status":"completed","items":[{"type":"plan","id":"plan","text":"Build the feature."}]}],"nextCursor":null}`), nil
		case "thread/start":
			if p["cwd"] != "/work" || p["permissions"] != ":workspace" || p["modelProvider"] != "third-party" || p["projectId"] != "project" {
				t.Fatalf("fresh settings %v", p)
			}
			return json.RawMessage(`{"thread":{"id":"fresh","cwd":"/work","modelProvider":"third-party"},"model":"custom-model","modelProvider":"third-party","approvalPolicy":"on-request","approvalsReviewer":"user","activePermissionProfile":{"id":":workspace"},"sandbox":{"type":"workspaceWrite","networkAccess":false,"writableRoots":[]}}`), nil
		case "thread/prepareApprovedPlan":
			if p["threadId"] != "fresh" || p["markdown"] != "Build the feature." {
				t.Fatalf("wrong approved plan %v", p)
			}
			return json.RawMessage(`{"prompt":"Implement the prepared plan."}`), nil
		case "turn/start":
			mode := p["collaborationMode"].(map[string]any)
			if mode["mode"] != "default" || mode["settings"].(map[string]any)["model"] != "custom-model" {
				t.Fatalf("not switched to default %v", p)
			}
			if fail {
				return nil, errors.New("delivery uncertain")
			}
			return json.RawMessage(`{"turn":{"id":"implementation","status":"inProgress"}}`), nil
		default:
			t.Fatalf("unexpected %s", method)
			return nil, nil
		}
	}, &calls
}

func planParams(action string) map[string]any {
	return map[string]any{"threadId": "source", "turnId": "turn", "itemId": "plan", "cwd": "/work", "action": action}
}

func TestPlanActionsNativeThreeChoices(t *testing.T) {
	for _, action := range []string{"implement", "fresh", "stay"} {
		t.Run(action, func(t *testing.T) {
			caller, calls := planFixture(t, false)
			actions := NewPlanActions(filepath.Join(t.TempDir(), "receipts"), caller)
			raw, err := actions.Respond(context.Background(), planParams(action))
			if err != nil {
				t.Fatal(err)
			}
			var result map[string]json.RawMessage
			if json.Unmarshal(raw, &result) != nil {
				t.Fatal(string(raw))
			}
			turns := 0
			prepared := 0
			for _, method := range *calls {
				if method == "turn/start" {
					turns++
				}
				if method == "thread/prepareApprovedPlan" {
					prepared++
				}
			}
			if action == "stay" {
				if turns != 0 {
					t.Fatal("stay submitted work")
				}
				return
			}
			if turns != 1 || (action == "fresh" && prepared != 1) {
				t.Fatalf("calls %v", *calls)
			}
			if _, err = actions.Respond(context.Background(), planParams(action)); err == nil {
				t.Fatal("plan dispatched twice")
			}
		})
	}
}

func TestPlanActionUncertainDeliveryAndRestartNeverReplay(t *testing.T) {
	caller, calls := planFixture(t, true)
	dir := filepath.Join(t.TempDir(), "receipts")
	actions := NewPlanActions(dir, caller)
	if _, err := actions.Respond(context.Background(), planParams("implement")); err == nil {
		t.Fatal("expected uncertain result")
	}
	actions = NewPlanActions(dir, caller)
	if _, err := actions.Respond(context.Background(), planParams("implement")); err == nil {
		t.Fatal("replayed after restart")
	}
	turns := 0
	for _, m := range *calls {
		if m == "turn/start" {
			turns++
		}
	}
	if turns != 1 {
		t.Fatal(*calls)
	}
}

func TestPlanActionRejectsStaleAndWrongWorkspaceBeforeMutation(t *testing.T) {
	for _, field := range []string{"turnId", "itemId", "cwd", "action"} {
		t.Run(field, func(t *testing.T) {
			caller, calls := planFixture(t, false)
			p := planParams("implement")
			p[field] = "wrong"
			if field == "cwd" {
				p[field] = "/other"
			}
			dir := filepath.Join(t.TempDir(), "receipts")
			_, err := NewPlanActions(dir, caller).Respond(context.Background(), p)
			if err == nil {
				t.Fatal("accepted invalid plan")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("receipt created: %v", err)
			}
			for _, m := range *calls {
				if m == "thread/resume" || m == "turn/start" || m == "thread/start" {
					t.Fatal("mutated stale plan")
				}
			}
		})
	}
}

func TestPlanActionConcurrentResponsesDispatchOnce(t *testing.T) {
	var mu sync.Mutex
	turns := 0
	caller, _ := planFixture(t, false)
	locked := func(ctx context.Context, m string, p any) (json.RawMessage, error) {
		mu.Lock()
		defer mu.Unlock()
		if m == "turn/start" {
			turns++
		}
		return caller(ctx, m, p)
	}
	a := NewPlanActions(filepath.Join(t.TempDir(), "receipts"), locked)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = a.Respond(context.Background(), planParams("implement")) }()
	}
	wg.Wait()
	if turns != 1 {
		t.Fatalf("submitted %d times", turns)
	}
}

func TestPlanActionPublicCommandAndHistoryProjection(t *testing.T) {
	command, err := DecodeCommand([]byte(`{"id":1,"method":"plans/respond","params":{"workspace":"/work","thread_id":"source","turn_id":"turn","item_id":"plan","action":"stay"}}`))
	if err != nil {
		t.Fatal(err)
	}
	caller, _ := planFixture(t, false)
	actions := NewPlanActions(filepath.Join(t.TempDir(), "receipts"), caller)
	routed := func(ctx context.Context, m string, p any) (json.RawMessage, error) {
		values := p.(map[string]any)
		switch m {
		case "modex/plan/respond":
			return actions.Respond(ctx, values)
		case "thread/read":
			return json.RawMessage(`{"thread":{"id":"source","cwd":"/work","status":{"type":"idle"}}}`), nil
		case "modex/plan/state":
			handled, err := actions.Handled("source", "turn", "plan")
			raw, _ := json.Marshal(map[string]bool{"handled": handled})
			return raw, err
		default:
			return caller(ctx, m, p)
		}
	}
	if _, err = Handle(context.Background(), "/work", command, routed); err != nil {
		t.Fatal(err)
	}
	result, err := Handle(context.Background(), "/work", Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"source","include_turns":true,"limit":1}`)}, routed)
	if err != nil {
		t.Fatal(err)
	}
	messages := result.(map[string]any)["messages"].([]map[string]any)
	if len(messages) != 1 || messages[0]["role"] != "plan" || messages[0]["plan_handled"] != true || messages[0]["turn_id"] != "turn" {
		t.Fatalf("plan lost on reconnect: %v", messages)
	}
}

func TestFreshPlanDoesNotSubmitWithChangedPermissions(t *testing.T) {
	caller, _ := planFixture(t, false)
	turns := 0
	changed := func(ctx context.Context, m string, p any) (json.RawMessage, error) {
		if m == "thread/start" {
			return json.RawMessage(`{"thread":{"id":"fresh","cwd":"/work"},"approvalPolicy":"never","approvalsReviewer":"user","activePermissionProfile":{"id":":full"},"sandbox":{"type":"dangerFullAccess"}}`), nil
		}
		if m == "turn/start" {
			turns++
		}
		return caller(ctx, m, p)
	}
	_, err := NewPlanActions(filepath.Join(t.TempDir(), "receipts"), changed).Respond(context.Background(), planParams("fresh"))
	if err == nil || turns != 0 {
		t.Fatalf("unsafe permission change: %v, turns=%d", err, turns)
	}
}

func TestFreshPlanCopiesLegacyReadOnlyWithoutProfile(t *testing.T) {
	caller, _ := planFixture(t, false)
	legacy := func(ctx context.Context, m string, p any) (json.RawMessage, error) {
		if m == "thread/resume" || m == "thread/start" {
			if m == "thread/start" {
				values := p.(map[string]any)
				if values["sandbox"] != "read-only" || values["permissions"] != nil {
					t.Fatalf("legacy policy not copied: %v", values)
				}
			}
			raw, err := caller(ctx, "thread/resume", p)
			if err != nil {
				return nil, err
			}
			var session map[string]any
			_ = json.Unmarshal(raw, &session)
			session["activePermissionProfile"] = nil
			session["sandbox"] = map[string]any{"type": "readOnly", "networkAccess": false}
			if m == "thread/start" {
				session["thread"].(map[string]any)["id"] = "fresh"
			}
			return json.Marshal(session)
		}
		return caller(ctx, m, p)
	}
	if _, err := NewPlanActions(filepath.Join(t.TempDir(), "receipts"), legacy).Respond(context.Background(), planParams("fresh")); err != nil {
		t.Fatal(err)
	}
}

func TestPlanActionRejectsUnverifiedGoalBeforeResume(t *testing.T) {
	for _, tc := range []struct {
		name, goal string
		fail       bool
	}{
		{"active", `{"goal":{"threadId":"source","status":"active"}}`, false},
		{"missing", `{}`, false}, {"malformed", `{"goal":false}`, false},
		{"wrong-thread", `{"goal":{"threadId":"other","status":"paused"}}`, false},
		{"unknown-status", `{"goal":{"threadId":"source","status":"unknown"}}`, false},
		{"unreadable", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller, calls := planFixture(t, false)
			dir := filepath.Join(t.TempDir(), "receipts")
			wrapped := func(ctx context.Context, method string, p any) (json.RawMessage, error) {
				if method == "thread/goal/get" {
					if tc.fail {
						return nil, errors.New("unreadable")
					}
					return json.RawMessage(tc.goal), nil
				}
				return caller(ctx, method, p)
			}
			_, err := NewPlanActions(dir, wrapped).Respond(context.Background(), planParams("stay"))
			if err == nil {
				t.Fatal("accepted unsafe goal")
			}
			for _, method := range *calls {
				if method == "thread/resume" || method == "thread/start" || method == "turn/start" {
					t.Fatalf("mutated unsafe source: %v", *calls)
				}
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("receipt created: %v", err)
			}
		})
	}
}

func TestPlanActionRejectsUnverifiedSourceStatus(t *testing.T) {
	for _, status := range []string{`null`, `{}`, `{"type":"active"}`} {
		t.Run(status, func(t *testing.T) {
			caller, calls := planFixture(t, false)
			wrapped := func(ctx context.Context, method string, p any) (json.RawMessage, error) {
				if method == "thread/read" {
					return json.RawMessage(`{"thread":{"id":"source","cwd":"/work","status":` + status + `}}`), nil
				}
				return caller(ctx, method, p)
			}
			if _, err := NewPlanActions(filepath.Join(t.TempDir(), "receipts"), wrapped).Respond(context.Background(), planParams("stay")); err == nil {
				t.Fatal("accepted unsafe status")
			}
			if len(*calls) != 0 {
				t.Fatalf("called after unsafe read: %v", *calls)
			}
		})
	}
}
