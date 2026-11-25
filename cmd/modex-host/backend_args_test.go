package main

import (
	"os/exec"
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

// TestModeCommandsUseArgHelpers guards the exec seam: a mode handler must pass
// the shared argv helper's output, not an inlined copy that could drift.
func TestModeCommandsUseArgHelpers(t *testing.T) {
	original := execCommand
	t.Cleanup(func() { execCommand = original })

	var calls [][]string
	execCommand = func(name string, args ...string) *exec.Cmd {
		calls = append(calls, append([]string{name}, args...))
		return exec.Command("true")
	}

	spec := backendSpec{kind: "codex", codex: "codex-bin", home: "/home/user/.codex"}
	_ = serveChildCommand("/self", "/work", spec, "/tmp/native.sock")
	_ = superviseChildCommand("/self", "/work", spec, "/state")

	wantServe := append([]string{"/self"}, serveArgs("/work", spec, "/tmp/native.sock")...)
	wantSupervise := append([]string{"/self"}, superviseArgs("/work", spec, "/state")...)
	if len(calls) != 2 || !reflect.DeepEqual(calls[0], wantServe) || !reflect.DeepEqual(calls[1], wantSupervise) {
		t.Fatalf("commands=%q want %q and %q", calls, wantServe, wantSupervise)
	}
}
