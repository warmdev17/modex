# Modex

Prebuilt distribution is being prepared: [installation and platform support](docs/install.md), [maintainer signing/release setup](docs/releasing.md). The pipeline creates draft releases; no public release is implied until the owner publishes one.

Android native control of Codex on your own laptop or VPS, preserving the target's provider and configuration. Early development: native Android and host/CLI source builds are available; public prebuilt releases are being prepared.

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

Create a fresh chat through Modex (not a fork of another session):

```json
{"id":9,"method":"threads/start"}
{"id":10,"method":"threads/start","params":{"model":"gpt-6.1-sol"}}
```

The first form inherits the configured model. The second selects a model for the new thread only. Both inherit the target provider and permissions, use the connection's fixed workspace, and return allowlisted `thread` metadata plus the model reported by Codex. The command cannot override cwd, provider, sandbox or approval policy. Use the returned ID with `turn/start`; an uncertain creation is never retried automatically.

`threads/read` returns metadata by default; pass `include_turns:true` to load stored user/assistant text and active-turn status (see docs/history-contract.md). Lists return `data` and `next_cursor`; list items also include bounded `name` and `preview` fields when Codex provides them. Limit is 1–100. Only threads with the exact selected workspace are exposed or resumed. Resume preserves the original thread ID and does not override model, provider or approvals. Codex can reject a locked or incompatible thread; Modex never forks automatically. An `active_writer` rejection means another Codex client currently owns the session. Resuming may initialize that thread's configured MCP servers.

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

The final response keeps the request ID. Modex validates the thread's exact workspace before sending. It exposes text/image/file turns and supported decision responses; interruption remains unimplemented. Background supervision is described below. A request timeout closes the Codex connection because replaying an uncertain turn could duplicate work.

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

This test makes paid requests using the target's existing Codex credentials. It creates a temporary read-only test workspace/thread, verifies direct inference, same-ID handoff, Modex streaming, one real command approval, and creating a new chat entirely through Modex. Only the exact `printf MODEX_APPROVAL_OK` command (optionally wrapped by a known Bash invocation) can be accepted; other approval requests are canceled. It verifies the command's exit code and stdout, decision acknowledgement and final turn status. It leaves global model/provider configuration unchanged.

```sh
GOCACHE=/tmp/modex-go-cache go build -o /tmp/modex-host ./cmd/modex-host
python3 scripts/live_provider_smoke.py --host /tmp/modex-host --model gpt-6.1-sol
python3 -m unittest discover -s scripts -p 'test_*.py'
```

Python 3 standard library only. Omit `--model` to use the configured model; `--report /path/to/report.json` writes a sanitized summary. The fixture workspace is removed after the run; Codex may retain ordinary test session records in its own home. This approval fixture requires Linux/Bash; host cross-compilation is not runtime validation on Windows or macOS.

## Thread permissions

Android connection failures identify the failed step: local configuration/key,
SSH authentication or network, verified fingerprint mismatch, remote executable,
supervisor attachment or journal replay. Messages are fixed guidance; raw SSH
errors and bounded remote stderr are never displayed or logged. Network guidance
cannot distinguish an outdated IP from firewall/sshd/algorithm failures by itself.
Disconnect intentionally does not show a transport failure. No automatic prompt
retry, host-key acceptance or cursor reset is performed.

Android's New conversation dialog offers Host default, Read-only, Workspace write
and Full access. Host default sends no override and may inherit full access.
Read-only and Workspace write request `on-request` approvals routed to the user;
Full access explicitly requests unrestricted sandbox access and `never` command
approvals. Host policy can reject a mode; file-change acceptance still requires
a future diff viewer. This does not change the global Codex configuration.

The conversation menu's Session settings shows model/provider and sandbox,
approval policy and reviewer reported by Codex at create/resume. Unknown fields
remain unknown, including restored history before resume. These labels summarize
the legacy sandbox policy, not a complete permission-profile or tool-scope audit.
Opening an existing session sends no permission overrides. A new host build is
required; old hosts reject explicit modes. Rebuild/restart the supervisor only
when ready to detach existing clients, then reset the phone cursor as needed.

## Background supervisor (Linux/macOS)

From the repository root, use:

```sh
make host          # build and start in the background
make host-attach   # attach JSON stdin/stdout; EOF detaches
make host-stop     # stop the supervisor
```

Defaults: binary `/tmp/modex-host`, workspace = current repository, state directory
`$HOME/.local/state/modex-phone`, Codex executable `codex`. Match the binary and
state paths in the phone's connection settings. Override them when needed:

```sh
make host WORKSPACE=/absolute/path/to/project STATE_DIR=/private/path/modex-state CODEX=/path/to/codex
make host-stop STATE_DIR=/private/path/modex-state
```

`make host-build` only builds; `HOST_BINARY=/absolute/path/modex-host` changes the
output path (its parent directory must exist). Builds replace the binary atomically;
an already running supervisor keeps its old executable until explicitly stopped
and started again. Starting while one is running fails without restarting it.
The default `/tmp` binary may be removed at reboot; run `make host` to rebuild.
These targets inherit your Codex configuration and do not enable approval prompts
or select an approval-testing wrapper automatically.

Build a stable executable first (do not use an ephemeral `go run` binary for a long-lived supervisor):

```sh
go build -o /tmp/modex-host ./cmd/modex-host
/tmp/modex-host --background --workspace /absolute/path/to/project --state-dir /private/path/modex-state
/tmp/modex-host --attach --state-dir /private/path/modex-state
```

`--background` waits until its persistent bridge answers status, then returns. `--supervise` runs the same supervisor in the foreground for service managers. The supervisor keeps one Codex app-server and its stdin open when a client disconnects. There is no automatic restart, prompt retry or fork. Use one state directory per workspace/supervisor. An existing state directory must belong to the current user with permissions `0700`; its endpoint file is `0600`. Keep that directory on a trusted local filesystem and never share its token.

The attach acknowledgement includes a non-secret instance ID:

```json
{"instance":"supervisor-instance-id","ready":true}
{"seq":1,"message":{"id":2147483647,"result":{"workspace":"/project","config":{"provider":"custom","model":"configured-model"}}}}
```

The supervisor journal wraps every bridge response/event in `{seq,message}`. The first status response is the supervisor's internal readiness probe. Send ordinary Modex JSON commands on stdin; the supervisor forwards each submitted command once. Keep stdin open while observing events; EOF detaches immediately. Only one controlling client may attach at a time.

Save the last fully handled `seq` together with `instance`. Reconnect without resending prompts:

```sh
/tmp/modex-host --attach --state-dir /private/path/modex-state --cursor 42 --instance supervisor-instance-id
/tmp/modex-host --stop --state-dir /private/path/modex-state
```

The cursor is required to be from the same supervisor instance. Replay is bounded to 1024 frames / 16 MiB in RAM; an expired cursor produces an explicit gap error. A slow client cannot stall journal collection; it may be disconnected or encounter a retention gap. A supervisor restart loses the journal and creates a new instance/token. Decisions remain in the live bridge across client disconnects, but cannot be answered twice; reconnect by cursor to receive missed requests. If submission delivery is uncertain, observe the journal rather than resubmitting the operation.

The transport listens on `127.0.0.1` only and requires a random token read from private state. Never expose that socket directly to the internet. Authenticated `--stop` requests shutdown and removes the owned endpoint/lock. Hard crashes may leave stale state: there is intentionally no lock deletion, PID-based kill or automatic recovery. Verify the old supervisor is stopped before manually recovering stale state.

Windows builds still support `--serve`; supervisor modes currently fail closed until private-state Windows ACL support is implemented. Linux has integration/runtime coverage; macOS currently has cross-build coverage only. This milestone does not install startup services, survive reboot or implement native mobile SSH.

Verification:

```sh
MODEX_HOST_BINARY=/tmp/modex-host python3 -m unittest discover -s scripts -p test_supervisor.py -v
python3 scripts/live_supervisor_smoke.py --host /tmp/modex-host --model gpt-6.1-sol
```

The first uses a fake Codex process and checks auth, exclusive ownership, disconnect/replay, CLI attach, orphan cleanup and stop. The second makes a paid real-provider turn, detaches after acknowledgement, reconnects by instance/cursor, verifies exact streamed output, then stops the fixture supervisor.

## Shared desktop CLI and phone sessions (Linux/macOS)

The supervisor owns one persistent Codex app-server. Android connects through
SSH; the real Codex terminal UI connects through a private Unix WebSocket gateway.
Both clients control the same loaded session. Closing a terminal or disconnecting
the phone does not stop a dispatched turn. Stop is an explicit user action.

After installing the two sibling binaries, set up the user service once:

```sh
modex service install --workspace /absolute/project
modex service start
cd /absolute/project
modex
# Equivalent entry point, with an optional exact-session shortcut:
modex cli --thread <session-uuid>
```

`modex` opens the existing title-based session picker; `0` creates a new native
conversation, `n`/`p` change pages, and `q` exits. The default state is
`~/.local/state/modex-phone`, shared by the CLI, service, phone and emulator.
Use `modex cli --state-dir /private/custom-state` for another supervisor.
`--workspace`, `--codex`, `--codex-home` and `--host-binary` remain available.

The supervisor endpoint publishes its native socket only in its private state
file. The socket is mode 0600 under a private directory, never a public TCP
listener. Native request IDs are multiplexed through the same backend and
approvals can be answered from either view; the first answer wins. Slow native
clients disconnect independently. Client unsubscribe never unloads another
client's session. Do not replay uncertain prompts after a transport failure.

Managed sessions remain owned by the service until it stops. An independent
Codex CLI/Desktop process outside Modex still owns a separate backend: close
that writer manually or explicitly use the legacy registered takeover. Modex
does not silently signal it. A new managed conversation becomes discoverable
once Codex persists it. Legacy JSONL sessions use bounded whole-history reads
with local page slicing when Codex explicitly rejects native history pagination;
native paginated sessions keep their native cursors.

An older running supervisor cannot gain the native gateway from a binary
replacement alone. Stop it **when idle**, then restart the updated host/service
using the same state directory. No profile wipe or session migration is needed.
The native TUI requires a Codex version supporting `--remote unix://PATH`.
Native-only tool requests require a connected native TUI; Android retains its
existing supported approval/input forms. Native Windows hosting still requires
WSL2. The gateway reuses coder/websocket under its [ISC license](docs/licenses/coder-websocket-ISC.txt).

## OpenSSH client transport

The desktop/CLI transport uses the system OpenSSH executable; it does not install SSH or upload the host. Start the supervisor on the target first, then build the client locally:

```sh
go build -o /tmp/modex ./cmd/modex
/tmp/modex --target user@server \
  --host-binary /opt/modex/modex-host \
  --state-dir /home/user/.local/state/modex
```

It runs only the remote host's `--attach` command over an exec channel, with no PTY. JSON commands go to stdin and journal envelopes come from stdout. Provider credentials and the supervisor token remain on the target. Client stdin EOF detaches; SSH loss does not stop the background supervisor. Retain the last fully handled sequence/instance and reconnect explicitly:

```sh
/tmp/modex --target user@server \
  --host-binary /opt/modex/modex-host \
  --state-dir /home/user/.local/state/modex \
  --cursor 42 --instance supervisor-instance-id
```

Optional flags: `--port 2222`, `--identity /local/private-key`, `--known-hosts /local/verified-known_hosts`, `--ssh /path/to/ssh`. Authentication uses existing OpenSSH keys/agent/config with `BatchMode=yes`; no password prompt enters the JSON stream. Host key checking is always strict. Verify and install the remote host key through your normal trusted process before connecting; the client never learns host keys automatically. No provider keys or supervisor tokens appear in SSH arguments.

SSH options include connection timeout and keepalives. Failed SSH/remote attach returns a generic error on stderr and never retries commands. To diagnose OpenSSH separately, use your normal terminal SSH connection. Remote exec uses POSIX shell quoting and requires absolute remote binary/state paths. Client binaries cross-build for Linux/Windows/macOS with OpenSSH installed; Windows **targets** remain unsupported for supervisor mode until private ACL support and shell-specific exec quoting are implemented. The native Android client is described below.

Real loopback SSH verification uses an isolated sshd, ephemeral host/client keys and known_hosts, without changing system SSH configuration:

```sh
MODEX_CLIENT_BINARY=/tmp/modex MODEX_HOST_BINARY=/tmp/modex-host \
  python3 -m unittest discover -s scripts -p test_ssh_transport.py -v
```

It checks quoting of shell metacharacters, exact JSON transport, rejection of mismatched host keys, and two authenticated attachments with cursor continuity against a real supervisor using a fake provider. This is Linux loopback integration, not remote VPS or Android runtime validation.


## Native Android client (first slice)

`android/` is a Kotlin/Jetpack Compose app, Android 8.0/API 26 or newer. It uses native Java SSH (mwiede JSch plus Bouncy Castle for Ed25519), pins the verified SSH host fingerprint and imports an **unencrypted private key** through the system file picker. Use a dedicated SSH key for this prototype. The single saved host, private key and session snapshot are encrypted with an Android Keystore AES-GCM key in the app no-backup directory. Forget saved host removes ciphertext and its encryption key. A device lock and a dedicated SSH key are recommended. Importing a file does not delete its original copy.

Build with JDK 17 or 21, Android SDK 36 and installed build tools:

```sh
cd android
./gradlew :app:assembleDebug :app:testDebugUnitTest :app:lintDebug
```

APK: `android/app/build/outputs/apk/debug/app-debug.apk` from the repository root. Install with Android Studio or `adb install -r android/app/build/outputs/apk/debug/app-debug.apk`. The wrapper pins Gradle 8.14.3; first build needs internet to download tools and dependencies.

Start the host supervisor as documented above. In the app enter hostname/IP, SSH port/user, absolute remote binary and state-directory paths, then import the key. Obtain the SSH server's SHA256 fingerprint **on the target or through another trusted channel**, for example `ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub -E sha256` on Linux. Do not trust an unverified network scan. The pin must match the host key algorithm negotiated with the server.

Connect, choose an existing workspace session or create a new one, then send a message. New chats initialize model and reasoning from the host configuration (or the advertised model default effort). The model picker has no Keep current option. Choosing Use model immediately saves model and effort per thread in the encrypted snapshot, before sending; reconnecting or reopening the app preserves the choice, and later turns explicitly send it. The composer shows the model and effort, or Reasoning unavailable when Codex supplies no effort information. Unknown custom models do not inherit an unrelated model’s effort; HTTP 403 may mean that model is not allowed for your provider key. Choose an allowed model explicitly for a new chat. The left session drawer offers search over loaded sessions, Refresh and Load more. Labels prefer the Codex session name, then first-message preview, then an untitled label; failed turns show fixed error guidance without raw provider details. The UI shows streamed live messages and one-time command approvals/non-secret questions. Approval review separates the exact command, working directory, reason and any extra filesystem/network permissions; command text can be selected and copied. Disconnect leaves the target supervisor working. After disconnect, idle persisted sessions release their Codex writer so Desktop/CLI can resume the same ID. Running turns and pending approvals continue; those workers release after completion while disconnected. Use Disconnect for handoff: removing the Android task may leave the foreground SSH service connected. New empty chats stay loaded until their first completed turn. Reconnecting and sending reacquires the same session, rejects another active writer and never automatically forks or replays a prompt. Reconnect uses the same instance and last handled cursor, without resending prompts. Reset clears local session/cursor state; observe remote state before retrying an uncertain operation.

The native chat uses Catppuccin Mocha with blue accents, a left conversation drawer, and a restrained multiline composer. Drafts survive session switches; rejected sends keep the draft. Assistant messages render CommonMark headings, emphasis, links, quotes, lists and GFM tables. Fenced/indented code uses a selectable monospace block with horizontal scrolling and Copy code. Code is not syntax highlighted. HTML is inert text; images are not fetched; only HTTP(S) links can open externally.

The app restores the saved connection and selected conversation when reopened; a foreground connection notification supports multitasking. A saved session is observed, never automatically resubmitted. `threads/read` with `include_turns` loads stored user/assistant text on selection/reconnect. Update the host binary along with the APK.

Current limits: text-only history, no image/tool transcript display; file-change requests can be declined/cancelled but **cannot be accepted** until a diff viewer exists. No passphrase-key UI, multiple saved hosts, SSH agent/config aliases, automatic host installation, done/decision push relay or bubbles yet. The connection notification is local foreground-service status. Android may terminate or time-limit foreground services (including data-sync service limits); reopening restores the encrypted snapshot and reconnects. Force-stop/reboot does not auto-start a service. A changed supervisor instance or expired journal still requires Reset session cursor; no prompt is retried. Windows supervisor targets remain unavailable. This is a development APK, not a published release.


Opt-in Android SSH integration (Linux host with `sshd`, running `emulator-5554`):

```sh
go build -o /tmp/modex-android-host ./cmd/modex-host
cd android
./gradlew :app:assembleDebug :app:assembleDebugAndroidTest
cd ..
adb -s emulator-5554 install -r android/app/build/outputs/apk/debug/app-debug.apk
adb -s emulator-5554 install -r android/app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk
python3 scripts/test_android_transport.py --host /tmp/modex-android-host
```

This uses temporary Ed25519 keys, an isolated loopback sshd and a real supervisor with a fake Codex provider. It verifies Android SSH status round-trips, same-instance cursor reconnect and rejection of a wrong fingerprint. It force-stops only the test app and removes its temporary key/host fixtures. It does not test paid inference or all UI actions.


Approval UI screenshot fixture (installed debug + test APKs, no SSH/command execution):

```sh
adb -s emulator-5554 shell am instrument -w -e preview approval dev.modex.test/dev.modex.TransportSmoke
adb -s emulator-5554 exec-out run-as dev.modex cat files/approval-light.png > /tmp/approval-light.png
adb -s emulator-5554 exec-out run-as dev.modex cat files/approval-dark.png > /tmp/approval-dark.png
```


Continuity checks on the emulator (isolated fixture; clears the test app saved host):

```sh
adb -s emulator-5554 shell pm grant dev.modex android.permission.POST_NOTIFICATIONS
adb -s emulator-5554 shell am instrument -w -e preview storage dev.modex.test/dev.modex.TransportSmoke
python3 scripts/test_android_transport.py --host /tmp/modex-host --continuity
```

The continuity fixture loads stored text over native SSH, saves the host/key, force-stops the app, opens a fresh process, checks automatic attach plus restored history, then forgets the fixture host. It uses a fake provider and never submits a prompt. The separate storage fixture verifies ciphertext, round-trip, tamper rejection and deletion with Android Keystore.


Render-only Android chat fixture (emulator; no SSH/model request):

```sh
adb -s emulator-5554 shell am instrument -w -e preview chat-ui dev.modex.test/dev.modex.TransportSmoke
```

It captures chat/drawer/composer/table screenshots and checks code copy, session selection and draft retention after a rejected send. Install both debug APKs first as above.

## Android attachments

In an open chat, tap **+** and choose **From this phone** or **From connected computer**.
The phone source uses Android's document picker; the computer source browses the
SSH account's readable folders in a full-screen browser, starting at home (`~`).
Each chat remembers its last successfully opened folder, including after app restart. Select files,
review/remove the composer previews, then Send. An attachment-only message is supported.
The separate OpenAI knot beside + opens the model/reasoning picker and follows the
Catppuccin Mocha Blue accent.

The SSH server must enable SFTP (for OpenSSH, `Subsystem sftp internal-sftp`).
Phone files stream over the existing pinned SSH connection into private directories
under `<supervisor state>/attachments`; no provider credentials reach the phone.
Uploads publish only after completion, with a limit of **8 files per message** and
**20 MiB per file**. PNG/JPEG/GIF/WebP use Codex native image inputs; other files
are path references for agent tools, subject to the session's sandbox/approvals.
Image support also depends on the selected model/provider.

Images show thumbnails in the composer and sent messages; tap to open a full-screen
viewer with pinch zoom. Tap files to read a bounded UTF-8 text preview or use **Open
with** for PDF and other formats supported by installed phone apps. Previews download
via SFTP into an app-private cache (20 MiB per file, approximately 100 MiB total).
Only cached preview files can receive temporary read access through Android's file
provider; no SSH key or connection snapshot is shared. Remote files must remain
readable for uncached previews; this is not an offline file archive. Older hosts
provide filename labels without tappable attachment metadata after history reload.

Completed uploads are retained for conversation history, including abandoned drafts;
there is no automatic retention cleanup yet. Draft selections survive rotation and
reconnect in the same app process, but are not restored after process death. No
failed/uncertain prompt is automatically resent. Update both the host and APK;
restart the host only when its active work can safely detach.

Attachment integration uses a temporary loopback SSH/SFTP server and emulator:

```sh
python3 scripts/test_android_transport.py --host /path/to/modex-host --adb /path/to/adb --attachments
```

This does not restart a production supervisor or install on a physical phone.
