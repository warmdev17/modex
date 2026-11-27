// Package opencode manages an isolated `opencode serve` child process.
//
// Unlike the Codex app-server, OpenCode exposes an HTTP API with Basic auth.
// The child therefore owns no stdin protocol: the parent only needs to find
// the loopback address it reported and carry an authenticated HTTP client.
package opencode

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	listenPrefix   = "server listening on "
	startupTimeout = 15 * time.Second
	shutdownGrace  = 2 * time.Second
	authUsername   = "opencode"
)

// errNotListening is deliberately generic: startup errors must never carry the
// generated password or any credential-bearing URL the child may have printed.
var errNotListening = errors.New("opencode server did not report a listening address")

// Process owns one isolated `opencode serve` child. Close and CloseIdle are
// idempotent; there is no separate flush for OpenCode, so both are symmetric.
type Process struct {
	cmd         *exec.Cmd
	stdout      *os.File
	baseURL     string
	password    string
	client      *http.Client
	isolatedDir string
	removeDir   bool
	listening   chan int
	waitDone    chan struct{}
	once        sync.Once
}

// StartProcess starts `binary serve --port 0` in cwd with an isolated data and
// state directory. The caller should pass os.Environ() so the user's provider
// auth and configuration remain available.
func StartProcess(ctx context.Context, binary, cwd string, env []string) (*Process, error) {
	isolatedDir, err := os.MkdirTemp("", "modex-opencode-")
	if err != nil {
		return nil, errors.New("cannot create isolated opencode directory")
	}
	return startProcess(ctx, binary, cwd, env, isolatedDir, true)
}

// StartPersistentProcess keeps OpenCode's isolated state across supervisor restarts.
func StartPersistentProcess(ctx context.Context, binary, cwd string, env []string, dir string) (*Process, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.New("invalid private opencode directory")
	}
	if err := validatePrivateDirectory(dir); err != nil {
		return nil, errors.New("invalid private opencode directory")
	}
	return startProcess(ctx, binary, cwd, env, dir, false)
}

func startProcess(ctx context.Context, binary, cwd string, env []string, isolatedDir string, removeDir bool) (*Process, error) {
	cleanup := func() {
		if removeDir {
			_ = os.RemoveAll(isolatedDir)
		}
	}
	password, err := randomPassword()
	if err != nil {
		cleanup()
		return nil, errors.New("cannot initialize opencode credentials")
	}
	dataDir := filepath.Join(isolatedDir, "data")
	stateDir := filepath.Join(isolatedDir, "state")
	for _, dir := range []string{dataDir, stateDir} {
		if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
			cleanup()
			return nil, errors.New("cannot create isolated opencode directory")
		}
		if err := validateProcessDirectory(dir, removeDir); err != nil {
			cleanup()
			return nil, errors.New("invalid private opencode directory")
		}
	}
	if !removeDir {
		dbDir := filepath.Join(dataDir, "opencode")
		if err := os.Mkdir(dbDir, 0700); err != nil && !os.IsExist(err) {
			return nil, errors.New("cannot create isolated opencode directory")
		}
		if err := validatePrivateDirectory(dbDir); err != nil {
			return nil, errors.New("invalid private opencode directory")
		}
		db := filepath.Join(dbDir, "opencode.db")
		f, err := os.OpenFile(db, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			err = f.Close()
		} else if os.IsExist(err) {
			err = validatePrivateFile(db)
		}
		if err != nil {
			return nil, errors.New("invalid private opencode database")
		}
	}
	cmd := exec.CommandContext(ctx, binary, "serve", "--port", "0")
	cmd.Dir = cwd
	cmd.Env = childEnvironment(env, password, dataDir, stateDir)
	if !removeDir {
		filtered := cmd.Env[:0]
		for _, kv := range cmd.Env {
			if !strings.HasPrefix(kv, "OPENCODE_DB=") {
				filtered = append(filtered, kv)
			}
		}
		cmd.Env = append(filtered, "OPENCODE_DB="+filepath.Join(dataDir, "opencode", "opencode.db"))
	}
	// Startup stderr may include credential-bearing URLs; never forward it.
	// Leaving Stderr nil connects it directly to os.DevNull without an exec
	// copying goroutine, which Wait would otherwise wait for (and a grandchild
	// inheriting stderr would keep alive indefinitely).
	stdout, writer, err := os.Pipe()
	if err != nil {
		cleanup()
		return nil, errors.New("cannot create opencode output pipe")
	}
	// cmd.Stdout is a plain *os.File, so exec passes the descriptor straight to
	// the child: cmd.Wait only waits for process exit, never for pipe EOF. A
	// grandchild that inherited the write end therefore cannot block reaping.
	cmd.Stdout = writer
	if err = cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = writer.Close()
		cleanup()
		return nil, errors.New("cannot start opencode executable")
	}
	// The child owns its descriptor now; drop the parent's write end so the
	// scanner observes EOF once no writer remains.
	_ = writer.Close()

	p := &Process{
		cmd:         cmd,
		stdout:      stdout,
		password:    password,
		isolatedDir: isolatedDir,
		removeDir:   removeDir,
		listening:   make(chan int, 1),
		waitDone:    make(chan struct{}),
	}
	go p.wait()
	go p.scan(stdout)

	var port int
	deadline := time.NewTimer(startupTimeout)
	defer deadline.Stop()
	select {
	case port = <-p.listening:
	case <-deadline.C:
		p.kill()
		return nil, errNotListening
	case <-ctx.Done():
		p.kill()
		return nil, ctx.Err()
	}
	if port <= 0 {
		p.kill()
		return nil, errNotListening
	}
	p.baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	p.client = &http.Client{Transport: &authTransport{
		base:     http.DefaultTransport,
		username: authUsername,
		password: password,
	}}
	return p, nil
}

// scan consumes stdout for the lifetime of the child. Startup waits for the
// listening line; afterwards lines are drained so the child never blocks on a
// full pipe. Closing the read end (kill/close) unblocks a scanner stuck behind
// a grandchild that inherited the write end.
func (p *Process) scan(stdout io.Reader) {
	defer p.stdout.Close()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 4096), 1<<20)
	reported := false
	for scanner.Scan() {
		if reported {
			continue
		}
		if port, ok := parseListening(scanner.Text()); ok {
			reported = true
			p.listening <- port
		}
	}
	if !reported {
		p.listening <- -1
	}
}

// wait reaps the direct child without waiting for stdout EOF, so a surviving
// grandchild holding the pipe cannot delay Close or startup failure.
func (p *Process) wait() {
	_ = p.cmd.Wait()
	close(p.waitDone)
}

// kill terminates a child that failed during startup, unblocks the stdout
// scanner, reaps the child and removes the isolated directory. The success
// path returns a live *Process whose close() owns its own cleanup, so this is
// never called twice for one directory.
func (p *Process) kill() {
	_ = p.stdout.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	<-p.waitDone
	if p.removeDir {
		_ = os.RemoveAll(p.isolatedDir)
	}
}

// Close terminates the child with SIGTERM, then SIGKILL after a short grace.
// Temporary data/state directories are removed; persistent ones are retained.
func (p *Process) Close() { p.close() }

// CloseIdle is symmetric with Close: OpenCode has no separate idle flush.
func (p *Process) CloseIdle() { p.close() }

func (p *Process) close() {
	p.once.Do(func() {
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Signal(syscall.SIGTERM)
		}
		select {
		case <-p.waitDone:
		case <-time.After(shutdownGrace):
			if p.cmd.Process != nil {
				_ = p.cmd.Process.Kill()
			}
			<-p.waitDone
		}
		// Unblock the scanner even if a grandchild still holds the write end.
		_ = p.stdout.Close()
		if p.removeDir {
			_ = os.RemoveAll(p.isolatedDir)
		}
	})
}

// BaseURL returns the loopback base address reported by the child.
func (p *Process) BaseURL() string { return p.baseURL }

// Client returns an HTTP client that authenticates as the OpenCode server user.
func (p *Process) Client() *http.Client { return p.client }

func randomPassword() (string, error) {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(data[:]), nil
}

// childEnvironment builds the child environment: the caller's environment
// (usually os.Environ(), keeping provider auth/config) with the per-process
// password and isolated XDG data/state directories overriding any inherited
// values.
func childEnvironment(env []string, password, dataDir, stateDir string) []string {
	if env == nil {
		env = os.Environ()
	}
	return append(filterEnv(env),
		"OPENCODE_PASSWORD="+password,
		"XDG_DATA_HOME="+dataDir,
		"XDG_STATE_HOME="+stateDir,
	)
}

// filterEnv drops inherited values the host must override to guarantee the
// per-process password and isolated data/state directories.
func filterEnv(env []string) []string {
	out := make([]string, 0, len(env)+3)
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "OPENCODE_PASSWORD="),
			strings.HasPrefix(kv, "XDG_DATA_HOME="),
			strings.HasPrefix(kv, "XDG_STATE_HOME="):
			continue
		}
		out = append(out, kv)
	}
	return out
}

// parseListening extracts the port from `server listening on http://HOST:PORT`.
// Only loopback hosts are accepted so the isolated server never answers on an
// external interface.
func parseListening(line string) (int, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, listenPrefix) {
		return 0, false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(line, listenPrefix))
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" {
		return 0, false
	}
	host := parsed.Hostname()
	if host != "127.0.0.1" && host != "localhost" {
		return 0, false
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port <= 0 || port > 65535 {
		return 0, false
	}
	return port, true
}

// authTransport injects Basic auth without mutating the caller's request.
type authTransport struct {
	base     http.RoundTripper
	username string
	password string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	clone := req.Clone(req.Context())
	clone.SetBasicAuth(t.username, t.password)
	return base.RoundTrip(clone)
}
