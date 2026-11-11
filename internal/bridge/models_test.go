package bridge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestModelCatalogProjection(t *testing.T) {
	if _, err := DecodeCommand([]byte(`{"id":1,"method":"models/list"}`)); err != nil {
		t.Fatal(err)
	}
	result, err := Handle(context.Background(), "/work", Command{Method: "models/list", Params: json.RawMessage(`{"cursor":"next"}`)}, func(_ context.Context, method string, params any) (json.RawMessage, error) {
		p := params.(map[string]any)
		if method != "model/list" || p["cursor"] != "next" || p["includeHidden"] != false {
			t.Fatalf("%s %v", method, p)
		}
		return json.RawMessage(`{"data":[{"id":"entry","model":"custom","displayName":"Custom","defaultReasoningEffort":"medium","supportedReasoningEfforts":[{"reasoningEffort":"low","description":"Fast"}],"secret":"private"}],"nextCursor":"after"}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "private") || !strings.Contains(string(raw), `"next_cursor":"after"`) || !strings.Contains(string(raw), `"reasoningEffort":"low"`) {
		t.Fatal(string(raw))
	}
}

func TestTurnReasoningEffort(t *testing.T) {
	for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh"} {
		_, err := Handle(context.Background(), "/work", Command{Method: "turn/start", Params: json.RawMessage(`{"thread_id":"one","text":"hello","effort":"` + effort + `"}`)}, func(_ context.Context, method string, params any) (json.RawMessage, error) {
			if method == "turn/start" {
				if params.(map[string]any)["effort"] != effort {
					t.Fatal(params)
				}
				return json.RawMessage(`{}`), nil
			}
			return json.RawMessage(`{"thread":{"id":"one","cwd":"/work"}}`), nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, effort := range []string{`null`, `""`, `"bad\neffort"`, `42`} {
		_, err := Handle(context.Background(), "/work", Command{Method: "turn/start", Params: json.RawMessage(`{"thread_id":"one","text":"hello","effort":` + effort + `}`)}, func(context.Context, string, any) (json.RawMessage, error) {
			t.Fatal("upstream called")
			return nil, nil
		})
		if err == nil {
			t.Fatal(effort)
		}
	}
}
