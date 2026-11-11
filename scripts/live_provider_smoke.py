"""Opt-in paid integration check using target-owned Codex credentials."""
import argparse
from collections import deque
import json
from pathlib import Path
import queue
import shlex
import subprocess
import tempfile
import threading
import time

MARKER = "MODEX_APPROVAL_OK"


def is_test_command(command):
    if not isinstance(command, str):
        return False
    try:
        argv = shlex.split(command)
        if (len(argv) == 3 and argv[0] in
                ("/usr/sbin/bash", "/usr/bin/bash", "/bin/bash") and
                argv[1] in ("-lc", "-c")):
            argv = shlex.split(argv[2])
        return argv == ["printf", MARKER]
    except ValueError:
        return False


class RPC:
    def __init__(self, argv, cwd):
        self.process = subprocess.Popen(
            argv, cwd=cwd, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL, text=True, bufsize=1)
        self.frames = queue.Queue()
        self.events = deque()
        self.sequence = 0
        self.reader = threading.Thread(target=self._read, daemon=True)
        self.reader.start()

    def _read(self):
        try:
            for line in self.process.stdout:
                self.frames.put(json.loads(line))
        except (ValueError, OSError):
            pass
        finally:
            self.frames.put(None)

    def send(self, frame):
        self.process.stdin.write(json.dumps(frame) + "\n")
        self.process.stdin.flush()

    def receive(self, deadline):
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise RuntimeError("deadline_exceeded")
        try:
            frame = self.frames.get(timeout=remaining)
        except queue.Empty:
            raise RuntimeError("deadline_exceeded") from None
        if frame is None:
            raise RuntimeError("transport_closed")
        return frame

    def call(self, method, params=None):
        self.sequence += 1
        request_id = self.sequence
        self.send({"id": request_id, "method": method, "params": params or {}})
        deadline = time.monotonic() + 30
        while True:
            frame = self.receive(deadline)
            if frame.get("id") == request_id:
                if "error" in frame:
                    # Never print the raw upstream response or credential-bearing URL.
                    raise RuntimeError("request_rejected:" + method)
                return frame["result"]
            self.events.append(frame)

    def close(self):
        if self.process.poll() is None:
            self.process.terminate()
            try:
                self.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait()
        self.process.stdin.close()
        self.reader.join(timeout=2)
        self.process.stdout.close()


def wait_turn(rpc, expected, workspace, approval=False):
    deadline = time.monotonic() + 180
    delta_seen = False
    final_text = ""
    decisions = 0
    delivered = 0
    command_ok = False
    resolved = False
    methods = set()
    while True:
        frame = rpc.events.popleft() if rpc.events else rpc.receive(deadline)
        method = frame.get("method", frame.get("event", ""))
        params = frame.get("params") or {}
        if method:
            methods.add(method)
        if method == "decision/required":
            context = params.get("context") or {}
            safe = (approval and params.get("method") ==
                    "item/commandExecution/requestApproval" and
                    is_test_command(context.get("command")) and
                    context.get("cwd") == str(workspace) and
                    not context.get("additionalPermissions") and
                    not context.get("networkApprovalContext"))
            if not safe:
                rpc.call("decision/respond", {"decision_id": params["decision_id"],
                         "result": {"decision": "cancel"}})
                raise RuntimeError("unexpected_approval_canceled")
            decisions += 1
            if decisions != 1:
                raise RuntimeError("unexpected_extra_approval")
            result = rpc.call("decision/respond", {
                "decision_id": params["decision_id"],
                "result": {"decision": "accept"}})
            delivered += result.get("delivered") is True
        elif method == "serverRequest/resolved":
            resolved = True
        elif method == "item/agentMessage/delta":
            delta_seen = True
        elif method == "item/completed":
            item = params.get("item") or {}
            if item.get("type") == "agentMessage":
                final_text = item.get("text", "")
            elif item.get("type") == "commandExecution":
                command_ok = (is_test_command(item.get("command")) and
                              item.get("exitCode") == 0 and
                              (item.get("aggregatedOutput") or "").strip() == MARKER)
        elif method == "turn/completed":
            status = (params.get("turn") or {}).get("status")
            result = {"status": status, "expected_text": final_text.strip() == expected,
                      "delta_observed": delta_seen, "event_types": sorted(methods)}
            if approval:
                result.update(approvals=decisions, delivered=delivered,
                              resolved=resolved, command_output_verified=command_ok)
            result["passed"] = (status == "completed" and result["expected_text"] and
                                delta_seen and (not approval or
                                (decisions == delivered == 1 and command_ok and resolved)))
            return result


def run(args, report):
    seed = host = None
    with tempfile.TemporaryDirectory(prefix="modex-live-smoke-") as directory:
        workspace = Path(directory)
        try:
            seed = RPC([args.codex, "app-server", "--listen", "stdio://"], workspace)
            seed.call("initialize", {"clientInfo": {"name": "modex_live_smoke", "version": "0.1"}})
            seed.send({"method": "initialized"})
            config = seed.call("config/read", {"cwd": directory, "includeLayers": False})["config"]
            report["configured_provider"] = config.get("model_provider")
            report["configured_model"] = config.get("model")
            params = {"cwd": directory, "ephemeral": False, "sandbox": "read-only",
                      "approvalPolicy": "on-request", "approvalsReviewer": "user"}
            if args.model:
                params["model"] = args.model
            started = seed.call("thread/start", params)
            thread_id = started["thread"]["id"]
            report["test_model"] = started.get("model")
            report["test_provider"] = started.get("modelProvider")
            seed.call("turn/start", {"threadId": thread_id, "input": [{"type": "text",
                      "text": "Reply exactly MODEX_PROVIDER_OK. Do not call any tools."}]})
            report["direct_provider"] = wait_turn(seed, "MODEX_PROVIDER_OK", workspace)
            if not report["direct_provider"]["passed"]:
                raise RuntimeError("direct_provider_failed")
            seed.close()
            seed = None
            host = RPC([str(Path(args.host).resolve()), "--serve", "--workspace", directory,
                        "--codex", args.codex], workspace)
            host.call("status")
            resumed = host.call("threads/resume", {"thread_id": thread_id})
            report["same_thread_resume"] = resumed["thread"]["id"] == thread_id
            host.call("turn/start", {"thread_id": thread_id, "text":
                      "Reply exactly MODEX_BRIDGE_OK. Do not call any tools."})
            report["bridge_stream"] = wait_turn(host, "MODEX_BRIDGE_OK", workspace)
            host.call("turn/start", {"thread_id": thread_id, "text":
                      "Authorized Modex approval integration test. Use exec_command exactly once "
                      "with cmd=printf MODEX_APPROVAL_OK, sandbox_permissions=require_escalated, "
                      "justification=Modex harmless printf approval test. Wait for user approval. "
                      "Do not run other commands or edit files. After execution reply exactly "
                      "MODEX_APPROVAL_OK with no other text."})
            report["bridge_approval"] = wait_turn(host, MARKER, workspace, approval=True)
            creation = host.call("threads/start", {"model": args.model} if args.model else {})
            new_thread = creation["thread"]
            report["new_thread_isolated"] = (
                new_thread["id"] != thread_id and new_thread["cwd"] == directory)
            report["new_thread_model"] = creation.get("model")
            host.call("turn/start", {"thread_id": new_thread["id"], "text":
                      "Reply exactly MODEX_NEW_CHAT_OK. Do not call any tools."})
            report["new_chat_stream"] = wait_turn(host, "MODEX_NEW_CHAT_OK", workspace)
            report["passed"] = (report["same_thread_resume"] and
                                report["bridge_stream"]["passed"] and
                                report["bridge_approval"]["passed"] and
                                report["new_thread_isolated"] and
                                report["new_chat_stream"]["passed"])
        finally:
            if seed:
                seed.close()
            if host:
                host.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", required=True, help="Built modex-host executable")
    parser.add_argument("--codex", default="codex")
    parser.add_argument("--model", help="Test-thread override only; leaves config unchanged")
    parser.add_argument("--report", type=Path, help="Write sanitized verification summary")
    args = parser.parse_args()
    report = {"passed": False}
    try:
        run(args, report)
    except Exception as error:
        report["failure"] = str(error) if type(error) is RuntimeError else type(error).__name__
    print(json.dumps(report, indent=2))
    if args.report:
        args.report.write_text(json.dumps(report, indent=2) + "\n")
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
