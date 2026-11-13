package service

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLifecycleUsesUserManagerAndRefusesActiveUninstall(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "config")
	tools := filepath.Join(root, "bin")
	os.MkdirAll(filepath.Join(config, "systemd", "user"), 0700)
	os.Mkdir(tools, 0700)
	registration := filepath.Join(config, "systemd", "user", Name+".service")
	os.WriteFile(registration, []byte("fixture"), 0600)
	log := filepath.Join(root, "calls")
	// Never invoke a real service manager from this regression.
	manager := `#!/bin/sh
printf '%s\n' "$*" >> "$MODEX_TEST_CALLS"
if [ "$2" = show ]; then printf '%s\n' "$MODEX_TEST_STATE"; fi
`
	os.WriteFile(filepath.Join(tools, "systemctl"), []byte(manager), 0700)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("PATH", tools)
	t.Setenv("MODEX_TEST_CALLS", log)
	t.Setenv("MODEX_TEST_STATE", "active")
	var out bytes.Buffer
	if err := Run([]string{"uninstall"}, &out); err == nil {
		t.Fatal("removed active registration")
	}
	if _, err := os.Stat(registration); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"start", "status", "stop"} {
		if err := Run([]string{action}, &out); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("MODEX_TEST_STATE", "inactive")
	if err := Run([]string{"uninstall"}, &out); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(log)
	for _, line := range bytes.Split(bytes.TrimSpace(calls), []byte("\n")) {
		if !bytes.HasPrefix(line, []byte("--user ")) {
			t.Fatal("non-user manager", string(line))
		}
	}
	if _, err := os.Stat(registration); !os.IsNotExist(err) {
		t.Fatal("registration remains")
	}
}
