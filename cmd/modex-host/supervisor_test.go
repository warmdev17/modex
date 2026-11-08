package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSupervisorStateExclusiveAndPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("private state ACL support pending")
	}
	dir := filepath.Join(t.TempDir(), "state")
	release, err := claimSupervisorState(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := claimSupervisorState(dir); err == nil {
		t.Fatal("claimed same state twice")
	}
	if err := writeEndpoint(dir, supervisorEndpoint{Address: "127.0.0.1:1234", Token: "secret", Instance: "instance"}); err != nil {
		t.Fatal(err)
	}
	endpoint, err := readEndpoint(dir)
	if err != nil || endpoint.Token != "secret" {
		t.Fatalf("endpoint=%+v err=%v", endpoint, err)
	}
	if err := os.Chmod(filepath.Join(dir, "endpoint.json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readEndpoint(dir); err == nil {
		t.Fatal("read public token file")
	}
}

func TestSupervisorRejectsUnsafeState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("private state ACL support pending")
	}
	root := t.TempDir()
	unsafe := filepath.Join(root, "public")
	if err := os.Mkdir(unsafe, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := claimSupervisorState(unsafe); err == nil {
		t.Fatal("accepted public state directory")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(unsafe, link); err != nil {
		t.Fatal(err)
	}
	if _, err := claimSupervisorState(link); err == nil {
		t.Fatal("accepted symlink")
	}
}

func TestSupervisorRefusesStaleEndpointWithoutDeletingIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("private state ACL support pending")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "endpoint.json")
	if err := os.WriteFile(path, []byte("old endpoint"), 0600); err != nil {
		t.Fatal(err)
	}
	if release, err := claimSupervisorState(dir); err == nil {
		release()
		t.Fatal("accepted stale endpoint")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "old endpoint" {
		t.Fatal("removed foreign state")
	}
	if _, err := os.Stat(filepath.Join(dir, "owner.lock")); !os.IsNotExist(err) {
		t.Fatal("left lock on failed claim")
	}
}
