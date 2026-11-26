package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
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

func TestSupervisorAttachCLIAdvertisesRunningBackend(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable fixture")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	root := t.TempDir()
	binary := filepath.Join(root, "modex-host")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	cases := []struct{ kind, attachKind, script, capabilities string }{
		{"opencode", "codex", `
import http.server, socketserver, time
class Handler(http.server.BaseHTTPRequestHandler):
 def log_message(self, *args): pass
 def do_GET(self):
  if self.path.startswith('/api/event'):
   self.send_response(200); self.send_header('Content-Type','text/event-stream'); self.end_headers()
   try:
    while True: self.wfile.write(b': keepalive\n\n'); self.wfile.flush(); time.sleep(.2)
   except Exception: pass
  else:
   body = b'[]' if self.path.startswith('/api/config') else b'{}'
   self.send_response(200); self.send_header('Content-Length',str(len(body))); self.end_headers(); self.wfile.write(body)
class Server(socketserver.ThreadingMixIn, http.server.HTTPServer): daemon_threads = True
s=Server(('127.0.0.1',0),Handler)
print('server listening on http://127.0.0.1:%d' % s.server_port,flush=True)
s.serve_forever()
`, `[]`},
		{"codex", "opencode", `
import json,sys
for line in sys.stdin:
 r=json.loads(line)
 if r.get('method') != 'initialized':
  result={'config':{'model':'fixture','model_provider':'fixture'}} if r.get('method') == 'config/read' else {}
  print(json.dumps({'id':r.get('id'),'result':result}),flush=True)
`, `["session_status","plan_mode","permission_profiles","session_lifecycle","projects","references","takeover"]`},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			dir := t.TempDir()
			fake := filepath.Join(dir, tc.kind)
			if err := os.WriteFile(fake, []byte("#!"+python+"\n"+tc.script), 0700); err != nil {
				t.Fatal(err)
			}
			state := filepath.Join(dir, "state")
			start := exec.CommandContext(ctx, binary, "--background", "--workspace", dir, "--state-dir", state, "--backend", tc.kind, "--"+tc.kind, fake)
			if out, err := start.CombinedOutput(); err != nil {
				t.Fatalf("start: %v: %s", err, out)
			}
			defer exec.CommandContext(ctx, binary, "--stop", "--state-dir", state).Run()
			attach := exec.CommandContext(ctx, binary, "--attach", "--state-dir", state, "--backend", tc.attachKind)
			stdin, err := attach.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			stdout, err := attach.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := attach.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { attach.Process.Kill(); attach.Wait() }()
			line, err := bufio.NewReader(stdout).ReadBytes('\n')
			if err != nil {
				t.Fatal(err)
			}
			var ready map[string]json.RawMessage
			if err := json.Unmarshal(line, &ready); err != nil || string(ready["backend"]) != `"`+tc.kind+`"` || string(ready["capabilities"]) != tc.capabilities || string(ready["ready"]) != `true` {
				t.Fatalf("ready=%s err=%v", line, err)
			}
		})
	}
}
