package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	opencode "github.com/warmdev/modex/internal/opencode"
)

// serveOpenCodeHost starts an isolated OpenCode server and serves the Modex
// read/turn surface over stdio for the lifetime of this connection.
func serveOpenCodeHost(cwd string, b backendSpec) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	p, err := opencode.StartProcess(ctx, b.opencode, cwd, os.Environ())
	if err != nil {
		return err
	}
	defer p.Close()
	return serveOpenCodeBridge(ctx, p, os.Stdin, os.Stdout, cwd)
}

func serveOpenCodeBridge(ctx context.Context, p *opencode.Process, in io.Reader, out io.Writer, cwd string) error {
	writer := opencode.NewWriter(out)
	synth := opencode.NewSynthesizer(cwd, writer.Emit)
	client := opencode.NewClient(p.BaseURL(), p.Client(), cwd)
	decisions := opencode.NewDecisions(cwd, writer.Emit, synth.TurnID, func(replyCtx context.Context, sessionID, requestID, decision string) error {
		return client.ReplyPermission(replyCtx, sessionID, requestID, decision)
	})
	synth.OnTurnComplete = decisions.Resolve
	decisions.OnUnsupportedInput = synth.UnsupportedInput

	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()
	stream := opencode.NewStream(client, synth, decisions.Handle)
	stream.OnPermissions = decisions.Reconcile
	go func() { _ = stream.Run(streamCtx) }()

	call := func(requestCtx context.Context, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error) {
		ctx, cancel := context.WithTimeout(requestCtx, 20*time.Second)
		defer cancel()
		if method == "decision/respond" {
			values, _ := params.(map[string]any)
			decisionID, _ := values["decisionID"].(string)
			result, _ := values["result"].(json.RawMessage)
			if err := decisions.Answer(ctx, decisionID, result); err != nil {
				return nil, err
			}
			return json.RawMessage(`{"delivered":true}`), nil
		}
		raw, err := openCodeTurnCall(ctx, client, synth, method, params, emit)
		if err == nil && method == "client/attached" {
			values, _ := params.(map[string]any)
			if attached, _ := values["attached"].(bool); attached {
				decisions.Replay(true)
			}
		}
		return raw, err
	}
	return opencode.ServeEventsWithWriter(ctx, in, cwd, call, writer, synth)
}

func openCodeTurnCall(ctx context.Context, client *opencode.Client, synth *opencode.Synthesizer, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error) {
	if method == "turn/interrupt" || method == "turn/steer" {
		values, _ := params.(map[string]any)
		sessionID, _ := values["sessionID"].(string)
		expected, _ := values["turnID"].(string)
		id, active := synth.ActiveTurn(sessionID)
		if !active || id != expected {
			return nil, errors.New("turn is no longer active")
		}
	}
	raw, err := client.Call(ctx, method, params, emit)
	if err != nil {
		return nil, err
	}
	values, _ := params.(map[string]any)
	sessionID, _ := values["sessionID"].(string)
	switch method {
	case "turn/start":
		var result struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if json.Unmarshal(raw, &result) == nil && result.Turn.ID != "" {
			dir, _ := values["directory"].(string)
			synth.StartTurn(sessionID, result.Turn.ID, dir)
		}
	case "turn/interrupt":
		var result struct {
			Interrupted bool `json:"interrupted"`
		}
		if json.Unmarshal(raw, &result) == nil && result.Interrupted {
			expected, _ := values["turnID"].(string)
			synth.MarkInterrupted(sessionID, expected)
		}
	}
	return raw, nil
}
