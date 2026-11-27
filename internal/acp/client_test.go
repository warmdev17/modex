package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestACPChild(t *testing.T) {
	if os.Getenv("MODEX_ACP_FIXTURE") != "1" {
		return
	}
	cwd, _ := os.Getwd()
	scanner := bufio.NewScanner(os.Stdin)
	send := json.NewEncoder(os.Stdout)
	var pending json.RawMessage
	for scanner.Scan() {
		var request struct {
			Version string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  struct {
				SessionID string                  `json:"sessionId"`
				Prompt    []struct{ Text string } `json:"prompt"`
			} `json:"params"`
			Result struct {
				Outcome struct{ Outcome string } `json:"outcome"`
			} `json:"result"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || request.Version != "2.0" {
			os.Exit(2)
		}
		reply := func(id json.RawMessage, result any) {
			_ = send.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
		}
		update := func(id, kind, text string) {
			_ = send.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
				"sessionId": id, "update": map[string]any{"sessionUpdate": kind, "content": map[string]string{"type": "text", "text": text}},
			}})
		}
		switch request.Method {
		case "initialize":
			reply(request.ID, json.RawMessage(`{"protocolVersion":1,"agentInfo":{"name":"Antigravity ACP Server","version":"1.5.17"},"agentCapabilities":{"loadSession":true,"sessionCapabilities":{"list":{}}}}`))
		case "session/list":
			reply(request.ID, map[string]any{"sessions": []map[string]string{
				{"sessionId": "saved", "cwd": cwd, "title": "Saved"},
				{"sessionId": "outside", "cwd": cwd + "-other"},
			}, "nextCursor": "another-page"})
		case "session/load":
			if request.Params.SessionID != "saved" {
				os.Exit(3)
			}
			update("saved", "user_message_chunk", "previous prompt")
			reply(request.ID, map[string]any{})
		case "session/new":
			reply(request.ID, map[string]string{"sessionId": "fresh"})
			if os.Getenv("MODEX_ACP_STALLED_INPUT") == "1" {
				time.Sleep(30 * time.Second)
				os.Exit(7)
			}
		case "session/prompt":
			switch request.Params.Prompt[0].Text {
			case "auth":
				_ = send.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32000, "message": "private extra provider detail"}})
			case "cancel":
				pending = request.ID
				update("outside", "agent_message_chunk", "must not escape")
				update("fresh", "agent_message_chunk", "partial")
			case "permission":
				pending = request.ID
				_ = send.Encode(json.RawMessage(`{"jsonrpc":"2.0","id":"permission-1","method":"session/request_permission","params":{"sessionId":"fresh","options":[{"optionId":"allow","kind":"allow_once"}]}}`))
			case "uncertain":
				_, _ = os.Stdout.WriteString(strings.Repeat("x", 16*1024*1024+1) + "\n")
			default:
				update("fresh", "agent_message_chunk", "pong")
				reply(request.ID, map[string]string{"stopReason": "end_turn"})
			}
		case "session/cancel":
			if request.Params.SessionID != "fresh" || pending == nil {
				os.Exit(4)
			}
			reply(pending, map[string]string{"stopReason": "cancelled"})
			pending = nil
		case "":
			if request.Result.Outcome.Outcome != "cancelled" || string(request.ID) != `"permission-1"` || pending == nil {
				os.Exit(5)
			}
			reply(pending, map[string]string{"stopReason": "end_turn"})
			pending = nil
		default:
			os.Exit(6)
		}
	}
	os.Exit(0)
}

func TestLiveAntigravitySessionDiscovery(t *testing.T) {
	binary := os.Getenv("MODEX_TEST_AGY")
	if binary == "" {
		t.Skip("set MODEX_TEST_AGY to the Antigravity CLI executable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "XDG_CONFIG_HOME=" + filepath.Join(root, "config")}
	args := []string{"acp", "--disable-telemetry", "--log-file", filepath.Join(root, "acp.log")}
	client, err := start(ctx, binary, args, workspace, env, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	page, err := client.List(ctx, "")
	if err != nil || len(page.Sessions) != 0 {
		t.Fatalf("isolated workspace list: count=%d err=%v", len(page.Sessions), err)
	}
	id, err := client.NewSession(ctx)
	if errors.Is(err, ErrAuthenticationRequired) {
		t.Log("the installed CLI requires sign-in for session creation")
		return
	}
	if err != nil || id == "" {
		t.Fatalf("native session creation failed: %v", err)
	}
	t.Log("native session creation succeeded")
}

func TestAntigravityPromptDeadlineInterruptsBlockedWrite(t *testing.T) {
	env := append(os.Environ(), "MODEX_ACP_FIXTURE=1", "MODEX_ACP_STALLED_INPUT=1")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	client, err := start(ctx, os.Args[0], []string{"-test.run=^TestACPChild$"}, t.TempDir(), env, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	id, err := client.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	deadline, cancelPrompt := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancelPrompt()
	result := make(chan error, 1)
	go func() {
		_, err := client.Prompt(deadline, id, "stalled", strings.Repeat("x", 1024*1024))
		result <- err
	}()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("stalled prompt returned success")
		}
	case <-time.After(time.Second):
		t.Fatal("prompt did not return after the deadline while stdin was blocked")
	}
}

func TestAntigravitySessionLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	events := make(chan Update, 8)
	env := append(os.Environ(), "MODEX_ACP_FIXTURE=1")
	client, err := start(ctx, os.Args[0], []string{"-test.run=^TestACPChild$"}, t.TempDir(), env, func(update Update) {
		events <- update
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if client.Info().Version != "1.5.17" || !client.Info().ListSessions || !client.Info().LoadSession {
		t.Fatalf("handshake: %+v", client.Info())
	}
	page, err := client.List(ctx, "")
	if err != nil || len(page.Sessions) != 1 || page.Sessions[0].ID != "saved" || page.NextCursor != "another-page" {
		t.Fatalf("session page: %+v %v", page, err)
	}
	if err := client.Load(ctx, "outside"); err == nil {
		t.Fatal("loaded an out-of-workspace session")
	}
	if err := client.Load(ctx, "saved"); err != nil {
		t.Fatal(err)
	}
	checkUpdate := func(id, turn, kind, text string) {
		t.Helper()
		select {
		case event := <-events:
			if event.SessionID != id || event.TurnID != turn || event.Kind != kind || event.Text != text {
				t.Fatalf("unexpected update: %+v", event)
			}
		case <-ctx.Done():
			t.Fatal("missing ACP update")
		}
	}
	checkUpdate("saved", "", "user_message_chunk", "previous prompt")
	id, err := client.NewSession(ctx)
	if err != nil || id != "fresh" {
		t.Fatalf("new session: %s %v", id, err)
	}
	if _, err := client.Prompt(ctx, id, "auth", "auth"); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("authentication error: %v", err)
	}
	if stop, err := client.Prompt(ctx, id, "one", "ping"); err != nil || stop != "end_turn" {
		t.Fatalf("prompt: %s %v", stop, err)
	}
	checkUpdate(id, "one", "agent_message_chunk", "pong")

	type result struct {
		stop string
		err  error
	}
	completed := make(chan result, 1)
	go func() {
		stop, err := client.Prompt(ctx, id, "two", "cancel")
		completed <- result{stop, err}
	}()
	checkUpdate(id, "two", "agent_message_chunk", "partial")
	if _, err := client.Prompt(ctx, id, "duplicate", "ping"); err == nil {
		t.Fatal("overlapping prompt was accepted")
	}
	if err := client.Cancel(ctx, id, "one"); err == nil {
		t.Fatal("stale Stop was accepted")
	}
	if err := client.Cancel(ctx, id, "two"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-completed:
		if got.err != nil || got.stop != "cancelled" {
			t.Fatalf("cancel: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("cancel did not finish")
	}
	if stop, err := client.Prompt(ctx, id, "three", "permission"); err != nil || stop != "end_turn" {
		t.Fatalf("permission fallback: %s %v", stop, err)
	}
	if _, err := client.Prompt(ctx, id, "four", "uncertain"); err == nil {
		t.Fatal("oversized frame should make turn outcome uncertain")
	}
	if _, err := client.Prompt(ctx, id, "five", "ping"); err == nil {
		t.Fatal("a new prompt used the ACP process with an uncertain active turn")
	}
}
