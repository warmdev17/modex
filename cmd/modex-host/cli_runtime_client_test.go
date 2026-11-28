package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLocalCLIConcurrentEventsAndCalls(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		c, e := listener.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		d := json.NewDecoder(c)
		enc := json.NewEncoder(c)
		var hello map[string]any
		if d.Decode(&hello) != nil || hello["token"] != "secret" || hello["instance"] != "one" || hello["cli"] != true {
			return
		}
		_ = enc.Encode(map[string]any{"ready": true, "instance": "one"})
		var req map[string]any
		if d.Decode(&req) != nil {
			return
		}
		for i := 0; i < 300; i++ {
			_ = enc.Encode(map[string]any{"event": "notice", "params": map[string]any{}})
		}
		_ = enc.Encode(map[string]any{"id": req["id"], "result": map[string]any{"ok": true}})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, err := dialLocalCLI(ctx, supervisorEndpoint{Address: listener.Addr().String(), Token: "secret", Instance: "one"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	result, err := c.Call(ctx, "threads/list", map[string]any{})
	if err != nil || string(result) != `{"ok":true}` {
		t.Fatalf("result=%s err=%v", result, err)
	}
}

func TestCLISafeTextRemovesTerminalControls(t *testing.T) {
	got := cliSafeText("ok\x1b[31m\r\u202ebad\nnext\x00")
	if got != "ok[31mbad\nnext" {
		t.Fatalf("unsafe output: %q", got)
	}
}

type notifyCLIWriter struct {
	bytes.Buffer
	seen chan struct{}
	once sync.Once
}

func (w *notifyCLIWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if strings.Contains(w.Buffer.String(), "safe[31m") {
		w.once.Do(func() { close(w.seen) })
	}
	return n, err
}

func TestAgentCLIChatShowsSharedEventsAndKeepsSupervisor(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	turn := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		defer conn.Close()
		d := json.NewDecoder(conn)
		enc := json.NewEncoder(conn)
		var hello map[string]any
		if d.Decode(&hello) != nil {
			return
		}
		_ = enc.Encode(map[string]any{"ready": true, "instance": "one"})
		for {
			var req struct {
				ID     uint64         `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if d.Decode(&req) != nil {
				return
			}
			var result any = map[string]any{}
			switch req.Method {
			case "threads/resume":
				result = map[string]any{"thread": map[string]any{"id": "agy:one"}}
			case "threads/read":
				result = map[string]any{"messages": []any{map[string]any{"role": "user", "text": "old"}}, "active_turn": false}
			case "turn/start":
				if req.Params["text"] != "hello" {
					return
				}
				_ = enc.Encode(map[string]any{"event": "item/agentMessage/delta", "params": map[string]any{"threadId": "agy:other", "delta": "wrong thread"}})
				_ = enc.Encode(map[string]any{"event": "turn/started", "params": map[string]any{"threadId": "agy:one", "turn": map[string]any{"id": "t1"}}})
				_ = enc.Encode(map[string]any{"event": "item/agentMessage/delta", "params": map[string]any{"threadId": "agy:one", "delta": "safe\u001b[31m"}})
				_ = enc.Encode(map[string]any{"event": "turn/completed", "params": map[string]any{"threadId": "agy:one"}})
				result = map[string]any{"turn": map[string]any{"id": "t1"}}
				close(turn)
			}
			if enc.Encode(map[string]any{"id": req.ID, "result": result}) != nil {
				return
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, err := dialLocalCLI(ctx, supervisorEndpoint{Address: listener.Addr().String(), Token: "secret", Instance: "one"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	reader, writer := io.Pipe()
	defer writer.Close()
	out := notifyCLIWriter{seen: make(chan struct{})}
	chatDone := make(chan error, 1)
	go func() {
		chatDone <- runAgentCLIChat(ctx, bufio.NewReader(reader), &out, c, "antigravity", "AGY", "/work", "agy:one")
	}()
	if _, err = io.WriteString(writer, "hello\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-turn:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case <-out.seen:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err = io.WriteString(writer, "/quit\n"); err != nil {
		t.Fatal(err)
	}
	if err = <-chatDone; err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "old") || !strings.Contains(got, "safe[31m") || strings.Contains(got, "wrong thread") || strings.Contains(got, "\x1b[31m") {
		t.Fatalf("unexpected output: %q", got)
	}
	select {
	case <-done:
		t.Fatal("quit closed supervisor connection")
	default:
	}
}

func TestLocalCLIPreservesPendingDecisionBeforeChat(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, e := l.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		d := json.NewDecoder(c)
		enc := json.NewEncoder(c)
		var x map[string]any
		if d.Decode(&x) != nil {
			return
		}
		_ = enc.Encode(map[string]any{"ready": true, "instance": "one"})
		_ = enc.Encode(map[string]any{"event": "decision/required", "params": map[string]any{"thread_id": "agy:one", "decision_id": "d1"}})
		var req map[string]any
		if d.Decode(&req) != nil {
			return
		}
		_ = enc.Encode(map[string]any{"id": req["id"], "result": map[string]any{}})
		<-time.After(time.Second)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c, err := dialLocalCLI(ctx, supervisorEndpoint{Address: l.Addr().String(), Token: "secret", Instance: "one"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = c.Call(ctx, "threads/list", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	c.watchEvents()
	select {
	case ev := <-c.events:
		if ev.Event != "decision/required" || !strings.Contains(string(ev.Params), "d1") {
			t.Fatalf("event %v", ev)
		}
	case <-ctx.Done():
		t.Fatal("pending decision lost during picker")
	}
}

func TestCLIPermissionShowsAdditionalScope(t *testing.T) {
	got := strings.Join(cliDecisionExtra(json.RawMessage(`{"cwd":"/tmp","permissions":{"network":["example.com"]},"command":"ls","reason":"inspect","availableDecisions":["accept"]}`)), "\n")
	if !strings.Contains(got, "/tmp") || !strings.Contains(got, "example.com") || strings.Contains(got, "availableDecisions") {
		t.Fatalf("missing scope: %s", got)
	}
}

type formPromptWriter struct {
	seen chan struct{}
	once sync.Once
}

func (w *formPromptWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "Question one") {
		w.once.Do(func() { close(w.seen) })
	}
	return len(p), nil
}

func TestAgentCLIChatKeepsAnswersForQueuedForms(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	replies := make(chan map[string]any, 2)
	go func() {
		c, e := l.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		d := json.NewDecoder(c)
		enc := json.NewEncoder(c)
		var hello map[string]any
		if d.Decode(&hello) != nil {
			return
		}
		_ = enc.Encode(map[string]any{"ready": true, "instance": "one"})

		for {
			var req struct {
				ID     int            `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if d.Decode(&req) != nil {
				return
			}
			result := map[string]any{}
			switch req.Method {
			case "threads/resume":
				result["thread"] = map[string]any{"id": "oc:one"}
			case "threads/read":
				for _, form := range []struct{ id, key, question string }{{"d1", "key1", "Question one"}, {"d2", "key2", "Question two"}} {
					_ = enc.Encode(map[string]any{"event": "decision/required", "params": map[string]any{"thread_id": "oc:one", "decision_id": form.id, "method": "item/tool/requestUserInput", "context": map[string]any{"questions": []any{map[string]any{"id": form.key, "question": form.question}}}}})
				}
				result["messages"] = []any{}
			case "decision/respond":
				replies <- req.Params
			}
			if enc.Encode(map[string]any{"id": req.ID, "result": result}) != nil {
				return
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := dialLocalCLI(ctx, supervisorEndpoint{Address: l.Addr().String(), Token: "secret", Instance: "one"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	r, w := io.Pipe()
	defer w.Close()
	out := &formPromptWriter{seen: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- runAgentCLIChat(ctx, bufio.NewReader(r), out, client, "opencode", "OpenCode", "/work", "oc:one")
	}()
	select {
	case <-out.seen:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for len(client.events) > 0 {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if _, err = io.WriteString(w, "first answer\n"); err != nil {
		t.Fatal(err)
	}
	var first map[string]any
	select {
	case first = <-replies:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	encoded, _ := json.Marshal(first)
	if !strings.Contains(string(encoded), `"key1":{"answers":["first answer"]}`) {
		t.Fatalf("first form lost answer: %s", encoded)
	}
	if _, err = io.WriteString(w, "second answer\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case second := <-replies:
		encoded, _ = json.Marshal(second)
		if !strings.Contains(string(encoded), `"key2":{"answers":["second answer"]}`) {
			t.Fatalf("second form lost answer: %s", encoded)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	_, _ = io.WriteString(w, "/quit\n")
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

type historyOverlapWriter struct {
	bytes.Buffer
	done chan struct{}
	once sync.Once
}

func (w *historyOverlapWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if strings.Contains(w.Buffer.String(), "hist live") {
		w.once.Do(func() { close(w.done) })
	}
	return n, err
}
func TestAgentCLIChatReconcilesHistoryWithQueuedEvents(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, e := l.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		d := json.NewDecoder(c)
		enc := json.NewEncoder(c)
		var hello map[string]any
		if d.Decode(&hello) != nil {
			return
		}
		_ = enc.Encode(map[string]any{"ready": true, "instance": "one"})
		for {
			var req struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if d.Decode(&req) != nil {
				return
			}
			result := map[string]any{}
			switch req.Method {
			case "threads/resume":
				result["thread"] = map[string]any{"id": "oc:one"}
			case "threads/read":
				_ = enc.Encode(map[string]any{"event": "item/completed", "params": map[string]any{"threadId": "oc:one", "item": map[string]any{"id": "u1", "type": "userMessage", "content": []any{map[string]any{"text": "original prompt"}}}}})
				_ = enc.Encode(map[string]any{"event": "item/agentMessage/delta", "params": map[string]any{"threadId": "oc:one", "itemId": "a1", "delta": "his"}})
				_ = enc.Encode(map[string]any{"event": "item/agentMessage/delta", "params": map[string]any{"threadId": "oc:one", "itemId": "a1", "delta": "t plus"}})
				result["messages"] = []any{map[string]any{"id": "u1", "role": "user", "text": "original prompt"}, map[string]any{"id": "a1", "role": "assistant", "text": "hist"}}
				result["active_turn"] = true
				result["active_turn_id"] = "t1"
			}
			if enc.Encode(map[string]any{"id": req.ID, "result": result}) != nil {
				return
			}
			if req.Method == "threads/read" {
				_ = enc.Encode(map[string]any{"event": "item/agentMessage/delta", "params": map[string]any{"threadId": "oc:one", "itemId": "a1", "delta": "hist live"}})
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, err := dialLocalCLI(ctx, supervisorEndpoint{Address: l.Addr().String(), Token: "secret", Instance: "one"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r, w := io.Pipe()
	defer w.Close()
	out := &historyOverlapWriter{done: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- runAgentCLIChat(ctx, bufio.NewReader(r), out, c, "opencode", "OpenCode", "/work", "oc:one")
	}()
	select {
	case <-out.done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	_, _ = io.WriteString(w, "/quit\n")
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Count(got, "original prompt") != 1 || strings.Count(got, "hist") != 2 || !strings.Contains(got, " plus") || !strings.Contains(got, "hist live") {
		t.Fatalf("history overlap: %q", got)
	}
}
