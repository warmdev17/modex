# Pixel development setup

Start the existing Pixel in Android Studio, or run `make emulator-start` (`AVD=Pixel_4_XL` by default). Then run `make emulator` to build both debug APKs, install them, import the SSH profile once and open Modex. Subsequent installs preserve the encrypted profile, history, session choices and journal. Use `make emulator-install` to reuse existing APKs. Stop it with `make emulator-stop` after testing.

For local development, start a separate supervisor once with `make emulator-host`. Its state is `~/.local/state/modex-emulator`; it uses the same Codex history/workspace while keeping the phone's relay connection independent. A fresh client can be rejected by an old shared journal with a replay-gap error, and a supervisor accepts only one attached client. The installer does not reset/restart the phone supervisor. To stop the dev supervisor, use `make host-stop STATE_DIR="$HOME/.local/state/modex-emulator"` when its work is idle.

All emulator commands target `EMULATOR_SERIAL=emulator-5554`, so a USB phone is unaffected. Override this serial when using another emulator port. If emulator/adb are outside PATH, pass `EMULATOR=/path/to/emulator ADB=/path/to/adb`. A signature mismatch fails without uninstalling or deleting saved data.

The installer reads `.modex/emulator.json` (ignored by Git). Create it once with the following shape; verify the fingerprint from the SSH server's public host key. The workstation-specific profile can use the existing `~/.ssh/modex_phone` key, provided its public key is already authorized for SSH.

```json
{
  "connection": {
    "host": "10.0.2.2",
    "port": 22,
    "user": "your-linux-user",
    "pin": "SHA256:verified-server-fingerprint",
    "binary": "/tmp/modex-host",
    "state": "/home/your-linux-user/.local/state/modex-emulator"
  },
  "private_key_file": "~/.ssh/modex_phone"
}
```

`10.0.2.2` is Android Emulator's alias for the laptop's loopback address. For a VPS use its real SSH hostname and fingerprint. SSH and the Modex supervisor must already be running; installation does not start/restart them or change authorized keys.

The key is streamed over ADB stdin into a temporary app-private file, encrypted with the existing Android Keystore-backed SavedHost store, then the temporary file is deleted. Neither the private key nor its base64 form appears in command arguments or logs. Import code is in the test APK only and checks the emulator/debug environment. Do not commit the local profile or private key. The saved connection is independent of the phone's profile. To replace it, choose Forget saved host in the emulator app, then rerun the installer. Import keeps an existing saved profile even when the local JSON changes.
