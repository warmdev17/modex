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

func TestPollRejectsSnapshotForReplacedTurn(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/s":
			close(started)
			<-release
			jsonResponse(w, http.StatusOK, `{"data":{"id":"s","outcome":"succeeded","location":{"directory":"/work"}}}`)
		case "/api/session/s/message":
			jsonResponse(w, http.StatusOK, `{"data":[{"id":"new","type":"user"},{"id":"old","type":"user"}],"cursor":{}}`)
		case "/api/session/s/permission":
			jsonResponse(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("s", "old", "/work")
	stream := NewStream(NewClient(server.URL, server.Client(), "/work"), synth)
	done := make(chan struct{})
	go func() { defer close(done); stream.reconcile(context.Background(), "s", true) }()
	<-started
	synth.StartTurn("s", "new", "/work")
	close(release)
	<-done
	if id, active := synth.ActiveTurn("s"); !active || id != "new" {
		t.Fatalf("replacement turn affected: %v", collector.snapshot())
	}
}

func TestPollFindsTurnAnchorOnOlderPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/s":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"s","location":{"directory":"/work"}}}`)
		case "/api/session/s/message":
			if r.URL.Query().Get("cursor") == "older" {
				jsonResponse(w, http.StatusOK, `{"data":[{"id":"start","type":"user"}],"cursor":{}}`)
				return
			}
			recent := []map[string]string{{"id": "idle", "type": "idle", "outcome": "succeeded"}}
			for i := 1; i < pollMessages; i++ {
				recent = append(recent, map[string]string{"id": fmt.Sprintf("followup-%d", i), "type": "user"})
			}
			raw, _ := json.Marshal(map[string]any{"data": recent, "cursor": map[string]string{"next": "older"}})
			jsonResponse(w, http.StatusOK, string(raw))
		case "/api/session/s/permission":
			jsonResponse(w, http.StatusOK, `{"data":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	synth := NewSynthesizer("/work", nil)
	synth.StartTurn("s", "start", "/work")
	NewStream(NewClient(server.URL, server.Client(), "/work"), synth).reconcile(context.Background(), "s", true)
	if _, active := synth.ActiveTurn("s"); active {
		t.Fatal("missed completion with the original prompt on the second page")
	}
}

func TestPollFindsTurnAnchorAfterSeveralShrunkPages(t *testing.T) {
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		switch r.URL.Path {
		case "/api/session/s":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"s","location":{"directory":"/work"}}}`)
		case "/api/session/s/message":
			if r.URL.Query().Get("limit") != "1" {
				w.Header().Set("Content-Length", "268435457")
				w.WriteHeader(http.StatusOK)
				return
			}
			var page int
			_, _ = fmt.Sscan(r.URL.Query().Get("cursor"), &page)
			switch page {
			case 0:
				jsonResponse(w, 200, `{"data":[{"id":"idle","type":"idle","outcome":"succeeded"}],"cursor":{"next":"1"}}`)
			case 6:
				jsonResponse(w, 200, `{"data":[{"id":"start","type":"user"}],"cursor":{}}`)
			default:
				jsonResponse(w, 200, fmt.Sprintf(`{"data":[{"id":"followup-%d","type":"user"}],"cursor":{"next":"%d"}}`, page, page+1))
			}
		default:
			http.NotFound(w, r)
		}
	})
	synth := NewSynthesizer("/work", nil)
	synth.StartTurn("s", "start", "/work")
	NewStream(NewClient(server.URL, server.Client(), "/work"), synth).reconcile(context.Background(), "s", false)
	if _, active := synth.ActiveTurn("s"); active {
		t.Fatal("missed completion past five shrunken pages")
	}
}
