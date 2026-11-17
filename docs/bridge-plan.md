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

`threads/list` accepts optional `all_workspaces:boolean` (default false). True omits the cwd filter, includes interactive CLI, VS Code and app-server sources across providers, and returns existing projected cwd/title metadata with normal cursor pagination. Non-archived sessions are sorted by updated_at; no filesystem project scan or desktop-private pin database is read.

`threads/start`, `threads/read`, `threads/resume`, and `turn/start` accept optional `workspace`: absolute native-host path, at most 32768 UTF-8 bytes and no CR/LF/NUL. Omission retains the configured host workspace. The caller explicitly chooses a remote project; returned thread IDs and exact cwd remain checked before history access, resume and sending. Explicit resume passes the same cwd to Codex. New thread creation uses selected project cwd and existing permission mode behavior. This expands access via authenticated SSH to other Codex projects owned by that remote user; it is not a restricted single-workspace deployment when opted in.

2026-11-11 correction: listing explicitly filters sourceKinds to cli/vscode/appServer in both workspace modes. Guardian approval assessments and other internal subagent/exec/unknown sessions are excluded by source, never by title or folder name. No sessions are deleted.

## Canonical Desktop projects (2026-11-12)

`projects/list {limit?:1..100,cursor?:string}` projects Codex `project/list` into `{data:[{id,name,roots:[{path}]}],next_cursor}`. Host initialization opts into experimentalApi because project/list requires it on current Codex. No project mutations are exposed. `threads/list` now preserves nullable canonical `projectId`. Android prioritizes explicit projectId and otherwise groups unassigned sessions by the longest matching registered project root (directory boundaries, normalized separators and Windows drive case). Explicit projectless sessions remain outside projects; equally specific ambiguous roots remain in Recents. Cwd never creates a project. Registered empty projects remain visible. Discovery failure is isolated to Projects; Recents remains available. For older Desktop membership not yet migrated to app-server, the host reads only explicit local assignments and legacy-to-canonical ID mappings from the child process CODEX_HOME/.codex-global-state.json (8MiB bound). Existing canonical projectId wins; explicit projectless exclusions are respected. Missing/invalid state or unknown mapping leaves membership unset. The host also projects `projectless:true` for explicit Desktop projectless exclusions when no canonical projectId exists, so Android can distinguish exclusions from native CLI sessions without assignment. No title heuristics or Desktop state writes. The legacy mapping is a compatibility fallback for current Desktop versions.

## File and image attachments (2026-11-12)

`turn/start` accepts optional `attachments:[{path,kind:"image"|"file"}]`, 1–8 entries and at most 20MiB per regular readable absolute file. Unknown attachment fields, controls, directories, missing files and unsupported image signatures are rejected before upstream calls. `text` remains a string but may be blank when attachments exist. Images map to native Codex `localImage`; other files become JSON-encoded user-selected path references in a text input, subject to the thread's existing sandbox/approval policy. There is no universal binary/document parser or remote-write bridge endpoint. History preserves image-only messages as filename labels and includes attachments:[{path,name,kind}] metadata for local images and exact validated Modex file references. The optional display_text field retains original user text without generated filename labels; older clients can keep using text. No image data or external URLs are returned. Android uses those paths for bounded SFTP previews; old clients can keep displaying the filename labels. Phone uploads and remote directory browsing use a separate SFTP channel on the existing pinned SSH session; file bytes never enter the journal. Model/provider image capability is still authoritative and may reject image turns.


## Explicit registered CLI takeover (2026-11-12)

`threads/takeover {thread_id, confirm:true, workspace?}` is a non-replayable, explicitly confirmed state change. Validate workspace via `thread/read`, ask the CODEX_HOME-scoped launcher to release its exact session, then perform one `thread/resume` with the same ID and normal projected response. No `turn/start` is generated. Rejections preserve stable `takeover_unmanaged`, `takeover_mismatch`, `takeover_unavailable` and `takeover_uncertain` categories, without raw upstream diagnostics. A mobile timeout is not permission to replay takeover or a prompt. The command uses the existing authenticated SSH/supervisor channel; the private local launcher socket only controls its owned children. Linux/macOS only; ordinary CLI/Desktop is never terminated by inference from its process name.

## Creating remote projects

`projects/create` requires `workspace` (an existing absolute directory), `name` (nonblank, at most 256 UTF-8 bytes, no control characters), and `idempotency_key` (same bounds). It forwards one root to Codex `project/create` and returns only `{project: {id, name, roots}}`; arbitrary project metadata is not exposed. A missing/unreadable folder or a regular file is rejected before calling Codex. No folders are created by this RPC.

`threads/start` accepts optional `project_id` alongside `workspace`, forwarding `projectId` and `cwd` to Codex. Android's New project action uses SFTP to select an existing folder or explicitly create one child folder, then registers a canonical Codex project. New chat offers registered projects and the default workspace; a project's sidebar action starts chat settings with that project selected. Remote folder browsing begins at SSH home. No Git repository is initialized.

Neither project creation nor folder creation retries automatically. After an uncertain result, refresh projects/folders and inspect before creating again. Older hosts need updating for `projects/create` and project-aware `threads/start`; existing thread commands retain their behavior. Build and test host upgrades separately, then activate only when the supervisor's active turns and pending decisions are settled.

Standalone Android chat: `threads/start {standalone:true}` selects remote `~/Documents/Codex` and creates that directory if absent. It sends a null canonical project identity and never creates a project. The flag must be boolean and cannot be combined with workspace or project_id. Omitted/false retains the existing host-workspace behavior for older clients. Project-specific requests still send both workspace and project_id.

## Proposed plan confirmation

Codex structured `plan` items are projected as role `plan`, preserving the canonical Markdown, item/turn IDs and originating turn status. Live `item/plan/delta` is rendered; the completed item's text replaces accumulated deltas. A task checklist (`turn/plan/updated`) is not a proposed plan.

`plans/respond` accepts `workspace`, `thread_id`, `turn_id`, `item_id`, `action` (`implement`, `fresh`, `stay`), and optional `model`/`effort`. The host resumes metadata and verifies the exact latest completed turn, last plan item and workspace before claiming the action. Private supervisor `plan-receipts` files prevent a second dispatch, including after a process restart or an uncertain response. Never replay an uncertain action.

`implement` submits `Implement the plan.` in Default collaboration mode in the same thread. `fresh` creates a new thread in the same workspace/project/provider, copies the source permission profile or supported legacy sandbox, compares effective permissions, calls native `thread/prepareApprovedPlan`, then submits the returned prompt in Default mode with the chosen model/effort. Unsupported or changed permissions refuse implementation; no host full-access fallback is allowed. `stay` records dismissal without starting a turn or changing the source's Plan mode. The original thread/history is retained.

Responses identify `source_thread_id`, `thread_id`, `plan_key`, `action`, and, for implementation, the projected `session` and native `turn_result`. `plan/resolved` synchronizes dismissal across phone views; history adds `plan_handled`. A fresh-thread failure after creation can leave an empty thread: inspect history instead of retrying. CLI's confirmation popup is local TUI state; Modex does not inject keys into it or promise to close it remotely.

## Conversation lifecycle and mobile turn notices (2026-11-17)

`threads/list` additionally accepts `archived:boolean` (default false). True lists archived conversations only, retaining existing workspace/source/provider filters and opaque pagination. `threads/archive`, `threads/unarchive`, `threads/delete` and `threads/fork` require `thread_id`, with optional exact `workspace`. Unknown fields and mismatched stored workspaces are rejected before mutation. Delete also requires `confirm:true`; all mutations use Codex's native lifecycle APIs. A dependency refusal returns `thread_has_dependents`; clients retain the conversation and suggest archiving. Modex never deletes rollout files or database records directly.

Archive/delete return `{}`; unarchive returns `{thread}`. Fork returns the existing projected session response, verifies workspace/project/provider/model/effort and effective permissions, requests `excludeTurns:true,deferGoalContinuation:true`, and does not start a turn. Active/unknown source state, an active/unverifiable persisted goal, or unsupported permission copying refuses the fork before the source is resumed. A post-dispatch validation failure can leave a new conversation; inspect and refresh instead of replaying an uncertain fork.

The mobile JSON event encoder projects Codex `error` into `params.notice` and a safe error message, and projects completed-turn failures into `turn.notice`. A notice is `{category,message,retrying,http_status?}` with fixed allowlisted messages; arbitrary upstream error text and additional details are omitted. Persisted history carries the same object as `turn_notice` alongside the turn identity/status/timing. Native TUI WebSocket forwarding retains the original native protocol. Thread metadata now includes available native status and project identity for observation without resuming.

Android reconciles lifecycle/status/turn events against thread and turn identity; observation must not start or replay work. A TUI launched through Modex uses the shared backend event stream. A separately owned Codex Desktop/plain CLI process does not automatically publish into that stream merely because it uses the same CODEX_HOME; report independently verified observation limits rather than promising cross-process live synchronization.
