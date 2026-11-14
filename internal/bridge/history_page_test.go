package bridge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestHistoryPageUsesNativeTurnsAndReturnsChronologicalMessages(t *testing.T) {
	calls := []string{}
	caller := func(_ context.Context, method string, params any) (json.RawMessage, error) {
		calls = append(calls, method)
		p := params.(map[string]any)
		if method == "thread/read" {
			if p["includeTurns"] != false {
				t.Fatal("paged history loaded all turns")
			}
			return json.RawMessage(`{"thread":{"id":"one","cwd":"/work"}}`), nil
		}
		if method != "thread/turns/list" || p["limit"] != 10 || p["itemsView"] != "full" || p["sortDirection"] != "desc" {
			t.Fatalf("unexpected pagination %s %v", method, p)
		}
		return json.RawMessage(`{"data":[{"id":"new","status":"completed","items":[{"id":"b","type":"agentMessage","text":"new"}]},{"id":"old","status":"completed","items":[{"id":"a","type":"agentMessage","text":"old"}]}],"nextCursor":"opaque"}`), nil
	}
	result, err := Handle(context.Background(), "/work", Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"one","include_turns":true,"limit":10}`)}, caller)
	if err != nil {
		t.Fatal(err)
	}
	data := result.(map[string]any)
	messages := data["messages"].([]map[string]any)
	if len(messages) != 2 || messages[0]["text"] != "old" || messages[1]["text"] != "new" || data["next_cursor"] != "opaque" || data["history_revision"] == "" {
		t.Fatalf("bad page metadata or ordering")
	}
	if len(calls) != 2 {
		t.Fatal(calls)
	}
}

func TestHistoryPageRejectsInvalidParametersAndWrongWorkspace(t *testing.T) {
	for _, params := range []string{`"limit":0`, `"limit":51`, `"limit":null`, `"limit":1.5`, `"limit":10,"cursor":123`, `"cursor":"x"`} {
		_, err := Handle(context.Background(), "/work", Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"one","include_turns":true,` + params + `}`)}, func(context.Context, string, any) (json.RawMessage, error) {
			t.Fatal("invalid params reached provider")
			return nil, nil
		})
		if err == nil {
			t.Fatal(params)
		}
	}
	calls := 0
	_, err := Handle(context.Background(), "/work", Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"one","include_turns":true,"limit":10}`)}, func(_ context.Context, m string, _ any) (json.RawMessage, error) {
		calls++
		return json.RawMessage(`{"thread":{"id":"one","cwd":"/elsewhere"}}`), nil
	})
	if err == nil || !strings.Contains(err.Error(), "outside workspace") || calls != 1 {
		t.Fatalf("workspace guard: %v %d", err, calls)
	}
}

func TestHistoryPageCursorContinuationAndRepeatedCursor(t *testing.T) {
	for _, next := range []string{"", "same"} {
		_, cursor, _, err := fetchHistoryPage(func(method string, params any) (json.RawMessage, error) {
			if method == "thread/read" {
				return json.RawMessage(`{"thread":{"id":"one","cwd":"/work"}}`), nil
			}
			if params.(map[string]any)["cursor"] != "same" {
				t.Fatal("continuation cursor lost")
			}
			return json.RawMessage(`{"data":[],"nextCursor":"` + next + `"}`), nil
		}, "/work", "one", 10, "same")
		if next == "same" && err == nil {
			t.Fatal("repeated cursor accepted")
		}
		if next == "" && (err != nil || cursor != "") {
			t.Fatalf("empty last page: %v", err)
		}
	}
}
