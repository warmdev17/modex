package opencode

import (
	"encoding/json"
	"reflect"
	"testing"
)

// OpenCode 2.4 stores the tool id, name and state directly on the content part.
const nativeToolHistory = `[
  {"id":"msg_prompt","type":"user","text":"fixture"},
  {"id":"msg_assistant","type":"assistant","content":[
    {"type":"tool","id":"call_done","name":"bash","executed":true,"state":{"status":"completed","content":[{"type":"text","text":"fixture output"}]}},
    {"type":"tool","id":"call_error","name":"read","executed":true,"state":{"status":"error","content":[{"type":"text","text":"fixture failure"}]}},
    {"type":"tool","id":"call_pending","name":"bash","executed":true,"state":{"status":"running"}}
  ]}
]`

func TestNativeToolHistoryPreservesStateAndCallIDs(t *testing.T) {
	var messages []message
	if err := json.Unmarshal([]byte(nativeToolHistory), &messages); err != nil {
		t.Fatal(err)
	}
	entries, _, _ := projectMessages("ses_1", "/work", messages)
	assertJSON(t, entries[1:], `[
		{"id":"call_done","role":"command","command":"bash","cwd":"/work","status":"completed","text":"fixture output","exit_code":null},
		{"id":"call_error","role":"command","command":"read","cwd":"/work","status":"failed","text":"fixture failure","exit_code":null},
		{"id":"call_pending","role":"command","command":"bash","cwd":"/work","status":"inProgress","text":"","exit_code":null}
	]`)
}

func TestNativeToolPollCompletesExistingLiveRow(t *testing.T) {
	var messages []message
	if err := json.Unmarshal([]byte(nativeToolHistory), &messages); err != nil {
		t.Fatal(err)
	}
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	synth.StartTurn("ses_1", "msg_prompt", "/work")
	synth.Handle("session.tool.input.started", json.RawMessage(`{"sessionID":"ses_1","id":"call_done","name":"bash","assistantMessageID":"msg_assistant"}`))
	messages[1].Content = messages[1].Content[:1]
	synth.Reconcile(session{ID: "ses_1"}, messages)
	events := collector.snapshot()
	want := []string{"turn/started", "item/started", "item/completed"}
	if got := eventMethods(events); !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
	assertJSON(t, events[2].params, `{"threadId":"ses_1","turnId":"msg_prompt","item":{"id":"call_done","type":"commandExecution","command":"bash","cwd":"/work","status":"completed","aggregatedOutput":"fixture output","exitCode":null}}`)
}
