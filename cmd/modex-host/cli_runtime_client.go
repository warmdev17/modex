package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/warmdev/modex/internal/bridge"
	"github.com/warmdev/modex/internal/supervisor"
)

type localCLIEvent struct {
	Event  string          `json:"event"`
	Params json.RawMessage `json:"params"`
	seq    uint64
}
type localCLIReply struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
	Event  string          `json:"event"`
	Params json.RawMessage `json:"params"`
	seq    uint64
}
type localCLIClient struct {
	conn       net.Conn
	writeMu    sync.Mutex
	mu         sync.Mutex
	next       uint64
	eventSeq   uint64
	historySeq uint64
	pending    map[uint64]chan localCLIReply
	events     chan localCLIEvent
	watching   bool
	decisions  map[string]localCLIEvent
	done       chan struct{}
	closeOnce  sync.Once
}

func dialLocalCLI(ctx context.Context, endpoint supervisorEndpoint) (*localCLIClient, error) {
	host, _, err := net.SplitHostPort(endpoint.Address)
	if err != nil || host != "127.0.0.1" || endpoint.Token == "" || endpoint.Instance == "" {
		return nil, errors.New("invalid supervisor endpoint")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp4", endpoint.Address)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			conn.Close()
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	if d, yes := ctx.Deadline(); yes && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if err = json.NewEncoder(conn).Encode(map[string]any{"token": endpoint.Token, "instance": endpoint.Instance, "cli": true, "ssh_connection": os.Getenv("SSH_CONNECTION")}); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), bridge.MaxResponseFrame)
	if !scanner.Scan() {
		return nil, errors.New("supervisor handshake failed")
	}
	var ready struct {
		Ready    bool   `json:"ready"`
		Instance string `json:"instance"`
	}
	if json.Unmarshal(scanner.Bytes(), &ready) != nil || !ready.Ready || ready.Instance != endpoint.Instance {
		return nil, errors.New("supervisor handshake rejected")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	c := &localCLIClient{conn: conn, pending: make(map[uint64]chan localCLIReply), events: make(chan localCLIEvent, 256), decisions: make(map[string]localCLIEvent), done: make(chan struct{})}
	ok = true
	go c.read(scanner)
	return c, nil
}

func (c *localCLIClient) read(scanner *bufio.Scanner) {
	defer c.Close()
	for scanner.Scan() {
		var m localCLIReply
		if json.Unmarshal(scanner.Bytes(), &m) != nil {
			return
		}
		if m.Event != "" {
			c.mu.Lock()
			c.eventSeq++
			m.seq = c.eventSeq
			if !c.watching {
				var p struct {
					ID string `json:"decision_id"`
				}
				if m.Event == "decision/required" || m.Event == "decision/resolved" {
					_ = json.Unmarshal(m.Params, &p)
				}
				if m.Event == "decision/reset" {
					clear(c.decisions)
				}
				if m.Event == "decision/resolved" {
					delete(c.decisions, p.ID)
				}
				if m.Event == "decision/required" && p.ID != "" {
					if len(c.decisions) >= 128 {
						c.mu.Unlock()
						return
					}
					c.decisions[p.ID] = localCLIEvent{m.Event, m.Params, m.seq}
				}
				c.mu.Unlock()
				continue
			}
			select {
			case c.events <- localCLIEvent{m.Event, m.Params, m.seq}:
				c.mu.Unlock()
			default:
				c.mu.Unlock()
				return
			}
			continue
		}
		c.mu.Lock()
		m.seq = c.eventSeq
		ch := c.pending[m.ID]
		delete(c.pending, m.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- m
		}
	}
}
func (c *localCLIClient) watchEvents() {
	c.mu.Lock()
	for _, event := range c.decisions {
		c.events <- event
	}
	clear(c.decisions)
	c.watching = true
	c.mu.Unlock()
}
func (c *localCLIClient) Close() error {
	c.closeOnce.Do(func() { close(c.done); _ = c.conn.Close() })
	return nil
}
func (c *localCLIClient) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.next++
	id := c.next
	ch := make(chan localCLIReply, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	payload, err := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if len(payload) >= supervisor.MaxFrame {
		return nil, errors.New("request exceeds frame limit")
	}
	c.writeMu.Lock()
	if err = ctx.Err(); err == nil {
		deadline := time.Now().Add(5 * time.Second)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		_ = c.conn.SetWriteDeadline(deadline)
		stop := context.AfterFunc(ctx, func() { _ = c.conn.SetWriteDeadline(time.Now()) })
		_, err = c.conn.Write(append(payload, '\n'))
		if !stop() {
			c.Close()
		} else {
			_ = c.conn.SetWriteDeadline(time.Time{})
		}
	}
	c.writeMu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, errors.New("supervisor disconnected")
	case reply := <-ch:
		if method == "threads/read" {
			c.mu.Lock()
			c.historySeq = reply.seq
			c.mu.Unlock()
		}
		if len(reply.Error) > 0 && string(reply.Error) != "null" {
			var e struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(reply.Error, &e) == nil && e.Message != "" {
				return nil, fmt.Errorf("%s", e.Message)
			}
			return nil, errors.New("supervisor request failed")
		}
		return reply.Result, nil
	}
}

func (c *localCLIClient) historySequence() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.historySeq
}
