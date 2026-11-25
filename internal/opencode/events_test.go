package opencode

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type collectedEvent struct {
	method string
	params json.RawMessage
}

type eventCollector struct {
	mu     sync.Mutex
	events []collectedEvent
}

func (c *eventCollector) emit(method string, params json.RawMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, collectedEvent{method, append(json.RawMessage(nil), params...)})
}

func (c *eventCollector) snapshot() []collectedEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]collectedEvent(nil), c.events...)
}

func (c *eventCollector) hasMethod(method string) bool {
	for _, event := range c.snapshot() {
		if event.method == method {
			return true
		}
	}
	return false
}

func eventMethods(events []collectedEvent) []string {
	out := make([]string, len(events))
	for i, event := range events {
		out[i] = event.method
	}
	return out
}

func TestSynthesizerEmitsTextToolReasoningTurn(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "msg_user", "/work")
	for _, event := range []struct{ typ, data string }{
		{"session.execution.started", `{"sessionID":"ses_1"}`},
		{"session.text.started", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0}`},
		{"session.text.delta", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0,"delta":"Hel"}`},
		{"session.text.delta", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0,"delta":"lo"}`},
		{"session.reasoning.delta", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0,"delta":"think"}`},
		{"session.tool.input.started", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","id":"call_1","name":"bash"}`},
		{"session.tool.called", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","id":"call_1","input":{}}`},
		{"session.tool.success", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","id":"call_1","content":[{"type":"text","text":"out"}]}`},
		{"session.text.ended", `{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0,"text":"Hello"}`},
		{"session.execution.succeeded", `{"sessionID":"ses_1"}`},
	} {
		synth.Handle(event.typ, json.RawMessage(event.data))
	}

	events := collector.snapshot()
	want := []string{
		"turn/started",
		"item/started",
		"item/agentMessage/delta",
		"item/agentMessage/delta",
		"item/reasoning/summaryTextDelta",
		"item/started",
		"item/completed",
		"item/completed",
		"turn/completed",
	}
	if got := eventMethods(events); !reflect.DeepEqual(got, want) {
		t.Fatalf("methods=%v want %v", got, want)
	}
	assertJSON(t, events[0].params, `{"threadId":"ses_1","turn":{"id":"msg_user"}}`)
	assertJSON(t, events[1].params, `{"threadId":"ses_1","turnId":"msg_user","item":{"id":"msg_asst","type":"agentMessage"}}`)
	assertJSON(t, events[2].params, `{"threadId":"ses_1","turnId":"msg_user","itemId":"msg_asst","delta":"Hel"}`)
	assertJSON(t, events[3].params, `{"threadId":"ses_1","turnId":"msg_user","itemId":"msg_asst","delta":"lo"}`)
	assertJSON(t, events[4].params, `{"threadId":"ses_1","turnId":"msg_user","itemId":"msg_asst","summaryIndex":0,"delta":"think"}`)
	assertJSON(t, events[5].params, `{"threadId":"ses_1","turnId":"msg_user","item":{"id":"call_1","type":"commandExecution","command":"bash","cwd":"/work","status":"inProgress"}}`)
	assertJSON(t, events[6].params, `{"threadId":"ses_1","turnId":"msg_user","item":{"id":"call_1","type":"commandExecution","command":"bash","cwd":"/work","status":"completed","aggregatedOutput":"out","exitCode":null}}`)
	assertJSON(t, events[7].params, `{"threadId":"ses_1","turnId":"msg_user","item":{"id":"msg_asst","type":"agentMessage","text":"Hello"}}`)

	var completed struct {
		ThreadID string `json:"threadId"`
		Turn     struct {
			ID         string  `json:"id"`
			Status     string  `json:"status"`
			DurationMs float64 `json:"durationMs"`
		} `json:"turn"`
	}
	if json.Unmarshal(events[8].params, &completed) != nil || completed.ThreadID != "ses_1" || completed.Turn.ID != "msg_user" || completed.Turn.Status != "completed" || completed.Turn.DurationMs < 0 {
		t.Fatalf("turn/completed=%s", events[8].params)
	}
}

func TestSynthesizerReconcileCompletesMissedTurn(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "msg_user", "/work")
	created := int64(1)
	completed := int64(2)
	idle := int64(3)
	busy := int64(2)
	session := session{ID: "ses_1", Outcome: "succeeded"}
	session.Time.Created = created
	session.Time.Updated = completed
	session.Time.Busy = &busy
	session.Time.Idle = &idle
	messages := []message{
		{ID: "msg_user", Type: "user", Text: "hi"},
		{ID: "msg_asst", Type: "assistant", Content: []part{{Type: "text", Text: "done"}}},
	}
	messages[0].Time.Created = created
	messages[1].Time.Created = completed
	messages[1].Time.Completed = &completed
	synth.Reconcile(session, messages)

	events := collector.snapshot()
	want := []string{"turn/started", "item/started", "item/agentMessage/delta", "item/completed", "turn/completed"}
	if got := eventMethods(events); !reflect.DeepEqual(got, want) {
		t.Fatalf("methods=%v want %v", got, want)
	}
	assertJSON(t, events[2].params, `{"threadId":"ses_1","turnId":"msg_user","itemId":"msg_asst","delta":"done"}`)
	assertJSON(t, events[3].params, `{"threadId":"ses_1","turnId":"msg_user","item":{"id":"msg_asst","type":"agentMessage","text":"done"}}`)

	// Idempotent: a second reconciliation must not emit duplicates.
	synth.Reconcile(session, messages)
	if len(collector.snapshot()) != len(events) {
		t.Fatal("reconcile emitted duplicate events")
	}
}

func TestSynthesizerAcceptedInterruptFinishesInterrupted(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "msg_user", "/work")
	synth.MarkInterrupted("ses_1")
	synth.Handle("session.idle", json.RawMessage(`{"sessionID":"ses_1"}`))

	events := collector.snapshot()
	if got := eventMethods(events); !reflect.DeepEqual(got, []string{"turn/started", "turn/completed"}) {
		t.Fatalf("methods=%v", got)
	}
	var completed struct {
		Turn struct {
			Status string `json:"status"`
		} `json:"turn"`
	}
	if json.Unmarshal(events[1].params, &completed) != nil || completed.Turn.Status != "interrupted" {
		t.Fatalf("turn/completed=%s", events[1].params)
	}
}

func TestSynthesizerFailureUsesFixedNotice(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "msg_user", "/work")
	synth.Handle("session.execution.failed", json.RawMessage(`{"sessionID":"ses_1","error":{"message":"SECRET provider detail"}}`))

	events := collector.snapshot()
	if got := eventMethods(events); !reflect.DeepEqual(got, []string{"turn/started", "error", "turn/completed"}) {
		t.Fatalf("methods=%v", got)
	}
	assertJSON(t, events[1].params, `{
		"threadId":"ses_1","turnId":"msg_user","willRetry":false,
		"notice":{"category":"turn_failed","message":"OpenCode could not complete this turn. Review the conversation before trying again.","retrying":false},
		"error":{"message":"OpenCode could not complete this turn. Review the conversation before trying again."}
	}`)
	if strings.Contains(string(events[1].params), "SECRET") {
		t.Fatal("failure event leaked the upstream error")
	}
	var completed struct {
		Turn struct {
			Status string `json:"status"`
		} `json:"turn"`
	}
	if json.Unmarshal(events[2].params, &completed) != nil || completed.Turn.Status != "failed" {
		t.Fatalf("turn/completed=%s", events[2].params)
	}
}
