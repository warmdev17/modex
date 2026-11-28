//go:build !windows

package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/warmdev/modex/internal/bridge"
)

func TestNativeCLIHandoffValidatesScopeAndPrivateConnection(t *testing.T) {
	workspace := t.TempDir()
	state := t.TempDir()
	if err := os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	registry, err := openRegistry(state)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := registry.add("ses_native", workspace, "", false)
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binaryDir := t.TempDir()
	if err := os.Symlink(binary, filepath.Join(binaryDir, "opencode")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binaryDir)
	var busy atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/session/ses_native" {
			stamp := map[string]any{}
			if busy.Load() {
				stamp["busy"] = 123
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "ses_native", "location": map[string]string{"directory": workspace}, "time": stamp}})
			return
		}
		jsonResponse(w, 200, `{"data":[]}`)
	}))
	defer server.Close()
	client := NewClient(server.URL, server.Client(), workspace)
	synth := NewSynthesizer(workspace, nil)
	stream := NewStream(client, synth)
	router := &Router{binary: "opencode", registry: registry, process: &Process{baseURL: server.URL, password: "private-password"}, workspaces: map[string]*managedWorkspace{workspace: {client: client, synth: synth, stream: stream}}}
	if _, err := router.NativeCLI(context.Background(), workspace, "ses_native"); err == nil {
		t.Fatal("accepted unregistered native id")
	}
	if _, err := router.NativeCLI(context.Background(), t.TempDir(), chat.ID); err == nil {
		t.Fatal("accepted foreign workspace")
	}
	busy.Store(true)
	if _, err := router.NativeCLI(context.Background(), workspace, chat.ID); err == nil {
		t.Fatal("accepted a busy native session")
	}
	busy.Store(false)
	got, err := router.NativeCLI(context.Background(), workspace, chat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got["binary"] != filepath.Join(binaryDir, "opencode") || got["workspace"] != workspace || got["session_id"] != "ses_native" || got["server"] != server.URL || got["password"] != "private-password" {
		t.Fatalf("connection: %#v", got)
	}
	router.ReleaseNativeCLI(chat.ID)
}

func TestNativeWatchAdoptsPromptDecisionsCompletionAndHistory(t *testing.T) {
	workspace := t.TempDir()
	state := t.TempDir()
	if err := os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	registry, err := openRegistry(state)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := registry.add("ses_native", workspace, "", false)
	if err != nil {
		t.Fatal(err)
	}
	messages := `[]`
	permissions := `[{"id":"old","sessionID":"ses_native","action":"write"}]`
	replied := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/session/ses_native":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "ses_native", "location": map[string]string{"directory": workspace}}})
		case strings.HasSuffix(r.URL.Path, "/message"):
			jsonResponse(w, 200, `{"data":`+messages+`}`)
		case strings.HasSuffix(r.URL.Path, "/permission"):
			jsonResponse(w, 200, `{"data":`+permissions+`}`)
		case strings.HasSuffix(r.URL.Path, "/form"):
			jsonResponse(w, 200, `{"data":[]}`)
		case strings.HasSuffix(r.URL.Path, "/reply"):
			replied = true
			jsonResponse(w, 200, `{}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	collector := &eventCollector{}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	router := &Router{binary: binary, registry: registry, process: &Process{baseURL: server.URL, password: "private"}, emit: collector.emit}
	client := NewClient(server.URL, server.Client(), workspace)
	emit := func(method string, raw json.RawMessage) { router.emitEvent(workspace, method, raw) }
	synth := NewSynthesizer(workspace, emit)
	decisions := NewDecisions(workspace, emit, synth.TurnID, client.ReplyPermission)
	synth.OnTurnComplete = decisions.Resolve
	stream := NewStream(client, synth)
	stream.ScopedPoll = true
	stream.OnScopedPermissions = decisions.reconcileTurn
	stream.Forms = decisions
	router.workspaces = map[string]*managedWorkspace{workspace: {client: client, synth: synth, decisions: decisions, stream: stream}}
	if _, err := router.NativeCLI(context.Background(), workspace, chat.ID); err != nil {
		t.Fatal(err)
	}
	messages = `[{"id":"a1","type":"assistant","text":"native answer"},{"id":"u1","type":"user","text":"native prompt"}]`
	permissions = `[{"id":"old","sessionID":"ses_native","action":"write"},{"id":"per_native","sessionID":"ses_native","action":"bash","source":{"messageID":"a1"}}]`
	stream.reconcileNative(context.Background(), "ses_native")
	if id, active := synth.ActiveTurn("ses_native"); id != "u1" || !active {
		t.Fatalf("native turn: %s %v", id, active)
	}
	if len(decisions.Pending()) != 1 {
		t.Fatalf("projected stale permissions: %+v", decisions.Pending())
	}
	if !collector.hasMethod("decision/required") {
		t.Fatalf("missing native decision: %v", eventMethods(collector.snapshot()))
	}
	params, _ := json.Marshal(map[string]any{"decision_id": chat.ID + "/per_native", "result": map[string]string{"decision": "accept"}})
	if _, _, err := router.Handle(context.Background(), bridge.Command{Method: "decision/respond", Params: params}, collector.emit); err != nil || !replied {
		t.Fatalf("decision: %v replied=%v", err, replied)
	}
	messages = `[{"id":"idle1","type":"idle","outcome":"succeeded"},{"id":"a1","type":"assistant","text":"native answer"},{"id":"u1","type":"user","text":"native prompt"}]`
	permissions = `[]`
	stream.reconcileNative(context.Background(), "ses_native")
	stream.reconcileNative(context.Background(), "ses_native")
	counts := map[string]int{}
	for _, event := range collector.snapshot() {
		var v map[string]any
		_ = json.Unmarshal(event.params, &v)
		if id, _ := v["threadId"].(string); id != "" && id != chat.ID {
			t.Fatalf("unmapped event: %s", event.params)
		}
		counts[event.method]++
	}
	if counts["turn/started"] != 1 || counts["turn/completed"] != 1 {
		t.Fatalf("turn lifecycle: %v", counts)
	}
	users := 0
	for _, event := range collector.snapshot() {
		if strings.Contains(string(event.params), `"userMessage"`) {
			users++
		}
	}
	if users != 1 {
		t.Fatalf("native prompts: %d", users)
	}
	params, _ = json.Marshal(map[string]any{"thread_id": chat.ID, "include_turns": true})
	value, _, err := router.Handle(context.Background(), bridge.Command{Method: "threads/read", Params: params}, collector.emit)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(value)
	if !strings.Contains(string(raw), "native prompt") || !strings.Contains(string(raw), "native answer") || strings.Contains(string(raw), `"active_turn":true`) {
		t.Fatalf("history: %s", raw)
	}
	router.ReleaseNativeCLI(chat.ID)
	messages = `[{"id":"u2","type":"user","text":"after release"},{"id":"idle1","type":"idle","outcome":"succeeded"}]`
	stream.reconcileNative(context.Background(), "ses_native")
	if id := synth.TurnID("ses_native"); id != "u1" {
		t.Fatalf("released watch adopted %s", id)
	}
}

func TestNativeWatchDoesNotDuplicateConcurrentModexPrompt(t *testing.T) {
	workspace := t.TempDir()
	state := t.TempDir()
	if err := os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	registry, err := openRegistry(state)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := registry.add("ses_native", workspace, "", false)
	if err != nil {
		t.Fatal(err)
	}
	var messages atomic.Value
	messages.Store(`[{"id":"old_idle","type":"idle","outcome":"succeeded"}]`)
	promptArrived, polling := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/prompt"):
			messages.Store(`[{"id":"idle","type":"idle","outcome":"succeeded"},{"id":"a","type":"assistant","text":"answer"},{"id":"modex_user","type":"user","text":"phone prompt"},{"id":"old_idle","type":"idle","outcome":"succeeded"}]`)
			close(promptArrived)
			<-polling
			jsonResponse(w, 200, `{"data":{"id":"modex_user"}}`)
		case strings.HasSuffix(r.URL.Path, "/message"):
			jsonResponse(w, 200, `{"data":`+messages.Load().(string)+`}`)
		case r.URL.Path == "/api/session/ses_native":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "ses_native", "location": map[string]string{"directory": workspace}}})
		default:
			jsonResponse(w, 200, `{"data":[]}`)
		}
	}))
	defer server.Close()
	collector := &eventCollector{}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	router := &Router{binary: binary, registry: registry, process: &Process{baseURL: server.URL, password: "private"}, emit: collector.emit}
	client := NewClient(server.URL, server.Client(), workspace)
	emit := func(method string, raw json.RawMessage) { router.emitEvent(workspace, method, raw) }
	synth := NewSynthesizer(workspace, emit)
	decisions := NewDecisions(workspace, emit, synth.TurnID, client.ReplyPermission)
	stream := NewStream(client, synth)
	stream.ScopedPoll = true
	router.workspaces = map[string]*managedWorkspace{workspace: {client: client, synth: synth, decisions: decisions, stream: stream}}
	if _, err := router.NativeCLI(context.Background(), workspace, chat.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-promptArrived
		close(polling)
		stream.reconcileNative(context.Background(), "ses_native")
	}()
	params, _ := json.Marshal(map[string]any{"thread_id": chat.ID, "text": "phone prompt"})
	if _, _, err := router.Handle(context.Background(), bridge.Command{Method: "turn/start", Params: params}, collector.emit); err != nil {
		t.Fatal(err)
	}
	<-done
	users, started, completed := 0, 0, 0
	for _, event := range collector.snapshot() {
		if strings.Contains(string(event.params), `"userMessage"`) {
			users++
		}
		if event.method == "turn/started" {
			started++
		}
		if event.method == "turn/completed" {
			completed++
		}
	}
	if users != 1 || started != 1 || completed != 1 {
		t.Fatalf("users=%d started=%d completed=%d events=%v", users, started, completed, eventMethods(collector.snapshot()))
	}
}

func TestNativeWatchReplaysOnlyNewTurnsAndStopsOnLeaseRelease(t *testing.T) {
	workspace := t.TempDir()
	var messages atomic.Value
	messages.Store(`[{"id":"old_idle","type":"idle","outcome":"succeeded"},{"id":"old_user","type":"user","text":"old prompt"}]`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/message") {
			jsonResponse(w, 200, `{"data":`+messages.Load().(string)+`}`)
			return
		}
		if r.URL.Path == "/api/session/s" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "s", "location": map[string]string{"directory": workspace}}})
			return
		}
		jsonResponse(w, 200, `{"data":[]}`)
	}))
	defer server.Close()
	collector := &eventCollector{}
	synth := NewSynthesizer(workspace, collector.emit)
	stream := NewStream(NewClient(server.URL, server.Client(), workspace), synth)
	stream.ScopedPoll = true
	stream.native = map[string]*nativeWatch{"s": {anchor: "old_idle", refs: 1}}
	stream.reconcileNative(context.Background(), "s")
	if len(collector.snapshot()) != 0 {
		t.Fatal("replayed pre-handoff history")
	}
	messages.Store(`[{"id":"i2","type":"idle","outcome":"succeeded"},{"id":"a2","type":"assistant","text":"second"},{"id":"u2","type":"user","text":"two"},{"id":"i1","type":"idle","outcome":"succeeded"},{"id":"a1","type":"assistant","text":"first"},{"id":"u1","type":"user","text":"one"},{"id":"old_idle","type":"idle","outcome":"succeeded"}]`)
	stream.reconcileNative(context.Background(), "s")
	stream.reconcileNative(context.Background(), "s")
	started, completed := 0, 0
	for _, event := range collector.snapshot() {
		if event.method == "turn/started" {
			started++
		}
		if event.method == "turn/completed" {
			completed++
		}
	}
	if started != 2 || completed != 2 {
		t.Fatalf("started=%d completed=%d events=%v", started, completed, eventMethods(collector.snapshot()))
	}
	messages.Store(`[{"id":"u3","type":"user","text":"running"},{"id":"i2","type":"idle","outcome":"succeeded"}]`)
	stream.reconcileNative(context.Background(), "s")
	router := &Router{workspaces: map[string]*managedWorkspace{workspace: {stream: stream, synth: synth}}}
	router.ReleaseNativeCLIs()
	if len(stream.native) != 0 {
		t.Fatal("lease retained watches")
	}
	if _, active := synth.ActiveTurn("s"); !active {
		t.Fatal("lease release stopped an accepted turn")
	}
	messages.Store(`[{"id":"i3","type":"idle","outcome":"succeeded"},{"id":"a3","type":"assistant","text":"finished after detach"},{"id":"u3","type":"user","text":"running"}]`)
	stream.reconcile(context.Background(), "s", true)
	if _, active := synth.ActiveTurn("s"); active {
		t.Fatal("released active turn no longer reconciled")
	}
}

func TestNativeWatchRequiresAnchorAndBoundsHistoryReads(t *testing.T) {
	for _, mode := range []string{"missing", "loop", "gap", "pages"} {
		t.Run(mode, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				switch mode {
				case "missing":
					jsonResponse(w, 200, `{"data":[{"id":"foreign","type":"user"}]}`)
				case "loop":
					jsonResponse(w, 200, `{"data":[{"id":"new","type":"user"}],"cursor":{"next":"repeat"}}`)
				case "gap":
					var data []map[string]string
					for i := 0; i < 20; i++ {
						data = append(data, map[string]string{"id": fmt.Sprintf("message-%d-%d", requests, i), "type": "assistant"})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "cursor": map[string]string{"next": fmt.Sprintf("page-%d", requests)}})
				case "pages":
					if r.URL.Query().Get("cursor") == "" {
						jsonResponse(w, 200, `{"data":[{"id":"new","type":"user"}],"cursor":{"next":"older"}}`)
					} else {
						jsonResponse(w, 200, `{"data":[{"id":"anchor","type":"idle"}]}`)
					}
				}
			}))
			defer server.Close()
			stream := NewStream(NewClient(server.URL, server.Client(), "/work"), NewSynthesizer("/work", nil))
			messages, ok := stream.nativeMessages(context.Background(), "s", "anchor")
			if mode == "pages" {
				if !ok || len(messages) != 2 || messages[0].ID != "anchor" || requests != 2 {
					t.Fatalf("messages=%+v ok=%v requests=%d", messages, ok, requests)
				}
				return
			}
			if ok || requests > 5 {
				t.Fatalf("accepted unanchored or unbounded history: ok=%v requests=%d", ok, requests)
			}
		})
	}
}

func TestNativeCompletedBacklogPrecedesImmediatePhoneTurn(t *testing.T) {
	workspace := t.TempDir()
	state := t.TempDir()
	if err := os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	registry, err := openRegistry(state)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := registry.add("ses_native", workspace, "", false)
	if err != nil {
		t.Fatal(err)
	}
	var messages atomic.Value
	nativeHistory := `{"id":"native_idle","type":"idle","outcome":"succeeded"},{"id":"native_answer","type":"assistant","text":"native answer"},{"id":"native_user","type":"user","text":"native prompt"},{"id":"baseline","type":"idle","outcome":"succeeded"}`
	messages.Store(`[` + nativeHistory + `]`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/prompt"):
			messages.Store(`[{"id":"phone_idle","type":"idle","outcome":"succeeded"},{"id":"phone_answer","type":"assistant","text":"phone answer"},{"id":"phone_user","type":"user","text":"phone prompt"},` + nativeHistory + `]`)
			jsonResponse(w, 200, `{"data":{"id":"phone_user"}}`)
		case strings.HasSuffix(r.URL.Path, "/message"):
			jsonResponse(w, 200, `{"data":`+messages.Load().(string)+`}`)
		case r.URL.Path == "/api/session/ses_native":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "ses_native", "location": map[string]string{"directory": workspace}}})
		default:
			jsonResponse(w, 200, `{"data":[]}`)
		}
	}))
	defer server.Close()
	collector := &eventCollector{}
	router := &Router{registry: registry, emit: collector.emit}
	emit := func(method string, raw json.RawMessage) { router.emitEvent(workspace, method, raw) }
	synth := NewSynthesizer(workspace, emit)
	client := NewClient(server.URL, server.Client(), workspace)
	decisions := NewDecisions(workspace, emit, synth.TurnID, client.ReplyPermission)
	stream := NewStream(client, synth)
	stream.ScopedPoll = true
	stream.native = map[string]*nativeWatch{"ses_native": {anchor: "baseline", refs: 1}}
	router.workspaces = map[string]*managedWorkspace{workspace: {client: client, synth: synth, stream: stream, decisions: decisions}}
	params, _ := json.Marshal(map[string]any{"thread_id": chat.ID, "text": "phone prompt"})
	if _, _, err := router.Handle(context.Background(), bridge.Command{Method: "turn/start", Params: params}, collector.emit); err != nil {
		t.Fatal(err)
	}
	stream.reconcileNative(context.Background(), "ses_native")
	var starts, completions []string
	nativePrompt, nativeAnswer := false, false
	for _, event := range collector.snapshot() {
		var p struct {
			TurnID string `json:"turnId"`
			Turn   struct {
				ID string `json:"id"`
			} `json:"turn"`
			Item struct {
				ID string `json:"id"`
			} `json:"item"`
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal(event.params, &p); err != nil {
			t.Fatal(err)
		}
		if event.method == "turn/started" {
			starts = append(starts, p.Turn.ID)
		}
		if event.method == "turn/completed" {
			completions = append(completions, p.Turn.ID)
		}
		if event.method == "item/completed" && p.Item.ID == "native_user" && p.TurnID == "native_user" {
			nativePrompt = true
		}
		if event.method == "item/agentMessage/delta" && p.Delta == "native answer" && p.TurnID == "native_user" {
			nativeAnswer = true
		}
	}
	if strings.Join(starts, ",") != "native_user,phone_user" || strings.Join(completions, ",") != "native_user,phone_user" || !nativePrompt || !nativeAnswer {
		t.Fatalf("starts=%v completions=%v nativePrompt=%v nativeAnswer=%v events=%v", starts, completions, nativePrompt, nativeAnswer, eventMethods(collector.snapshot()))
	}
}
