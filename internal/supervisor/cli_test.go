package supervisor

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestCLIIndependentLeasePrivateResponsesAndEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child lockedBuffer
	s := NewServer(ctx, "token", "instance", &child, cancel)
	phone, pd := connect(t, s, Hello{Token: "token"})
	defer phone.Close()
	var ready map[string]any
	if err := pd.Decode(&ready); err != nil {
		t.Fatal(err)
	}
	a, b := net.Pipe()
	go s.Handle(b)
	defer a.Close()
	_ = a.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := a.Write([]byte("{\"token\":\"token\",\"cli\":true}\n")); err != nil {
		t.Fatal(err)
	}
	cd := json.NewDecoder(a)
	ready = nil
	if err := cd.Decode(&ready); err != nil || ready["ready"] != true {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if _, err := a.Write([]byte("{\"id\":7,\"method\":\"threads/list\",\"params\":{}}\n")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for child.String() == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	var cmd struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal([]byte(child.String()), &cmd); err != nil || cmd.ID < 2000000000 {
		t.Fatalf("child=%s err=%v", child.String(), err)
	}
	if err := s.Publish(json.RawMessage(fmtResponse(cmd.ID))); err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := cd.Decode(&response); err != nil || response["id"] != float64(7) {
		t.Fatalf("response=%v err=%v", response, err)
	}
	if s.Journal.Checkpoint() != 0 {
		t.Fatal("private response entered journal")
	}
	if err := s.Publish(json.RawMessage(`{"event":"turn/completed","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := cd.Decode(&event); err != nil || event["event"] != "turn/completed" {
		t.Fatalf("event=%v err=%v", event, err)
	}
	var entry Entry
	if err := pd.Decode(&entry); err != nil || entry.Seq != 1 {
		t.Fatalf("entry=%v err=%v", entry, err)
	}
}

func fmtResponse(id int) []byte {
	raw, _ := json.Marshal(map[string]any{"id": id, "result": map[string]any{"ok": true}})
	return raw
}

func TestCLIIsolationAndDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child lockedBuffer
	s := NewServer(ctx, "token", "instance", &child, cancel)
	contexts := make(chan string, 2)
	s.OnCLIContext = func(value string) error { contexts <- value; return nil }
	cli, decoder := connect(t, s, Hello{Token: "token", CLI: true, SSHConnection: "192.0.2.1 1234 192.0.2.2 22"})
	var ready map[string]any
	if err := decoder.Decode(&ready); err != nil {
		t.Fatal(err)
	}
	if got := <-contexts; got != "192.0.2.1 1234 192.0.2.2 22" {
		t.Fatal(got)
	}
	second, secondDecoder := connect(t, s, Hello{Token: "token", CLI: true})
	defer second.Close()
	var reply map[string]any
	if err := secondDecoder.Decode(&reply); err != nil || reply["error"] == nil {
		t.Fatalf("reply=%v err=%v", reply, err)
	}
	if err := s.Publish(json.RawMessage(fmtResponse(cliIDStart))); err != nil {
		t.Fatal(err)
	}
	if s.Journal.Checkpoint() != 0 {
		t.Fatal("unknown reserved response was journaled")
	}
	cli.Close()
	select {
	case got := <-contexts:
		if got != "" {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("CLI context not cleared")
	}
	select {
	case <-ctx.Done():
		t.Fatal("CLI disconnect stopped supervisor")
	default:
	}
	phone, pd := connect(t, s, Hello{Token: "token"})
	defer phone.Close()
	if err := pd.Decode(&ready); err != nil {
		t.Fatal(err)
	}
	if _, err := phone.Write([]byte("{\"id\":2000000000,\"method\":\"threads/list\"}\n")); err != nil {
		t.Fatal(err)
	}
	var ignored any
	if err := pd.Decode(&ignored); err == nil {
		t.Fatal("phone forged reserved request")
	}
	if child.String() != "" {
		t.Fatalf("unexpected child command %s", child.String())
	}
}

func TestCLIStalledReaderDoesNotBlockPublisher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child lockedBuffer
	s := NewServer(ctx, "token", "instance", &child, cancel)
	cli, decoder := connect(t, s, Hello{Token: "token", CLI: true})
	defer cli.Close()
	var ready map[string]any
	if err := decoder.Decode(&ready); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := cli.Write([]byte("{\"id\":1,\"method\":\"threads/list\"}\n")); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		n := len(s.cli.pending)
		s.mu.Unlock()
		if n == 8 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("requests not queued")
		}
		time.Sleep(time.Millisecond)
	}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 8; i++ {
			_ = s.Publish(json.RawMessage(fmtResponse(cliIDStart + i)))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stalled CLI blocked backend")
	}
	select {
	case <-ctx.Done():
		t.Fatal("stalled CLI stopped supervisor")
	default:
	}
}

func TestCLIRejectsUnsupportedCommandsAndDoesNotReplayHistory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child lockedBuffer
	s := NewServer(ctx, "token", "instance", &child, cancel)
	if err := s.Publish(json.RawMessage(`{"event":"old","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	cli, decoder := connect(t, s, Hello{Token: "token", CLI: true})
	defer cli.Close()
	var ready map[string]any
	if err := decoder.Decode(&ready); err != nil {
		t.Fatal(err)
	}
	if err := s.Publish(json.RawMessage(`{"event":"new","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := decoder.Decode(&event); err != nil || event["event"] != "new" {
		t.Fatalf("event=%v err=%v", event, err)
	}
	if _, err := cli.Write([]byte("{\"id\":1,\"method\":\"client/attached\",\"params\":{\"attached\":false}}\n")); err != nil {
		t.Fatal(err)
	}
	var reply any
	if err := decoder.Decode(&reply); err == nil {
		t.Fatal("lifecycle accepted from CLI")
	}
	if child.String() != "" {
		t.Fatalf("forwarded forbidden command %s", child.String())
	}
}

func TestPublishDoesNotWaitForDisconnectContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child lockedBuffer
	s := NewServer(ctx, "token", "instance", &child, cancel)
	entered := make(chan struct{})
	release := make(chan struct{})
	s.OnCLIContext = func(value string) error {
		if value == "" {
			close(entered)
			<-release
		}
		return nil
	}
	cli, decoder := connect(t, s, Hello{Token: "token", CLI: true, SSHConnection: "192.0.2.1 1234 192.0.2.2 22"})
	var ready map[string]any
	if err := decoder.Decode(&ready); err != nil {
		t.Fatal(err)
	}
	cli.Close()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("disconnect context not entered")
	}
	done := make(chan struct{})
	go func() { _ = s.Publish(json.RawMessage(fmtResponse(cliIDStart))); close(done) }()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		close(release)
		t.Fatal("publish blocked behind disconnect context")
	}
	close(release)
}

func TestPublishDoesNotWaitForPhoneDetach(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child lockedBuffer
	s := NewServer(ctx, "token", "instance", &child, cancel)
	entered := make(chan struct{})
	release := make(chan struct{})
	s.OnAttachment = func(attached bool, _ string) error {
		if !attached {
			close(entered)
			<-release
		}
		return nil
	}
	phone, decoder := connect(t, s, Hello{Token: "token"})
	var ready map[string]any
	if err := decoder.Decode(&ready); err != nil {
		t.Fatal(err)
	}
	phone.Close()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("phone detach not entered")
	}
	done := make(chan struct{})
	go func() { _ = s.Publish(json.RawMessage(fmtResponse(cliIDStart))); close(done) }()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		close(release)
		t.Fatal("publish blocked behind phone detach")
	}
	close(release)
}

func TestCLIReceivesOnlyCurrentlyPendingDecisions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child lockedBuffer
	s := NewServer(ctx, "token", "instance", &child, cancel)
	for _, raw := range []string{`{"event":"turn/text","params":{"text":"old"}}`, `{"event":"decision/required","params":{"decision_id":"a"}}`, `{"event":"decision/required","params":{"decision_id":"b"}}`, `{"event":"decision/resolved","params":{"decision_id":"a"}}`} {
		if err := s.Publish(json.RawMessage(raw)); err != nil {
			t.Fatal(err)
		}
	}
	cli, decoder := connect(t, s, Hello{Token: "token", CLI: true})
	defer cli.Close()
	var ready, event map[string]any
	if err := decoder.Decode(&ready); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&event); err != nil || event["event"] != "decision/required" || event["params"].(map[string]any)["decision_id"] != "b" {
		t.Fatalf("event=%v err=%v", event, err)
	}
	if err := s.Publish(json.RawMessage(`{"event":"decision/resolved","params":{"decision_id":"b"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&event); err != nil || event["event"] != "decision/resolved" {
		t.Fatalf("event=%v err=%v", event, err)
	}
	cli.Close()
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		closed := s.cli == nil
		s.mu.Unlock()
		if closed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("CLI lease not released")
		}
		time.Sleep(time.Millisecond)
	}
	next, nd := connect(t, s, Hello{Token: "token", CLI: true})
	defer next.Close()
	if err := nd.Decode(&ready); err != nil {
		t.Fatal(err)
	}
	if err := s.Publish(json.RawMessage(`{"event":"fresh","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	if err := nd.Decode(&event); err != nil || event["event"] != "fresh" {
		t.Fatalf("event=%v err=%v", event, err)
	}
}

func TestCLIDecisionSnapshotOverflowRejectsAttachUntilReset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child lockedBuffer
	s := NewServer(ctx, "token", "instance", &child, cancel)
	for i := 0; i < 129; i++ {
		raw, _ := json.Marshal(map[string]any{"event": "decision/required", "params": map[string]any{"decision_id": strconv.Itoa(i)}})
		if err := s.Publish(raw); err != nil {
			t.Fatal(err)
		}
	}
	cli, decoder := connect(t, s, Hello{Token: "token", CLI: true})
	defer cli.Close()
	var reply map[string]any
	if err := decoder.Decode(&reply); err != nil || reply["error"] == nil {
		t.Fatalf("reply=%v err=%v", reply, err)
	}
	if err := s.Publish(json.RawMessage(`{"event":"decision/reset","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	next, nd := connect(t, s, Hello{Token: "token", CLI: true})
	defer next.Close()
	if err := nd.Decode(&reply); err != nil || reply["ready"] != true {
		t.Fatalf("reply=%v err=%v", reply, err)
	}
}
