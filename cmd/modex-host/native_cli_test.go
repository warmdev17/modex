//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warmdev/modex/internal/bridge"
)

func TestNativeTerminalDescriptorRejectsPhoneRequests(t *testing.T) {
	handler := agentCommands("/work", nil, &fixtureAgentRouter{})
	for _, method := range []string{"cli/native", "cli/native/end"} {
		_, err := handler(context.Background(), bridge.Command{ID: 1, Method: method, Params: json.RawMessage(`{"thread_id":"agy:session","workspace":"/work"}`)}, nil)
		if err == nil || !strings.Contains(err.Error(), "requires the local CLI") {
			t.Fatalf("public %s accepted: %v", method, err)
		}
	}
}

func TestNativeAgentLaunchUsesRealBinaryAndPersistentAuth(t *testing.T) {
	for _, agent := range []string{"antigravity", "opencode"} {
		t.Run(agent, func(t *testing.T) {
			root := t.TempDir()
			binary := filepath.Join(root, "native")
			capture := filepath.Join(root, "capture")
			script := "#!/bin/sh\nprintf '%s\\n' \"$AGY_CLI_FORCE_FILE_TOKEN_STORAGE\" \"$OPENCODE_PASSWORD\" \"$@\" > '" + strings.ReplaceAll(capture, "'", "'\\''") + "'\n"
			if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			released := make(chan struct{})
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				decoder, encoder := json.NewDecoder(conn), json.NewEncoder(conn)
				var hello any
				_ = decoder.Decode(&hello)
				_ = encoder.Encode(map[string]any{"ready": true, "instance": "fixture"})
				for {
					var request struct {
						ID     int    `json:"id"`
						Method string `json:"method"`
					}
					if decoder.Decode(&request) != nil {
						return
					}
					var result any = map[string]string{"binary": binary, "workspace": root, "session_id": "native-session", "server": "http://127.0.0.1:12345", "password": "fixture-password"}
					if request.Method == "cli/native/end" {
						result = map[string]bool{"released": true}
					}
					_ = encoder.Encode(map[string]any{"id": request.ID, "result": result})
					if request.Method == "cli/native/end" {
						close(released)
					}
				}
			}()
			client, err := dialLocalCLI(context.Background(), supervisorEndpoint{Address: listener.Addr().String(), Token: "fixture", Instance: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if err := runNativeAgentCLI(context.Background(), client, agent, cliChoice{ID: "agy:registered", Workspace: root}); err != nil {
				t.Fatal(err)
			}
			<-released
			raw, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			text := string(raw)
			if agent == "antigravity" && !strings.Contains(text, "1\n") || agent == "antigravity" && !strings.Contains(text, "--conversation\nnative-session\n") {
				t.Fatalf("AGY launch: %q", text)
			}
			if agent == "opencode" && (!strings.Contains(text, "fixture-password\n") || !strings.Contains(text, "--server\nhttp://127.0.0.1:12345\n--session\nnative-session\n")) {
				t.Fatalf("OpenCode launch: %q", text)
			}
		})
	}
}
