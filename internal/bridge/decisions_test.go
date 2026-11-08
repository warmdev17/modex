package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestDecisionValidationAndSingleResponse(t *testing.T) {
	calls := 0
	d := NewDecisions(func(_ context.Context, id, result json.RawMessage) error {
		calls++
		if string(id) != `"upstream"` || string(result) != `{"decision":"accept"}` {
			t.Fatalf("id=%s result=%s", id, result)
		}
		return nil
	})
	event, err := d.Add(json.RawMessage(`"upstream"`), "item/commandExecution/requestApproval", json.RawMessage(`{"threadId":"one","turnId":"t","itemId":"i","command":"echo ok","secret":"never-emit"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(event)
	if strings.Contains(string(raw), "never-emit") {
		t.Fatal("leaked unknown fields")
	}
	if err := d.Answer(context.Background(), event.ID, json.RawMessage(`{"decision":"acceptForSession"}`)); err == nil {
		t.Fatal("accepted policy escalation")
	}
	if len(d.List()) != 1 {
		t.Fatal("invalid answer removed decision")
	}
	if err := d.Answer(context.Background(), event.ID, json.RawMessage(`{"decision":"accept"}`)); err != nil {
		t.Fatal(err)
	}
	if err := d.Answer(context.Background(), event.ID, json.RawMessage(`{"decision":"accept"}`)); err == nil {
		t.Fatal("replayed decision")
	}
	if calls != 1 || len(d.List()) != 0 {
		t.Fatalf("calls=%d pending=%d", calls, len(d.List()))
	}
}

func TestQuestionsAndResolution(t *testing.T) {
	d := NewDecisions(func(context.Context, json.RawMessage, json.RawMessage) error { return nil })
	event, err := d.Add(json.RawMessage(`9`), "item/tool/requestUserInput", json.RawMessage(`{"threadId":"one","turnId":"t","itemId":"i","isBlocking":true,"questions":[{"id":"q1","header":"Choice","question":"Which?","options":[{"label":"A","description":"First"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{`{"answers":{}}`, `{"answers":{"q1":{"answers":[]}}}`, `{"answers":{"other":{"answers":["A"]}}}`, `{"answers":{"q1":{"answers":["A"]}},"extra":true}`} {
		if d.Answer(context.Background(), event.ID, json.RawMessage(invalid)) == nil {
			t.Fatalf("accepted %s", invalid)
		}
	}
	if err := d.Answer(context.Background(), event.ID, json.RawMessage(`{"answers":{"q1":{"answers":["A"]}}}`)); err != nil {
		t.Fatal(err)
	}
	event, err = d.Add(json.RawMessage(`10`), "item/fileChange/requestApproval", json.RawMessage(`{"threadId":"one","turnId":"t","itemId":"i"}`))
	if err != nil {
		t.Fatal(err)
	}
	d.Resolve("serverRequest/resolved", json.RawMessage(`{"threadId":"one","requestId":10}`))
	if d.Answer(context.Background(), event.ID, json.RawMessage(`{"decision":"accept"}`)) == nil {
		t.Fatal("answered resolved request")
	}
}

func TestDecisionRejectsUnsupportedAndDuplicateRequests(t *testing.T) {
	d := NewDecisions(func(context.Context, json.RawMessage, json.RawMessage) error { return nil })
	if _, err := d.Add(json.RawMessage(`1`), "account/chatgptAuthTokens/refresh", json.RawMessage(`{}`)); err == nil {
		t.Fatal("accepted auth request")
	}
	for _, raw := range []string{`{}`, `{"threadId":"x","turnId":"t"}`, `{"threadId":"x","turnId":"t","itemId":"i","questions":[{"id":"q","question":"secret","isSecret":true}]}`} {
		if _, err := d.Add(json.RawMessage(`1`), "item/tool/requestUserInput", json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	params := json.RawMessage(`{"threadId":"x","turnId":"t","itemId":"i"}`)
	first, err := d.Add(json.RawMessage(`1`), "item/fileChange/requestApproval", params)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Add(json.RawMessage(`1`), "item/fileChange/requestApproval", params); err == nil {
		t.Fatal("accepted duplicate ID")
	}
	d.Resolve("turn/completed", json.RawMessage(`{"threadId":"x","turn":{"id":"t"}}`))
	if len(d.List()) != 0 {
		t.Fatal("stale turn decision")
	}
	next, err := d.Add(json.RawMessage(`1`), "item/fileChange/requestApproval", params)
	if err != nil || next.ID == first.ID {
		t.Fatal("reused local token")
	}
}

func TestDecisionWriteFailureCannotReplay(t *testing.T) {
	calls := 0
	d := NewDecisions(func(context.Context, json.RawMessage, json.RawMessage) error {
		calls++
		return errors.New("credential-bearing raw failure")
	})
	event, err := d.Add(json.RawMessage(`2`), "item/fileChange/requestApproval", json.RawMessage(`{"threadId":"x","turnId":"t","itemId":"i"}`))
	if err != nil {
		t.Fatal(err)
	}
	err = d.Answer(context.Background(), event.ID, json.RawMessage(`{"decision":"decline"}`))
	if err == nil || strings.Contains(err.Error(), "credential") {
		t.Fatalf("err=%v", err)
	}
	_ = d.Answer(context.Background(), event.ID, json.RawMessage(`{"decision":"decline"}`))
	if calls != 1 {
		t.Fatal("replayed uncertain write")
	}
}

func TestDecisionHonorsAvailableChoicesAndCancellation(t *testing.T) {
	d := NewDecisions(func(context.Context, json.RawMessage, json.RawMessage) error { t.Fatal("unexpected write"); return nil })
	event, err := d.Add(json.RawMessage(`2`), "item/commandExecution/requestApproval", json.RawMessage(`{"threadId":"x","turnId":"t","itemId":"i","availableDecisions":["decline"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Answer(context.Background(), event.ID, json.RawMessage(`{"decision":"accept"}`)) == nil {
		t.Fatal("accepted unavailable decision")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if d.Answer(ctx, event.ID, json.RawMessage(`{"decision":"decline"}`)) == nil || len(d.List()) != 1 {
		t.Fatal("canceled decision consumed")
	}
}
