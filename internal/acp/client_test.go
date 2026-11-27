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
	var permissionMode string
	for scanner.Scan() {
		var request struct {
			Version string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  struct {
				SessionID string                  `json:"sessionId"`
				ConfigID  string                  `json:"configId"`
				Value     string                  `json:"value"`
				Prompt    []struct{ Text string } `json:"prompt"`
			} `json:"params"`
			Result struct {
				Outcome struct{ Outcome, OptionID string } `json:"outcome"`
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
			reply(request.ID, json.RawMessage(`{"configOptions":[{"id":"thought_level","category":"thought_level","name":"Thinking","type":"select","currentValue":"low","options":[{"value":"low","name":"Low"}]}]}`))
		case "session/new":
			reply(request.ID, json.RawMessage(`{"sessionId":"fresh","configOptions":[{"id":"model","category":"model","name":"Model","type":"select","currentValue":"a","options":[{"group":"g","name":"Group","options":[{"value":"a","name":"A"},{"value":"b","name":"B"}]}]},{"id":"thought_level","category":"thought_level","name":"Thinking","type":"select","currentValue":"low","options":[{"value":"low","name":"Low"}]}]}`))
			if os.Getenv("MODEX_ACP_STALLED_INPUT") == "1" {
				time.Sleep(30 * time.Second)
				os.Exit(7)
			}
		case "session/set_config_option":
			if os.Getenv("MODEX_ACP_SET_UNCERTAIN") == "1" {
				_, _ = os.Stdout.WriteString(strings.Repeat("x", 16*1024*1024+1) + "\n")
				continue
			}
			if request.Params.SessionID != "fresh" || request.Params.ConfigID != "model" || request.Params.Value != "b" {
				os.Exit(8)
			}
			if os.Getenv("MODEX_ACP_CONFLICT") == "1" {
				_ = send.Encode(json.RawMessage(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"fresh","update":{"sessionUpdate":"config_option_update","configOptions":[{"id":"model","category":"model","name":"Model","type":"select","currentValue":"a","options":[{"value":"a","name":"A"}]}]}}}`))
			}
			reply(request.ID, json.RawMessage(`{"configOptions":[{"id":"model","category":"model","name":"Model","type":"select","currentValue":"b","options":[{"value":"b","name":"B"}]},{"id":"thought_level","category":"thought_level","name":"Thinking","type":"select","currentValue":"high","options":[{"value":"high","name":"High"}]}]}`))
		case "session/prompt":
			switch request.Params.Prompt[0].Text {
			case "tools":
				_ = send.Encode(json.RawMessage(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"fresh","update":{"sessionUpdate":"tool_call","name":"invoke_subagent","toolCallId":"call-1","status":"in_progress","rawInput":{"Role":"reviewer","Prompt":"SECRET","Model":"SECRET"}}}}`))
				_ = send.Encode(json.RawMessage(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"fresh","update":{"sessionUpdate":"tool_call_update","toolCallId":"call-1","status":"completed","rawOutput":"SECRET"}}}`))
				reply(request.ID, map[string]string{"stopReason": "end_turn"})
			case "auth":
				_ = send.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32000, "message": "private extra provider detail"}})
			case "config-update":
				_ = send.Encode(json.RawMessage(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"fresh","update":{"sessionUpdate":"config_option_update","configOptions":[{"id":"model","category":"model","name":"Model","type":"select","currentValue":"b","options":[{"value":"b","name":"B"}]}]}}}`))
				reply(request.ID, map[string]string{"stopReason": "end_turn"})
			case "cancel", "cancel-late":
				permissionMode = request.Params.Prompt[0].Text
				pending = request.ID
				update("outside", "agent_message_chunk", "must not escape")
				update("fresh", "agent_message_chunk", "partial")
			case "permission", "permission-select", "permission-deny", "permission-cancel", "permission-ambiguous":
				pending = request.ID
				permissionMode = request.Params.Prompt[0].Text
				if permissionMode == "permission-ambiguous" {
					_ = send.Encode(json.RawMessage(`{"jsonrpc":"2.0","id":"permission-1","method":"session/request_permission","params":{"sessionId":"fresh","options":[{"optionId":"allow","kind":"allow_once"},{"optionId":"other","kind":"allow_once"},{"optionId":"same","kind":"reject_once"},{"optionId":"same","kind":"allow_always"}]}}`))
				} else {
					_ = send.Encode(json.RawMessage(`{"jsonrpc":"2.0","id":"permission-1","method":"session/request_permission","params":{"sessionId":"fresh","toolCall":{"title":"Run command"},"options":[{"optionId":"allow","kind":"allow_once"},{"optionId":"deny","kind":"reject_once"},{"optionId":"always","kind":"allow_always"}]}}`))
				}
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
			if permissionMode == "cancel-late" {
				_ = send.Encode(json.RawMessage(`{"jsonrpc":"2.0","id":"permission-1","method":"session/request_permission","params":{"sessionId":"fresh","options":[{"optionId":"allow","kind":"allow_once"}]}}`))
				reply(pending, map[string]string{"stopReason": "cancelled"})
			} else {
				reply(pending, map[string]string{"stopReason": "cancelled"})
				pending = nil
			}
		case "":
			if ((permissionMode == "permission" || permissionMode == "cancel-late") && request.Result.Outcome.Outcome != "cancelled") || (permissionMode == "permission-select" && (request.Result.Outcome.Outcome != "selected" || request.Result.Outcome.OptionID != "allow")) || (permissionMode == "permission-deny" && (request.Result.Outcome.Outcome != "selected" || request.Result.Outcome.OptionID != "deny")) || ((permissionMode == "permission-cancel" || permissionMode == "permission-ambiguous") && request.Result.Outcome.Outcome != "cancelled") || string(request.ID) != `"permission-1"` || pending == nil {
				os.Exit(5)
			}
			if permissionMode != "cancel-late" {
				reply(pending, map[string]string{"stopReason": "end_turn"})
			}
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

func TestPermissionReplyIsOneShotAndUsesOfferedOption(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	requests := make(chan PermissionRequest, 1)
	env := append(os.Environ(), "MODEX_ACP_FIXTURE=1")
	client, err := startWithPermissions(ctx, os.Args[0], []string{"-test.run=^TestACPChild$"}, t.TempDir(), env, nil, func(p PermissionRequest) bool { requests <- p; return true })
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	id, err := client.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	sent := make(chan struct{}, 1)
	go func() {
		_, err := client.PromptWithSent(ctx, id, "turn", "permission-select", func() { sent <- struct{}{} })
		finished <- err
	}()
	select {
	case <-sent:
	case <-ctx.Done():
		t.Fatal("prompt not sent")
	}
	var p PermissionRequest
	select {
	case p = <-requests:
	case <-ctx.Done():
		t.Fatal("permission missing")
	}
	if p.ID == "" || p.SessionID != id || p.TurnID != "turn" || p.Title != "Run command" || p.AllowOption != "allow" || p.DenyOption != "deny" {
		t.Fatalf("permission: %+v", p)
	}
	if err := client.ReplyPermission(ctx, p.ID, "always"); err == nil {
		t.Fatal("accepted unsafe option")
	}
	if err := client.ReplyPermission(ctx, p.ID, "allow"); err != nil {
		t.Fatal(err)
	}
	if err := client.ReplyPermission(ctx, p.ID, "allow"); err == nil {
		t.Fatal("accepted duplicate response")
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestPermissionDenyCancelAndExpiry(t *testing.T) {
	for _, tc := range []struct {
		prompt, option string
		cancelTurn     bool
	}{{"permission-deny", "deny", false}, {"permission-cancel", "", false}, {"permission-select", "allow", true}} {
		t.Run(tc.prompt+tc.option, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			requests := make(chan PermissionRequest, 1)
			env := append(os.Environ(), "MODEX_ACP_FIXTURE=1")
			client, err := startWithPermissions(ctx, os.Args[0], []string{"-test.run=^TestACPChild$"}, t.TempDir(), env, nil, func(p PermissionRequest) bool { requests <- p; return true })
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			id, err := client.NewSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, err := client.Prompt(ctx, id, "turn", tc.prompt); done <- err }()
			var p PermissionRequest
			select {
			case p = <-requests:
			case <-ctx.Done():
				t.Fatal("permission missing")
			}
			if tc.cancelTurn {
				if err := client.Cancel(ctx, id, "turn"); err != nil {
					t.Fatal(err)
				}
				if err := client.ReplyPermission(ctx, p.ID, tc.option); err == nil {
					t.Fatal("accepted expired decision")
				}
			} else if err := client.ReplyPermission(ctx, p.ID, tc.option); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if err := client.ReplyPermission(ctx, p.ID, tc.option); err == nil {
				t.Fatal("accepted stale decision")
			}
		})
	}
}

func TestPermissionAmbiguousOptionsAreUnavailable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	requests := make(chan PermissionRequest, 1)
	env := append(os.Environ(), "MODEX_ACP_FIXTURE=1")
	client, err := startWithPermissions(ctx, os.Args[0], []string{"-test.run=^TestACPChild$"}, t.TempDir(), env, nil, func(p PermissionRequest) bool { requests <- p; return true })
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	id, err := client.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := client.Prompt(ctx, id, "turn", "permission-ambiguous"); done <- err }()
	var p PermissionRequest
	select {
	case p = <-requests:
	case <-ctx.Done():
		t.Fatal("permission missing")
	}
	if p.AllowOption != "" || p.DenyOption != "" {
		t.Fatalf("ambiguous options exposed: %+v", p)
	}
	if err := client.ReplyPermission(ctx, p.ID, "allow"); err == nil {
		t.Fatal("ambiguous option accepted")
	}
	if err := client.ReplyPermission(ctx, p.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPermissionCallbackDoesNotHoldClientLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	requests := make(chan PermissionRequest, 1)
	release := make(chan struct{})
	env := append(os.Environ(), "MODEX_ACP_FIXTURE=1")
	client, err := startWithPermissions(ctx, os.Args[0], []string{"-test.run=^TestACPChild$"}, t.TempDir(), env, nil, func(p PermissionRequest) bool { requests <- p; <-release; return true })
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	id, err := client.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := client.Prompt(ctx, id, "turn", "permission-select"); done <- err }()
	var p PermissionRequest
	select {
	case p = <-requests:
	case <-ctx.Done():
		t.Fatal("permission missing")
	}
	cancelled := make(chan error, 1)
	go func() { cancelled <- client.Cancel(ctx, id, "turn") }()
	select {
	case err := <-cancelled:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("cancel blocked by permission callback")
	}
	if err := client.ReplyPermission(ctx, p.ID, p.AllowOption); err == nil {
		t.Fatal("cancelled permission accepted")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPermissionAfterStopIsCancelled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	requests := make(chan PermissionRequest, 1)
	updates := make(chan Update, 2)
	env := append(os.Environ(), "MODEX_ACP_FIXTURE=1")
	client, err := startWithPermissions(ctx, os.Args[0], []string{"-test.run=^TestACPChild$"}, t.TempDir(), env, func(u Update) { updates <- u }, func(p PermissionRequest) bool { requests <- p; return true })
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	id, err := client.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := client.Prompt(ctx, id, "turn", "cancel-late"); done <- err }()
	select {
	case <-updates:
	case <-ctx.Done():
		t.Fatal("update missing")
	}
	if err := client.Cancel(ctx, id, "turn"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-requests:
		t.Fatalf("permission after stop exposed: %+v", p)
	default:
	}
}

func TestSessionOptionsAreScopedValidatedAndAcknowledged(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	c, err := start(ctx, os.Args[0], []string{"-test.run=^TestACPChild$"}, t.TempDir(), append(os.Environ(), "MODEX_ACP_FIXTURE=1"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = c.List(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err = c.Load(ctx, "saved"); err != nil {
		t.Fatal(err)
	}
	id, err := c.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	options := c.Options(id)
	if len(options) != 2 || options[0].CurrentValue != "a" || len(options[0].Values) != 2 {
		t.Fatalf("new options: %+v", options)
	}
	options[0].Values[0].Value = "mutated"
	if c.Options(id)[0].Values[0].Value != "a" {
		t.Fatal("options expose mutable state")
	}
	if err := c.SetOption(ctx, id, "model", "invalid"); err == nil {
		t.Fatal("accepted invalid option")
	}
	if err := c.SetOption(ctx, id, "model", "b"); err != nil {
		t.Fatal(err)
	}
	if got := c.Options(id); len(got) != 2 || got[0].CurrentValue != "b" || got[1].CurrentValue != "high" {
		t.Fatalf("acknowledged options: %+v", got)
	}
	if got := c.Options("saved"); len(got) != 1 || got[0].CurrentValue != "low" {
		t.Fatalf("load or session isolation: %+v", got)
	}
	if err := c.SetOption(ctx, id, "model", "a"); err == nil {
		t.Fatal("accepted stale option")
	}
}

func TestSubagentEventsKeepOnlySafeScalars(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	events := make(chan Update, 4)
	c, err := start(ctx, os.Args[0], []string{"-test.run=^TestACPChild$"}, t.TempDir(), append(os.Environ(), "MODEX_ACP_FIXTURE=1"), func(u Update) { events <- u })
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	id, err := c.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prompt(ctx, id, "turn", "tools"); err != nil {
		t.Fatal(err)
	}
	start, end := <-events, <-events
	if start.Kind != "tool_call" || start.ToolCallID != "call-1" || !start.Subagent || start.Role != "reviewer" || start.Status != "in_progress" || start.TurnID != "turn" {
		t.Fatalf("start: %+v", start)
	}
	if end.Kind != "tool_call_update" || end.ToolCallID != "call-1" || end.Subagent || end.Status != "completed" {
		t.Fatalf("end: %+v", end)
	}
	raw, _ := json.Marshal([]Update{start, end})
	if strings.Contains(string(raw), "SECRET") {
		t.Fatal("retained private tool data")
	}
}

func TestOptionUpdatesAndBusySession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	events := make(chan Update, 4)
	c, err := start(ctx, os.Args[0], []string{"-test.run=^TestACPChild$"}, t.TempDir(), append(os.Environ(), "MODEX_ACP_FIXTURE=1"), func(u Update) { events <- u })
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	id, err := c.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Prompt(ctx, id, "update", "config-update"); err != nil {
		t.Fatal(err)
	}
	if got := c.Options(id); len(got) != 1 || got[0].CurrentValue != "b" {
		t.Fatalf("notification: %+v", got)
	}
	if err := c.SetOption(ctx, id, "model", "a"); err == nil {
		t.Fatal("accepted invalidated value")
	}
	done := make(chan error, 1)
	go func() { _, err := c.Prompt(ctx, id, "busy", "cancel"); done <- err }()
	<-events
	if err := c.SetOption(ctx, id, "model", "b"); err == nil {
		t.Fatal("accepted change during prompt")
	}
	if err := c.Cancel(ctx, id, "busy"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestConflictingOptionUpdateRequiresReload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	c, err := start(ctx, os.Args[0], []string{"-test.run=^TestACPChild$"}, t.TempDir(), append(os.Environ(), "MODEX_ACP_FIXTURE=1", "MODEX_ACP_CONFLICT=1"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	id, err := c.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetOption(ctx, id, "model", "b"); err == nil {
		t.Fatal("conflicting notification accepted as confirmed")
	}
	if len(c.Options(id)) != 0 {
		t.Fatal("conflicting options remained available")
	}
}

func TestSessionLoadedDistinguishesRejectionFromUncertainExchange(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	c, err := start(ctx, os.Args[0], []string{"-test.run=^TestACPChild$"}, t.TempDir(), append(os.Environ(), "MODEX_ACP_FIXTURE=1", "MODEX_ACP_SET_UNCERTAIN=1"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	id, err := c.NewSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetOption(ctx, id, "model", "invalid"); err == nil || !c.SessionLoaded(id) {
		t.Fatal("local rejection should preserve loaded session")
	}
	if err := c.SetOption(ctx, id, "model", "b"); err == nil || c.SessionLoaded(id) {
		t.Fatal("uncertain exchange should invalidate session")
	}
}
