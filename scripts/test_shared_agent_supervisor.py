"""Opt-in mixed-runtime supervisor test with synthetic local runtimes."""
import json
import os
from pathlib import Path
import queue
import subprocess
import sys
import tempfile
import threading
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

    def prompts():
        return json.loads(prompt_file.read_text()) if prompt_file.exists() else []

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
                count = len(prompts())
                if count:
                    session.update(outcome="succeeded", time={"busy": count * 3 - 2, "idle": count * 3})
                if waiting.exists():
                    session.update(outcome=None, time={"busy": 4, "idle": 3})
                if stopped.exists():
                    session.update(outcome="interrupted", time={"busy": 4, "idle": 6})
                self.send_json({"data": session})
            elif path in ("/api/session/oc-native/permission", "/api/session/oc-native/form"):
                self.send_json({"data": []})
            elif path == "/api/session/oc-native/message":
                messages = []
                for index, text in reversed(list(enumerate(prompts(), 1))):
                    messages.extend([
                        {"id": f"idle-{index}", "type": "idle", "outcome": "succeeded"},
                        {"id": f"answer-{index}", "type": "assistant", "content": [
                            {"type": "text", "text": "OpenCode reply: " + text}],
                         "time": {"created": index * 3 - 1, "completed": index * 3}},
                        {"id": f"prompt-{index}", "type": "user", "text": text,
                         "time": {"created": index * 3 - 2}},
                    ])
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
                stopped.unlink(missing_ok=True)
                if prompt["text"] == "Wait for Stop":
                    waiting.touch()
                    prompt_id = "prompt-stop"
                else:
                    history = prompts() + [prompt["text"]]
                    temporary = prompt_file.with_suffix(".pending")
                    temporary.write_text(json.dumps(history))
                    temporary.replace(prompt_file)
                    prompt_id = f"prompt-{len(history)}"
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
    agy_permission = None
    edit_record = Path(workspace) / ".edit-fixture"

    def edit_update(update, status):
        value = {"sessionUpdate": update, "toolCallId": "edit-1", "kind": "edit",
                 "status": status, "content": [
                     {"type": "diff", "path": str(Path(workspace) / "sample.txt"),
                      "oldText": "same\nbefore\n", "newText": "same\nafter\n"},
                 ]}
        print(json.dumps({"jsonrpc": "2.0", "method": "session/update",
                          "params": {"sessionId": "agy-native", "update": value}}), flush=True)

    def text_update(role, text):
        value = {"sessionUpdate": role + "_message_chunk", "content": {"type": "text", "text": text}}
        print(json.dumps({"jsonrpc": "2.0", "method": "session/update",
                          "params": {"sessionId": "agy-native", "update": value}}), flush=True)

    for line in sys.stdin:
        request = json.loads(line)
        method = request.get("method")
        if kind == "agy" and request.get("id") == "permission-fixture" and method is None:
            if request["result"]["outcome"].get("optionId") == "allow":
                text_update("agent", "Antigravity permission answered from the terminal.")
            print(json.dumps({"jsonrpc": "2.0", "id": agy_permission,
                              "result": {"stopReason": "end_turn"}}), flush=True)
            agy_permission = None
            continue
        if kind == "agy" and method == "session/cancel" and agy_prompt is not None:
            print(json.dumps({"jsonrpc": "2.0", "id": agy_prompt,
                              "result": {"stopReason": "cancelled"}}), flush=True)
            agy_prompt = None
            continue
        if "id" not in request:
            continue
        if kind == "agy":
            if method == "session/prompt":
                text = request["params"]["prompt"][0]["text"]
                if text == "sync-prompt-with-approval":
                    agy_permission = request["id"]
                    print(json.dumps({"jsonrpc": "2.0", "id": "permission-fixture",
                                      "method": "session/request_permission", "params": {
                        "sessionId": "agy-native", "toolCall": {"title": "Confirm fixture permission"},
                        "options": [{"optionId": "allow", "kind": "allow_once"},
                                    {"optionId": "deny", "kind": "reject_once"}]}}), flush=True)
                    continue
                elif text.startswith("sync-"):
                    text_update("agent", "Antigravity reply: " + text)
                    result = {"stopReason": "end_turn"}
                elif text != "Show edit fixture":
                    agy_prompt = request["id"]
                    continue
                else:
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
    def exercise_cli(self, control, workspace, phone, agent, thread_id):
        label = "Antigravity" if agent == "antigravity" else "OpenCode"
        target = {"thread_id": thread_id, "workspace": str(workspace)}
        phone.events.clear()
        if agent == "antigravity":
            phone.call("turn/start", dict(target, text="sync-prompt-with-approval"))
            deadline = time.monotonic() + 10
            while True:
                event = phone.events.popleft() if phone.events else phone.receive(deadline)
                if event.get("event") == "decision/required":
                    self.assertEqual(event["params"]["thread_id"], thread_id)
                    break
        cli = subprocess.Popen(control + ["--cli", "--workspace", str(workspace), "--thread", thread_id],
                               cwd=workspace, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                               stderr=subprocess.STDOUT, text=True)
        frames = queue.Queue()

        def read():
            while True:
                chunk = os.read(cli.stdout.fileno(), 8192)
                frames.put(chunk.decode("utf-8", "replace") if chunk else None)
                if not chunk:
                    return

        reader = threading.Thread(target=read, daemon=True)
        reader.start()
        output = ""

        def wait_for(expected):
            nonlocal output
            deadline = time.monotonic() + 18
            while expected not in output:
                try:
                    chunk = frames.get(timeout=max(0, deadline - time.monotonic()))
                except queue.Empty:
                    self.fail(f"Terminal did not show {expected!r}: {output[-1200:]}")
                if chunk is None:
                    self.fail(f"Terminal exited before {expected!r}: {output[-1200:]}")
                output += chunk

        def enter(text):
            cli.stdin.write(text + "\n")
            cli.stdin.flush()

        def phone_completed():
            deadline = time.monotonic() + 15
            while True:
                event = phone.events.popleft() if phone.events else phone.receive(deadline)
                params = event.get("params", {})
                if event.get("event") == "turn/completed" and params.get("threadId") == thread_id:
                    self.assertEqual(params["turn"]["status"], "completed")
                    return

        try:
            wait_for(label)
            if agent == "antigravity":
                wait_for("Confirm fixture permission")
                enter("accept")
                wait_for("Antigravity permission answered from the terminal.")
                phone_completed()
            cli_prompt = "sync-terminal-" + agent
            enter(cli_prompt)
            wait_for(label + " reply: " + cli_prompt)
            phone_completed()
            history = phone.call("threads/read", dict(target, include_turns=True, limit=20))
            self.assertTrue(any(row.get("role") == "user" and row.get("text") == cli_prompt
                                for row in history["messages"]))

            phone_prompt = "sync-phone-" + agent
            phone.call("turn/start", dict(target, text=phone_prompt))
            wait_for(label + " reply: " + phone_prompt)
            phone_completed()
            history = phone.call("threads/read", dict(target, include_turns=True, limit=20))
            self.assertTrue(any(row.get("role") == "assistant" and phone_prompt in row.get("text", "")
                                for row in history["messages"]))
            enter("/quit")
            self.assertEqual(cli.wait(timeout=5), 0)
            self.assertIn("config", phone.call("status"))
            resumed = subprocess.run(control + ["--cli", "--workspace", str(workspace), "--thread", thread_id],
                                     cwd=workspace, input="/quit\n", capture_output=True, text=True, timeout=12)
            self.assertEqual(resumed.returncode, 0, resumed.stderr)
            self.assertIn(label + " reply: " + phone_prompt, resumed.stdout)
        finally:
            if cli.poll() is None:
                cli.terminate()
                cli.wait(timeout=5)
            cli.stdin.close()
            reader.join(timeout=2)
            cli.stdout.close()

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
                self.exercise_cli(control, workspace, client, "antigravity", chats["antigravity"])
                self.exercise_cli(control, workspace, client, "opencode", chats["opencode"])
            finally:
                if client is not None:
                    client.close()
                if (state / "endpoint.json").exists():
                    stop()
