package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/warmdev/modex/internal/bridge"
)

func runCommand(t *testing.T, client *Client, cwd, method string, params any) (any, error) {
	t.Helper()
	var raw json.RawMessage
	if params != nil {
		encoded, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		raw = encoded
	}
	return HandleEvents(context.Background(), cwd, bridge.Command{ID: 1, Method: method, Params: raw}, client.Call, nil)
}

func assertJSON(t *testing.T, got any, want string) {
	t.Helper()
	gotRaw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var gotValue, wantValue any
	if json.Unmarshal(gotRaw, &gotValue) != nil || json.Unmarshal([]byte(want), &wantValue) != nil {
		t.Fatalf("invalid comparison JSON: got=%s want=%s", gotRaw, want)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("got  %s\nwant %s", gotRaw, want)
	}
}

const sessionBody = `{"id":"ses_1","projectID":"p","agent":"builder","model":{"id":"m1","providerID":"p1","variant":"default"},"title":"Thread title","location":{"directory":"/work"},"time":{"created":1,"updated":2}}`

func TestStatusProjectsDefaultAgent(t *testing.T) {
	server, requests := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		switch r.URL.Path {
		case "/api/info":
			jsonResponse(w, http.StatusOK, `{"version":"2.4.1","pid":1,"urls":[]}`)
		case "/api/config":
			jsonResponse(w, http.StatusOK, `[{"type":"document","path":"/cfg","info":{"default_agent":"builder","agents":{"builder":{"model":{"providerID":"opencode-go","model":"deepseek-v4-pro"}}}}}]`)
		default:
			http.NotFound(w, r)
		}
	})
	client := NewClient(server.URL, server.Client(), "/work")
	result, err := runCommand(t, client, "/work", "status", nil)
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, result, `{"workspace":"/work","config":{"provider":"opencode-go","model":"deepseek-v4-pro"}}`)
	for _, request := range *requests {
		if request.Path == "/api/config" && request.Query != "directory=%2Fwork" {
			t.Fatalf("config query=%q", request.Query)
		}
	}
}

func TestThreadsListProjectsSessions(t *testing.T) {
	longTitle := strings.Repeat("t", 200)
	page, _ := json.Marshal(map[string]any{
		"data": []map[string]any{
			{"id": "ses_1", "model": map[string]any{"id": "m1", "providerID": "p1"}, "title": longTitle, "location": map[string]string{"directory": "/work"}},
			{"id": "ses_2", "title": "", "location": map[string]string{"directory": "/other"}},
		},
		"cursor": map[string]any{"previous": nil, "next": nil},
	})
	server, requests := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		if r.URL.Path != "/api/session" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		jsonResponse(w, http.StatusOK, string(page))
	})
	client := NewClient(server.URL, server.Client(), "/work")
	result, err := runCommand(t, client, "/work", "threads/list", map[string]any{"limit": 20, "all_workspaces": true})
	if err != nil {
		t.Fatal(err)
	}
	title, _ := json.Marshal(safePreview(longTitle))
	assertJSON(t, result, `{"data":[
		{"id":"ses_1","cwd":"/work","modelProvider":"p1","model":"m1","preview":`+string(title)+`,"name":`+string(title)+`},
		{"id":"ses_2","cwd":"/other","modelProvider":"","preview":"","name":""}
	],"next_cursor":null}`)
	if len(*requests) != 1 || (*requests)[0].Query != "limit=100&order=desc" {
		t.Fatalf("requests=%+v", *requests)
	}
}

func TestThreadsStartCreatesSessionAndResolvesModel(t *testing.T) {
	server, requests := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		switch r.URL.Path {
		case "/api/model":
			jsonResponse(w, http.StatusOK, `{"location":{"directory":"/work"},"data":[{"id":"m1","modelID":"m1","providerID":"p1","name":"M1"}]}`)
		case "/api/session":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"ses_new","model":{"id":"m1","providerID":"p1"},"location":{"directory":"/work"},"time":{"created":1,"updated":1}}}`)
		default:
			http.NotFound(w, r)
		}
	})
	client := NewClient(server.URL, server.Client(), "/work")
	result, err := runCommand(t, client, "/work", "threads/start", map[string]any{"workspace": "/work", "model": "m1"})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, result, `{"thread":{"id":"ses_new","cwd":"/work","modelProvider":"p1","model":"m1"},"model":"m1"}`)
	var createBody string
	for _, request := range *requests {
		if request.Method == http.MethodPost && request.Path == "/api/session" {
			createBody = request.Body
		}
	}
	if createBody != `{"location":{"directory":"/work"},"model":{"id":"m1","providerID":"p1"}}` {
		t.Fatalf("create body=%s", createBody)
	}
}

func TestThreadsStartStandaloneCreatesPrivateDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	directory := filepath.Join(home, "Documents", "Codex")
	server, requests := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		if r.URL.Path != "/api/session" {
			http.NotFound(w, r)
			return
		}
		data, _ := json.Marshal(map[string]any{"data": map[string]any{"id": "ses_new", "location": map[string]string{"directory": directory}}})
		jsonResponse(w, http.StatusOK, string(data))
	})
	client := NewClient(server.URL, server.Client(), "/work")
	result, err := runCommand(t, client, "/work", "threads/start", map[string]any{"standalone": true, "workspace": "", "project_id": nil})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		t.Fatalf("directory: info=%v err=%v", info, err)
	}
	expected, _ := json.Marshal(map[string]any{"thread": map[string]any{"id": "ses_new", "cwd": directory, "modelProvider": ""}, "model": ""})
	assertJSON(t, result, string(expected))
	expectedBody, _ := json.Marshal(map[string]any{"location": map[string]string{"directory": directory}})
	if len(*requests) != 1 || (*requests)[0].Body != string(expectedBody) {
		t.Fatalf("requests=%+v", *requests)
	}
}

func TestThreadsStartStandaloneRejectsConflictingParameters(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	server, requests := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		t.Errorf("unexpected request: %s", r.URL.Path)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	for _, params := range []map[string]any{{"standalone": true, "workspace": "/other"}, {"standalone": true, "project_id": "p1"}} {
		if _, err := runCommand(t, client, "/work", "threads/start", params); err == nil {
			t.Fatalf("accepted conflicting params: %+v", params)
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("unexpected requests: %+v", *requests)
	}
}

func TestThreadsReadMetadata(t *testing.T) {
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		if r.URL.Path != "/api/session/ses_1" {
			http.NotFound(w, r)
			return
		}
		jsonResponse(w, http.StatusOK, `{"data":`+sessionBody+`}`)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	result, err := runCommand(t, client, "/work", "threads/read", map[string]any{"thread_id": "ses_1", "workspace": "/work"})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, result, `{"thread":{"id":"ses_1","cwd":"/work","modelProvider":"p1","model":"m1"}}`)
}

func TestThreadsReadIncludeTurnsProjectsParts(t *testing.T) {
	// Upstream desc order: newest first, oldest last.
	messages := `{"data":[
		{"id":"msg_run","type":"assistant","content":[{"type":"text","text":"working"}],"time":{"created":4}},
		{"id":"msg_err","type":"assistant","outcome":"error","content":[{"type":"text","text":"boom"}],"time":{"created":3}},
		{"id":"msg_asst","type":"assistant","content":[
			{"type":"reasoning","text":"think"},
			{"type":"text","text":"answer"},
			{"type":"tool","tool":{"id":"call_1","name":"bash","state":{"status":"completed","input":{},"content":[{"type":"text","text":"out"}]}}}
		],"time":{"created":2}},
		{"id":"msg_user","type":"user","text":"hello","time":{"created":1}}
	],"cursor":{"previous":null,"next":null}}`
	server, requests := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		switch r.URL.Path {
		case "/api/session/ses_1":
			jsonResponse(w, http.StatusOK, `{"data":`+sessionBody+`}`)
		case "/api/session/ses_1/message":
			jsonResponse(w, http.StatusOK, messages)
		default:
			http.NotFound(w, r)
		}
	})
	client := NewClient(server.URL, server.Client(), "/work")
	result, err := runCommand(t, client, "/work", "threads/read", map[string]any{"thread_id": "ses_1", "workspace": "/work", "include_turns": true})
	if err != nil {
		t.Fatal(err)
	}
	notice := `{"category":"turn_failed","message":"OpenCode could not complete this turn. Review the conversation before trying again.","retrying":false}`
	assertJSON(t, result, `{
		"thread":{"id":"ses_1","cwd":"/work","modelProvider":"p1","model":"m1"},
		"messages":[
			{"id":"msg_user","role":"user","text":"hello","display_text":"hello"},
			{"id":"msg_asst/r1","role":"reasoning","text":"think","display_text":"think","summary_parts":["think"]},
			{"id":"msg_asst","role":"assistant","text":"answer","display_text":"answer"},
			{"id":"call_1","role":"command","text":"out","command":"bash","cwd":"/work","status":"completed","exit_code":null},
			{"id":"msg_err","role":"assistant","text":"boom","display_text":"boom","status":"failed","turn_notice":`+notice+`,"notice":`+notice+`},
			{"id":"msg_run","role":"assistant","text":"working","display_text":"working"}
		],
		"active_turn":true,
		"active_turn_id":"msg_run"
	}`)
	for _, request := range *requests {
		if request.Path == "/api/session/ses_1/message" && request.Query != "order=desc" {
			t.Fatalf("message query=%q", request.Query)
		}
	}
	if _, ok := result.(map[string]any)["history_revision"]; ok {
		t.Fatal("non-paginated reads should not emit history_revision")
	}
}

func TestThreadsReadPagedHistoryRevisionAndCursor(t *testing.T) {
	// Page 1 is newest-first; cursor.next points at older messages. Following
	// the cursor must not repeat order (OpenCode encodes it in the cursor).
	var messageQueries []string
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		switch r.URL.Path {
		case "/api/session/ses_1":
			jsonResponse(w, http.StatusOK, `{"data":`+sessionBody+`}`)
		case "/api/session/ses_1/message":
			messageQueries = append(messageQueries, r.URL.RawQuery)
			if r.URL.Query().Get("cursor") == "" {
				jsonResponse(w, http.StatusOK, `{"data":[
					{"id":"msg_5","type":"user","text":"five","time":{"created":5}},
					{"id":"msg_4","type":"user","text":"four","time":{"created":4}}
				],"cursor":{"previous":null,"next":"older-1"}}`)
				return
			}
			jsonResponse(w, http.StatusOK, `{"data":[
				{"id":"msg_3","type":"user","text":"three","time":{"created":3}},
				{"id":"msg_2","type":"user","text":"two","time":{"created":2}}
			],"cursor":{"previous":"newer-1","next":"older-2"}}`)
		default:
			http.NotFound(w, r)
		}
	})
	client := NewClient(server.URL, server.Client(), "/work")
	first, err := runCommand(t, client, "/work", "threads/read", map[string]any{
		"thread_id": "ses_1", "workspace": "/work", "include_turns": true, "limit": 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstResult := first.(map[string]any)
	revision, _ := firstResult["history_revision"].(string)
	if revision == "" {
		t.Fatal("paged history read did not emit a history_revision")
	}
	if firstResult["next_cursor"] != "older-1" {
		t.Fatalf("next_cursor=%v want older-1", firstResult["next_cursor"])
	}
	assertJSON(t, firstResult["messages"], `[
		{"id":"msg_4","role":"user","text":"four","display_text":"four"},
		{"id":"msg_5","role":"user","text":"five","display_text":"five"}
	]`)

	again, err := runCommand(t, client, "/work", "threads/read", map[string]any{
		"thread_id": "ses_1", "workspace": "/work", "include_turns": true, "limit": 2, "cursor": nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.(map[string]any)["history_revision"] != revision {
		t.Fatal("history_revision is not stable for identical content")
	}

	second, err := runCommand(t, client, "/work", "threads/read", map[string]any{
		"thread_id": "ses_1", "workspace": "/work", "include_turns": true, "limit": 2, "cursor": "older-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondResult := second.(map[string]any)
	if secondResult["next_cursor"] != "older-2" {
		t.Fatalf("next_cursor=%v want older-2", secondResult["next_cursor"])
	}
	if secondResult["history_revision"] == revision {
		t.Fatal("history_revision did not change for an older page")
	}
	assertJSON(t, secondResult["messages"], `[
		{"id":"msg_2","role":"user","text":"two","display_text":"two"},
		{"id":"msg_3","role":"user","text":"three","display_text":"three"}
	]`)
	if len(messageQueries) != 3 || messageQueries[0] != "limit=2&order=desc" || messageQueries[1] != "limit=2&order=desc" || messageQueries[2] != "cursor=older-1&limit=2" {
		t.Fatalf("message queries=%q", messageQueries)
	}
}

func TestThreadsResumeReturnsModel(t *testing.T) {
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		if r.URL.Path != "/api/session/ses_1" {
			http.NotFound(w, r)
			return
		}
		jsonResponse(w, http.StatusOK, `{"data":`+sessionBody+`}`)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	result, err := runCommand(t, client, "/work", "threads/resume", map[string]any{"thread_id": "ses_1", "workspace": "/work"})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, result, `{"thread":{"id":"ses_1","cwd":"/work","modelProvider":"p1","model":"m1"},"model":"m1"}`)
}

func TestModelsListProjection(t *testing.T) {
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		if r.URL.Path != "/api/model" {
			http.NotFound(w, r)
			return
		}
		jsonResponse(w, http.StatusOK, `{"location":{"directory":"/work"},"data":[
			{"id":"m1","modelID":"m1","providerID":"p1","name":"M1"},
			{"id":"m2","modelID":"m2","providerID":"p2","name":"M2"},
			{"id":"m1","modelID":"m1","providerID":"p1","name":"duplicate"}
		]}`)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	result, err := runCommand(t, client, "/work", "models/list", map[string]any{"limit": 100})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, result, `{"data":[
		{"id":"p1/m1","model":"m1","displayName":"m1","defaultReasoningEffort":"","supportedReasoningEfforts":[]},
		{"id":"p2/m2","model":"m2","displayName":"m2","defaultReasoningEffort":"","supportedReasoningEfforts":[]}
	],"next_cursor":null}`)
}

func TestClientAttached(t *testing.T) {
	server, requests := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		t.Errorf("client/attached must not call the OpenCode server")
		http.NotFound(w, r)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	result, err := runCommand(t, client, "/work", "client/attached", map[string]any{"attached": true})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, result, `{"updated":true}`)
	if len(*requests) != 0 {
		t.Fatalf("unexpected requests: %+v", *requests)
	}
}

func TestUnsupportedMethods(t *testing.T) {
	server, requests := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		t.Errorf("unsupported method %s called the server", r.URL.Path)
		http.NotFound(w, r)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	methods := []string{
		"threads/archive", "threads/unarchive", "threads/delete", "threads/fork",
		"threads/takeover", "threads/mode", "threads/status", "threads/permissions",
		"plans/respond", "projects/list", "projects/create", "references/list",
	}
	for _, method := range methods {
		_, err := runCommand(t, client, "/work", method, map[string]any{})
		want := method + " is unsupported by the opencode backend"
		if err == nil || err.Error() != want {
			t.Fatalf("%s: err=%v want %q", method, err, want)
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("unsupported methods made requests: %+v", *requests)
	}
}

func TestValidationErrors(t *testing.T) {
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		t.Errorf("validation error reached the server: %s", r.URL.Path)
		http.NotFound(w, r)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	for _, test := range []struct {
		method string
		params any
		want   string
	}{
		{"status", map[string]any{"unexpected": 1}, "unknown parameter"},
		{"threads/list", map[string]any{"limit": 0}, "limit must be an integer from 1 to 100"},
		{"threads/list", map[string]any{"archived": true}, "archived is unsupported by the opencode backend"},
		{"threads/start", map[string]any{"project_id": "p1"}, "project_id is unsupported by the opencode backend"},
		{"threads/start", map[string]any{"permission_mode": "read-only"}, "permission_mode is unsupported by the opencode backend"},
		{"threads/start", map[string]any{"model": ""}, "model must be a nonempty single-line string up to 256 bytes"},
		{"threads/read", map[string]any{"thread_id": ""}, "thread_id must be a nonempty single-line string up to 256 bytes"},
		{"threads/read", map[string]any{"thread_id": "ses_1", "workspace": "relative"}, "workspace must be an absolute path"},
		{"threads/read", map[string]any{"thread_id": "ses_1", "limit": 10}, "history limit must be 1..50 with include_turns"},
		{"threads/read", map[string]any{"thread_id": "ses_1", "include_turns": true, "cursor": "abc"}, "history cursor requires limit"},
		{"threads/read", map[string]any{"thread_id": "ses_1", "include_turns": true, "limit": 51}, "history limit must be an integer from 1 to 50"},
		{"client/attached", map[string]any{}, "attached must be boolean"},
	} {
		_, err := runCommand(t, client, "/work", test.method, test.params)
		if err == nil || err.Error() != test.want {
			t.Fatalf("%s %+v: err=%v want %q", test.method, test.params, err, test.want)
		}
	}
}

func TestThreadOutsideWorkspaceRejected(t *testing.T) {
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		if r.URL.Path != "/api/session/ses_1" {
			http.NotFound(w, r)
			return
		}
		jsonResponse(w, http.StatusOK, `{"data":{"id":"ses_1","location":{"directory":"/other"}}}`)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	_, err := runCommand(t, client, "/work", "threads/read", map[string]any{"thread_id": "ses_1", "workspace": "/work"})
	if err == nil || err.Error() != "thread outside workspace" {
		t.Fatalf("err=%v", err)
	}
}

func TestTurnStartSendsPromptAndReturnsUserTurn(t *testing.T) {
	server, requests := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		switch r.URL.Path {
		case "/api/model":
			jsonResponse(w, http.StatusOK, `{"location":{"directory":"/work"},"data":[{"id":"m1","modelID":"m1","providerID":"p1","name":"M1"}]}`)
		case "/api/session/ses_1/model":
			jsonResponse(w, http.StatusOK, `{}`)
		case "/api/session/ses_1/prompt":
			jsonResponse(w, http.StatusOK, `{"data":{"id":"msg_user","type":"user"}}`)
		default:
			http.NotFound(w, r)
		}
	})
	client := NewClient(server.URL, server.Client(), "/work")
	result, err := runCommand(t, client, "/work", "turn/start", map[string]any{
		"thread_id": "ses_1",
		"workspace": "/work",
		"text":      "hi",
		"model":     "m1",
		"effort":    "high",
		"attachments": []map[string]string{
			{"path": "/tmp/a.png", "kind": "image"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, result, `{"turn":{"id":"msg_user","status":"inProgress"}}`)

	var modelBody, promptBody string
	for _, request := range *requests {
		switch request.Path {
		case "/api/session/ses_1/model":
			modelBody = request.Body
		case "/api/session/ses_1/prompt":
			promptBody = request.Body
		}
	}
	if modelBody != `{"model":{"id":"m1","providerID":"p1"}}` {
		t.Fatalf("model body=%s", modelBody)
	}
	if promptBody != `{"files":[{"uri":"file:///tmp/a.png","name":"a.png"}],"text":"hi"}` {
		t.Fatalf("prompt body=%s", promptBody)
	}
}

func TestTurnStartValidatesReferencesAndAttachments(t *testing.T) {
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		t.Errorf("invalid turn reached the server: %s", r.URL.Path)
		http.NotFound(w, r)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	for _, test := range []struct {
		params map[string]any
		want   string
	}{
		{map[string]any{"thread_id": "ses_1", "text": "hi", "references": []any{map[string]any{"path": "x"}}}, "references are unsupported by the opencode backend"},
		{map[string]any{"thread_id": "ses_1", "text": "hi", "attachments": []any{map[string]any{"path": "relative", "kind": "file"}}}, "attachment path must be an absolute path"},
		{map[string]any{"thread_id": "ses_1", "text": "hi", "attachments": []any{map[string]any{"path": "/tmp/a", "kind": "other"}}}, "invalid attachment kind"},
		{map[string]any{"thread_id": "ses_1", "text": nil}, "text must be a string"},
		{map[string]any{"thread_id": "ses_1", "text": "hi", "effort": ""}, "invalid reasoning effort"},
	} {
		_, err := runCommand(t, client, "/work", "turn/start", test.params)
		if err == nil || err.Error() != test.want {
			t.Fatalf("%+v: err=%v want %q", test.params, err, test.want)
		}
	}
}

func TestTurnInterruptPostsAndValidatesTurn(t *testing.T) {
	server, requests := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		if r.URL.Path != "/api/session/ses_1/interrupt" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		jsonResponse(w, http.StatusOK, `{"interrupted":false}`)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	result, err := runCommand(t, client, "/work", "turn/interrupt", map[string]any{"thread_id": "ses_1", "turn_id": "msg_user"})
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, result, `{}`)
	if len(*requests) != 1 || (*requests)[0].Path != "/api/session/ses_1/interrupt" {
		t.Fatalf("requests=%+v", *requests)
	}
	if _, err := runCommand(t, client, "/work", "turn/interrupt", map[string]any{"thread_id": "ses_1"}); err == nil || err.Error() != "turn_id must be a nonempty single-line string up to 256 bytes" {
		t.Fatalf("missing turn_id err=%v", err)
	}
}

func TestTurnStartBusyErrorIsSanitized(t *testing.T) {
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		if r.URL.Path == "/api/session/ses_1/prompt" {
			jsonResponse(w, http.StatusConflict, `{"error":"SECRET provider detail"}`)
			return
		}
		http.NotFound(w, r)
	})
	client := NewClient(server.URL, server.Client(), "/work")
	_, err := runCommand(t, client, "/work", "turn/start", map[string]any{"thread_id": "ses_1", "text": "hi"})
	if err == nil {
		t.Fatal("expected a busy error")
	}
	if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "provider detail") {
		t.Fatalf("error leaked upstream body: %v", err)
	}
	if !strings.Contains(err.Error(), "status 409") {
		t.Fatalf("error=%v", err)
	}
}

func TestServeEventsRoutesInterruptAheadOfPendingTurn(t *testing.T) {
	release := make(chan struct{})
	interrupted := make(chan struct{})
	call := func(ctx context.Context, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error) {
		switch method {
		case callTurnStart:
			<-release
			return json.RawMessage(`{"turn":{"id":"msg_user","status":"inProgress"}}`), nil
		case callTurnInterrupt:
			close(interrupted)
			return json.RawMessage(`{}`), nil
		default:
			return json.RawMessage(`{}`), nil
		}
	}
	input := strings.NewReader(
		"{\"id\":1,\"method\":\"turn/start\",\"params\":{\"thread_id\":\"ses_1\",\"text\":\"hi\"}}\n" +
			"{\"id\":2,\"method\":\"turn/interrupt\",\"params\":{\"thread_id\":\"ses_1\",\"turn_id\":\"msg_user\"}}\n",
	)
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- ServeEvents(context.Background(), input, &output, "/work", call) }()

	select {
	case <-interrupted:
		// turn/interrupt reached the call seam while turn/start was blocked.
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("turn/interrupt was starved behind the pending turn/start")
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeEvents did not return")
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"id":2`) || !strings.Contains(lines[1], `"id":1`) {
		t.Fatalf("output=%q", output.String())
	}
}

func TestClientAttachedRoutesThroughSeam(t *testing.T) {
	called := false
	call := func(ctx context.Context, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error) {
		if method != callClientAttached {
			t.Fatalf("method=%s", method)
		}
		values, _ := params.(map[string]any)
		if values["attached"] != true {
			t.Fatalf("params=%+v", params)
		}
		called = true
		return json.RawMessage(`{"updated":true}`), nil
	}
	params, _ := json.Marshal(map[string]any{"attached": true})
	result, err := HandleEvents(context.Background(), "/work", bridge.Command{ID: 1, Method: "client/attached", Params: params}, call, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("client/attached did not reach the call seam")
	}
	assertJSON(t, result, `{"updated":true}`)
}

func TestDecisionRespondForwardsToStore(t *testing.T) {
	var forwarded map[string]any
	call := func(ctx context.Context, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error) {
		if method != callDecisionRespond {
			t.Fatalf("method=%s", method)
		}
		forwarded, _ = params.(map[string]any)
		return json.RawMessage(`{"delivered":true}`), nil
	}
	params, _ := json.Marshal(map[string]any{
		"decision_id": "per_1",
		"result":      map[string]any{"decision": "accept"},
	})
	result, err := HandleEvents(context.Background(), "/work", bridge.Command{ID: 1, Method: "decision/respond", Params: params}, call, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, result, `{"delivered":true}`)
	if forwarded["decisionID"] != "per_1" || string(forwarded["result"].(json.RawMessage)) != `{"decision":"accept"}` {
		t.Fatalf("forwarded=%+v", forwarded)
	}

	for _, invalid := range []map[string]any{
		{"result": map[string]any{"decision": "accept"}},
		{"decision_id": "per_1"},
		{"decision_id": "per_1", "result": nil},
	} {
		raw, _ := json.Marshal(invalid)
		if _, err := HandleEvents(context.Background(), "/work", bridge.Command{ID: 1, Method: "decision/respond", Params: raw}, call, nil); err == nil {
			t.Fatalf("accepted invalid decision respond: %+v", invalid)
		}
	}
}

func TestServeEventsRoutesDecisionInline(t *testing.T) {
	release := make(chan struct{})
	answered := make(chan struct{})
	call := func(ctx context.Context, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error) {
		switch method {
		case callTurnStart:
			<-release
			return json.RawMessage(`{"turn":{"id":"msg_user","status":"inProgress"}}`), nil
		case callDecisionRespond:
			close(answered)
			return json.RawMessage(`{"delivered":true}`), nil
		default:
			return json.RawMessage(`{}`), nil
		}
	}
	input := strings.NewReader(
		"{\"id\":1,\"method\":\"turn/start\",\"params\":{\"thread_id\":\"ses_1\",\"text\":\"hi\"}}\n" +
			"{\"id\":2,\"method\":\"decision/respond\",\"params\":{\"decision_id\":\"per_1\",\"result\":{\"decision\":\"accept\"}}}\n",
	)
	var output bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- ServeEvents(context.Background(), input, &output, "/work", call) }()
	select {
	case <-answered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("decision/respond was starved behind the pending turn/start")
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ServeEvents did not return")
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"id":2`) || !strings.Contains(lines[1], `"id":1`) {
		t.Fatalf("output=%q", output.String())
	}
}

func TestServeEventsSupervisorReadiness(t *testing.T) {
	server, _ := fakeServer(t, func(w http.ResponseWriter, r *http.Request, body string) {
		switch r.URL.Path {
		case "/api/info":
			jsonResponse(w, http.StatusOK, `{"version":"2.4.1","pid":1,"urls":[]}`)
		case "/api/config":
			jsonResponse(w, http.StatusOK, `[{"type":"document","info":{"default_agent":"builder","agents":{"builder":{"model":{"providerID":"p1","model":"m1"}}}}}]`)
		default:
			http.NotFound(w, r)
		}
	})
	client := NewClient(server.URL, server.Client(), "/work")
	input := strings.NewReader(
		"{\"id\":2147483647,\"method\":\"status\"}\n" +
			"{\"id\":2147483646,\"method\":\"client/attached\",\"params\":{\"attached\":true}}\n",
	)
	var output bytes.Buffer
	if err := ServeEvents(context.Background(), input, &output, "/work", client.Call); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("output=%q", output.String())
	}
	var status struct {
		ID     int `json:"id"`
		Result struct {
			Workspace string `json:"workspace"`
			Config    struct {
				Provider string `json:"provider"`
				Model    string `json:"model"`
			} `json:"config"`
		} `json:"result"`
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal([]byte(lines[0]), &status) != nil || status.ID != 2147483647 || status.Result.Workspace != "/work" || status.Result.Config.Provider != "p1" || status.Result.Config.Model != "m1" || len(status.Error) != 0 {
		t.Fatalf("status line=%s", lines[0])
	}
	var attached struct {
		ID     int `json:"id"`
		Result struct {
			Updated bool `json:"updated"`
		} `json:"result"`
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal([]byte(lines[1]), &attached) != nil || attached.ID != 2147483646 || !attached.Result.Updated || len(attached.Error) != 0 {
		t.Fatalf("attached line=%s", lines[1])
	}
}
