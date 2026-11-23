package diagnostic

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStructuredAndLegacySafeTurnGuidance(t *testing.T) {
	for _, tc := range []struct {
		raw, category string
		http          int
	}{
		{`{"codexErrorInfo":{"responseStreamConnectionFailed":{"httpStatusCode":401}},"message":"secret-key"}`, "unauthorized", 401},
		{`{"codexErrorInfo":{"httpConnectionFailed":{"httpStatusCode":403}},"message":"secret-key"}`, "unauthorized", 403},
		{`{"codexErrorInfo":{"responseStreamDisconnected":{"httpStatusCode":429}},"message":"secret-key"}`, "rate_limit_exceeded", 429},
		{`{"codexErrorInfo":{"responseStreamDisconnected":{"httpStatusCode":999}},"message":"secret-key"}`, "stream_disconnected", 0},
		{`{"message":"stream closed before completed: secret-key https://secret.invalid"}`, "stream_disconnected", 0},
		{`{"message":"opaque secret-key https://secret.invalid"}`, "turn_failed", 0},
		{`{"message":"You've hit your usage limit. Try again at 3:00 PM. secret-key"}`, "usage_limit_exceeded", 0},
		{`{"codexErrorInfo":{"unknownSecret":{"httpStatusCode":401}},"message":"secret-key"}`, "turn_failed", 0},
	} {
		notice := Notice(json.RawMessage(tc.raw), false)
		encoded, _ := json.Marshal(notice)
		if notice.Category != tc.category || notice.HTTPStatus != tc.http || strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "http:") {
			t.Fatalf("notice: %s", encoded)
		}
	}
}
