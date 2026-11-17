package bridge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMobileTurnNoticesSanitizeErrorsWithoutChangingNativeInput(t *testing.T) {
	for _, tc := range []struct{ info, category string }{
		{`"contextWindowExceeded"`, "context_window_exceeded"},
		{`"usageLimitExceeded"`, "usage_limit_exceeded"},
		{`"unauthorized"`, "unauthorized"},
		{`"modelNotFound"`, "model_not_found"},
		{`{"responseStreamDisconnected":{"httpStatusCode":502}}`, "stream_disconnected"},
		{`{"responseTooManyFailedAttempts":{"httpStatusCode":503}}`, "retry_exhausted"},
		{`{"newSecretVariant":{"secret":"secret-key"}}`, "turn_failed"},
	} {
		for _, retry := range []bool{false, true} {
			raw := json.RawMessage(`{"threadId":"thread","turnId":"turn","willRetry":` + map[bool]string{false: "false", true: "true"}[retry] + `,"error":{"message":"secret-key https://key@example.invalid","additionalDetails":"secret-key","codexErrorInfo":` + tc.info + `}}`)
			original := string(raw)
			encoded, err := EncodeEvent("error", raw)
			if err != nil {
				t.Fatal(err)
			}
			var event struct {
				Params struct {
					ThreadID string `json:"threadId"`
					TurnID   string `json:"turnId"`
					Notice   struct {
						Category string `json:"category"`
						Message  string `json:"message"`
						Retrying bool   `json:"retrying"`
						HTTP     int    `json:"http_status"`
					} `json:"notice"`
				} `json:"params"`
			}
			if json.Unmarshal(encoded, &event) != nil || event.Params.Notice.Category != tc.category || event.Params.Notice.Message == "" || event.Params.Notice.Retrying != retry || event.Params.ThreadID != "thread" || event.Params.TurnID != "turn" {
				t.Fatalf("missing notice %s", encoded)
			}
			if strings.Contains(string(encoded), "secret-key") || strings.Contains(string(encoded), "example.invalid") || string(raw) != original {
				t.Fatalf("privacy/native input changed: %s", encoded)
			}
			if strings.Contains(tc.info, "502") && event.Params.Notice.HTTP != 502 {
				t.Fatal("HTTP status lost")
			}
		}
		raw := json.RawMessage(`{"threadId":"thread","turn":{"id":"turn","status":"failed","error":{"message":"secret-key","additionalDetails":"secret-key","codexErrorInfo":` + tc.info + `}}}`)
		encoded, err := EncodeEvent("turn/completed", raw)
		if err != nil {
			t.Fatal(err)
		}
		var event map[string]any
		_ = json.Unmarshal(encoded, &event)
		turn := event["params"].(map[string]any)["turn"].(map[string]any)
		notice, _ := turn["notice"].(map[string]any)
		if notice["category"] != tc.category || notice["retrying"] != false || strings.Contains(string(encoded), "secret-key") {
			t.Fatalf("unsafe completion: %s", encoded)
		}
	}
}

func TestHistoryProjectsSafeTurnNoticeAndMetadataStatus(t *testing.T) {
	result, err := Handle(context.Background(), "/work", Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"one","include_turns":true}`)}, func(context.Context, string, any) (json.RawMessage, error) {
		return json.RawMessage(`{"thread":{"id":"one","cwd":"/work","status":{"type":"idle"},"turns":[{"id":"turn","status":"failed","error":{"message":"secret-key","additionalDetails":"secret-key","codexErrorInfo":{"responseTooManyFailedAttempts":{"httpStatusCode":503}}},"items":[{"id":"user","type":"userMessage","content":[{"type":"text","text":"hello"}]}]}]}}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	var projection struct {
		Messages []struct {
			Notice struct {
				Category string `json:"category"`
				HTTP     int    `json:"http_status"`
			} `json:"turn_notice"`
		} `json:"messages"`
		Thread struct {
			Status struct {
				Type string `json:"type"`
			} `json:"status"`
		} `json:"thread"`
	}
	_ = json.Unmarshal(raw, &projection)
	if len(projection.Messages) != 1 || projection.Messages[0].Notice.Category != "retry_exhausted" || projection.Messages[0].Notice.HTTP != 503 || projection.Thread.Status.Type != "idle" || strings.Contains(string(raw), "secret-key") {
		t.Fatalf("history: %s", raw)
	}
}

func TestPagedHistoryRetainsAuthoritativeActiveStatus(t *testing.T) {
	result, err := Handle(context.Background(), "/work", Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"one","include_turns":true,"limit":10}`)}, func(_ context.Context, m string, _ any) (json.RawMessage, error) {
		if m == "thread/read" {
			return json.RawMessage(`{"thread":{"id":"one","cwd":"/work","status":{"type":"active","activeFlags":["active"]}}}`), nil
		}
		if m != "thread/turns/list" {
			t.Fatal(m)
		}
		return json.RawMessage(`{"data":[{"id":"old","status":"completed","items":[]}],"nextCursor":null}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fields := result.(map[string]any)
	if fields["active_turn"] != true {
		t.Fatalf("active thread became idle: %v", fields)
	}
}
