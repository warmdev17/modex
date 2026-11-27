//go:build !windows

package opencode

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
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

func TestChildEnvironmentReplacesParentCredentials(t *testing.T) {
	got := childEnvironment([]string{
		"OPENCODE_PASSWORD=parent",
		"XDG_DATA_HOME=/parent/data",
		"XDG_STATE_HOME=/parent/state",
		"NEUTRAL=kept",
	}, "child-password", "/iso/data", "/iso/state")

	counts := map[string]int{}
	values := map[string][]string{}
	for _, kv := range got {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		counts[key]++
		values[key] = append(values[key], value)
	}
	for _, tt := range []struct{ key, want string }{
		{"OPENCODE_PASSWORD", "child-password"},
		{"XDG_DATA_HOME", "/iso/data"},
		{"XDG_STATE_HOME", "/iso/state"},
	} {
		if counts[tt.key] != 1 {
			t.Fatalf("%s entries=%d want 1", tt.key, counts[tt.key])
		}
		if values[tt.key][0] != tt.want {
			t.Fatalf("%s=%q want %q", tt.key, values[tt.key][0], tt.want)
		}
	}
	if counts["NEUTRAL"] != 1 || values["NEUTRAL"][0] != "kept" {
		t.Fatalf("neutral variable lost: %v", values["NEUTRAL"])
	}
}

func TestFilterEnvRemovesHostOverrides(t *testing.T) {
	got := filterEnv([]string{
		"OPENCODE_PASSWORD=parent",
		"XDG_DATA_HOME=/parent/data",
		"XDG_STATE_HOME=/parent/state",
		"NEUTRAL=kept",
	})
	if len(got) != 1 || got[0] != "NEUTRAL=kept" {
		t.Fatalf("env=%q", got)
	}
}

func TestStartProcessRemovesIsolatedDirOnDiscoveryFailure(t *testing.T) {
	dir := t.TempDir()
	dataEnvFile := filepath.Join(dir, "data-home")
	fake := writeFake(t, fmt.Sprintf(
		"printenv XDG_DATA_HOME > '%s'\nexit 1\n", dataEnvFile))

	p, err := StartProcess(context.Background(), fake, t.TempDir(), os.Environ())
	if err == nil {
		if p != nil {
			p.Close()
		}
		t.Fatal("expected a discovery failure")
	}
	raw, readErr := os.ReadFile(dataEnvFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	dataDir := strings.TrimSpace(string(raw))
	if dataDir == "" {
		t.Fatal("child did not report XDG_DATA_HOME")
	}
	// XDG_DATA_HOME points at <isolated>/data; its parent is the isolated dir.
	isolated := filepath.Dir(dataDir)
	if !strings.HasPrefix(filepath.Base(isolated), "modex-opencode-") {
		t.Fatalf("unexpected isolated directory %q", isolated)
	}
	if _, statErr := os.Stat(isolated); !os.IsNotExist(statErr) {
		t.Fatalf("isolated directory was not removed after failure: %v", statErr)
	}
}

// Bounded by the select below; run with -timeout 120s (or larger) so a
// regression reports the explicit 25s message instead of a package panic.
func TestStartProcessDiscoveryFailureWithHungGrandchild(t *testing.T) {
	dir := t.TempDir()
	dataFile := filepath.Join(dir, "data-home")
	pidFile := filepath.Join(dir, "grandchild.pid")
	fake := writeFake(t, fmt.Sprintf(
		"printenv XDG_DATA_HOME > '%s'\nsleep 120 &\necho $! > '%s'\nexit 0\n",
		dataFile, pidFile))
	defer killPID(t, pidFile)

	type startResult struct {
		p   *Process
		err error
	}
	started := make(chan startResult, 1)
	start := time.Now()
	go func() {
		p, err := StartProcess(context.Background(), fake, t.TempDir(), os.Environ())
		started <- startResult{p, err}
	}()
	var result startResult
	select {
	case result = <-started:
	case <-time.After(25 * time.Second):
		t.Fatal("StartProcess did not return within 25s; the stdout drain is gated on EOF again")
	}
	elapsed := time.Since(start)
	p, err := result.p, result.err
	if err == nil {
		if p != nil {
			p.Close()
		}
		t.Fatal("expected a discovery failure")
	}
	// Without a bounded stdout drain this would wait for the grandchild (120s).
	if elapsed > 25*time.Second {
		t.Fatalf("StartProcess was blocked by inherited stdout for %v", elapsed)
	}
	// The grandchild must still be alive, otherwise the scenario is not
	// exercising the inherited-descriptor path.
	if elapsed < 10*time.Second {
		t.Fatalf("returned before the discovery deadline (%v); grandchild did not hold stdout", elapsed)
	}
	raw, readErr := os.ReadFile(dataFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	isolated := filepath.Dir(strings.TrimSpace(string(raw)))
	if !strings.HasPrefix(filepath.Base(isolated), "modex-opencode-") {
		t.Fatalf("unexpected isolated directory %q", isolated)
	}
	if _, statErr := os.Stat(isolated); !os.IsNotExist(statErr) {
		t.Fatalf("isolated directory was not removed after failure: %v", statErr)
	}
}

// Bounded by the select below; run with -timeout 120s (or larger) so a
// regression reports the explicit 4s message instead of a package panic.
func TestCloseWithHungGrandchild(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")
	fake := writeFake(t, fmt.Sprintf(
		"trap '' TERM\nsleep 120 &\necho $! > '%s'\nprintf 'server listening on http://127.0.0.1:41236\\n'\nwait\n",
		pidFile))
	defer killPID(t, pidFile)

	p, err := StartProcess(context.Background(), fake, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	closed := make(chan struct{})
	go func() {
		p.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(4 * time.Second):
		t.Fatal("Close did not return within 4s; the stdout drain is gated on EOF again")
	}
	elapsed := time.Since(start)
	// 2s SIGTERM grace plus reaping; a pipe-EOF gate would hang for 120s.
	if elapsed > 4*time.Second {
		t.Fatalf("Close was blocked by inherited stdout for %v", elapsed)
	}
	if _, statErr := os.Stat(p.isolatedDir); !os.IsNotExist(statErr) {
		t.Fatalf("isolated directory was not removed: %v", statErr)
	}
}

// killPID removes a grandchild a fake opencode left behind so tests do not
// accumulate long-lived sleep processes.
func killPID(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
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

func TestPersistentProcessPreservesStateAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	fake := writeFake(t, "printf 'server listening on http://127.0.0.1:41234\\n'\nexec sleep 60\n")
	for i := 0; i < 2; i++ {
		p, err := StartPersistentProcess(context.Background(), fake, t.TempDir(), os.Environ(), dir)
		if err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(dir, "data", "marker")
		if i == 0 {
			if err := os.WriteFile(marker, []byte("saved"), 0600); err != nil {
				t.Fatal(err)
			}
		} else if b, err := os.ReadFile(marker); err != nil || string(b) != "saved" {
			t.Fatalf("state lost: %q %v", b, err)
		}
		p.Close()
	}
}

func TestPersistentProcessRejectsSymlinkStateDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "data")); err != nil {
		t.Fatal(err)
	}
	fake := writeFake(t, "exit 1\n")
	if p, err := StartPersistentProcess(context.Background(), fake, t.TempDir(), nil, dir); err == nil {
		p.Close()
		t.Fatal("accepted symlink state directory")
	}
}

func TestPersistentProcessOverridesInheritedDatabase(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(t.TempDir(), "db")
	fake := writeFake(t, fmt.Sprintf("printenv OPENCODE_DB > '%s'\nprintf 'server listening on http://127.0.0.1:41234\\n'\nexec sleep 60\n", envFile))
	p, err := StartPersistentProcess(context.Background(), fake, t.TempDir(), append(os.Environ(), "OPENCODE_DB=/outside/user.db"), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	raw, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(raw)); got != filepath.Join(dir, "data", "opencode", "opencode.db") {
		t.Fatalf("database path=%q", got)
	}
}

func TestPersistentProcessRejectsSymlinkDatabaseDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "data", "opencode")); err != nil {
		t.Fatal(err)
	}
	fake := writeFake(t, "printf 'server listening on http://127.0.0.1:41234\\n'\nexec sleep 60\n")
	if p, err := StartPersistentProcess(context.Background(), fake, t.TempDir(), nil, dir); err == nil {
		p.Close()
		t.Fatal("accepted symlink database directory")
	}
}

func TestPersistentProcessRejectsSymlinkDatabase(t *testing.T) {
	dir := t.TempDir()
	dbDir := filepath.Join(dir, "data", "opencode")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), filepath.Join(dbDir, "opencode.db")); err != nil {
		t.Fatal(err)
	}
	fake := writeFake(t, "printf 'server listening on http://127.0.0.1:41234\\n'\nexec sleep 60\n")
	if p, err := StartPersistentProcess(context.Background(), fake, t.TempDir(), nil, dir); err == nil {
		p.Close()
		t.Fatal("accepted symlink database")
	}
}

func TestPersistentProcessPreservesDataAfterStartupFailure(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(data, "marker")
	if err := os.WriteFile(marker, []byte("retained"), 0600); err != nil {
		t.Fatal(err)
	}
	fake := writeFake(t, "exit 1\n")
	if p, err := StartPersistentProcess(context.Background(), fake, t.TempDir(), nil, dir); err == nil {
		p.Close()
		t.Fatal("expected startup failure")
	}
	if raw, err := os.ReadFile(marker); err != nil || string(raw) != "retained" {
		t.Fatalf("state lost: %q %v", raw, err)
	}
}

func TestPersistentProcessUsesAbsoluteDatabasePath(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(cwd, ".managed-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	rel, err := filepath.Rel(cwd, dir)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "db")
	fake := writeFake(t, fmt.Sprintf("printenv OPENCODE_DB > '%s'\nprintf 'server listening on http://127.0.0.1:41234\\n'\nexec sleep 60\n", output))
	p, err := StartPersistentProcess(context.Background(), fake, t.TempDir(), nil, rel)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(strings.TrimSpace(string(raw))) {
		t.Fatalf("relative database path: %q", raw)
	}
}
