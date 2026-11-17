//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/warmdev/modex/internal/bridge"
)

// Opt-in native storage check: an isolated Codex home, synthetic history, no inference.
func TestNativeConversationLifecycle(t *testing.T) {
	binary := os.Getenv("MODEX_NATIVE_CODEX")
	if binary == "" {
		t.Skip("opt-in isolated native Codex lifecycle check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	root := t.TempDir()
	home := filepath.Join(root, "codex")
	workspace := filepath.Join(root, "workspace")
	for _, dir := range []string{home, workspace} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model = \"gpt-6.1-sol\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "CODEX_HOME=" + home}
	p, err := bridge.StartProcess(ctx, binary, []string{"app-server", "--listen", "stdio://"}, workspace, env)
	if err != nil {
		t.Fatal(err)
	}
	defer p.CloseIdle()
	var turns atomic.Int32
	backend, err := newSharedBackend(ctx, p, func(method string, _ json.RawMessage) {
		if method == "turn/started" {
			turns.Add(1)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	native := func(method string, params any) json.RawMessage {
		t.Helper()
		raw, err := backend.call(ctx, method, params, nil)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		return raw
	}
	mobile := func(method string, params map[string]any) (json.RawMessage, error) {
		data, _ := json.Marshal(params)
		result, err := bridge.HandleEvents(ctx, workspace, bridge.Command{ID: 1, Method: method, Params: data}, backend.call, nil)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	}
	check := func(method string, params map[string]any) json.RawMessage {
		t.Helper()
		raw, err := mobile(method, params)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		return raw
	}
	threadID := func(raw json.RawMessage) string {
		t.Helper()
		var r struct {
			Thread struct {
				ID string `json:"id"`
			}
		}
		if json.Unmarshal(raw, &r) != nil || r.Thread.ID == "" {
			t.Fatal("missing thread identity")
		}
		return r.Thread.ID
	}
	source := threadID(native("thread/start", map[string]any{"cwd": workspace, "historyMode": "paginated", "sandbox": "read-only", "approvalPolicy": "on-request"}))
	native("thread/inject_items", map[string]any{"threadId": source, "items": []any{map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "Synthetic lifecycle storage fixture; never run a model turn."}}}}})
	target := func(id string) map[string]any { return map[string]any{"thread_id": id, "workspace": workspace} }
	native("thread/goal/set", map[string]any{"threadId": source, "objective": "Synthetic paused goal; never run a model turn.", "status": "paused", "origin": "user"})
	fork := threadID(check("threads/fork", target(source)))
	if fork == source {
		t.Fatal("fork reused source identity")
	}
	check("threads/archive", target(source))
	archived := check("threads/list", map[string]any{"archived": true, "all_workspaces": true, "limit": 100})
	var list struct {
		Data []struct {
			ID string `json:"id"`
		}
	}
	if json.Unmarshal(archived, &list) != nil {
		t.Fatal("invalid archived list")
	}
	found := false
	for _, thread := range list.Data {
		if thread.ID == source {
			found = true
		}
	}
	if !found {
		t.Fatal("archived source absent")
	}
	if _, err = mobile("threads/delete", target(source)); err == nil {
		t.Fatal("delete accepted missing confirmation")
	}
	deleting := target(source)
	deleting["confirm"] = true
	if _, err = mobile("threads/delete", deleting); err == nil || !strings.Contains(err.Error(), "thread_has_dependents") {
		t.Fatalf("source dependency guard: %v", err)
	}
	if threadID(check("threads/unarchive", target(source))) != source {
		t.Fatal("restore changed source identity")
	}
	check("threads/read", map[string]any{"thread_id": fork, "workspace": workspace, "include_turns": true, "limit": 1})
	deleting = target(fork)
	deleting["confirm"] = true
	check("threads/delete", deleting)
	deleting = target(source)
	deleting["confirm"] = true
	check("threads/delete", deleting)
	if turns.Load() != 0 {
		t.Fatal("lifecycle operation started inference")
	}
	t.Log("native fork/archive/list/refused source delete/restore/confirmed leaf and source deletion; no turn started")
}

// Opt-in provider check, using only a new temporary-workspace conversation.
func TestNativeInterruptReachesMobile(t *testing.T) {
	binary := os.Getenv("MODEX_LIVE_SHARED_CODEX")
	if binary == "" {
		t.Skip("opt-in real-provider native interrupt check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	workspace := t.TempDir()
	p, err := bridge.StartProcess(ctx, binary, []string{"app-server", "--listen", "stdio://"}, workspace, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.CloseIdle()
	completed := make(chan json.RawMessage, 8)
	observedStart := make(chan json.RawMessage, 8)
	observedInput := make(chan struct{}, 8)
	backend, err := newSharedBackend(ctx, p, func(method string, params json.RawMessage) {
		if method == "turn/started" {
			observedStart <- append(json.RawMessage(nil), params...)
		}
		if method == "item/started" || method == "item/completed" {
			var event struct {
				Item struct {
					Type string `json:"type"`
				} `json:"item"`
			}
			_ = json.Unmarshal(params, &event)
			if event.Item.Type == "userMessage" {
				observedInput <- struct{}{}
			}
		}
		if method == "turn/completed" {
			raw, e := bridge.EncodeEvent(method, params)
			if e == nil {
				completed <- raw
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(t.TempDir(), "native.sock")
	stop, err := backend.listen(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	start, err := callNativeSocket(ctx, socket, "thread/start", map[string]any{"cwd": workspace, "model": "gpt-6.1-sol", "approvalPolicy": "on-request", "sandbox": "read-only"})
	if err != nil {
		t.Fatal(err)
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		}
	}
	if json.Unmarshal(start, &thread) != nil || thread.Thread.ID == "" {
		t.Fatal("missing new thread")
	}
	id := thread.Thread.ID
	started, err := callNativeSocket(ctx, socket, "turn/start", map[string]any{"threadId": id, "input": []any{map[string]string{"type": "text", "text": "Count from 1 to 10000, one number per line. Do not use tools."}}})
	if err != nil {
		t.Fatal(err)
	}
	var active struct {
		Turn struct {
			ID string `json:"id"`
		}
	}
	if json.Unmarshal(started, &active) != nil || active.Turn.ID == "" {
		t.Fatal("missing active turn")
	}
	select {
	case <-observedStart:
	case <-ctx.Done():
		t.Fatal("turn start not observed before interrupt")
	}
	select {
	case <-observedInput:
	case <-ctx.Done():
		t.Fatal("persistable native user input not observed before interrupt")
	}
	if _, err = callNativeSocket(ctx, socket, "turn/interrupt", map[string]any{"threadId": id, "turnId": active.Turn.ID}); err != nil {
		select {
		case raw := <-completed:
			var event struct {
				Params struct {
					Turn struct {
						Status string `json:"status"`
						Notice struct {
							Category string `json:"category"`
						} `json:"notice"`
					} `json:"turn"`
				} `json:"params"`
			}
			_ = json.Unmarshal(raw, &event)
			t.Fatalf("interrupt rejected; observed terminal status=%s notice=%s: %v", event.Params.Turn.Status, event.Params.Turn.Notice.Category, err)
		default:
			t.Fatal(err)
		}
	}
	select {
	case raw := <-completed:
		var event struct {
			Params struct {
				ThreadID string `json:"threadId"`
				Turn     struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"turn"`
			} `json:"params"`
		}
		if json.Unmarshal(raw, &event) != nil || event.Params.ThreadID != id || event.Params.Turn.ID != active.Turn.ID || event.Params.Turn.Status != "interrupted" {
			t.Fatal("mobile completion did not preserve exact interrupted identity/outcome")
		}
	case <-ctx.Done():
		t.Fatal("native interruption was not emitted to mobile")
	}
	result, err := bridge.HandleEvents(ctx, workspace, bridge.Command{ID: 1, Method: "threads/read", Params: mustJSON(map[string]any{"thread_id": id, "include_turns": true, "limit": 1})}, backend.call, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result)
	var history struct {
		Active   bool `json:"active_turn"`
		Messages []struct {
			TurnID string `json:"turn_id"`
			Status string `json:"turn_status"`
		} `json:"messages"`
	}
	if json.Unmarshal(encoded, &history) != nil || history.Active {
		t.Fatal("interrupted history remains active")
	}
	found := false
	for _, m := range history.Messages {
		if m.TurnID == active.Turn.ID && m.Status == "interrupted" {
			found = true
		}
	}
	if !found {
		t.Fatal("interrupted outcome absent from persisted mobile history")
	}
	t.Log("native turn/interrupt -> mobile completion with exact identity -> authoritative interrupted history")
}
