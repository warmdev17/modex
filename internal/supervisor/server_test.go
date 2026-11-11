package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}
func (b *lockedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.b.String() }

func connect(t *testing.T, s *Server, h Hello) (net.Conn, *json.Decoder) {
	t.Helper()
	a, b := net.Pipe()
	go s.Handle(b)
	_ = a.SetDeadline(time.Now().Add(2 * time.Second))
	if err := json.NewEncoder(a).Encode(h); err != nil {
		t.Fatal(err)
	}
	return a, json.NewDecoder(a)
}

func TestServerAuthReplayAndSingleWriter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child lockedBuffer
	s := NewServer(ctx, "token", "instance", &child, cancel)
	_ = s.Journal.Append(json.RawMessage(`{"event":"first"}`))
	bad, d := connect(t, s, Hello{Token: "bad"})
	var reply map[string]any
	if err := d.Decode(&reply); err != nil || reply["error"] == nil {
		t.Fatalf("reply=%v err=%v", reply, err)
	}
	bad.Close()
	client, d := connect(t, s, Hello{Token: "token"})
	defer client.Close()
	if err := d.Decode(&reply); err != nil || reply["ready"] != true {
		t.Fatalf("reply=%v err=%v", reply, err)
	}
	var entry Entry
	if err := d.Decode(&entry); err != nil || entry.Seq != 1 {
		t.Fatalf("entry=%v err=%v", entry, err)
	}
	other, otherD := connect(t, s, Hello{Token: "token"})
	defer other.Close()
	reply = nil
	if err := otherD.Decode(&reply); err != nil || reply["error"] == nil {
		t.Fatal("second writer accepted", err)
	}
	if _, err := client.Write([]byte("{\"id\":1,\"method\":\"status\"}\n")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for child.String() == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if child.String() != "{\"id\":1,\"method\":\"status\"}\n" {
		t.Fatalf("child=%s", child.String())
	}
	client.Close()
	select {
	case <-ctx.Done():
		t.Fatal("client loss stopped supervisor")
	default:
	}
	// Wait until the old writer has been released, then replay an offline event.
	deadline = time.Now().Add(time.Second)
	for s.Attached() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	_ = s.Journal.Append(json.RawMessage(`{"event":"offline-completion"}`))
	next, nextD := connect(t, s, Hello{Token: "token", Instance: "instance", Cursor: 1})
	defer next.Close()
	reply = nil
	if err := nextD.Decode(&reply); err != nil || reply["ready"] != true {
		t.Fatalf("reply=%v err=%v", reply, err)
	}
	if err := nextD.Decode(&entry); err != nil || entry.Seq != 2 {
		t.Fatalf("entry=%v err=%v", entry, err)
	}
}

func TestServerRejectsStaleInstanceAndAuthenticatedStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewServer(ctx, "token", "instance", &bytes.Buffer{}, cancel)
	client, d := connect(t, s, Hello{Token: "token", Cursor: 1, Instance: "old"})
	defer client.Close()
	var reply map[string]any
	if err := d.Decode(&reply); err != nil || reply["error"] == nil {
		t.Fatal("stale instance accepted", err)
	}
	stop, decoder := connect(t, s, Hello{Token: "token", Stop: true})
	defer stop.Close()
	if err := decoder.Decode(&reply); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel")
	}
}
