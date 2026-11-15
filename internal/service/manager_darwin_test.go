package service

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRestartUsesLoadedLaunchAgent(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	path, err := registration(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("keep existing configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	tools := filepath.Join(root, "bin")
	if err := os.Mkdir(tools, 0700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "calls")
	manager := `#!/bin/sh
printf '%s\n' "$*" >> "$MODEX_TEST_CALLS"
exit "$MODEX_TEST_EXIT"
`
	if err := os.WriteFile(filepath.Join(tools, "launchctl"), []byte(manager), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools)
	t.Setenv("MODEX_TEST_CALLS", log)
	for _, code := range []string{"0", "7"} {
		t.Setenv("MODEX_TEST_EXIT", code)
		if err := os.WriteFile(log, nil, 0600); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err := Run([]string{"restart"}, &out)
		if (err != nil) != (code != "0") {
			t.Fatalf("exit=%s err=%v", code, err)
		}
		calls, _ := os.ReadFile(log)
		want := fmt.Sprintf("kickstart -k gui/%d/%s\n", os.Getuid(), Name)
		if string(calls) != want {
			t.Fatalf("calls=%q want=%q", calls, want)
		}
		if !bytes.Contains(out.Bytes(), []byte("interrupt active turns")) {
			t.Fatal("missing restart warning")
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep existing configuration" {
		t.Fatal("restart changed registration")
	}
}
