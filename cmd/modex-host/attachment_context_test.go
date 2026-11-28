package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/warmdev/modex/internal/bridge"
	"github.com/warmdev/modex/internal/supervisor"
)

func TestAttachmentContextOnlyUsesInternalLifecycle(t *testing.T) {
	var current string
	var forwarded bridge.Command
	handler := attachmentContext(func(value string) { current = value }, func(_ context.Context, cmd bridge.Command, _ func(string, json.RawMessage)) (any, error) {
		forwarded = cmd
		return nil, nil
	})
	call := func(id int, method, params string) {
		t.Helper()
		if _, err := handler(context.Background(), bridge.Command{ID: id, Method: method, Params: json.RawMessage(params)}, nil); err != nil {
			t.Fatal(err)
		}
	}
	call(supervisor.AttachmentCommandID, "client/attached", `{"attached":true,"ssh_connection":"one"}`)
	if current != "one" || string(forwarded.Params) != `{"attached":true}` {
		t.Fatalf("context=%q forwarded=%s", current, forwarded.Params)
	}
	call(10, "client/attached", `{"attached":true,"ssh_connection":"forged"}`)
	if current != "one" {
		t.Fatal("external lifecycle changed context")
	}
	call(supervisor.AttachmentCommandID, "status", `{}`)
	if current != "one" {
		t.Fatal("other method changed context")
	}
	call(supervisor.AttachmentCommandID, "client/attached", `{"attached":false}`)
	if current != "" {
		t.Fatal("detach retained context")
	}
	call(supervisor.AttachmentCommandID, "client/attached", `{"attached":true,"ssh_connection":"one"}`)
	call(supervisor.AttachmentCommandID, "client/attached", `{"attached":true}`)
	if current != "" {
		t.Fatal("legacy attachment retained context")
	}
}

func TestPhoneSSHContextTakesPrecedenceOverCLI(t *testing.T) {
	var current string
	var lifecycle int
	handler := attachmentContext(func(value string) { current = value }, func(_ context.Context, _ bridge.Command, _ func(string, json.RawMessage)) (any, error) {
		lifecycle++
		return nil, nil
	})
	check := func(params, want string, forwards int) {
		t.Helper()
		cmd := bridge.Command{ID: supervisor.AttachmentCommandID, Method: "client/attached", Params: json.RawMessage(params)}
		if _, err := handler(context.Background(), cmd, nil); err != nil || current != want || lifecycle != forwards {
			t.Fatalf("context=%q, lifecycle=%d, err=%v", current, lifecycle, err)
		}
	}
	check(`{"cli_ssh":true,"ssh_connection":"terminal"}`, "terminal", 0)
	check(`{"attached":true,"ssh_connection":"phone"}`, "phone", 1)
	check(`{"cli_ssh":true,"ssh_connection":"terminal-2"}`, "phone", 1)
	check(`{"attached":false}`, "terminal-2", 2)
	check(`{"cli_ssh":true}`, "", 2)
}
