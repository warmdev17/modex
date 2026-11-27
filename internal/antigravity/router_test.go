package antigravity

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/warmdev/modex/internal/acp"
	"github.com/warmdev/modex/internal/bridge"
)

func TestRouterRejectsUnknownTaggedThread(t *testing.T) {
	r := New(context.Background(), "missing-agy", t.TempDir(), t.TempDir())
	defer r.Close()
	_, handled, err := r.Handle(context.Background(), bridge.Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"agy:missing"}`)}, nil)
	if !handled || err == nil {
		t.Fatalf("unknown AGY thread: handled=%v err=%v", handled, err)
	}
}

func TestStoredThreadsStayWorkspaceBound(t *testing.T) {
	root := t.TempDir()
	state := t.TempDir()
	data := `[{"id":"agy:123","native_id":"native","workspace":` + quote(root) + `}]`
	if err := os.WriteFile(filepath.Join(state, "antigravity-threads.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	r := New(context.Background(), "missing-agy", root, state)
	defer r.Close()
	threads := r.Threads()
	if len(threads) != 1 || threads[0]["id"] != "agy:123" {
		t.Fatalf("threads: %#v", threads)
	}
	_, handled, err := r.Handle(context.Background(), bridge.Command{Method: "threads/status", Params: json.RawMessage(`{"thread_id":"agy:123","workspace":"/other"}`)}, nil)
	if !handled || err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("workspace mismatch: handled=%v err=%v", handled, err)
	}
}
func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestRouterACPFixture(t *testing.T) {
	if os.Getenv("MODEX_ROUTER_FIXTURE") != "1" {
		return
	}
	cwd, _ := os.Getwd()
	s := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	var promptID json.RawMessage
	for s.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Session string `json:"sessionId"`
				Prompt  []struct {
					Text string `json:"text"`
				} `json:"prompt"`
			} `json:"params"`
		}
		if json.Unmarshal(s.Bytes(), &req) != nil {
			os.Exit(2)
		}
		reply := func(v any) { _ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": v}) }
		switch req.Method {
		case "initialize":
			reply(json.RawMessage(`{"protocolVersion":1,"agentInfo":{"name":"Antigravity ACP Server","version":"1"},"agentCapabilities":{"loadSession":true,"sessionCapabilities":{"list":{}}}}`))
		case "":
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": promptID, "result": map[string]string{"stopReason": "end_turn"}})
		case "session/cancel":
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": promptID, "result": map[string]string{"stopReason": "cancelled"}})
		case "session/new":
			reply(map[string]string{"sessionId": "native"})
		case "session/list":
			reply(map[string]any{"sessions": []any{map[string]string{"sessionId": "native", "cwd": cwd}}})
		case "session/load":
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "native", "update": map[string]any{"sessionUpdate": "user_message_chunk", "content": map[string]string{"type": "text", "text": "restored"}}}})
			reply(map[string]any{})
		case "session/prompt":
			if req.Params.Prompt[0].Text == "wait" {
				promptID = req.ID
				continue
			}
			if req.Params.Prompt[0].Text == "permission" {
				promptID = req.ID
				_ = out.Encode(json.RawMessage(`{"jsonrpc":"2.0","id":"request-1","method":"session/request_permission","params":{"sessionId":"native","toolCall":{"title":"Run tool"},"options":[{"optionId":"yes","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]}}`))
				continue
			}
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "native", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "hello"}}}})
			reply(map[string]string{"stopReason": "end_turn"})
		default:
			os.Exit(3)
		}
	}
	os.Exit(0)
}

func TestSessionStreamAndRestart(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	bin := filepath.Join(t.TempDir(), "agy")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nMODEX_ROUTER_FIXTURE=1 exec " + strconv.Quote(executable) + " -test.run=^TestRouterACPFixture$\n"
	if err = os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	r := New(context.Background(), bin, root, state)
	result, handled, err := r.Handle(context.Background(), bridge.Command{Method: "threads/start", Params: json.RawMessage(`{"agent":"antigravity"}`)}, nil)
	if err != nil || !handled {
		t.Fatalf("start: %v", err)
	}
	id := result.(map[string]any)["thread"].(map[string]any)["id"].(string)
	var mu sync.Mutex
	var events []string
	complete := make(chan struct{})
	emit := func(method string, _ json.RawMessage) {
		mu.Lock()
		events = append(events, method)
		mu.Unlock()
		if method == "turn/completed" {
			close(complete)
		}
	}
	_, _, err = r.Handle(context.Background(), bridge.Command{Method: "turn/start", Params: json.RawMessage(`{"thread_id":` + quote(id) + `,"text":"hi"}`)}, emit)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-complete:
	case <-time.After(3 * time.Second):
		t.Fatal("missing completion")
	}
	mu.Lock()
	got := strings.Join(events, ",")
	mu.Unlock()
	if got != "turn/started,item/completed,item/agentMessage/delta,turn/completed" {
		t.Fatalf("event ordering: %s", got)
	}
	r.Close()
	r = New(context.Background(), bin, root, state)
	defer r.Close()
	result, _, err = r.Handle(context.Background(), bridge.Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":` + quote(id) + `,"include_turns":true,"limit":10}`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	history := result.(map[string]any)["messages"].([]message)
	if len(history) != 1 || history[0].Text != "restored" {
		t.Fatalf("recovered history: %#v", history)
	}
}

func TestDecisionRejectsUnrequestedFields(t *testing.T) {
	r := New(context.Background(), "missing", t.TempDir(), t.TempDir())
	defer r.Close()
	r.pending["agy:decision:test"] = pending{request: acp.PermissionRequest{AllowOption: "yes"}}
	_, handled, err := r.Handle(context.Background(), bridge.Command{Method: "decision/respond", Params: json.RawMessage(`{"decision_id":"agy:decision:test","result":{"decision":"accept","extra":true}}`)}, nil)
	if !handled || err == nil {
		t.Fatalf("unexpected decision accepted: %v", err)
	}
	if _, ok := r.pending["agy:decision:test"]; !ok {
		t.Fatal("invalid decision removed pending request")
	}
}

func TestPermissionIsReplayedAndOneShot(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	bin := filepath.Join(t.TempDir(), "agy")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(bin, []byte("#!/bin/sh\nMODEX_ROUTER_FIXTURE=1 exec "+strconv.Quote(exe)+" -test.run=^TestRouterACPFixture$\n"), 0700); err != nil {
		t.Fatal(err)
	}
	r := New(context.Background(), bin, root, state)
	defer r.Close()
	result, _, err := r.Handle(context.Background(), bridge.Command{Method: "threads/start", Params: json.RawMessage(`{"agent":"antigravity"}`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := result.(map[string]any)["thread"].(map[string]any)["id"].(string)
	decisions := make(chan json.RawMessage, 2)
	complete := make(chan struct{})
	emit := func(method string, raw json.RawMessage) {
		if method == "decision/required" {
			decisions <- raw
		}
		if method == "turn/completed" {
			close(complete)
		}
	}
	_, _, err = r.Handle(context.Background(), bridge.Command{Method: "turn/start", Params: json.RawMessage(`{"thread_id":` + quote(id) + `,"text":"permission"}`)}, emit)
	if err != nil {
		t.Fatal(err)
	}
	var decision struct {
		ID string `json:"decision_id"`
	}
	select {
	case raw := <-decisions:
		if json.Unmarshal(raw, &decision) != nil {
			t.Fatal("invalid decision")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing decision")
	}
	_, _, _ = r.Handle(context.Background(), bridge.Command{Method: "client/attached", Params: json.RawMessage(`{"attached":true}`)}, emit)
	select {
	case <-decisions:
	case <-time.After(time.Second):
		t.Fatal("missing replay")
	}
	answer := bridge.Command{Method: "decision/respond", Params: json.RawMessage(`{"decision_id":` + quote(decision.ID) + `,"result":{"decision":"decline"}}`)}
	if _, _, err = r.Handle(context.Background(), answer, emit); err != nil {
		t.Fatal(err)
	}
	if _, _, err = r.Handle(context.Background(), answer, emit); err == nil {
		t.Fatal("accepted decision twice")
	}
	select {
	case <-complete:
	case <-time.After(3 * time.Second):
		t.Fatal("missing completion")
	}
}

func TestInterruptMatchesActiveTurn(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	bin := filepath.Join(t.TempDir(), "agy")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(bin, []byte("#!/bin/sh\nMODEX_ROUTER_FIXTURE=1 exec "+strconv.Quote(exe)+" -test.run=^TestRouterACPFixture$\n"), 0700); err != nil {
		t.Fatal(err)
	}
	r := New(context.Background(), bin, root, state)
	defer r.Close()
	start, _, err := r.Handle(context.Background(), bridge.Command{Method: "threads/start", Params: json.RawMessage(`{"agent":"antigravity"}`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := start.(map[string]any)["thread"].(map[string]any)["id"].(string)
	completed := make(chan json.RawMessage, 1)
	emit := func(method string, raw json.RawMessage) {
		if method == "turn/completed" {
			completed <- raw
		}
	}
	turn, _, err := r.Handle(context.Background(), bridge.Command{Method: "turn/start", Params: json.RawMessage(`{"thread_id":` + quote(id) + `,"text":"wait"}`)}, emit)
	if err != nil {
		t.Fatal(err)
	}
	turnID := turn.(map[string]any)["turn"].(map[string]any)["id"].(string)
	interrupt := func(turn string) error {
		_, _, e := r.Handle(context.Background(), bridge.Command{Method: "turn/interrupt", Params: json.RawMessage(`{"thread_id":` + quote(id) + `,"turn_id":` + quote(turn) + `}`)}, emit)
		return e
	}
	if interrupt("wrong") == nil {
		t.Fatal("wrong turn was interrupted")
	}
	if err = interrupt(turnID); err != nil {
		t.Fatal(err)
	}
	select {
	case raw := <-completed:
		if !strings.Contains(string(raw), `"interrupted"`) {
			t.Fatalf("status: %s", raw)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no completion")
	}
}

func TestReadinessRejectsSuccessfulExitWithErrorStatus(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "agy")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s' '{\"status\":\"ERROR\"}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	r := New(context.Background(), bin, t.TempDir(), t.TempDir())
	defer r.Close()
	if ready := r.readiness()["ready"]; ready != false {
		t.Fatalf("error probe marked ready: %v", ready)
	}
}
