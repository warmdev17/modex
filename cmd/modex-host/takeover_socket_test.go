//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestLoadedThreadsUsesWebSocketUnixTransport(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "mx-ws-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	socket := filepath.Join(root, "server.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var request map[string]any
		if wsjson.Read(ctx, conn, &request) != nil || request["method"] != "initialize" {
			return
		}
		if wsjson.Write(ctx, conn, map[string]any{"id": request["id"], "result": map[string]any{}}) != nil {
			return
		}
		if wsjson.Read(ctx, conn, &request) != nil || request["method"] != "initialized" {
			return
		}
		if wsjson.Read(ctx, conn, &request) != nil || request["method"] != "thread/loaded/list" {
			return
		}
		_ = wsjson.Write(ctx, conn, map[string]any{"method": "thread/status/changed", "params": map[string]any{}})
		_ = wsjson.Write(ctx, conn, map[string]any{"id": request["id"], "result": json.RawMessage(`{"data":["target"]}`)})
	})}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := loadedThreads(ctx, socket)
	if err != nil || !singleLoadedThread(raw, "target") {
		t.Fatalf("Unix WebSocket inspection: %s %v", raw, err)
	}
}
