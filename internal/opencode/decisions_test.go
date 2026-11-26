package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

type decisionReply struct {
	session  string
	request  string
	decision string
}

func decisionStore(t *testing.T, replies *[]decisionReply, replyErr error) (*Decisions, *eventCollector) {
	t.Helper()
	collector := &eventCollector{}
	store := NewDecisions("/work", collector.emit, func(string) string { return "msg_user" }, func(_ context.Context, session, request, decision string) error {
		*replies = append(*replies, decisionReply{session, request, decision})
		return replyErr
	})
	return store, collector
}

func perm(id, action string, resources ...string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"id":        id,
		"sessionID": "ses_1",
		"action":    action,
		"resources": resources,
	})
	return raw
}

func TestDecisionsCommandPermissionContext(t *testing.T) {
	var replies []decisionReply
	store, collector := decisionStore(t, &replies, nil)
	store.Handle("permission.asked", json.RawMessage(`{"id":"per_1","sessionID":"ses_1","action":"bash","resources":["echo hi"],"source":{"type":"tool","messageID":"msg_1","id":"call_1"}}`), "/srv/work")
	events := collector.snapshot()
	if len(events) != 1 || events[0].method != "decision/required" {
		t.Fatalf("events=%v", eventMethods(events))
	}
	assertJSON(t, events[0].params, `{
		"decision_id":"ses_1/per_1",
		"method":"item/commandExecution/requestApproval",
		"thread_id":"ses_1",
		"turn_id":"msg_user",
		"item_id":"call_1",
		"context":{
			"command":"echo hi",
			"cwd":"/srv/work",
			"kind":"shell",
			"reason":"OpenCode is waiting for permission to run this action.",
			"availableDecisions":["accept","decline","cancel"]
		}
	}`)
}

func TestDecisionsFilePermissionContext(t *testing.T) {
	var replies []decisionReply
	store, collector := decisionStore(t, &replies, nil)
	store.Handle("permission.asked", json.RawMessage(`{"id":"per_2","sessionID":"ses_1","action":"edit","resources":["/srv/work/a.txt"],"message":"Allow edit?"}`), "/srv/work")
	events := collector.snapshot()
	assertJSON(t, events[0].params, `{
		"decision_id":"ses_1/per_2",
		"method":"item/fileChange/requestApproval",
		"thread_id":"ses_1",
		"turn_id":"msg_user",
		"item_id":"",
		"context":{
			"reason":"Allow edit?",
			"grantRoot":"/srv/work"
		}
	}`)
}

func TestDecisionsWebPermissionContext(t *testing.T) {
	var replies []decisionReply
	store, collector := decisionStore(t, &replies, nil)
	store.Handle("permission.asked", perm("per_3", "webfetch", "https://example.com/path"), "/srv/work")
	events := collector.snapshot()
	assertJSON(t, events[0].params, `{
		"decision_id":"ses_1/per_3",
		"method":"item/commandExecution/requestApproval",
		"thread_id":"ses_1",
		"turn_id":"msg_user",
		"item_id":"",
		"context":{
			"command":"https://example.com/path",
			"cwd":"/srv/work",
			"kind":"web",
			"reason":"OpenCode is waiting for permission to run this action.",
			"networkApprovalContext":{"host":"example.com","port":443},
			"availableDecisions":["accept","decline","cancel"]
		}
	}`)
}

func TestDecisionsDedupeReplayAndResolve(t *testing.T) {
	var replies []decisionReply
	store, collector := decisionStore(t, &replies, nil)
	store.Handle("permission.asked", perm("per_1", "bash", "one"), "/work")
	store.Handle("permission.asked", perm("per_1", "bash", "one"), "/work")
	store.Handle("permission.asked", perm("per_2", "read", "/a"), "/work")
	if len(collector.snapshot()) != 2 {
		t.Fatalf("dedupe failed: %v", eventMethods(collector.snapshot()))
	}
	store.Replay(false)
	if len(collector.snapshot()) != 2 {
		t.Fatal("detach replay emitted events")
	}
	store.Replay(true)
	events := collector.snapshot()
	if len(events) != 5 || events[2].method != "decision/reset" || events[3].method != "decision/required" || events[4].method != "decision/required" {
		t.Fatalf("replay events=%v", eventMethods(events))
	}
	var first struct {
		DecisionID string `json:"decision_id"`
	}
	var second struct {
		DecisionID string `json:"decision_id"`
	}
	_ = json.Unmarshal(events[3].params, &first)
	_ = json.Unmarshal(events[4].params, &second)
	if first.DecisionID != "ses_1/per_1" || second.DecisionID != "ses_1/per_2" {
		t.Fatalf("replay order=%v", []string{first.DecisionID, second.DecisionID})
	}
	store.Handle("permission.replied", json.RawMessage(`{"sessionID":"ses_1","requestID":"per_1","reply":"once"}`), "/work")
	last := collector.snapshot()[5]
	if last.method != "decision/resolved" {
		t.Fatalf("resolution=%v", last.method)
	}
	assertJSON(t, last.params, `{"decision_id":"ses_1/per_1"}`)
	if pending := store.Pending(); len(pending) != 1 || pending[0].ID != "ses_1/per_2" {
		t.Fatalf("pending=%v", pending)
	}
}

func TestDecisionsAnswerMapsOnceAndReject(t *testing.T) {
	var replies []decisionReply
	store, collector := decisionStore(t, &replies, nil)
	store.Handle("permission.asked", perm("per_1", "bash", "one"), "/work")
	if err := store.Answer(context.Background(), "ses_1/per_1", json.RawMessage(`{"decision":"accept"}`)); err != nil {
		t.Fatal(err)
	}
	store.Handle("permission.asked", perm("per_2", "bash", "two"), "/work")
	if err := store.Answer(context.Background(), "ses_1/per_2", json.RawMessage(`{"decision":"decline"}`)); err != nil {
		t.Fatal(err)
	}
	store.Handle("permission.asked", perm("per_3", "bash", "three"), "/work")
	if err := store.Answer(context.Background(), "ses_1/per_3", json.RawMessage(`{"decision":"cancel"}`)); err != nil {
		t.Fatal(err)
	}
	want := []decisionReply{
		{"ses_1", "per_1", "once"},
		{"ses_1", "per_2", "reject"},
		{"ses_1", "per_3", "reject"},
	}
	if !reflect.DeepEqual(replies, want) {
		t.Fatalf("replies=%+v want=%+v", replies, want)
	}
	if len(store.Pending()) != 0 {
		t.Fatal("answered decisions remained pending")
	}
	resolved := 0
	for _, event := range collector.snapshot() {
		if event.method == "decision/resolved" {
			resolved++
		}
	}
	if resolved != 3 {
		t.Fatalf("resolved events=%d", resolved)
	}
}

func TestDecisionsFileAcceptIsNotOffered(t *testing.T) {
	var replies []decisionReply
	store, _ := decisionStore(t, &replies, nil)
	store.Handle("permission.asked", perm("per_1", "edit", "/work/a.txt"), "/work")
	err := store.Answer(context.Background(), "ses_1/per_1", json.RawMessage(`{"decision":"accept"}`))
	if err == nil || err.Error() != "decision not offered by OpenCode" {
		t.Fatalf("err=%v", err)
	}
	if len(replies) != 0 || len(store.Pending()) != 1 {
		t.Fatalf("replies=%v pending=%v", replies, store.Pending())
	}
}

func TestDecisionAnswerValidationAndDeliveryFailure(t *testing.T) {
	var replies []decisionReply
	store, _ := decisionStore(t, &replies, nil)
	store.Handle("permission.asked", perm("per_1", "bash", "one"), "/work")
	if err := store.Answer(context.Background(), "missing", json.RawMessage(`{"decision":"accept"}`)); err == nil || err.Error() != "decision is missing or already resolved" {
		t.Fatalf("missing err=%v", err)
	}
	if err := store.Answer(context.Background(), "ses_1/per_1", json.RawMessage(`{"answers":{"q":{"answers":["a"]}}}`)); err == nil || err.Error() != "questions are unsupported by the opencode backend" {
		t.Fatalf("question err=%v", err)
	}
	if err := store.Answer(context.Background(), "ses_1/per_1", json.RawMessage(`{"decision":"always"}`)); err == nil || err.Error() != "decision must be accept, decline or cancel" {
		t.Fatalf("always err=%v", err)
	}
	if err := store.Answer(context.Background(), "ses_1/per_1", json.RawMessage(`{"decision":"accept","extra":1}`)); err == nil || err.Error() != "invalid decision response" {
		t.Fatalf("strict err=%v", err)
	}
	if len(store.Pending()) != 1 {
		t.Fatal("invalid answers consumed the decision")
	}

	failing, _ := decisionStore(t, &replies, errors.New("boom"))
	failing.Handle("permission.asked", perm("per_2", "bash", "two"), "/work")
	if err := failing.Answer(context.Background(), "ses_1/per_2", json.RawMessage(`{"decision":"accept"}`)); err == nil || err.Error() != "decision delivery failed; do not replay" {
		t.Fatalf("delivery err=%v", err)
	}
	if len(failing.Pending()) != 0 {
		t.Fatal("uncertain delivery remained replayable")
	}
}

func TestDecisionsCompositeSessionKey(t *testing.T) {
	var replies []decisionReply
	store, _ := decisionStore(t, &replies, nil)
	store.Handle("permission.asked", json.RawMessage(`{"id":"per_1","sessionID":"ses_1","action":"bash","resources":["one"]}`), "/work")
	store.Handle("permission.asked", json.RawMessage(`{"id":"per_1","sessionID":"ses_2","action":"bash","resources":["two"]}`), "/work")
	pending := store.Pending()
	if len(pending) != 2 {
		t.Fatalf("same request id in two sessions collided: %v", pending)
	}
	ids := []string{pending[0].ID, pending[1].ID}
	if !reflect.DeepEqual(ids, []string{"ses_1/per_1", "ses_2/per_1"}) {
		t.Fatalf("ids=%v", ids)
	}
	if err := store.Answer(context.Background(), "ses_1/per_1", json.RawMessage(`{"decision":"accept"}`)); err != nil {
		t.Fatal(err)
	}
	if len(replies) != 1 || replies[0] != (decisionReply{"ses_1", "per_1", "once"}) {
		t.Fatalf("replies=%+v", replies)
	}
	pending = store.Pending()
	if len(pending) != 1 || pending[0].ID != "ses_2/per_1" {
		t.Fatalf("wrong session answered: %v", pending)
	}
}

func TestDecisionsResolveOnTurnCompletion(t *testing.T) {
	collector := &eventCollector{}
	turn := "msg_user"
	store := NewDecisions("/work", collector.emit, func(session string) string {
		if session == "ses_1" {
			return turn
		}
		return "msg_other"
	}, func(context.Context, string, string, string) error { return nil })
	store.Handle("permission.asked", json.RawMessage(`{"id":"per_1","sessionID":"ses_1","action":"bash","resources":["one"]}`), "/work")
	turn = ""
	store.Handle("permission.asked", json.RawMessage(`{"id":"per_3","sessionID":"ses_1","action":"read","resources":["/a"]}`), "/work")
	turn = "msg_user"
	store.Handle("permission.asked", json.RawMessage(`{"id":"per_2","sessionID":"ses_2","action":"bash","resources":["two"]}`), "/work")

	store.Resolve("ses_1", "msg_user")
	pending := store.Pending()
	if len(pending) != 1 || pending[0].ID != "ses_2/per_2" {
		t.Fatalf("pending=%v", pending)
	}
	resolved := 0
	for _, event := range collector.snapshot() {
		if event.method == "decision/resolved" {
			resolved++
		}
	}
	if resolved != 2 {
		t.Fatalf("resolved events=%d", resolved)
	}
}

func TestUnsupportedFormFailsTurnVisibly(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	store := NewDecisions("/work", collector.emit, synth.TurnID, func(context.Context, string, string, string) error { return nil })
	synth.OnTurnComplete = store.Resolve
	store.OnUnsupportedInput = synth.UnsupportedInput
	synth.StartTurn("ses_1", "msg_user", "/work")
	store.Handle("form.created", json.RawMessage(`{"form":{"id":"frm_1","sessionID":"ses_1","title":"SECRET FORM TITLE","fields":[{"key":"q","title":"SECRET FIELD"}]}}`), "/work")

	var failed bool
	var notice string
	for _, event := range collector.snapshot() {
		if strings.Contains(string(event.params), "SECRET") {
			t.Fatalf("raw form data leaked: %s", event.params)
		}
		if event.method == "error" {
			var params struct {
				Notice struct {
					Category string `json:"category"`
					Message  string `json:"message"`
				} `json:"notice"`
			}
			_ = json.Unmarshal(event.params, &params)
			if params.Notice.Category == "unsupported_input" {
				notice = params.Notice.Message
			}
		}
		if event.method == "turn/completed" {
			var params struct {
				Turn struct {
					Status string `json:"status"`
				} `json:"turn"`
			}
			_ = json.Unmarshal(event.params, &params)
			if params.Turn.Status == "failed" {
				failed = true
			}
		}
	}
	if notice == "" || !failed {
		t.Fatalf("form request did not fail visibly: notice=%q failed=%v", notice, failed)
	}
	if id, active := synth.ActiveSession(); id != "" || active {
		t.Fatalf("turn still in progress after unsupported input: %q,%v", id, active)
	}
}

func TestDecisionCapFailsVisibly(t *testing.T) {
	collector := &eventCollector{}
	synth := NewSynthesizer("/work", collector.emit)
	store := NewDecisions("/work", collector.emit, synth.TurnID, func(context.Context, string, string, string) error { return nil })
	synth.OnTurnComplete = store.Resolve
	store.OnUnsupportedInput = synth.UnsupportedInput
	synth.StartTurn("ses_1", "msg_user", "/work")
	for i := 0; i < 128; i++ {
		store.Handle("permission.asked", perm(fmt.Sprintf("per_%d", i), "bash", "cmd"), "/work")
	}
	store.Handle("permission.asked", perm("per_overflow", "bash", "cmd"), "/work")

	required := 0
	failed := false
	capNotice := ""
	for _, event := range collector.snapshot() {
		switch event.method {
		case "decision/required":
			required++
		case "error":
			var params struct {
				Notice struct {
					Category string `json:"category"`
					Message  string `json:"message"`
				} `json:"notice"`
			}
			_ = json.Unmarshal(event.params, &params)
			if params.Notice.Category == "unsupported_input" && strings.Contains(params.Notice.Message, "too many pending approvals") {
				capNotice = params.Notice.Message
			}
		case "turn/completed":
			var params struct {
				Turn struct {
					Status string `json:"status"`
				} `json:"turn"`
			}
			_ = json.Unmarshal(event.params, &params)
			if params.Turn.Status == "failed" {
				failed = true
			}
		}
	}
	if required != 128 || capNotice == "" || !failed {
		t.Fatalf("cap was silent: required=%d notice=%q failed=%v", required, capNotice, failed)
	}
}

func TestPermissionReplyWithoutSessionIsUnambiguous(t *testing.T) {
	var replies []decisionReply
	store, collector := decisionStore(t, &replies, nil)
	store.Handle("permission.asked", json.RawMessage(`{"id":"per_1","sessionID":"ses_1","action":"bash","resources":["one"]}`), "/work")
	store.Handle("permission.asked", json.RawMessage(`{"id":"per_1","sessionID":"ses_2","action":"bash","resources":["two"]}`), "/work")
	store.Handle("permission.replied", json.RawMessage(`{"requestID":"per_1"}`), "/work")
	if len(store.Pending()) != 2 {
		t.Fatalf("ambiguous reply guessed a session: %v", store.Pending())
	}
	resolved := 0
	for _, event := range collector.snapshot() {
		if event.method == "decision/resolved" {
			resolved++
		}
	}
	if resolved != 0 {
		t.Fatalf("ambiguous reply emitted %d resolutions", resolved)
	}
	store.Handle("permission.replied", json.RawMessage(`{"sessionID":"ses_2","requestID":"per_1"}`), "/work")
	pending := store.Pending()
	if len(pending) != 1 || pending[0].ID != "ses_1/per_1" {
		t.Fatalf("wrong session resolved: %v", pending)
	}
}

func TestDecisionsReconcile(t *testing.T) {
	var replies []decisionReply
	store, collector := decisionStore(t, &replies, nil)
	store.Handle("permission.asked", perm("per_1", "bash", "one"), "/work")
	store.Reconcile("ses_1", "/work", json.RawMessage(`{"data":[]}`))
	if len(store.Pending()) != 0 {
		t.Fatal("resolved permission remained pending")
	}
	store.Reconcile("ses_1", "/work", json.RawMessage(`{"data":[{"id":"per_2","sessionID":"ses_1","action":"read","resources":["/a"]}]}`))
	pending := store.Pending()
	if len(pending) != 1 || pending[0].ID != "ses_1/per_2" {
		t.Fatalf("pending=%v", pending)
	}
	methods := eventMethods(collector.snapshot())
	if methods[len(methods)-2] != "decision/resolved" || methods[len(methods)-1] != "decision/required" {
		t.Fatalf("events=%v", methods)
	}
}

func TestDecisionsTurnLookupDoesNotBlockCompletion(t *testing.T) {
	for _, path := range []string{"event", "reconcile"} {
		t.Run(path, func(t *testing.T) {
			synth := NewSynthesizer("/work", nil)
			synth.StartTurn("ses_1", "msg_user", "/work")
			lookup := make(chan struct{})
			store := NewDecisions("/work", nil, func(session string) string {
				close(lookup)
				return synth.TurnID(session)
			}, nil)
			synth.OnTurnComplete = store.Resolve
			synth.mu.Lock()
			admitted := make(chan struct{})
			go func() {
				defer close(admitted)
				if path == "event" {
					store.Handle("permission.asked", perm("per_1", "bash", "cmd"), "/work")
				} else {
					store.Reconcile("ses_1", "/work", json.RawMessage(`{"data":[{"id":"per_1","sessionID":"ses_1","action":"bash"}]}`))
				}
			}()
			<-lookup
			resolved := make(chan struct{})
			go func() {
				synth.OnTurnComplete("ses_1", "msg_user")
				close(resolved)
			}()
			select {
			case <-resolved:
			case <-time.After(time.Second):
				synth.mu.Unlock()
				<-admitted
				<-resolved
				t.Fatal("completion blocked by a turn lookup holding the decision lock")
			}
			synth.mu.Unlock()
			<-admitted
			pending := store.Pending()
			if len(pending) != 1 || pending[0].TurnID != "msg_user" {
				t.Fatalf("pending=%+v", pending)
			}
		})
	}
}
