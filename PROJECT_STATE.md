# Modex project state

Updated: 2026-11-09, Asia/Ho_Chi_Minh. Branch: main; no commits. All source and documents are uncommitted.

## Objective

Native Android access to the user's actual Codex environment on Linux, Windows and macOS, including custom API providers and existing sessions. [Design](docs/design.md); [initial milestone plan](docs/plan.md).

## Implemented

Initial Go host diagnostic: explicit workspace, optional executable/CODEX_HOME, stdio app-server initialize/config/read/thread/list, safe output projection, 4 MiB frame cap, 20-second process timeout. The --serve host now keeps an asynchronous app-server reader and handles supported approval/question server requests. No external Go dependencies. Named profile selection fails explicitly rather than silently ignoring the request.

Primary agent owns architecture and implementation. Go and milestone order are provisional builder choices. Connection-scoped bridge, persistent event streaming and in-memory approval/question responses are implemented; Android, native SSH transport, detached host supervisor, shared live sessions and push relay are not implemented. License and monetization are undecided.

## Evidence on current uncommitted revision

- `GOCACHE=/tmp/modex-go-cache go test ./...`: PASS; config summary redaction/malformed data and RPC ID/error/frame boundary tests.
- `GOCACHE=/tmp/modex-go-cache go vet ./...`: PASS.
- `CGO_ENABLED=0 GOARCH=amd64 GOOS=<linux|windows|darwin> go build ... ./cmd/modex-host`: PASS for all three targets; runtime tested on Linux only.
- `go run ./cmd/modex-host --workspace /home/warmdev/Workspace/modex --codex-home <temporary isolated home>`: PASS with Codex 0.167.0, configured provider `modex-test`, model `test-model`, empty thread list. No inference and no real API key used.
- Executable boundary checks for missing workspace, nonexistent workspace and unsupported profile: PASS.
- Persistent reader/event pump test for notifications arriving after the response: PASS; safe upstream rejection categories are covered without exposing raw error text.

## Connection-scoped bridge milestone

`--serve` retains one app-server for multiple stdio requests. Implements status, paginated threads/list, metadata-only threads/read, same-ID threads/resume, and workspace-validated text `turn/start` with notification forwarding. No arbitrary methods, permissions overrides or automatic fork/replay. See [bridge contract and plan](docs/bridge-plan.md).

Verified on current uncommitted files: `go test ./...` PASS; `go test -race ./...` PASS; `go vet ./...` PASS; cross-build Linux/Windows/macOS PASS; real Codex 0.167.0 status, empty list, missing-thread rejection and continued requests PASS. Same-ID resume and cursor forwarding verified with protocol doubles only: a no-turn thread/start fixture was not durable, so real stored-thread resume is still unverified. Runtime child timeout/crash, late notification forwarding and request-rejection recovery are covered by subprocess tests.

## Limits and next steps

Diagnostic mode lists one page (20 threads) and does not mutate sessions; --serve additionally supports text turns, streaming, resume and decision responses. Neither mode exposes stored message history or archived threads. Config/read is not proof of a provider request or resolved runtime model. No direct confirmation of named-profile selection; no lock bypass or silent fork permitted. Child stderr/errors are suppressed rather than exposed because they may contain credentials.

The process now has a persistent app-server reader/event pump: responses are routed by request ID and notifications remain forwardable after `turn/start` returns. Safe rejection categories are exposed (`thread_not_found`, `active_writer`, `unauthorized`, `approval_required`, `model_provider_mismatch`, or `request_rejected`). The real provider turn remains unverified; the live smoke test was rejected as `thread_not_found` for the rendered thread ID.

Next: finish live decision validation, then follow [the next milestone plan](docs/next-milestone-plan.md): detached lifecycle, SSH framing, Kotlin/Compose session browser and reconnect cursors. Connection ends at EOF; it does not survive SSH loss. The full product's continuous synchronization requirement remains open, including compatibility with the official Desktop writer ownership.

## Multi-agent execution note

The requested delegated workflow was attempted on 2026-11-09, but every specialist stream disconnected before producing a result; no delegated edits were accepted or treated as complete. The primary agent remains responsible for integration. The persisted UI direction is in `design-system/modex/MASTER.md`.

## Decision protocol milestone (2026-11-09)

Implemented inline per user request; no subagents, commits or publishing. Inspected installed Codex schema via `codex app-server generate-json-schema --experimental`. Supported server requests: command approval, file-change approval, non-secret user questions. The bridge emits projected `decision/required` review context and handles one `decision/respond`; original string/integer upstream IDs are preserved. Unsupported methods receive a generic JSON-RPC rejection. Ordinary commands are queued in order (capacity 16); decisions can be answered while an ordinary RPC is pending. Pending decisions are capped at 128 and invalidated by matching server resolution or turn completion. Policy amendments and session-wide grants are unavailable.

Evidence on uncommitted source: `GOCACHE=/tmp/modex-go-cache go test ./...`, `go test -race ./...`, `go vet ./...` and Linux/Windows/macOS cross-builds PASS, including early server request handling, late events, unknown fields, invalid answers, unavailable approval choices, duplicate IDs, consumed responses, cancellation, turn completion, server resolution and uncertain-write replay prevention. Live provider inference is now verified in an isolated temporary workspace: direct Codex and Modex both used `third-party`, the test thread was configured with `gpt-6.1-sol`, same-ID resume succeeded, and both turns returned the exact requested text with `item/agentMessage/delta` and `turn/completed`. Live approval response is still unverified; the first approval harness attempt was rejected because its scripted command context did not match the exact allowlist. Decision state is memory-only and lost on disconnect. Review context intentionally contains task content; it is not a generic secret redactor. If an approval arrives before the RPC response, the existing 20-second deadline still applies.
