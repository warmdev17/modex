# Modex project state

Updated: 2026-11-09, Asia/Ho_Chi_Minh. Branch: `feat/android-approval-review`; base commit `82e799c`. This snapshot accompanies readable approval review; Git records its final commit. Prior milestones: live approval `0668e2d`, new chat `187cb87`, supervisor `31e06f4`, OpenSSH `4f535d6`.

## Objective

Native Android access to the user's actual Codex environment on Linux, Windows and macOS, preserving custom API providers, existing sessions, skills and MCP. [Design](docs/design.md); [milestone plan](docs/next-milestone-plan.md); [bridge contract](docs/bridge-plan.md).

## Implemented

Go standard-library host: diagnostic mode and connection-scoped `--serve` bridge with status, workspace-bound thread creation with optional model, paginated workspace-filtered thread list/read/resume, text turn/start, persistent app-server reader, late event streaming, and one-time command/file approval and non-secret question responses. No automatic fork, writer-lock bypass, session-wide permission grants or uncertain operation replay. Frames capped at 4 MiB; each RPC has a 20-second deadline. Pending decisions are memory-only, capped at 128; ordinary command queue capped at 16.

`scripts/live_provider_smoke.py` reproduces paid direct inference, same-thread handoff, streaming and a real approval in a temporary workspace. The command allowlist is tested separately. Review context intentionally contains task content; it is not a generic credential redactor. Raw upstream errors/configuration are suppressed.

## Verified evidence

Previous live-provider evidence at `187cb87`:

- `python3 -m unittest discover -s scripts -p 'test_*.py'`: PASS; direct/wrapped printf allowlist and shell-injection rejection.
- `python3 scripts/live_provider_smoke.py --host /tmp/modex-host-linux --model gpt-6.1-sol --report /tmp/modex-new-thread-verified.json`: PASS against real `third-party` provider. Configured model was `gpt-5.5`; only the test thread used `gpt-6.1-sol`. Global config was not changed.
- Direct provider and Modex text turns returned exact markers, streamed `item/agentMessage/delta`, and ended `completed`. Same thread ID survived closing the first app-server and resuming through Modex.
- Real approval: one `decision/required`, one delivered `accept`, `serverRequest/resolved`, exact printf command stdout, exit code 0, final text and completed turn all verified. No raw logs, keys or endpoint values stored here.
- Newly created chat through `threads/start` used the fixed temporary workspace and `gpt-6.1-sol`, had an independent thread ID, streamed the exact marker and completed successfully.
- Current working tree: Go tests, race tests, vet and amd64 cross-builds for Linux/Windows/macOS PASS. Runtime/live integration tested on Linux only. All new feature tests were observed failing before implementation.

Earlier attempts with configured `gpt-5.5` returned HTTP 403 in both direct Codex and Modex; `gpt-6.1-sol` succeeded. Do not describe that earlier 403 as proven transient or a proven auth failure. The original approval harness failed because Codex supplied a Bash-wrapped command and offered accept/cancel rather than decline; the corrected harness checks the shell wrapper and actual execution instead of assuming failure was in the bridge.

## In progress and next

Primary agent owns integration/state. Latest user authorized subagent workflow if available: planner probe and retry failed with stream disconnect; later read-only reviewer also failed before returning findings. Implementation and review therefore completed inline; no independent review is claimed.

Completed: first native Kotlin/Compose Android client, built debug APK, emulator UI launch and Android-native SSH integration (fake provider). [Android plan](docs/android-plan.md). Next: full Android UI-to-real-provider smoke, stored session history, file diff review, secure saved connections, then foreground/relay notifications and bubbles. Windows supervisor ACL support remains outstanding.

No Android background service/push, license, billing or release decisions; no publishing/push performed. Desktop/CLI simultaneous writer ownership remains unresolved. Legacy approvals, MCP elicitation and secret questions remain unsupported. Android supports one-time command approval/question UI; file-change acceptance is intentionally disabled pending a diff viewer.

## Background supervisor milestone

Design/contract: [supervisor plan](docs/supervisor-plan.md). Linux/macOS detached launcher plus foreground mode, authenticated loopback attach and stop, random instance/token in private owned 0700/0600 state, exclusive owner lock and no automatic stale recovery. Journal retains 1024 frames/16 MiB, reports gaps, checks instance on resume; one client controls at a time. No automatic operation replay, model override, Codex restart, on-disk transcript or boot service. Windows supervisor modes fail closed pending native ACL validation; --serve remains available. macOS is cross-build-only.

Tests: journal retention/count/byte gaps, wakeups, invalid JSON, auth, competing clients, stale instance, offline replay, explicit stop, private/exclusive/stale state; black-box detached fake app-server test verifies CLI attach, disconnected turn completion, state cleanup and no orphan child. Provider test `python3 scripts/live_supervisor_smoke.py --host /tmp/modex-supervisor-host --model gpt-6.1-sol --report /tmp/modex-supervisor-live.json` PASS with `third-party`: submitted once, disconnected after acknowledgement, reattached same instance/cursor, exact delta-streamed reply and completed turn, authenticated stop and cleaned state. Linux runtime only. Pending decisions persist while bridge is alive but their original notification must remain within journal retention to replay. A hard crash loses RAM journal and can leave stale private state; do not bypass locks.

Final supervisor checks on the working tree: `GOCACHE=/tmp/modex-go-cache go test -race ./...` PASS; `go vet ./...` PASS; Linux/Windows/macOS amd64 cross-builds PASS. `MODEX_HOST_BINARY=/tmp/modex-supervisor-host PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p 'test_*.py' -v` PASS (4 tests, including failed-start cleanup). Live reconnect smoke PASS twice. Self-review only, per no-subagent instruction; no SSH/mobile runtime claim. All fixture supervisors were stopped; no service installed. New source and related docs committed together as one supervisor feature.

## OpenSSH transport milestone

`cmd/modex` + `internal/sshtransport`: use installed OpenSSH to exec remote `modex-host --attach`; fixed strict host key checking, key/agent-only noninteractive authentication, no PTY, timeouts/keepalives, shell-quoted absolute POSIX remote paths and cursor/instance reconnect. No token or provider key leaves the target; no prompt replay or automatic host deployment. SSH config aliases are allowed and remain trusted local configuration.

Verification on working tree: Go tests/race/vet PASS; client cross-builds Linux/Windows/macOS PASS. Linux real loopback sshd test uses ephemeral keys and isolated config: JSON round-trip and metacharacter paths PASS, wrong host key rejected, real supervisor status and same-instance cursor continuity across two SSH attachments PASS (fake provider). No remote VPS, Android, Windows client runtime or macOS runtime validation. Supervisor real-provider test was already validated at `31e06f4`; combined SSH plus paid inference is not claimed. Related contract in docs/ssh-transport-plan.md. Self-review only, no subagents per user instruction.


## Native Android milestone

Kotlin/Compose min API 26, target/compile 36; Gradle wrapper 8.14.3. Native mwiede JSch with Bouncy Castle Ed25519, strict out-of-band SHA256 host pin, SAF key import held in RAM. Connection form, paginated IDs, new chat/model, same-ID resume, live deltas, offered command decisions and non-secret questions. Same-instance journal cursor advances only after apply succeeds; no automatic prompt retry. Connection metadata survives rotation; ViewModel key/journal do not survive process death. No stored history loading or file approval acceptance.

Verified on this milestone's uncommitted working tree before commit:

- Gradle `:app:assembleDebug :app:assembleDebugAndroidTest :app:testDebugUnitTest :app:lintDebug --no-daemon`: PASS; five JVM protocol tests, zero test failures/lint errors. Lint warns about newer dependencies, default icon and backup-rule metadata; no production release readiness claim. Used installed Gradle 8.14.3/JDK 21/SDK 36 with `/tmp/modex-gradle` cache. Wrapper generated, but fresh wrapper download not separately tested.
- Emulator Pixel 4 XL: APK installed, activity launched, connection screen visually inspected; no AndroidRuntime crash observed at launch.
- `python3 scripts/test_android_transport.py --host /tmp/modex-android-host --adb /opt/android-sdk/platform-tools/adb`: PASS on Android emulator with ephemeral Ed25519 keys and isolated loopback sshd, real supervisor/fake provider. Two status round-trips, same-instance cursor continuity and wrong-pin rejection checked. Fixture stopped and private test key deleted. Initial test timed out; traced to JSch returning a new buffered output stream on each getter. Keeping write/flush on the same stream fixed the reproduced failure.
- `GOCACHE=/tmp/modex-go-cache go test ./...`: PASS; Go behavior unchanged.

APK: `android/app/build/outputs/apk/debug/app-debug.apk` (ignored build output). No physical phone, remote VPS, full UI chat/approval automation or Android-to-paid-provider inference checked; previous Go host provider evidence above still applies only to the host path. UI implementation is a first functional slice, not a finished visual design. No persistent Android storage of private keys; selected original key file remains wherever the user imported it from.


## Physical-phone troubleshooting: turn feedback

User reached the session/new-chat UI after correcting remote host path to `/tmp/modex-host`. SSH key authentication succeeded. Existing phone session (prefix `01a1bdba`) recorded `gpt-5.5`, then provider HTTP 403 explicitly rejecting model access; no agent message was produced. Android ignored asynchronous turn errors. No global model/provider config changed and the user prompt was not replayed.

Fix: classify failed-turn and non-retrying error events into fixed safe UI messages (HTTP 403 gets model/key-permission guidance), never display raw provider URL/error. Session picker now refreshes on open and exposes Refresh/count/empty state. The cause of an empty list on the physical phone is not independently proven; no claim that full stored-session support is fixed.

Verification on uncommitted fix tree before commit: regression test initially failed compilation for missing error classifier; final Gradle `:app:assembleDebug :app:testDebugUnitTest :app:lintDebug --offline --no-daemon` PASS, 6 tests / 0 failures, lint 0 errors (existing warnings remain). One separate real-provider turn through `/tmp/modex-host --serve` in a temporary workspace with explicit `gpt-6.1-sol`: exact marker, streamed delta and completed status PASS. This does not validate physical-phone rendering; user needs to install updated APK and create a chat with an allowed model. Existing session was not resumed or mutated by troubleshooting. No Go source changed; Go tests not repeated for this Android-only fix. Self-review only; prior reviewer transport unavailable. No publish/push.


## Readable approval review and phone validation

User reported physical-phone success with `gpt-6.1-sol`: response streaming, disconnect during a roughly 100-word composition then reconnect receiving messages, command approval visible, Allow once executed the command and returned its result. These are user-observed device results, not automated device evidence. The phone supervisor was explicitly stopped/restarted for approval testing with a temporary `/tmp/modex-codex-approval` wrapper: `read-only`, `on-request`, reviewer `user`, model `gpt-6.1-sol`; config/read confirmed these values. Global config/history unchanged. New supervisor instance requires resetting the client cursor and creating a new thread, since old threads used danger-full-access. Wrapper/binary in /tmp are temporary, not installed services.

UI improvement: approval dialog displays exact selectable monospace command, working directory, reason, action type and all supplied extra filesystem/network scope, with fixed labels. Nested permission values become readable field/value lines rather than a JSON envelope; unknown fields remain visible. Offered one-time buttons and response payloads unchanged; file-change acceptance remains disabled with an explicit explanation until a diff viewer exists.

Verification before commit on uncommitted milestone tree: Gradle `:app:assembleDebug :app:assembleDebugAndroidTest :app:testDebugUnitTest :app:lintDebug --offline --no-daemon` PASS; 7 unit tests, 0 failures/lint errors (existing warnings remain). Formatter regression initially failed compilation for absent helper, then passed. `adb shell am instrument -w -e preview approval dev.modex.test/dev.modex.TransportSmoke` PASS: light/dark screenshots captured and manually inspected on Pixel 4 XL emulator. No SSH connection/command executes in this preview. Initial fixture text-node assertion failed despite successful screenshot rendering; fixture now captures images for manual inspection and makes no accessibility-assertion claim. Test runner remains TransportSmoke, with opt-in preview mode; default SSH fixture behavior preserved. No Go source changes or repeated Go tests. New UI not yet verified on physical phone, tablet, landscape, large fonts or screen reader. No independent reviewer used for this narrow change; no push/publication.
