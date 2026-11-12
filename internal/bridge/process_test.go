package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReadFrameAllowsLargeUpstreamHistory(t *testing.T) {
	frame, err := readFrame(bufio.NewReader(strings.NewReader(strings.Repeat("x", 5*1024*1024) + "\n")))
	if err != nil || len(frame) != 5*1024*1024 {
		t.Fatalf("len=%d err=%v", len(frame), err)
	}
}

func TestReadFrameRejectsHistoryAboveUpstreamLimit(t *testing.T) {
	_, err := readFrame(bufio.NewReader(strings.NewReader(strings.Repeat("x", maxUpstreamFrame+1) + "\n")))
	if err == nil {
		t.Fatal("accepted oversized upstream frame")
	}
}

func TestChildProtocol(t *testing.T) {
	if os.Getenv("MODEX_FAKE_CHILD") != "1" {
		return
	}
	var request struct {
		ID     int             `json:"id"`
		Method string          `json:"method"`
		Result json.RawMessage `json:"result"`
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for decoder.Decode(&request) == nil {
		if request.Method == "initialized" {
			continue
		}
		if request.Method == "hang" {
			time.Sleep(time.Minute)
			continue
		}
		if request.Method == "crash" {
			os.Exit(2)
		}
		if request.Method == "late" {
			_ = encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{"ok": true}})
			time.Sleep(20 * time.Millisecond)
			_ = encoder.Encode(map[string]any{"method": "turn/completed", "params": map[string]any{"turnId": "t1"}})
			continue
		}
		if request.Method == "server_request" {
			_ = encoder.Encode(map[string]any{"id": 77, "method": "execCommandApproval", "params": map[string]any{"itemId": "item-1"}})
			var response struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
			}
			if decoder.Decode(&response) == nil {
				_ = encoder.Encode(map[string]any{"id": request.ID, "result": response.Result})
			}
			continue
		}
		if request.Method == "reject" {
			_ = encoder.Encode(map[string]any{"id": request.ID, "error": map[string]any{"code": -32600, "message": "secret"}})
			continue
		}
		_ = encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{"ok": true}})
	}
	if marker := os.Getenv("MODEX_CLOSE_MARKER"); marker != "" {
		_ = os.WriteFile(marker, []byte("closed"), 0600)
	}
	os.Exit(0)
}

func TestProcessPreservesAndAnswersServerRequest(t *testing.T) {
	t.Setenv("MODEX_FAKE_CHILD", "1")
	p, err := StartProcess(context.Background(), os.Args[0], []string{"-test.run=^TestChildProtocol$"}, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	requests := make(chan string, 1)
	p.SetRequestHandler(func(id json.RawMessage, method string, _ json.RawMessage) {
		requests <- method
		if err := p.Respond(context.Background(), id, json.RawMessage(`{"decision":"accept"}`)); err != nil {
			t.Errorf("respond: %v", err)
		}
	})
	result, err := p.Call(context.Background(), "server_request", nil)
	if err != nil || string(result) != `{"decision":"accept"}` {
		t.Fatalf("result=%s err=%v", result, err)
	}
	select {
	case method := <-requests:
		if method != "execCommandApproval" {
			t.Fatalf("method=%s", method)
		}
	case <-time.After(time.Second):
		t.Fatal("server request was not forwarded")
	}
}

func TestProcessForwardsNotificationsAfterResponse(t *testing.T) {
	t.Setenv("MODEX_FAKE_CHILD", "1")
	p, err := StartProcess(context.Background(), os.Args[0], []string{"-test.run=^TestChildProtocol$"}, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	events := make(chan string, 1)
	if _, err := p.CallEvents(context.Background(), "late", nil, func(method string, _ json.RawMessage) { events <- method }); err != nil {
		t.Fatal(err)
	}
	select {
	case method := <-events:
		if method != "turn/completed" {
			t.Fatalf("event=%s", method)
		}
	case <-time.After(time.Second):
		t.Fatal("late notification was lost")
	}
}

func TestProcessKeepsConnectionAfterRequestRejection(t *testing.T) {
	t.Setenv("MODEX_FAKE_CHILD", "1")
	p, err := StartProcess(context.Background(), os.Args[0], []string{"-test.run=^TestChildProtocol$"}, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err = p.Call(context.Background(), "reject", nil); err == nil {
		t.Fatal("missing rejection")
	}
	if _, err = p.Call(context.Background(), "test", nil); err != nil {
		t.Fatal(err)
	}
}

func TestProcessMultipleCallsAndCleanup(t *testing.T) {
	t.Setenv("MODEX_FAKE_CHILD", "1")
	p, err := StartProcess(context.Background(), os.Args[0], []string{"-test.run=^TestChildProtocol$"}, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err = p.Call(context.Background(), "test", map[string]any{}); err != nil {
			t.Fatal(err)
		}
	}
	p.Close()
	if _, err = p.Call(context.Background(), "test", nil); err == nil {
		t.Fatal("called closed child")
	}
}

func TestProcessTimeoutAndCrashTerminateCleanly(t *testing.T) {
	t.Setenv("MODEX_FAKE_CHILD", "1")
	for _, method := range []string{"hang", "crash"} {
		p, err := StartProcess(context.Background(), os.Args[0], []string{"-test.run=^TestChildProtocol$"}, t.TempDir(), os.Environ())
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_, err = p.Call(ctx, method, nil)
		cancel()
		p.Close()
		if err == nil {
			t.Fatalf("%s did not fail", method)
		}
	}
}

func TestProcessCloseIdleAllowsEOFShutdown(t *testing.T) {
	t.Setenv("MODEX_FAKE_CHILD", "1")
	marker := t.TempDir() + "/graceful-close"
	t.Setenv("MODEX_CLOSE_MARKER", marker)
	p, err := StartProcess(context.Background(), os.Args[0], []string{"-test.run=^TestChildProtocol$"}, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err = p.Call(context.Background(), "status", nil); err != nil {
		t.Fatal(err)
	}
	p.CloseIdle()
	if raw, err := os.ReadFile(marker); err != nil || string(raw) != "closed" {
		t.Fatalf("child did not finish EOF cleanup: %v", err)
	}
}
