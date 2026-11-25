package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func ssePayload(id, typ, data string) string {
	raw, _ := json.Marshal(map[string]any{"id": id, "type": typ, "data": json.RawMessage(data)})
	return string(raw)
}

func TestStreamParsesSSEAndDedupesReconnect(t *testing.T) {
	payloads := []string{
		ssePayload("evt_1", "session.execution.started", `{"sessionID":"ses_1"}`),
		ssePayload("evt_2", "session.text.started", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0}`),
		ssePayload("evt_3", "session.text.delta", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0,"delta":"Hel"}`),
		ssePayload("evt_4", "session.text.delta", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0,"delta":"lo"}`),
		ssePayload("evt_5", "session.reasoning.delta", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0,"delta":"think"}`),
		ssePayload("evt_6", "session.tool.input.started", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","id":"call_1","name":"bash"}`),
		ssePayload("evt_7", "session.tool.success", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","id":"call_1","content":[{"type":"text","text":"out"}]}`),
		ssePayload("evt_8", "session.text.ended", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0,"text":"Hello"}`),
		ssePayload("evt_9", "session.execution.succeeded", `{"sessionID":"ses_1"}`),
	}
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/event" {
			http.NotFound(w, r)
			return
		}
		connections.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, payload := range payloads {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
		// Returning closes the stream; Run reconnects and replays the same ids.
	}))
	defer server.Close()

	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "msg_user", "/work")
	client := NewClient(server.URL, server.Client(), "/work")
	stream := NewStream(client, synth)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = stream.Run(ctx)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !collector.hasMethod("turn/completed") {
		time.Sleep(10 * time.Millisecond)
	}
	// Allow at least one reconnect + replay before shutting down.
	time.Sleep(800 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not stop after cancellation")
	}
	if connections.Load() < 2 {
		t.Fatalf("expected a reconnect, connections=%d", connections.Load())
	}

	events := collector.snapshot()
	want := []string{
		"turn/started",
		"item/started",
		"item/agentMessage/delta",
		"item/agentMessage/delta",
		"item/started",
		"item/reasoning/summaryTextDelta",
		"item/started",
		"item/completed",
		"item/completed",
		"item/completed",
		"turn/completed",
	}
	if got := eventMethods(events); !reflect.DeepEqual(got, want) {
		t.Fatalf("methods=%v want %v (replayed events must be deduped)", got, want)
	}
}

func TestStreamPollingCompletesMissedTurn(t *testing.T) {
	sessionJSON := `{"id":"ses_1","outcome":"succeeded","time":{"created":1,"updated":3,"busy":2,"idle":3},"location":{"directory":"/work"}}`
	messagesJSON := `{"data":[
		{"id":"msg_asst","type":"assistant","content":[{"type":"text","text":"done"}],"time":{"created":2,"completed":3}},
		{"id":"msg_user","type":"user","text":"hi","time":{"created":1}}
	],"cursor":{"previous":null,"next":null}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/event":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				_, _ = fmt.Fprint(w, ": connected\n\n")
				flusher.Flush()
			}
			<-r.Context().Done()
		case "/api/session/ses_1":
			jsonResponse(w, http.StatusOK, `{"data":`+sessionJSON+`}`)
		case "/api/session/ses_1/message":
			jsonResponse(w, http.StatusOK, messagesJSON)
		case "/api/session/ses_1/permission":
			jsonResponse(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "msg_user", "/work")
	client := NewClient(server.URL, server.Client(), "/work")
	stream := NewStream(client, synth)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = stream.Run(ctx)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !collector.hasMethod("turn/completed") {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not stop after cancellation")
	}
	events := collector.snapshot()
	want := []string{"turn/started", "item/started", "item/agentMessage/delta", "item/completed", "turn/completed"}
	if got := eventMethods(events); !reflect.DeepEqual(got, want) {
		t.Fatalf("methods=%v want %v", got, want)
	}
	if !collector.hasMethod("turn/completed") {
		t.Fatal("polling reconciler did not complete the missed turn")
	}
}
