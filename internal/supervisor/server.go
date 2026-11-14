package supervisor

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net"
	"sync"
	"time"

	"github.com/warmdev/modex/internal/bridge"
)

const MaxFrame = 4 * 1024 * 1024

type Hello struct {
	Token    string `json:"token"`
	Instance string `json:"instance,omitempty"`
	Cursor   uint64 `json:"cursor"`
	Stop     bool   `json:"stop,omitempty"`
}

type Server struct {
	Journal         *Journal
	ctx             context.Context
	token, instance string
	child           io.Writer
	stop            context.CancelFunc
	mu              sync.Mutex
	attached        bool
	OnAttachment    func(bool) error
	childMu         sync.Mutex
}

func NewServer(ctx context.Context, token, instance string, child io.Writer, stop context.CancelFunc) *Server {
	return &Server{Journal: NewJournal(1024, 2*bridge.MaxResponseFrame), ctx: ctx, token: token, instance: instance, child: child, stop: stop}
}

func (s *Server) Attached() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.attached }

func (s *Server) Handle(conn net.Conn) {
	defer conn.Close()
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	go func() { <-ctx.Done(); _ = conn.Close() }()
	scanner := bufio.NewScanner(conn)
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
	if _, _, err := s.Journal.After(hello.Cursor); err != nil {
		reject(err.Error())
		return
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
		return s.OnAttachment(value)
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
	if send(map[string]any{"ready": true, "instance": s.instance}) != nil {
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer cancel()
		for scanner.Scan() {
			if _, err := bridge.DecodeCommand(scanner.Bytes()); err != nil {
				return
			}
			s.childMu.Lock()
			if ctx.Err() != nil {
				s.childMu.Unlock()
				return
			}
			_, err := s.child.Write(append(append([]byte(nil), scanner.Bytes()...), '\n'))
			s.childMu.Unlock()
			if err != nil {
				s.stop()
				return
			}
		}
	}()
	cursor := hello.Cursor
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
