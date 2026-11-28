package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCLIFullscreenPickerDiscoversAgentsInPlace(t *testing.T) {
	t.Setenv("LC_ALL", "C.UTF-8")
	t.Setenv("TERM", "xterm-256color")
	p := newCLIFullscreenPicker(context.Background(), io.Discard, "/work", func(context.Context) ([]cliAgent, error) {
		return []cliAgent{{ID: "codex", Label: "Codex", Ready: true}}, nil
	}, nil)
	if !strings.Contains(p.View(), "Discovering agent runtimes") {
		t.Fatalf("discovery status missing: %q", p.View())
	}
	p.Update(p.Init()())
	if view := p.View(); !strings.Contains(view, "› ◇  Codex") || !strings.Contains(view, "/work") {
		t.Fatalf("discovered agents or workspace missing: %q", view)
	}
}

func TestCLIFullscreenPickerSkipsActivationForUnavailableAgent(t *testing.T) {
	p := readyCLIFullscreenPicker(t, []cliAgent{
		{ID: "codex", Label: "Codex", Ready: true},
		{ID: "antigravity", Label: "Antigravity", Reason: "Sign in first\x1b[31m"},
	}, "/work", func(context.Context, cliAgent, string) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[]}`), nil
	})
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	if view := p.View(); !strings.Contains(view, "› ✦  Antigravity") || !strings.Contains(view, "Sign in first") || strings.Contains(view, "\x1b") {
		t.Fatalf("unavailable agent row did not explain the restriction safely: %q", view)
	}
	if _, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("unavailable agent accepted Enter")
	}
	p.Update(tea.KeyMsg{Type: tea.KeyUp})
	pressCLIPickerKey(t, p, tea.KeyEnter)
	if !strings.Contains(p.View(), "New conversation") {
		t.Fatalf("new conversation missing from empty page: %q", p.View())
	}
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || p.err != nil || p.choice != (cliChoice{Workspace: "/work"}) {
		t.Fatalf("new conversation was not selected: %+v, %v", p.choice, p.err)
	}
}

func TestCLIFullscreenPickerScrollsAndPaginates(t *testing.T) {
	var cursors []string
	p := readyCLIFullscreenPicker(t, []cliAgent{
		{ID: "opencode", Label: "OpenCode", Ready: true},
	}, "/work", func(_ context.Context, _ cliAgent, cursor string) (json.RawMessage, error) {
		cursors = append(cursors, cursor)
		if cursor == "older" {
			return json.RawMessage(`{"data":[{"id":"oc:older","name":"Older","workspace":"/work"}]}`), nil
		}
		return json.RawMessage(`{"data":[{"id":"oc:one","name":"First"},{"id":"oc:two","name":"Second"},{"id":"oc:three","name":"Third"},{"id":"oc:four","name":"Fourth"},{"id":"oc:five","name":"Fifth"},{"id":"oc:six","name":"Sixth"},{"id":"oc:seven","name":"Seventh"}],"next_cursor":"older"}`), nil
	})
	p.Update(tea.WindowSizeMsg{Width: 50, Height: 10})
	pressCLIPickerKey(t, p, tea.KeyEnter)
	for range 7 {
		p.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if view := p.View(); !strings.Contains(view, "› Seventh") || strings.Contains(view, "First") || strings.Count(strings.TrimSuffix(view, "\n"), "\n")+1 > 10 {
		t.Fatalf("viewport did not scroll within terminal height: %q", view)
	}
	pressCLIPickerKey(t, p, tea.KeyPgDown)
	if !strings.Contains(p.View(), "Older") {
		t.Fatalf("next page missing: %q", p.View())
	}
	pressCLIPickerKey(t, p, tea.KeyPgUp)
	if view := p.View(); !strings.Contains(view, "› New conversation") || !strings.Contains(view, "First") {
		t.Fatalf("previous page missing or selection did not reset: %q", view)
	}
	p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if view := p.View(); !strings.Contains(view, "Choose an agent") || !strings.Contains(view, "› ▣  OpenCode") {
		t.Fatalf("agent picker was not restored: %q", view)
	}
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if strings.Join(cursors, ",") != ",older" || cmd == nil || !errors.Is(p.err, errCLICancelled) {
		t.Fatalf("cursors=%q cancellation=%v", cursors, p.err)
	}
}

func TestCLIPickerCodexDoesNotWaitForOtherReadiness(t *testing.T) {
	p := newCLIFullscreenPicker(context.Background(), io.Discard, "/work", func(context.Context) ([]cliAgent, error) {
		return []cliAgent{{ID: "codex", Label: "Codex", Ready: true}, {ID: "antigravity", Label: "Antigravity", Checking: true}}, nil
	}, func(context.Context, cliAgent, string) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[]}`), nil
	})
	p.checkAgent = func(context.Context, string) (cliAgent, error) {
		t.Fatal("pending probe ran synchronously")
		return cliAgent{}, nil
	}
	_, checks := p.Update(p.Init()())
	if checks == nil || !strings.Contains(p.View(), "Checking") {
		t.Fatal("background readiness not scheduled")
	}
	pressCLIPickerKey(t, p, tea.KeyEnter)
	if !strings.Contains(p.View(), "New conversation") {
		t.Fatal("Codex waited for optional runtime")
	}
	p.Update(cliPickerAgentMsg{id: "antigravity", agent: cliAgent{ID: "antigravity", Label: "Antigravity", Ready: true}})
	if !p.agents[1].Ready || p.agents[1].Checking {
		t.Fatal("probe did not update independently")
	}
}

func TestCLIPickerCanBrowseCheckingAgentButCannotLaunch(t *testing.T) {
	p := readyCLIFullscreenPicker(t, []cliAgent{{ID: "antigravity", Label: "Antigravity", Checking: true}}, "/work", func(context.Context, cliAgent, string) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[]}`), nil
	})
	pressCLIPickerKey(t, p, tea.KeyEnter)
	if !strings.Contains(p.View(), "New conversation") {
		t.Fatal("saved sessions blocked by readiness")
	}
	p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.choice.Workspace != "" {
		t.Fatal("unverified runtime launched")
	}
	p.Update(cliPickerAgentMsg{id: "antigravity", agent: cliAgent{ID: "antigravity", Label: "Antigravity", Ready: true}})
	p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.choice.Workspace != "/work" {
		t.Fatal("verified runtime remained blocked")
	}
}

func TestCLIFullscreenPickerIgnoresAbandonedSessionList(t *testing.T) {
	p := readyCLIFullscreenPicker(t, []cliAgent{
		{ID: "codex", Label: "Codex", Ready: true},
		{ID: "opencode", Label: "OpenCode", Ready: true},
	}, "/work", func(ctx context.Context, agent cliAgent, _ string) (json.RawMessage, error) {
		if agent.ID == "codex" {
			return nil, ctx.Err()
		}
		return json.RawMessage(`{"data":[{"id":"oc:two","name":"Second"}]}`), nil
	})
	_, stale := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if stale == nil {
		t.Fatal("no initial session fetch")
	}
	p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	pressCLIPickerKey(t, p, tea.KeyEnter)
	p.Update(stale())
	if view := p.View(); !strings.Contains(view, "Second") || p.err != nil {
		t.Fatalf("abandoned response replaced the current sessions: %q, %v", view, p.err)
	}
}

func TestCLIFullscreenPickerCommitsSelectionBeforeTerminalExit(t *testing.T) {
	p := readyCLIFullscreenPicker(t, []cliAgent{
		{ID: "codex", Label: "Codex", Ready: true},
		{ID: "opencode", Label: "OpenCode", Ready: true},
	}, "/work", func(context.Context, cliAgent, string) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[{"id":"oc:one","cwd":"/work"}]}`), nil
	})
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	pressCLIPickerKey(t, p, tea.KeyEnter)
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, quit := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// Input already queued by the terminal can arrive before the quit message.
	p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	p.Update(tea.KeyMsg{Type: tea.KeyUp})
	if quit == nil || p.agents[p.agentIndex].ID != "opencode" || p.choice != (cliChoice{ID: "oc:one", Workspace: "/work"}) {
		t.Fatalf("queued keys changed the selected session: agent=%+v choice=%+v", p.agents[p.agentIndex], p.choice)
	}
}

func TestCLIFullscreenPickerRejectsMalformedSessionSelection(t *testing.T) {
	for _, tc := range []struct {
		name, data, message string
		keys                []tea.KeyType
	}{
		{"foreign ID", `{"data":[{"id":"agy:foreign"}]}`, "invalid session ID", nil},
		{"relative workspace", `{"data":[{"id":"oc:session","workspace":"relative"}]}`, "invalid session workspace", []tea.KeyType{tea.KeyDown, tea.KeyEnter}},
		{"repeated cursor", `{"data":[],"next_cursor":"duplicate"}`, "repeated session list cursor", []tea.KeyType{tea.KeyPgDown, tea.KeyPgDown}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := readyCLIFullscreenPicker(t, []cliAgent{
				{ID: "opencode", Ready: true},
			}, "/work", func(context.Context, cliAgent, string) (json.RawMessage, error) {
				return json.RawMessage(tc.data), nil
			})
			pressCLIPickerKey(t, p, tea.KeyEnter)
			for _, key := range tc.keys {
				_, cmd := p.Update(tea.KeyMsg{Type: key})
				if key == tea.KeyPgDown && p.err == nil && cmd != nil {
					p.Update(cmd())
				}
			}
			if tc.name != "repeated cursor" {
				if p.pageError == "" || p.choice.Workspace != "" {
					t.Fatal("malformed page was selectable")
				}
				return
			}
			if p.err == nil || !strings.Contains(p.err.Error(), tc.message) {
				t.Fatalf("got %v, want %q", p.err, tc.message)
			}
		})
	}
}

func pressCLIPickerKey(t *testing.T, p *cliFullscreenPicker, key tea.KeyType) {
	t.Helper()
	_, cmd := p.Update(tea.KeyMsg{Type: key})
	if cmd == nil {
		t.Fatalf("key %v did not start fetching sessions", key)
	}
	p.Update(cmd())
}

func readyCLIFullscreenPicker(t *testing.T, agents []cliAgent, cwd string, list func(context.Context, cliAgent, string) (json.RawMessage, error)) *cliFullscreenPicker {
	t.Helper()
	t.Setenv("LC_ALL", "C.UTF-8")
	t.Setenv("TERM", "xterm-256color")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	p := newCLIFullscreenPicker(ctx, io.Discard, cwd, func(context.Context) ([]cliAgent, error) { return agents, nil }, list)
	p.Update(p.Init()())
	return p
}

func TestCLIPickerUsesCachedPagesUntilRefresh(t *testing.T) {
	calls := 0
	p := readyCLIFullscreenPicker(t, []cliAgent{{ID: "codex", Ready: true}}, "/work", func(context.Context, cliAgent, string) (json.RawMessage, error) {
		calls++
		return json.RawMessage(`{"data":[{"id":"01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6","cwd":"/work","name":"Scoped"}]}`), nil
	})
	pressCLIPickerKey(t, p, tea.KeyEnter)
	p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		p.Update(cmd())
	}
	if calls != 1 || !strings.Contains(p.View(), "Scoped") {
		t.Fatalf("cached page lost, calls=%d", calls)
	}
	_, refresh := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if refresh == nil {
		t.Fatal("missing explicit refresh")
	}
	p.Update(refresh())
	if calls != 2 {
		t.Fatalf("refresh calls %d", calls)
	}
}

func TestCLIPickerForeignWorkspaceNeverAppears(t *testing.T) {
	p := readyCLIFullscreenPicker(t, []cliAgent{{ID: "opencode", Ready: true}}, "/work", func(context.Context, cliAgent, string) (json.RawMessage, error) {
		return json.RawMessage(`{"data":[{"id":"oc:one","cwd":"/other","name":"Foreign"},{"id":"oc:two","cwd":"/work","name":"Local"}]}`), nil
	})
	pressCLIPickerKey(t, p, tea.KeyEnter)
	if view := p.View(); strings.Contains(view, "Foreign") || !strings.Contains(view, "Local") {
		t.Fatalf("workspace leak: %s", view)
	}
}

func TestCLIPickerPageFailureStaysRetryable(t *testing.T) {
	calls := 0
	p := readyCLIFullscreenPicker(t, []cliAgent{{ID: "codex", Ready: true}}, "/work", func(context.Context, cliAgent, string) (json.RawMessage, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("unavailable")
		}
		return json.RawMessage(`{"data":[]}`), nil
	})
	pressCLIPickerKey(t, p, tea.KeyEnter)
	if p.err != nil || !strings.Contains(p.View(), "Retry") {
		t.Fatalf("page failure exited picker: %v", p.err)
	}
	_, retry := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if retry == nil {
		t.Fatal("missing retry")
	}
	p.Update(retry())
	p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.choice.Workspace != "/work" || p.pageError != "" {
		t.Fatal("retry did not restore selection")
	}
}

func TestCLIPickerCachedSelectionCannotUsePreviousAgentPage(t *testing.T) {
	p := readyCLIFullscreenPicker(t, []cliAgent{{ID: "codex", Ready: true}, {ID: "opencode", Ready: true}}, "/work", func(_ context.Context, agent cliAgent, _ string) (json.RawMessage, error) {
		if agent.ID == "codex" {
			return json.RawMessage(`{"data":[{"id":"01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6","name":"Codex"}]}`), nil
		}
		return json.RawMessage(`{"data":[{"id":"oc:one","name":"OpenCode"}]}`), nil
	})
	pressCLIPickerKey(t, p, tea.KeyEnter)
	p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	pressCLIPickerKey(t, p, tea.KeyEnter)
	p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	p.Update(tea.KeyMsg{Type: tea.KeyUp})
	p.Update(tea.KeyMsg{Type: tea.KeyEnter}) // leave the cached-result message queued
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if p.choice.ID != "01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6" {
		t.Fatalf("cached agent used foreign page: %+v", p.choice)
	}
}
