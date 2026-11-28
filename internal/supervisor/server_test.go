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

func TestRelayRejectsForgedAttachmentCommands(t *testing.T) {
	for _, frame := range []string{
		`{"id":1,"method":"client/attached","params":{"attached":true,"ssh_connection":"forged"}}`,
		`{"id":2147483646,"method":"status"}`,
	} {
		t.Run(frame, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var child lockedBuffer
			s := NewServer(ctx, "token", "instance", &child, cancel)
			client, decoder := connect(t, s, Hello{Token: "token"})
			defer client.Close()
			var ready map[string]any
			if err := decoder.Decode(&ready); err != nil {
				t.Fatal(err)
			}
			if _, err := client.Write([]byte(frame + "\n")); err != nil {
				t.Fatal(err)
			}
			var response any
			if err := decoder.Decode(&response); err == nil {
				t.Fatal("forged command accepted")
			}
			if got := child.String(); got != "" {
				t.Fatalf("forwarded forged command: %q", got)
			}
		})
	}
}

func TestHeartbeatLeaseReleasesSilentClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewServer(ctx, "token", "instance", &bytes.Buffer{}, cancel)
	s.leaseTimeout = 300 * time.Millisecond
	client, decoder := connect(t, s, Hello{Token: "token", Heartbeat: true})
	defer client.Close()
	var reply map[string]any
	if err := decoder.Decode(&reply); err != nil || reply["ready"] != true {
		t.Fatalf("reply=%v err=%v", reply, err)
	}
	deadline := time.Now().Add(time.Second)
	for s.Attached() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.Attached() {
		t.Fatal("silent heartbeat client kept the supervisor attachment")
	}
	next, nextDecoder := connect(t, s, Hello{Token: "token"})
	defer next.Close()
	reply = nil
	if err := nextDecoder.Decode(&reply); err != nil || reply["ready"] != true {
		t.Fatalf("replacement reply=%v err=%v", reply, err)
	}
}

func TestHeartbeatRenewsLeaseWithoutForwardingCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child lockedBuffer
	s := NewServer(ctx, "token", "instance", &child, cancel)
	s.leaseTimeout = 200 * time.Millisecond
	client, decoder := connect(t, s, Hello{Token: "token", Heartbeat: true})
	defer client.Close()
	var reply map[string]any
	if err := decoder.Decode(&reply); err != nil || reply["ready"] != true {
		t.Fatalf("reply=%v err=%v", reply, err)
	}
	for range 5 {
		time.Sleep(80 * time.Millisecond)
		if _, err := client.Write([]byte("{\"heartbeat\":true}\n")); err != nil {
			t.Fatal(err)
		}
	}
	if !s.Attached() {
		t.Fatal("heartbeat client lost its lease")
	}
	if child.String() != "" {
		t.Fatalf("heartbeat reached Codex bridge: %q", child.String())
	}
}

func TestHeartbeatLeaseTracksPartialFrameActivity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child lockedBuffer
	s := NewServer(ctx, "token", "instance", &child, cancel)
	s.leaseTimeout = 200 * time.Millisecond
	client, decoder := connect(t, s, Hello{Token: "token", Heartbeat: true})
	defer client.Close()
	var reply map[string]any
	if err := decoder.Decode(&reply); err != nil || reply["ready"] != true {
		t.Fatalf("reply=%v err=%v", reply, err)
	}
	for index, part := range []string{`{"id":1,`, `"method":`, `"status"`, "}\n"} {
		if index > 0 {
			time.Sleep(80 * time.Millisecond)
		}
		if _, err := client.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(time.Second)
	for child.String() == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if child.String() != "{\"id\":1,\"method\":\"status\"}\n" {
		t.Fatalf("partial command was not forwarded: %q", child.String())
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

func TestFreshAttachRecoversExpiredJournalWithoutRestartOrCommandReplay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var child lockedBuffer
	s := NewServer(ctx, "token", "same-instance", &child, cancel)
	s.Journal = NewJournal(2, 1024)
	for i := 0; i < 4; i++ {
		if err := s.Journal.Append(json.RawMessage(`{"event":"old-delta"}`)); err != nil {
			t.Fatal(err)
		}
	}
	stale, decoder := connect(t, s, Hello{Token: "token", Instance: "same-instance", Cursor: 1})
	var reply map[string]any
	if err := decoder.Decode(&reply); err != nil || reply["error"] == nil {
		t.Fatalf("expired resume must remain explicit: reply=%v err=%v", reply, err)
	}
	stale.Close()
	s.OnAttachment = func(attached bool, _ string) error {
		if attached {
			return s.Journal.Append(json.RawMessage(`{"event":"decision/reset","params":{}}`))
		}
		return nil
	}
	client, decoder := connect(t, s, Hello{Token: "token"})
	defer client.Close()
	reply = nil
	if err := decoder.Decode(&reply); err != nil || reply["ready"] != true || reply["instance"] != "same-instance" || reply["resynced"] != true || reply["cursor"] != float64(4) {
		t.Fatalf("fresh attach must acknowledge a new checkpoint: reply=%v err=%v", reply, err)
	}
	var entry Entry
	if err := decoder.Decode(&entry); err != nil || entry.Seq != 5 || !bytes.Contains(entry.Message, []byte("decision/reset")) {
		t.Fatalf("must deliver events after the checkpoint: entry=%v err=%v", entry, err)
	}
	if ctx.Err() != nil || child.String() != "" {
		t.Fatal("recovery restarted the backend or replayed a command")
	}
}

func TestAttachmentSSHContextIsValidatedAndPrivate(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"192.0.2.1 1234 2001:db8::1 22", "192.0.2.1 1234 2001:db8::1 22"},
		{"", ""}, {"bad 1234 192.0.2.2 22", ""}, {"192.0.2.1 0 192.0.2.2 22", ""},
		{"192.0.2.1 65536 192.0.2.2 22", ""}, {"192.0.2.1 12 192.0.2.2 -1", ""},
		{"192.0.2.1 12 192.0.2.2 22 extra", ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := NewServer(ctx, "token", "instance", &lockedBuffer{}, cancel)
			type event struct {
				attached bool
				context  string
			}
			events := make(chan event, 2)
			s.OnAttachment = func(attached bool, ssh string) error { events <- event{attached, ssh}; return nil }
			client, decoder := connect(t, s, Hello{Token: "token", SSHConnection: tc.input})
			var ready json.RawMessage
			if err := decoder.Decode(&ready); err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(ready, []byte("ssh_connection")) || (tc.input != "" && bytes.Contains(ready, []byte(tc.input))) {
				t.Fatalf("context leaked in ready: %s", ready)
			}
			if got := <-events; !got.attached || got.context != tc.want {
				t.Fatalf("attach = %+v, want %q", got, tc.want)
			}
			client.Close()
			select {
			case got := <-events:
				if got.attached || got.context != "" {
					t.Fatalf("detach = %+v", got)
				}
			case <-time.After(time.Second):
				t.Fatal("missing detach")
			}
			entries, _, err := s.Journal.After(0)
			if err != nil || len(entries) != 0 {
				t.Fatalf("context entered journal: %v %v", entries, err)
			}
		})
	}
}

func TestReadyMetadataAndAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name, backend string
		capabilities  []string
		want          string
	}{
		{"legacy", "", nil, `{"instance":"instance","ready":true}`},
		{"opencode", "opencode", []string{}, `{"backend":"opencode","capabilities":[],"instance":"instance","ready":true}`},
		{"codex", "codex", []string{"session_status", "plan_mode"}, `{"backend":"codex","capabilities":["session_status","plan_mode"],"instance":"instance","ready":true}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := NewServer(ctx, "token", "instance", &bytes.Buffer{}, cancel)
			s.Backend, s.Capabilities = tc.backend, tc.capabilities
			bad, decoder := connect(t, s, Hello{Token: "bad"})
			var rejected map[string]any
			if err := decoder.Decode(&rejected); err != nil || rejected["error"] == nil || rejected["backend"] != nil || rejected["capabilities"] != nil {
				t.Fatalf("rejected=%v err=%v", rejected, err)
			}
			bad.Close()
			client, decoder := connect(t, s, Hello{Token: "token"})
			defer client.Close()
			var reply json.RawMessage
			if err := decoder.Decode(&reply); err != nil || string(reply) != tc.want {
				t.Fatalf("ready=%s err=%v, want %s", reply, err, tc.want)
			}
		})
	}
}
