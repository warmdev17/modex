package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/warmdev/modex/internal/bridge"
	"github.com/warmdev/modex/internal/diagnostic"
)

func TestNativeReplyReportsSafeResumeFailure(t *testing.T) {
	for _, tc := range []struct {
		name, message, category, advice string
	}{
		{"writer", "thread already has an active writer; secret-key", "active_writer", "Close"},
		{"auth", "authentication failed; secret-key", "unauthorized", "authentication"},
		{"provider", "model provider unavailable; secret-key", "model_provider_mismatch", "provider"},
		{"unknown", "secret-key", "request_rejected", "refresh"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream, _ := json.Marshal(map[string]any{"code": -32600, "message": tc.message})
			client := &nativeClient{out: make(chan []byte, 1)}
			client.reply(json.RawMessage(`42`), nil, fmt.Errorf("resume: %w", diagnostic.NewRPCError(upstream)))
			raw := <-client.out
			var reply struct {
				ID    int `json:"id"`
				Error struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
					Data    struct {
						Category string `json:"category"`
					} `json:"data"`
				} `json:"error"`
			}
			if json.Unmarshal(raw, &reply) != nil || reply.ID != 42 || reply.Error.Code != -32000 || reply.Error.Data.Category != tc.category || !strings.Contains(reply.Error.Message, tc.advice) || strings.Contains(string(raw), "secret-key") {
				t.Fatalf("unsafe or unhelpful reply: %s", raw)
			}
		})
	}
	client := &nativeClient{out: make(chan []byte, 1)}
	client.reply(json.RawMessage(`43`), nil, errors.New("secret-key"))
	if raw := <-client.out; strings.Contains(string(raw), "secret-key") {
		t.Fatalf("raw error leaked: %s", raw)
	}
}

func TestSharedCodexChild(t *testing.T) {
	if os.Getenv("MODEX_SHARED_CHILD") != "1" {
		return
	}
	d, e := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	var r struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Result json.RawMessage `json:"result"`
	}
	resumeCount := 0
	for d.Decode(&r) == nil {
		if r.Method == "initialized" {
			continue
		}
		if r.Method == "thread/unsubscribe" {
			os.Exit(3)
		}
		if r.Method == "turn/start" && os.Getenv("MODEX_REJECT_TURN") == "1" {
			_ = e.Encode(map[string]any{"id": r.ID, "error": map[string]any{"code": -32600, "message": "rejected"}})
			continue
		}
		if os.Getenv("MODEX_SHARED_INTERRUPT") == "1" {
			if r.Method == "turn/start" {
				_ = e.Encode(map[string]any{"id": r.ID, "result": map[string]any{"turn": map[string]string{"id": "turn", "status": "inProgress"}}})
				_ = e.Encode(map[string]any{"method": "turn/started", "params": map[string]any{"threadId": "shared", "turn": map[string]string{"id": "turn", "status": "inProgress"}}})
				continue
			}
			if r.Method == "turn/interrupt" {
				_ = e.Encode(map[string]any{"id": r.ID, "result": map[string]any{}})
				_ = e.Encode(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "shared", "turn": map[string]string{"id": "turn", "status": "interrupted"}}})
				_ = e.Encode(map[string]any{"method": "thread/status/changed", "params": map[string]any{"threadId": "shared", "status": map[string]string{"type": "idle"}}})
				continue
			}
		}
		if r.Method == "turn/start" {
			_ = e.Encode(map[string]any{"id": r.ID, "result": map[string]any{"turn": map[string]string{"id": "turn"}}})
			_ = e.Encode(map[string]any{"id": 77, "method": "item/commandExecution/requestApproval", "params": map[string]any{"threadId": "shared", "turnId": "turn", "itemId": "shell", "command": "echo shared"}})
			var answer struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if d.Decode(&answer) != nil || answer.ID != 77 {
				os.Exit(2)
			}
			if os.Getenv("MODEX_SHARED_ERROR") == "1" {
				if string(answer.Error) != `{"code":-32601,"message":"unsupported"}` {
					os.Exit(2)
				}
			} else if string(answer.Result) != `{"decision":"accept"}` {
				os.Exit(2)
			}
			_ = e.Encode(map[string]any{"method": "item/agentMessage/delta", "params": map[string]string{"threadId": "shared", "delta": "still running"}})
			_ = e.Encode(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "shared", "turn": map[string]string{"id": "turn", "status": "completed"}}})
			continue
		}
		var result any = map[string]any{}
		if r.Method == "initialize" {
			result = map[string]string{"userAgent": "shared-test", "platformFamily": "unix", "platformOs": "linux"}
		}
		if r.Method == "thread/resume" && os.Getenv("MODEX_LEGACY_RESUME") == "1" {
			resumeCount++
			if resumeCount == 1 {
				_ = e.Encode(map[string]any{"id": r.ID, "error": map[string]any{"code": -32600, "message": "list_turns is not supported"}})
				continue
			}
		}
		if r.Method == "thread/resume" {
			result = map[string]any{"thread": map[string]any{"id": "shared", "cwd": "/work", "status": map[string]string{"type": "idle"}}}
		}
		if r.Method == "config/read" {
			result = map[string]any{"config": map[string]string{"model": "fixture", "model_provider": "fixture"}}
		}
		_ = e.Encode(map[string]any{"id": r.ID, "result": result})
	}
	os.Exit(0)
}

func TestMobileAttachmentSnapshotsOnlyPendingReviewableDecisions(t *testing.T) {
	var events []struct {
		Method string
		Params json.RawMessage
	}
	s := &sharedBackend{pending: map[string]bridge.Decision{
		"77": {ID: "approval", Method: "item/commandExecution/requestApproval", ThreadID: "shared", TurnID: "turn", ItemID: "shell", Context: map[string]json.RawMessage{"command": json.RawMessage(`"echo shared"`)}},
		"78": {ID: "question", Method: "item/tool/requestUserInput", ThreadID: "shared", TurnID: "turn", ItemID: "question", Context: map[string]json.RawMessage{"questions": json.RawMessage(`[{"id":"choice","question":"Choose","options":[{"label":"Yes"}]}]`)}},
		"79": {Method: "unsupported/native-only"},
	}}
	emit := func(method string, params json.RawMessage) {
		events = append(events, struct {
			Method string
			Params json.RawMessage
		}{method, append(json.RawMessage(nil), params...)})
	}
	if _, err := s.call(context.Background(), "client/attached", map[string]any{"attached": true}, emit); err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Method != "decision/reset" {
		t.Fatalf("expected authoritative pending snapshot, got %d events", len(events))
	}
	seen := map[string]bool{}
	for _, event := range events[1:] {
		var d bridge.Decision
		if event.Method != "decision/required" || json.Unmarshal(event.Params, &d) != nil || d.ID == "" || d.Context == nil {
			t.Fatal("snapshot lost review context or included an unsupported request")
		}
		seen[d.ID] = true
	}
	if !seen["approval"] || !seen["question"] {
		t.Fatal("missing approval or question")
	}
	delete(s.pending, "77")
	delete(s.pending, "78")
	_, _ = s.call(context.Background(), "client/attached", map[string]any{"attached": false}, emit)
	if len(events) != 3 {
		t.Fatal("detach must not emit a snapshot")
	}
	_, _ = s.call(context.Background(), "client/attached", map[string]any{"attached": true}, emit)
	if len(events) != 4 || events[3].Method != "decision/reset" {
		t.Fatal("empty snapshot must clear decisions resolved while disconnected")
	}
}

func TestSharedBackendDisconnectDoesNotInterruptApprovalOrTurn(t *testing.T) {
	testSharedBackendDisconnect(t, false)
}
func TestSharedBackendNativeErrorResolvesPendingApproval(t *testing.T) {
	testSharedBackendDisconnect(t, true)
}
func testSharedBackendDisconnect(t *testing.T, nativeError bool) {
	if nativeError {
		t.Setenv("MODEX_SHARED_ERROR", "1")
	}
	t.Setenv("MODEX_SHARED_CHILD", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	p, err := bridge.StartProcess(ctx, os.Args[0], []string{"-test.run=^TestSharedCodexChild$"}, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	events := make(chan string, 16)
	s, err := newSharedBackend(ctx, p, func(method string, _ json.RawMessage) { events <- method })
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s)
	defer server.Close()
	connect := func() *websocket.Conn {
		c, _, err := websocket.Dial(ctx, "ws"+server.URL[4:], nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.CloseNow() })
		return c
	}
	read := func(c *websocket.Conn, id int) map[string]json.RawMessage {
		for {
			var r map[string]json.RawMessage
			if err := wsjson.Read(ctx, c, &r); err != nil {
				t.Fatal(err)
			}
			var got int
			if json.Unmarshal(r["id"], &got) == nil && got == id && r["method"] == nil {
				return r
			}
		}
	}
	request := func(c *websocket.Conn, id int, method string, params any) map[string]json.RawMessage {
		if err := wsjson.Write(ctx, c, map[string]any{"id": id, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		return read(c, id)
	}
	a, b := connect(), connect()
	if r := request(a, 1, "config/read", nil); r["error"] == nil {
		t.Fatal("uninitialized client accepted")
	}
	for _, c := range []*websocket.Conn{a, b} {
		if r := request(c, 2, "initialize", map[string]any{}); r["error"] != nil {
			t.Fatalf("initialize: %s", r)
		}
		if r := request(c, 3, "thread/resume", map[string]string{"threadId": "shared"}); r["error"] != nil {
			t.Fatalf("resume: %s", r)
		}
	}
	if r := request(a, 5, "thread/unsubscribe", map[string]string{"threadId": "shared"}); r["error"] != nil {
		t.Fatalf("unsubscribe: %s", r)
	}
	if r := request(a, 6, "thread/resume", map[string]string{"threadId": "shared"}); r["error"] != nil {
		t.Fatalf("unsubscribe unloaded shared thread: %s", r)
	}
	if r := request(a, 4, "turn/start", map[string]string{"threadId": "shared"}); r["error"] != nil {
		t.Fatalf("turn: %s", r)
	}
	a.CloseNow()
	for {
		var r struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := wsjson.Read(ctx, b, &r); err != nil {
			t.Fatal(err)
		}
		if r.Method == "item/commandExecution/requestApproval" {
			response := map[string]any{"id": r.ID, "result": map[string]string{"decision": "accept"}}
			if nativeError {
				response = map[string]any{"id": r.ID, "error": map[string]any{"code": -32601, "message": "unsupported"}}
			}
			if err := wsjson.Write(ctx, b, response); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	for {
		select {
		case method := <-events:
			if method == "turn/completed" {
				if len(s.decisions.List()) != 0 {
					t.Fatal("answered decision remains pending")
				}
				if _, err := s.call(ctx, "config/read", nil, nil); err != nil {
					t.Fatalf("backend died: %v", err)
				}
				return
			}
		case <-ctx.Done():
			t.Fatal("turn stopped after CLI disconnected")
		}
	}
}

func TestNativeInterruptReachesMobileWithoutSelectingThread(t *testing.T) {
	t.Setenv("MODEX_SHARED_CHILD", "1")
	t.Setenv("MODEX_SHARED_INTERRUPT", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p, err := bridge.StartProcess(ctx, os.Args[0], []string{"-test.run=^TestSharedCodexChild$"}, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	mobile := make(chan []byte, 16)
	s, err := newSharedBackend(ctx, p, func(method string, params json.RawMessage) {
		raw, err := bridge.EncodeEvent(method, params)
		if err != nil {
			t.Errorf("encode: %v", err)
			return
		}
		mobile <- raw
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s)
	defer server.Close()
	c, _, err := websocket.Dial(ctx, "ws"+server.URL[4:], nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	nativeCompleted := false
	request := func(id int, method string) {
		t.Helper()
		if err := wsjson.Write(ctx, c, map[string]any{"id": id, "method": method, "params": map[string]any{"threadId": "shared", "turnId": "turn"}}); err != nil {
			t.Fatal(err)
		}
		for {
			var reply map[string]json.RawMessage
			if err := wsjson.Read(ctx, c, &reply); err != nil {
				t.Fatal(err)
			}
			if string(reply["method"]) == `"turn/completed"` {
				var completion struct {
					Thread string `json:"threadId"`
					Turn   struct {
						Status string `json:"status"`
					} `json:"turn"`
				}
				_ = json.Unmarshal(reply["params"], &completion)
				if completion.Thread != "shared" || completion.Turn.Status != "interrupted" {
					t.Fatal("native outcome changed")
				}
				nativeCompleted = true
			}
			if string(reply["id"]) == fmt.Sprint(id) && reply["method"] == nil {
				if reply["error"] != nil {
					t.Fatalf("RPC error: %s", reply["error"])
				}
				break
			}
		}
	}
	request(1, "initialize")
	request(2, "thread/resume")
	request(3, "turn/start")
	request(4, "turn/interrupt")
	completed := false
	for !completed {
		select {
		case raw := <-mobile:
			var event struct {
				Method string `json:"event"`
				Params struct {
					Thread string `json:"threadId"`
					Turn   struct {
						ID     string `json:"id"`
						Status string `json:"status"`
					} `json:"turn"`
				} `json:"params"`
			}
			if json.Unmarshal(raw, &event) != nil {
				t.Fatal("bad event")
			}
			if event.Method == "turn/completed" {
				if event.Params.Thread != "shared" || event.Params.Turn.ID != "turn" || event.Params.Turn.Status != "interrupted" {
					t.Fatalf("wrong outcome: %s", raw)
				}
				completed = true
			}
		case <-ctx.Done():
			t.Fatal("native interruption did not reach mobile")
		}
	}
	// The native subscriber receives the original documented completion too.
	if nativeCompleted {
		return
	}
	for {
		var event struct {
			Method string `json:"method"`
			Params struct {
				Thread string `json:"threadId"`
				Turn   struct {
					Status string `json:"status"`
				} `json:"turn"`
			} `json:"params"`
		}
		if err := wsjson.Read(ctx, c, &event); err != nil {
			t.Fatal(err)
		}
		if event.Method == "turn/completed" {
			if event.Params.Thread != "shared" || event.Params.Turn.Status != "interrupted" {
				t.Fatal("native completion changed")
			}
			return
		}
	}
}

func TestSharedStatusRequiresLoadedSession(t *testing.T) {
	s := &sharedBackend{pending: map[string]bridge.Decision{}, sessions: map[string]*sharedSession{}}
	_, err := s.call(context.Background(), "modex/thread/status", map[string]any{"threadId": "t"}, nil)
	if !errors.Is(err, bridge.ErrSessionNotOwned) {
		t.Fatalf("missing owner: %v", err)
	}
	s.sessions["t"] = &sharedSession{busy: true, settings: map[string]json.RawMessage{"thread": json.RawMessage(`{"id":"t"}`)}}
	_, err = s.call(context.Background(), "modex/thread/mode", map[string]any{"threadId": "t"}, nil)
	if !errors.Is(err, bridge.ErrSessionBusy) {
		t.Fatalf("busy mutation: %v", err)
	}
	s.sessions["t"].busy = false
	s.pending["d"] = bridge.Decision{ThreadID: "t"}
	if _, err := s.call(context.Background(), "modex/thread/mode", map[string]any{"threadId": "t"}, nil); !errors.Is(err, bridge.ErrSessionBusy) {
		t.Fatalf("pending mutation: %v", err)
	}
	updateSharedSettings(s.sessions["t"], json.RawMessage(`{"threadSettings":{"model":"m","collaborationMode":{"mode":"plan","settings":{"developer_instructions":"SECRET"}},"approvalPolicy":"never","sandboxPolicy":{"type":"readOnly","secret":"SECRET"}}}`))
	raw, err := s.call(context.Background(), "modex/thread/status", map[string]any{"threadId": "t"}, nil)
	if err != nil || !strings.Contains(string(raw), `"token_usage":null`) || strings.Contains(string(raw), "SECRET") || !strings.Contains(string(raw), `"collaboration_mode":"plan"`) {
		t.Fatalf("status: %s %v", raw, err)
	}
}

func TestIsolatedSharedModeAndStatus(t *testing.T) {
	binary := os.Getenv("MODEX_ISOLATED_CODEX")
	if binary == "" {
		t.Skip("isolated native probe not requested")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := t.TempDir()
	env := append(os.Environ(), "CODEX_HOME="+root, "HOME="+root)
	p, err := bridge.StartProcess(ctx, binary, []string{"app-server"}, root, env)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	s, err := newSharedBackend(ctx, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.call(ctx, "thread/start", map[string]any{"cwd": root, "model": "fixture-model", "approvalPolicy": "never", "sandbox": "read-only"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var started struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if json.Unmarshal(result, &started) != nil || started.Thread.ID == "" {
		t.Fatal("missing thread")
	}
	id := started.Thread.ID
	if _, err = s.call(ctx, "thread/inject_items", map[string]any{"threadId": id, "items": []any{map[string]any{"type": "message", "role": "user", "content": []any{map[string]string{"type": "input_text", "text": "fixture"}}}}}, nil); err != nil {
		t.Fatal(err)
	}
	cmd := bridge.Command{Method: "threads/mode", Params: mustJSON(map[string]string{"workspace": root, "thread_id": id, "mode": "plan", "model": "fixture-model"})}
	got, err := bridge.HandleEvents(ctx, root, cmd, s.call, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(got)
	if !strings.Contains(string(raw), `"collaboration_mode":"plan"`) {
		t.Fatalf("mode: %s", raw)
	}
	cmd = bridge.Command{Method: "threads/status", Params: mustJSON(map[string]string{"workspace": root, "thread_id": id})}
	got, err = bridge.HandleEvents(ctx, root, cmd, s.call, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(got)
	if !strings.Contains(string(raw), `"token_usage":null`) || !strings.Contains(string(raw), `"rate_limits":null`) {
		t.Fatalf("status: %s", raw)
	}
}

func TestLegacyResumeRegistersSession(t *testing.T) {
	t.Setenv("MODEX_SHARED_CHILD", "1")
	t.Setenv("MODEX_LEGACY_RESUME", "1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := bridge.StartProcess(ctx, os.Args[0], []string{"-test.run=^TestSharedCodexChild$"}, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	s, err := newSharedBackend(ctx, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.call(ctx, "thread/resume", map[string]any{"threadId": "shared", "excludeTurns": true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = s.call(ctx, "modex/thread/status", map[string]any{"threadId": "shared"}, nil); err != nil {
		t.Fatalf("legacy status: %v", err)
	}
}
