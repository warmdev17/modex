//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/warmdev/modex/internal/diagnostic"
)

func takeoverError(code string) error {
	raw, _ := json.Marshal(map[string]string{"message": code})
	return diagnostic.NewRPCError(raw)
}

func contactLauncher(ctx context.Context, root, id string) (json.RawMessage, error) {
	if !validTakeoverThread(id) {
		return nil, takeoverError("takeover_invalid")
	}
	directory := filepath.Join(root, id)
	if checkPrivatePath(root, true) != nil || checkPrivatePath(directory, true) != nil {
		return nil, takeoverError("takeover_unmanaged")
	}
	socket := filepath.Join(directory, "owner.sock")
	info, err := os.Lstat(socket)
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm()&0077 != 0 {
		return nil, takeoverError("takeover_unmanaged")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, takeoverError("takeover_unmanaged")
	}
	defer conn.Close()
	deadline := time.Now().Add(12 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	if json.NewEncoder(conn).Encode(map[string]any{"thread_id": id, "confirm": true}) != nil {
		return nil, takeoverError("takeover_uncertain")
	}
	var reply struct {
		Released bool   `json:"released"`
		Error    string `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&reply) != nil {
		return nil, takeoverError("takeover_uncertain")
	}
	if !reply.Released {
		switch reply.Error {
		case "takeover_mismatch", "takeover_unavailable":
			return nil, takeoverError(reply.Error)
		}
		return nil, takeoverError("takeover_uncertain")
	}
	return json.RawMessage(`{"released":true}`), nil
}

type ownedCLI struct {
	released            bool
	mu                  sync.Mutex
	id                  string
	cli, server         *exec.Cmd
	cliDone, serverDone <-chan struct{}
	inspect             func(context.Context) (json.RawMessage, error)
}

func waitOwned(done <-chan struct{}, timeout time.Duration) bool {
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (owner *ownedCLI) release(ctx context.Context, id string, confirm bool) string {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if !confirm || id != owner.id {
		return "takeover_mismatch"
	}
	select {
	case <-owner.cliDone:
		return "takeover_unavailable"
	default:
	}
	// Pause only our child so its terminal cannot switch sessions during the ownership check.
	if owner.cli.Process.Signal(syscall.SIGSTOP) != nil {
		return "takeover_unavailable"
	}
	defer owner.cli.Process.Signal(syscall.SIGCONT)
	raw, err := owner.inspect(ctx)
	if err != nil || !singleLoadedThread(raw, id) {
		return "takeover_mismatch"
	}
	if owner.server.Process.Signal(syscall.SIGTERM) != nil || !waitOwned(owner.serverDone, 3*time.Second) {
		return "takeover_uncertain"
	}
	_ = owner.cli.Process.Signal(syscall.SIGCONT)
	_ = owner.cli.Process.Signal(os.Interrupt)
	if !waitOwned(owner.cliDone, 3*time.Second) {
		return "takeover_uncertain"
	}
	owner.released = true
	return ""
}

func (owner *ownedCLI) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(12 * time.Second))
	var request struct {
		ThreadID string `json:"thread_id"`
		Confirm  bool   `json:"confirm"`
	}
	decoder := json.NewDecoder(io.LimitReader(conn, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	code := owner.release(ctx, request.ThreadID, request.Confirm)
	_ = json.NewEncoder(conn).Encode(map[string]any{"released": code == "", "error": code})
}

func runCLI(cwd, binary, home, id string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)

	if id != "" && !validTakeoverThread(id) {
		return errors.New("--thread must be an exact Codex session UUID")
	}
	root, err := takeoverRoot(home)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return err
	}
	if err = checkPrivatePath(root, true); err != nil {
		return err
	}
	directory := filepath.Join(root, id)
	if id == "" {
		directory, err = os.MkdirTemp(root, "launch-")
	} else {
		err = os.Mkdir(directory, 0700)
	}
	if err != nil {
		return errors.New("CLI registration already exists; close its launcher first (stale registration requires manual inspection)")
	}
	defer os.RemoveAll(directory)
	socket := filepath.Join(directory, "codex.sock")
	env := os.Environ()
	if home != "" {
		env = append(env, "CODEX_HOME="+home)
	}
	server := exec.Command(binary, "app-server", "--listen", "unix://"+socket)
	server.Dir, server.Env, server.Stderr = cwd, env, io.Discard
	detachSupervisor(server)
	if err = server.Start(); err != nil {
		return err
	}
	serverDone := make(chan struct{})
	go func() { _ = server.Wait(); close(serverDone) }()
	defer func() {
		_ = server.Process.Signal(syscall.SIGTERM)
		if !waitOwned(serverDone, 3*time.Second) {
			_ = server.Process.Kill()
			<-serverDone
		}
	}()
	ready := false
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
		select {
		case <-serverDone:
			return errors.New("dedicated Codex server exited before startup")
		case <-ctx.Done():
			return errors.New("registered CLI launcher stopped")
		default:
		}
		if stat, e := os.Stat(socket); e == nil && stat.Mode()&os.ModeSocket != 0 {
			ready = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		return errors.New("dedicated Codex server did not create its socket")
	}
	args := []string{"--remote", "unix://" + socket}
	if id != "" {
		args = append(args, "resume", id)
	}
	cli := exec.Command(binary, args...)
	cli.Dir, cli.Env = cwd, env
	cli.Stdin, cli.Stdout, cli.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err = cli.Start(); err != nil {
		return err
	}
	cliDone := make(chan struct{})
	var cliErr error
	go func() { cliErr = cli.Wait(); close(cliDone) }()
	defer func() {
		select {
		case <-cliDone:
			return
		default:
		}
		_ = server.Process.Signal(syscall.SIGTERM)
		_ = cli.Process.Signal(os.Interrupt)
		if !waitOwned(cliDone, 3*time.Second) {
			_ = cli.Process.Kill()
			<-cliDone
		}
	}()
	registration := directory
	if id == "" {
		id, err = waitCLIThread(ctx, cliDone, func(ctx context.Context) (json.RawMessage, error) { return loadedThreads(ctx, socket) })
		if err != nil {
			return err
		}
		if id == "" {
			return cliErr
		}
		registration = filepath.Join(root, id)
		if err = os.Mkdir(registration, 0700); err != nil {
			return errors.New("new session registration already exists; inspect it before retrying")
		}
		defer os.RemoveAll(registration)
	}
	listener, err := net.Listen("unix", filepath.Join(registration, "owner.sock"))
	if err != nil {
		return err
	}
	defer listener.Close()
	if err = os.Chmod(filepath.Join(registration, "owner.sock"), 0600); err != nil {
		return err
	}
	owner := &ownedCLI{id: id, cli: cli, server: server, cliDone: cliDone, serverDone: serverDone}
	owner.inspect = func(ctx context.Context) (json.RawMessage, error) { return loadedThreads(ctx, socket) }

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			owner.handle(conn)
		}
	}()
	select {
	case <-cliDone:
	case <-ctx.Done():
		_ = server.Process.Signal(syscall.SIGTERM)
		_ = cli.Process.Signal(os.Interrupt)
		if !waitOwned(cliDone, 3*time.Second) {
			_ = cli.Process.Kill()
			<-cliDone
		}
	}
	_ = listener.Close()
	<-handlerDone // Complete a pending release acknowledgement before removing registration.
	if owner.released {
		return nil
	}
	if ctx.Err() != nil {
		return errors.New("registered CLI launcher stopped")
	}
	if cliErr != nil {
		return fmt.Errorf("registered Codex CLI exited: %w", cliErr)
	}
	return nil
}

// A native new conversation gets its ID from Codex, never from a synthetic seed or replay.
func waitCLIThread(ctx context.Context, done <-chan struct{}, inspect func(context.Context) (json.RawMessage, error)) (string, error) {
	for {
		select {
		case <-done:
			return "", nil
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		raw, err := inspect(probeCtx)
		cancel()
		if err != nil {
			return "", errors.New("cannot discover the new native session; no takeover was registered")
		}
		var page struct {
			Data []string `json:"data"`
			Next *string  `json:"nextCursor"`
		}
		if json.Unmarshal(raw, &page) != nil || page.Data == nil || page.Next != nil || len(page.Data) > 1 {
			return "", errors.New("ambiguous native session; no takeover was registered")
		}
		if len(page.Data) == 1 {
			if !validTakeoverThread(page.Data[0]) {
				return "", errors.New("invalid native session ID")
			}
			return page.Data[0], nil
		}
		select {
		case <-done:
			return "", nil
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}
