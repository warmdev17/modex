package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/warmdev/modex/internal/diagnostic"
)

const lifecycleSource = `{"thread":{"id":"source","cwd":"/work","modelProvider":"custom","projectId":"project","status":{"type":"idle"}},"model":"model","modelProvider":"custom","approvalPolicy":"on-request","approvalsReviewer":"user","activePermissionProfile":{"id":":workspace"},"sandbox":{"type":"workspaceWrite","networkAccess":false,"writableRoots":["/work"]}}`

const lifecycleGoal = `{"goal":{"threadId":"source","objective":"Synthetic goal","status":"paused","createdAt":1,"updatedAt":1,"tokensUsed":0,"timeUsedSeconds":0,"tokenBudget":null}}`

func TestForkRejectsActiveOrUnverifiedGoalBeforeResume(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		err            error
	}{
		{"active", strings.Replace(lifecycleGoal, `"paused"`, `"active"`, 1), nil},
		{"missing-goal", `{}`, nil},
		{"null-response", `null`, nil},
		{"malformed-response", `{`, nil},
		{"malformed-goal", `{"goal":false}`, nil},
		{"missing-status", `{"goal":{"threadId":"source"}}`, nil},
		{"unknown-status", strings.Replace(lifecycleGoal, `"paused"`, `"unknown"`, 1), nil},
		{"wrong-thread", strings.Replace(lifecycleGoal, `"source"`, `"other"`, 1), nil},
		{"read-failed", "", errors.New("goal read failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			result, err := Handle(context.Background(), "/work", Command{Method: "threads/fork", Params: json.RawMessage(`{"thread_id":"source"}`)}, func(_ context.Context, method string, params any) (json.RawMessage, error) {
				calls = append(calls, method)
				switch method {
				case "thread/read":
					return json.RawMessage(lifecycleSource), nil
				case "thread/goal/get":
					p := params.(map[string]any)
					if len(p) != 1 || p["threadId"] != "source" {
						t.Fatalf("goal identity: %v", p)
					}
					return json.RawMessage(tc.response), tc.err
				case "thread/fork":
					return json.RawMessage(strings.Replace(lifecycleSource, `"id":"source"`, `"id":"fork"`, 1)), nil
				default:
					return json.RawMessage(lifecycleSource), nil
				}
			})
			if err == nil || result != nil || strings.Join(calls, ",") != "thread/read,thread/goal/get" {
				t.Fatalf("unsafe goal resumed or mutated: calls=%v result=%v err=%v", calls, result, err)
			}
		})
	}
}

func TestForkAllowsVerifiedInactiveGoalAndDefersContinuation(t *testing.T) {
	for _, status := range []string{"paused", "blocked", "usageLimited", "budgetLimited", "complete"} {
		t.Run(status, func(t *testing.T) {
			var calls []string
			_, err := Handle(context.Background(), "/work", Command{Method: "threads/fork", Params: json.RawMessage(`{"thread_id":"source"}`)}, func(_ context.Context, method string, params any) (json.RawMessage, error) {
				calls = append(calls, method)
				if method == "thread/goal/get" {
					return json.RawMessage(strings.Replace(lifecycleGoal, `"paused"`, `"`+status+`"`, 1)), nil
				}
				if method == "thread/fork" {
					if params.(map[string]any)["deferGoalContinuation"] != true {
						t.Fatal("fork may continue inherited goal")
					}
					return json.RawMessage(strings.Replace(lifecycleSource, `"id":"source"`, `"id":"fork"`, 1)), nil
				}
				return json.RawMessage(lifecycleSource), nil
			})
			if err != nil || strings.Join(calls, ",") != "thread/read,thread/goal/get,thread/resume,thread/fork" {
				t.Fatalf("calls=%v err=%v", calls, err)
			}
		})
	}
}

func TestThreadLifecycleCommands(t *testing.T) {
	for _, action := range []string{"archive", "unarchive", "delete", "fork"} {
		t.Run(action, func(t *testing.T) {
			if _, err := DecodeCommand([]byte(`{"id":1,"method":"threads/` + action + `"}`)); err != nil {
				t.Fatal(err)
			}
			calls := []string{}
			result, err := Handle(context.Background(), "/work", Command{Method: "threads/" + action, Params: json.RawMessage(`{"thread_id":"source"` + map[bool]string{true: `,"confirm":true`}[action == "delete"] + `}`)}, func(_ context.Context, method string, params any) (json.RawMessage, error) {
				calls = append(calls, method)
				p := params.(map[string]any)
				if p["threadId"] != "source" {
					t.Fatalf("identity: %v", p)
				}
				switch method {
				case "thread/read":
					if len(p) != 2 || p["includeTurns"] != false {
						t.Fatal(p)
					}
					return json.RawMessage(lifecycleSource), nil
				case "thread/goal/get":
					if len(p) != 1 {
						t.Fatal(p)
					}
					return json.RawMessage(`{"goal":null}`), nil
				case "thread/resume":
					if len(p) != 2 || p["excludeTurns"] != true {
						t.Fatal(p)
					}
					return json.RawMessage(lifecycleSource), nil
				case "thread/fork":
					if p["excludeTurns"] != true || p["deferGoalContinuation"] != true || p["permissions"] != ":workspace" || p["modelProvider"] == nil || p["cwd"] != "/work" {
						t.Fatalf("unsafe fork: %v", p)
					}
					return json.RawMessage(strings.Replace(lifecycleSource, `"id":"source"`, `"id":"fork"`, 1)), nil
				case "thread/unarchive":
					return json.RawMessage(lifecycleSource), nil
				case "thread/archive", "thread/delete":
					if len(p) != 1 {
						t.Fatal(p)
					}
					return json.RawMessage(`{}`), nil
				default:
					t.Fatalf("unexpected %s", method)
					return nil, nil
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(result)
			expected := "thread/read,thread/" + action
			if action == "fork" {
				expected = "thread/read,thread/goal/get,thread/resume,thread/fork"
				if !strings.Contains(string(raw), `"id":"fork"`) || !strings.Contains(string(raw), `"sandbox":"workspaceWrite"`) {
					t.Fatalf("projection: %s", raw)
				}
			}
			if strings.Join(calls, ",") != expected {
				t.Fatal(calls)
			}
		})
	}
}

func TestLifecycleValidationBeforeMutation(t *testing.T) {
	for _, action := range []string{"archive", "unarchive", "delete", "fork"} {
		for _, params := range []string{`{}`, `{"thread_id":""}`, `{"thread_id":"x\ny"}`, `{"thread_id":"source","extra":true}`, `{"thread_id":"source","workspace":"relative"}`} {
			_, err := Handle(context.Background(), "/work", Command{Method: "threads/" + action, Params: json.RawMessage(params)}, func(context.Context, string, any) (json.RawMessage, error) {
				t.Fatal("invalid request called upstream")
				return nil, nil
			})
			if err == nil {
				t.Fatalf("accepted %s %s", action, params)
			}
		}
		_, err := Handle(context.Background(), "/work", Command{Method: "threads/" + action, Params: json.RawMessage(`{"thread_id":"source"` + map[bool]string{true: `,"confirm":true`}[action == "delete"] + `}`)}, func(_ context.Context, m string, _ any) (json.RawMessage, error) {
			if m != "thread/read" {
				t.Fatal("mutation before workspace validation")
			}
			return json.RawMessage(strings.Replace(lifecycleSource, "/work", "/other", 1)), nil
		})
		if err == nil {
			t.Fatal("accepted wrong workspace")
		}
	}
	for _, confirm := range []string{"", `,"confirm":false`, `,"confirm":null`, `,"confirm":"true"`} {
		_, err := Handle(context.Background(), "/work", Command{Method: "threads/delete", Params: json.RawMessage(`{"thread_id":"source"` + confirm + `}`)}, func(context.Context, string, any) (json.RawMessage, error) {
			t.Fatal("missing confirmation called upstream")
			return nil, nil
		})
		if err == nil {
			t.Fatal("accepted missing confirmation")
		}
	}
}

func TestNativeDeleteRefusalIsSafeAndHasNoFallback(t *testing.T) {
	calls := 0
	_, err := Handle(context.Background(), "/work", Command{Method: "threads/delete", Params: json.RawMessage(`{"thread_id":"source","confirm":true}`)}, func(_ context.Context, m string, _ any) (json.RawMessage, error) {
		calls++
		if m == "thread/read" {
			return json.RawMessage(lifecycleSource), nil
		}
		if m != "thread/delete" {
			t.Fatal(m)
		}
		return nil, diagnostic.NewRPCError(json.RawMessage(`{"code":-32600,"message":"forked history still references it secret-key"}`))
	})
	if calls != 2 || err == nil || !strings.Contains(err.Error(), "thread_has_dependents") || strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestForkRejectsActiveAndChangedSemantics(t *testing.T) {
	for _, replacement := range [][2]string{{`"type":"idle"`, `"type":"active","activeFlags":["waitingOnApproval"]`}, {`"modelProvider":"custom"`, `"modelProvider":"other"`}, {`"projectId":"project"`, `"projectId":"other"`}, {`"model":"model"`, `"model":"other"`}, {`"approvalPolicy":"on-request"`, `"approvalPolicy":"never"`}, {`"writableRoots":["/work"]`, `"writableRoots":["/other"]`}} {
		_, err := Handle(context.Background(), "/work", Command{Method: "threads/fork", Params: json.RawMessage(`{"thread_id":"source"}`)}, func(_ context.Context, m string, _ any) (json.RawMessage, error) {
			if strings.Contains(replacement[0], "idle") {
				if m != "thread/read" {
					t.Fatal("active fork called upstream")
				}
				return json.RawMessage(strings.Replace(lifecycleSource, replacement[0], replacement[1], 1)), nil
			}
			if m == "thread/goal/get" {
				return json.RawMessage(`{"goal":null}`), nil
			}
			raw := lifecycleSource
			if m == "thread/fork" {
				raw = strings.ReplaceAll(raw, replacement[0], replacement[1])
				raw = strings.Replace(raw, `"id":"source"`, `"id":"fork"`, 1)
			}
			return json.RawMessage(raw), nil
		})
		if err == nil {
			t.Fatalf("accepted semantics drift %v", replacement)
		}
	}
}

func TestArchivedListPreservesScopeAndSourceFilters(t *testing.T) {
	for _, archived := range []string{"true", "false", "null", `"true"`} {
		result, err := Handle(context.Background(), "/work", Command{Method: "threads/list", Params: json.RawMessage(`{"archived":` + archived + `,"cursor":"next","limit":10}`)}, func(_ context.Context, m string, p any) (json.RawMessage, error) {
			if archived != "true" && archived != "false" {
				t.Fatal("invalid boolean reached upstream")
			}
			params := p.(map[string]any)
			if m != "thread/list" || params["archived"] != (archived == "true") || params["cwd"] != "/work" || params["cursor"] != "next" || params["sourceKinds"] == nil {
				t.Fatal(params)
			}
			return json.RawMessage(`{"data":[],"nextCursor":null}`), nil
		})
		if archived == "true" || archived == "false" {
			if err != nil || result == nil {
				t.Fatal(err)
			}
		} else if err == nil {
			t.Fatal("invalid archived accepted")
		}
	}
}

func TestForkPreservesReasoningAndRejectsUnavailablePermissionsBeforeMutation(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		_, err := Handle(context.Background(), "/work", Command{Method: "threads/fork", Params: json.RawMessage(`{"thread_id":"source"}`)}, func(_ context.Context, m string, p any) (json.RawMessage, error) {
			if m == "thread/goal/get" {
				return json.RawMessage(`{"goal":null}`), nil
			}
			raw := strings.Replace(lifecycleSource, `"model":"model"`, `"model":"model","reasoningEffort":"high"`, 1)
			if invalid {
				raw = strings.Replace(raw, `"approvalPolicy":"on-request"`, `"approvalPolicy":null`, 1)
			}
			if m == "thread/fork" {
				if invalid {
					t.Fatal("mutation with unavailable permissions")
				}
				params := p.(map[string]any)
				config, _ := params["config"].(map[string]any)
				if config["model_reasoning_effort"] != "high" {
					t.Fatal("reasoning effort was not preserved")
				}
				raw = strings.Replace(raw, `"id":"source"`, `"id":"fork"`, 1)
			}
			return json.RawMessage(raw), nil
		})
		if invalid && err == nil {
			t.Fatal("missing permissions accepted")
		}
		if !invalid && err != nil {
			t.Fatal(err)
		}
	}
}

func TestForkChecksEffectiveProviderAndPermissionCopies(t *testing.T) {
	for _, tc := range []struct {
		name, source, result string
		reject               bool
	}{
		{"effective-provider", lifecycleSource, strings.Replace(strings.Replace(lifecycleSource, `,"modelProvider":"custom","approvalPolicy"`, `,"modelProvider":"other","approvalPolicy"`, 1), `"id":"source"`, `"id":"fork"`, 1), true},
		{"unsafe-sandbox", strings.Replace(strings.Replace(lifecycleSource, `"activePermissionProfile":{"id":":workspace"},`, "", 1), `"type":"workspaceWrite"`, `"type":"externalSandbox"`, 1), "", true},
		{"workspace-copy", strings.Replace(lifecycleSource, `"activePermissionProfile":{"id":":workspace"},`, "", 1), "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Handle(context.Background(), "/work", Command{Method: "threads/fork", Params: json.RawMessage(`{"thread_id":"source"}`)}, func(_ context.Context, m string, value any) (json.RawMessage, error) {
				if m == "thread/goal/get" {
					return json.RawMessage(`{"goal":null}`), nil
				}
				if m != "thread/fork" {
					return json.RawMessage(tc.source), nil
				}
				if tc.name == "unsafe-sandbox" {
					t.Fatal("unsafe permissions mutated source")
				}
				if tc.name == "workspace-copy" {
					params := value.(map[string]any)
					if params["sandbox"] != "workspace-write" || params["config"] == nil || params["permissions"] != nil {
						t.Fatalf("permissions not copied: %v", params)
					}
				}
				raw := tc.result
				if raw == "" {
					raw = strings.Replace(tc.source, `"id":"source"`, `"id":"fork"`, 1)
				}
				return json.RawMessage(raw), nil
			})
			if tc.reject != (err != nil) {
				t.Fatalf("reject=%v err=%v", tc.reject, err)
			}
		})
	}
}
