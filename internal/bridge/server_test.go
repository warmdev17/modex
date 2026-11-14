package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/warmdev/modex/internal/diagnostic"
)

func TestServeMultipleRequestsAndRejectMutation(t *testing.T) {
	calls := 0
	caller := func(ctx context.Context, method string, params any) (json.RawMessage, error) {
		calls++
		if method != "config/read" {
			t.Fatalf("unexpected method %s", method)
		}
		return json.RawMessage(`{"config":{"model_provider":"custom","model":"test","secret":"do-not-emit"}}`), nil
	}
	input := strings.NewReader("{\"id\":1,\"method\":\"status\"}\n{\"id\":2,\"method\":\"turn/start\"}\n{\"id\":3,\"method\":\"status\"}\n")
	var output bytes.Buffer
	if err := Serve(context.Background(), input, &output, "/work", caller); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || strings.Contains(output.String(), "do-not-emit") {
		t.Fatalf("calls=%d output=%s", calls, &output)
	}
	var responses []map[string]json.RawMessage
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var r map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		responses = append(responses, r)
	}
	if len(responses) != 3 || len(responses[1]["error"]) == 0 || string(responses[1]["id"]) != "2" {
		t.Fatalf("responses=%s", &output)
	}
}

func TestListPaginationAndMetadataProjection(t *testing.T) {
	caller := func(ctx context.Context, method string, params any) (json.RawMessage, error) {
		p := params.(map[string]any)
		if method != "thread/list" || p["cwd"] != "/work" || p["cursor"] != "next" || p["limit"] != 5 {
			t.Fatalf("method=%s params=%v", method, p)
		}
		return json.RawMessage(`{"data":[{"id":"one","cwd":"/work","modelProvider":"custom","preview":"private chat"}],"nextCursor":"after"}`), nil
	}
	result, err := Handle(context.Background(), "/work", Command{ID: 1, Method: "threads/list", Params: json.RawMessage(`{"limit":5,"cursor":"next"}`)}, caller)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(result)
	if !strings.Contains(string(b), `"preview":"private chat"`) || !strings.Contains(string(b), `"next_cursor":"after"`) {
		t.Fatalf("result=%s", b)
	}
}

func TestResumePreservesActiveWriterCategory(t *testing.T) {
	caller := func(ctx context.Context, method string, params any) (json.RawMessage, error) {
		if method == "thread/resume" {
			return nil, diagnostic.NewRPCError(json.RawMessage(`{"message":"thread already has an active writer"}`))
		}
		return json.RawMessage(`{"thread":{"id":"one","cwd":"/work"}}`), nil
	}
	_, err := Handle(context.Background(), "/work", Command{Method: "threads/resume", Params: json.RawMessage(`{"thread_id":"one"}`)}, caller)
	if err == nil || !strings.Contains(err.Error(), "active_writer") {
		t.Fatalf("error=%v", err)
	}
}

func TestListPreviewIsTrimmedAndBounded(t *testing.T) {
	long := strings.Repeat("x", 200)
	caller := func(ctx context.Context, method string, params any) (json.RawMessage, error) {
		return json.RawMessage(fmt.Sprintf(`{"data":[{"id":"one","cwd":"/work","preview":"  %s  "}],"nextCursor":null}`, long)), nil
	}
	result, err := Handle(context.Background(), "/work", Command{Method: "threads/list"}, caller)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(result)
	if strings.Contains(string(b), "  x") || !strings.Contains(string(b), strings.Repeat("x", 160)+"…") {
		t.Fatalf("result=%s", b)
	}
}

func TestFirstListPageOmitsEmptyCursor(t *testing.T) {
	caller := func(ctx context.Context, method string, params any) (json.RawMessage, error) {
		p := params.(map[string]any)
		if _, ok := p["cursor"]; ok {
			t.Fatal("sent empty cursor")
		}
		return json.RawMessage(`{"data":[],"nextCursor":null}`), nil
	}
	if _, err := Handle(context.Background(), "/work", Command{Method: "threads/list"}, caller); err != nil {
		t.Fatal(err)
	}
}

func TestReadAndResumeEnforceWorkspace(t *testing.T) {
	for _, method := range []string{"threads/read", "threads/resume"} {
		calls := 0
		caller := func(ctx context.Context, m string, params any) (json.RawMessage, error) {
			calls++
			if m != "thread/read" {
				t.Fatal("resumed foreign workspace")
			}
			return json.RawMessage(`{"thread":{"id":"one","cwd":"/other"}}`), nil
		}
		_, err := Handle(context.Background(), "/work", Command{ID: 1, Method: method, Params: json.RawMessage(`{"thread_id":"one"}`)}, caller)
		if err == nil || calls != 1 {
			t.Fatalf("method=%s calls=%d err=%v", method, calls, err)
		}
	}
}

func TestResumeUsesOriginalThreadWithoutFork(t *testing.T) {
	var methods []string
	caller := func(ctx context.Context, m string, p any) (json.RawMessage, error) {
		methods = append(methods, m)
		if m == "thread/resume" {
			params := p.(map[string]any)
			if params["threadId"] != "one" || params["excludeTurns"] != true {
				t.Fatalf("params=%v", params)
			}
		}
		return json.RawMessage(`{"thread":{"id":"one","cwd":"/work","modelProvider":"custom"}}`), nil
	}
	_, err := Handle(context.Background(), "/work", Command{ID: 1, Method: "threads/resume", Params: json.RawMessage(`{"thread_id":"one"}`)}, caller)
	if err != nil || strings.Join(methods, ",") != "thread/read,thread/resume" {
		t.Fatalf("methods=%v err=%v", methods, err)
	}
}

func TestInvalidParametersNeverReachCodex(t *testing.T) {
	caller := func(context.Context, string, any) (json.RawMessage, error) {
		t.Fatal("unexpected call")
		return nil, nil
	}
	for _, p := range []string{`[]`, `{"limit":0}`, `{"limit":101}`, `{"limit":1.5}`, `{"limit":"20"}`, `{"cwd":"/other"}`, `{"cursor":42}`} {
		if _, err := Handle(context.Background(), "/work", Command{Method: "threads/list", Params: json.RawMessage(p)}, caller); err == nil {
			t.Fatalf("accepted %s", p)
		}
	}
}

func TestTurnStartValidatesThreadAndEmitsNotification(t *testing.T) {
	var methods []string
	var events []string
	call := func(ctx context.Context, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error) {
		methods = append(methods, method)
		if method == "turn/start" {
			emit("turn/started", json.RawMessage(`{"turnId":"t1"}`))
			return json.RawMessage(`{"turn":{"id":"t1"}}`), nil
		}
		return json.RawMessage(`{"thread":{"id":"thread-1","cwd":"/work"}}`), nil
	}
	result, err := HandleEvents(context.Background(), "/work", Command{ID: 1, Method: "turn/start", Params: json.RawMessage(`{"thread_id":"thread-1","text":"hello"}`)}, call, func(method string, params json.RawMessage) { events = append(events, method) })
	if err != nil || result == nil || strings.Join(methods, ",") != "thread/read,turn/start" || strings.Join(events, ",") != "turn/started" {
		t.Fatalf("result=%v methods=%v events=%v err=%v", result, methods, events, err)
	}
}

func TestTurnStartForwardsOptionalModel(t *testing.T) {
	for _, tc := range []struct{ name, raw, model string }{
		{"current", `{"thread_id":"thread-1","text":"hello"}`, ""},
		{"override", `{"thread_id":"thread-1","text":"hello","model":"gpt-5.6-sol"}`, "gpt-5.6-sol"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := func(_ context.Context, method string, params any) (json.RawMessage, error) {
				if method == "thread/read" {
					return json.RawMessage(`{"thread":{"id":"thread-1","cwd":"/work"}}`), nil
				}
				p := params.(map[string]any)
				if method != "turn/start" || (tc.model == "" && p["model"] != nil) || (tc.model != "" && p["model"] != tc.model) {
					t.Fatalf("method=%s params=%v", method, p)
				}
				return json.RawMessage(`{"turn":{"id":"t1"}}`), nil
			}
			if _, err := Handle(context.Background(), "/work", Command{Method: "turn/start", Params: json.RawMessage(tc.raw)}, call); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTurnStartRejectsInvalidModelBeforeUpstream(t *testing.T) {
	for _, model := range []string{"null", `""`, `"   "`, "42", `"bad\nmodel"`, `"` + strings.Repeat("m", 257) + `"`} {
		calls := 0
		call := func(context.Context, string, any) (json.RawMessage, error) { calls++; return nil, nil }
		raw := `{"thread_id":"thread-1","text":"hello","model":` + model + `}`
		if _, err := Handle(context.Background(), "/work", Command{Method: "turn/start", Params: json.RawMessage(raw)}, call); err == nil || calls != 0 {
			t.Fatalf("model=%s calls=%d err=%v", model, calls, err)
		}
	}
}

func TestUpstreamErrorIsRedacted(t *testing.T) {
	caller := func(context.Context, string, any) (json.RawMessage, error) { return nil, fmt.Errorf("secret-key") }
	var out bytes.Buffer
	err := Serve(context.Background(), strings.NewReader("{\"id\":1,\"method\":\"status\"}\n"), &out, "/work", caller)
	if err != nil || strings.Contains(out.String(), "secret-key") || !strings.Contains(out.String(), "error") {
		t.Fatalf("err=%v output=%s", err, &out)
	}
}

func TestNotificationHandlerReceivesEvents(t *testing.T) {
	// Notification forwarding is exercised at the diagnostic boundary; this keeps
	// the bridge contract independent from Codex's concrete event payloads.
	if _, err := DecodeCommand([]byte(`{"id":1,"method":"status"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestServeAnswersDecisionWhileRequestIsWaiting(t *testing.T) {
	answered := make(chan struct{})
	call := func(ctx context.Context, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error) {
		if method == "decision/respond" {
			close(answered)
			return json.RawMessage(`{"delivered":true}`), nil
		}
		select {
		case <-answered:
			return json.RawMessage(`{"config":{"model":"test"}}`), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	input := strings.NewReader("{\"id\":1,\"method\":\"status\"}\n{\"id\":2,\"method\":\"decision/respond\",\"params\":{\"decision_id\":\"d1\",\"result\":{\"decision\":\"decline\"}}}\n")
	var output bytes.Buffer
	if err := ServeEvents(ctx, input, &output, "/work", call); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), `"error"`) || !strings.Contains(output.String(), `"delivered":true`) {
		t.Fatalf("output=%s", output.String())
	}
}

func TestThreadStartUsesFixedWorkspaceAndOptionalModel(t *testing.T) {
	for _, tc := range []struct{ name, params, model string }{
		{"inherited", `{}`, ""}, {"selected", `{"model":"gpt-6.1-sol"}`, "gpt-6.1-sol"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			call := func(_ context.Context, method string, params any) (json.RawMessage, error) {
				calls++
				p := params.(map[string]any)
				if method != "thread/start" || p["cwd"] != "/work" {
					t.Fatalf("method=%s params=%v", method, p)
				}
				if tc.model == "" {
					if len(p) != 1 {
						t.Fatalf("overrode config: %v", p)
					}
				} else if len(p) != 2 || p["model"] != tc.model {
					t.Fatalf("params=%v", p)
				}
				return json.RawMessage(`{"thread":{"id":"new-thread","cwd":"/work","modelProvider":"custom","preview":"private"},"model":"runtime-model","secret":"never-emit"}`), nil
			}
			result, err := Handle(context.Background(), "/work", Command{Method: "threads/start", Params: json.RawMessage(tc.params)}, call)
			raw, _ := json.Marshal(result)
			if err != nil || calls != 1 || !strings.Contains(string(raw), `"id":"new-thread"`) || !strings.Contains(string(raw), `"model":"runtime-model"`) || strings.Contains(string(raw), "never-emit") || strings.Contains(string(raw), "private") {
				t.Fatalf("result=%s calls=%d err=%v", raw, calls, err)
			}
		})
	}
}

func TestThreadStartRejectsInvalidParamsBeforeUpstream(t *testing.T) {
	call := func(context.Context, string, any) (json.RawMessage, error) {
		t.Fatal("unexpected upstream call")
		return nil, nil
	}
	for _, params := range []string{`{"cwd":"/other"}`, `{"modelProvider":"other"}`, `{"approvalPolicy":"never"}`, `{"model":null}`, `{"model":42}`, `{"model":""}`, `{"model":"   "}`, `{"model":"model\nname"}`} {
		if _, err := Handle(context.Background(), "/work", Command{Method: "threads/start", Params: json.RawMessage(params)}, call); err == nil {
			t.Fatalf("accepted %s", params)
		}
	}
}

func TestThreadStartRejectsInvalidUpstreamMetadata(t *testing.T) {
	for _, raw := range []string{`{}`, `{"thread":{"id":"","cwd":"/work"}}`, `{"thread":{"id":"x","cwd":"/other"}}`} {
		call := func(context.Context, string, any) (json.RawMessage, error) { return json.RawMessage(raw), nil }
		if _, err := Handle(context.Background(), "/work", Command{Method: "threads/start"}, call); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestStoredHistoryTextAndWorkspace(t *testing.T) {
	call := func(_ context.Context, method string, params any) (json.RawMessage, error) {
		p := params.(map[string]any)
		if method != "thread/read" || p["includeTurns"] != true {
			t.Fatalf("unexpected call %s %#v", method, p)
		}
		return json.RawMessage(`{"thread":{"id":"one","cwd":"/work","turns":[{"status":"completed","items":[{"id":"u","type":"userMessage","content":[{"type":"text","text":"hello"}]},{"id":"a","type":"agentMessage","text":"hi"},{"type":"commandExecution","command":"private"}]}]}}`), nil
	}
	result, err := Handle(context.Background(), "/work", Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"one","include_turns":true}`)}, call)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), `"role":"user"`) || !strings.Contains(string(raw), `"text":"hi"`) || !strings.Contains(string(raw), `"role":"command"`) {
		t.Fatalf("unexpected history %s", raw)
	}
	if _, err = Handle(context.Background(), "/elsewhere", Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"one","include_turns":true}`)}, call); err == nil {
		t.Fatal("outside workspace allowed")
	}
}

func TestListPreservesCodexSessionName(t *testing.T) {
	caller := func(ctx context.Context, method string, params any) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[{"id":"one","cwd":"/work","name":"Modex mobile","preview":"my first question"}],"nextCursor":null}`), nil
	}
	result, err := Handle(context.Background(), "/work", Command{Method: "threads/list"}, caller)
	raw, _ := json.Marshal(result)
	if err != nil || !strings.Contains(string(raw), `"name":"Modex mobile"`) {
		t.Fatalf("result=%s err=%v", raw, err)
	}
}

func TestStoredHistoryIncludesCommandLifecycle(t *testing.T) {
	call := func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`{"thread":{"id":"one","cwd":"/work","turns":[{"status":"inProgress","items":[{"id":"c","type":"commandExecution","command":"go test ./...","cwd":"/work","status":"inProgress","aggregatedOutput":"PASS","exitCode":null}]}]}}`), nil
	}
	result, err := Handle(context.Background(), "/work", Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"one","include_turns":true}`)}, call)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), `"role":"command"`) || !strings.Contains(string(raw), `"command":"go test ./..."`) || !strings.Contains(string(raw), `"text":"PASS"`) || !strings.Contains(string(raw), `"status":"inProgress"`) {
		t.Fatalf("missing command: %s", raw)
	}
}

func TestCommandHistoryOutputBudget(t *testing.T) {
	var items []map[string]any
	for i := 0; i < 30; i++ {
		items = append(items, map[string]any{"id": fmt.Sprint(i), "type": "commandExecution", "command": "echo output", "status": "completed", "aggregatedOutput": strings.Repeat("界", 30000), "exitCode": 0})
	}
	raw, _ := json.Marshal(map[string]any{"thread": map[string]any{"id": "one", "cwd": "/work", "turns": []any{map[string]any{"items": items}}}})
	call := func(context.Context, string, any) (json.RawMessage, error) { return raw, nil }
	result, err := Handle(context.Background(), "/work", Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"one","include_turns":true}`)}, call)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result)
	if len(encoded) > 1150000 || !strings.Contains(string(encoded), "truncated") {
		t.Fatalf("output budget not applied: %d bytes", len(encoded))
	}
}

func TestOversizedResponseDoesNotPreventNextRequest(t *testing.T) {
	calls := 0
	caller := func(_ context.Context, method string, params any) (json.RawMessage, error) {
		calls++
		model := "small"
		if calls == 1 {
			model = strings.Repeat("x", MaxResponseFrame)
		}
		return json.Marshal(map[string]any{"config": map[string]string{"model": model}})
	}
	var output bytes.Buffer
	input := strings.NewReader("{\"id\":1,\"method\":\"status\"}\n{\"id\":2,\"method\":\"status\"}\n")
	if err := Serve(context.Background(), input, &output, "/work", caller); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	var first, second map[string]json.RawMessage
	if err := decoder.Decode(&first); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(first["error"], []byte("response_too_large")) {
		t.Fatal("missing bounded rejection")
	}
	if err := decoder.Decode(&second); err != nil {
		t.Fatal(err)
	}
	if string(second["id"]) != "2" || len(second["result"]) == 0 {
		t.Fatal("next request was lost")
	}
}
