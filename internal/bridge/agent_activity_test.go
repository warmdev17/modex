package bridge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMobileAgentActivityLiveAndHistory(t *testing.T) {
	items := []string{
		`{"type":"subAgentActivity","id":"spawn","kind":"started","agentThreadId":"child","agentPath":"/root/reviewer","private":"SECRET"}`,
		`{"type":"collabAgentToolCall","id":"wait","tool":"wait","status":"completed","senderThreadId":"one","receiverThreadIds":["child","other"],"agentsStates":{"child":{"status":"completed","message":"SECRET"},"other":{"status":"errored"},"unrelated":{"status":"running"}},"prompt":"SECRET"}`,
	}
	call := func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`{"thread":{"id":"one","cwd":"/work","turns":[{"id":"turn","items":[` + strings.Join(items, ",") + `]}]}}`), nil
	}
	result, err := Handle(context.Background(), "/work", Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"one","include_turns":true}`)}, call)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	var history struct {
		Messages []struct {
			Role     string          `json:"role"`
			Turn     string          `json:"turn_id"`
			Activity json.RawMessage `json:"activity_item"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &history); err != nil || len(history.Messages) != len(items) {
		t.Fatalf("unexpected history: %s (%v)", raw, err)
	}
	for i, item := range items {
		event, err := EncodeEvent("item/completed", json.RawMessage(`{"threadId":"one","turnId":"turn","item":`+item+`}`))
		var frame struct {
			Params struct {
				Item json.RawMessage `json:"item"`
			} `json:"params"`
		}
		if err != nil || json.Unmarshal(event, &frame) != nil {
			t.Fatalf("unexpected event: %s (%v)", event, err)
		}
		message := history.Messages[i]
		if message.Role != "agent_activity" || message.Turn != "turn" || string(message.Activity) != string(frame.Params.Item) {
			t.Fatalf("history and live event differ: %s %s", message.Activity, frame.Params.Item)
		}
		if strings.Contains(string(event), "SECRET") || strings.Contains(string(event), "unrelated") {
			t.Fatalf("unexpected fields in projection: %s", event)
		}
	}
	if !strings.Contains(string(raw), `"label":"reviewer"`) || !strings.Contains(string(raw), `"status":"errored"`) {
		t.Fatalf("missing agent label or status: %s", raw)
	}
}

func TestMalformedAgentActivityDoesNotRelayRawFields(t *testing.T) {
	raw, err := EncodeEvent("item/started", json.RawMessage(`{"threadId":"one","item":{"type":"collabAgentToolCall","id":"x","receiverThreadIds":false,"prompt":"SECRET"}}`))
	if err != nil || strings.Contains(string(raw), "SECRET") {
		t.Fatalf("malformed agent item bypassed projection: %s (%v)", raw, err)
	}
}
