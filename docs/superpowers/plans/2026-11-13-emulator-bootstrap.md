# Emulator bootstrap implementation plan

> Execute inline with executing-plans; no delegation needed.

**Goal:** Install debug Modex on the Pixel emulator with a saved SSH profile and key, without repeated form entry.

**Architecture:** A local ignored JSON profile references a private key file. Python targets an explicit emulator, streams a temporary snapshot into app-private storage, and invokes the existing debug-only instrumentation runner to encrypt it with SavedHost. Existing saved state is preserved. No production import endpoint or SSH policy change.

**Tech stack:** Python standard library, ADB, existing Android instrumentation/Keystore, Make.

- [x] Add Python tests for profile validation, key transport and emulator-only targeting; observe failure, then implement installer.
- [x] Add a debug instrumentation mode that validates/imports a fresh profile, preserves an existing snapshot, and always deletes temporary credentials. Add Make targets and usage documentation. Prepare this workstation's ignored profile using the verified local SSH host public key and existing modex_phone key path.
- [x] Run Python tests, Android build/unit/lint and an emulator import/preservation probe. Stop the emulator started for verification. Update PROJECT_STATE and commit the coherent tooling change; no push, merge or production host restart.
