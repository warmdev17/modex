"""Real loopback OpenSSH test with temporary keys and no system config changes."""
import getpass
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time
import unittest

CLIENT = os.environ.get("MODEX_CLIENT_BINARY")


@unittest.skipUnless(CLIENT and shutil.which("sshd") and os.name == "posix", "requires built client and Unix OpenSSH server")
class SSHTransportTests(unittest.TestCase):
    def test_real_ssh_quotes_paths_and_rejects_untrusted_host(self):
        with tempfile.TemporaryDirectory(prefix="modex-ssh-test-") as directory:
            root = Path(directory)
            for key in ("host", "client", "wrong-host"):
                subprocess.run(["ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", str(root / key)], check=True, capture_output=True)
            authorized = root / "authorized_keys"
            authorized.write_text((root / "client.pub").read_text())
            with socket.socket() as reserve:
                reserve.bind(("127.0.0.1", 0))
                port = reserve.getsockname()[1]
            config = root / "sshd_config"
            config.write_text(f'''Port {port}
ListenAddress 127.0.0.1
HostKey {root / 'host'}
PidFile {root / 'sshd.pid'}
AuthorizedKeysFile {authorized}
StrictModes no
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
UsePAM no
PermitRootLogin prohibit-password
AllowUsers {getpass.getuser()}
''')
            server = subprocess.Popen([shutil.which("sshd"), "-D", "-e", "-f", str(config)], stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
            try:
                deadline = time.monotonic() + 5
                while True:
                    if server.poll() is not None:
                        self.fail("temporary sshd failed: " + server.stderr.read().decode()[:1000])
                    try:
                        with socket.create_connection(("127.0.0.1", port), timeout=.1):
                            break
                    except OSError:
                        if time.monotonic() > deadline:
                            self.fail("sshd startup timeout")
                        time.sleep(.05)
                known = root / "known_hosts"
                host_key = (root / "host.pub").read_text().split()
                known.write_text(f"[127.0.0.1]:{port} {host_key[0]} {host_key[1]}\n")
                # Executable and state paths include shell metacharacters intentionally.
                remote = root / "host ' $(touch SHOULD_NOT_EXIST)"
                remote.write_text("#!/usr/bin/env python3\nimport sys,json\nprint(json.dumps({'argv':sys.argv[1:]}),flush=True)\nfor line in sys.stdin: print(line.strip(),flush=True)\n")
                remote.chmod(0o700)
                state = str(root / "state ' $(id)")
                command = [str(Path(CLIENT).resolve()), "--target", getpass.getuser() + "@127.0.0.1", "--port", str(port), "--identity", str(root / "client"), "--known-hosts", str(known), "--host-binary", str(remote), "--state-dir", state, "--cursor", "7", "--instance", "test-instance"]
                result = subprocess.run(command, input='{"id":1,"method":"status"}\n', text=True, capture_output=True, timeout=15)
                self.assertEqual(result.returncode, 0, result.stderr)
                lines = result.stdout.splitlines()
                self.assertEqual(json.loads(lines[0])["argv"], ["--attach", "--state-dir", state, "--cursor", "7", "--instance", "test-instance"])
                self.assertEqual(json.loads(lines[1]), {"id": 1, "method": "status"})
                self.assertFalse((Path.home() / "SHOULD_NOT_EXIST").exists())
                host_binary = os.environ.get("MODEX_HOST_BINARY")
                if host_binary:
                    self.check_supervisor(command, root, Path(host_binary).resolve())
                wrong = (root / "wrong-host.pub").read_text().split()
                known.write_text(f"[127.0.0.1]:{port} {wrong[0]} {wrong[1]}\n")
                rejected = subprocess.run(command, input="", text=True, capture_output=True, timeout=15)
                self.assertNotEqual(rejected.returncode, 0)
                self.assertEqual(rejected.stdout, "")
            finally:
                server.terminate()
                server.wait(timeout=5)
                server.stderr.close()


    def check_supervisor(self, command, root, host_binary):
        state = root / "supervisor-state"
        fake = root / "fake-codex"
        fake.write_text("#!/usr/bin/env python3\n" + """import sys,json
for line in sys.stdin:
 r=json.loads(line)
 if r.get('method')=='initialized':continue
 result={'config':{'model':'fixture','model_provider':'fixture'}} if r.get('method')=='config/read' else {}
 print(json.dumps({'id':r['id'],'result':result}),flush=True)
""")
        fake.chmod(0o700)
        base = [str(host_binary), "--state-dir", str(state)]
        started = subprocess.run(base + ["--background", "--workspace", str(root), "--codex", str(fake)], capture_output=True, text=True, timeout=40)
        self.assertEqual(started.returncode, 0, started.stderr)
        try:
            remote_command = command[:]
            remote_command[remote_command.index("--host-binary") + 1] = str(host_binary)
            remote_command[remote_command.index("--state-dir") + 1] = str(state)
            remote_command[remote_command.index("--cursor") + 1] = "0"
            del remote_command[remote_command.index("--instance"):]
            instance = None
            cursor = 0
            for request_id in (10, 11):
                connect = remote_command[:]
                if instance:
                    connect[connect.index("--cursor") + 1] = str(cursor)
                    connect += ["--instance", instance]
                process = subprocess.Popen(connect, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                try:
                    import queue
                    import threading
                    frames = queue.Queue()
                    def read_frames():
                        for line in process.stdout:
                            frames.put(json.loads(line))
                    worker = threading.Thread(target=read_frames, daemon=True)
                    worker.start()
                    ack = frames.get(timeout=10)
                    self.assertTrue(ack["ready"])
                    if instance:
                        self.assertEqual(ack["instance"], instance)
                    instance = ack["instance"]
                    process.stdin.write(json.dumps({"id": request_id, "method": "status"}) + "\n")
                    process.stdin.flush()
                    while True:
                        frame = frames.get(timeout=10)
                        self.assertEqual(frame["seq"], cursor + 1)
                        cursor = frame["seq"]
                        if frame["message"].get("id") == request_id:
                            self.assertEqual(frame["message"]["result"]["config"]["provider"], "fixture")
                            break
                    process.stdin.close()
                    self.assertEqual(process.wait(timeout=5), 0)
                finally:
                    if process.poll() is None:
                        process.kill()
                        process.wait()
                    worker.join(timeout=2)
                    process.stdout.close()
                    process.stderr.close()
                time.sleep(.1)
        finally:
            stopped = subprocess.run(base + ["--stop"], capture_output=True, timeout=10)
            self.assertEqual(stopped.returncode, 0)
            deadline = time.monotonic() + 5
            while (state / "owner.lock").exists() and time.monotonic() < deadline:
                time.sleep(.02)
            self.assertFalse((state / "owner.lock").exists())


if __name__ == "__main__":
    unittest.main()
