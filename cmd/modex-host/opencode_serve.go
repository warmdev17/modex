package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	opencode "github.com/warmdev/modex/internal/opencode"
)

// serveOpenCodeHost starts an isolated OpenCode server and serves the Modex
// read surface over stdio for the lifetime of this connection.
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
	client := opencode.NewClient(p.BaseURL(), p.Client(), cwd)
	call := func(requestCtx context.Context, method string, params any, emit func(string, json.RawMessage)) (json.RawMessage, error) {
		ctx, cancel := context.WithTimeout(requestCtx, 20*time.Second)
		defer cancel()
		return client.Call(ctx, method, params, emit)
	}
	return opencode.ServeEvents(ctx, in, out, cwd, call)
}
