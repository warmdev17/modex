//go:build linux || darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOwnershipCrashHelper(t *testing.T) {
	dir := os.Getenv("MODEX_LOCK_TEST_DIR")
	if dir == "" {
		return
	}
	if _, err := claimSupervisorState(dir); err != nil {
		os.Exit(2)
	}
	if err := writeEndpoint(dir, supervisorEndpoint{Address: "127.0.0.1:1", Token: "fixture", Instance: "fixture"}); err != nil {
		os.Exit(3)
	}
	os.Exit(0) // Deliberately exit without releasing state, like an abrupt crash.
}

func TestOwnershipSurvivesProcessDeathWithoutDeletingAttachments(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnershipCrashHelper$")
	cmd.Env = append(os.Environ(), "MODEX_LOCK_TEST_DIR="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	attachment := filepath.Join(dir, "attachment")
	if err := os.WriteFile(attachment, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	release, err := claimSupervisorState(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := os.Stat(filepath.Join(dir, "endpoint.json")); !os.IsNotExist(err) {
		t.Fatal("stale endpoint retained")
	}
	if _, err := claimSupervisorState(dir); err == nil {
		t.Fatal("second owner accepted")
	}
	if data, _ := os.ReadFile(attachment); string(data) != "keep" {
		t.Fatal("attachment lost")
	}
}

func TestLegacyOwnerIsNeverRecovered(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	os.WriteFile(filepath.Join(dir, "owner.lock"), nil, 0600)
	if release, err := claimSupervisorState(dir); err == nil {
		release()
		t.Fatal("accepted legacy marker")
	}
}
