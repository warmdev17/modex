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

func TestRestartUsesExistingUserService(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "config")
	tools := filepath.Join(root, "bin")
	if err := os.MkdirAll(filepath.Join(config, "systemd", "user"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(tools, 0700); err != nil {
		t.Fatal(err)
	}
	registration := filepath.Join(config, "systemd", "user", Name+".service")
	if err := os.WriteFile(registration, []byte("keep existing configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "calls")
	manager := `#!/bin/sh
printf '%s\n' "$*" >> "$MODEX_TEST_CALLS"
if [ "$2" = "$MODEX_TEST_FAIL" ]; then exit 7; fi
`
	if err := os.WriteFile(filepath.Join(tools, "systemctl"), []byte(manager), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("PATH", tools)
	t.Setenv("MODEX_TEST_CALLS", log)
	for _, fail := range []string{"", "daemon-reload", "restart"} {
		t.Setenv("MODEX_TEST_FAIL", fail)
		if err := os.WriteFile(log, nil, 0600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err := Run([]string{"restart"}, &out)
		if (err != nil) != (fail != "") {
			t.Fatalf("fail=%q err=%v", fail, err)
		}
		calls, _ := os.ReadFile(log)
		want := "--user daemon-reload\n"
		if fail != "daemon-reload" {
			want += "--user restart " + Name + ".service\n"
		}
		if string(calls) != want {
			t.Fatalf("calls=%q want=%q", calls, want)
		}
		if !bytes.Contains(out.Bytes(), []byte("interrupt active turns")) {
			t.Fatal("missing restart warning")
		}
	}
	data, err := os.ReadFile(registration)
	if err != nil || string(data) != "keep existing configuration" {
		t.Fatal("restart changed registration")
	}
	if err := os.Chmod(registration, 0644); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"restart"}, &bytes.Buffer{}); err == nil {
		t.Fatal("accepted public registration")
	}
	if err := os.Remove(registration); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"restart"}, &bytes.Buffer{}); err == nil {
		t.Fatal("accepted missing registration")
	}
}
