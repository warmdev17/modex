"""Install debug APKs and seed a device-local SSH profile on an explicit emulator."""
import argparse
import base64
import json
from pathlib import Path
import re
import shlex
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parent.parent
MAX_KEY = 256 * 1024


def emulator_adb(adb, serial):
    if not re.fullmatch(r"emulator-[0-9]+", serial):
        raise ValueError("Select an explicit emulator-NNNN serial, not a phone")
    return [adb, "-s", serial]


def read_profile(path):
    data = json.loads(path.read_text())
    connection = data["connection"]
    for field in ("host", "user", "pin", "binary", "state"):
        value = connection[field]
        if not isinstance(value, str) or not value.strip() or any(c in value for c in "\r\n\0"):
            raise ValueError(f"Invalid connection field: {field}")
    if type(connection["port"]) is not int or not 1 <= connection["port"] <= 65535:
        raise ValueError("Invalid SSH port")
    if not re.fullmatch(r"SHA256:[A-Za-z0-9+/]{43}", connection["pin"]):
        raise ValueError("Use the verified SSH host SHA256 fingerprint")
    if any(not connection[field].startswith("/") for field in ("binary", "state")):
        raise ValueError("Remote binary and state paths must be absolute")
    key_path = Path(data["private_key_file"]).expanduser()
    if not key_path.is_absolute():
        key_path = path.parent / key_path
    with key_path.open("rb") as source:
        key = source.read(MAX_KEY + 1)
    if not 0 < len(key) <= MAX_KEY:
        raise ValueError("Private key must be nonempty and at most 256 KiB")
    return {"connection": connection, "key": base64.b64encode(key).decode("ascii"),
            "selected": "", "cursor": 0, "messages": [], "pending": []}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--adb", default="adb")
    parser.add_argument("--serial", default="emulator-5554")
    parser.add_argument("--profile", type=Path, default=ROOT / ".modex/emulator.json")
    args = parser.parse_args()
    adb = emulator_adb(args.adb, args.serial)
    payload = json.dumps(read_profile(args.profile)).encode()
    apks = [ROOT / "android/app/build/outputs/apk/debug/app-debug.apk",
            ROOT / "android/app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk"]
    if not all(apk.is_file() for apk in apks):
        raise ValueError("Build both debug APKs first with make emulator-apk")

    def run(*command, **kwargs):
        return subprocess.run(adb + list(command), check=True, capture_output=True,
                              timeout=120, **kwargs).stdout.decode().strip()

    run("wait-for-device")
    if run("shell", "getprop", "ro.kernel.qemu") != "1":
        raise ValueError("Target is not an Android emulator; nothing installed")
    deadline = time.monotonic() + 120
    while run("shell", "getprop", "sys.boot_completed") != "1":
        if time.monotonic() >= deadline:
            raise TimeoutError("Emulator did not finish booting")
        time.sleep(1)
    for apk in apks:
        run("install", "-r", str(apk))
    run("shell", "am", "force-stop", "dev.modex")
    remote = "files/emulator-profile.json"
    try:
        run("shell", "run-as", "dev.modex", "mkdir", "-p", "files")
        run("shell", shlex.join(["run-as", "dev.modex", "sh", "-c",
                                f"umask 077; cat > {remote}"]), input=payload)
        result = run("shell", "am", "instrument", "-w", "-e", "preview", "emulator-profile",
                     "dev.modex.test/dev.modex.TransportSmoke")
        if "PASS: emulator profile" not in result or "INSTRUMENTATION_CODE: -1" not in result:
            raise RuntimeError("Emulator rejected the profile; existing saved data was not replaced")
        print(result)
    finally:
        run("shell", "run-as", "dev.modex", "rm", "-f", remote)
    run("shell", "am", "start", "-n", "dev.modex/.MainActivity")
    print("Modex opened on " + args.serial + "; SSH settings are saved on this emulator.")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError) as failure:
        # Never dump the credential payload or subprocess input on failure.
        print("Emulator setup failed (" + type(failure).__name__ + "): " + str(failure) +
              ". Check the local profile, running Pixel and APK signatures.", file=sys.stderr)
        sys.exit(1)
