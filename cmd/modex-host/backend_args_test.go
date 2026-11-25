package main

import (
	"reflect"
	"testing"
)

func TestServeArgsKeepsCodexContract(t *testing.T) {
	got := serveArgs("/work", backendSpec{kind: "codex", codex: "codex-bin", home: "/home/user/.codex"}, "/tmp/native.sock")
	want := []string{"--serve", "--workspace", "/work", "--codex", "codex-bin", "--native-socket", "/tmp/native.sock", "--codex-home", "/home/user/.codex"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q", got)
	}
	got = serveArgs("/work", backendSpec{kind: "codex", codex: "codex"}, "/tmp/native.sock")
	want = []string{"--serve", "--workspace", "/work", "--codex", "codex", "--native-socket", "/tmp/native.sock"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q", got)
	}
}

func TestSuperviseArgsKeepsCodexContract(t *testing.T) {
	got := superviseArgs("/work", backendSpec{kind: "codex", codex: "codex-bin", home: "/home/user/.codex"}, "/state")
	want := []string{"--supervise", "--workspace", "/work", "--codex", "codex-bin", "--state-dir", "/state", "--codex-home", "/home/user/.codex"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q", got)
	}
	got = superviseArgs("/work", backendSpec{kind: "codex", codex: "codex"}, "/state")
	want = []string{"--supervise", "--workspace", "/work", "--codex", "codex", "--state-dir", "/state"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q", got)
	}
}

func TestSuperviseArgsOpenCodeHasNoCodexHome(t *testing.T) {
	got := superviseArgs("/work", backendSpec{kind: "opencode", opencode: "opencode-bin", home: "/ignored"}, "/state")
	want := []string{"--supervise", "--workspace", "/work", "--backend", "opencode", "--opencode", "opencode-bin", "--state-dir", "/state"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q", got)
	}
}

func TestServeArgsOpenCodeHasNoNativeSocket(t *testing.T) {
	got := serveArgs("/work", backendSpec{kind: "opencode", opencode: "opencode-bin", home: "/ignored"}, "/tmp/native.sock")
	want := []string{"--serve", "--workspace", "/work", "--backend", "opencode", "--opencode", "opencode-bin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q", got)
	}
}
