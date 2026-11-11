# Connection-scoped host bridge

## Thread permission extension (2026-11-10)

`threads/start` additionally accepts optional `permission_mode`:

| Value | Upstream sandbox | Approval policy | Reviewer |
| --- | --- | --- | --- |
| omitted / `host-default` | inherited | inherited | inherited |
| `read-only` | `read-only` | `on-request` | `user` |
| `workspace-write` | `workspace-write` | `on-request` | `user` |
| `full-access` | `danger-full-access` | `never` | `user` |

Null/unknown/non-string values fail before upstream calls. No arbitrary policy
objects, filesystem/network scopes or provider overrides accepted. Existing
resume/turn commands do not accept permission overrides. Global config unchanged.

Create/resume results additionally project optional `approval_policy`,
`approvals_reviewer`, and `sandbox` (legacy policy type only), plus runtime `model`.
Only recognized enum labels are forwarded; unsupported/missing policies are
omitted and rendered Unknown. Raw policy objects/scopes are never forwarded.
Returned settings describe Codex's response, not the requested mode. Exact
thread ID/workspace checks remain mandatory. Host policy can reject choices.

Verified with installed Codex schema, fake upstream tests, and real Codex create
for all explicit modes. One real third-party-model marker turn followed by a
fresh bridge resume retained same ID and returned readOnly/on-request/user.

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

## Command timeline extension (2026-11-11)

`threads/read` messages may additionally have `role: command`, stable `id`, `command`, `cwd`, `status`, nullable integer `exit_code`, and plain output in `text`. Text roles remain `user`/`assistant`. Command output is bounded to 64 KiB per item and 1 MiB total per history response, with a visible truncation notice; original Codex logs are unchanged. Existing frame limits still apply to the full response.

Android reduces forwarded `item/started`, `item/completed`, `item/agentMessage/delta`, and `item/commandExecution/outputDelta` by item ID after selected-thread validation. This separates commentary/final messages and keeps each command's state and output in one collapsible card. Existing text-only saved state remains readable. This does not enumerate OS processes, expose additional execution controls or change approval policy.

## Model catalog and reasoning extension (2026-11-11)

`models/list {limit?:1..100,cursor?:string}` calls Codex `model/list` with visible models only. Result `{data:[{id,model,displayName,defaultReasoningEffort,supportedReasoningEfforts:[{reasoningEffort,description}]}],next_cursor:string|null}` projects picker fields only. Catalog may be bundled/cached; it is not proof of provider model access. Android follows cursor pages on connect and retains custom IDs if discovery fails.

`turn/start` accepts optional `effort`: nonblank single-line string up to 64 UTF-8 bytes, forwarded unchanged to Codex. Omission inherits the session's current setting. Current schema advertises arbitrary effort strings; the Android slider only offers efforts from the chosen model. Changing model clears the queued effort; changes apply to the next explicit send. No provider HTTP calls or credentials on the phone.

## Remote project extension (2026-11-11)

`threads/list` accepts optional `all_workspaces:boolean` (default false). True omits the cwd filter, includes all known session sources/providers, and returns existing projected cwd/title metadata with normal cursor pagination. Non-archived sessions are sorted by updated_at; no filesystem project scan or desktop-private pin database is read.

`threads/start`, `threads/read`, `threads/resume`, and `turn/start` accept optional `workspace`: absolute native-host path, at most 32768 UTF-8 bytes and no CR/LF/NUL. Omission retains the configured host workspace. The caller explicitly chooses a remote project; returned thread IDs and exact cwd remain checked before history access, resume and sending. Explicit resume passes the same cwd to Codex. New thread creation uses selected project cwd and existing permission mode behavior. This expands access via authenticated SSH to other Codex projects owned by that remote user; it is not a restricted single-workspace deployment when opted in.
