package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/warmdev/modex/internal/bridge"
)

func TestLiveMobileSkillReference(t *testing.T) {
	binary := os.Getenv("MODEX_LIVE_SKILLS_CODEX")
	if binary == "" {
		t.Skip("set MODEX_LIVE_SKILLS_CODEX for a real provider turn in a temporary workspace")
	}
	workspace := t.TempDir()
	skill := filepath.Join(workspace, ".agents", "skills", "modex-skill-check", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("---\nname: modex-skill-check\ndescription: Verify Modex skill invocation with a harmless fixed response.\n---\nReply exactly MODEX_SKILL_OK without running tools or changing files.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	p, err := bridge.StartProcess(ctx, binary, []string{"app-server", "--listen", "stdio://"}, workspace, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	completed := make(chan json.RawMessage, 1)
	backend, err := newSharedBackend(ctx, p, func(method string, params json.RawMessage) {
		if method == "turn/completed" {
			select {
			case completed <- params:
			default:
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, params any) (any, error) {
		raw, _ := json.Marshal(params)
		return bridge.HandleEvents(ctx, workspace, bridge.Command{Method: method, Params: raw}, backend.call, nil)
	}
	catalog, err := call("references/list", map[string]any{"force_reload": true})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(catalog)
	if !strings.Contains(string(raw), "modex-skill-check") {
		t.Fatal("workspace skill not discovered")
	}
	created, err := call("threads/start", map[string]any{"permission_mode": "read-only"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(created)
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if json.Unmarshal(raw, &thread) != nil || thread.Thread.ID == "" {
		t.Fatal("missing thread id")
	}
	_, err = call("turn/start", map[string]any{"thread_id": thread.Thread.ID, "text": "$modex-skill-check Follow the selected skill.", "references": []map[string]string{{"kind": "skill", "name": "modex-skill-check", "path": skill}}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-completed:
		var event struct {
			Turn struct {
				Status string `json:"status"`
			} `json:"turn"`
		}
		_ = json.Unmarshal(result, &event)
		if event.Turn.Status != "completed" {
			t.Fatalf("turn status: %s", event.Turn.Status)
		}
	case <-ctx.Done():
		t.Fatal("skill turn did not complete")
	}
	history, err := backend.call(ctx, "thread/read", map[string]any{"threadId": thread.Thread.ID, "includeTurns": true}, nil)
	if err != nil || !strings.Contains(string(history), "MODEX_SKILL_OK") {
		t.Fatalf("skill instruction was not observed; read error: %v", err)
	}
	t.Log("PASS: remote catalogue, typed skill input and real-provider skill instructions")
}

// This exercises an installed plugin without invoking its external tools.
func TestLiveMobilePluginReference(t *testing.T) {
	binary := os.Getenv("MODEX_LIVE_PLUGINS_CODEX")
	if binary == "" {
		t.Skip("set MODEX_LIVE_PLUGINS_CODEX for a read-only installed-plugin provider test")
	}
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	p, err := bridge.StartProcess(ctx, binary, []string{"app-server", "--listen", "stdio://"}, workspace, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	completed := make(chan json.RawMessage, 1)
	backend, err := newSharedBackend(ctx, p, func(method string, params json.RawMessage) {
		if method == "turn/completed" {
			select {
			case completed <- params:
			default:
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, params any) (any, error) {
		raw, _ := json.Marshal(params)
		return bridge.HandleEvents(ctx, workspace, bridge.Command{Method: method, Params: raw}, backend.call, nil)
	}
	catalog, err := call("references/list", nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(catalog)
	var entries struct {
		Entries []struct {
			Kind, Name, Path string
			Enabled          bool
		}
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	name, path := "", ""
	for _, entry := range entries.Entries {
		if entry.Kind == "plugin" && entry.Name == "frontend-design" && entry.Enabled {
			name, path = entry.Name, entry.Path
			break
		}
	}
	if path == "" {
		t.Fatal("install and enable frontend-design before running this opt-in test")
	}
	created, err := call("threads/start", map[string]any{"permission_mode": "read-only"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(created)
	var thread struct{ Thread struct{ ID string } }
	if json.Unmarshal(raw, &thread) != nil || thread.Thread.ID == "" {
		t.Fatal("missing thread id")
	}
	_, err = call("turn/start", map[string]any{"thread_id": thread.Thread.ID, "text": "@frontend-design This is a transport verification only. Do not run tools or change files. Reply exactly MODEX_PLUGIN_OK.", "references": []map[string]string{{"kind": "plugin", "name": name, "path": path}}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-completed:
		var event struct{ Turn struct{ Status string } }
		if json.Unmarshal(result, &event) != nil || event.Turn.Status != "completed" {
			t.Fatal("plugin turn failed")
		}
	case <-ctx.Done():
		t.Fatal("plugin turn did not complete")
	}
	history, err := backend.call(ctx, "thread/read", map[string]any{"threadId": thread.Thread.ID, "includeTurns": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var transcript struct {
		Thread struct {
			Turns []struct {
				Items []struct{ Type, Role, Text string }
			}
		}
	}
	if json.Unmarshal(history, &transcript) != nil {
		t.Fatal("invalid history")
	}
	found := false
	for _, turn := range transcript.Thread.Turns {
		for _, item := range turn.Items {
			if item.Type == "agentMessage" && strings.TrimSpace(item.Text) == "MODEX_PLUGIN_OK" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("missing completed assistant response")
	}
	t.Log("PASS: installed catalogue, freshly validated typed plugin mention, real-provider completed assistant response; no external plugin tool invocation asserted")
}
