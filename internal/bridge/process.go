package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/warmdev/modex/internal/diagnostic"
)

const maxUpstreamFrame = 16 * 1024 * 1024

// Process owns one app-server. A failed/timed-out exchange closes it rather than
// retrying an operation whose outcome may be unknown.
type Process struct {
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	reader         *bufio.Reader
	mu             sync.Mutex
	writeMu        sync.Mutex
	once           sync.Once
	closed         bool
	id             int
	onNotification func(string, json.RawMessage)
	onRequest      func(json.RawMessage, string, json.RawMessage)
	pending        map[int]chan processResponse
}

type processResponse struct {
	raw json.RawMessage
	err error
}

func (p *Process) SetNotificationHandler(handler func(string, json.RawMessage)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onNotification = handler
}

func (p *Process) SetRequestHandler(handler func(json.RawMessage, string, json.RawMessage)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onRequest = handler
}

func (p *Process) Respond(ctx context.Context, id, result json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return json.NewEncoder(p.stdin).Encode(map[string]any{"id": id, "result": result})
}

func (p *Process) Reject(id json.RawMessage) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return json.NewEncoder(p.stdin).Encode(map[string]any{"id": id, "error": map[string]any{"code": -32601, "message": "unsupported or invalid Modex decision request"}})
}

func StartProcess(ctx context.Context, binary string, args []string, cwd string, env []string) (*Process, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = cwd
	cmd.Env = env
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.New("cannot create Codex input pipe")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, errors.New("cannot create Codex output pipe")
	}
	if err = cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, errors.New("cannot start Codex executable")
	}
	p := &Process{cmd: cmd, stdin: stdin, reader: bufio.NewReader(stdout), pending: map[int]chan processResponse{}}
	go p.readLoop()
	return p, nil
}

// Fork starts an independent app-server with the exact same executable, workspace and environment.
func (p *Process) Fork(ctx context.Context) (*Process, error) {
	return StartProcess(ctx, p.cmd.Path, p.cmd.Args[1:], p.cmd.Dir, p.cmd.Environ())
}

func (p *Process) Close() { p.close(false) }

// CloseIdle lets Codex flush persisted history and stop its tools before reaping it.
func (p *Process) CloseIdle() { p.close(true) }

func (p *Process) close(idle bool) {
	p.once.Do(func() {
		p.mu.Lock()
		p.closed = true
		for id, ch := range p.pending {
			delete(p.pending, id)
			ch <- processResponse{err: errors.New("Codex connection closed")}
		}
		p.mu.Unlock()
		_ = p.stdin.Close()
		if !idle {
			_ = p.cmd.Process.Kill()
			_ = p.cmd.Wait()
			return
		}
		done := make(chan struct{})
		go func() { _ = p.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = p.cmd.Process.Kill()
			<-done
		}
	})
}

func (p *Process) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return p.CallEvents(ctx, method, params, nil)
}

func (p *Process) CallEvents(ctx context.Context, method string, params any, onNotification func(string, json.RawMessage)) (json.RawMessage, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errors.New("Codex connection closed")
	}
	p.id++
	id := p.id
	done := make(chan processResponse, 1)
	p.pending[id] = done
	if onNotification != nil {
		p.onNotification = onNotification
	}
	p.mu.Unlock()
	p.writeMu.Lock()
	err := json.NewEncoder(p.stdin).Encode(map[string]any{"id": id, "method": method, "params": params})
	p.writeMu.Unlock()
	if err != nil {
		p.removePending(id)
		p.Close()
		return nil, errors.New("cannot write Codex request")
	}
	select {
	case result := <-done:
		p.removePending(id)
		if result.err == nil && method == "thread/list" {
			result.raw = p.desktopMembership(result.raw)
		}
		return result.raw, result.err
	case <-ctx.Done():
		p.removePending(id)
		p.Close()
		return nil, errors.New("Codex request timed out or canceled; connection closed")
	}
}

func (p *Process) removePending(id int) {
	p.mu.Lock()
	delete(p.pending, id)
	p.mu.Unlock()
}

func (p *Process) readLoop() {
	defer p.Close()
	for {
		line, err := readFrame(p.reader)
		if err != nil {
			p.mu.Lock()
			if !p.closed {
				p.closed = true
				for id, ch := range p.pending {
					delete(p.pending, id)
					ch <- processResponse{err: errors.New("Codex connection closed")}
				}
			}
			p.mu.Unlock()
			return
		}
		var message struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(line, &message) != nil {
			p.failPending(errors.New("invalid app-server JSON"))
			return
		}
		if len(message.ID) == 0 || string(message.ID) == "null" {
			p.mu.Lock()
			handler := p.onNotification
			p.mu.Unlock()
			if message.Method != "" && handler != nil {
				handler(message.Method, message.Params)
			}
			continue
		}
		if message.Method != "" {
			p.mu.Lock()
			handler := p.onRequest
			p.mu.Unlock()
			if handler == nil {
				p.failPending(errors.New("unsupported Codex server request"))
				return
			}
			handler(message.ID, message.Method, message.Params)
			continue
		}
		var id int
		if json.Unmarshal(message.ID, &id) != nil {
			p.failPending(errors.New("invalid Codex response ID"))
			return
		}
		p.mu.Lock()
		ch := p.pending[id]
		delete(p.pending, id)
		p.mu.Unlock()
		if ch == nil {
			continue
		}
		if len(message.Error) > 0 && string(message.Error) != "null" {
			ch <- processResponse{err: diagnostic.NewRPCError(message.Error)}
			continue
		}
		if len(message.Result) == 0 {
			ch <- processResponse{err: errors.New("app-server response has no result")}
			continue
		}
		ch <- processResponse{raw: message.Result}
	}
}

func (p *Process) failPending(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for id, ch := range p.pending {
		delete(p.pending, id)
		ch <- processResponse{err: err}
	}
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, prefix, err := reader.ReadLine()
		if err != nil {
			return nil, err
		}
		if len(line)+len(chunk) > maxUpstreamFrame {
			return nil, errors.New("app-server frame exceeds 16 MiB limit")
		}
		line = append(line, chunk...)
		if !prefix {
			return line, nil
		}
	}
}

func (p *Process) Initialize(ctx context.Context) error {
	_, err := p.Call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "modex_host", "version": "0.0.2"}, "capabilities": map[string]bool{"experimentalApi": true}})
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return json.NewEncoder(p.stdin).Encode(map[string]string{"method": "initialized"})
}
