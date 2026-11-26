package main

import (
	"strings"
	"testing"
)

func TestBackendFlagValidation(t *testing.T) {
	// attach/stop are backend-agnostic: --backend and codex-only flags are ignored.
	for _, flags := range []backendFlags{
		{backend: "opencode", attach: true},
		{backend: "opencode", stop: true},
		{backend: "opencode", attach: true, home: "/home/user/.codex", thread: "thread", nativeSocket: "/tmp/codex.sock"},
		{backend: "opencode", stop: true, home: "/home/user/.codex"},
	} {
		if err := validateBackendFlags(flags); err != nil {
			t.Fatalf("attach/stop rejected opencode: %+v: %v", flags, err)
		}
	}

	// Spawn/open modes accept opencode when no codex-only flag is present.
	for _, flags := range []backendFlags{
		{backend: "opencode", serve: true},
		{backend: "opencode", background: true},
		{backend: "opencode", supervise: true},
		{backend: "codex"},
		{backend: "codex", attach: true},
		{backend: "codex", serve: true, nativeSocket: "/tmp/codex.sock", home: "/home/user/.codex"},
	} {
		if err := validateBackendFlags(flags); err != nil {
			t.Fatalf("valid flags rejected: %+v: %v", flags, err)
		}
	}

	// Backend-owning modes still reject opencode (and its codex-only flags).
	for _, test := range []struct {
		flags backendFlags
		want  string
	}{
		{backendFlags{backend: "opencode", cli: true}, "does not support --cli"},
		{backendFlags{backend: "opencode", serve: true, nativeSocket: "/tmp/codex.sock"}, "does not support --native-socket"},
		{backendFlags{backend: "opencode", supervise: true, home: "/home/user/.codex"}, "does not support --codex-home"},
		{backendFlags{backend: "opencode", serve: true, thread: "thread"}, "does not support --thread"},
		{backendFlags{backend: "opencode"}, "requires --serve, --background, or --supervise"},
	} {
		err := validateBackendFlags(test.flags)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%+v: err=%v want %q", test.flags, err, test.want)
		}
	}
}
