package bridge

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestGoalCommands(t *testing.T) {
	for _, tc := range []struct{ method, params, native, expected string }{
		{"threads/goal/get", `{"thread_id":"t"}`, "thread/goal/get", `{"goal":null}`},
		{"threads/goal/set", `{"thread_id":"t","objective":"ship","status":"paused","token_budget":100}`, "thread/goal/set", `{"goal":{"threadId":"t","objective":"ship","status":"paused","tokenBudget":100,"tokensUsed":4,"timeUsedSeconds":2,"createdAt":1,"updatedAt":3}}`},
		{"threads/goal/clear", `{"thread_id":"t"}`, "thread/goal/clear", `{"cleared":true}`},
	} {
		t.Run(tc.method, func(t *testing.T) {
			var calls []string
			result, err := Handle(context.Background(), "/work", Command{Method: tc.method, Params: json.RawMessage(tc.params)}, func(_ context.Context, method string, params any) (json.RawMessage, error) {
				calls = append(calls, method)
				if method == "thread/read" {
					return json.RawMessage(`{"thread":{"id":"t","cwd":"/work"}}`), nil
				}
				if method != tc.native {
					t.Fatalf("unexpected method %s", method)
				}
				p := params.(map[string]any)
				if p["threadId"] != "t" {
					t.Fatalf("thread id: %v", p)
				}
				if tc.method == "threads/goal/set" && (p["objective"] != "ship" || p["status"] != "paused" || p["tokenBudget"] != int64(100)) {
					t.Fatalf("set parameters: %v", p)
				}
				if tc.method != "threads/goal/get" && p["origin"] != "user" {
					t.Fatalf("origin: %v", p)
				}
				return json.RawMessage(tc.expected), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, []string{"thread/read", tc.native}) {
				t.Fatalf("calls: %v", calls)
			}
			b, _ := json.Marshal(result)
			if tc.method == "threads/goal/set" && string(b) != `{"goal":{"thread_id":"t","objective":"ship","status":"paused","token_budget":100,"tokens_used":4,"time_used_seconds":2,"created_at":1,"updated_at":3}}` {
				t.Fatalf("result %s", b)
			}
		})
	}
}

func TestGoalRejectsInvalidRequestsAndWrongThread(t *testing.T) {
	for _, params := range []string{`{"thread_id":"t","status":"complete"}`, `{"thread_id":"t","token_budget":0}`, `{"thread_id":"t","token_budget":1.5}`, `{"thread_id":"t","objective":" "}`, `{"thread_id":"t","extra":1}`} {
		called := false
		_, err := Handle(context.Background(), "/work", Command{Method: "threads/goal/set", Params: json.RawMessage(params)}, func(context.Context, string, any) (json.RawMessage, error) { called = true; return nil, nil })
		if err == nil || called {
			t.Fatalf("invalid %s: err %v called %v", params, err, called)
		}
	}
	for _, response := range []string{`{"thread":{"id":"t","cwd":"/other"}}`, `{"thread":{"id":"wrong","cwd":"/work"}}`} {
		calls := 0
		_, err := Handle(context.Background(), "/work", Command{Method: "threads/goal/get", Params: json.RawMessage(`{"thread_id":"t"}`)}, func(context.Context, string, any) (json.RawMessage, error) {
			calls++
			return json.RawMessage(response), nil
		})
		if err == nil || calls != 1 {
			t.Fatalf("thread response %s err %v calls %d", response, err, calls)
		}
	}
	_, err := Handle(context.Background(), "/work", Command{Method: "threads/goal/get", Params: json.RawMessage(`{"thread_id":"t"}`)}, func(_ context.Context, method string, _ any) (json.RawMessage, error) {
		if method == "thread/read" {
			return json.RawMessage(`{"thread":{"id":"t","cwd":"/work"}}`), nil
		}
		return json.RawMessage(`{"goal":{"threadId":"other","objective":"x","status":"active","tokensUsed":0,"timeUsedSeconds":0,"createdAt":1,"updatedAt":1}}`), nil
	})
	if err == nil {
		t.Fatal("accepted another thread's goal")
	}
}

func TestGoalEventRejectsMismatchedThread(t *testing.T) {
	_, err := EncodeEvent("thread/goal/updated", json.RawMessage(`{"threadId":"t","goal":{"threadId":"other","objective":"x","status":"active","tokensUsed":0,"timeUsedSeconds":0,"createdAt":1,"updatedAt":1}}`))
	if err == nil {
		t.Fatal("accepted mismatched goal event")
	}
}

func TestGoalEventPreservesNativeFields(t *testing.T) {
	raw, err := EncodeEvent("thread/goal/updated", json.RawMessage(`{"threadId":"t","turnId":"turn","goal":{"threadId":"t","objective":"x","status":"paused","tokenBudget":null,"tokensUsed":3,"timeUsedSeconds":2,"createdAt":1,"updatedAt":4,"secret":"hide"},"secret":"hide"}`))
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Params struct {
			ThreadID string         `json:"threadId"`
			TurnID   string         `json:"turnId"`
			Goal     map[string]any `json:"goal"`
		} `json:"params"`
	}
	if json.Unmarshal(raw, &event) != nil || event.Params.ThreadID != "t" || event.Params.TurnID != "turn" || event.Params.Goal["threadId"] != "t" || event.Params.Goal["tokensUsed"] != float64(3) || event.Params.Goal["secret"] != nil {
		t.Fatalf("event %s", raw)
	}
}
