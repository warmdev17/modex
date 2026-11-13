"""Black-box lifecycle tests; set MODEX_HOST_BINARY to a built modex-host."""
import json
import os
from pathlib import Path
import queue
import socket
import subprocess
import sys
import tempfile
import threading
import time
import unittest

BINARY = os.environ.get("MODEX_HOST_BINARY")


@unittest.skipUnless(BINARY and os.name == "posix", "requires built host and Unix private-state support")
class SupervisorLifecycleTests(unittest.TestCase):
    def test_failed_start_cleans_private_state(self):
        with tempfile.TemporaryDirectory(prefix="modex-failed-supervisor-") as directory:
            state = Path(directory) / "state"
            command = [str(Path(BINARY).resolve()), "--supervise", "--workspace", directory,
                       "--state-dir", str(state), "--codex", str(Path(directory) / "missing-codex")]
            failed = subprocess.run(command, capture_output=True, text=True, timeout=10)
            self.assertNotEqual(failed.returncode, 0)
            if (state / "owner.lock").exists():
                self.assertEqual((state / "owner.lock").read_text(), "modex-flock-v1\n")
            self.assertFalse((state / "endpoint.json").exists())

    def test_detach_replay_auth_exclusive_stop_and_cleanup(self):
        with tempfile.TemporaryDirectory(prefix="modex-supervisor-test-") as directory:
            root = Path(directory)
            state = root / "state"
            fake = root / "fake-codex"
            fake.write_text("#!" + sys.executable + "\n" + '''import json,sys,time,threading
__import__('pathlib').Path('fixture.pid').write_text(str(__import__('os').getpid()))
lock=threading.Lock()
def send(frame):
 with lock:
  print(json.dumps(frame),flush=True)
def delayed():
 time.sleep(.25)
 send({'method':'turn/completed','params':{'threadId':'one','turn':{'id':'turn','status':'completed'}}})
for line in sys.stdin:
 r=json.loads(line);m=r.get('method');i=r.get('id')
 if m=='initialized':continue
 if m=='initialize':result={}
 elif m=='config/read':result={'config':{'model':'fixture','model_provider':'fixture'}}
 elif m in ('thread/read','thread/resume'):result={'thread':{'id':'one','cwd':__import__('os').getcwd()}}
 elif m=='turn/start':
  result={'turn':{'id':'turn'}}
  threading.Thread(target=delayed,daemon=True).start()
 else:result={}
 send({'id':i,'result':result})
''')
            fake.chmod(0o700)
            base = [str(Path(BINARY).resolve()), "--state-dir", str(state)]
            launch = base + ["--background", "--workspace", str(root), "--codex", str(fake)]
            started = subprocess.run(launch, capture_output=True, text=True, timeout=40)
            self.assertEqual(started.returncode, 0, started.stderr)
            try:
                endpoint = json.loads((state / "endpoint.json").read_text())
                self.assertEqual((state / "endpoint.json").stat().st_mode & 0o777, 0o600)
                duplicate = subprocess.run(launch, capture_output=True, text=True, timeout=5)
                self.assertNotEqual(duplicate.returncode, 0)

                def connect(token=None, cursor=0, instance=None):
                    host, port = endpoint["address"].split(":")
                    sock = socket.create_connection((host, int(port)), timeout=3)
                    reader = sock.makefile("rb")
                    hello = {"token": token or endpoint["token"], "cursor": cursor,
                             "instance": instance or endpoint["instance"]}
                    sock.sendall(json.dumps(hello).encode() + b"\n")
                    return sock, reader, json.loads(reader.readline())

                bad, reader, ack = connect(token="invalid")
                self.assertIn("error", ack)
                reader.close()
                bad.close()
                first, reader, ack = connect()
                self.assertTrue(ack["ready"])
                second, other_reader, rejected = connect()
                self.assertIn("error", rejected)
                other_reader.close()
                second.close()
                first.sendall(json.dumps({"id": 1, "method": "turn/start", "params": {
                    "thread_id": "one", "text": "fixture"}}).encode() + b"\n")
                cursor = 0
                while True:
                    entry = json.loads(reader.readline())
                    cursor = entry["seq"]
                    if entry["message"].get("id") == 1:
                        self.assertIn("result", entry["message"])
                        break
                reader.close()
                first.close()
                time.sleep(.5)
                again, replay_reader, ack = connect(cursor=cursor)
                self.assertTrue(ack["ready"])
                replay = json.loads(replay_reader.readline())
                self.assertGreater(replay["seq"], cursor)
                self.assertEqual(replay["message"]["event"], "turn/completed")
                replay_reader.close()
                again.close()

                # The CLI transport also attaches without printing its token.
                attached = subprocess.Popen(base + ["--attach", "--cursor", str(cursor),
                    "--instance", endpoint["instance"]], stdin=subprocess.PIPE,
                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                lines = queue.Queue()
                def read_lines():
                    for line in attached.stdout:
                        lines.put(json.loads(line))
                worker = threading.Thread(target=read_lines, daemon=True)
                worker.start()
                try:
                    self.assertTrue(lines.get(timeout=3)["ready"])
                    self.assertEqual(lines.get(timeout=3)["message"]["event"], "turn/completed")
                    attached.stdin.close()
                    self.assertEqual(attached.wait(timeout=5), 0)
                finally:
                    if attached.poll() is None:
                        attached.kill()
                        attached.wait()
                    worker.join(timeout=2)
                    attached.stdout.close()
                    attached.stderr.close()
            finally:
                stopped = subprocess.run(base + ["--stop"], capture_output=True, text=True, timeout=5)
                self.assertEqual(stopped.returncode, 0, stopped.stderr)
                deadline = time.monotonic() + 5
                while (state / "endpoint.json").exists() and time.monotonic() < deadline:
                    time.sleep(.02)
                self.assertEqual((state / "owner.lock").read_text(), "modex-flock-v1\n")
                self.assertFalse((state / "endpoint.json").exists())
                pid = int((root / "fixture.pid").read_text())
                deadline = time.monotonic() + 3
                alive = True
                while alive and time.monotonic() < deadline:
                    try:
                        os.kill(pid, 0)
                        time.sleep(.02)
                    except ProcessLookupError:
                        alive = False
                self.assertFalse(alive, "supervisor left an orphan app-server")


if __name__ == "__main__":
    unittest.main()
