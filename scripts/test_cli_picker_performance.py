"""Opt-in real-PTY timing against a controlled slow optional runtime, no inference."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import struct
import subprocess
import sys
import tempfile
import termios
import time

from test_shared_agent_supervisor import FIXTURE


def measure(host):
    with tempfile.TemporaryDirectory(prefix="modex-picker-timing-") as directory:
        root = Path(directory)
        workspace = root / "project"
        workspace.mkdir()
        state = root / "state"
        source = FIXTURE.replace('elif kind == "agy" and "models" in sys.argv:\n',
                                 'elif kind == "agy" and "models" in sys.argv:\n    time.sleep(2)\n')
        source = source.replace('"codex-fixture"', '"01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6"')
        launch = [host, "--state-dir", str(state), "--background", "--workspace", str(workspace)]
        for runtime in ("codex", "agy", "opencode"):
            binary = root / runtime
            binary.write_text(f"#!{sys.executable}\n{source}")
            binary.chmod(0o700)
            launch += ["--" + runtime, str(binary)]
        subprocess.run(launch, check=True, capture_output=True, timeout=30)
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 90, 0, 0))
        env = dict(os.environ, TERM="xterm-256color", LC_ALL="C.UTF-8", NO_COLOR="1")
        cli = None
        output = bytearray()

        def expect(text):
            deadline = time.monotonic() + 12
            needle = text.encode()
            while needle not in output:
                if not select.select([master], [], [], max(0, deadline-time.monotonic()))[0]:
                    raise AssertionError("Picker did not show " + text)
                output.extend(os.read(master, 65536))

        try:
            start = time.monotonic()
            cli = subprocess.Popen([host, "--state-dir", str(state), "--cli", "--workspace", str(workspace)],
                                   cwd=workspace, env=env, stdin=slave, stdout=slave, stderr=slave)
            expect("Codex")
            roster = time.monotonic() - start
            os.write(master, b"\r")
            expect("New conversation")
            sessions = time.monotonic() - start
            capture = output.decode(errors="replace")
            return {"agent_seconds": round(roster, 4), "sessions_seconds": round(sessions, 4),
                    "project_visible": str(workspace) in capture, "capture": capture}
        finally:
            if cli is not None and cli.poll() is None:
                os.write(master, b"\x03")
                try:
                    cli.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    cli.kill()
                    cli.wait(timeout=5)
            os.close(master)
            os.close(slave)
            subprocess.run([host, "--state-dir", str(state), "--stop"], capture_output=True, timeout=10)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", required=True)
    parser.add_argument("--baseline", required=True)
    parser.add_argument("--capture", type=Path)
    args = parser.parse_args()
    before, after = measure(args.baseline), measure(args.host)
    if args.capture:
        args.capture.write_text(after["capture"])
    before.pop("capture")
    after.pop("capture")
    print(json.dumps({"baseline": before, "candidate": after}, indent=2))
    assert before["agent_seconds"] >= 1.8, "baseline did not exercise delayed readiness"
    assert after["agent_seconds"] < .5 and after["sessions_seconds"] < .5, "optional runtime blocked Codex"
    assert after["project_visible"], "project identity missing"


if __name__ == "__main__":
    main()
