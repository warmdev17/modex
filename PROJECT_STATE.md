# Modex project state

Updated: 2026-11-09, Asia/Ho_Chi_Minh. Branch: `feat/ssh-transport`; base commit `31e06f4`. This snapshot accompanies the SSH transport milestone; Git records the final hash. Prior milestones: live approval `0668e2d`, new chat `187cb87`.

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

Primary agent is sole builder/state writer; execution is inline, no subagents. Completed: workspace-bound thread creation and background supervisor with bounded cursor replay, verified with real inference. Completed: initial system OpenSSH client transport. Next: Kotlin/Compose client with native SSH, Windows supervisor ACL support, notification relay.

Android, Android-native SSH and push relay remain unimplemented; the Go CLI now provides system OpenSSH transport. Direct --serve is connection-scoped; --background/--supervise keep the bridge alive across attach disconnects; simultaneous official Desktop/CLI ownership is unresolved. Stored message history UI, named profiles, live file approvals/questions, legacy approvals, MCP elicitation and secret questions are not validated/supported. No license, billing or release decisions. No publishing or push performed.

## Background supervisor milestone

Design/contract: [supervisor plan](docs/supervisor-plan.md). Linux/macOS detached launcher plus foreground mode, authenticated loopback attach and stop, random instance/token in private owned 0700/0600 state, exclusive owner lock and no automatic stale recovery. Journal retains 1024 frames/16 MiB, reports gaps, checks instance on resume; one client controls at a time. No automatic operation replay, model override, Codex restart, on-disk transcript or boot service. Windows supervisor modes fail closed pending native ACL validation; --serve remains available. macOS is cross-build-only.

Tests: journal retention/count/byte gaps, wakeups, invalid JSON, auth, competing clients, stale instance, offline replay, explicit stop, private/exclusive/stale state; black-box detached fake app-server test verifies CLI attach, disconnected turn completion, state cleanup and no orphan child. Provider test `python3 scripts/live_supervisor_smoke.py --host /tmp/modex-supervisor-host --model gpt-6.1-sol --report /tmp/modex-supervisor-live.json` PASS with `third-party`: submitted once, disconnected after acknowledgement, reattached same instance/cursor, exact delta-streamed reply and completed turn, authenticated stop and cleaned state. Linux runtime only. Pending decisions persist while bridge is alive but their original notification must remain within journal retention to replay. A hard crash loses RAM journal and can leave stale private state; do not bypass locks.

Final supervisor checks on the working tree: `GOCACHE=/tmp/modex-go-cache go test -race ./...` PASS; `go vet ./...` PASS; Linux/Windows/macOS amd64 cross-builds PASS. `MODEX_HOST_BINARY=/tmp/modex-supervisor-host PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p 'test_*.py' -v` PASS (4 tests, including failed-start cleanup). Live reconnect smoke PASS twice. Self-review only, per no-subagent instruction; no SSH/mobile runtime claim. All fixture supervisors were stopped; no service installed. New source and related docs committed together as one supervisor feature.

## OpenSSH transport milestone

`cmd/modex` + `internal/sshtransport`: use installed OpenSSH to exec remote `modex-host --attach`; fixed strict host key checking, key/agent-only noninteractive authentication, no PTY, timeouts/keepalives, shell-quoted absolute POSIX remote paths and cursor/instance reconnect. No token or provider key leaves the target; no prompt replay or automatic host deployment. SSH config aliases are allowed and remain trusted local configuration.

Verification on working tree: Go tests/race/vet PASS; client cross-builds Linux/Windows/macOS PASS. Linux real loopback sshd test uses ephemeral keys and isolated config: JSON round-trip and metacharacter paths PASS, wrong host key rejected, real supervisor status and same-instance cursor continuity across two SSH attachments PASS (fake provider). No remote VPS, Android, Windows client runtime or macOS runtime validation. Supervisor real-provider test was already validated at `31e06f4`; combined SSH plus paid inference is not claimed. Related contract in docs/ssh-transport-plan.md. Self-review only, no subagents per user instruction.
