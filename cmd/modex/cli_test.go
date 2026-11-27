package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestCLISubcommandHelper(t *testing.T) {
	if os.Getenv("MODEX_CLI_SUBPROCESS") != "1" {
		return
	}
	if err := launchCLI(os.Args[3:]); err != nil {
		t.Fatal(err)
	}
}

func TestCLISubcommandForwardsWorkspaceWithoutRequiringID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	output := filepath.Join(dir, "args")
	fake := filepath.Join(dir, "host")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > '"+output+"'\nprintf '%s' \"$$\" > '"+output+".pid'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCLISubcommandHelper$", "--", "--host-binary", fake, "--codex", "custom-codex", "--codex-home", dir)
	cmd.Env = append(os.Environ(), "MODEX_CLI_SUBPROCESS=1")
	if result, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("launch failed: %s %v", result, err)
	}
	pid, err := os.ReadFile(output + ".pid")
	if err != nil || string(pid) != strconv.Itoa(cmd.Process.Pid) {
		t.Fatalf("wrapper was not replaced; termination signals would miss the launcher: %s %v", pid, err)
	}
	args, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if string(args) != "--cli\n--workspace\n"+cwd+"\n--codex\ncustom-codex\n--codex-home\n"+dir+"\n" {
		t.Fatalf("forwarded args: %s", args)
	}
	if err := launchCLI([]string{"--host-binary", fake, "unexpected"}); err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("accepted positional input: %v", err)
	}
}

func TestDefaultCLICodexPrefersExecutableSibling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix launcher")
	}
	dir := t.TempDir()
	launcher := filepath.Join(dir, "modex")
	executable, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, executable, 0700); err != nil {
		t.Fatal(err)
	}
	host := filepath.Join(dir, "modex-host")
	if err := os.WriteFile(host, []byte("#!/bin/sh\nprintf '%s\\n' \"$5\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	patched := filepath.Join(dir, "codex-modex-plan")
	check := func(want string, flags ...string) {
		t.Helper()
		cmd := exec.Command(launcher, append([]string{"-test.run=^TestCLISubcommandHelper$", "--"}, flags...)...)
		cmd.Env = append(os.Environ(), "MODEX_CLI_SUBPROCESS=1")
		if got, err := cmd.CombinedOutput(); err != nil || strings.TrimSpace(string(got)) != want {
			t.Fatalf("Codex executable: got %q (%v), want %q", got, err, want)
		}
	}
	check("codex")
	if err := os.WriteFile(patched, nil, 0600); err != nil {
		t.Fatal(err)
	}
	check("codex")
	if err := os.Chmod(patched, 0700); err != nil {
		t.Fatal(err)
	}
	check(patched)
	check("custom-codex", "--codex", "custom-codex")
}
