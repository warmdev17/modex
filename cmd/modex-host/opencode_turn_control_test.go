package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/warmdev/modex/internal/opencode"
)

func TestOpenCodeTurnControl(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/api/session/ses_1/interrupt" {
			_, _ = w.Write([]byte(`{"interrupted":true}`))
			return
		}
		if r.URL.Path == "/api/session/ses_1/prompt" {
			_, _ = w.Write([]byte(`{"data":{"id":"followup","type":"user"}}`))
			return
		}
		t.Errorf("unexpected request %s", r.URL.Path)
	}))
	defer server.Close()
	synth := opencode.NewSynthesizer("/work", nil)
	client := opencode.NewClient(server.URL, server.Client(), "/work")
	synth.StartTurn("ses_1", "turn_1", "/work")
	for _, method := range []string{"turn/interrupt", "turn/steer"} {
		_, err := openCodeTurnCall(context.Background(), client, synth, method, map[string]any{"sessionID": "ses_1", "turnID": "stale", "text": "follow up"}, nil)
		if err == nil || requests.Load() != 0 {
			t.Fatalf("%s stale: err=%v requests=%d", method, err, requests.Load())
		}
	}
	raw, err := openCodeTurnCall(context.Background(), client, synth, "turn/steer", map[string]any{"sessionID": "ses_1", "turnID": "turn_1", "text": "follow up"}, nil)
	if err != nil || string(raw) != `{"turn":{"id":"turn_1","status":"inProgress"}}` || synth.TurnID("ses_1") != "turn_1" {
		t.Fatalf("steer: raw=%s err=%v turn=%s", raw, err, synth.TurnID("ses_1"))
	}
	_, err = openCodeTurnCall(context.Background(), client, synth, "turn/interrupt", map[string]any{"sessionID": "ses_1", "turnID": "turn_1"}, nil)
	if err != nil || requests.Load() != 2 {
		t.Fatalf("interrupt: err=%v requests=%d", err, requests.Load())
	}
	synth.Handle("session.idle", json.RawMessage(`{"sessionID":"ses_1"}`))
	for _, method := range []string{"turn/interrupt", "turn/steer"} {
		_, err := openCodeTurnCall(context.Background(), client, synth, method, map[string]any{"sessionID": "ses_1", "turnID": "turn_1", "text": "follow up"}, nil)
		if err == nil || requests.Load() != 2 {
			t.Fatalf("%s completed: err=%v requests=%d", method, err, requests.Load())
		}
	}
}
