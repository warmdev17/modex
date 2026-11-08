# Modex

Android native control of Codex on your own laptop or VPS, preserving the target's provider and configuration. Early development: this repository currently contains a host diagnostic, not a usable mobile product.

## First executable milestone

Requires Go 1.23+ and an installed Codex CLI. The diagnostic launches a short-lived app-server over stdio, asks Codex for effective configuration, and lists up to 20 stored threads for an explicit workspace. It does not start model turns, resume active threads, or require an OpenAI login of its own.

```sh
go test ./...
go run ./cmd/modex-host --workspace /absolute/path/to/project
```

Optional flags: `--codex /path/to/codex`, `--codex-home /path/to/isolated/codex-home`.

`--profile` currently fails explicitly: named profile support must be established against Codex's actual configuration API before it is advertised. The target process inherits the launching shell's environment and credentials. An empty configuration model means Codex has not supplied an explicit configured model; the diagnostic does not guess a runtime default.

Output contains workspace, provider, configured model and thread IDs/directories/providers. It intentionally excludes chat content, raw config, endpoints, credentials and raw server errors. Response frames are capped at 4 MiB, execution at 20 seconds. This is a local diagnostic, not a network service. Codex itself may write normal runtime data inside its home; use an isolated home for experiments.

## Product direction

- Kotlin / Jetpack Compose Android client over SSH.
- Host on Linux, Windows and macOS from the first public release.
- Existing Codex sessions, streamed events, approvals and user questions.
- Background completion/decision notifications through a minimal push relay.
- API keys and execution stay on the target machine.

Shared live ownership with the official CLI/Desktop is an unresolved compatibility requirement. Stored history listing is not live attachment. No automatic fork, lock deletion, uncertain-message replay or automatic approval is allowed.

See [design](docs/design.md), [first milestone plan](docs/plan.md) and [current state](PROJECT_STATE.md). No license or commercial model has been selected yet.

## Connection-scoped host bridge

```sh
go run ./cmd/modex-host --serve --workspace /absolute/path/to/project
```

Send one JSON object per line, with a positive integer ID. Replies have the same ID and either `result` or `error`:

```json
{"id":1,"method":"status"}
{"id":2,"method":"threads/list","params":{"limit":20}}
{"id":3,"method":"threads/list","params":{"limit":20,"cursor":"cursor-from-next_cursor"}}
{"id":4,"method":"threads/read","params":{"thread_id":"existing-thread-id"}}
{"id":5,"method":"threads/resume","params":{"thread_id":"existing-thread-id"}}
```

`threads/read` returns metadata only, not chat history. Lists return `data` and `next_cursor`; limit is 1–100. Only threads with the exact selected workspace are exposed or resumed. Resume preserves the original thread ID and does not override model, provider or approvals. Codex can reject a locked or incompatible thread; Modex never forks automatically. Resuming may initialize that thread's configured MCP servers.

The bridge owns one app-server for the lifetime of its stdio connection and closes it at EOF. Each Codex exchange has a 20-second deadline. Protocol failures/timeouts close the Codex connection without replay; normal RPC rejections leave it usable. Text turns and supported approval/question responses are exposed below. Arbitrary Codex methods remain unavailable. This is not a detached daemon: disconnecting SSH stops its child. Native SSH/mobile integration remains unimplemented.

### Send a text turn (connection-scoped preview)

After selecting an existing thread, send:

```json
{"id":6,"method":"turn/start","params":{"thread_id":"existing-thread-id","text":"your prompt"}}
```

Codex notifications are forwarded as event lines while the turn runs, for example:

```json
{"event":"turn/started","params":{"threadId":"...","turn":{"id":"..."}}}
```

The final response keeps the request ID. Modex validates the thread's exact workspace before sending. It exposes text turns and supported decision responses; interruption, attachments and background supervision remain unimplemented. A request timeout closes the Codex connection because replaying an uncertain turn could duplicate work.

### Respond to approvals and questions

Supported Codex server requests: `item/commandExecution/requestApproval`, `item/fileChange/requestApproval` and `item/tool/requestUserInput` (Codex 0.167.0 schema).

```json
{"event":"decision/required","params":{"decision_id":"1","method":"item/commandExecution/requestApproval","thread_id":"...","turn_id":"...","item_id":"...","context":{"command":"git status","cwd":"/workspace"}}}
{"id":7,"method":"decision/respond","params":{"decision_id":"1","result":{"decision":"accept"}}}
```

Approval responses accept `accept`, `decline` or `cancel` only and honor `availableDecisions` when provided. Session-wide permission grants and policy amendments are unavailable. Correlate file-change approvals with the preceding item events to review the diff before accepting.

For a question, preserve its IDs and send all answers:

```json
{"id":8,"method":"decision/respond","params":{"decision_id":"2","result":{"answers":{"question-id":{"answers":["selected label or text"]}}}}}
```

Each decision has a connection-local token. Invalid responses leave it pending; a valid response consumes it before writing to Codex, so uncertain delivery cannot be replayed. Completion/resolution notifications invalidate stale decisions. Unknown request methods and secret questions receive a generic unsupported-request error; no automatic approval occurs.

Review context intentionally contains task content (command, permissions, question text) needed by the authenticated client. It is not a credential redactor for user-authored text. Raw configuration and upstream errors remain suppressed. There is no decision persistence across disconnects yet. If Codex awaits a decision before its RPC acknowledgement, the existing 20-second RPC deadline still applies; decisions after the acknowledgement have no additional timer in this milestone.

### Opt-in real-provider verification

This test makes paid requests using the target's existing Codex credentials. It creates a temporary read-only test workspace/thread, verifies direct inference, same-ID handoff, Modex streaming, and one real command approval. Only the exact `printf MODEX_APPROVAL_OK` command (optionally wrapped by a known Bash invocation) can be accepted; other approval requests are canceled. It verifies the command's exit code and stdout, decision acknowledgement and final turn status. It leaves global model/provider configuration unchanged.

```sh
GOCACHE=/tmp/modex-go-cache go build -o /tmp/modex-host ./cmd/modex-host
python3 scripts/live_provider_smoke.py --host /tmp/modex-host --model gpt-6.1-sol
python3 -m unittest discover -s scripts -p 'test_*.py'
```

Python 3 standard library only. Omit `--model` to use the configured model; `--report /path/to/report.json` writes a sanitized summary. The fixture workspace is removed after the run; Codex may retain ordinary test session records in its own home. This approval fixture requires Linux/Bash; host cross-compilation is not runtime validation on Windows or macOS.
