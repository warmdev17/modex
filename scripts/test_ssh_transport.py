"""Real loopback OpenSSH test with temporary keys and no system config changes."""
import getpass
import json
import os
from pathlib import Path
import select
import shutil
import socket
import subprocess
import tempfile
import time
import unittest

CLIENT = os.environ.get("MODEX_CLIENT_BINARY")
AGY = os.environ.get("MODEX_TEST_AGY")


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
        agy = root / "fake-agy"
        port = command[command.index("--port") + 1]
        agy.write_text("#!/usr/bin/env python3\n" + f"expected_port = {port!r}\n" + """import json, os, sys
connection = os.environ.get("SSH_CONNECTION", "").split()
authenticated = (len(connection) == 4 and connection[0] == connection[2] == "127.0.0.1"
                 and connection[1].isdigit() and connection[3] == expected_port)
if "-p" in sys.argv:
    print(json.dumps({"status": "SUCCESS" if authenticated else "MISSING_SSH_CONTEXT"}))
    sys.exit(0)
for line in sys.stdin:
    request = json.loads(line)
    result = {}
    method = request.get("method")
    if method == "initialize":
        result = {"protocolVersion": 1, "agentInfo": {"name": "Antigravity ACP Server", "version": "1"},
                  "agentCapabilities": {"loadSession": True, "sessionCapabilities": {"list": {}}}}
    if method == "session/new":
        if not authenticated:
            print(json.dumps({"jsonrpc":"2.0","id":request["id"],
                              "error":{"code":-32000,"message":"Missing SSH context"}}), flush=True)
            continue
        result = {"sessionId": "ssh-test-session"}
    print(json.dumps({"jsonrpc":"2.0","id":request["id"],"result":result}), flush=True)
""")
        agy.chmod(0o700)
        base = [str(host_binary), "--state-dir", str(state)]
        service_env = dict(os.environ)
        service_env.pop("SSH_CONNECTION", None)
        started = subprocess.run(base + ["--background", "--workspace", str(root), "--codex", str(fake),
                                         "--agy", AGY or str(agy), "--opencode", str(root / "missing-opencode")],
                                 env=service_env, capture_output=True, text=True, timeout=40)
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
                    self.assertNotIn("ssh_connection", json.dumps(ack))
                    if instance:
                        self.assertEqual(ack["instance"], instance)
                    instance = ack["instance"]
                    events = []
                    def receive(timeout=10):
                        nonlocal cursor
                        frame = frames.get(timeout=timeout)
                        self.assertNotIn("ssh_connection", json.dumps(frame))
                        self.assertEqual(frame["seq"], cursor + 1)
                        cursor = frame["seq"]
                        return frame["message"]

                    def call(identifier, method, params=None):
                        process.stdin.write(json.dumps({"id": identifier, "method": method, "params": params or {}}) + "\n")
                        process.stdin.flush()
                        while True:
                            message = receive()
                            if message.get("id") == identifier:
                                self.assertIn("result", message, f"{method} was rejected")
                                return message["result"]
                            events.append(message)
                    self.assertEqual(call(request_id, "status")["config"]["provider"], "fixture")
                    if request_id == 10:
                        agents = call(20, "agents/list")["data"]
                        self.assertTrue(next(agent for agent in agents if agent["id"] == "antigravity")["ready"],
                                        "AGY readiness did not inherit the genuine attach context")
                        chat = call(21, "threads/start", {"agent": "antigravity"})["thread"]
                        self.assertEqual(chat["agent"], "antigravity")
                        if AGY:
                            target = {"thread_id": chat["id"], "workspace": str(root)}
                            call(22, "threads/options/set", dict(target, id="model", value="gemini-4-hydrogen"))
                            call(23, "threads/options/set", dict(target, id="thought_level", value="medium"))
                            marker = "MODEX_AGY_SSH_CONTEXT_OK"
                            turn = call(24, "turn/start", dict(target, text=f"Reply exactly {marker}. Do not use tools."))["turn"]["id"]
                            answer = []
                            deadline = time.monotonic() + 45
                            pending = iter(events)
                            while True:
                                message = next(pending, None)
                                if message is None:
                                    message = receive(max(.01, deadline - time.monotonic()))
                                params = message.get("params", {})
                                if params.get("threadId") != chat["id"]:
                                    continue
                                if message.get("event") == "item/agentMessage/delta":
                                    answer.append(params["delta"])
                                if message.get("event") == "turn/completed" and params["turn"]["id"] == turn:
                                    self.assertEqual(params["turn"]["status"], "completed")
                                    break
                            self.assertIn(marker, "".join(answer), "native AGY did not answer through the real SSH attachment")
                            if os.environ.get("MODEX_TEST_SHARED_CLI"):
                                self.check_native_shared_cli(base, root, chat["id"], receive, call)
                    process.stdin.close()
                    self.assertEqual(process.wait(timeout=5), 0)
                finally:
                    if not process.stdin.closed:
                        process.stdin.close()
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
            while (state / "endpoint.json").exists() and time.monotonic() < deadline:
                time.sleep(.02)
            self.assertFalse((state / "endpoint.json").exists())
            self.assertEqual((state / "owner.lock").read_text(), "modex-flock-v1\n")

    def check_native_shared_cli(self, base, workspace, thread_id, receive, call):
        cli = subprocess.Popen(base + ["--cli", "--workspace", str(workspace), "--thread", thread_id],
                               stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        output = bytearray()

        def wait_text(text, count=1):
            deadline = time.monotonic() + 40
            while output.count(text) < count:
                remaining = max(0, deadline - time.monotonic())
                self.assertTrue(select.select([cli.stdout], [], [], remaining)[0], f"CLI did not show {text!r}")
                chunk = os.read(cli.stdout.fileno(), 8192)
                self.assertTrue(chunk, "CLI exited")
                output.extend(chunk)

        try:
            wait_text(b"Antigravity")
            target = {"thread_id": thread_id, "workspace": str(workspace)}
            phone_marker = "MODEX_PHONE_TO_CLI_OK"
            call(25, "turn/start", dict(target, text=f"Reply exactly {phone_marker}. Do not use tools."))
            wait_text(phone_marker.encode(), 2)  # User message and AGY response.
            wait_text(b"Turn completed.")
            cli_marker = "MODEX_CLI_TO_PHONE_OK"
            cli.stdin.write(f"Reply exactly {cli_marker}. Do not use tools.\n".encode())
            cli.stdin.flush()
            reply = []
            deadline = time.monotonic() + 40
            while cli_marker not in "".join(reply):
                message = receive(max(.01, deadline - time.monotonic()))
                params = message.get("params", {})
                if params.get("threadId") == thread_id and message.get("event") == "item/agentMessage/delta":
                    reply.append(params["delta"])
            cli.stdin.write(b"/quit\n")
            cli.stdin.flush()
            self.assertEqual(cli.wait(timeout=5), 0)
            self.assertIn("config", call(26, "status"))
        finally:
            cli.stdin.close()
            if cli.poll() is None:
                cli.terminate()
                cli.wait(timeout=5)
            cli.stdout.close()


if __name__ == "__main__":
    unittest.main()
