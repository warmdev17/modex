package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/warmdev/modex/internal/diagnostic"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "modex-host:", err)
		os.Exit(1)
	}
}

func run() error {
	workspace := flag.String("workspace", "", "Existing workspace directory (required)")
	binary := flag.String("codex", "codex", "Codex executable")
	home := flag.String("codex-home", "", "Override CODEX_HOME; defaults to inherited environment")
	profile := flag.String("profile", "", "Named profile (not yet supported; fails explicitly)")
	serve := flag.Bool("serve", false, "Serve Modex commands over stdin/stdout for the lifetime of this connection")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *profile != "" {
		return fmt.Errorf("named profiles are not supported by this diagnostic; no profile was applied")
	}
	if *workspace == "" {
		return fmt.Errorf("--workspace is required")
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
	if *serve {
		return serveHost(cwd, *binary, *home)
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
