package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSessionStatusAndModeContract(t *testing.T) {
	for _, method := range []string{"threads/mode", "threads/status"} {
		params := `{"workspace":"/work","thread_id":"t"}`
		if method == "threads/mode" {
			params = `{"workspace":"/work","thread_id":"t","mode":"plan","model":"model"}`
		}
		var calls []string
		result, err := Handle(context.Background(), "/work", Command{Method: method, Params: json.RawMessage(params)}, func(_ context.Context, m string, p any) (json.RawMessage, error) {
			calls = append(calls, m)
			switch m {
			case "thread/read":
				return json.RawMessage(`{"thread":{"id":"t","cwd":"/work"}}`), nil
			case "modex/thread/mode", "modex/thread/status":
				return json.RawMessage(`{"session":{"thread":{"id":"t","cwd":"/work"},"model":"model","collaboration_mode":"plan"},"token_usage":null}`), nil
			case "account/rateLimits/read":
				return nil, errors.New("secret")
			}
			t.Fatalf("unexpected call %s", m)
			return nil, nil
		})
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		b, _ := json.Marshal(result)
		if strings.Contains(string(b), "secret") || !strings.Contains(string(b), `"model":"model"`) {
			t.Fatalf("unsafe result: %s", b)
		}
		if strings.Contains(strings.Join(calls, ","), "turn/start") {
			t.Fatal("started turn")
		}
	}
}

func TestSettingsNotificationDoesNotLeak(t *testing.T) {
	raw, err := EncodeEvent("thread/settings/updated", json.RawMessage(`{"threadId":"t","threadSettings":{"model":"m","collaborationMode":{"mode":"plan","settings":{"model":"m","developer_instructions":"SECRET"}},"sandboxPolicy":{"secret":"SECRET"}}}`))
	if err != nil || strings.Contains(string(raw), "SECRET") {
		t.Fatalf("unsafe notification: %s %v", raw, err)
	}
}

func TestStatusValidationAndProjection(t *testing.T) {
	for _, params := range []string{`{"thread_id":"t","mode":"other","model":"m"}`, `{"thread_id":"t","mode":"plan","model":""}`, `{"thread_id":"t","mode":"plan","model":"m","effort":null}`} {
		_, err := Handle(context.Background(), "/work", Command{Method: "threads/mode", Params: json.RawMessage(params)}, func(context.Context, string, any) (json.RawMessage, error) {
			t.Fatal("invalid request reached native")
			return nil, nil
		})
		if err == nil {
			t.Fatalf("accepted %s", params)
		}
	}
	v := safeRateLimits(json.RawMessage(`{"accountId":"SECRET","rateLimits":{"limitId":"codex","primary":{"usedPercent":0,"resetsAt":null},"credits":{"balance":"SECRET"}}}`))
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "SECRET") || !strings.Contains(string(raw), `"usedPercent":0`) {
		t.Fatalf("projection: %s", raw)
	}
	b, err := EncodeEvent("thread/tokenUsage/updated", json.RawMessage(`{"threadId":"t","tokenUsage":{"last":{"totalTokens":0,"secret":"SECRET"},"modelContextWindow":null},"secret":"SECRET"}`))
	if err != nil || strings.Contains(string(b), "SECRET") || !strings.Contains(string(b), `"totalTokens":0`) {
		t.Fatalf("token event %s %v", b, err)
	}
}

func TestUnloadedMetadataRejectsStatusAndMode(t *testing.T) {
	for _, method := range []string{"threads/status", "threads/mode"} {
		params := `{"thread_id":"t"}`
		if method == "threads/mode" {
			params = `{"thread_id":"t","mode":"plan","model":"m"}`
		}
		calls := 0
		_, err := Handle(context.Background(), "/work", Command{Method: method, Params: json.RawMessage(params)}, func(context.Context, string, any) (json.RawMessage, error) {
			calls++
			return json.RawMessage(`{"thread":{"id":"t","cwd":"/work","status":{"type":"notLoaded"}}}`), nil
		})
		if !errors.Is(err, ErrSessionNotOwned) || calls != 1 {
			t.Fatalf("%s: calls=%d err=%v", method, calls, err)
		}
	}
}
