//go:build !windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/warmdev/modex/internal/bridge"
)

func runSharedCLI(cwd, binary, home, id, dir string) error {
	if id != "" && !validTakeoverThread(id) {
		return errors.New("--thread must be an exact Codex session UUID")
	}
	endpoint, err := readEndpoint(dir)
	if err != nil {
		return fmt.Errorf("supervisor unavailable in %s; start modex service or modex-host first", dir)
	}
	socket := endpoint.NativeSocket
	if socket == "" {
		return errors.New("running supervisor predates shared CLI support; stop it when idle and restart with the updated host")
	}
	if !filepath.IsAbs(socket) || checkPrivatePath(filepath.Dir(socket), true) != nil {
		return errors.New("unsafe native supervisor socket directory")
	}
	info, err := os.Lstat(socket)
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("native supervisor socket is unavailable or not private")
	}
	if id == "" {
		fmt.Fprintln(os.Stdout, "Modex · "+cliLabel(cwd))
		id, err = pickCLIThread(context.Background(), os.Stdin, os.Stdout, func(ctx context.Context, cursor string) (json.RawMessage, error) {
			ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			params, _ := json.Marshal(map[string]any{"limit": 20, "cursor": cursor})
			page, err := bridge.Handle(ctx, cwd, bridge.Command{Method: "threads/list", Params: params}, func(ctx context.Context, method string, params any) (json.RawMessage, error) {
				return callNativeSocket(ctx, socket, method, params)
			})
			if err != nil {
				return nil, err
			}
			return json.Marshal(page)
		})
		if err != nil {
			return err
		}
	}
	args := []string{"--remote", "unix://" + socket, "--cd", cwd}
	if id != "" {
		args = append(args, "resume", id)
	}
	cli := exec.Command(binary, args...)
	cli.Dir = cwd
	cli.Env = os.Environ()
	if home != "" {
		cli.Env = append(cli.Env, "CODEX_HOME="+home)
	}
	cli.Stdin, cli.Stdout, cli.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Ctrl-C belongs to the TUI. Terminating this launcher must only close its TUI.
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	if err = cli.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cli.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = cli.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = cli.Process.Kill()
			<-done
		}
		return errors.New("CLI disconnected; supervisor and running turns retained")
	}
}
