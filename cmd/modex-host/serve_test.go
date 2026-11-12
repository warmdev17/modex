package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/warmdev/modex/internal/bridge"
)

func TestFakeApprovalServer(t *testing.T) {
	if os.Getenv("MODEX_APPROVAL_CHILD") != "1" {
		return
	}
	decoder, encoder := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	var request struct {
		ID     int    `json:"id"`
		Method string `json:"method"`
	}
	for decoder.Decode(&request) == nil {
		switch request.Method {
		case "initialized":
			continue
		case "initialize":
			_ = encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{}})
		case "config/read":
			_ = encoder.Encode(map[string]any{"id": "approval-1", "method": "item/commandExecution/requestApproval", "params": map[string]any{"threadId": "thread", "turnId": "turn", "itemId": "item", "command": "echo OK", "rawSecret": "never-emit"}})
			var response struct {
				ID     string `json:"id"`
				Result struct {
					Decision string `json:"decision"`
				} `json:"result"`
			}
			if decoder.Decode(&response) != nil || response.ID != "approval-1" || response.Result.Decision != "decline" {
				os.Exit(2)
			}
			_ = encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{"config": map[string]any{"model": "test", "model_provider": "fake"}}})
		}
	}
	os.Exit(0)
}

func TestServeBridgeApprovalRoundTripBeforeRPCResponse(t *testing.T) {
	t.Setenv("MODEX_APPROVAL_CHILD", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p, err := bridge.StartProcess(ctx, os.Args[0], []string{"-test.run=^TestFakeApprovalServer$"}, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	input, clientWrite := io.Pipe()
	clientRead, output := io.Pipe()
	defer input.Close()
	defer clientWrite.Close()
	defer clientRead.Close()
	defer output.Close()
	// Close pipe reads as well as the subprocess if the bounded test expires.
	go func() { <-ctx.Done(); input.CloseWithError(ctx.Err()); output.CloseWithError(ctx.Err()) }()
	done := make(chan error, 1)
	go func() { done <- serveBridge(ctx, p, input, output, "/work", "") }()
	if _, err := io.WriteString(clientWrite, "{\"id\":1,\"method\":\"status\"}\n"); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(clientRead)
	var event struct {
		Event  string `json:"event"`
		Params struct {
			ID      string                     `json:"decision_id"`
			Context map[string]json.RawMessage `json:"context"`
		} `json:"params"`
	}
	if err := decoder.Decode(&event); err != nil {
		t.Fatal(err)
	}
	if event.Event != "decision/required" || event.Params.ID == "" || event.Params.Context["rawSecret"] != nil {
		t.Fatalf("event=%+v", event)
	}
	command := map[string]any{"id": 2, "method": "decision/respond", "params": map[string]any{"decision_id": event.Params.ID, "result": map[string]any{"decision": "decline"}}}
	if err := json.NewEncoder(clientWrite).Encode(command); err != nil {
		t.Fatal(err)
	}
	responses := map[int]json.RawMessage{}
	for len(responses) < 2 {
		var response struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := decoder.Decode(&response); err != nil {
			t.Fatal(err)
		}
		if len(response.Error) > 0 || len(response.Result) == 0 {
			t.Fatalf("response=%+v", response)
		}
		responses[response.ID] = response.Result
	}
	if string(responses[2]) != `{"delivered":true}` {
		t.Fatalf("responses=%v", responses)
	}
	clientWrite.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
