//go:build !windows

package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedCLILaunchesOnlyTUIOnPublishedBackend(t *testing.T) {
	root, err := os.MkdirTemp("", "modex-cli-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	state := filepath.Join(root, "state")
	if err = os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(root, "native.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err = os.Chmod(socket, 0600); err != nil {
		t.Fatal(err)
	}
	if err = writeEndpoint(state, supervisorEndpoint{Address: "127.0.0.1:1234", Token: "fixture", Instance: "fixture", NativeSocket: socket}); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(root, "codex")
	output := filepath.Join(root, "args")
	if err = os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > '"+output+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	id := "01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6"
	if err = runSharedCLI(root, fake, "", id, state); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "--remote\nunix://"+socket+"\n--cd\n"+root+"\nresume\n"+id+"\n" {
		t.Fatalf("args=%s", args)
	}
	if _, err = os.Stat(socket); err != nil {
		t.Fatal("CLI exit removed backend socket")
	}
	if _, err = readEndpoint(state); err != nil {
		t.Fatal("CLI exit removed supervisor endpoint")
	}
}

func TestSharedCLIRefusesOlderSupervisorWithoutLaunchingAnotherBackend(t *testing.T) {
	state := t.TempDir()
	if err := os.Chmod(state, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeEndpoint(state, supervisorEndpoint{Address: "127.0.0.1:1234", Token: "fixture", Instance: "fixture"}); err != nil {
		t.Fatal(err)
	}
	err := runSharedCLI(state, "missing-codex", "", "01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6", state)
	if err == nil || !strings.Contains(err.Error(), "predates shared CLI") {
		t.Fatalf("err=%v", err)
	}
}
