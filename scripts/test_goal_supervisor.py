"""Opt-in native Codex goal journal/reconnect check without provider inference."""
import os
from pathlib import Path
import subprocess
import tempfile
import time
import unittest

from live_supervisor_smoke import AttachedRPC


HOST = os.environ.get("MODEX_HOST_BINARY")
CODEX = os.environ.get("MODEX_NATIVE_CODEX")


@unittest.skipUnless(HOST and CODEX, "requires host candidate and native Codex")
class GoalSupervisorTests(unittest.TestCase):
    def wait_event(self, client, name):
        deadline = time.monotonic() + 5
        while not any(event.get("event") == name for event in client.events):
            client.events.append(client.receive(deadline))

    def test_paused_goal_survives_reconnect_without_starting_turn(self):
        with tempfile.TemporaryDirectory(prefix="modex-goal-supervisor-") as directory:
            root = Path(directory)
            workspace, home, state = root / "workspace", root / "codex", root / "state"
            workspace.mkdir()
            home.mkdir()
            env = dict(os.environ, CODEX_HOME=str(home), HOME=str(root))
            control = [HOST, "--state-dir", str(state)]
            client = None
            started = subprocess.run(control + ["--background", "--workspace", str(workspace), "--codex", CODEX],
                                     env=env, capture_output=True, timeout=30)
            self.assertEqual(started.returncode, 0, "isolated supervisor did not start")
            try:
                client = AttachedRPC(HOST, state, workspace)
                self.assertIn("goals", client.capabilities)
                thread = client.call("threads/start", {})["thread"]["id"]
                target = dict(thread_id=thread, workspace=str(workspace))
                self.assertIsNone(client.call("threads/goal/get", target)["goal"])
                goal = client.call("threads/goal/set", dict(target, objective="Synthetic paused goal", status="paused", token_budget=1000))["goal"]
                self.assertEqual((goal["thread_id"], goal["status"], goal["token_budget"]), (thread, "paused", 1000))
                self.wait_event(client, "thread/goal/updated")
                self.assertFalse(any(event.get("event") == "turn/started" for event in client.events))
                cursor, instance = client.cursor, client.instance
                client.close()
                client = AttachedRPC(HOST, state, workspace, cursor, instance)
                client.sequence = 100
                self.assertEqual(client.call("threads/goal/get", target)["goal"], goal)
                self.assertTrue(client.call("threads/goal/clear", target)["cleared"])
                self.assertIsNone(client.call("threads/goal/get", target)["goal"])
                self.wait_event(client, "thread/goal/cleared")
                self.assertFalse(any(event.get("event") == "turn/started" for event in client.events))
            finally:
                if client:
                    client.close()
                stopped = subprocess.run(control + ["--stop"], capture_output=True, timeout=10)
                self.assertEqual(stopped.returncode, 0, "isolated supervisor did not stop")
