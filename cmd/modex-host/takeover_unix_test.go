//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestOwnedCLIChild(t *testing.T) {
	if os.Getenv("MODEX_CLI_TEST_CHILD") != "1" {
		return
	}
	for {
		time.Sleep(time.Hour)
	}
}

func startTakeoverChild(t *testing.T) (*exec.Cmd, <-chan struct{}) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnedCLIChild$")
	cmd.Env = append(os.Environ(), "MODEX_CLI_TEST_CHILD=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })
	return cmd, done
}

func TestOwnedCLITakeoverRefusalAndExit(t *testing.T) {
	cli, cliDone := startTakeoverChild(t)
	server, serverDone := startTakeoverChild(t)
	inspected := false
	owner := ownedCLI{id: "target", cli: cli, server: server, cliDone: cliDone, serverDone: serverDone, inspect: func(context.Context) (json.RawMessage, error) {
		inspected = true
		return json.RawMessage(`{"data":["target"]}`), nil
	}}
	if code := owner.release(context.Background(), "target", false); code != "takeover_mismatch" || inspected {
		t.Fatal("unconfirmed release")
	}
	if code := owner.release(context.Background(), "other", true); code != "takeover_mismatch" || inspected {
		t.Fatal("wrong target release")
	}
	owner.inspect = func(context.Context) (json.RawMessage, error) {
		return json.RawMessage(`{"data":["target","other"]}`), nil
	}
	if code := owner.release(context.Background(), "target", true); code != "takeover_mismatch" {
		t.Fatal("multi-thread release")
	}
	select {
	case <-cliDone:
		t.Fatal("CLI died after refusal")
	default:
	}
	select {
	case <-serverDone:
		t.Fatal("backend died after refusal")
	default:
	}
	owner.inspect = func(context.Context) (json.RawMessage, error) { return json.RawMessage(`{"data":["target"]}`), nil }
	if code := owner.release(context.Background(), "target", true); code != "" {
		t.Fatalf("release failed: %s", code)
	}
	select {
	case <-serverDone:
	default:
		t.Fatal("ACK before backend exited")
	}
	select {
	case <-cliDone:
	default:
		t.Fatal("ACK before CLI exited")
	}
}

func TestLauncherSocketValidatesBeforeSignalling(t *testing.T) {
	cli, cliDone := startTakeoverChild(t)
	server, serverDone := startTakeoverChild(t)
	id := "01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6"
	root, err := os.MkdirTemp("/tmp", "modex-own-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	dir := filepath.Join(root, id)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "owner.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err = os.Chmod(socket, 0600); err != nil {
		t.Fatal(err)
	}
	owner := ownedCLI{id: id, cli: cli, server: server, cliDone: cliDone, serverDone: serverDone, inspect: func(context.Context) (json.RawMessage, error) { return json.RawMessage(`{"data":["` + id + `"]}`), nil }}
	handled := make(chan struct{})
	go func() {
		defer close(handled)
		conn, err := listener.Accept()
		if err == nil {
			owner.handle(conn)
		}
	}()
	raw, err := contactLauncher(context.Background(), root, id)
	if err != nil || string(raw) != `{"released":true}` {
		t.Fatalf("socket release: %s %v", raw, err)
	}
	<-handled
}

func TestRefusedTakeoverResumesOwnedCLI(t *testing.T) {
	cli, cliDone := startTakeoverChild(t)
	server, serverDone := startTakeoverChild(t)
	owner := ownedCLI{id: "target", cli: cli, server: server, cliDone: cliDone, serverDone: serverDone, inspect: func(context.Context) (json.RawMessage, error) { return json.RawMessage(`{"data":["other"]}`), nil }}
	if owner.release(context.Background(), "target", true) != "takeover_mismatch" {
		t.Fatal("changed session accepted")
	}
	if err := cli.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if !waitOwned(cliDone, time.Second) {
		t.Fatal("refused takeover left the CLI frozen")
	}
	select {
	case <-serverDone:
		t.Fatal("refusal stopped backend")
	default:
	}
}
