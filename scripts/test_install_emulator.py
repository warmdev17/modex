"""Credential transport and device targeting regressions for emulator setup."""
import base64
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import subprocess

from install_emulator import emulator_adb, read_profile, main


class EmulatorInstallTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.key = self.root / "key with spaces"
        self.key.write_bytes(b"fixture-private-key")
        self.profile = self.root / "profile.json"
        self.data = {
            "connection": {"host": "10.0.2.2", "port": 22, "user": "tester",
                           "pin": "SHA256:" + "A" * 43, "binary": "/tmp/modex-host",
                           "state": "/home/tester/.local/state/modex-phone"},
            "private_key_file": str(self.key),
        }
        for path in ("debug/app-debug.apk", "androidTest/debug/app-debug-androidTest.apk"):
            apk = self.root / "android/app/build/outputs/apk" / path
            apk.parent.mkdir(parents=True, exist_ok=True)
            apk.touch()

    def load(self):
        self.profile.write_text(json.dumps(self.data))
        return read_profile(self.profile)

    def test_snapshot_contains_key_bytes_without_local_key_path(self):
        snapshot = self.load()
        self.assertEqual(base64.b64decode(snapshot["key"]), self.key.read_bytes())
        self.assertNotIn("private_key_file", snapshot)
        self.assertEqual(snapshot["connection"], self.data["connection"])
        self.assertEqual(snapshot["cursor"], 0)

    def test_oversized_or_empty_key_rejected(self):
        for size in (0, 256 * 1024 + 1):
            with self.subTest(size=size):
                self.key.write_bytes(b"x" * size)
                with self.assertRaises(ValueError):
                    self.load()

    def test_invalid_connection_rejected(self):
        for field, value in (("pin", "unverified"), ("port", True), ("port", 0),
                             ("host", ""), ("user", "user\nother"),
                             ("binary", "relative"), ("state", "~/.state")):
            with self.subTest(field=field, value=value):
                original = self.data["connection"][field]
                self.data["connection"][field] = value
                with self.assertRaises(ValueError):
                    self.load()
                self.data["connection"][field] = original

    def test_only_explicit_emulator_serials_accepted(self):
        self.assertEqual(emulator_adb("adb", "emulator-5554"), ["adb", "-s", "emulator-5554"])
        for serial in ("", "R58PHONE", "emulator-5554;echo", "10.0.0.2:5555"):
            with self.subTest(serial=serial), self.assertRaises(ValueError):
                emulator_adb("adb", serial)

    def test_physical_device_cannot_spoof_emulator_serial(self):
        self.load()
        with patch("install_emulator.ROOT", self.root), patch("sys.argv", ["setup", "--profile", str(self.profile)]), \
                patch("subprocess.run", return_value=subprocess.CompletedProcess([], 0, b"0\n")) as run:
            with self.assertRaises(ValueError):
                main()
        self.assertEqual(run.call_count, 2)
        self.assertFalse(any("install" in call.args[0] for call in run.call_args_list))

    def test_failed_import_cleans_temporary_credentials_and_does_not_launch(self):
        self.load()

        def respond(command, **kwargs):
            output = b"INSTRUMENTATION_CODE: 0\n" if "instrument" in command else b"1\n"
            return subprocess.CompletedProcess(command, 0, output)

        with patch("install_emulator.ROOT", self.root), patch("sys.argv", ["setup", "--profile", str(self.profile)]), \
                patch("subprocess.run", side_effect=respond) as run:
            with self.assertRaises(RuntimeError):
                main()
        calls = run.call_args_list
        self.assertEqual(calls[-1].args[0][-3:], ["rm", "-f", "files/emulator-profile.json"])
        self.assertFalse(any("start" in call.args[0] for call in calls))
        uploads = [call for call in calls if "input" in call.kwargs]
        self.assertEqual(len(uploads), 1)
        self.assertEqual(base64.b64decode(json.loads(uploads[0].kwargs["input"])["key"]), self.key.read_bytes())
        self.assertNotIn(base64.b64encode(self.key.read_bytes()).decode(), str([call.args for call in calls]))
