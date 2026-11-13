"""Opt-in paid reconnect check; never replays a submitted turn."""
import argparse
import json
from pathlib import Path
import subprocess
import tempfile
import time

from live_provider_smoke import RPC, wait_turn


class AttachedRPC(RPC):
    def __init__(self, host, state, workspace, cursor=0, instance=None):
        argv = [host, "--attach", "--state-dir", str(state), "--cursor", str(cursor)]
        if instance:
            argv += ["--instance", instance]
        super().__init__(argv, workspace)
        hello = RPC.receive(self, time.monotonic() + 10)
        if not hello.get("ready"):
            self.close()
            raise RuntimeError("attach_failed")
        self.instance = hello["instance"]
        self.cursor = cursor

    def receive(self, deadline):
        entry = RPC.receive(self, deadline)
        if "seq" not in entry or "message" not in entry:
            raise RuntimeError("invalid_journal_entry")
        if entry["seq"] != self.cursor + 1:
            raise RuntimeError("journal_sequence_gap")
        self.cursor = entry["seq"]
        return entry["message"]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", required=True)
    parser.add_argument("--codex", default="codex")
    parser.add_argument("--model", required=True)
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    report = {"passed": False}
    host = str(Path(args.host).resolve())
    client = None
    with tempfile.TemporaryDirectory(prefix="modex-live-supervisor-") as directory:
        root = Path(directory)
        workspace = root / "workspace"
        workspace.mkdir()
        state = root / "state"
        control = [host, "--state-dir", str(state)]
        launched = False
        try:
            start = subprocess.run(control + ["--background", "--workspace", str(workspace),
                "--codex", args.codex], capture_output=True, timeout=40)
            if start.returncode:
                raise RuntimeError("background_start_failed")
            launched = True
            client = AttachedRPC(host, state, workspace)
            status = client.call("status")
            report["provider"] = status["config"].get("provider")
            created = client.call("threads/start", {"model": args.model})
            report["test_model"] = created["model"]
            client.call("turn/start", {"thread_id": created["thread"]["id"], "text":
                "Reply exactly MODEX_RECONNECTED_OK. Do not call tools."})
            cursor, instance = client.cursor, client.instance
            client.close()
            client = None
            report["detached_after_turn_ack"] = True
            time.sleep(3)
            client = AttachedRPC(host, state, workspace, cursor, instance)
            # Match request IDs across this journal; never resend turn/start.
            client.sequence = 100
            report["same_supervisor_instance"] = client.instance == instance
            report["turn_after_reconnect"] = wait_turn(client, "MODEX_RECONNECTED_OK", workspace)
            report["cursor_advanced"] = client.cursor > cursor
            report["passed"] = (report["turn_after_reconnect"]["passed"] and
                                report["same_supervisor_instance"] and report["cursor_advanced"])
        except Exception as error:
            report["failure"] = str(error) if type(error) is RuntimeError else type(error).__name__
        finally:
            if client:
                client.close()
            if launched:
                stopped = subprocess.run(control + ["--stop"], capture_output=True, timeout=10)
                report["stop_acknowledged"] = stopped.returncode == 0
                deadline = time.monotonic() + 5
                while (state / "endpoint.json").exists() and time.monotonic() < deadline:
                    time.sleep(.05)
                report["state_cleaned"] = (state / "owner.lock").read_text() == "modex-flock-v1\n" and not (state / "endpoint.json").exists()
                report["passed"] = report["passed"] and report["stop_acknowledged"] and report["state_cleaned"]
    print(json.dumps(report, indent=2))
    if args.report:
        args.report.write_text(json.dumps(report, indent=2) + "\n")
    return 0 if report["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
