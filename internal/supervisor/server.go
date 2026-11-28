package supervisor

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/warmdev/modex/internal/bridge"
)

const MaxFrame = 4 * 1024 * 1024
const AttachmentCommandID = 2147483646

type Hello struct {
	Token         string `json:"token"`
	Instance      string `json:"instance,omitempty"`
	Cursor        uint64 `json:"cursor"`
	Stop          bool   `json:"stop,omitempty"`
	Heartbeat     bool   `json:"heartbeat,omitempty"`
	SSHConnection string `json:"ssh_connection,omitempty"`
}

type Server struct {
	Journal         *Journal
	Backend         string
	Capabilities    []string
	ctx             context.Context
	token, instance string
	child           io.Writer
	stop            context.CancelFunc
	mu              sync.Mutex
	attached        bool
	OnAttachment    func(bool, string) error
	childMu         sync.Mutex
	leaseTimeout    time.Duration
}

type leaseReader struct {
	net.Conn
	timeout time.Duration
	enabled bool
}

func (r *leaseReader) Read(p []byte) (int, error) {
	n, err := r.Conn.Read(p)
	if n > 0 && r.enabled {
		_ = r.SetReadDeadline(time.Now().Add(r.timeout))
	}
	return n, err
}

func NewServer(ctx context.Context, token, instance string, child io.Writer, stop context.CancelFunc) *Server {
	return &Server{Journal: NewJournal(1024, 2*bridge.MaxResponseFrame), ctx: ctx, token: token, instance: instance, child: child, stop: stop, leaseTimeout: time.Minute}
}

func (s *Server) Attached() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.attached }

func validSSHConnection(value string) string {
	fields := strings.Fields(value)
	if len(fields) != 4 {
		return ""
	}
	for _, i := range []int{0, 2} {
		if _, err := netip.ParseAddr(fields[i]); err != nil {
			return ""
		}
	}
	for _, i := range []int{1, 3} {
		for _, c := range fields[i] {
			if c < '0' || c > '9' {
				return ""
			}
		}
		port, err := strconv.ParseUint(fields[i], 10, 16)
		if err != nil || port == 0 {
			return ""
		}
	}
	return strings.Join(fields, " ")
}

func (s *Server) Handle(conn net.Conn) {
	defer conn.Close()
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	go func() { <-ctx.Done(); _ = conn.Close() }()
	input := &leaseReader{Conn: conn, timeout: s.leaseTimeout}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), MaxFrame)
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	send := func(value any) error {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return json.NewEncoder(conn).Encode(value)
	}
	reject := func(message string) { _ = send(map[string]string{"error": message}) }
	if !scanner.Scan() || len(scanner.Bytes()) > 4096 {
		return
	}
	var hello Hello
	if json.Unmarshal(scanner.Bytes(), &hello) != nil || subtle.ConstantTimeCompare([]byte(hello.Token), []byte(s.token)) != 1 {
		reject("authentication failed")
		return
	}
	if hello.Stop {
		_ = send(map[string]bool{"stopping": true})
		s.stop()
		return
	}
	if (hello.Cursor > 0 && hello.Instance != s.instance) || (hello.Instance != "" && hello.Instance != s.instance) {
		reject("supervisor instance changed; do not reuse cursor")
		return
	}
	cursor := hello.Cursor
	resynced := false
	if _, _, err := s.Journal.After(cursor); err != nil {
		if cursor != 0 {
			reject(err.Error())
			return
		}
		// A deliberate reset starts from current state, not an expired replay.
		cursor = s.Journal.Checkpoint()
		resynced = true
	}
	s.mu.Lock()
	if s.attached {
		s.mu.Unlock()
		reject("another client is attached")
		return
	}
	s.attached = true
	s.mu.Unlock()
	attachment := func(value bool) error {
		if s.OnAttachment == nil || s.ctx.Err() != nil {
			return nil
		}
		s.childMu.Lock()
		defer s.childMu.Unlock()
		ssh := ""
		if value {
			ssh = validSSHConnection(hello.SSHConnection)
		}
		return s.OnAttachment(value, ssh)
	}
	defer func() {
		cancel()
		s.mu.Lock()
		defer s.mu.Unlock()
		if attachment(false) != nil {
			s.stop()
		}
		s.attached = false
	}()
	if attachment(true) != nil {
		s.stop()
		return
	}
	ready := map[string]any{"ready": true, "instance": s.instance}
	if s.Backend != "" {
		ready["backend"], ready["capabilities"] = s.Backend, s.Capabilities
	}
	if resynced {
		ready["resynced"], ready["cursor"] = true, cursor
	}
	if send(ready) != nil {
		return
	}
	input.enabled = hello.Heartbeat
	if hello.Heartbeat {
		_ = conn.SetReadDeadline(time.Now().Add(s.leaseTimeout))
	} else {
		_ = conn.SetReadDeadline(time.Time{})
	}
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer cancel()
		for scanner.Scan() {
			if hello.Heartbeat {
				if bytes.Equal(scanner.Bytes(), []byte(`{"heartbeat":true}`)) {
					continue
				}
			}
			command, err := bridge.DecodeCommand(scanner.Bytes())
			if err != nil || command.Method == "client/attached" || command.ID == AttachmentCommandID {
				return
			}
			s.childMu.Lock()
			if ctx.Err() != nil {
				s.childMu.Unlock()
				return
			}
			_, err = s.child.Write(append(append([]byte(nil), scanner.Bytes()...), '\n'))
			s.childMu.Unlock()
			if err != nil {
				s.stop()
				return
			}
		}
	}()
	for {
		entries, wake, err := s.Journal.After(cursor)
		if err != nil {
			reject(err.Error())
			return
		}
		for _, entry := range entries {
			if send(entry) != nil {
				return
			}
			cursor = entry.Seq
		}
		select {
		case <-ctx.Done():
			return
		case <-readerDone:
			return
		case <-wake:
		}
	}
}
