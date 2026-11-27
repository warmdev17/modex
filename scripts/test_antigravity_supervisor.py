"""Opt-in Antigravity turn, reconnect and history through the Modex supervisor."""

import os
from pathlib import Path
import subprocess
import tempfile
import time
import unittest

from live_supervisor_smoke import AttachedRPC


HOST = os.environ.get("MODEX_HOST_BINARY")
AGY = os.environ.get("MODEX_TEST_AGY")
MARKER = "MODEX_AGY_RECONNECT_OK"


@unittest.skipUnless(HOST and AGY, "requires a built host and authenticated Antigravity CLI")
class AntigravitySupervisorTests(unittest.TestCase):
    def test_antigravity_conversation_survives_restart(self):
        with tempfile.TemporaryDirectory(prefix="modex-agy-supervisor-") as directory:
            root = Path(directory)
            workspace = root / "workspace"
            workspace.mkdir()
            state = root / "state"
            control = [str(Path(HOST).resolve()), "--state-dir", str(state)]
            launch = control + ["--background", "--workspace", str(workspace), "--agy", AGY]

            def start():
                result = subprocess.run(launch, capture_output=True, timeout=40)
                self.assertEqual(result.returncode, 0, "isolated supervisor did not start")

            def stop():
                result = subprocess.run(control + ["--stop"], capture_output=True, timeout=10)
                self.assertEqual(result.returncode, 0, "isolated supervisor did not stop")
                deadline = time.monotonic() + 8
                while (state / "endpoint.json").exists() and time.monotonic() < deadline:
                    time.sleep(.05)
                self.assertFalse((state / "endpoint.json").exists())

            client = None
            started = False
            try:
                start()
                started = True
                client = AttachedRPC(control[0], state, workspace)
                runtimes = client.call("agents/list")["data"]
                self.assertIn("antigravity", {item["id"] for item in runtimes if item["ready"]})
                thread = client.call("threads/start", {"agent": "antigravity", "workspace": str(workspace)})["thread"]
                self.assertEqual(thread["agent"], "antigravity")
                self.assertTrue(thread["id"].startswith("agy:"))
                target = {"thread_id": thread["id"], "workspace": str(workspace)}
                ack = client.call("turn/start", dict(target, text=f"Reply exactly {MARKER}. Do not use tools."))
                cursor, instance = client.cursor, client.instance
                before_disconnect = list(client.events)
                client.close()
                client = AttachedRPC(control[0], state, workspace, cursor, instance)
                client.events.extend(before_disconnect)
                stop_reason = ""
                received = []
                deadline = time.monotonic() + 120
                while not stop_reason:
                    frame = client.events.popleft() if client.events else client.receive(deadline)
                    params = frame.get("params") or {}
                    if params.get("threadId") != thread["id"]:
                        continue
                    if frame["event"] == "item/agentMessage/delta":
                        received.append(params["delta"])
                    if frame["event"] == "turn/completed":
                        self.assertEqual(params["turn"]["id"], ack["turn"]["id"])
                        stop_reason = params["turn"]["status"]
                self.assertEqual(stop_reason, "completed")
                self.assertIn(MARKER, "".join(received))
                client.close()
                client = None
                stop()
                started = False

                start()
                started = True
                client = AttachedRPC(control[0], state, workspace)
                self.assertIn(thread["id"], {entry["id"] for entry in client.call("threads/list")["data"]})
                self.assertEqual(client.call("threads/resume", target)["thread"]["id"], thread["id"])
                history = client.call("threads/read", dict(target, include_turns=True, limit=20))
                self.assertTrue(history["history_revision"])
                self.assertTrue(any(item["role"] == "assistant" and MARKER in item["text"] for item in history["messages"]))
            finally:
                if client is not None:
                    client.close()
                if started:
                    stop()
