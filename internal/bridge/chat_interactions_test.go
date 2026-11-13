package bridge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestInterruptValidatesWorkspaceAndMapsExactTurn(t *testing.T) {
	for _, outside := range []bool{false, true} {
		calls := []string{}
		_, err := Handle(context.Background(), "/work", Command{Method: "turn/interrupt", Params: json.RawMessage(`{"thread_id":"one","turn_id":"turn-one"}`)}, func(_ context.Context, method string, params any) (json.RawMessage, error) {
			calls = append(calls, method)
			if method == "thread/read" {
				cwd := "/work"
				if outside {
					cwd = "/other"
				}
				return json.Marshal(map[string]any{"thread": Thread{ID: "one", Cwd: cwd}})
			}
			p := params.(map[string]any)
			if method != "turn/interrupt" || p["threadId"] != "one" || p["turnId"] != "turn-one" || len(p) != 2 {
				t.Fatalf("unexpected dispatch %s %#v", method, p)
			}
			return json.RawMessage(`{}`), nil
		})
		if outside {
			if err == nil || len(calls) != 1 {
				t.Fatalf("outside workspace dispatched: %v %v", calls, err)
			}
		} else if err != nil || strings.Join(calls, ",") != "thread/read,turn/interrupt" {
			t.Fatalf("interrupt: %v %v", calls, err)
		}
	}
	for _, raw := range []string{`{}`, `{"thread_id":"one"}`, `{"thread_id":"one","turn_id":null}`, `{"thread_id":"one","turn_id":""}`, `{"thread_id":"one","turn_id":"x","force":true}`} {
		_, err := Handle(context.Background(), "/work", Command{Method: "turn/interrupt", Params: json.RawMessage(raw)}, func(context.Context, string, any) (json.RawMessage, error) {
			t.Fatal("invalid input reached Codex")
			return nil, nil
		})
		if err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if _, err := DecodeCommand([]byte(`{"id":1,"method":"turn/interrupt","params":{"thread_id":"one","turn_id":"t"}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryPreservesReasoningSummaryAndTurnDuration(t *testing.T) {
	result, err := Handle(context.Background(), "/work", Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"one","include_turns":true}`)}, func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`{"thread":{"id":"one","cwd":"/work","turns":[{"id":"turn-one","status":"completed","durationMs":1450,"items":[{"id":"u","type":"userMessage","content":[{"type":"text","text":"hello"}]},{"id":"r","type":"reasoning","summary":["Visible summary"],"content":["PRIVATE_RAW_REASONING"]},{"id":"a","type":"agentMessage","text":"answer"}]}]}}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	messages := result.(map[string]any)["messages"].([]map[string]any)
	if len(messages) != 3 || messages[1]["role"] != "reasoning" || messages[1]["text"] != "Visible summary" || messages[0]["turn_id"] != "turn-one" {
		t.Fatalf("missing history: %#v", messages)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), `"duration_ms":1450`) || strings.Contains(string(raw), "PRIVATE_RAW_REASONING") {
		t.Fatalf("unsafe/incomplete projection: %s", raw)
	}
}

func TestActiveTimingDoesNotFreezeAtReportedDuration(t *testing.T) {
	duration := int64(500)
	m := map[string]any{}
	(turnMetadata{ID: "t", Status: "inProgress", DurationMs: &duration}).project(m)
	if _, exists := m["duration_ms"]; exists {
		t.Fatal("active timer projected a final duration")
	}
}
