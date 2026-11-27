# OpenCode backend

Modex normally remote-controls the Codex CLI. This document describes the
optional OpenCode backend, verified against OpenCode 2.4.1 at revision
`771c518`. Codex remains the default; the OpenCode backend is selected
explicitly with `--backend opencode`.

The backend keeps the existing Modex protocol for the implemented surface: the
same method names, result shapes and event names that the Codex bridge already
returns. The adapter in `internal/opencode` maps OpenCode sessions, messages,
and permission prompts to those shapes.

## Prerequisites

- An installed OpenCode 2.4.x executable that supports `opencode serve --port 0`
  and prints its listening address on stdout.
- Provider authentication and configuration available to the account running
  the host. The child inherits the caller's environment, including
  `XDG_CONFIG_HOME`, so installed provider auth is reused. Data and state
  directories are isolated per child (see below).
- A workspace directory for the supervisor. It is passed to OpenCode as the
  session location and is checked on every history read and resume.

## Starting the host

Use an explicit binary path when `opencode` is not on the host service's `PATH`:

```sh
/tmp/modex-host --background --workspace /absolute/path/to/project \
  --state-dir /private/path/modex-state \
  --backend opencode --opencode /usr/local/bin/opencode
```

`--supervise` runs the same supervisor in the foreground for a service manager.
`--serve` runs a connection-scoped host. Attach and stop are backend-agnostic:

```sh
/tmp/modex-host --attach --state-dir /private/path/modex-state
/tmp/modex-host --stop --state-dir /private/path/modex-state
```

Those two commands accept `--backend opencode` but ignore it; they operate on
the supervisor selected by the state directory.

After successful SSH attach, the supervisor's ready frame identifies its
backend and optional conversation capabilities. OpenCode currently sends
`"backend":"opencode","capabilities":[]`. Android offers chat, model
selection, attachments, and permission approvals. The optional Codex controls
for Usage, Plan mode, permission profiles, session actions, projects,
references, and takeover are hidden when their capabilities are absent.
Ready frames from older hosts use the legacy Codex defaults.

### Flags and validation

| Flag | Default | Behavior |
| --- | --- | --- |
| `--backend codex\|opencode` | `codex` | Selects the agent backend. |
| `--opencode <binary>` | `opencode` | OpenCode executable; only valid with `--backend opencode`. |

Rules enforced before any process is spawned:

- `--backend opencode` requires one of `--serve`, `--background`, `--supervise`.
- `--cli`, `--native-socket`, `--codex-home` and `--thread` are rejected with
  the OpenCode backend. Native CLI/TUI is Codex-only.
- `--opencode` requires `--backend opencode`; an explicit `--codex` requires
  `--backend codex`. Both binaries have defaults, so only explicit flags
  conflict.
- `--attach` and `--stop` skip these checks and behave the same for both
  backends.

## Isolated child process and security

The host spawns its own `opencode serve --port 0` child for the lifetime of the
backend:

- A random 32-byte password (64 hex characters) is generated per child and
  passed as `OPENCODE_PASSWORD`. The HTTP client authenticates with Basic auth
  as user `opencode`.
- `XDG_DATA_HOME` and `XDG_STATE_HOME` point to a temporary directory owned by
  the host and are removed when the child closes. `XDG_CONFIG_HOME` is inherited
  so the user's provider auth and configuration remain available.
- Only a loopback address is accepted from the child's listening line. The
  backend never listens on an external interface.
- Startup stderr is discarded. The password is never logged, written to the
  journal, or placed in SSH arguments. Upstream HTTP error bodies are not
  echoed, so prompts and credentials cannot leak through error text.
- Shutdown sends `SIGTERM`, then `SIGKILL` after a short grace period, and
  removes the isolated directory.

Because the data and state directories are temporary, OpenCode session state is
scoped to that supervisor process. Modex does not claim session continuity
across a supervisor restart for this backend.

## Modex protocol mapping

Method names and parameter names are unchanged from the Codex bridge. The table
lists the upstream OpenCode surface behind each implemented method.

| Modex method | OpenCode surface |
| --- | --- |
| `status` | `GET /api/info` and `GET /api/config`; returns `{workspace,config:{provider,model}}` from the configured default agent. |
| `threads/list` | `GET /api/session`; client-side limit, `next_cursor` is always null. |
| `threads/start` | `POST /api/session` with `{location:{directory}}` and an optional model ref; `standalone:true` uses `$HOME/Documents/Codex`. |
| `threads/read` | `GET /api/session/:id` and `GET /api/session/:id/message`; projects content parts into Modex messages. |
| `threads/resume` | `GET /api/session/:id` plus an exact directory check. No writer lock or fork. |
| `turn/start` | Applies the selected model to the session, then `POST /api/session/:id/prompt` with `{text,files}`. |
| `turn/steer` | `POST /api/session/:id/prompt` with `{text,files,delivery:"steer"}`; returns the active Modex turn id. |
| `turn/interrupt` | Checks the exact active Modex turn id, then `POST /api/session/:id/interrupt`. |
| `decision/respond` | `POST /api/session/:id/permission/:requestId/reply` with `{decision}`. |
| `models/list` | `GET /api/model?directory=…`, with a short lazy-discovery retry. |
| `client/attached` | Returns `{"updated":true}` and replays pending decisions. |

### `status`

No parameters. Calls `GET /api/info` to prove reachability and reads the layered
config documents to resolve the configured default agent's model. Returns:

```json
{"workspace":"/absolute/project","config":{"provider":"provider-id","model":"provider-id/model-id"}}
```

Provider or model may be empty when no default agent/model is configured.

### `threads/list`

Parameters: `limit` (1–100, default 20), `cursor` (validated string),
`all_workspaces` (boolean, accepted for client compatibility and does not change
the returned set in this slice), and `archived` (boolean). `archived:true` is
rejected. OpenCode has no Modex project split, so entries omit `projectId` and
clients fall back to workspace matching. Each entry contains `id`, `cwd`,
`modelProvider`, a bounded `preview`/`name` from the session title, and `model`
when set. Returns:

```json
{"data":[],"next_cursor":null}
```

The client walks OpenCode's cursor pages internally and slices to `limit`; the
Modex cursor is not surfaced.

### `threads/start`

Parameters: optional `model` (`provider/model` or a bare model id from the
catalogue), optional `workspace` (absolute path), and optional `standalone`.
With `standalone:true`, the host creates `$HOME/Documents/Codex` with private
permissions and creates the session there. Standalone cannot be combined with a
workspace or non-null `project_id`. Otherwise, a fresh session is created in
the connection workspace. A non-null `permission_mode` is unsupported. Session
creation uses `{location:{directory}}` and the resolved model ref
`{id,providerID}`. Returns:

```json
{"thread":{"id":"session-id","cwd":"/absolute/project","modelProvider":"provider-id","model":"provider-id/model-id"},"model":"provider-id/model-id"}
```

`model` is omitted from the thread when OpenCode has not attached one.

### `threads/read`

Parameters: `thread_id`, optional `include_turns`, `limit` (1–50, requires
`include_turns`), optional `cursor` (requires `limit`), optional `workspace`.
Metadata-only reads return `{thread}`. History reads return `{thread,
messages, active_turn, active_turn_id}`; a paged read (`limit` present) also
carries `history_revision`, and `next_cursor` appears when more messages remain.
`history_revision` is an opaque SHA-256 token prefixed with the projection
version; it changes whenever the thread id or any returned message changes.

Messages are returned oldest first. OpenCode is queried newest first, each page
is reversed, and its cursor toward older messages becomes `next_cursor`. An
unpaged history read walks up to 32 pages or 2000 messages, whichever comes
first.

Projected message roles:

- `user` — text from the user message.
- `assistant` — assistant text; text parts of one message collapse into one
  entry.
- `reasoning` — one entry per reasoning part, id `<messageID>/r<N>` (1-based),
  with `summary_parts`.
- `command` — tool parts, id from the tool call id (with stable fallbacks),
  `command`, `cwd`, `status`, nullable `exit_code`, and bounded output text.

Tool output is capped at 64 KiB per item with a visible truncation notice.
Failed assistant messages carry `status:"failed"` and a fixed `turn_notice`
(`category:"turn_failed"`).

### `threads/resume`

Parameters: `thread_id`, optional `workspace`. Reads the session, verifies the
stored directory equals the selected connection/workspace, and returns
`{thread,model}`. There is no writer lock, fork, or permission override. A
session outside the selected workspace is refused.

### `turn/start`

Parameters: `thread_id`, `text`, optional `model`, optional `workspace`,
optional `attachments`, optional `effort`, and `references`.

- `attachments` is an array of `{path,kind}` where `path` is absolute and `kind`
  is `image`, `file` or empty; at most 32 entries. Each becomes an OpenCode file
  input `{"uri":"file://<path>","name":"<base name>"}`. OpenCode infers MIME;
  image and file kinds are validated but do not select a different upstream
  field.
- Confirmed prompts carry attachment preview metadata to Android, including
  file-only prompts and follow-ups. History restores local file paths from
  OpenCode's persisted attachment sources. Modex does not expose inline
  attachment bytes or create previews for remote URLs.
- A non-empty `references` array is rejected. Codex references have no OpenCode
  equivalent in this slice.
- `effort` is accepted and validated for client compatibility but is not
  forwarded; OpenCode 2.4.1 exposes no reasoning-effort parameter here.
- A requested model is split into `provider/model` (or resolved from the
  catalogue for a bare id) and applied to the session before prompting. No
  agent is set by this backend.

Returns the accepted user message id as the turn id:

```json
{"turn":{"id":"user-message-id","status":"inProgress"}}
```

### `turn/steer`

Parameters: `thread_id`, `turn_id`, `text`, and the same optional model,
attachments and effort fields accepted by `turn/start`. Modex checks that the
requested id is the exact active turn, then submits the prompt with
`delivery:"steer"`. It returns the same active turn id and emits the accepted
user message with its own item id. Modex checks the session workspace before
sending prompts or interrupting a session.

### `turn/interrupt`

Parameters: `thread_id`, `turn_id`. Modex compares the requested id with the
currently active synthesized turn immediately before the OpenCode request.
Missing, stale or completed ids are rejected locally and never reach OpenCode.
OpenCode's interrupt endpoint accepts no expected turn id, so a narrow race
remains if the active turn changes between Modex's snapshot and the HTTP call.
An accepted interrupt marks that turn so a later idle event completes it as
`interrupted`.

### `models/list`

Parameters `limit`/`cursor` are validated for compatibility; the catalogue is
returned in one page. Each entry is projected to:

```json
{"id":"provider/model","model":"provider/model","displayName":"model","defaultReasoningEffort":"","supportedReasoningEfforts":[]}
```

The response is `{data:[…],next_cursor:null}`. OpenCode has no reasoning-effort
catalogue, so both effort fields are empty. The client makes up to four attempts
at 250 ms intervals when provider discovery returns an empty catalogue.
Model choices keep the provider prefix when creating, reading or resuming a
chat. Older bare model IDs still work if exactly one provider offers the ID.
If several do, select the desired provider in the picker before retrying.

### `client/attached`

Parameter: `attached` (boolean). Returns `{"updated":true}`. When `attached` is
true, the host first emits `decision/reset`, then re-emits every pending
`decision/required`, matching the Codex attach behavior.

## Events

The host subscribes to OpenCode's SSE feed at `GET /api/event`. The v2 envelope
is `{id,type,data}`; event ids are deduplicated across reconnects. The feed can
close mid-turn, so the reader reconnects with bounded backoff (500 ms to 8 s)
while a polling reconciler periodically reads the active session and its recent
messages (about once per second while a turn is in flight). The reconciler fills
in terminal states, missed parts and pending permissions. Replayed event ids are
ignored and already-emitted items are not re-emitted.

OpenCode `session.*`, `permission.*` and legacy `message.*` events are
synthesized into the Codex-shaped events the Android client already consumes:

| Event | Projection |
| --- | --- |
| `turn/started` | Emitted when the prompt is accepted; `turn.id` is the user message id. |
| `item/started` | `agentMessage` on first text, `reasoning` per part, `commandExecution` per tool. |
| `item/agentMessage/delta` | Streamed assistant text. |
| `item/commandExecution/outputDelta` | Not emitted in this slice; tool output is delivered in `item/completed`. |
| `item/reasoning/summaryTextDelta` | One summary section per reasoning item. |
| `item/completed` | Final assistant text, reasoning summary, or command output/status; `exitCode` is null. |
| `turn/completed` | `{id,status,durationMs}` where status is `completed`, `failed` or `interrupted`. |
| `decision/required`, `decision/resolved`, `decision/reset` | Permission requests and their lifecycle. |
| `error` | Fixed notice for failed or unsupported turns. |

Reasoning items are emitted as `<messageID>/r<N>` with a single summary section
at index 0. Empty assistant text, empty reasoning, and tool-only turns do not
create empty chat bubbles. A completed turn carries the accumulated assistant
text so a non-prefix replacement still renders correctly.

Failure notices are fixed strings, not raw upstream errors:

- `turn_failed`: "OpenCode could not complete this turn. Review the conversation
  before trying again."
- `unsupported_input`: "OpenCode is waiting for an input form that this Modex
  build cannot display. Resolve it on the computer and retry."
- Pending-permission cap: "OpenCode requested too many pending approvals. Review
  them on the computer and reconnect."

## Permissions and decisions

OpenCode emits `permission.asked` (and sometimes `permission.updated`) with an
id, session, action, resources, optional save list and source. `permission.replied`
resolves a request independently, including when it was answered elsewhere.

Each pending request becomes a Codex-shaped decision:

- `decision_id` is `<sessionID>/<requestID>`, scoped so identical request ids in
  two sessions cannot collide.
- `thread_id` is the OpenCode session id; `turn_id` is the active turn's user
  message id (empty when no turn is active); `item_id` comes from the permission
  source when present.
- The permission action is classified as `shell`, `write`, `read`, `web`,
  `subagent`, `external_directory`, `doom_loop`, or a sanitized action label.

Command approvals (`item/commandExecution/requestApproval`) carry `kind`,
`availableDecisions` (`accept`, `decline`, `cancel`), `command` (joined
resources), `cwd`, `reason`, optional `additionalPermissions` (up to 16 save
entries), and `networkApprovalContext` (`{host,port}` parsed from an HTTP(S)
resource).

Write and external-directory approvals (`item/fileChange/requestApproval`) carry
only `reason` and optional `grantRoot`. They offer `decline` and `cancel`; there
is no diff viewer, and `accept` is refused with "decision not offered by
OpenCode".

Reply mapping:

| Modex decision | OpenCode reply |
| --- | --- |
| `accept` | `once` |
| `decline` | `reject` |
| `cancel` | `reject` |

`accept` is never mapped to `always`; session-wide persistence is not exposed and
there is no Android button for it. The result must be exactly
`{"decision":"accept"|"decline"|"cancel"}`; unknown fields are rejected. A
decision is consumed before delivery, so a failed delivery returns "decision
delivery failed; do not replay". Completing a turn resolves its still-pending
decisions; attach replays only decisions that are still pending.

At most 128 approvals may be pending across the backend. A new request past that
cap emits a fixed error and fails the turn visibly rather than dropping the
request silently.

Answered approvals stay suppressed for the lifetime of the OpenCode bridge,
including after an uncertain delivery. The bridge retains up to 16,384 pending
and consumed identities before declining new approvals.

## Unsupported methods

The following methods return `<method> is unsupported by the opencode backend`.
They are not partially implemented and must not be described as working with
this backend:

| Method | Reason |
| --- | --- |
| `threads/archive`, `threads/unarchive`, `threads/delete` | No OpenCode lifecycle mapping in this slice. |
| `threads/fork`, `threads/takeover` | No OpenCode equivalent in this slice. |
| `threads/mode`, `threads/status`, `threads/permissions` | Codex-specific session features. |
| `plans/respond` | No plan mode. |
| `projects/list`, `projects/create` | OpenCode has no Modex project concept. |
| `references/list` | No Codex reference catalogue. |

Unknown methods and unknown parameters also fail before any upstream call.
`archived:true`, non-null `project_id` on ordinary session creation,
non-null `permission_mode`, and non-empty `references` are rejected.

Questions and forms are not mapped. OpenCode 2.4.1 has no `/api/question`
endpoint; it uses a different "forms" API. `form.created`, `question.created`,
`question.asked`, `input.asked`, and any unknown `*.asked` event fail the turn
with the fixed `unsupported_input` notice instead of leaving it stalled. A
`decision/respond` payload containing `answers` without `decision` returns
"questions are unsupported by the opencode backend".

## Known limitations

- `always` permission persistence is unavailable. One-shot accept never persists
  a grant.
- File-change approvals are decline/cancel-only; there is no diff viewer.
- No plan mode, projects, references, fork, takeover, archive, unarchive or
  delete.
- `threads/list` accepts a cursor for compatibility but always returns the first
  `limit` sessions; server pagination is not surfaced through a Modex cursor.
- Questions/forms are unsupported; affected turns fail visibly.
- A session's `model` and `modelProvider` are empty until OpenCode attaches a
  model. Android always sends a model for `threads/start` and `turn/start`, so
  this is a display fallback rather than a crash path.
- `effort` is accepted but not applied, and the model picker exposes empty
  reasoning-effort fields.
- Native CLI/TUI (`--cli`) remains Codex-only. There is no OpenCode native
  gateway in this slice.
- Session state lives in a temporary isolated data directory, so cross-restart
  OpenCode session continuity is not claimed.

## Operational limits

- The host bridge still uses the existing 4 MiB frame cap and per-request
  deadlines. Upstream HTTP requests time out after 20 seconds and responses are
  capped at 8 MiB after projection. For prompt confirmations and history,
  Modex discards embedded attachment base64 while reading the upstream
  response. Those endpoints have a separate 256 MiB wire budget to accommodate
  the Android limit of eight 20 MiB files. An oversized history page is
  retried with fewer entries and the same cursor. If a single native message
  still exceeds the limits, the read fails visibly. A prompt is never reposted
  because its response exceeded a limit.
- `turn/interrupt` and `decision/respond` are handled ahead of queued ordinary
  commands so a long `turn/start` cannot starve them. The ordinary command queue
  holds 16 items; overflow returns an explicit error.
- A failed or timed-out read does not close the tunnel. An uncertain `turn/start`
  must not be replayed automatically; reconnect and inspect the session.

## Verification boundary and next stages

At revision `771c518`, `go test -race ./...`, `go vet ./...`, and the
`modex-host` build passed. An isolated supervisor using OpenCode 2.4.1 passed
standalone creation, start, steer and exact-turn interrupt checks. Paid provider
completion, physical Android behavior and service activation were not checked.
The decision/synthesizer lock inversion was removed by avoiding the reverse
lock edge; a delayed interrupt response also checks turn identity before it can
mark the session interrupted.

Later stages remain open: Stage 2 adds capability reporting and completes
conversation parity for attachments, permissions and forms; Stage 3 adds
persistent state and session navigation/lifecycle; Stage 4 considers utilities
such as projects, references, plan mode and agents where OpenCode has stable
contracts.
