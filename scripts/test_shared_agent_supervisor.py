"""Opt-in mixed-runtime supervisor test with synthetic local runtimes."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest

from live_supervisor_smoke import AttachedRPC


HOST = os.environ.get("MODEX_HOST_BINARY")

FIXTURE = r"""
import http.server
import json
import os
from pathlib import Path
import sys
import time

kind = Path(sys.argv[0]).name
workspace = str(Path.cwd())
if kind == "opencode":
    if "--version" in sys.argv:
        print("opencode v2.4.1")
        sys.exit(0)
    stored = Path(os.environ["XDG_DATA_HOME"]) / "session-fixture.json"
    prompt_file = stored.with_name("prompt-fixture.txt")
    waiting = stored.with_name("waiting")
    stopped = stored.with_name("stopped")

    class Handler(http.server.BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def send_json(self, value):
            data = json.dumps(value).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            path = self.path.split("?", 1)[0]
            if path == "/api/event":
                self.send_response(200)
                self.end_headers()
                try:
                    while True:
                        self.wfile.write(b": keepalive\n\n")
                        self.wfile.flush()
                        time.sleep(.2)
                except OSError:
                    pass
            elif path == "/api/info":
                self.send_json({"version": "2.4.1"})
            elif path == "/api/config":
                self.send_json([])
            elif path == "/api/model":
                self.send_json({"data": [{"id": "fixture", "providerID": "local-fixture"}]})
            elif path == "/api/session/oc-native":
                session = json.loads(stored.read_text())
                if prompt_file.exists():
                    session.update(outcome="succeeded", time={"busy": 1, "idle": 3})
                if waiting.exists():
                    session.update(outcome=None, time={"busy": 4, "idle": 3})
                if stopped.exists():
                    session.update(outcome="interrupted", time={"busy": 4, "idle": 6})
                self.send_json({"data": session})
            elif path in ("/api/session/oc-native/permission", "/api/session/oc-native/form"):
                self.send_json({"data": []})
            elif path == "/api/session/oc-native/message":
                messages = []
                if prompt_file.exists():
                    messages = [
                        {"id": "idle", "type": "idle", "outcome": "succeeded"},
                        {"id": "answer", "type": "assistant", "content": [
                            {"type": "text", "text": "OpenCode reply fixture"}],
                         "time": {"created": 2, "completed": 3}},
                        {"id": "prompt", "type": "user", "text": prompt_file.read_text(),
                         "time": {"created": 1}},
                    ]
                if waiting.exists() or stopped.exists():
                    messages.insert(0, {"id": "prompt-stop", "type": "user", "text": "Wait for Stop",
                                        "time": {"created": 4}})
                if stopped.exists():
                    messages.insert(0, {"id": "idle-stop", "type": "idle", "outcome": "interrupted",
                                        "time": {"created": 6}})
                self.send_json({"data": messages})
            else:
                self.send_error(404)

        def do_POST(self):
            if self.path == "/api/session/oc-native/prompt":
                prompt = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                if prompt["text"] == "Wait for Stop":
                    waiting.touch()
                    prompt_id = "prompt-stop"
                else:
                    prompt_file.write_text(prompt["text"])
                    prompt_id = "prompt"
                self.send_json({"data": {"id": prompt_id, "type": "user"}})
                return
            if self.path == "/api/session/oc-native/interrupt":
                interrupted = waiting.exists()
                if interrupted:
                    waiting.rename(stopped)
                self.send_json({"interrupted": interrupted})
                return
            if self.path != "/api/session":
                self.send_error(404)
                return
            request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            session = {"id": "oc-native", "model": request.get("model", {}),
                       "title": "OpenCode fixture", "location": request["location"]}
            stored.write_text(json.dumps(session))
            self.send_json({"data": session})

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    print(f"server listening on http://127.0.0.1:{server.server_port}", flush=True)
    server.serve_forever()
elif kind == "agy" and "-p" in sys.argv:
    print('{"status":"SUCCESS"}')
else:
    thread = {"id": "codex-fixture", "cwd": workspace, "name": "Codex fixture",
              "projectId": "shared-project", "turns": []}
    agy_prompt = None
    edit_record = Path(workspace) / ".edit-fixture"

    def edit_update(update, status):
        value = {"sessionUpdate": update, "toolCallId": "edit-1", "kind": "edit",
                 "status": status, "content": [
                     {"type": "diff", "path": str(Path(workspace) / "sample.txt"),
                      "oldText": "same\nbefore\n", "newText": "same\nafter\n"},
                 ]}
        print(json.dumps({"jsonrpc": "2.0", "method": "session/update",
                          "params": {"sessionId": "agy-native", "update": value}}), flush=True)

    for line in sys.stdin:
        request = json.loads(line)
        method = request.get("method")
        if kind == "agy" and method == "session/cancel" and agy_prompt is not None:
            print(json.dumps({"jsonrpc": "2.0", "id": agy_prompt,
                              "result": {"stopReason": "cancelled"}}), flush=True)
            agy_prompt = None
            continue
        if "id" not in request:
            continue
        if kind == "agy":
            if method == "session/prompt":
                if request["params"]["prompt"][0]["text"] != "Show edit fixture":
                    agy_prompt = request["id"]
                    continue
                edit_record.touch()
                edit_update("tool_call", "pending")
                edit_update("tool_call_update", "completed")
                edit_update("tool_call_update", "completed")
                result = {"stopReason": "end_turn"}
            else:
                if method == "session/load" and edit_record.exists():
                    edit_update("tool_call", "completed")
                result = {
                    "initialize": {"protocolVersion": 1, "agentInfo": {
                        "name": "Antigravity ACP Server", "version": "1"},
                        "agentCapabilities": {"loadSession": True, "sessionCapabilities": {"list": {}}}},
                    "session/new": {"sessionId": "agy-native"},
                    "session/list": {"sessions": [{"sessionId": "agy-native", "cwd": workspace}]},
                }.get(method, {})
        else:
            result = {
                "config/read": {"config": {"model": "fixture", "model_provider": "fixture"}},
                "project/list": {"data": [{"id": "shared-project", "name": "Shared fixture",
                                          "roots": [{"path": workspace}]}]},
                "thread/start": {"thread": thread},
                "thread/list": {"data": [thread]},
                "thread/read": {"thread": thread},
            }.get(method, {})
        print(json.dumps({"jsonrpc": "2.0", "id": request["id"], "result": result}), flush=True)
"""


@unittest.skipUnless(HOST and os.name == "posix", "requires a built Unix modex-host")
class SharedAgentSupervisorTests(unittest.TestCase):
    def test_chat_identity_and_restart(self):
        with tempfile.TemporaryDirectory(prefix="modex-shared-agent-smoke-") as directory:
            root = Path(directory)
            workspace, state = root / "workspace", root / "state"
            workspace.mkdir()
            host = str(Path(HOST).resolve())
            control = [host, "--state-dir", str(state)]
            launch = control + ["--background", "--workspace", str(workspace)]
            for runtime in ("codex", "opencode", "agy"):
                binary = root / runtime
                binary.write_text(f"#!{sys.executable}\n{FIXTURE}")
                binary.chmod(0o700)
                launch += [f"--{runtime}", str(binary)]

            def start():
                result = subprocess.run(launch, capture_output=True, text=True, timeout=30)
                self.assertEqual(result.returncode, 0, result.stderr)
                return AttachedRPC(host, state, workspace)

            def stop():
                self.assertEqual(subprocess.run(control + ["--stop"], capture_output=True, timeout=10).returncode, 0)
                deadline = time.monotonic() + 5
                while (state / "endpoint.json").exists() and time.monotonic() < deadline:
                    time.sleep(.03)
                self.assertFalse((state / "endpoint.json").exists())

            client = None
            try:
                client = start()
                self.assertIn("runtime_routing", client.capabilities)
                ready = client.call("agents/list", {"workspace": str(workspace)})["data"]
                self.assertEqual({r["id"] for r in ready if r["ready"]},
                                 {"codex", "opencode", "antigravity"})
                models = client.call("models/list", {"agent": "opencode", "workspace": str(workspace)})["data"]
                self.assertEqual([m["model"] for m in models], ["local-fixture/fixture"])
                project = {"workspace": str(workspace), "project_id": "shared-project"}
                chats = {}
                for agent in ("codex", "opencode", "antigravity"):
                    params = dict(project, agent=agent)
                    if agent == "opencode":
                        params["model"] = "local-fixture/fixture"
                    chat = client.call("threads/start", params)["thread"]
                    self.assertEqual(chat["agent"], agent)
                    self.assertEqual(chat["projectId"], "shared-project")
                    chats[agent] = chat["id"]
                self.assertEqual(len(set(chats.values())), 3)
                for agent, expected in chats.items():
                    page = client.call("threads/list", {"agent": agent, "all_workspaces": True})
                    self.assertEqual([entry["id"] for entry in page["data"]], [expected])
                legacy = client.call("threads/list", {"all_workspaces": True})
                self.assertEqual({entry["id"] for entry in legacy["data"]},
                                 {chats["codex"], chats["antigravity"]})
                target = {"thread_id": chats["opencode"], "workspace": str(workspace)}
                agy_target = {"thread_id": chats["antigravity"], "workspace": str(workspace)}
                agy_turn = client.call("turn/start", dict(agy_target, text="Wait fixture"))["turn"]["id"]
                oc_turn = client.call("turn/start", dict(target, text="OpenCode restart fixture"))["turn"]["id"]
                with self.assertRaisesRegex(RuntimeError, "request_rejected:turn/interrupt"):
                    client.call("turn/interrupt", dict(agy_target, turn_id=oc_turn))
                client.call("turn/interrupt", dict(agy_target, turn_id=agy_turn))
                completions = {}
                deadline = time.monotonic() + 10
                while len(completions) < 2:
                    frame = client.events.popleft() if client.events else client.receive(deadline)
                    params = frame.get("params") or {}
                    if frame.get("event") == "turn/completed":
                        completions[params["threadId"]] = params["turn"]["id"]
                self.assertEqual(completions, {chats["opencode"]: oc_turn, chats["antigravity"]: agy_turn})
                edit_turn = client.call("turn/start", dict(agy_target, text="Show edit fixture"))["turn"]["id"]
                cards = []
                deadline = time.monotonic() + 10
                while True:
                    frame = client.events.popleft() if client.events else client.receive(deadline)
                    params = frame.get("params") or {}
                    if params.get("threadId") != chats["antigravity"]:
                        continue
                    item = params.get("item") or {}
                    if item.get("type") == "fileChange":
                        self.assertEqual(frame["event"], "item/completed")
                        self.assertEqual(params["turnId"], edit_turn)
                        cards.append(item)
                    if frame.get("event") == "turn/completed" and params["turn"]["id"] == edit_turn:
                        self.assertEqual(params["turn"]["status"], "completed")
                        break
                self.assertEqual(len(cards), 1, "expected one completed edit card")
                self.assertEqual(cards[0]["status"], "completed")
                changes = cards[0]["changes"]
                self.assertEqual(len(changes), 1)
                self.assertEqual(Path(changes[0]["path"]).name, "sample.txt")
                self.assertIn("-before", changes[0]["diff"])
                self.assertIn("+after", changes[0]["diff"])
                target["include_turns"] = True
                client.close()
                client = None
                stop()
                client = start()
                agy_history = client.call("threads/read", dict(agy_target, include_turns=True, limit=10))
                replay = [row for row in agy_history["messages"] if row["role"] == "file_change"]
                self.assertEqual(len(replay), 1)
                self.assertEqual(replay[0]["activity_item"]["changes"], changes)
                history = client.call("threads/read", target)
                self.assertEqual(history["thread"]["id"], chats["opencode"])
                self.assertTrue(any(m["text"] == "OpenCode restart fixture" for m in history["messages"]))
                target.pop("include_turns")
                wait_turn = client.call("turn/start", dict(target, text="Wait for Stop"))["turn"]["id"]
                with self.assertRaisesRegex(RuntimeError, "request_rejected:turn/interrupt"):
                    client.call("turn/interrupt", dict(target, turn_id=oc_turn))
                client.call("turn/interrupt", dict(target, turn_id=wait_turn))
                deadline = time.monotonic() + 10
                while True:
                    frame = client.events.popleft() if client.events else client.receive(deadline)
                    params = frame.get("params") or {}
                    if frame.get("event") == "turn/completed" and params.get("threadId") == chats["opencode"]:
                        self.assertEqual(params["turn"]["id"], wait_turn)
                        self.assertEqual(params["turn"]["status"], "interrupted")
                        break
                outside = root / "outside"
                outside.mkdir()
                with self.assertRaisesRegex(RuntimeError, "request_rejected:threads/read"):
                    client.call("threads/read", dict(target, workspace=str(outside)))
            finally:
                if client is not None:
                    client.close()
                if (state / "endpoint.json").exists():
                    stop()
