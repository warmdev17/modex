package opencode

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/warmdev/modex/internal/bridge"
)

func TestAttachmentOnlyHistoryProjectsLocalFiles(t *testing.T) {
	var messages []message
	if err := json.Unmarshal([]byte(`[{"id":"u","type":"user","files":[
		{"name":"notes #1.txt","mime":"text/plain","data":"omitted","source":{"type":"uri","uri":"file:///tmp/notes%20%231.txt"}},
		{"name":"diagram.png","mime":"image/png","data":"omitted","source":{"type":"uri","uri":"file:///tmp/diagram.png"}},
		{"name":"remote.png","mime":"image/png","source":{"type":"uri","uri":"file://remote/tmp/remote.png"}},
		{"name":"remote.txt","mime":"text/plain","source":{"type":"uri","uri":"https://example.test/remote.txt"}},
		{"name":"selected.txt","mime":"text/plain","source":{"type":"uri","uri":"file:///tmp/selected.txt?start=2&end=4"}},
		{"name":"control.txt","mime":"text/plain","source":{"type":"uri","uri":"file:///tmp/control%00.txt"}},
		{"name":"inline.png","mime":"image/png","source":{"type":"inline"}}
	]}]`), &messages); err != nil {
		t.Fatal(err)
	}
	entries, _, _ := projectMessages("s", "/work", messages)
	assertJSON(t, entries, `[{"id":"u","role":"user","text":"","display_text":"","attachments":[
		{"path":"/tmp/notes #1.txt","name":"notes #1.txt","kind":"file"},
		{"path":"/tmp/diagram.png","name":"diagram.png","kind":"image"}
	]}]`)
}

func TestConfirmedSteerProjectsRegularFile(t *testing.T) {
	var event json.RawMessage
	_, err := HandleEvents(context.Background(), "/work", bridge.Command{
		Method: "turn/steer", Params: json.RawMessage(`{"thread_id":"s","turn_id":"t","text":"","attachments":[{"path":"/tmp/notes #1.txt","kind":"file"}]}`),
	}, func(_ context.Context, method string, _ any, _ func(string, json.RawMessage)) (json.RawMessage, error) {
		switch method {
		case callSessionRead:
			return json.RawMessage(`{"id":"s","location":{"directory":"/work"}}`), nil
		case "turn/steer":
			return json.RawMessage(`{"turn":{"id":"t"},"userMessageId":"u"}`), nil
		default:
			t.Fatalf("unexpected call %s", method)
			return nil, nil
		}
	}, func(method string, raw json.RawMessage) {
		if method == "item/completed" {
			event = raw
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, event, `{"threadId":"s","turnId":"t","item":{"id":"u","type":"userMessage","content":[{"type":"text","text":""}],"attachments":[{"path":"/tmp/notes #1.txt","name":"notes #1.txt","kind":"file"}]}}`)
}
