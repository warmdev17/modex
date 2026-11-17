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

// Opt-in, isolated provider turns. Never resumes an existing user session.
func TestLivePlanActions(t *testing.T) {
	binary := os.Getenv("MODEX_LIVE_SHARED_CODEX")
	if binary == "" {
		t.Skip("opt-in real Codex plan check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	workspace := t.TempDir()
	if err := os.Chmod(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	p, err := bridge.StartProcess(ctx, binary, []string{"app-server", "--listen", "stdio://"}, workspace, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	events := make(chan json.RawMessage, 2048)
	backend, err := newSharedBackend(ctx, p, func(method string, params json.RawMessage) {
		if method == "decision/required" {
			t.Log("isolated turn waiting for a decision")
		}
		if method == "turn/started" {
			t.Log("isolated turn started")
		}
		if method == "turn/completed" {
			events <- params
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	backend.plans = bridge.NewPlanActions(filepath.Join(t.TempDir(), "receipts"), func(ctx context.Context, m string, p any) (json.RawMessage, error) {
		t.Log("plan API", m)
		return backend.call(ctx, m, p, nil)
	})
	wait := func(t *testing.T) {
		t.Helper()
		select {
		case raw := <-events:
			var e struct {
				Turn struct {
					Status string `json:"status"`
				} `json:"turn"`
			}
			_ = json.Unmarshal(raw, &e)
			if e.Turn.Status != "completed" {
				t.Fatalf("turn status %s", e.Turn.Status)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	for _, action := range []string{"stay", "implement", "fresh"} {
		t.Run(action, func(t *testing.T) {
			raw, err := p.CallPersistent(ctx, "thread/start", map[string]any{"cwd": workspace, "approvalPolicy": "on-request", "sandbox": "read-only"})
			if err != nil {
				t.Fatal(err)
			}
			var start struct {
				Thread struct {
					ID string `json:"id"`
				} `json:"thread"`
			}
			if json.Unmarshal(raw, &start) != nil || start.Thread.ID == "" {
				t.Fatal("invalid source")
			}
			thread := start.Thread.ID
			_, err = backend.call(ctx, "turn/start", map[string]any{"threadId": thread, "input": []map[string]string{{"type": "text", "text": "Produce a proposed plan with exactly these steps inside a proposed_plan block: 1. Reply MODEX_PLAN_OK. 2. Stop. No files, tools, questions or other work. Do not implement yet."}}, "collaborationMode": map[string]any{"mode": "plan", "settings": map[string]any{"model": "gpt-6.1-sol", "reasoning_effort": "low", "developer_instructions": nil}}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			wait(t)
			raw, err = backend.call(ctx, "thread/turns/list", map[string]any{"threadId": thread, "limit": 1, "sortDirection": "desc", "itemsView": "full"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			var page struct {
				Data []struct {
					ID    string `json:"id"`
					Items []struct {
						ID   string `json:"id"`
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"items"`
				} `json:"data"`
			}
			if json.Unmarshal(raw, &page) != nil || len(page.Data) != 1 {
				t.Fatal("invalid turns")
			}
			item := ""
			for _, i := range page.Data[0].Items {
				if i.Type == "plan" && i.Text != "" {
					item = i.ID
				}
			}
			if item == "" {
				t.Fatal("provider did not produce a structured plan")
			}
			raw, err = backend.call(ctx, "modex/plan/respond", map[string]any{"threadId": thread, "turnId": page.Data[0].ID, "itemId": item, "cwd": workspace, "action": action}, nil)
			if err != nil {
				t.Fatalf("plan %s: %v", action, err)
			}
			var result struct {
				Thread string `json:"thread_id"`
			}
			_ = json.Unmarshal(raw, &result)
			if action == "fresh" && result.Thread == thread {
				t.Fatal("fresh reused source")
			}
			if action != "stay" {
				wait(t)
			}
			t.Log("isolated native plan action verified", action)
		})
	}
}
