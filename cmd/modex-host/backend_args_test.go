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

func TestServeArgsOpenCodeHasNoNativeSocket(t *testing.T) {
	got := serveArgs("/work", backendSpec{kind: "opencode", opencode: "opencode-bin", home: "/ignored"}, "/tmp/native.sock")
	want := []string{"--serve", "--workspace", "/work", "--backend", "opencode", "--opencode", "opencode-bin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q", got)
	}
}
