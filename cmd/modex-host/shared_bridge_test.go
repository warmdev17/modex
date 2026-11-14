package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/warmdev/modex/internal/bridge"
)

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
	for d.Decode(&r) == nil {
		if r.Method == "initialized" {
			continue
		}
		if r.Method == "thread/unsubscribe" {
			os.Exit(3)
		}
		if r.Method == "turn/start" {
			_ = e.Encode(map[string]any{"id": r.ID, "result": map[string]any{"turn": map[string]string{"id": "turn"}}})
			_ = e.Encode(map[string]any{"id": 77, "method": "item/commandExecution/requestApproval", "params": map[string]any{"threadId": "shared", "turnId": "turn", "itemId": "shell", "command": "echo shared"}})
			var answer struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
			}
			if d.Decode(&answer) != nil || answer.ID != 77 || string(answer.Result) != `{"decision":"accept"}` {
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

func TestSharedBackendDisconnectDoesNotInterruptApprovalOrTurn(t *testing.T) {
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
			if err := wsjson.Write(ctx, b, map[string]any{"id": r.ID, "result": map[string]string{"decision": "accept"}}); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	for {
		select {
		case method := <-events:
			if method == "turn/completed" {
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
