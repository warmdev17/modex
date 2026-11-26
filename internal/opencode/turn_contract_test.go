package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/warmdev/modex/internal/bridge"
)

func TestFinalHistorySteerKeepsOriginalTurn(t *testing.T) {
	var messages []message
	_ = json.Unmarshal([]byte(`[{"id":"u1","type":"user","text":"start"},{"id":"a1","type":"assistant"},{"id":"u2","type":"user","text":"steer","metadata":{"modex_turn_id":"u1"}},{"id":"a2","type":"assistant"}]`), &messages)
	_, active, id := projectMessages("s", "/work", messages)
	if !active || id != "u1" {
		t.Fatalf("active=%v id=%q", active, id)
	}
}

func TestPagedHistoryRetainsLiveTurnWithoutUserAnchor(t *testing.T) {
	synth := NewSynthesizer("/work", nil)
	synth.StartTurn("ses_1", "msg_start", "/work")
	call := func(_ context.Context, method string, _ any, _ func(string, json.RawMessage)) (json.RawMessage, error) {
		if method == callSessionRead {
			return json.RawMessage(sessionBody), nil
		}
		return json.RawMessage(`{"data":[{"id":"msg_assistant","type":"assistant"}],"cursor":{"next":"older"}}`), nil
	}
	cmd := bridge.Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"ses_1","include_turns":true,"limit":1}`)}
	result, err := handleEvents(context.Background(), "/work", cmd, call, nil, synth)
	if err != nil {
		t.Fatal(err)
	}
	fields := result.(map[string]any)
	if fields["active_turn"] != true || fields["active_turn_id"] != "msg_start" {
		t.Fatalf("history returned wrong turn: %+v", fields)
	}
}

func TestFinalReconcileRejectsMissingAnchor(t *testing.T) {
	c := &eventCollector{}
	s := NewSynthesizer("/work", c.emit)
	s.StartTurn("s", "new", "/work")
	var session session
	session.ID = "s"
	session.Outcome = "succeeded"
	var messages []message
	_ = json.Unmarshal([]byte(`[{"id":"old","type":"user"},{"id":"answer","type":"assistant","text":"stale"}]`), &messages)
	s.Reconcile(session, messages)
	if _, active := s.ActiveTurn("s"); !active || c.hasMethod("item/agentMessage/delta") {
		t.Fatalf("stale snapshot applied: %+v", c.snapshot())
	}
}

func TestTurnWorkspaceMismatchRejectsMutation(t *testing.T) {
	for _, method := range []string{"turn/start", "turn/steer", "turn/interrupt"} {
		t.Run(method, func(t *testing.T) {
			server, req := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ string) {
				if r.Method == "GET" {
					jsonResponse(w, 200, `{"data":{"id":"ses_1","location":{"directory":"/other"}}}`)
				} else {
					jsonResponse(w, 200, `{"data":{"id":"u"},"interrupted":true}`)
				}
			})
			params := map[string]any{"thread_id": "ses_1"}
			if method != "turn/start" {
				params["turn_id"] = "turn_1"
			}
			if method != "turn/interrupt" {
				params["text"] = "hello"
			}
			_, err := runCommand(t, NewClient(server.URL, server.Client(), "/work"), "/work", method, params)
			if err == nil || err.Error() != "thread outside workspace" {
				t.Fatalf("err=%v", err)
			}
			for _, r := range *req {
				if r.Method != "GET" {
					t.Fatalf("mutation: %+v", r)
				}
			}
		})
	}
}

func TestSteerProjectsConfirmedUserMessage(t *testing.T) {
	for _, status := range []int{200, 409} {
		server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ string) {
			if r.Method == "GET" {
				jsonResponse(w, 200, `{"data":{"id":"ses_1","location":{"directory":"/work"}}}`)
				return
			}
			jsonResponse(w, status, `{"data":{"id":"msg_followup"}}`)
		})
		client := NewClient(server.URL, server.Client(), "/work")
		var events []json.RawMessage
		_, err := HandleEvents(context.Background(), "/work", bridge.Command{Method: "turn/steer", Params: json.RawMessage(`{"thread_id":"ses_1","turn_id":"msg_start","text":"follow up"}`)}, client.Call, func(method string, raw json.RawMessage) {
			if method == "item/completed" {
				events = append(events, raw)
			}
		})
		if status == 409 {
			if err == nil || len(events) != 0 {
				t.Fatalf("rejected steer: err=%v events=%s", err, events)
			}
			continue
		}
		if err != nil || len(events) != 1 {
			t.Fatalf("accepted steer: err=%v events=%s", err, events)
		}
		assertJSON(t, events[0], `{"threadId":"ses_1","turnId":"msg_start","item":{"id":"msg_followup","type":"userMessage","content":[{"type":"text","text":"follow up"}]}}`)
	}
}

func TestFinalConsumedDecisionNeverReoffered(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failure], func(t *testing.T) {
			var replies []decisionReply
			var replyErr error
			if failure {
				replyErr = errors.New("uncertain")
			}
			store, c := decisionStore(t, &replies, replyErr)
			raw := json.RawMessage(`{"id":"per_1","sessionID":"ses_1","action":"bash"}`)
			store.Handle("permission.asked", raw, "/work")
			_ = store.Answer(context.Background(), "ses_1/per_1", json.RawMessage(`{"decision":"accept"}`))
			store.Reconcile("ses_1", "/work", json.RawMessage(`{"data":[{"id":"per_1","sessionID":"ses_1","action":"bash"}]}`))
			store.Handle("permission.asked", raw, "/work")
			store.Replay(true)
			if len(store.Pending()) != 0 {
				t.Fatalf("reoffered: %+v events=%+v", store.Pending(), c.snapshot())
			}
		})
	}
}

func TestFinalAttachmentURIEncodesPath(t *testing.T) {
	server, req := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		if r.URL.Path == "/api/session/ses_1" {
			jsonResponse(w, 200, `{"data":{"id":"ses_1","location":{"directory":"/work"}}}`)
			return
		}
		jsonResponse(w, 200, `{"data":{"id":"u"}}`)
	})
	_, err := runCommand(t, NewClient(server.URL, server.Client(), "/work"), "/work", "turn/start", map[string]any{"thread_id": "ses_1", "text": "hi", "attachments": []map[string]string{{"path": "/tmp/a #?% b.txt"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range *req {
		if strings.HasSuffix(r.Path, "/prompt") && strings.Contains(r.Body, `file:///tmp/a%20%23%3F%25%20b.txt`) {
			return
		}
	}
	t.Fatalf("requests=%+v", *req)
}
