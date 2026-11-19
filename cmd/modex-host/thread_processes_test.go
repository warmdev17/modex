package main

import (
	"context"
	"encoding/json"
	"github.com/warmdev/modex/internal/bridge"
	"os"
	"strings"
	"testing"
	"time"
)

func TestStatusOwnershipAndBusyMode(t *testing.T) {
	s := newThreadProcesses(context.Background(), nil, nil)
	p := map[string]any{"threadId": "t", "mode": "plan", "model": "model"}
	if _, err := s.Call(context.Background(), "modex/thread/status", p); err == nil || !strings.Contains(err.Error(), "session_not_owned") {
		t.Fatalf("missing owner: %v", err)
	}
	w := &threadWorker{busy: true, session: map[string]json.RawMessage{"thread": json.RawMessage(`{"id":"t"}`)}, decisions: nil}
	s.workers["t"] = w
	if _, err := s.Call(context.Background(), "modex/thread/mode", p); err == nil {
		t.Fatal("busy mode accepted")
	}
	raw, err := s.Call(context.Background(), "modex/thread/status", p)
	if err != nil || !strings.Contains(string(raw), `"token_usage":null`) {
		t.Fatalf("status %s %v", raw, err)
	}
}

func TestModeRequestHasBoundedDeadline(t *testing.T) {
	ctx, cancel := modeRequestContext(context.Background())
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 20*time.Second || time.Until(deadline) < 19*time.Second {
		t.Fatal("mode request is not bounded to 20 seconds")
	}
}
func TestSettingsUpdateClearsUnavailableValues(t *testing.T) {
	state := &sharedSession{settings: map[string]json.RawMessage{"thread": json.RawMessage(`{"id":"t"}`), "effort": json.RawMessage(`"high"`), "approval_policy": json.RawMessage(`"never"`), "sandbox": json.RawMessage(`"readOnly"`)}}
	updateSharedSettings(state, json.RawMessage(`{"threadSettings":{"model":"m","effort":null,"collaborationMode":{"mode":"plan"},"approvalPolicy":{"granular":{}},"sandboxPolicy":null}}`))
	for _, key := range []string{"effort", "approval_policy", "sandbox"} {
		if _, ok := state.settings[key]; ok {
			t.Fatalf("stale %s", key)
		}
	}
}

func TestReacquiredWorkerHasSessionStatus(t *testing.T) {
	t.Setenv("MODEX_SHARED_CHILD", "1")
	t.Setenv("MODEX_SHARED_INTERRUPT", "1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := bridge.StartProcess(ctx, os.Args[0], []string{"-test.run=^TestSharedCodexChild$"}, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	s := newThreadProcesses(ctx, p, func(string, json.RawMessage) {})
	defer s.Close()
	if _, err = s.Call(ctx, "turn/start", map[string]any{"threadId": "shared", "input": []any{}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Call(ctx, "modex/thread/status", map[string]any{"threadId": "shared"}); err != nil {
		t.Fatalf("reacquired status: %v", err)
	}
}

func TestRejectedTurnLeavesWorkerIdle(t *testing.T) {
	t.Setenv("MODEX_SHARED_CHILD", "1")
	t.Setenv("MODEX_REJECT_TURN", "1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := bridge.StartProcess(ctx, os.Args[0], []string{"-test.run=^TestSharedCodexChild$"}, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	s := newThreadProcesses(ctx, p, func(string, json.RawMessage) {})
	defer s.Close()
	if _, err = s.Call(ctx, "turn/start", map[string]any{"threadId": "shared"}); err == nil {
		t.Fatal("expected rejection")
	}
	if s.workers["shared"].busy {
		t.Fatal("definitive rejection left worker busy")
	}
}

func TestSessionUnloadNotifications(t *testing.T) {
	for _, tc := range []struct {
		method, raw string
		expected    bool
	}{
		{"thread/status/changed", `{"threadId":"t","status":{"type":"notLoaded"}}`, true},
		{"thread/status/changed", `{"threadId":"t","status":{"type":"idle"}}`, false},
		{"thread/archived", `{"threadId":"t"}`, true}, {"thread/closed", `{"threadId":"t"}`, true},
	} {
		if invalidatesSession(tc.method, json.RawMessage(tc.raw)) != tc.expected {
			t.Fatalf("incorrect invalidation: %s %s", tc.method, tc.raw)
		}
	}
}
