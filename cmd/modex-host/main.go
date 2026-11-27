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

	"github.com/warmdev/modex/internal/acp"
	"github.com/warmdev/modex/internal/diagnostic"
	"github.com/warmdev/modex/internal/version"
)

// backendSpec selects which agent backend the host runs. Codex keeps the
// existing stdio app-server path; OpenCode uses an isolated `opencode serve`
// child with an HTTP/SSE surface added by later milestones.
type backendSpec struct {
	kind     string // "codex" | "opencode"
	codex    string
	opencode string
	home     string // codex-home; codex only
}

// backendFlags captures the flags that decide whether an opencode backend is
// compatible with the selected host mode.
type backendFlags struct {
	backend      string
	nativeSocket string
	home         string
	thread       string
	cli          bool
	serve        bool
	background   bool
	supervise    bool
	attach       bool
	stop         bool
}

// validateBackendFlags rejects opencode only for modes that spawn or open a
// backend. attach/stop are backend-agnostic and must ignore --backend entirely.
func validateBackendFlags(f backendFlags) error {
	if f.backend != "opencode" || f.attach || f.stop {
		return nil
	}
	switch {
	case f.cli:
		return fmt.Errorf("open code backend does not support --cli")
	case f.nativeSocket != "":
		return fmt.Errorf("open code backend does not support --native-socket")
	case f.home != "":
		return fmt.Errorf("open code backend does not support --codex-home")
	case f.thread != "":
		return fmt.Errorf("open code backend does not support --thread")
	case !f.serve && !f.background && !f.supervise:
		return fmt.Errorf("open code backend requires --serve, --background, or --supervise")
	}
	return nil
}

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
	backend := flag.String("backend", "codex", "Agent backend: codex or opencode")
	opencodeBinary := flag.String("opencode", "opencode", "OpenCode executable (only with --backend opencode)")
	probeAntigravity := flag.Bool("probe-antigravity", false, "Probe Antigravity ACP compatibility without running a turn")
	agy := flag.String("agy", "agy", "Antigravity CLI executable for --probe-antigravity")
	home := flag.String("codex-home", "", "Override CODEX_HOME; defaults to inherited environment")
	profile := flag.String("profile", "", "Named profile (not yet supported; fails explicitly)")
	cli := flag.Bool("cli", false, "Open a native Codex TUI on the shared supervisor backend (Linux/macOS)")
	nativeSocket := flag.String("native-socket", "", "Private native TUI gateway socket (internal --serve option)")
	thread := flag.String("thread", "", "Existing session UUID for --cli; omit to choose by title or create new")
	serve := flag.Bool("serve", false, "Serve Modex commands over stdin/stdout for the lifetime of this connection")
	background := flag.Bool("background", false, "Start a detached supervisor (Linux/macOS)")
	supervise := flag.Bool("supervise", false, "Run a persistent foreground supervisor (Linux/macOS)")
	attach := flag.Bool("attach", false, "Attach to a running supervisor using stdin/stdout")
	heartbeat := flag.Bool("heartbeat", false, "Require attach heartbeats to release stale clients")
	stop := flag.Bool("stop", false, "Stop the authenticated supervisor")
	stateDir := flag.String("state-dir", "", "Private supervisor state directory (required for supervisor modes)")
	cursor := flag.Uint64("cursor", 0, "Last observed supervisor sequence number")
	instance := flag.String("instance", "", "Supervisor instance from the attach acknowledgement")
	flag.Parse()
	modes := 0
	for _, enabled := range []bool{*serve, *background, *supervise, *attach, *stop, *cli, *probeAntigravity} {
		if enabled {
			modes++
		}
	}
	if modes > 1 {
		return fmt.Errorf("choose only one host mode")
	}
	if *backend != "codex" && *backend != "opencode" {
		return fmt.Errorf("--backend must be codex or opencode")
	}
	codexExplicit, opencodeExplicit := false, false
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "codex":
			codexExplicit = true
		case "opencode":
			opencodeExplicit = true
		}
	})
	if *backend == "codex" && opencodeExplicit {
		return fmt.Errorf("--opencode requires --backend opencode")
	}
	if *backend == "opencode" && codexExplicit {
		return fmt.Errorf("--codex requires --backend codex")
	}
	if err := validateBackendFlags(backendFlags{
		backend:      *backend,
		nativeSocket: *nativeSocket,
		home:         *home,
		thread:       *thread,
		cli:          *cli,
		serve:        *serve,
		background:   *background,
		supervise:    *supervise,
		attach:       *attach,
		stop:         *stop,
	}); err != nil {
		return err
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
	if *heartbeat && !*attach {
		return fmt.Errorf("--heartbeat requires --attach")
	}
	if *profile != "" {
		return fmt.Errorf("named profiles are not supported by this diagnostic; no profile was applied")
	}
	if *attach || *stop {
		return supervisorClient(*stateDir, *cursor, *instance, *stop, *heartbeat)
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
	if *probeAntigravity {
		return runAntigravityProbe(cwd, *agy)
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
	spec := backendSpec{kind: *backend, codex: *binary, opencode: *opencodeBinary, home: *home}
	if *background {
		return startBackground(cwd, spec, *stateDir)
	}
	if *supervise {
		return superviseHost(cwd, spec, *stateDir)
	}
	if *serve {
		return serveHostWithSocket(cwd, spec, *nativeSocket)
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

func runAntigravityProbe(cwd, binary string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := acp.Start(ctx, binary, cwd, nil)
	if err != nil {
		return err
	}
	defer client.Close()
	return json.NewEncoder(os.Stdout).Encode(struct {
		Backend        string `json:"backend"`
		Authentication string `json:"authentication"`
		acp.Info
	}{
		Backend: "antigravity", Authentication: "unverified", Info: client.Info(),
	})
}
