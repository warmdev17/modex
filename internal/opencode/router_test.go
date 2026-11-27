//go:build !windows

package opencode

import (
	"context"
	"encoding/json"
	"github.com/warmdev/modex/internal/bridge"
	"os"
	"testing"
)

func TestManagedEventsRewriteDecisionAndRejectOtherWorkspace(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	registry, err := openRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := registry.add("native", "/work", "p", false)
	if err != nil {
		t.Fatal(err)
	}
	events := []map[string]any{}
	router := &Router{registry: registry, emit: func(_ string, raw json.RawMessage) {
		var v map[string]any
		_ = json.Unmarshal(raw, &v)
		events = append(events, v)
	}}
	router.emitEvent("/other", "decision/required", json.RawMessage(`{"thread_id":"native","decision_id":"native/request"}`))
	router.emitEvent("/work", "decision/required", json.RawMessage(`{"thread_id":"native","decision_id":"native/request"}`))
	router.emitEvent("/work", "decision/resolved", json.RawMessage(`{"decision_id":"native/request"}`))
	if len(events) != 2 || events[0]["thread_id"] != chat.ID || events[0]["decision_id"] != chat.ID+"/request" || events[1]["decision_id"] != chat.ID+"/request" {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestManagedUnknownThreadDoesNotFallThrough(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	registry, err := openRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	router := &Router{registry: registry}
	_, handled, err := router.Handle(context.Background(), bridge.Command{Method: "threads/read", Params: json.RawMessage(`{"thread_id":"oc:unknown"}`)}, nil)
	if !handled || err == nil {
		t.Fatalf("unknown ID: handled=%v err=%v", handled, err)
	}
}

func TestManagedTurnExcludesPendingSnapshotsBeforeSubmission(t *testing.T) {
	store := NewDecisions("/work", nil, func(string) string { return "turn" }, nil)
	permissions := json.RawMessage(`{"data":[{"id":"old","sessionID":"ses_1","action":"write"}]}`)
	forms := []nativeForm{{ID: "frm_old", SessionID: "ses_1"}}
	if err := store.excludeExisting("ses_1", permissions, forms); err != nil {
		t.Fatal(err)
	}
	store.Reconcile("ses_1", "/work", json.RawMessage(`{"data":[{"id":"old","sessionID":"ses_1","action":"write"},{"id":"new","sessionID":"ses_1","action":"write"}]}`))
	pending := store.Pending()
	if len(pending) != 1 || pending[0].ID != "ses_1/new" {
		t.Fatalf("stale permission was exposed: %+v", pending)
	}
	if _, ok := store.consumed["ses_1/frm_old"]; !ok {
		t.Fatal("stale form was not excluded")
	}
	if err := store.excludeExisting("ses_1", json.RawMessage(`{}`), nil); err == nil {
		t.Fatal("accepted missing permission snapshot")
	}
	if err := store.excludeExisting("ses_1", json.RawMessage(`{"data":[{"id":"x","sessionID":"ses_other","action":"write"}]}`), nil); err == nil {
		t.Fatal("accepted foreign permission")
	}
}

func TestManagedDecisionRespondUsesMappedNativeIdentity(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	registry, err := openRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := registry.add("ses_1", dir, "", true)
	if err != nil {
		t.Fatal(err)
	}
	synth := NewSynthesizer(dir, nil)
	synth.StartTurn("ses_1", "turn", dir)
	replied := false
	decisions := NewDecisions(dir, nil, synth.TurnID, func(_ context.Context, session, request, answer string) error {
		replied = session == "ses_1" && request == "req_1" && answer == "once"
		return nil
	})
	decisions.Handle("permission.asked", json.RawMessage(`{"id":"req_1","sessionID":"ses_1","action":"bash"}`), dir)
	router := &Router{registry: registry, workspaces: map[string]*managedWorkspace{dir: {synth: synth, decisions: decisions}}}
	params, _ := json.Marshal(map[string]any{"decision_id": chat.ID + "/req_1", "result": map[string]string{"decision": "accept"}})
	_, handled, err := router.Handle(context.Background(), bridge.Command{Method: "decision/respond", Params: params}, nil)
	if err != nil || !handled || !replied {
		t.Fatalf("decision response: handled=%v replied=%v err=%v", handled, replied, err)
	}
	if threads := router.Threads(); len(threads) != 1 || threads[0]["projectless"] != true {
		t.Fatalf("projectless list: %+v", threads)
	}
}

func TestManagedRuntimeRequiresCompatibleVersion(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"2.4.1", true}, {"2.5.0", true}, {"v2.4.0", true}, {"2.3.9", false}, {"1.9.9", false}, {"3.0.0", false}, {"", false}, {"2.4", false}, {"2.4.0-rc1", false},
	} {
		if got := compatibleVersion(tc.version); got != tc.want {
			t.Errorf("version %q: got %v want %v", tc.version, got, tc.want)
		}
	}
}
