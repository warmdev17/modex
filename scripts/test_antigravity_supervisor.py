"""Opt-in Antigravity turn, reconnect and history through the Modex supervisor."""

import json
import os
from pathlib import Path
import secrets
import struct
import subprocess
import tempfile
import time
import unittest
import zlib

from live_supervisor_smoke import AttachedRPC


HOST = os.environ.get("MODEX_HOST_BINARY")
AGY = os.environ.get("MODEX_TEST_AGY")
MARKER = "MODEX_AGY_RECONNECT_OK"


def red_png():
    def chunk(kind, data):
        return struct.pack("!I", len(data)) + kind + data + struct.pack("!I", zlib.crc32(kind + data))
    scanlines = (b"\x00" + b"\xff\x00\x00" * 16) * 16
    return (b"\x89PNG\r\n\x1a\n" +
            chunk(b"IHDR", struct.pack("!2I5B", 16, 16, 8, 2, 0, 0, 0)) +
            chunk(b"IDAT", zlib.compress(scanlines)) + chunk(b"IEND", b""))


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

    def test_antigravity_attachments_reach_model(self):
        with tempfile.TemporaryDirectory(prefix="modex-agy-attachments-") as directory:
            root = Path(directory)
            workspace, state = root / "workspace", root / "state"
            workspace.mkdir()
            image, note = workspace / "sample image.png", workspace / "sample note.txt"
            image.write_bytes(red_png())
            marker = "MODEX-" + secrets.token_hex(4).upper()
            note.write_text(marker, encoding="utf-8")
            host = str(Path(HOST).resolve())
            control = [host, "--state-dir", str(state)]
            launch = control + ["--background", "--workspace", str(workspace), "--agy", AGY]
            self.assertEqual(subprocess.run(launch, capture_output=True, timeout=40).returncode, 0)
            client = None
            try:
                client = AttachedRPC(host, state, workspace)
                self.assertIn("antigravity_attachments", client.capabilities)
                self.assertIn("antigravity", {entry["id"] for entry in client.call("agents/list")["data"] if entry["ready"]})
                thread = client.call("threads/start", {"agent": "antigravity", "workspace": str(workspace)})["thread"]
                target = {"thread_id": thread["id"], "workspace": str(workspace)}
                ack = client.call("turn/start", dict(target,
                    text="Without tools, reply with the dominant basic color in the attached image and the exact token in the attached note.",
                    attachments=[{"path": str(image), "kind": "image"}, {"path": str(note), "kind": "file"}]))
                answer, user_files = [], []
                deadline = time.monotonic() + 120
                while True:
                    frame = client.events.popleft() if client.events else client.receive(deadline)
                    params = frame.get("params", {})
                    if params.get("threadId") != thread["id"]:
                        continue
                    if frame.get("event") == "item/completed" and params.get("item", {}).get("type") == "userMessage":
                        user_files = params["item"].get("attachments", [])
                        self.assertNotIn(marker, json.dumps(params))
                    if frame.get("event") == "item/agentMessage/delta":
                        answer.append(params["delta"])
                    if frame.get("event") == "turn/completed":
                        self.assertEqual(params["turn"]["id"], ack["turn"]["id"])
                        self.assertEqual(params["turn"]["status"], "completed")
                        break
                self.assertIn("red", "".join(answer).lower())
                self.assertIn(marker, "".join(answer))
                self.assertEqual([(file["path"], file["kind"]) for file in user_files],
                                 [(str(image), "image"), (str(note), "file")])
                history = client.call("threads/read", dict(target, include_turns=True, limit=20))
                self.assertEqual(next(item for item in history["messages"] if item["role"] == "user")["attachments"], user_files)
            finally:
                if client is not None:
                    client.close()
                self.assertEqual(subprocess.run(control + ["--stop"], capture_output=True, timeout=10).returncode, 0)
