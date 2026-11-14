//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Codex --listen unix:// uses WebSocket framing, unlike the daemon's stdio proxy.
func loadedThreads(ctx context.Context, socket string) (json.RawMessage, error) {
	return callNativeSocket(ctx, socket, "thread/loaded/list", map[string]any{"limit": 100})
}

func callNativeSocket(ctx context.Context, socket, method string, params any) (json.RawMessage, error) {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	conn, _, err := websocket.Dial(ctx, "ws://localhost/", &websocket.DialOptions{HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		return nil, err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1024 * 1024)
	readReply := func(id int) (json.RawMessage, error) {
		for count := 0; count < 1024; count++ {
			var reply struct {
				ID     *int            `json:"id"`
				Method string          `json:"method"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if err := wsjson.Read(ctx, conn, &reply); err != nil {
				return nil, err
			}
			if reply.ID != nil && reply.Method != "" {
				return nil, errors.New("unexpected takeover observer request")
			}
			if reply.ID == nil || *reply.ID != id {
				continue
			}
			if len(reply.Error) > 0 && string(reply.Error) != "null" {
				return nil, errors.New("takeover observer request rejected")
			}
			if len(reply.Result) == 0 || string(reply.Result) == "null" {
				return nil, errors.New("missing takeover observer response")
			}
			return reply.Result, nil
		}
		return nil, errors.New("too many takeover observer notifications")
	}
	if err = wsjson.Write(ctx, conn, map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "modex_takeover", "version": "1"}, "capabilities": map[string]bool{"experimentalApi": true}}}); err != nil {
		return nil, err
	}
	if _, err = readReply(1); err != nil {
		return nil, err
	}
	if err = wsjson.Write(ctx, conn, map[string]any{"method": "initialized"}); err != nil {
		return nil, err
	}
	if err = wsjson.Write(ctx, conn, map[string]any{"id": 2, "method": method, "params": params}); err != nil {
		return nil, err
	}
	return readReply(2)
}
