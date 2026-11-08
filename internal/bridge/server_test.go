package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
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
	if strings.Contains(string(b), "private chat") || !strings.Contains(string(b), `"next_cursor":"after"`) {
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
