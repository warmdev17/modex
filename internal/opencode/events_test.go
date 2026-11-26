package opencode

import (
	"context"
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
		"item/started",
		"item/reasoning/summaryTextDelta",
		"item/started",
		"item/completed",
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
	assertJSON(t, events[4].params, `{"threadId":"ses_1","turnId":"msg_user","item":{"id":"msg_asst/r1","type":"reasoning","summary":[]}}`)
	assertJSON(t, events[5].params, `{"threadId":"ses_1","turnId":"msg_user","itemId":"msg_asst/r1","summaryIndex":0,"delta":"think"}`)
	assertJSON(t, events[6].params, `{"threadId":"ses_1","turnId":"msg_user","item":{"id":"call_1","type":"commandExecution","command":"bash","cwd":"/work","status":"inProgress"}}`)
	assertJSON(t, events[7].params, `{"threadId":"ses_1","turnId":"msg_user","item":{"id":"call_1","type":"commandExecution","command":"bash","cwd":"/work","status":"completed","aggregatedOutput":"out","exitCode":null}}`)
	assertJSON(t, events[8].params, `{"threadId":"ses_1","turnId":"msg_user","item":{"id":"msg_asst/r1","type":"reasoning","summary":["think"]}}`)
	assertJSON(t, events[9].params, `{"threadId":"ses_1","turnId":"msg_user","item":{"id":"msg_asst","type":"agentMessage","text":"Hello"}}`)

	var completed struct {
		ThreadID string `json:"threadId"`
		Turn     struct {
			ID         string  `json:"id"`
			Status     string  `json:"status"`
			DurationMs float64 `json:"durationMs"`
		} `json:"turn"`
	}
	if json.Unmarshal(events[10].params, &completed) != nil || completed.ThreadID != "ses_1" || completed.Turn.ID != "msg_user" || completed.Turn.Status != "completed" || completed.Turn.DurationMs < 0 {
		t.Fatalf("turn/completed=%s", events[10].params)
	}
}

// TestReasoningItemIDIsDistinctFromAgentMessage guards the Android collision:
// reasoning deltas/completions must use <msgID>/r<N>, never the assistant id.
func TestReasoningItemIDIsDistinctFromAgentMessage(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "msg_user", "/work")
	synth.Handle("session.text.started", json.RawMessage(`{"sessionID":"ses_1","assistantMessageID":"msg_asst"}`))
	synth.Handle("session.text.delta", json.RawMessage(`{"sessionID":"ses_1","assistantMessageID":"msg_asst","delta":"answer"}`))
	synth.Handle("session.reasoning.delta", json.RawMessage(`{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0,"delta":"think"}`))
	synth.Handle("session.reasoning.ended", json.RawMessage(`{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0,"text":"think"}`))
	synth.Handle("session.execution.succeeded", json.RawMessage(`{"sessionID":"ses_1"}`))

	var reasoningDelta, reasoningDone map[string]any
	var agentStarted bool
	for _, event := range collector.snapshot() {
		switch event.method {
		case "item/agentMessage/delta":
			agentStarted = true
		case "item/reasoning/summaryTextDelta":
			_ = json.Unmarshal(event.params, &reasoningDelta)
		case "item/completed":
			var params struct {
				Item map[string]any `json:"item"`
			}
			_ = json.Unmarshal(event.params, &params)
			if params.Item["type"] == "reasoning" {
				reasoningDone = params.Item
			}
		}
	}
	if !agentStarted {
		t.Fatal("assistant text item was not emitted")
	}
	if reasoningDelta == nil {
		t.Fatal("reasoning delta was not emitted")
	}
	reasoningID, _ := reasoningDelta["itemId"].(string)
	if reasoningID == "" || reasoningID == "msg_asst" {
		t.Fatalf("reasoning itemId=%q must differ from the agentMessage id", reasoningID)
	}
	if reasoningID != "msg_asst/r1" {
		t.Fatalf("reasoning itemId=%q want msg_asst/r1", reasoningID)
	}
	if reasoningDone == nil {
		t.Fatal("reasoning item/completed was not emitted")
	}
	if reasoningDone["id"] != reasoningID || reasoningDone["type"] != "reasoning" {
		t.Fatalf("reasoning completion=%v", reasoningDone)
	}
	summary, _ := reasoningDone["summary"].([]any)
	if len(summary) != 1 || summary[0] != "think" {
		t.Fatalf("reasoning summary=%v", reasoningDone["summary"])
	}
}

func TestTurnCompletionResolvesPendingDecisions(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	store := NewDecisions("/work", collector.emit, synth.TurnID, func(context.Context, string, string, string) error { return nil })
	synth.OnTurnComplete = store.Resolve
	synth.StartTurn("ses_1", "msg_user", "/work")
	store.Handle("permission.asked", json.RawMessage(`{"id":"per_1","sessionID":"ses_1","action":"bash","resources":["one"]}`), "/work")
	if len(store.Pending()) != 1 {
		t.Fatalf("pending=%v", store.Pending())
	}
	synth.Handle("session.execution.succeeded", json.RawMessage(`{"sessionID":"ses_1"}`))
	if len(store.Pending()) != 0 {
		t.Fatalf("turn completion left stale decisions: %v", store.Pending())
	}
	if !collector.hasMethod("decision/resolved") {
		t.Fatal("turn completion did not emit decision/resolved")
	}
}

func TestActiveSessionStopsAfterTerminalTurn(t *testing.T) {
	synth := NewSynthesizer("/work", nil)
	synth.StartTurn("ses_1", "msg_user", "/work")
	if id, active := synth.ActiveSession(); id != "ses_1" || !active {
		t.Fatalf("active=%q,%v", id, active)
	}
	synth.Handle("session.execution.succeeded", json.RawMessage(`{"sessionID":"ses_1"}`))
	if id, active := synth.ActiveSession(); id != "" || active {
		t.Fatalf("terminal turn still polls: %q,%v", id, active)
	}
}

func TestReasoningCompletionDeduped(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "msg_user", "/work")
	delta := json.RawMessage(`{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0,"delta":"think"}`)
	ended := json.RawMessage(`{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0,"text":"think"}`)
	synth.Handle("session.reasoning.delta", delta)
	synth.Handle("session.reasoning.ended", ended)
	synth.Handle("session.reasoning.ended", ended)
	synth.Handle("session.execution.succeeded", json.RawMessage(`{"sessionID":"ses_1"}`))

	completed := 0
	for _, event := range collector.snapshot() {
		if event.method != "item/completed" {
			continue
		}
		var params struct {
			Item map[string]any `json:"item"`
		}
		_ = json.Unmarshal(event.params, &params)
		if params.Item["type"] == "reasoning" {
			completed++
		}
	}
	if completed != 1 {
		t.Fatalf("reasoning completions=%d", completed)
	}
}

func TestReasoningNonZeroOrdinalSection(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "msg_user", "/work")
	synth.Handle("session.reasoning.delta", json.RawMessage(`{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":1,"delta":"second"}`))
	synth.Handle("session.reasoning.ended", json.RawMessage(`{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":1,"text":"second"}`))

	var delta, done map[string]any
	for _, event := range collector.snapshot() {
		switch event.method {
		case "item/reasoning/summaryTextDelta":
			_ = json.Unmarshal(event.params, &delta)
		case "item/completed":
			var params struct {
				Item map[string]any `json:"item"`
			}
			_ = json.Unmarshal(event.params, &params)
			if params.Item["type"] == "reasoning" {
				done = params.Item
			}
		}
	}
	// Each OpenCode reasoning part maps to one history-aligned item, so the
	// item id carries the 1-based part index and summaryIndex stays 0.
	if delta == nil || delta["itemId"] != "msg_asst/r2" || delta["summaryIndex"] != float64(0) {
		t.Fatalf("reasoning delta=%v", delta)
	}
	summary, _ := done["summary"].([]any)
	if done == nil || done["id"] != "msg_asst/r2" || len(summary) != 1 || summary[0] != "second" {
		t.Fatalf("reasoning completion=%v", done)
	}
}

func TestEmptyReasoningSectionSuppressed(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "msg_user", "/work")
	synth.Handle("session.reasoning.started", json.RawMessage(`{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0}`))
	synth.Handle("session.reasoning.ended", json.RawMessage(`{"sessionID":"ses_1","assistantMessageID":"msg_asst","ordinal":0,"text":""}`))
	synth.Handle("session.execution.succeeded", json.RawMessage(`{"sessionID":"ses_1"}`))
	for _, event := range collector.snapshot() {
		if event.method != "item/started" && event.method != "item/completed" {
			continue
		}
		var params struct {
			Item map[string]any `json:"item"`
		}
		_ = json.Unmarshal(event.params, &params)
		if params.Item["type"] == "reasoning" {
			t.Fatalf("empty reasoning emitted an item: %s", event.params)
		}
	}
}

func TestToolOnlyTurnSuppressesEmptyAgentMessage(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "msg_user", "/work")
	synth.Handle("session.tool.input.started", json.RawMessage(`{"sessionID":"ses_1","assistantMessageID":"msg_asst","id":"call_1","name":"bash"}`))
	synth.Handle("session.tool.success", json.RawMessage(`{"sessionID":"ses_1","assistantMessageID":"msg_asst","id":"call_1","content":[{"type":"text","text":"out"}]}`))
	synth.Handle("session.execution.succeeded", json.RawMessage(`{"sessionID":"ses_1"}`))

	for _, event := range collector.snapshot() {
		if event.method != "item/started" && event.method != "item/completed" {
			continue
		}
		var params struct {
			Item map[string]any `json:"item"`
		}
		_ = json.Unmarshal(event.params, &params)
		if params.Item["type"] == "agentMessage" {
			t.Fatalf("tool-only turn emitted an empty agentMessage item: %s", event.params)
		}
	}
	if !collector.hasMethod("turn/completed") {
		t.Fatal("tool-only turn did not complete")
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
	synth.MarkInterrupted("ses_1", "msg_user")
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
