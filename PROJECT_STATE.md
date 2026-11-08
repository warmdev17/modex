# Modex project state

Updated: 2026-11-09, Asia/Ho_Chi_Minh. Branch: `test/live-approval`; base commit `eab0a63`. This snapshot accompanies the live-test commit; Git records its final hash.

## Objective

Native Android access to the user's actual Codex environment on Linux, Windows and macOS, preserving custom API providers, existing sessions, skills and MCP. [Design](docs/design.md); [milestone plan](docs/next-milestone-plan.md); [bridge contract](docs/bridge-plan.md).

## Implemented

Go standard-library host: diagnostic mode and connection-scoped `--serve` bridge with status, paginated workspace-filtered thread list/read/resume, text turn/start, persistent app-server reader, late event streaming, and one-time command/file approval and non-secret question responses. No automatic fork, writer-lock bypass, session-wide permission grants or uncertain operation replay. Frames capped at 4 MiB; each RPC has a 20-second deadline. Pending decisions are memory-only, capped at 128; ordinary command queue capped at 16.

`scripts/live_provider_smoke.py` reproduces paid direct inference, same-thread handoff, streaming and a real approval in a temporary workspace. The command allowlist is tested separately. Review context intentionally contains task content; it is not a generic credential redactor. Raw upstream errors/configuration are suppressed.

## Verified evidence

On the current live-test working tree:

- `python3 -m unittest discover -s scripts -p 'test_*.py'`: PASS; direct/wrapped printf allowlist and shell-injection rejection.
- `python3 scripts/live_provider_smoke.py --host /tmp/modex-host-linux --model gpt-6.1-sol --report /tmp/modex-live-approval-verified.json`: PASS against real `third-party` provider. Configured model was `gpt-5.5`; only the test thread used `gpt-6.1-sol`. Global config was not changed.
- Direct provider and Modex text turns returned exact markers, streamed `item/agentMessage/delta`, and ended `completed`. Same thread ID survived closing the first app-server and resuming through Modex.
- Real approval: one `decision/required`, one delivered `accept`, `serverRequest/resolved`, exact printf command stdout, exit code 0, final text and completed turn all verified. No raw logs, keys or endpoint values stored here.
- At base `eab0a63`: Go tests, race tests, vet and amd64 cross-builds for Linux/Windows/macOS PASS. Runtime/live integration tested on Linux only.

Earlier attempts with configured `gpt-5.5` returned HTTP 403 in both direct Codex and Modex; `gpt-6.1-sol` succeeded. Do not describe that earlier 403 as proven transient or a proven auth failure. The original approval harness failed because Codex supplied a Bash-wrapped command and offered accept/cancel rather than decline; the corrected harness checks the shell wrapper and actual execution instead of assuming failure was in the bridge.

## In progress and next

Primary agent is sole builder/state writer; execution is inline, no subagents. Next: expose workspace-bound thread creation through Modex so a new mobile chat needs no external seed. Then detached host lifecycle, reconnect/event cursors, SSH transport, Kotlin/Compose client, notification relay.

Android, native SSH, detached supervisor and push relay remain unimplemented. Phone disconnect currently ends the connection and stops its child; simultaneous official Desktop/CLI ownership is unresolved. Stored message history UI, named profiles, live file approvals/questions, legacy approvals, MCP elicitation and secret questions are not validated/supported. No license, billing or release decisions. No publishing or push performed.
