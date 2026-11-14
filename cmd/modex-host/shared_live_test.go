//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/warmdev/modex/internal/bridge"
)

// Opt in: MODEX_LIVE_SHARED_CODEX=codex go test ./cmd/modex-host -run TestLiveSharedCodex -v.
// Creates only a new temporary-workspace session, never resumes a user's session.
func TestLiveSharedCodex(t *testing.T) {
	binary := os.Getenv("MODEX_LIVE_SHARED_CODEX")
	if binary == "" {
		t.Skip("opt-in real Codex/provider check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	workspace := t.TempDir()
	p, err := bridge.StartProcess(ctx, binary, []string{"app-server", "--listen", "stdio://"}, workspace, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	events := make(chan string, 2048)
	s, err := newSharedBackend(ctx, p, func(method string, _ json.RawMessage) {
		select {
		case events <- method:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("", "modex-live-native-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	socket := filepath.Join(root, "native.sock")
	stop, err := s.listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	raw, err := callNativeSocket(ctx, socket, "thread/start", map[string]any{"cwd": workspace, "approvalPolicy": "on-request", "sandbox": "read-only"})
	if err != nil {
		t.Fatal(err)
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if json.Unmarshal(raw, &thread) != nil || thread.Thread.ID == "" {
		t.Fatal("invalid native thread")
	}
	id := thread.Thread.ID
	if _, err = callNativeSocket(ctx, socket, "turn/start", map[string]any{"threadId": id, "input": []any{map[string]string{"type": "text", "text": "Reply exactly MODEX_SHARED_OK. Do not use tools."}}}); err != nil {
		t.Fatal(err)
	}
	params := mustJSON(map[string]any{"thread_id": id})
	if _, err = bridge.HandleEvents(ctx, workspace, bridge.Command{Method: "threads/resume", Params: params}, s.call, nil); err != nil {
		t.Fatalf("phone resume of native session: %v", err)
	}
	if _, err = callNativeSocket(ctx, socket, "thread/resume", map[string]any{"threadId": id, "excludeTurns": true}); err != nil {
		t.Fatalf("native resume during phone turn: %v", err)
	}
	if _, err = s.call(ctx, "client/attached", map[string]any{"attached": false}, nil); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case event := <-events:
			if event == "turn/completed" {
				goto completed
			}
		case <-ctx.Done():
			t.Fatal("turn did not finish after both views disconnected")
		}
	}
completed:
	page, err := bridge.HandleEvents(ctx, workspace, bridge.Command{Method: "threads/read", Params: mustJSON(map[string]any{"thread_id": id, "include_turns": true, "limit": 1})}, s.call, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(page)
	var history struct {
		Messages []struct {
			Text string `json:"text"`
		} `json:"messages"`
	}
	if json.Unmarshal(encoded, &history) != nil {
		t.Fatal("invalid history")
	}
	found := false
	for _, m := range history.Messages {
		if m.Text == "MODEX_SHARED_OK" {
			found = true
		}
	}
	if !found {
		t.Fatal("completed turn did not contain the expected provider response")
	}
	if _, err = callNativeSocket(ctx, socket, "config/read", map[string]any{"cwd": workspace, "includeLayers": false}); err != nil {
		t.Fatalf("native view could not reconnect: %v", err)
	}
	params = mustJSON(map[string]any{"thread_id": id, "text": "Reply exactly MODEX_PHONE_OK. Do not use tools."})
	if _, err = bridge.HandleEvents(ctx, workspace, bridge.Command{Method: "turn/start", Params: params}, s.call, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = callNativeSocket(ctx, socket, "thread/resume", map[string]any{"threadId": id, "excludeTurns": true}); err != nil {
		t.Fatalf("native reattach during phone turn: %v", err)
	}
	for {
		select {
		case event := <-events:
			if event == "turn/completed" {
				t.Log("native turn -> phone attach -> disconnect -> completion; phone turn -> native reattach -> completion verified")
				return
			}
		case <-ctx.Done():
			t.Fatal("phone turn did not complete")
		}
	}
}
