package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/warmdev/modex/internal/diagnostic"
	"github.com/warmdev/modex/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "modex-host:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(version.String("modex-host"))
		return nil
	}
	workspace := flag.String("workspace", "", "Existing workspace directory (required)")
	binary := flag.String("codex", "codex", "Codex executable")
	home := flag.String("codex-home", "", "Override CODEX_HOME; defaults to inherited environment")
	profile := flag.String("profile", "", "Named profile (not yet supported; fails explicitly)")
	cli := flag.Bool("cli", false, "Open a native Codex TUI on the shared supervisor backend (Linux/macOS)")
	nativeSocket := flag.String("native-socket", "", "Private native TUI gateway socket (internal --serve option)")
	thread := flag.String("thread", "", "Existing session UUID for --cli; omit to choose by title or create new")
	serve := flag.Bool("serve", false, "Serve Modex commands over stdin/stdout for the lifetime of this connection")
	background := flag.Bool("background", false, "Start a detached supervisor (Linux/macOS)")
	supervise := flag.Bool("supervise", false, "Run a persistent foreground supervisor (Linux/macOS)")
	attach := flag.Bool("attach", false, "Attach to a running supervisor using stdin/stdout")
	stop := flag.Bool("stop", false, "Stop the authenticated supervisor")
	stateDir := flag.String("state-dir", "", "Private supervisor state directory (required for supervisor modes)")
	cursor := flag.Uint64("cursor", 0, "Last observed supervisor sequence number")
	instance := flag.String("instance", "", "Supervisor instance from the attach acknowledgement")
	flag.Parse()
	modes := 0
	for _, enabled := range []bool{*serve, *background, *supervise, *attach, *stop, *cli} {
		if enabled {
			modes++
		}
	}
	if modes > 1 {
		return fmt.Errorf("choose only one host mode")
	}
	if *nativeSocket != "" && (!*serve || !filepath.IsAbs(*nativeSocket)) {
		return fmt.Errorf("--native-socket requires --serve and an absolute private path")
	}
	if *cli && *stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		*stateDir = filepath.Join(home, ".local", "state", "modex-phone")
	}
	if *background || *supervise || *attach || *stop || *cli {
		if *stateDir == "" {
			return fmt.Errorf("--state-dir is required for supervisor modes")
		}
		absolute, err := filepath.Abs(*stateDir)
		if err != nil {
			return err
		}
		*stateDir = absolute
	} else if *stateDir != "" || *cursor != 0 || *instance != "" {
		return fmt.Errorf("supervisor options require a supervisor mode")
	}
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *thread != "" && !*cli {
		return fmt.Errorf("--thread requires --cli")
	}
	if *profile != "" {
		return fmt.Errorf("named profiles are not supported by this diagnostic; no profile was applied")
	}
	if *attach || *stop {
		return supervisorClient(*stateDir, *cursor, *instance, *stop)
	}
	if *workspace == "" {
		if !*cli {
			return fmt.Errorf("--workspace is required")
		}
		*workspace = "."
	}
	cwd, err := filepath.Abs(*workspace)
	if err != nil {
		return err
	}
	stat, err := os.Stat(cwd)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return fmt.Errorf("workspace must be a directory")
	}
	if *home != "" {
		absolute, err := filepath.Abs(*home)
		if err != nil {
			return err
		}
		*home = absolute
	}
	if *cli {
		err := runSharedCLI(cwd, *binary, *home, *thread, *stateDir)
		if errors.Is(err, errCLICancelled) {
			return nil
		}
		return err
	}
	if *background {
		return startBackground(cwd, *binary, *home, *stateDir)
	}
	if *supervise {
		return superviseHost(cwd, *binary, *home, *stateDir)
	}
	if *serve {
		return serveHostWithSocket(cwd, *binary, *home, *nativeSocket)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, *binary, "app-server", "--listen", "stdio://")
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	if *home != "" {
		absolute, err := filepath.Abs(*home)
		if err != nil {
			return err
		}
		cmd.Env = append(cmd.Env, "CODEX_HOME="+absolute)
	}
	// Startup stderr may include credential-bearing URLs; never forward it by default.
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	defer func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	reader := bufio.NewReader(stdout)
	if _, err = diagnostic.Call(stdin, reader, 1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "modex_host", "version": "0.0.1"}}); err != nil {
		return err
	}
	if err = json.NewEncoder(stdin).Encode(map[string]string{"method": "initialized"}); err != nil {
		return err
	}
	raw, err := diagnostic.Call(stdin, reader, 2, "config/read", map[string]any{"cwd": cwd, "includeLayers": false})
	if err != nil {
		return err
	}
	summary, err := diagnostic.Summarize(raw)
	if err != nil {
		return err
	}
	raw, err = diagnostic.Call(stdin, reader, 3, "thread/list", map[string]any{"cwd": cwd, "limit": 20, "modelProviders": []string{}})
	if err != nil {
		return err
	}
	var page struct {
		Data []struct {
			ID       string `json:"id"`
			Cwd      string `json:"cwd"`
			Provider string `json:"modelProvider"`
		} `json:"data"`
		NextCursor *string `json:"nextCursor"`
	}
	if json.Unmarshal(raw, &page) != nil {
		return fmt.Errorf("invalid thread list response")
	}
	report := struct {
		Workspace string             `json:"workspace"`
		Config    diagnostic.Summary `json:"config"`
		Threads   any                `json:"threads"`
		HasMore   bool               `json:"has_more_threads"`
	}{cwd, summary, page.Data, page.NextCursor != nil}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}
