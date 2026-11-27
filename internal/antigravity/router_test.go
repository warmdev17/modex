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
	var promptID, secondID json.RawMessage
	model, level := "fixture-default", "high"
	options := func() []map[string]any {
		return []map[string]any{
			{"id": "model", "type": "select", "name": "Model", "category": "model", "currentValue": model, "options": []map[string]string{
				{"value": "fixture-default", "name": "Default"}, {"value": "fixture-alt", "name": "Alternate"},
			}},
			{"id": "thought_level", "type": "select", "name": "Thinking level", "category": "thought_level", "currentValue": level, "options": []map[string]string{
				{"value": "low", "name": "Low"}, {"value": "high", "name": "High"},
			}},
		}
	}
	for s.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Session string `json:"sessionId"`
				Config  string `json:"configId"`
				Value   string `json:"value"`
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
			reply(json.RawMessage(`{"protocolVersion":1,"agentInfo":{"name":"Antigravity ACP Server","version":"1.5.17"},"agentCapabilities":{"loadSession":true,"promptCapabilities":{"image":true,"embeddedContext":true},"sessionCapabilities":{"list":{}}}}`))
		case "":
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": promptID, "result": map[string]string{"stopReason": "end_turn"}})
		case "session/cancel":
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": promptID, "result": map[string]string{"stopReason": "cancelled"}})
			if len(secondID) > 0 {
				_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": secondID, "result": map[string]string{"stopReason": "cancelled"}})
				secondID = nil
			}
		case "session/new":
			reply(map[string]any{"sessionId": "native", "configOptions": options()})
		case "session/list":
			reply(map[string]any{"sessions": []any{map[string]string{"sessionId": "native", "cwd": cwd}}})
		case "session/load":
			_ = out.Encode(json.RawMessage(`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"native","update":{"sessionUpdate":"user_message_chunk","content":{"type":"resource_link","uri":"file:///tmp/note%20%231.txt","mimeType":"text/plain"}}}}`))
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "native", "update": map[string]any{"sessionUpdate": "user_message_chunk", "content": map[string]string{"type": "text", "text": "restored"}}}})
			reply(map[string]any{"configOptions": options()})
		case "session/set_config_option":
			switch req.Params.Config {
			case "model":
				model, level = req.Params.Value, "high"
			case "thought_level":
				level = req.Params.Value
			default:
				os.Exit(4)
			}
			reply(map[string]any{"configOptions": options()})
		case "session/prompt":
			_ = os.WriteFile(filepath.Join(cwd, "captured-prompt.json"), s.Bytes(), 0600)
			if req.Params.Prompt[0].Text == "wait" {
				_ = out.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "native", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "before"}}}})
				promptID = req.ID
				continue
			}
			if req.Params.Prompt[0].Text == "both" {
				secondID = req.ID
				continue
			}
			if req.Params.Prompt[0].Text == "fast" {
				reply(map[string]string{"stopReason": "end_turn"})
				continue
			}
			if req.Params.Prompt[0].Text == "fail" {
				_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32001, "message": "private"}})
				continue
			}
			if req.Params.Prompt[0].Text == "follow" {
				_ = out.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "native", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "after"}}}})
				_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": promptID, "result": map[string]string{"stopReason": "end_turn"}})
				promptID = req.ID
				continue
			}
			if req.Params.Prompt[0].Text == "permission" {
				promptID = req.ID
				_ = out.Encode(json.RawMessage(`{"jsonrpc":"2.0","id":"request-1","method":"session/request_permission","params":{"sessionId":"native","toolCall":{"title":"Run tool"},"options":[{"optionId":"yes","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]}}`))
				continue
			}
			if req.Params.Prompt[0].Text == "subagent" {
				for _, update := range []string{
					`{"sessionUpdate":"tool_call","toolCallId":"a","status":"in_progress","name":"invoke_subagent","kind":"other","rawInput":{"Role":"reviewer","Prompt":"PRIVATE_PROBE_TEXT"}}`,
					`{"sessionUpdate":"tool_call_update","toolCallId":"unrelated","status":"completed"}`,
					`{"sessionUpdate":"tool_call_update","toolCallId":"a","status":"completed"}`,
				} {
					_ = out.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "native", "update": json.RawMessage(update)}})
				}
			}
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "native", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "hello"}}}})
			reply(map[string]string{"stopReason": "end_turn"})
		default:
			os.Exit(3)
		}
	}
	os.Exit(0)
}

func TestSessionOptionsRestoreAcknowledgedSettings(t *testing.T) {
	root, bin, state := stage3RouterFixture(t)
	r := New(context.Background(), bin, root, state)
	defer func() { r.Close() }()
	call := func(method string, params map[string]any) any {
		t.Helper()
		raw, _ := json.Marshal(params)
		value, handled, err := r.Handle(context.Background(), bridge.Command{Method: method, Params: raw}, nil)
		if err != nil || !handled {
			t.Fatalf("%s: handled=%v err=%v", method, handled, err)
		}
		return value
	}
	result := call("threads/start", map[string]any{"agent": "antigravity"})
	id := result.(map[string]any)["thread"].(map[string]any)["id"].(string)
	call("threads/options/set", map[string]any{"thread_id": id, "id": "model", "value": "fixture-alt"})
	call("threads/options/set", map[string]any{"thread_id": id, "id": "thought_level", "value": "low"})
	r.Close()
	r = New(context.Background(), bin, root, state)
	options := call("threads/options", map[string]any{"thread_id": id})
	raw, _ := json.Marshal(options)
	var snapshot struct {
		Options []struct {
			ID    string `json:"id"`
			Value string `json:"currentValue"`
		} `json:"options"`
	}
	if json.Unmarshal(raw, &snapshot) != nil || len(snapshot.Options) != 2 {
		t.Fatalf("invalid options: %s", raw)
	}
	values := map[string]string{}
	for _, option := range snapshot.Options {
		values[option.ID] = option.Value
	}
	if values["model"] != "fixture-alt" || values["thought_level"] != "low" {
		t.Fatalf("restored options = %v", values)
	}
}

func TestInvocationActivityUsesIdentifiedNativeCall(t *testing.T) {
	root, bin, state := stage3RouterFixture(t)
	r := New(context.Background(), bin, root, state)
	defer r.Close()
	result, _, err := r.Handle(context.Background(), bridge.Command{Method: "threads/start", Params: json.RawMessage(`{"agent":"antigravity"}`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := result.(map[string]any)["thread"].(map[string]any)["id"].(string)
	var activity []string
	done := make(chan struct{})
	emit := func(method string, raw json.RawMessage) {
		if strings.Contains(string(raw), "agentActivity") {
			activity = append(activity, string(raw))
		}
		if method == "turn/completed" {
			close(done)
		}
	}
	if _, _, err := r.Handle(context.Background(), bridge.Command{Method: "turn/start", Params: json.RawMessage(`{"thread_id":` + quote(id) + `,"text":"subagent"}`)}, emit); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("missing turn completion")
	}
	if len(activity) != 2 || !strings.Contains(activity[0], "reviewer") || !strings.Contains(activity[1], "invocationCompleted") {
		t.Fatalf("unexpected activity: %v", activity)
	}
	if strings.Contains(strings.Join(activity, " "), "PRIVATE_PROBE_TEXT") {
		t.Fatal("raw tool prompt escaped to the journal")
	}
}

func TestSessionOptionsRetryFailedSaveBeforeAcknowledging(t *testing.T) {
	root, bin, state := stage3RouterFixture(t)
	r := New(context.Background(), bin, root, state)
	defer func() { r.Close() }()
	call := func(method string, params map[string]any) (any, error) {
		t.Helper()
		raw, _ := json.Marshal(params)
		value, handled, err := r.Handle(context.Background(), bridge.Command{Method: method, Params: raw}, nil)
		if !handled {
			t.Fatalf("%s was not handled", method)
		}
		return value, err
	}
	result, err := call("threads/start", map[string]any{"agent": "antigravity"})
	if err != nil {
		t.Fatal(err)
	}
	id := result.(map[string]any)["thread"].(map[string]any)["id"].(string)
	broken := state + "-away"
	if err := os.Rename(state, broken); err != nil {
		t.Fatal(err)
	}
	if _, err := call("threads/options/set", map[string]any{"thread_id": id, "id": "model", "value": "fixture-alt"}); err == nil {
		t.Fatal("acknowledged an unsaved option change")
	}
	params := map[string]any{"thread_id": id}
	if _, err := call("threads/options", params); err == nil {
		t.Fatal("refresh acknowledged an unsaved configuration")
	}
	if err := os.Rename(broken, state); err != nil {
		t.Fatal(err)
	}
	if _, err := call("threads/options", params); err != nil {
		t.Fatal(err)
	}
	r.Close()
	r = New(context.Background(), bin, root, state)
	got, err := call("threads/options", params)
	if err != nil {
		t.Fatal(err)
	}
	if options := got.(map[string]any)["options"].([]map[string]any); options[0]["currentValue"] != "fixture-alt" {
		t.Fatalf("restored options: %v", options)
	}
}

func TestReplayInvocationUpdatesExistingCall(t *testing.T) {
	c := chat{loading: true}
	launch := acp.Update{Kind: "tool_call", ToolCallID: "a", Subagent: true, Status: "in_progress", Role: "reviewer"}
	c.updateInvocation(launch)
	c.updateInvocation(launch)
	c.updateInvocation(acp.Update{Kind: "tool_call_update", ToolCallID: "other", Status: "completed"})
	c.updateInvocation(acp.Update{Kind: "tool_call_update", ToolCallID: "a", Status: "completed"})
	c.updateInvocation(acp.Update{Kind: "tool_call", ToolCallID: "b", Subagent: true, Status: "pending"})
	if len(c.messages) != 2 {
		t.Fatalf("replayed %d invocations, want 2", len(c.messages))
	}
	first, second := c.messages[0].Activity.Targets[0], c.messages[1].Activity.Targets[0]
	if first.Status != "invocationCompleted" || second.Status != "unknown" || first.Label != "reviewer" {
		t.Fatalf("replayed statuses: %+v, %+v", first, second)
	}
}

func stage3RouterFixture(t *testing.T) (root, binary, state string) {
	t.Helper()
	root, state = t.TempDir(), t.TempDir()
	binary = filepath.Join(t.TempDir(), "agy")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(binary, []byte("#!/bin/sh\nMODEX_ROUTER_FIXTURE=1 exec "+strconv.Quote(exe)+" -test.run=^TestRouterACPFixture$\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return
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
	if len(history) != 1 || history[0].Text != "restored" || len(history[0].Attachments) != 1 || history[0].Attachments[0].Path != "/tmp/note #1.txt" {
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

func TestSteerKeepsActiveTurnUntilBothPromptsFinish(t *testing.T) {
	root, bin, state := stage3RouterFixture(t)
	r := New(context.Background(), bin, root, state)
	defer r.Close()
	call := func(method string, p map[string]any, emit func(string, json.RawMessage)) (any, error) {
		raw, _ := json.Marshal(p)
		v, _, err := r.Handle(context.Background(), bridge.Command{Method: method, Params: raw}, emit)
		return v, err
	}
	start, err := call("threads/start", map[string]any{"agent": "antigravity"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := start.(map[string]any)["thread"].(map[string]any)["id"].(string)
	completed := make(chan json.RawMessage, 1)
	firstDelta := make(chan struct{}, 1)
	emit := func(method string, raw json.RawMessage) {
		if method == "item/agentMessage/delta" && strings.Contains(string(raw), "before") {
			firstDelta <- struct{}{}
		}
		if method == "turn/completed" {
			completed <- raw
		}
	}
	turn, err := call("turn/start", map[string]any{"thread_id": id, "text": "wait"}, emit)
	if err != nil {
		t.Fatal(err)
	}
	turnID := turn.(map[string]any)["turn"].(map[string]any)["id"].(string)
	select {
	case <-firstDelta:
	case <-time.After(3 * time.Second):
		t.Fatal("missing initial delta")
	}
	if _, err = call("turn/steer", map[string]any{"thread_id": id, "turn_id": "stale", "text": "follow"}, emit); err == nil {
		t.Fatal("accepted stale turn")
	}
	for _, extra := range []map[string]any{{"text": " "}, {"text": "follow", "attachments": []any{}}, {"text": "follow", "references": []any{}}, {"text": "follow", "model": "x"}, {"text": "follow", "effort": "high"}} {
		extra["thread_id"] = id
		extra["turn_id"] = turnID
		if _, err = call("turn/steer", extra, emit); err == nil {
			t.Fatalf("accepted invalid steer %v", extra)
		}
	}
	got, err := call("turn/steer", map[string]any{"thread_id": id, "turn_id": turnID, "text": "follow"}, emit)
	if err != nil || got.(map[string]any)["turnId"] != turnID {
		t.Fatalf("steer: %v %v", got, err)
	}
	select {
	case <-completed:
		t.Fatal("completed before follow-up response")
	case <-time.After(100 * time.Millisecond):
	}
	if _, err = call("turn/steer", map[string]any{"thread_id": id, "turn_id": turnID, "text": "again"}, emit); err == nil {
		t.Fatal("accepted second outstanding steer")
	}
	if _, err = call("turn/interrupt", map[string]any{"thread_id": id, "turn_id": turnID}, emit); err != nil {
		t.Fatal(err)
	}
	select {
	case raw := <-completed:
		if !strings.Contains(string(raw), `"interrupted"`) {
			t.Fatalf("completion: %s", raw)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing completion")
	}
	history, err := call("threads/read", map[string]any{"thread_id": id, "include_turns": true}, emit)
	if err != nil {
		t.Fatal(err)
	}
	messages := history.(map[string]any)["messages"].([]message)
	var assistants []message
	for _, m := range messages {
		if m.Role == "assistant" {
			assistants = append(assistants, m)
		}
	}
	if len(assistants) != 2 || assistants[0].ID == assistants[1].ID || assistants[0].Text != "before" || assistants[1].Text != "after" {
		t.Fatalf("assistant segments: %+v", assistants)
	}
}

func TestSteerResponseBeforeOriginalAndNativeError(t *testing.T) {
	for _, tc := range []struct{ text, status string }{{"fast", "interrupted"}, {"fail", "failed"}, {"both", "interrupted"}} {
		t.Run(tc.text, func(t *testing.T) {
			root, bin, state := stage3RouterFixture(t)
			r := New(context.Background(), bin, root, state)
			defer r.Close()
			call := func(method string, p map[string]any, emit func(string, json.RawMessage)) (any, error) {
				raw, _ := json.Marshal(p)
				v, _, e := r.Handle(context.Background(), bridge.Command{Method: method, Params: raw}, emit)
				return v, e
			}
			start, e := call("threads/start", map[string]any{"agent": "antigravity"}, nil)
			if e != nil {
				t.Fatal(e)
			}
			id := start.(map[string]any)["thread"].(map[string]any)["id"].(string)
			completed := make(chan json.RawMessage, 1)
			emit := func(method string, raw json.RawMessage) {
				if method == "turn/completed" {
					completed <- raw
				}
			}
			turn, e := call("turn/start", map[string]any{"thread_id": id, "text": "wait"}, emit)
			if e != nil {
				t.Fatal(e)
			}
			turnID := turn.(map[string]any)["turn"].(map[string]any)["id"].(string)
			_, e = call("turn/steer", map[string]any{"thread_id": id, "turn_id": turnID, "text": tc.text}, emit)
			if e != nil {
				t.Fatal(e)
			}
			select {
			case <-completed:
				t.Fatal("completed before original response")
			case <-time.After(100 * time.Millisecond):
			}
			_, e = call("turn/interrupt", map[string]any{"thread_id": id, "turn_id": turnID}, emit)
			if e != nil {
				t.Fatal(e)
			}
			select {
			case raw := <-completed:
				if !strings.Contains(string(raw), `"`+tc.status+`"`) {
					t.Fatalf("completion: %s", raw)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("missing completion")
			}
		})
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

func TestAttachmentTurnSendsNativeBlocksAndConfirmsMetadata(t *testing.T) {
	root, bin, state := stage3RouterFixture(t)
	path := filepath.Join(root, "note #1.txt")
	if err := os.WriteFile(path, []byte("hello attached"), 0600); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(root, "image #1.png")
	if err := os.WriteFile(imagePath, []byte("\x89PNG\r\n\x1a\nexample"), 0600); err != nil {
		t.Fatal(err)
	}
	r := New(context.Background(), bin, root, state)
	defer r.Close()
	start, _, err := r.Handle(context.Background(), bridge.Command{Method: "threads/start", Params: json.RawMessage(`{"agent":"antigravity"}`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := start.(map[string]any)["thread"].(map[string]any)["id"].(string)
	raw, _ := json.Marshal(map[string]any{"thread_id": id, "text": "look", "attachments": []map[string]string{{"path": imagePath, "kind": "image"}, {"path": path, "kind": "file"}}})
	events := make(chan json.RawMessage, 1)
	done := make(chan struct{})
	_, _, err = r.Handle(context.Background(), bridge.Command{Method: "turn/start", Params: raw}, func(method string, b json.RawMessage) {
		if method == "item/completed" {
			events <- b
		} else if method == "turn/completed" {
			close(done)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	event := <-events
	<-done
	if !strings.Contains(string(event), `"attachments"`) || strings.Contains(string(event), "hello attached") {
		t.Fatalf("confirmation: %s", event)
	}
	b, err := os.ReadFile(filepath.Join(root, "captured-prompt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Params struct {
			Prompt []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				MIME     string `json:"mimeType"`
				Data     string `json:"data"`
				URI      string `json:"uri"`
				Resource struct {
					URI  string `json:"uri"`
					MIME string `json:"mimeType"`
					Text string `json:"text"`
				} `json:"resource"`
			} `json:"prompt"`
		} `json:"params"`
	}
	if err := json.Unmarshal(b, &request); err != nil {
		t.Fatal(err)
	}
	p := request.Params.Prompt
	if len(p) != 3 || p[0].Type != "text" || p[0].Text != "look" || p[1].Type != "image" || p[1].MIME != "image/png" || p[1].Data != "iVBORw0KGgpleGFtcGxl" || !strings.HasSuffix(p[1].URI, "/image%20%231.png") || p[2].Type != "resource" || p[2].Resource.MIME != "text/plain" || p[2].Resource.Text != "hello attached" || !strings.HasSuffix(p[2].Resource.URI, "/note%20%231.txt") {
		t.Fatalf("native prompt: %s", b)
	}
}

func TestAttachmentOnlyAndRefusalBeforeDispatch(t *testing.T) {
	root, bin, state := stage3RouterFixture(t)
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("private note"), 0600); err != nil {
		t.Fatal(err)
	}
	r := New(context.Background(), bin, root, state)
	defer r.Close()
	start, _, err := r.Handle(context.Background(), bridge.Command{Method: "threads/start", Params: json.RawMessage(`{"agent":"antigravity"}`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := start.(map[string]any)["thread"].(map[string]any)["id"].(string)
	call := func(p string, emit func(string, json.RawMessage)) error {
		raw, _ := json.Marshal(map[string]any{"thread_id": id, "text": "", "attachments": []map[string]string{{"path": p, "kind": "file"}}})
		_, _, err := r.Handle(context.Background(), bridge.Command{Method: "turn/start", Params: raw}, emit)
		return err
	}
	if err := call(filepath.Join(root, "missing-secret"), nil); err == nil || strings.Contains(err.Error(), "missing-secret") {
		t.Fatalf("invalid file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "captured-prompt.json")); !os.IsNotExist(err) {
		t.Fatal("invalid attachment dispatched")
	}
	done := make(chan struct{})
	if err := call(path, func(method string, raw json.RawMessage) {
		if method == "turn/completed" {
			close(done)
		}
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("missing completion")
	}
	b, err := os.ReadFile(filepath.Join(root, "captured-prompt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Params struct {
			Prompt []struct {
				Type string `json:"type"`
			} `json:"prompt"`
		} `json:"params"`
	}
	if json.Unmarshal(b, &request) != nil || len(request.Params.Prompt) != 1 || request.Params.Prompt[0].Type != "resource" {
		t.Fatalf("attachment only prompt: %s", b)
	}
}
