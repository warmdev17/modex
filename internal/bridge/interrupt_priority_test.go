package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

func TestStopBypassesPendingStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	readingLimits := make(chan struct{})
	interrupted := make(chan struct{})
	reader, writer := io.Pipe()
	var output bytes.Buffer
	done := make(chan error, 1)
	call := func(ctx context.Context, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error) {
		switch method {
		case "thread/read":
			return json.RawMessage(`{"thread":{"id":"one","cwd":"/work"}}`), nil
		case "modex/thread/status":
			return json.RawMessage(`{"session":{"thread":{"id":"one"},"model":"fixture"},"token_usage":null}`), nil
		case "account/rateLimits/read":
			close(readingLimits)
			select {
			case <-interrupted:
				return json.RawMessage(`{"rateLimits":null}`), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		case "turn/interrupt":
			values := params.(map[string]any)
			if values["threadId"] != "one" || values["turnId"] != "active" {
				return nil, io.ErrUnexpectedEOF
			}
			close(interrupted)
			emit("turn/completed", json.RawMessage(`{"threadId":"one","turn":{"id":"active","status":"interrupted"}}`))
			return json.RawMessage(`{}`), nil
		default:
			return nil, io.ErrUnexpectedEOF
		}
	}
	go func() { done <- ServeEvents(ctx, reader, &output, "/work", call) }()
	_, err := io.WriteString(writer, "{\"id\":1,\"method\":\"threads/status\",\"params\":{\"workspace\":\"/work\",\"thread_id\":\"one\"}}\n")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-readingLimits:
	case <-ctx.Done():
		t.Fatal("status never reached rate-limit read")
	}
	_, err = io.WriteString(writer, "{\"id\":2,\"method\":\"turn/interrupt\",\"params\":{\"workspace\":\"/work\",\"thread_id\":\"one\",\"turn_id\":\"active\"}}\n")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-interrupted:
	case <-ctx.Done():
	}
	timedOut := ctx.Err() != nil
	_ = writer.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if timedOut {
		t.Fatal("Stop waited behind the pending status request")
	}
	if !bytes.Contains(output.Bytes(), []byte(`"status":"interrupted"`)) {
		t.Fatalf("missing authoritative interrupted event: %s", &output)
	}
}
