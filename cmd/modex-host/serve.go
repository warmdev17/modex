package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/warmdev/modex/internal/bridge"
)

func serveHost(cwd, binary, home string) error {
	return serveHostWithSocket(cwd, binary, home, "")
}

func serveHostWithSocket(cwd, binary, home, socket string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	env := os.Environ()
	if home != "" {
		absolute, err := filepath.Abs(home)
		if err != nil {
			return err
		}
		env = append(env, "CODEX_HOME="+absolute)
	}
	p, err := bridge.StartProcess(ctx, binary, []string{"app-server", "--listen", "stdio://"}, cwd, env)
	if err != nil {
		return err
	}
	defer p.Close()
	if socket != "" {
		if err := checkPrivatePath(filepath.Dir(socket), true); err != nil {
			return err
		}
		backend, err := newSharedBackend(ctx, p, nil)
		if err != nil {
			return err
		}
		backend.home = home
		closeGateway, err := backend.listen(socket)
		if err != nil {
			return err
		}
		defer closeGateway()
		return bridge.ServeEvents(ctx, os.Stdin, os.Stdout, cwd, backend.call)
	}
	return serveBridge(ctx, p, os.Stdin, os.Stdout, cwd, home)
}

func serveBridge(ctx context.Context, p *bridge.Process, in io.Reader, out io.Writer, cwd, home string) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	decisions := bridge.NewDecisions(func(ctx context.Context, id, result json.RawMessage) error {
		return p.Respond(ctx, id, result)
	})

	var emitMu sync.RWMutex
	var currentEmit func(string, json.RawMessage)
	p.SetRequestHandler(func(id json.RawMessage, method string, params json.RawMessage) {
		event, err := decisions.Add(id, method, params)
		if err != nil {
			if p.Reject(id) != nil {
				stop()
			}
			return
		}
		emitMu.RLock()
		emit := currentEmit
		emitMu.RUnlock()
		if emit != nil {
			if raw, encodeErr := json.Marshal(event); encodeErr == nil {
				emit("decision/required", raw)
			}
		}
	})
	initCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	err := p.Initialize(initCtx)
	cancel()
	if err != nil {
		return err
	}
	sessions := newThreadProcesses(ctx, p, func(method string, params json.RawMessage) {
		emitMu.RLock()
		emit := currentEmit
		emitMu.RUnlock()
		if emit != nil {
			emit(method, params)
		}
	})
	defer sessions.Close()
	call := func(ctx context.Context, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error) {
		if method == "client/takeover" {
			values, ok := params.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid takeover")
			}
			id, _ := values["threadId"].(string)
			return requestTakeover(ctx, home, id)
		}
		if method == "decision/respond" {
			p, ok := params.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid decision response")
			}
			id, ok := p["decisionId"].(string)
			result, resultOK := p["result"].(json.RawMessage)
			if !ok || !resultOK {
				return nil, fmt.Errorf("invalid decision response")
			}
			var answerErr error
			if strings.Contains(id, "/") {
				answerErr = sessions.Answer(ctx, id, result)
			} else {
				answerErr = decisions.Answer(ctx, id, result)
			}
			if err := answerErr; err != nil {
				return nil, err
			}
			return json.RawMessage(`{"delivered":true}`), nil
		}
		emitMu.Lock()
		currentEmit = emit
		emitMu.Unlock()
		requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		p.SetNotificationHandler(func(eventMethod string, eventParams json.RawMessage) {
			decisions.Resolve(eventMethod, eventParams)
			emit(eventMethod, eventParams)
		})
		return sessions.Call(requestCtx, method, params)
	}
	return bridge.ServeEvents(ctx, in, out, cwd, call)
}
