//go:build !windows

package opencode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryPersistsPrivateThreadIdentity(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	r, err := openRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := r.add("native-session", t.TempDir(), "shared-project", false)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := openRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reopened.get(entry.ID)
	if !ok || got.Native != "native-session" || got.Workspace != entry.Workspace || got.Project != "shared-project" {
		t.Fatalf("registry lost mapping: %+v", got)
	}
	if got.ID == got.Native || len(got.ID) < 4 || got.ID[:3] != "oc:" {
		t.Fatalf("invalid routed identity %q", got.ID)
	}
	if err := validatePrivateFile(filepath.Join(dir, "opencode-threads.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryRejectsMalformedOrInsecureState(t *testing.T) {
	for _, tc := range []struct {
		name, contents string
		mode           os.FileMode
	}{
		{"malformed", `[{"id":"oc:x","native_id":"n","workspace":"relative"}]`, 0600},
		{"public", `[]`, 0644},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "opencode-threads.json")
			if err := os.WriteFile(path, []byte(tc.contents), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, tc.mode); err != nil {
				t.Fatal(err)
			}
			if _, err := openRegistry(dir); err == nil {
				t.Fatal("accepted unsafe registry")
			}
		})
	}
}

func TestRegistryRejectsDuplicateNativeChatInSameWorkspace(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	r, err := openRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	if _, err := r.add("native", workspace, "p", false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.add("native", workspace, "other", false); err == nil {
		t.Fatal("accepted duplicate native chat")
	}
	reopened, err := openRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.chats) != 1 {
		t.Fatalf("registry changed: %+v", reopened.chats)
	}
}
