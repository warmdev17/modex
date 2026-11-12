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
