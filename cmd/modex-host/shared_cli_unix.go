//go:build !windows

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/warmdev/modex/internal/bridge"
)

func runSharedCLI(cwd, binary, home, id, dir string) error {
	endpoint, err := readEndpoint(dir)
	if err != nil {
		return fmt.Errorf("supervisor unavailable in %s; start modex service or modex-host first", dir)
	}
	if validTakeoverThread(id) || !endpoint.CLIRelay && id == "" {
		return runNativeCodex(cwd, binary, home, id, endpoint)
	}
	if !endpoint.CLIRelay {
		return errors.New("the running supervisor needs an update to open Antigravity or OpenCode in the CLI")
	}
	fullscreen := interactiveCLI(os.Stdin, os.Stdout)
	ctx := context.Background()
	stop := func() {}
	if fullscreen {
		ctx, stop = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	}
	defer stop()
	client, err := dialLocalCLI(ctx, endpoint)
	if err != nil {
		return err
	}
	defer client.Close()
	input := bufio.NewReader(os.Stdin)
	openChat := func(agent cliAgent, choice cliChoice) error {
		if fullscreen {
			return runCLIChatFullscreen(ctx, os.Stdin, os.Stdout, client, agent.ID, agent.Label, choice.Workspace, choice.ID)
		}
		chatCtx, stopChat := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		defer stopChat()
		return runAgentCLIChat(chatCtx, input, os.Stdout, client, agent.ID, agent.Label, choice.Workspace, choice.ID)
	}
	if id != "" {
		var agent cliAgent
		switch {
		case strings.HasPrefix(id, "agy:"):
			agent = cliAgent{ID: "antigravity", Label: "Antigravity"}
		case strings.HasPrefix(id, "oc:"):
			agent = cliAgent{ID: "opencode", Label: "OpenCode"}
		default:
			return errors.New("--thread must identify a Codex, OpenCode or Antigravity session")
		}
		return openChat(agent, cliChoice{ID: id, Workspace: cwd})
	}
	loadAgents := func(ctx context.Context) ([]cliAgent, error) {
		discover, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		raw, err := client.Call(discover, "agents/list", map[string]string{"workspace": cwd})
		if err != nil {
			return nil, err
		}
		var catalogue struct {
			Data []cliAgent `json:"data"`
		}
		if json.Unmarshal(raw, &catalogue) != nil || len(catalogue.Data) == 0 {
			return nil, errors.New("the host returned no agent runtimes")
		}
		return catalogue.Data, nil
	}
	list := func(ctx context.Context, agent cliAgent, cursor string) (json.RawMessage, error) {
		ctx, done := context.WithTimeout(ctx, 20*time.Second)
		defer done()
		params := map[string]any{"agent": agent.ID, "limit": 20, "all_workspaces": true}
		if cursor != "" {
			params["cursor"] = cursor
		}
		return client.Call(ctx, "threads/list", params)
	}
	launch := func(agent cliAgent, choice cliChoice) error {
		if agent.ID == "codex" {
			client.Close()
			stop()
			return runNativeCodex(choice.Workspace, binary, home, choice.ID, endpoint)
		}
		return openChat(agent, choice)
	}
	if fullscreen {
		agent, choice, err := pickCLIFullscreen(ctx, os.Stdin, os.Stdout, loadAgents, cwd, list)
		if err != nil {
			return err
		}
		return launch(agent, choice)
	}
	fmt.Fprintln(os.Stdout, "Discovering agent runtimes…")
	agents, err := loadAgents(ctx)
	if err != nil {
		return err
	}
	picker := newCLIPicker(input, os.Stdout)
	for {
		agent, err := picker.chooseAgent(agents)
		if err != nil {
			return err
		}
		choice, err := picker.chooseSession(ctx, agent, cwd, func(ctx context.Context, cursor string) (json.RawMessage, error) {
			return list(ctx, agent, cursor)
		})
		if errors.Is(err, errCLIPickerBack) {
			continue
		}
		if err != nil {
			return err
		}
		return launch(agent, choice)
	}
}

func runNativeCodex(cwd, binary, home, id string, endpoint supervisorEndpoint) error {
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
	if id == "" && !endpoint.CLIRelay {
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
