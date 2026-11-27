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
    def test_antigravity_followup_while_streaming(self):
        with tempfile.TemporaryDirectory(prefix="modex-agy-steer-") as directory:
            workspace = Path(directory) / "workspace"
            workspace.mkdir()
            state = Path(directory) / "state"
            host = str(Path(HOST).resolve())
            control = [host, "--state-dir", str(state)]
            launch = control + ["--background", "--workspace", str(workspace), "--agy", AGY]
            self.assertEqual(subprocess.run(launch, capture_output=True, timeout=40).returncode, 0)
            client = None
            try:
                client = AttachedRPC(host, state, workspace)
                self.assertIn("antigravity_steer", client.capabilities)
                runtimes = {item["id"]: item for item in client.call("agents/list")["data"]}
                self.assertIn("antigravity", runtimes)
                self.assertTrue(runtimes["antigravity"]["ready"], runtimes["antigravity"].get("reason"))
                thread = client.call("threads/start", {"agent": "antigravity", "workspace": str(workspace)})["thread"]["id"]
                target = {"thread_id": thread, "workspace": str(workspace)}
                turn = client.call("turn/start", dict(target,
                    text="Without tools, list 120 different everyday objects and describe each in one sentence."))["turn"]["id"]
                marker = "STEER-" + secrets.token_hex(4).upper()
                followup = f"Without tools, reply with exactly {marker} instead of continuing the list."
                deadline = time.monotonic() + 120
                sent, echo, starts = False, 0, 0
                output = []
                while True:
                    frame = client.events.popleft() if client.events else client.receive(deadline)
                    params = frame.get("params", {})
                    if params.get("threadId") != thread:
                        continue
                    event = frame.get("event")
                    if event == "turn/started":
                        starts += 1
                    elif event == "item/agentMessage/delta":
                        self.assertEqual(params["turnId"], turn)
                        if sent:
                            output.append(params["delta"])
                        else:
                            ack = client.call("turn/steer", dict(target, turn_id=turn, text=followup))
                            self.assertEqual(ack["turnId"], turn)
                            sent = True
                    elif event == "item/completed":
                        item = params.get("item", {})
                        if item.get("type") == "userMessage" and any(block.get("text") == followup for block in item["content"]):
                            echo += 1
                    elif event == "turn/completed":
                        self.assertEqual(params["turn"]["id"], turn)
                        self.assertEqual(params["turn"]["status"], "completed")
                        break
                self.assertEqual(starts, 1)
                self.assertEqual(echo, 1)
                self.assertIn(marker, "".join(output))
                history = client.call("threads/read", dict(target, include_turns=True))
                self.assertFalse(history["active_turn"])
                messages = history["messages"]
                indices = [i for i, item in enumerate(messages)
                           if item["role"] == "user" and item["text"] == followup]
                self.assertEqual(len(indices), 1)
                self.assertIn(marker, "".join(item["text"] for item in messages[indices[0] + 1:]
                                              if item["role"] == "assistant"))
            finally:
                if client is not None:
                    client.close()
                self.assertEqual(subprocess.run(control + ["--stop"], capture_output=True, timeout=10).returncode, 0)

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
                self.assertIn("antigravity_context_usage", client.capabilities)
                runtimes = client.call("agents/list")["data"]
                self.assertIn("antigravity", {item["id"] for item in runtimes if item["ready"]})
                thread = client.call("threads/start", {"agent": "antigravity", "workspace": str(workspace)})["thread"]
                self.assertEqual(thread["agent"], "antigravity")
                self.assertTrue(thread["id"].startswith("agy:"))
                target = {"thread_id": thread["id"], "workspace": str(workspace)}
                ack = client.call("turn/start", dict(target, text=f"For a paper kite description test, reply exactly {MARKER}. Do not use tools."))
                cursor, instance = client.cursor, client.instance
                before_disconnect = list(client.events)
                client.close()
                client = AttachedRPC(control[0], state, workspace, cursor, instance)
                client.events.extend(before_disconnect)
                stop_reason = ""
                title = ""
                latest_context = None
                received = []
                deadline = time.monotonic() + 120
                while not stop_reason or not title:
                    frame = client.events.popleft() if client.events else client.receive(deadline)
                    params = frame.get("params") or {}
                    if params.get("threadId") != thread["id"]:
                        continue
                    if frame["event"] == "item/agentMessage/delta":
                        received.append(params["delta"])
                    if frame["event"] == "thread/name/updated":
                        title = params["threadName"]
                    if frame["event"] == "thread/tokenUsage/updated":
                        latest_context = params["tokenUsage"]
                    if frame["event"] == "turn/completed":
                        self.assertEqual(params["turn"]["id"], ack["turn"]["id"])
                        stop_reason = params["turn"]["status"]
                        deadline = min(deadline, time.monotonic() + 15)
                self.assertEqual(stop_reason, "completed")
                self.assertIn(MARKER, "".join(received))
                self.assertTrue(title.strip())
                names = {entry["id"]: entry.get("name") for entry in client.call("threads/list")["data"]}
                self.assertEqual(names[thread["id"]], title)
                while client.events:
                    frame = client.events.popleft()
                    params = frame.get("params", {})
                    if frame.get("event") == "thread/tokenUsage/updated" and params.get("threadId") == thread["id"]:
                        latest_context = params["tokenUsage"]
                usage = client.call("threads/status", target)
                self.assertIsNotNone(latest_context, "missing native ACP context sample")
                self.assertGreater(latest_context["last"]["totalTokens"], 0)
                self.assertGreater(latest_context["modelContextWindow"], 0)
                self.assertEqual(usage["token_usage"]["last"]["totalTokens"], latest_context["last"]["totalTokens"])
                self.assertEqual(usage["token_usage"]["modelContextWindow"], latest_context["modelContextWindow"])
                self.assertIsNone(usage["rate_limits"])
                client.close()
                client = None
                stop()
                started = False

                start()
                started = True
                client = AttachedRPC(control[0], state, workspace)
                names = {entry["id"]: entry.get("name") for entry in client.call("threads/list")["data"]}
                self.assertEqual(names[thread["id"]], title)
                resumed = client.call("threads/resume", target)["thread"]
                self.assertEqual(resumed["id"], thread["id"])
                self.assertEqual(resumed["name"], title)
                history = client.call("threads/read", dict(target, include_turns=True, limit=20))
                self.assertTrue(history["history_revision"])
                self.assertEqual(history["thread"]["name"], title)
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
