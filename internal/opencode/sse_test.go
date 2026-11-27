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

func TestStreamFiltersEventsBeforeSynthesisAndDeduplication(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "prompt", "/work")
	observed := 0
	stream := NewStream(nil, synth, func(string, json.RawMessage, string) { observed++ })
	stream.Filter = func(_ string, _ json.RawMessage, directory string) bool { return directory == "/work" }
	stream.dispatch(`{"id":"same","type":"session.text.delta","location":{"directory":"/other"},"data":{"sessionID":"ses_1","assistantMessageID":"a","delta":"wrong"}}`)
	stream.dispatch(`{"id":"same","type":"session.text.delta","location":{"directory":"/work"},"data":{"sessionID":"ses_1","assistantMessageID":"a","delta":"right"}}`)
	if observed != 1 || len(collector.snapshot()) != 3 {
		t.Fatalf("filtered event was processed or consumed the id: observed=%d events=%v", observed, eventMethods(collector.snapshot()))
	}
}

func TestStreamDoesNotCloseTurnFromUncorrelatedTerminalEvent(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "new_prompt", "/work")
	stream := NewStream(nil, synth)
	stream.dispatch(ssePayload("evt_previous", "session.execution.succeeded", `{"sessionID":"ses_1"}`))
	if _, active := synth.ActiveTurn("ses_1"); !active {
		t.Fatal("uncorrelated session event closed the new turn")
	}
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
	var terminalDelivered atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/session/ses_1" {
			jsonResponse(w, http.StatusOK, `{"data":{"id":"ses_1","location":{"directory":"/work"}}}`)
			return
		}
		if r.URL.Path == "/api/session/ses_1/message" {
			if terminalDelivered.Load() {
				jsonResponse(w, http.StatusOK, `{"data":[{"id":"idle","type":"idle","outcome":"succeeded"},{"id":"msg_user","type":"user"}]}`)
			} else {
				jsonResponse(w, http.StatusOK, `{"data":[{"id":"msg_user","type":"user"}]}`)
			}
			return
		}
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
	stream := NewStream(client, synth, func(typ string, _ json.RawMessage, _ string) {
		if typ == "session.execution.succeeded" {
			terminalDelivered.Store(true)
		}
	})

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
		{"id":"msg_idle","type":"idle","outcome":"succeeded"},
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

func TestStreamPollingCompletesBothActiveSessions(t *testing.T) {
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		switch r.URL.Path {
		case "/api/session/one":
			jsonResponse(w, 200, `{"data":{"id":"one"}}`)
		case "/api/session/two":
			jsonResponse(w, 200, `{"data":{"id":"two"}}`)
		case "/api/session/one/message", "/api/session/two/message":
			jsonResponse(w, 200, `{"data":[{"id":"idle","type":"idle","outcome":"succeeded"},{"id":"prompt","type":"user"}]}`)
		default:
			http.NotFound(w, r)
		}
	})
	synth := NewSynthesizer("/work", nil)
	synth.StartTurn("one", "prompt", "/work")
	synth.StartTurn("two", "prompt", "/work")
	stream := NewStream(NewClient(server.URL, server.Client(), "/work"), synth)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stream.poll(ctx)
	for _, id := range []string{"one", "two"} {
		if _, active := synth.ActiveTurn(id); active {
			t.Fatalf("session %s remained active", id)
		}
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
			recent := []map[string]any{{"id": "idle", "type": "idle", "outcome": "succeeded"}}
			for i := 1; i < pollMessages; i++ {
				recent = append(recent, map[string]any{"id": fmt.Sprintf("followup-%d", i), "type": "user", "metadata": map[string]string{"modex_turn_id": "start"}})
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
				jsonResponse(w, 200, fmt.Sprintf(`{"data":[{"id":"followup-%d","type":"user","metadata":{"modex_turn_id":"start"}}],"cursor":{"next":"%d"}}`, page, page+1))
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

func TestStreamPollDoesNotExposeFormsFromOtherWorkspace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/ses_1":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"ses_1","location":{"directory":"/other"}}}`)
		case "/api/session/ses_1/form":
			jsonResponse(w, http.StatusOK, `{"data":[{"id":"frm_1","sessionID":"ses_1","fields":[{"key":"name","type":"string","title":"Name","required":true}]}]}`)
		default:
			jsonResponse(w, http.StatusOK, `{"data":[]}`)
		}
	}))
	defer server.Close()
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "prompt", "/work")
	decisions := NewDecisions("/work", collector.emit, synth.TurnID, nil)
	decisions.ReplyForm = func(context.Context, string, string, map[string]string) error { return nil }
	stream := NewStream(NewClient(server.URL, server.Client(), "/work"), synth)
	stream.Forms = decisions
	stream.ScopedPoll = true
	stream.reconcile(context.Background(), "ses_1", true)
	if collector.hasMethod("decision/required") {
		t.Fatal("exposed a form from another workspace")
	}
}

func TestManagedPollRequiresTurnAnchorBeforeExposingPendingForm(t *testing.T) {
	for _, tc := range []struct {
		name, messages string
		want           bool
	}{
		{"other turn", `{"data":[{"id":"old","type":"user"}]}`, false},
		{"current turn", `{"data":[{"id":"prompt","type":"user"}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/session/ses_1":
					jsonResponse(w, http.StatusOK, `{"data":{"id":"ses_1","location":{"directory":"/work"}}}`)
				case "/api/session/ses_1/message":
					jsonResponse(w, http.StatusOK, tc.messages)
				case "/api/session/ses_1/form":
					jsonResponse(w, http.StatusOK, `{"data":[{"id":"frm_new","sessionID":"ses_1","fields":[{"key":"name","type":"string","title":"Name","required":true}]}]}`)
				default:
					jsonResponse(w, http.StatusOK, `{"data":[]}`)
				}
			}))
			defer server.Close()
			collector := &eventCollector{}
			synth := NewSynthesizer("/work", collector.emit)
			synth.StartTurn("ses_1", "prompt", "/work")
			decisions := NewDecisions("/work", collector.emit, synth.TurnID, nil)
			decisions.ReplyForm = func(context.Context, string, string, map[string]string) error { return nil }
			stream := NewStream(NewClient(server.URL, server.Client(), "/work"), synth)
			stream.Forms = decisions
			stream.ScopedPoll = true
			stream.reconcile(context.Background(), "ses_1", true)
			if got := collector.hasMethod("decision/required"); got != tc.want {
				t.Fatalf("decision exposed=%v want %v", got, tc.want)
			}
		})
	}
}

func TestManagedPollRejectsPermissionSnapshotAfterTurnReplacement(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "old", "/work")
	decisions := NewDecisions("/work", collector.emit, synth.TurnID, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/ses_1":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"ses_1","location":{"directory":"/work"}}}`)
		case "/api/session/ses_1/message":
			jsonResponse(w, http.StatusOK, `{"data":[{"id":"old","type":"user"}]}`)
		case "/api/session/ses_1/permission":
			synth.StartTurn("ses_1", "new", "/work")
			jsonResponse(w, http.StatusOK, `{"data":[{"id":"stale","sessionID":"ses_1","action":"write"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	stream := NewStream(NewClient(server.URL, server.Client(), "/work"), synth)
	stream.ScopedPoll = true
	stream.OnScopedPermissions = decisions.reconcileTurn
	stream.reconcile(context.Background(), "ses_1", true)
	if got := decisions.Pending(); len(got) != 0 {
		t.Fatalf("old snapshot entered replacement turn: %+v", got)
	}
}

func TestManagedPollFinishesTurnAtPermissionCap(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "prompt", "/work")
	decisions := NewDecisions("/work", collector.emit, synth.TurnID, nil)
	decisions.OnUnsupportedInput = synth.UnsupportedInput
	for i := range 128 {
		decisions.add(permissionRequest{ID: fmt.Sprint(i), SessionID: "other", Action: "write"}, "/work")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session/ses_1":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"ses_1","location":{"directory":"/work"}}}`)
		case "/api/session/ses_1/message":
			jsonResponse(w, http.StatusOK, `{"data":[{"id":"prompt","type":"user"}]}`)
		case "/api/session/ses_1/permission":
			jsonResponse(w, http.StatusOK, `{"data":[{"id":"new","sessionID":"ses_1","action":"write"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	stream := NewStream(NewClient(server.URL, server.Client(), "/work"), synth)
	stream.ScopedPoll = true
	stream.OnScopedPermissions = decisions.reconcileTurn
	done := make(chan struct{})
	go func() {
		stream.reconcile(context.Background(), "ses_1", true)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reconciliation deadlocked at the permission cap")
	}
	if _, active := synth.ActiveTurn("ses_1"); active || !collector.hasMethod("turn/completed") {
		t.Fatal("the capped turn was not completed")
	}
}
