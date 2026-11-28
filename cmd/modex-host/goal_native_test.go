//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/warmdev/modex/internal/bridge"
)

// No credentials or inference: verify native goal persistence and mobile projection.
func TestNativeGoalManagement(t *testing.T) {
	binary := os.Getenv("MODEX_NATIVE_CODEX")
	if binary == "" {
		t.Skip("opt-in isolated native Codex goal check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	home := filepath.Join(root, "codex")
	for _, dir := range []string{workspace, home} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	p, err := bridge.StartProcess(ctx, binary, []string{"app-server", "--listen", "stdio://"}, workspace,
		[]string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "CODEX_HOME=" + home})
	if err != nil {
		t.Fatal(err)
	}
	defer p.CloseIdle()
	var turns atomic.Int32
	events := make(chan string, 16)
	backend, err := newSharedBackend(ctx, p, func(method string, params json.RawMessage) {
		if method == "turn/started" {
			turns.Add(1)
		}
		if method == "thread/goal/updated" || method == "thread/goal/cleared" {
			if _, err := bridge.EncodeEvent(method, params); err == nil {
				events <- method
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := backend.call(ctx, "thread/start", map[string]any{"cwd": workspace, "sandbox": "read-only", "approvalPolicy": "on-request"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var start struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if json.Unmarshal(raw, &start) != nil || start.Thread.ID == "" {
		t.Fatal("missing native thread")
	}
	id := start.Thread.ID
	if _, err = backend.call(ctx, "thread/inject_items", map[string]any{"threadId": id, "items": []any{map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "Synthetic goal storage fixture; never run inference."}}}}}, nil); err != nil {
		t.Fatal(err)
	}
	call := func(method string, extra map[string]any) map[string]json.RawMessage {
		t.Helper()
		params := map[string]any{"thread_id": id, "workspace": workspace}
		for key, value := range extra {
			params[key] = value
		}
		data, _ := json.Marshal(params)
		result, err := bridge.HandleEvents(ctx, workspace, bridge.Command{ID: 1, Method: method, Params: data}, backend.call, nil)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		encoded, _ := json.Marshal(result)
		var projected map[string]json.RawMessage
		if json.Unmarshal(encoded, &projected) != nil {
			t.Fatal("invalid mobile response")
		}
		return projected
	}
	checkGoal := func(result map[string]json.RawMessage, objective string) {
		t.Helper()
		var goal struct {
			ID        string `json:"thread_id"`
			Objective string `json:"objective"`
			Status    string `json:"status"`
			Budget    int64  `json:"token_budget"`
		}
		if json.Unmarshal(result["goal"], &goal) != nil || goal.ID != id || goal.Objective != objective || goal.Status != "paused" || goal.Budget != 1000 {
			t.Fatalf("unexpected projected goal: %s", result["goal"])
		}
	}
	if string(call("threads/goal/get", nil)["goal"]) != "null" {
		t.Fatal("new thread already has goal")
	}
	checkGoal(call("threads/goal/set", map[string]any{"objective": "Synthetic paused goal", "status": "paused", "token_budget": 1000}), "Synthetic paused goal")
	if _, err = backend.call(ctx, "thread/goal/set", map[string]any{"threadId": id, "objective": "Changed from native CLI", "status": "paused", "origin": "user"}, nil); err != nil {
		t.Fatal(err)
	}
	checkGoal(call("threads/goal/get", nil), "Changed from native CLI")
	if _, err = backend.call(ctx, "thread/unsubscribe", map[string]any{"threadId": id}, nil); err != nil {
		t.Fatal(err)
	}
	checkGoal(call("threads/goal/get", nil), "Changed from native CLI")
	checkGoal(call("threads/goal/set", map[string]any{"objective": "Changed while unloaded", "status": "paused"}), "Changed while unloaded")
	call("threads/resume", nil)
	checkGoal(call("threads/goal/get", nil), "Changed while unloaded")
	if string(call("threads/goal/clear", nil)["cleared"]) != "true" {
		t.Fatal("goal was not cleared")
	}
	if string(call("threads/goal/get", nil)["goal"]) != "null" {
		t.Fatal("goal persisted after clear")
	}
	updates := 0
	cleared := false
	for !cleared {
		select {
		case got := <-events:
			if got == "thread/goal/updated" {
				updates++
			}
			cleared = got == "thread/goal/cleared"
		case <-ctx.Done():
			t.Fatal("missing native goal cleared event")
		}
	}
	if updates < 3 {
		t.Fatalf("missing native goal updates: %d", updates)
	}
	if turns.Load() != 0 {
		t.Fatal("goal reads/paused changes started inference")
	}
	t.Log("native paused goal create/read/external update/clear and mobile events passed without inference")
}
