//go:build !windows

package opencode

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFake creates a minimal `opencode` replacement. Tests skip on Windows
// because the helper is a POSIX shell script.
func writeFake(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStartProcessParsesListeningAddress(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	envFile := filepath.Join(dir, "env")
	fake := writeFake(t, fmt.Sprintf(
		"printf '%%s\\n' \"$@\" > '%s'\nprintenv OPENCODE_PASSWORD > '%s'\nprintf 'booting\\n'\nprintf 'server listening on http://127.0.0.1:41234\\n'\nexec sleep 60\n",
		argsFile, envFile))

	p, err := StartProcess(context.Background(), fake, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if p.BaseURL() != "http://127.0.0.1:41234" {
		t.Fatalf("baseURL=%q", p.BaseURL())
	}
	if p.Client() == nil {
		t.Fatal("client is nil")
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(args)); got != "serve\n--port\n0" {
		t.Fatalf("child args=%q", got)
	}
	password, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.TrimSpace(string(password))) != 64 {
		t.Fatalf("password length=%d", len(strings.TrimSpace(string(password))))
	}
	for _, name := range []string{"data", "state"} {
		info, err := os.Stat(filepath.Join(p.isolatedDir, name))
		if err != nil || !info.IsDir() {
			t.Fatalf("isolated %s dir: %v", name, err)
		}
	}
	isolated := p.isolatedDir
	p.Close()
	if _, err := os.Stat(isolated); !os.IsNotExist(err) {
		t.Fatalf("isolated directory was not removed: %v", err)
	}
	// Both closers are idempotent.
	p.Close()
	p.CloseIdle()
}

func TestStartProcessDoesNotLeakPasswordInError(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env")
	fake := writeFake(t, fmt.Sprintf(
		"printenv OPENCODE_PASSWORD > '%s'\nexit 1\n", envFile))

	p, err := StartProcess(context.Background(), fake, t.TempDir(), os.Environ())
	if err == nil {
		if p != nil {
			p.Close()
		}
		t.Fatal("expected a startup failure")
	}
	raw, readErr := os.ReadFile(envFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	password := strings.TrimSpace(string(raw))
	if len(password) != 64 {
		t.Fatalf("password length=%d", len(password))
	}
	if strings.Contains(err.Error(), password) {
		t.Fatal("startup error leaked the password")
	}
}

func TestCloseIdleRemovesIsolatedDirectory(t *testing.T) {
	fake := writeFake(t, "printf 'server listening on http://127.0.0.1:41235\\n'\nexec sleep 60\n")
	p, err := StartProcess(context.Background(), fake, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	isolated := p.isolatedDir
	p.CloseIdle()
	if _, err := os.Stat(isolated); !os.IsNotExist(err) {
		t.Fatalf("isolated directory was not removed: %v", err)
	}
	p.Close()
}

func TestParseListening(t *testing.T) {
	for _, tt := range []struct {
		line string
		port int
		ok   bool
	}{
		{"server listening on http://127.0.0.1:41234", 41234, true},
		{"server listening on http://localhost:8080", 8080, true},
		{"  server listening on http://127.0.0.1:1  ", 1, true},
		{"server listening on http://0.0.0.0:41234", 0, false},
		{"server listening on http://example.com:41234", 0, false},
		{"server listening on https://127.0.0.1:41234", 0, false},
		{"server listening on http://127.0.0.1:0", 0, false},
		{"server listening on http://127.0.0.1:notaport", 0, false},
		{"booting", 0, false},
	} {
		port, ok := parseListening(tt.line)
		if port != tt.port || ok != tt.ok {
			t.Fatalf("%q: port=%d ok=%v", tt.line, port, ok)
		}
	}
}

func TestAuthTransportSetsBasicAuthWithoutMutatingRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != authUsername || password != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := &http.Client{Transport: &authTransport{
		base:     http.DefaultTransport,
		username: authUsername,
		password: "secret",
	}}
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if _, _, ok := req.BasicAuth(); ok {
		t.Fatal("caller request was mutated")
	}
}
