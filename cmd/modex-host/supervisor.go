package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
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

	"github.com/warmdev/modex/internal/supervisor"
)

func randomID() string {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic("cannot obtain secure randomness")
	}
	return hex.EncodeToString(data[:])
}

func superviseHost(cwd, binary, home, dir string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	release, err := claimSupervisorState(dir)
	if err != nil {
		return err
	}
	defer release()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return errors.New("cannot listen on loopback")
	}
	defer listener.Close()
	self, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"--serve", "--workspace", cwd, "--codex", binary}
	if home != "" {
		args = append(args, "--codex-home", home)
	}
	child := exec.Command(self, args...)
	child.Dir = cwd
	child.Stderr = io.Discard
	input, err := child.StdinPipe()
	if err != nil {
		return err
	}
	output, err := child.StdoutPipe()
	if err != nil {
		input.Close()
		return err
	}
	if err = child.Start(); err != nil {
		input.Close()
		output.Close()
		return errors.New("cannot start persistent bridge")
	}
	defer input.Close()
	done := make(chan struct{})
	defer func() {
		stop()
		_ = input.Close()
		_ = child.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = child.Process.Kill()
			<-done
		}
	}()
	endpoint := supervisorEndpoint{Address: listener.Addr().String(), Token: randomID(), Instance: randomID(), Workspace: cwd}
	server := supervisor.NewServer(ctx, endpoint.Token, endpoint.Instance, input, stop)
	server.OnAttachment = func(attached bool) error {
		return json.NewEncoder(input).Encode(map[string]any{"id": 2147483646, "method": "client/attached", "params": map[string]bool{"attached": attached}})
	}
	go func() {
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), supervisor.MaxFrame)
		for scanner.Scan() {
			var lifecycle struct {
				ID    int             `json:"id"`
				Error json.RawMessage `json:"error"`
			}
			if json.Unmarshal(scanner.Bytes(), &lifecycle) == nil && lifecycle.ID == 2147483646 {
				if len(lifecycle.Error) > 0 {
					break
				}
				continue
			}
			if server.Journal.Append(json.RawMessage(scanner.Bytes())) != nil {
				break
			}
		}
		stop()
		_ = child.Wait()
		close(done)
	}()
	// Validate bridge readiness before publishing the endpoint; do not expose startup stderr.
	if _, err = input.Write([]byte("{\"id\":2147483647,\"method\":\"status\"}\n")); err != nil {
		return errors.New("bridge startup failed")
	}
	readyCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	for {
		entries, wake, _ := server.Journal.After(0)
		if len(entries) > 0 {
			var response struct {
				ID     int             `json:"id"`
				Error  json.RawMessage `json:"error"`
				Result json.RawMessage `json:"result"`
			}
			ready := false
			for _, entry := range entries {
				if json.Unmarshal(entry.Message, &response) == nil && response.ID == 2147483647 {
					if len(response.Error) > 0 || len(response.Result) == 0 {
						return errors.New("bridge startup failed")
					}
					ready = true
					break
				}
			}
			if ready {
				break
			}
		}
		select {
		case <-readyCtx.Done():
			return errors.New("bridge startup failed or timed out")
		case <-wake:
		}
	}
	if err = writeEndpoint(dir, endpoint); err != nil {
		return err
	}
	var workers sync.WaitGroup
	go func() { <-ctx.Done(); _ = listener.Close() }()
	slots := make(chan struct{}, 16)
	for {
		conn, err := listener.Accept()
		if err != nil {
			break
		}
		select {
		case slots <- struct{}{}:
			workers.Add(1)
			go func() { defer workers.Done(); defer func() { <-slots }(); server.Handle(conn) }()
		default:
			conn.Close()
		}
	}
	stop()
	workers.Wait()
	return nil
}

func startBackground(cwd, binary, home, dir string) error {
	// Fail before spawning if the state is unsafe or already owned.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := checkPrivatePath(dir, true); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(dir, "owner.lock")); err == nil {
		return errors.New("supervisor already owns this state directory")
	}
	if _, err := os.Lstat(filepath.Join(dir, "endpoint.json")); !os.IsNotExist(err) {
		return errors.New("existing supervisor endpoint; recover stale state explicitly")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"--supervise", "--workspace", cwd, "--codex", binary, "--state-dir", dir}
	if home != "" {
		args = append(args, "--codex-home", home)
	}
	child := exec.Command(self, args...)
	child.Dir = cwd
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	child.Stdin = null
	child.Stdout = null
	child.Stderr = null
	detachSupervisor(child)
	if err = child.Start(); err != nil {
		return errors.New("cannot start detached supervisor")
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if endpoint, err := readEndpoint(dir); err == nil && endpoint.Workspace == cwd {
				fmt.Fprintln(os.Stdout, "supervisor ready")
				return nil
			}
		case <-done:
			return errors.New("supervisor exited during startup; check state ownership and Codex configuration")
		case <-deadline.C:
			_ = child.Process.Signal(os.Interrupt)
			select {
			case <-done:
			case <-time.After(7 * time.Second):
				_ = child.Process.Kill()
				<-done
			}
			return errors.New("supervisor startup timed out")
		}
	}
}

func supervisorClient(dir string, cursor uint64, instance string, stop bool) error {
	endpoint, err := readEndpoint(dir)
	if err != nil {
		return err
	}
	if cursor > 0 && instance == "" {
		return errors.New("--instance is required when resuming a nonzero cursor")
	}
	if instance == "" {
		instance = endpoint.Instance
	}
	conn, err := net.DialTimeout("tcp4", endpoint.Address, 5*time.Second)
	if err != nil {
		return errors.New("supervisor is unavailable; do not replay uncertain commands")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err = json.NewEncoder(conn).Encode(supervisor.Hello{Token: endpoint.Token, Instance: instance, Cursor: cursor, Stop: stop}); err != nil {
		return errors.New("supervisor handshake failed")
	}
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return errors.New("supervisor handshake failed")
	}
	var reply struct {
		Ready    bool   `json:"ready"`
		Stopping bool   `json:"stopping"`
		Instance string `json:"instance"`
		Error    string `json:"error"`
	}
	if json.Unmarshal(line, &reply) != nil {
		return errors.New("invalid supervisor reply")
	}
	if reply.Error != "" {
		return errors.New(reply.Error)
	}
	if stop {
		if !reply.Stopping {
			return errors.New("supervisor stop not acknowledged")
		}
		return nil
	}
	if !reply.Ready {
		return errors.New("supervisor attach not acknowledged")
	}
	if _, err = os.Stdout.Write(line); err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Time{})
	// EOF deliberately detaches; the supervisor keeps the bridge input open.
	copied := make(chan error, 1)
	go func() { _, err := io.Copy(conn, os.Stdin); copied <- err; _ = conn.Close() }()
	_, err = io.Copy(os.Stdout, reader)
	select {
	case copyErr := <-copied:
		return copyErr
	default:
	}
	if err != nil {
		return errors.New("supervisor connection closed; reconnect using last cursor, never replay commands")
	}
	return nil
}
