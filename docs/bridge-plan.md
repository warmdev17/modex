# Connection-scoped host bridge

Design extension, 2026-11-08. User authorized continuing the host/session milestone. Single primary implementer; standard-library Go, no network listener or automatic commits.

Contract: `--serve` retains a stdio Codex app-server. Positive integer request IDs; `status`, `threads/list` (limit 1–100, optional opaque cursor), `threads/read` and `threads/resume` (required thread_id). Return allowlisted configuration/thread metadata only. Bound input/output frames to 4 MiB and each Codex exchange to 20 seconds. Workspace is immutable for the connection; read/resume validate stored cwd before any resume. Do not override model/provider/permissions, fork, replay uncertain calls or bypass locks.

- [x] Implement request validation and paginated metadata projection.
- [x] Implement same-ID resume after workspace verification.
- [x] Own one child per connection; terminate on EOF/timeout; distinguish RPC rejection from transport failure.
- [x] Test multiple requests, mutation rejection, parameter validation, workspace isolation, secret redaction, cursor forwarding, same-ID resume, child crash/timeout and request rejection recovery.
- [x] Real Codex smoke: status, empty page, invalid thread rejection followed by successful commands.
- [ ] Real stored nonempty history/resume and two-page pagination: not yet validated. A no-turn thread/start fixture is not durable on Codex 0.167.0, so it is insufficient evidence.

Limits: connection-scoped lifecycle, not a background daemon surviving SSH loss; no message history UI or durable approval store. Raw upstream messages are suppressed, so lock/provider failures currently return a generic error. The app-server reader is persistent for the connection and can forward notifications after a request response; detached lifecycle and reconnect reconciliation still require explicit protocol work.

## Vertical slice extension

- [x] Preserve app-server notifications while waiting for a response.
- [x] Add validated `turn/start` with `{thread_id,text}` and exact-workspace check.
- [x] Forward notifications as `{"event": method, "params": ...}` lines before or after the related response.
- [ ] Real provider inference and streamed turn validation: intentionally not run without an explicit test prompt/provider-cost approval.
- [ ] Interruption, detached supervisor and Android transport remain separate milestones.

## Decision slice (2026-11-09)

- [x] Preserve server requests with string or integer IDs; respond with the same upstream ID.
- [x] Track command/file approval and non-secret user questions in memory; expose `decision/required` and `decision/respond`.
- [x] Validate one-time approval choices and complete question answers; reject policy escalation, duplicate responses and resolved decisions.
- [x] Process decision responses while ordinary requests are pending, without reordering ordinary commands. Limit ordinary command queue to 16 and pending decisions to 128.
- [x] Test rejected escalation, early server requests, late events, cancellation and uncertain-delivery replay prevention.
- [ ] Validate real provider approval/question flow; protocol/subprocess doubles do not prove live support.

Ruling: keep existing uncommitted workspace and no auto-commits; use PROJECT_STATE.md as the single progress snapshot. No subagents, per user instruction. Legacy approvals, permissions requests, MCP elicitation, secret questions and automatic decision grants remain unsupported and receive a generic JSON-RPC rejection. Review context is task content and must travel only over the future authenticated SSH channel.

## New chat slice (2026-11-09)

- [x] `threads/start` accepts only optional `model`, fixes cwd to the connection workspace, and inherits provider/permissions.
- [x] Project returned thread metadata and runtime model; reject malformed or out-of-workspace responses.
- [x] Test model inheritance, explicit selection, invalid parameters, forbidden overrides and metadata projection.
- [x] Real provider validation: new Modex-created thread with `gpt-6.1-sol` streamed the exact marker and completed successfully. Existing-thread handoff and one-time command approval also passed in the same live run.
