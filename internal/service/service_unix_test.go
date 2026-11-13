//go:build linux || darwin

package service

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warmdev/modex/internal/hoststate"
)

func TestInstallIsPrivateExplicitAndDoesNotStartManager(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "state")
	config := filepath.Join(root, "config", "host.service")
	binary := filepath.Join(root, "fixture codex")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", "")
	t.Setenv("OPENAI_API_KEY", "DO_NOT_COPY_SECRET")
	args := []string{"--workspace", root, "--state-dir", state, "--host-binary", binary, "--codex", binary}
	var out bytes.Buffer
	if err := install(args, root, config, &out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "DO_NOT_COPY_SECRET") || !strings.Contains(out.String(), "not started") {
		t.Fatal("unsafe configuration")
	}
	for _, p := range []string{state, config} {
		info, err := os.Stat(p)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatal("non-private installation")
		}
	}
	if err := install(args, root, config, &out); err == nil {
		t.Fatal("overwrote registration")
	}
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "owner.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := install(args, root, config, &out); err == nil {
		t.Fatal("reused owned state")
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("partial registration")
	}
}

func TestInstallRejectsInvalidPaths(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "codex")
	os.WriteFile(binary, []byte("#!/bin/sh\n"), 0700)
	t.Setenv("CODEX_HOME", "")
	for _, extra := range [][]string{{"--workspace", "relative"}, {"--codex", filepath.Join(root, "missing")}, {"--codex-home", "relative"}} {
		args := []string{"--workspace", root, "--state-dir", filepath.Join(root, "state"), "--host-binary", binary, "--codex", binary}
		args = append(args, extra...)
		if err := install(args, root, filepath.Join(root, "unit"), &bytes.Buffer{}); err == nil {
			t.Fatal("invalid install accepted", extra)
		}
	}
	public := filepath.Join(root, "public")
	os.Mkdir(public, 0755)
	if err := hoststate.Validate(public, true); err == nil {
		t.Fatal("accepted public state")
	}
	link := filepath.Join(root, "link")
	os.Symlink(public, link)
	if err := hoststate.Validate(link, true); err == nil {
		t.Fatal("accepted symlink")
	}
}

func TestReinstallCanReuseStoppedNewFormatState(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "state")
	os.Mkdir(state, 0700)
	os.WriteFile(filepath.Join(state, "owner.lock"), []byte("modex-flock-v1\n"), 0600)
	attachment := filepath.Join(state, "keep")
	os.WriteFile(attachment, []byte("attachment"), 0600)
	binary := filepath.Join(root, "binary")
	os.WriteFile(binary, []byte("#!/bin/sh\n"), 0700)
	t.Setenv("CODEX_HOME", "")
	args := []string{"--workspace", root, "--state-dir", state, "--host-binary", binary, "--codex", binary}
	release, err := hoststate.Acquire(state)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := filepath.Join(state, "endpoint.json")
	os.WriteFile(endpoint, []byte("private stale endpoint"), 0600)
	if err := install(args, root, filepath.Join(root, "unit"), &bytes.Buffer{}); err == nil {
		release()
		t.Fatal("accepted live owner")
	}
	release()
	if err := install(args, root, filepath.Join(root, "unit"), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(endpoint); string(b) != "private stale endpoint" {
		t.Fatal("install mutated endpoint")
	}
	if b, _ := os.ReadFile(attachment); string(b) != "attachment" {
		t.Fatal("lost state")
	}
}
