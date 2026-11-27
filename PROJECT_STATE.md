# Modex project state

## Combined agent routing and Antigravity controls

Updated 2026-11-27T20:33:08+07:00 on `feat/antigravity-session-controls`.
Integrated shared agent routing through `a95ab4b` with Antigravity controls
through `f5334e6`. Both plans and the bilingual agent guide describe the
scoped capabilities and the managed OpenCode profile.

- Ran `umask 022; go test -race ./... -timeout=120s`, `go vet ./...` and the
  isolated three-runtime supervisor test on the combined tree. All passed.
  The managed OpenCode permission snapshot now stays bound to its original
  turn, and hitting the permission cap completes the turn instead of locking
  the reconciler. An earlier standalone round-trip test once hit reply
  ordering variability and passed in the final combined suite.
- Combined debug APK uses the existing development certificate and fixture
  versionCode 1002. JVM tests reported 180 passes and no failures. The
  upgrade fixture preserved encrypted state from versionCode 39. Android
  smoke tests confirmed runtime filtering, AGY model/thinking controls,
  Codex read-only transcripts and SSH cursor reconnect and wrong-pin
  rejection. The transcript test passed after using semantic clicks for
  controls that move during animation.
- Existing public APK and branch history on the remote have not changed.
  Physical-device verification and OpenCode provider inference remain
  unverified. AGY reports invocation status; child transcripts remain
  unavailable in the tested ACP runtime.
- Built the private installable preview `0.1.0-dev.39` with versionCode 48 and
  the established development certificate. Unit tests, lint, package/signature
  verification, the code-47-to-48 encrypted-state upgrade, shared-agent chats,
  Antigravity options and agent-activity emulator smokes passed. The exact APK
  is `/home/warmdev/Downloads/modex-0.1.0-dev.39-agent-routing.apk`, SHA-256
  `d21f03972add22ac3be4dee53cf9566e1673f5b54e8bccdf09f5ccb7e6c48c5b`.
  It has not been published, installed on a physical phone or tagged.

## Shared agent chats

Updated 2026-11-27T19:08:00+07:00 on `feat/shared-agent-chats`. Android
routing and the sidebar landed in `4f934c3`. The
[plan](docs/superpowers/plans/2026-11-27-shared-agent-chats.md) covers
agent-neutral projects, explicit per-chat routing and isolated OpenCode state.

- The isolated supervisor fixture created three chats in one project,
  checked per-agent lists and the legacy list, exercised concurrent
  OpenCode and AGY turns, rejected a stale Stop and a foreign workspace,
  stopped OpenCode, and reopened its history after a restart.
- Android Gradle unit, lint and APK tasks passed offline with 177 JVM tests.
  Pixel 4 XL emulator verified agent labels and filtering, scoped OpenCode
  creation and the same-signature upgrade from fixture code 1000 to 1001.
- Real OpenCode 2.4.1 started with a private database, reported a model
  catalogue and created a chat. Its native form, permission and history
  endpoints returned the expected lists. The managed profile did not list
  OpenCode Go. Live OpenCode inference was not verified.
- Host review fixes and the combined result are recorded above.

## Antigravity stage 3

Updated 2026-11-27T18:21:00+07:00 on `feat/antigravity-session-controls`.
ACP options/events landed in `8fce8ab`. This milestone adds acknowledged model
and thinking choices per chat, guarded save/reload, draft retention until turn
acknowledgement, and read-only invocation activity in Android. See the
[design](docs/superpowers/specs/2026-11-27-antigravity-session-controls-design.md)
and [plan](docs/superpowers/plans/2026-11-27-antigravity-session-controls.md).

- On this milestone's working tree, `umask 022; go test -race ./... -timeout=110s`,
  `go vet ./...` and `git diff --check` passed. Independent re-review found
  no remaining blockers after regressions for malformed native responses,
  failed saves followed by Refresh, and duplicate replayed invocations.
- Isolated supervisor with real AGY 1.5.17: two chats held independent settings,
  and model/thinking choices restored after restart in the chat with one
  recorded turn. Native session listing omitted the empty chat after restart.
  Native subagent probes observed invocation start/completion and replay.
  The CLI supplied no child-scoped transcript in those probes.
- Android `testDebugUnitTest lintDebug assembleDebug assembleDebugAndroidTest`
  passed offline, with 168 JVM tests. The Pixel 4 XL emulator passed the AGY
  picker, existing Codex Agents flow and same-signature upgrade with encrypted
  synthetic state from versionCode 39 to fixture versionCode 1000.
- The combined result is recorded above.

## Public documentation refresh

Updated 2026-11-27T16:09:44+07:00 on `docs/professional-readme`, source
`5cec493`. The repository front page is now a concise English product page with
an equivalent Vietnamese option. New bilingual architecture, agent connection,
status and roadmap pages separate current preview capabilities from future
Modex Cloud and Pro orchestration direction. The earlier OpenCode backend guide
is explicitly labeled as an implementation snapshot rather than the current
capability matrix. Both README languages also include a dated comparison of six
similar projects, their self-reported agent compatibility and a neutral T3 Code
telemetry note linking the community issue, current policy and external report.

- A standard-library check resolved 86 relative links and anchors across all 12
  public pages, verified language switches and six bilingual page pairs, and
  found no forbidden platform, license or encryption claims. `git diff --check`
  passed, dev.38 was confirmed as a public APK pre-release, and the credential
  pattern scan was clean.
- Independent documentation review found no remaining findings after correcting
  Android SSH onboarding, PATH/Go prerequisites, provider-readiness claims and
  stale OpenCode links. A follow-up factual review found no findings in the
  comparison or T3 note. Official repository/docs content and the linked T3
  issue were retrieved, the KuCoin report URL returned HTTP 200, 84 relative
  links across the 12 public pages resolved, both comparison tables contain six
  projects, and `git diff --check` passed. A later GitHub web retry returned a
  transient HTTP 503 for T3's telemetry page. The baseline
  `umask 022; go test ./...` passed before the documentation-only changes.
- GitHub rendering, physical-device setup, macOS/WSL runtime and fresh provider
  inference were not tested. Competitor interoperability and actual telemetry
  behavior were not tested; compatibility is based on current project docs.
  `umask 022; go test ./...` passed again before publishing the branch. The
  branch is pushed as `origin/docs/professional-readme`. Delivery to `main` is
  tracked by [PR #38](https://github.com/warmdev17/modex/pull/38); GitHub is the
  source of truth for its current merge status.

## OpenCode backend

Updated 2026-11-26 on `feat/opencode-backend`; Stage 1 implementation source is
`075697e`. Codex remains the default. The accepted roadmap and core plan are
[design](docs/superpowers/specs/2026-11-26-opencode-integration-design.md) and
[plan](docs/superpowers/plans/2026-11-26-opencode-core.md).

- Core entry is usable by the existing Android protocol: standalone New chat
  creates an OpenCode session under `$HOME/Documents/Codex`; `turn/start`
  supports text/files; `turn/steer` uses native `delivery:"steer"` and returns
  the active Modex turn id; exact-turn Stop rejects stale/completed targets
  before calling OpenCode.
- Concurrency corrections remove the decision/synthesizer reverse lock edge and
  prevent a delayed Stop response from marking a replacement turn interrupted.
  OpenCode's interrupt/prompt endpoints do not accept an expected turn id, so a
  narrow upstream change after the local snapshot remains unavoidable.
- Existing read/history/model/SSE/permission projections remain. OpenCode state
  is still temporary per supervisor; forms/questions, lifecycle navigation,
  projects, references, plan mode and cross-agent utilities remain later-stage
  work.

- Final review fixes preserve the active turn on history reload, discard stale
  polling snapshots, check workspace before mutations, suppress consumed
  approvals, emit confirmed user messages and encode attachment URIs. Polling
  searches up to 100 messages for the active turn anchor.

Stage 1 checks at `075697e`: Go race suite (umask 022), vet, temporary host
build, scoped re-review and isolated OpenCode session creation passed. The
earlier isolated supervisor run at `771c518` verified standalone creation,
same-turn steer admission and exact-turn interrupt. The final live steer
transcript, paid-provider completion, physical Android and production
activation were not exercised. [PR #36](https://github.com/warmdev17/modex/pull/36)
merged Stage 1 as `ebe7825` after all five GitHub Verify jobs passed.

Stage 2 capability milestone verified 2026-11-26T20:15:12+07:00 on
`feat/opencode-capabilities`, source `88c7a7c`. The
[capabilities plan](docs/superpowers/plans/2026-11-26-opencode-capabilities.md)
covers authenticated SSH backend identity, compatible legacy Codex defaults,
and Android controls and requests gated to the current connection.

- Host `8ad1cab` and test cleanup `b37e8d0` passed independent review and
  isolated OpenCode 2.4.1 attach, session creation and history read.
  `umask 022; go test -race ./... -timeout=90s` and `go vet ./...` passed.
- Android `b24779e` plus fix `88c7a7c` passed scoped re-review, 154 JVM
  tests, both debug APK builds and debug lint (0 errors). On API 33,
  `skill-picker` at 412 dp reproduced the missing Codex menu before
  the fix and passed after it. `opencode-capabilities` passed at 320 dp
  after the fix. Earlier checks at `b24779e` passed native Codex controls,
  loopback SSH reconnect/wrong pin and attachment transfer.
- Capability branch is local. No delivery build, service activation,
  physical device check or live provider inference.

Stage 2 model identity verified 2026-11-26T20:51:43+07:00 on `feat/opencode-models`,
source `b808702`, based on capability milestone `df35920`. OpenCode reports
`provider/model` as the selectable ID and carries it through default status,
creation, read and resume. Legacy unique bare IDs still resolve. Ambiguous
bare IDs require an explicit provider, and Android retains the draft when
its loaded catalogue detects the ambiguity before send. Active steer keeps
the current model.

- `umask 022; go test -race ./... -timeout=90s`, `go vet ./...` and
  independent review passed. Fixture regression tests cover equal model names
  across providers, exact prompt provider and rejected ambiguous mutations.
- Android `./gradlew :app:testDebugUnitTest :app:lintDebug --offline` passed
  with 155 tests and zero lint errors. An isolated OpenCode 2.4.1 supervisor
  passed catalogue, qualified session creation, resume and read.
- Work is local. Phone/emulator UI and real inference were not exercised
  for this milestone.

Stage 2 attachment projection verified 2026-11-26T21:00:26+07:00 on
`feat/opencode-attachments`, source `ddcc0ab`, based on model milestone
`2950f50`. Confirmations and history expose local file and image metadata,
including file-only messages. History decodes OpenCode's persisted file URI
without forwarding embedded bytes. Android still accepts legacy attachment
formats. The history projection revision was bumped.

- `umask 022; go test -race ./... -timeout=90s`, `go vet ./...`, 156
  Android JVM tests, debug lint (0 errors), and independent review passed.
  An earlier race-suite run failed the interrupt reply order assertion;
  a baseline 100-run focused check and the full rerun passed without changing
  that test or interrupt behavior.
- With isolated OpenCode 2.4.1 and a null native default model, the real
  client/bridge admitted a text file with an escaped name and a PNG with
  blank prompt text. The confirmation and persisted history matched.
- No physical/emulator UI test or APK release in the attachment projection
  milestone.

Stage 2 large attachments verified 2026-11-26T21:09:15+07:00 on
`feat/opencode-large-attachments`, source `4fe7fbf`. Prompt acknowledgments
and history discard embedded attachment bytes under bounded wire and output
limits. Oversized history pages retry with the same cursor and a smaller
limit. Turn polling retains the 100-message anchor window. Prompts are not
reposted in response to a read failure.

- Go race suite, vet, staged diff check, independent review and isolated
  exact-binary supervisor tests passed. OpenCode 2.4.1 with no default model
  preserved the message ID and file metadata across confirmation and history
  for a 20 MiB fixture. History was 27,962,727 bytes on the wire. Native
  pagination with varied limits preserved order and IDs.
- Tested host binary installed at `/home/warmdev/.local/bin/modex-host`
  with SHA-256 `216be265d40071e3dcd9e04975c37bfba87da1a95566c129ca24959ae7005f9d`.
  Previous binary saved to `/home/warmdev/.local/state/modex-host-rollback.0ZZPU1pW/modex-host`.
  Activation prepared for one external 30-second restart. Running PID was
  still 1129122 at installation. Service remains configured for Codex.
  After restart, Android: Connection settings → Reset session cursor →
  reconnect. This snapshot does not verify timer execution or activation.
- No Android source change, APK release, physical phone run or real provider
  inference in this milestone.

Stage 2 permission replay fix verified 2026-11-26T22:51:14+07:00 on
`fix/opencode-permission-replay`, source `c2ce61c`. A native reply to a known
pending approval prevents stale SSE events or poll snapshots from reopening
it. Missing or null snapshot data preserves pending approvals.

- Both regressions failed before the fix. `go test -race ./... -count=1`
  (umask 022), `go vet ./...`, diff checks and the two isolated
  `scripts/test_supervisor.py` tests passed using the built host.
- Verified that the previous binary had activated. Installed the new
  exact tested binary with SHA-256
  `b9978f2f1c3531c7968dd93a84d912ec3d8f39ff68ada9dafabea2b0ca37e424`.
  Previous binary backed up at
  `/home/warmdev/.local/state/modex-host-rollback.lO454w/modex-host`.
  One delayed restart is prepared as the final operational step. Service
  runs Codex. This snapshot does not verify activation.
- Phone and live OpenCode approval were not exercised. Android source is
  unchanged. Next: native OpenCode forms and remaining permission parity.

## Stale Android attachment lease

Updated 2026-11-25T21:10:57+07:00 on `fix/stale-attach-lease`, source `0068718`. Android attach now negotiates a 15-second application heartbeat. The supervisor releases a negotiated client after one minute without frames or incoming bytes, while legacy CLI attaches retain unlimited idle behavior. This prevents a half-open phone SSH connection from indefinitely causing `another client is attached`; heartbeat frames never reach the Codex bridge, and slow command uploads renew the lease as bytes arrive.

- Regression tests reproduce silent-client eviction, replacement attach, heartbeat renewal, filtering and partial-frame activity beyond the original deadline. `go test -race ./...`, `go vet ./...`, Android unit/lint, isolated supervisor tests and real loopback SSH transport passed. Independent review found and verified the partial-frame correction; GitNexus had no Modex index, so review used direct caller inspection.
- A paired host activation and Android update are required. The host must activate before the new APK is used because old hosts do not recognize `--heartbeat`. Physical-phone background/Doze behavior remains untested; release, merge and live host activation are pending.

## Live follow-ups during a Codex turn

Updated 2026-11-25T19:44:00+07:00. [PR #34](https://github.com/warmdev17/modex/pull/34) merged as `77001e2d9165cd2ae83b862677b757b40b74eb9e`. Signed Android [dev.36](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.36), versionCode 40, uses native `turn/steer` during ordinary active turns, with separate Stop and acknowledgments that clear only the submitted draft. Host validates the workspace and follows the CLI's explicit turn-race handling. Native user-message events provide the transcript entry. [Plan](docs/superpowers/plans/2026-11-25-live-steering.md).

- Source checks passed: `go test -race ./...`, `go vet ./...`, 150 Android release unit tests, release lint/build and all five hosted PR jobs. The signed Android 13 emulator upgrade from code 39 to 40 preserved encrypted synthetic state, and the distributable passed the steering smoke. Public assets were downloaded anonymously and matched SHA-256 `d7663e1ca68692289d2503ab19df0def5e9356df9b44e5305cce15cfc6d6ec44` and the established signing certificate.
- An isolated supervisor with Codex CLI 0.169.0 and a local synthetic Responses provider verified the same turn ID on admission and the transcript event, and verified that the following model request contained the steer. Reviewer finding on pending draft persistence was addressed. Actual process death with a pending steer and a physical phone remain untested.
- The paired host binary is active as PID 574090; the running executable matched tested SHA-256 `108a510bd8b878ec5c1363879a9f476f0cf0328860501ab9e9c26bcfcb50f6ac`. The UI preserves the draft when native steering rejects a turn that cannot be steered. That path does not implement the CLI's separate queue. Physical-phone installation, TalkBack and a real paid-provider steering turn remain untested.

## Android UX release batch

Updated 2026-11-24 on `feat/android-ux-polish`, based on `2947736`. The batch adds Downloads export, inline host validation, remote file browsing, session menus and agent scrolling, and contextual notification permission.

- Published [dev.35](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.35), code 39, from `26b05d976db3e5430d2cd41c908686d78ed70d9e`. The public APK was downloaded and matched SHA-256 `35e67cbaddf4a0e30fae78333a85084f39f5b500c5b4972f982f9218a81d85ff`. Release unit tests (150), lint and signing identity checks passed, plus a code 38 to 39 encrypted-state upgrade on Android 13. Smoke checks passed for file preview/export on Android 9 and 13, remote browsing and projects, session actions, child scrolling at 320 dp, auto-follow with reduced motion, and returning to Hosts after failed SSH. Android 9 APK fixture bytes matched the source.
- Independent review findings were handled in the integration worktree after implementer limits interrupted per-task verification. The notification permission decision table has unit coverage. Physical-device use, a full TalkBack walkthrough, and SAF picker rotation remain unverified.

## Usage dialog refresh

Updated 2026-11-24T09:54:00+07:00 on `feat/previous-prompt-preview`, based on `fec8cdd`. Loading uses the same title icon slot as the idle state, so Usage keeps its layout while refreshing cached data.

- Native session controls smoke on the 412 dp emulator reproduced the bounds change on the old APK and passed with the fix. Android build, lint, 137 unit tests, and the signed code 37 to 38 upgrade with encrypted fixture state passed. Physical phone untested.
- SFTP preview `.modex/previews/modex-0.1.0-preview.37-usage-refresh.apk`, code 38, SHA-256 `ec88868ae25217a25ece44317aec56ebd007ac390117a9bfad020300c07d0506`. SFTP read-back matched the tested APK.

## Previous prompt preview

Updated 2026-11-24T08:59:00+07:00. Branch `feat/previous-prompt-preview`, based on `ca07c01`. In the main chat, scrolling into history reveals the nearest preceding hidden prompt below the header. Tap to jump to the original without resuming auto-follow.

- Verified the milestone with `:app:testDebugUnitTest :app:lintDebug :app:assembleDebug :app:assembleDebugAndroidTest --offline` (137 unit tests), and the auto-follow instrumentation at 320 dp with font scales 1.0 and 1.3. The fixture checks header placement, tapping to jump and viewport stability during streaming. The initial signed candidate preserved encrypted synthetic state over dev.34. The final same-signer APK was installed and exercised on the emulator. Physical phone installation remains unverified.
- Private SFTP preview on the connected host at `.modex/previews/modex-0.1.0-preview.35-previous-prompt.apk`. Version code 36, SHA-256 `cbefeaddb1835c1cb1a5c516ce5c7d8f1a6a024cefa4b72068930c834fa084aa`. SFTP read-back matched the exact tested APK. Include this UI change in a later release batch.
- Follow-up on 2026-11-24, based on `8a8fdf7`: Scroll to latest jumps close to the end before animating when far away, and hides the prompt preview during the animation. Android unit tests (137), debug lint/build, near/far return and streaming on the 320 dp emulator, and a same-signer upgrade from code 36 to 37 with encrypted fixture state passed. Physical-phone smoothness remains unverified. The new SFTP preview is `.modex/previews/modex-0.1.0-preview.36-scroll-to-bottom.apk`. Its SHA-256 is `75e71f85c04ef9c42cfb6ba1c21df7289733871fa26b8771075e7bd94b07776e`, verified by SFTP read-back.

## Read-only Subagent Conversations dev.34

Updated 2026-11-24T06:52:36+07:00. Feature branch `feat/subagent-activity`. Signed Android [dev.34](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.34), versionCode 35, source `6ffc9a99c9bc6b851d81326187ee2a7775ddddf8`. APK SHA-256 `8c4e4d6f8c268f06707d233a1b1e86cc729be6954361e01fa10b78f0de9220b7`. Public assets were redownloaded and checksum-verified.

- The Agents panel opens read-only child conversations with the main chat renderer, live events and paged history. Press highlights stay inside each control. The existing dev.33 host provided child events and saved messages in an isolated real Codex test. Android-only delivery kept the existing host process active.
- At the tagged revision, the Go suite, 137 Android release tests, release lint, matching signer validation, signed dev.33 to dev.34 data-preserving upgrade, 320 dp Agents UI smoke and existing keyboard/reference regression passed. Physical hardware and TalkBack remain unverified. This branch carries both subagent releases into main with their original source commits.

## Subagent Activity Timeline dev.33

Updated 2026-11-24T05:43:24+07:00. Feature branch `feat/subagent-activity`. Signed Android [dev.33](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.33), versionCode 34, source `c573c74cd79c720dbfddf8e6bfc35851a38fc8ef`. APK SHA-256 `5c20fea497daf47c0b76cfd37ca25400234faf49a4a5aa1e2b449344e745373a`. Public assets were downloaded and checksum-verified.

- The Agents chip and timeline display Codex activity from the current chat's live events and loaded history. Start, interaction and terminal markers survive reconnect and history restoration. Older activity appears as older turns are loaded.
- At the tagged source, `go test -race ./...`, `go vet ./...`, and the local Python suite with the built host passed (one optional test skipped). The exact installed host binary passed a live isolated supervisor test with two actual agents, journal reattach, and identical paged history. The previous executable was backed up before replacement.
- Android release unit tests (135), release lint, signer validation, signed dev.32 to dev.33 in-place upgrade with encrypted synthetic state, and the 320 dp Agents IME smoke passed. The existing composer regression with references and an image passed. Physical device and TalkBack were not exercised.
- The dev.33 host was installed and activated with a one-shot restart. The dev.34 Android update uses that host. The cursor reset applies to the dev.33 host upgrade.

## Android updates dev.31–dev.32

Updated 2026-11-24T05:07:13+07:00. Integration branch `fix/android-prompt-drafts`. Latest Android prerelease [dev.32](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.32), versionCode 33, source `b307028`. APK SHA-256 `d147ec09984ebc8e0a94cca3100c4d3898656798782345b13973c43fd2a8a414`.

- [dev.31](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.31), source `77cf8df`: shared connection error dialog with red title and icon, host identity and Edit host action.
- [dev.32](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.32), source `b307028`: encrypted prompt text and selected references keyed by host and conversation. Backgrounding flushes text. Accepted Send clears the draft. Images are outside the saved draft.

The dev.32 release verified 132 Android unit tests, lint, signed dev.31→dev.32 emulator upgrade, exact draft restoration after process restart, host/thread isolation, Send clearing, storage retry and Forget. Reference picker and narrow keyboard checks passed. Physical phone and TalkBack remain unverified. Source integration leaves the host running.

## Release train dev.18–dev.30

Updated 2026-11-24T00:05:03+07:00. Integration branch `integration/dev18-dev30`. Android prerelease at that integration [dev.30](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.30), versionCode 31, source `ae21f3653a0912825b636330443ec20526f2d13c`. The source commits are retained in order. Release tags identify the original build revisions.

| Releases | Source | What shipped |
| --- | --- | --- |
| dev.18–20 | `c8e1e99`, `97aba9b`, `ed91ee6` | Compact tool-call summaries, code highlighting and shell wrapper cleanup |
| dev.21 | `9c9b2a0` | Session context and remaining usage |
| dev.22 | `1bd22f8` | Markdown tables fitted to the chat viewport |
| dev.23 | `bcdb218` | Session permission profiles and quota warnings |
| dev.24 | `491a17f` | Native exploration, web activity and file diffs |
| dev.25 | `efe1e37` | Stale-journal recovery and editing saved hosts |
| dev.26–28 | `c964aee`, `247caa1`, `5e91848` | Add Host drafts, stable aligned status and immediate connection-error dialogs |
| dev.29 | `1e4cfd0` | Transparent compact composer row with work timer |
| dev.30 | `ae21f36` | Full-width, one-tap plan review actions |

Tagged releases carry their verification and checksum artifacts. The latest Android release passed 132 tests, release lint, approval and plan UI checks, and the signed dev.29→dev.30 emulator upgrade with encrypted synthetic state. APK SHA-256 `84bb05ff57f9fbf6a1511f87dc71d3fdcc2c147e0f51f29e574491d539b7205c`. Physical phone, TalkBack and live plan execution remain unverified. Integrating source does not deploy or restart the host.

## Earlier integration through dev.17

Updated: 2026-11-23 Asia/Ho_Chi_Minh. Active branch `main`, merged source `be1fc9554789b2cc0abbd44172b1054bb2bf1cbf` (PR #30). Android [dev.17](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.17), versionCode 18, source `170ab6a05d799ae718df8e2321d185c0bd3b2258`, was the latest delivered client at that point. The host at that point was dev.15/source `15bef480cf621c807fe9ad4d6684ca8c98c3166f`; dev.16 and dev.17 were Android-only and did not restart it. No active feature implementation remains.

Latest verified fix: streaming no longer serializes/encrypts the full history on every delta and no longer replays a redundant 20 ms typewriter. On the owned emulator, 80 deltas with 1,500 history items improved from 8,445 ms to 68 ms; a 500-character mixed-Markdown burst improved from 2,324 ms to 190 ms. Exact text, encrypted snapshot/cursor, Forget cancellation, auto-follow/manual viewport, Stop, native mode/status and @/image/history/real-IME regressions passed. The signed dev.16/code17 to dev.17/code18 in-place upgrade passed without uninstall or data clearing. PR #30 preserved the source commit, all five hosted jobs passed, and every public asset was anonymously downloaded and hash-verified. APK SHA256: `b13781b096aa789b44b430cdaa8dd4e1404afa477b94a826b344c71ad9494fdf`.

Delivered Android milestones now integrated into `main`:

- dev.10/code11, source `7169ae1`: disconnect returns to Hosts; reconnect does not auto-open the old session; archive/restore/delete/fork and durable interrupted/error notices. PR #23.
- dev.11/code12, source `e026154`: long-press sidebar actions and consistent, higher-contrast popup/upload icons. PR #24.
- dev.12/code13, source `e99b858`: Android command picker milestone and host activation; later superseded by removing slash commands. PR #25.
- dev.13/code14, source `41d93b3`: fixed `/` and `@` composer ANR with history/attachments/real IME and kept Send visible. PR #26.
- dev.14/code15, source `c9eb7dc`: removed slash commands, added native Default/Plan mode and compact session status. PR #27.
- dev.15/code16, source `15bef48`: Stop bypasses pending ordinary requests on the shared phone backend. PR #28.
- dev.16/code17, source `381fc88`: compact mode label outside the composer; Default blue, Plan pink, mode-colored Send; narrow-screen attachment/reference fixes. PR #29.
- dev.17/code18, source `170ab6a`: streaming performance fix described above. PR #30.

Current limits: physical-phone timing, landscape and TalkBack remain unverified. An unusually dense giant Markdown paragraph can still take seconds for its initial layout; dev.17 does not claim a universal frame-rate guarantee. Android dev.17 needs no host restart or session-cursor reset.

## Terminal resume blocked by deleted source history

Updated: 2026-11-17T17:22:07+07:00. Branch `fix/native-resume-diagnostics`, HEAD `154c23f` unchanged. Owner confirmed deleting older Codex conversations while retaining some. Exact terminal TUI bootstrap was reproduced through a temporary private WebSocket proxy. Upstream rejects thread 01a1e797 with `invalid paginated history lineage for 01a1dcc2-b294-71c1-bb2f-ba7ed665d20c: missing source rollout`; Modex suppresses this unknown error as request_rejected. This current failure supersedes the earlier active-writer diagnosis. Both metadata and nonpaginated resume, plus full thread/read, fail with the same missing-source error before inference. No compatibility fallback was applied.

The retained rollout has history_mode=paginated, history_base referencing 01a1dcc2 at ordinal 31341 / byte offset 82361667, and forked_from_id 01a1b731. Neither source has a threads registry row or a matching source file in sessions/archived_sessions. Checked Codex backups, Trash and /tmp filenames; no matching original found. Four current Codex processes expose no matching open source file descriptors. Source 01a1dcc2 still has 74 projected turns and 2879 projected items in thread_history_1.sqlite; these are not evidence of a complete original rollout. Homebrew and packaged app-server Codex binaries have identical SHA256, ruling out a binary difference for this reproduction. The owner's currently usable Codex session should remain open; its reason for continuing despite failed fresh resume is not established. Recovery of the exact lineage needs original source data/backup; do not synthesize it from incomplete projections or rewrite history metadata. Continuing project work from this PROJECT_STATE is available independently.

No source changes retained, history edits, credential changes, inference, installed binary changes, service restart, commits, push/PR/merge. A small missing-source diagnostic patch passed a red/green regression, full Go race suite, vet, six fake-provider handoff tests and isolated real native resume rejection; it was removed because it only changed the message and did not recover history. Those checks describe the temporary diagnostic tree, not a retained implementation. Go vet/build initially hit read-only default cache and passed outside sandbox. Prior HEAD targeted Go/handoff checks passed with umask 022; default 0077 changes the public-directory test fixture to private. Android/device/live-inference checks were not run. Temporary proxy stopped; isolated supervisor cleaned up. Existing historical state edits remain uncommitted. Next: restore original source history from an owner backup if available; otherwise preserve the usable Codex session and continue project work here. Do not restart a host merely for this data-loss diagnosis.

## Earlier native CLI resume diagnostic milestone

Updated: 2026-11-17T11:07:36+07:00. Branch `fix/native-resume-diagnostics`, commit `154c23f3714b942b54ade1e98f61f9fd6524b9b1`, local only; no push/PR/merge. Exact upstream metadata resume of owner thread 01a1e797 returned active writer, not an authentication error. The current thread is held by another backend; restarting Modex cannot release that writer. Host base config currently defaults to OpenAI and the thread metadata also reports OpenAI. No credentials changed, no inference sent, no history or writer locks deleted.

Native CLI errors now expose sanitized active-writer/auth/provider guidance plus safe category data. Unknown errors remain generic; raw credentials/errors are never forwarded. Regression test failed before the change and passed afterward, including numeric RPC codes, wrapped errors and secret suppression. Full Go race suite/vet and diff checks PASS. Exact built host in a separate supervisor/native WebSocket PASS for real active-writer rejection; fixture stopped. GitNexus has no Modex index, so source search used. Independent review and Android/device tests not run; Android unchanged.

Installed exact tested binary to `/home/warmdev/.local/bin/modex-host`, backup `.modex/releases/fix-native-resume-154c23f/modex-host.before`; existing service PID 80880 preserved. Final operational action will check pending timers and arm one external 30-second restart; not proof it executed. Owner must close this thread in external Desktop/plain CLI, then run modex and select it. After host restart Android requires Connection settings → Reset session cursor → reconnect. Restart may interrupt active Modex turns. Inference with new auth remains unverified until writer handoff. Existing uncommitted state preserved.


## Native plan confirmation delivered; host activation prepared

Updated: 2026-11-16T21:31:51+07:00. Branch `fix/plan-confirmation`, pushed commits `77992e9` (guarded backend/contract), `7c00903` (Android plan UI/state), `01d9509` (named isolated journal/continuity fixtures). Source `01d95090dbc74f9db17e4042a367c2c6a6494c7f`; tag/pre-release [v0.1.0-dev.9](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.9), Android versionCode 10, established development signer. All six public assets anonymously downloaded and hash-verified; remote branch and annotated source tag verified. APK SHA256 `0bbb7667ca8cebdbd8bfec5215113367bc3fe2ca8a7c9fe28118154530aca6ff`. No PR/merge requested or performed.

Completed structured plans render as Markdown and offer implement here, clear context/implement in a new chat, or stay in Plan mode. Fresh threads retain workspace/project/provider and verify copied effective permissions before dispatch. Durable private host receipts plus encrypted handled IDs prevent automatic uncertain-action replay. Interrupted/failed/superseded/handled plans suppress choices. Send replacing Stop after the planning turn finishes is expected; CLI's local popup is not a remotely controllable approval. Contracts: docs/bridge-plan.md; usage: docs/mobile-development.md.

Verified: Go race suite/vet PASS; 108 JVM tests forced to rerun, zero failures/errors; Android builds/lint PASS; package/version/signature PASS; exact dev.8/code9 to dev.9/code10 upgrade retained encrypted synthetic host/key/model/cursor; plan UI smoke PASS and screenshot inspected. All three real Codex/provider choices PASS in isolated new sessions (109.99 seconds). Final native binaries: 33 script tests PASS with one opt-in skip, real Android SSH reconnect/wrong-pin rejection PASS. Journal recovery initially crashed because its old ViewModel factory no longer matched the multi-host constructor; both stale fixtures now use dedicated named stores, and journal recovery plus reconnect/process-restore continuity PASS. Disposable emulator stopped and adb device list empty. Physical phone and other OS runtime untested; self-review only, independent reviewer tool unavailable.

Installed verified Linux amd64 release binaries atomically to `/home/warmdev/.local/bin/modex` and `modex-host`; both report dev.9/source 01d9509. Running supervisor PID 8623 remained unchanged at install. Rollback: `.modex/releases/v0.1.0-dev.9/rollback-before-activation-20261116T213007`. Activation is prepared; this snapshot does not prove a restart ran. Final operational step is to check existing jobs and arm one external 30-second restart, then reply immediately. This delay can still interrupt active turns. On Android: install APK, Connection settings → Reset session cursor → reconnect; inspect uncertain actions, never auto-resend.

Next: verify on owner phone, then PR/merge when requested, preserving all commits. Existing historical PROJECT_STATE.md changes are preserved; this state file remains uncommitted.


## Activity navigation and compact headers delivered

Updated: 2026-11-16T16:29:21+07:00. Branch `feat/activity-navigation`, source commit `2749ca34b4e9c1efbad26c57ccdf78bb02d2e77b`, pushed. Separate instruction commits `dc10e05` (delayed host activation rule) and `093ff3c` (compact UI headers) are preserved. Tag/pre-release [v0.1.0-dev.8](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.8), Android versionCode 9, established development signer retained. All seven public release assets anonymously downloaded and hash-verified; source tag verified. APK SHA256 `646dd6447a8c8504fce791ee86163dd998e132314d48067d326f322891975c95`.

Home now has Hosts / Activity / Settings. Activity deduplicates running conversations and waiting approvals/questions per remote, prioritizes waiting rows, labels offline cached activity explicitly, and routes row taps to the selected host/thread. It does not automatically answer anything. Home and host settings use single 56dp titleMedium toolbars without oversized headings or repeated brand/subtitles. Android Back from a host editor returns Home; settings entered from chat return to chat.

Verified on the feature source: 104 JVM tests, Android build/lint PASS; package/version/signing certificate verification PASS; exact distributable APK in-place upgrade 8 to 9 retained encrypted synthetic host/key/model/cursor. UI checks PASS for compact toolbar bounds, editor Back, empty/running/waiting/offline Activity rows and exact host/thread callbacks. Two isolated real SSH supervisors PASS for separate model choices, disconnect/service isolation, both chat screens, encrypted process restore and profile removal. Initial screenshots captured before the first frame; test-only capture now waits for rendering and all three final synthetic previews were visually inspected. Disposable emulator-5580 stopped; final adb device list empty.

Android-only: no host rebuild or restart. Physical phone and live-provider approval interaction not tested; self-review only, no independent reviewer tool available. No PR/merge requested or performed. Existing historical PROJECT_STATE.md changes preserved; this state file remains uncommitted. Next: owner phone verification, then PR/merge when requested.


## Android connection management delivered

Updated: 2026-11-16. Branch `feat/connection-management`, pushed commits `cb52f4d` (independent encrypted host profiles/controllers and aggregate foreground service) and `c244b3b` (Home, Connections, Settings, host editor and integration fixtures). Tag/pre-release [v0.1.0-dev.7](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.7), APK versionCode 8, source `c244b3bbd68de84e56903cc6a8ab5593c6e08c7e`, established development signer retained. All five public release assets downloaded and hash-verified. APK SHA256 `9e2fb92cbef757d3c198b834e4043a3794576c8c0a354fda6502f6b9425f44f9`.

Existing host.enc migrates into My computer without rewriting credentials; new remote profiles use stable independent IDs and encrypted stores. Home adds named hosts/edit/remove; Connections opens multiple simultaneous remotes without closing others; Settings controls reconnect-on-foreground. Each host retains its own key, cursor, history, model/effort and pending decisions. Chat headers identify the selected host. One disconnected host no longer stops the aggregate foreground service while another is active. Prompts/decisions are not automatically replayed.

Verified: 100 JVM tests, Android build/lint PASS; exact distributable APK in-place upgrade 7 to 8 preserves encrypted synthetic state and package/certificate; legacy host/key/cursor migration PASS; two isolated real SSH supervisors/fake provider PASS for separate model choices, disconnect/service isolation, Home/Connections/Settings touch navigation, both chat screens, encrypted process restore and removal. Early UI assertions relied on isClickable/immediate accessibility reads; real touch and bounded waits fixed the fixture without product navigation changes. Synthetic final screenshot inspected and published. Disposable emulator-5580 stopped; final adb device list empty. No physical-phone or simultaneous live-provider approval test. Self-review only. Android-only change: no host rebuild/restart. No PR/merge requested or performed; original commits preserved. Existing AGENTS.md rule edits and historical PROJECT_STATE.md entries remain uncommitted.


## New project vertical spacing corrected

Updated: 2026-11-16. Branch `feat/standalone-chat-sidebar`, commit `313e4acdabf60ad972026a94316c82526667d9ef`, pushed. The owner clarified that the issue was vertical spacing toward Search, not horizontal text alignment. Restore left-aligned icon/label content. Split the existing 12dp gap into 6dp above and below New project, shifting that action up by 6dp while preserving Search position. Supersedes the centered-label alignment decision below.

Published tag/pre-release v0.1.0-dev.6, APK versionCode 7 with established signer; all five public assets hash-verified, including a synthetic sidebar screenshot. Vertical regression failed on dev.5 with above/below distances 129px/88px, then passed on the exact corrected APK within 2px tolerance. 96 JVM tests, build, lint and package/version/certificate verification PASS. Upgrade 6 to 7 retained encrypted synthetic state; project/standalone routing and folder UI fixture PASS. Rendered screenshot inspected; emulator stopped. Physical phone not tested. No host changes or restart, no PR/merge. Existing state entries retained uncommitted.

## Standalone chats and centered sidebar actions delivered

Updated: 2026-11-16. Branch `feat/standalone-chat-sidebar`, commits `967fc26` (host and contract) and `3f1371a` (Android and UI regression). Source tag/pre-release v0.1.0-dev.5 published; all five public assets downloaded and hash verified. Android package/signature verified, versionCode 6. Both sidebar action groups centered; new-chat icon now square and pencil. Main New chat clears inherited workspace/project and requests standalone remote Documents/Codex; explicit project actions retain root and identity. Host validates standalone flag and rejects conflicting selection before mutation. No project is created for standalone chats.

Verification: 96 Android JVM tests, builds and lint PASS; exact APK upgrade 5 to 6 retained encrypted synthetic state; project fixture asserts centered text axes and standalone chat from an active project, plus explicit project routing and folder browsing PASS; chat UI fixture PASS and rendered screenshot inspected. Go suite/vet and bridge race PASS. Real isolated supervisor and provider turn PASS with saved thread in Documents/Codex and null project ID. Temporary emulator stopped; no physical-phone or macOS/Windows runtime test. Initial isolated test attempted list before first turn and found no persisted thread; rerun with real turn verified persistence.

Host upgrade required for standalone requests. Versioned Linux binaries staged in `.modex/releases/v0.1.0-dev.5/host-linux-amd64` and published in release archive. Installed binaries/service untouched; activation remains pending at an idle boundary outside the active Codex turn. No PR/merge. PROJECT_STATE.md retains existing local entries and remains uncommitted.

## Sidebar alignment delivered

Updated: 2026-11-16. Branch `feat/remote-project-create`, commit `919aaae`. New project now matches New chat content inset, icon spacing and full-width button bounds. Published source tag/pre-release v0.1.0-dev.4 with APK versionCode 5, established development signer and checksums. Public APK download hash verified. Clean Android build, 95 JVM tests and lint PASS; exact distributable APK upgrade from 4 to 5 retained encrypted synthetic state; chat/sidebar and remote-project instrumentation PASS, rendered drawer inspected. Temporary emulator stopped. No physical-phone test, host update/restart, PR or merge. Existing state changes retained uncommitted.

## Remote projects delivered; activation pending

Updated: 2026-11-16T04:43:25+07:00. Branch `feat/remote-project-create`, commits `018e751` and `ed500fd5200ecf43555021fc00f2a3a36c5e5e47`. Branch and source tag pushed; no PR/merge yet. Public pre-release [v0.1.0-dev.3](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.3) and all five anonymous HTTPS asset downloads verified against local hashes. APK versionCode 4, package/signature verified against the established development signer. APK SHA256 `223a3e3f5ccea0716e5a8257565733c1dfa36ee55c885392c2d4684ff251d4dc`.

Implemented canonical Codex project creation from an existing remote directory, explicit SFTP child-folder creation, folder-only fullscreen picker from SSH home, project picker in new-chat settings and per-project new-chat action. Host validates existing directory, bounded names/idempotency keys/project IDs and forwards canonical project identity together with workspace. No automatic mutation retry, Git init or file deletion. Plan: docs/superpowers/plans/2026-11-16-remote-projects.md; contract: docs/bridge-plan.md.

Verified on feature tree: full Go race/vet PASS; 95 Android JVM tests, builds and lint PASS; Python 33 tests PASS with 9 existing opt-in skips. Isolated real Codex project create/idempotency/list/thread root PASS; real SSH/SFTP child creation including literal metacharacters, traversal and regular-file rejection, attachment regressions PASS. Emulator remote-project, new-chat reducer, chat/drawer/model and remote-file-picker fixtures PASS. Exact distributable APK upgrade 3 to 4 retained encrypted synthetic state. macOS/Windows amd64 host cross-builds PASS, runtime untested. Self-review only; no physical phone claim. Disposable emulator stopped; owner AVD/profile preserved.

Both APK and updated host are required. Linux binaries are staged at `.modex/releases/v0.1.0-dev.3/host-linux-amd64` and published in the release archive. Agent did not replace the installed binaries or restart the service. Next: activate only at a deliberately idle boundary outside the supervisor's active turn; test with the owner phone, then PR/merge when requested. Existing local PROJECT_STATE.md changes remain uncommitted.

## Journal recovery merged

Updated: 2026-11-16. PR #22 merged into main with merge commit 30c2580864d8b7dd7548a61608a88a35bd95a889; original commit 0d69f8534061904f582f6e272cdecd19b4435d78 preserved. All five PR CI checks passed. Local main matches origin/main; merge tree exactly matches the verified release source, and go test ./... PASS before and after merge. User reports installing the new APK and restarting service. Installed host reports v0.1.0-dev.2; service active with PID 3265779 at verification. Agent did not restart service. Existing release/tag remains valid, no duplicate release. Existing local state retained uncommitted. Historical staging notes below are superseded.

## Journal recovery delivered; host activation staged

Updated: 2026-11-15T20:49:28+07:00. Branch `fix/journal-recovery`, commit `0d69f8534061904f582f6e272cdecd19b4435d78`. PR [#22](https://github.com/warmdev17/modex/pull/22) is open, not merged. Source tag and public development release [v0.1.0-dev.2](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.2) identify this exact commit. APK versionCode 3 retains package dev.modex and the established development signing certificate. APK SHA256: `c0996b3027510c2cefaacca0a7101d9b0dc1ef506c4228f2ae6379e50ff34184`.

Reset cursor 0 can receive a current authenticated checkpoint after retention; nonzero gaps and stale instances still reject. Android requires an explicitly cleared journal, then reloads selected session history, model/effort and activity. Shared-backend attachment resets/replays currently pending reviewable decisions individually under the pending-map lock. No uncertain prompt, approval or answer is automatically resent; pending response guards remain. Both Android and host need the update.

Verification on the committed source tree: 93 Android JVM tests, release/test builds and lint PASS; Go race/vet PASS; 33 Python tests PASS both with temporary binaries and with the versioned packaged Linux binaries. Actual release APK in-place upgrade 2 to 3 retained encrypted synthetic state. Isolated real SSH/emulator test recovered expired cursor, active turn/history/model, pending approval/question and resolved-decision state with the same supervisor/backend, zero turn/start replay and one explicitly submitted approval. Release APK auto-follow, history-scroll and command-timeline UI fixtures PASS. Temporary emulator/AVD removed; user emulator/profile preserved. Anonymous HTTPS downloads of all five release assets were independently hash/size verified. Hosted PR check snapshot: {"android": "SUCCESS", "go (macos-15)": "SUCCESS", "go (ubuntu-24.04)": "SUCCESS", "go (windows-2022)": "SUCCESS", "packaging": "SUCCESS"}.

Runtime follow-up (2026-11-15T21:18:12+07:00): the user restarted the service, but PID 2292030 still loaded old source a58c9b8 (vcs.modified=true), SHA256 ae46a6abf295717d9484b98c3c3b0081629bded77e1aa1dccf49329d1303790a, with no resynced checkpoint marker. This confirms the released fix had not reached the running host. Versioned release binaries have now been atomically installed to /home/warmdev/.local/bin/modex-host and modex, verified against the previously tested archive. Backups: /home/warmdev/Workspace/modex/.modex/releases/v0.1.0-dev.2/rollback-before-activation-twe95kje. No service restart: PID 2292030 still holds the original executable inode/hash and backend. Activation is pending a deliberately idle restart outside the active backend turn. APK v0.1.0-dev.2 and Reset session cursor/reconnect are required after activation. New Linux modex/modex-host are staged in `.modex/releases/v0.1.0-dev.2/host-linux-amd64` and included in the public archive. Next: activate host only at a deliberate idle boundary outside its own active backend turn; then install/reopen the APK and use Connection settings → Reset session cursor → reconnect when needed. Journal recovery is not a seamless host upgrade. Physical-phone upgrade and full macOS/Windows service/SSH operation remain unverified. Existing local state updates preserved; only PROJECT_STATE.md remains uncommitted. No merge or service restart; only the executable files were prepared for the next activation.

## Phone development PR integrated

Updated: 2026-11-15. PR [#21](https://github.com/warmdev17/modex/pull/21) merged with merge commit `442ce439ca0b735d873c505e5eba3e5e8fba8d03`; local main matches origin/main. All five original commits (`63ea6d8`, `96c6068`, `a2e2962`, `a2a0f7f`, `6cae13b`) are ancestors of main. Merge tree exactly matches the verified release source `6cae13bd5722f9ffb8dfd6768d7247e62e4d312f`. Five PR CI checks passed before merge. Existing development release/tag remains valid; no redundant APK or tag created for an identical source tree.

Merged-main verification: `umask 0022; GOCACHE=/tmp/modex-go-cache go test ./...` PASS; `git diff --check` PASS. The first full Go run inherited umask 0077 and failed TestSupervisorRejectsUnsafeState because its Mkdir(0755) fixture became private 0700. Existing production validation correctly accepts that private directory. The test fixture still depends on umask; no source fix was bundled into integration. Final-main hosted CI was not separately checked; PR CI and identical tree were verified. Existing local PROJECT_STATE.md changes preserved; this file remains uncommitted. No installed binary update, host restart, APK install or branch deletion.

Historical phone reconnect investigation, superseded by the release above: user screenshot identifies the journal-recovery error category, not another attached client. Reset creates cursor 0; Journal.After(0) still rejects when initial events have expired, so reset alone cannot recover that condition. Existing retention and stale-instance tests PASS and confirm this behavior. Screenshot does not distinguish retention gap from changed instance, and no runtime cause is claimed beyond that category. That pre-fix checkpoint/reconciliation gap is now covered by the release above. Do not restart the live host from its own active turn or replay uncertain prompts/approvals. Earlier statements that PR 21 is open are historical and superseded by this section.

## Phone-only Android release delivery

Updated: 2026-11-15. Branch `feat/mobile-release-workflow`, commit `6cae13bd5722f9ffb8dfd6768d7247e62e4d312f`. PR [#21](https://github.com/warmdev17/modex/pull/21) is open, not merged. Published development tag/pre-release [v0.1.0-dev.1](https://github.com/warmdev17/modex/releases/tag/v0.1.0-dev.1) with versionCode 2, the existing local development signing identity, APK, build manifest, version-code file and checksums. Signing key and connection profiles remain private. Development tags are excluded from the separate production signing workflow. See [phone development](docs/mobile-development.md).

Verification before publication: 90 Android unit tests, release/test APK builds and lint; actual versionCode 1-to-2 in-place upgrade retaining encrypted synthetic host/key, thread, cursor and model/effort; auto-follow, history-scroll and command-timeline fixtures on the distributable release APK; Go race/vet; 33 Python integration tests without skips. All five PR CI jobs passed: [run 43694527313](https://github.com/warmdev17/modex/actions/runs/43694527313). Disposable test emulator stopped and its temporary AVD removed; user emulator/profile preserved.

Public unauthenticated HTTPS download independently verified after publication: 10,515,894 bytes, SHA256 `e6d16af294af28448185e1eef66fdc42b879125b6e2ca7094d0cdb017f1d724a`. Download [APK](https://github.com/warmdev17/modex/releases/download/v0.1.0-dev.1/modex-0.1.0-dev.1.apk), install over the existing same-signer app and reopen. Do not uninstall or clear data to bypass a signer mismatch. Physical-phone upgrade remains untested by the agent.

Installed host was not replaced or restarted. Post-publication service check: active/running, original MainPID 878377. Android-only updates require no host restart. Uninterrupted host restart is NOT implemented: current supervisor owns the backend, and restart can interrupt work. Persistent engine/gateway separation, safe drain and Android journal/reconnect reconciliation are researched/documented, not delivered. First migration needs an idle activation boundary. Next host work must be isolated/tested before any activation; never replay uncertain prompts/approvals. PROJECT_STATE.md preserves previous local updates and remains the only uncommitted file.

## Thinking only above the prompt

Updated: 2026-11-15. Branch `fix/thinking-above-composer`, commit `a2a0f7f`, based on `a2e2962`. ChatWorkspace filters reasoning out of its rendered timeline, including historical reasoning, while preserving the full reasoning timeline for live composer action and elapsed time. Scrolling, empty-state handling and lazy older loading use the filtered display list. Removed the unused ThinkingSummary component. Canonical messages, history persistence, command cards, decisions and completed/interrupted timers remain unchanged.

Verification on the same source tree before commit: the new auto-follow assertion first failed on previous production code with Thinking is duplicated inside the transcript; the fixed APK passed auto-follow, history-scroll and command-timeline on a disposable API 33 emulator. Gradle testDebugUnitTest/assembleDebug/assembleDebugAndroidTest/lintDebug offline PASS; 90 unit tests with zero failures/errors/skips. Composer screenshot visually checked; thinking appears once above prompt. git diff --check PASS. Temporary ModexThinkingRegression emulator stopped and only its temporary AVD removed. No physical-phone install, paid-provider turn, host restart, push/PR/merge or release. Existing state updates preserved and PROJECT_STATE.md remains uncommitted.

## Codex question answers

Updated: 2026-11-15. Branch `feat/codex-question-input`, based on `96c6068`. Android question cards now show radio selection with blue accent and option descriptions, an Other choice for free text, and multiline text input for questions without options. Option selection and custom drafts are separate; every requested question must be answered before explicit submission. Offline/responding controls remain disabled; existing ViewModel pending-response guard is reused. Request-scoped form state prevents drafts leaking between different request contexts. Approval protocol is unchanged.

Verification on uncommitted feature tree: Gradle testDebugUnitTest/assembleDebug/assembleDebugAndroidTest/lintDebug offline PASS; 90 unit tests, zero failures/errors/skips. Three new tests cover required answers, blank text, invalid option index, exact requested IDs, retained custom drafts and multiline answers. Approval smoke fixture updated for explicit Other input. git diff --check PASS. No emulator UI run, physical-device install, real-provider question turn, host restart, push/PR/merge. Existing prior state updates preserved and remain uncommitted.

## Android live chat controls

Updated: 2026-11-15. Branch `feat/android-chat-controls`, commit `96c6068`, based on auto-follow fix `63ea6d8` (both are local/unmerged). Added a 48dp jump-to-latest control that resumes following; unified project, command and thinking disclosure chevrons using ChatGlyph. Active action/elapsed time now stays above the composer using the existing current-prompt reasoning summaries and timer metadata; completed/interrupted history timers remain. New-turn fallback avoids reusing previous thinking. Placeholder is Message Codex; removed working beside model/effort. Stop uses Catppuccin red and a vector stop glyph, retaining the interrupt callback and pending-stop guard. Activity accessibility announces the actual action and elapsed time.

Verification: new fixture failed first on the previous UI with no jump button. Final JDK 21 Gradle `:app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --offline --no-daemon` PASS; 87 unit tests, zero failures/errors/skips. Final API 33 emulator fixtures auto-follow, history-scroll, command-timeline and chat-ui all PASS. Coverage includes jump/resume, activity placement/time, exact red stop color/callback/pending guard, interruption and stale-thinking isolation, history/prepend, disclosure and sidebar/model interactions. Composer, drawer and command screenshots visually checked. Early fixture failures exposed an overly generic activity accessibility label; final labels contain actual status text.

Disposable ModexControlsRegression emulator was stopped and its temporary AVD removed; ADB showed no devices. No provider turn, host restart/binary update, physical-phone installation/test, push/PR/merge, tag or release. Debug APK is built; connect the intended device and run `make install-apk`. PROJECT_STATE.md remains uncommitted and preserves all prior local updates.

## Android chat auto-follow

Updated: 2026-11-15. Branch `fix/android-chat-auto-follow`, commit `63ea6d8`, based on `e53bed7`. Chat now follows rendered layout growth while pinned to the bottom, including typewriter streaming and batched new messages. Manual upward scrolling pauses following even within a tall last message; returning to the bottom resumes it. Existing initial-history positioning, lazy older loading and prepend anchoring remain covered. No backend changes.

Verification: original production code with notification permission granted failed the new `auto-follow` instrumentation at STREAM_END_FIRST; the fix passed. Final JDK 21 Gradle `:app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --offline --no-daemon` PASS, 87 unit tests with zero failures/errors/skips. Final fixed APK passed `-e preview auto-follow` and `-e preview history-scroll` using `dev.modex.test/dev.modex.TransportSmoke`. Fixture-only follow-up build/lint and auto-follow rerun after revoking notification permission also PASS; the fixture grants notification permission before opening the activity.

Only synthetic messages on a disposable API 33 emulator; no provider prompt, live host restart or installed host replacement. The emulator was stopped after testing and its temporary AVD removed; ADB showed no remaining devices. No physical-phone test or installation on the user's device. Debug APK is built; install it with `make install-apk` after connecting the intended device. No push/PR/merge. PROJECT_STATE.md remains uncommitted and preserves all preceding local state content.

## Skills, plugins and service restart integrated

Updated: 2026-11-15. Main is `e53bed7`, matching origin/main. PR [#16](https://github.com/warmdev17/modex/pull/16) merged as `daa8ca2`; [#17](https://github.com/warmdev17/modex/pull/17) as `66ebff2`; [#18](https://github.com/warmdev17/modex/pull/18) as `90ec985`; [#19](https://github.com/warmdev17/modex/pull/19) as `f7d59b6`; [#20](https://github.com/warmdev17/modex/pull/20) as `e53bed7`. All use merge commits. Original commits 5943fc0, a58c9b8, 1902605, 2fccad5, fdc4f5c, dbecc62 and e658abd remain reachable; the merged tree exactly matches tested feature head e658abd.

All five hosted jobs passed on every PR and final main [run 43676228665](https://github.com/warmdev17/modex/actions/runs/43676228665): Linux, macOS, Windows, Android and packaging. Local merged-main go test -race ./... and go vet ./... PASS; Python integration using private temporary main binaries 33 PASS, no skips. Existing implementation evidence for 87 Android unit tests and real-provider skill/plugin mentions is retained below. No additional live provider turn or device fixture was needed for integration.

The five feature/fix branches from these PRs were removed locally and remotely after ancestry verification; only main remains locally. Unrelated older remote branches were preserved. PROJECT_STATE.md remains the only uncommitted file and retains prior local entries. No installed binary update, host restart, APK install, emulator lifecycle/profile change, tag or release. Next: when idle, make build installs the new CLI/host and modex service restart activates the host; build/install the APK separately for phone/emulator plugin UI testing. Earlier pending PR/merge statements below are historical and superseded by this section.

## Service restart command

Updated: 2026-11-15. Branch `feat/service-restart`, commit `e658abd`, based on `dbecc62`. Added `modex service restart`: Linux reloads user units then restarts the existing service without changing enablement; macOS kickstarts the loaded LaunchAgent with -k. Existing registration validation and configured paths remain in use; manager failures propagate, no state or registration is recreated. Source-upgrade instructions now use make build followed by service restart when idle.

Verification: Linux regression failed first with unknown service action, then passed. Go race tests across all packages and go vet PASS. Tests use fake service managers, check exact restart arguments, failure propagation, unchanged registration, and refusal of absent/public registration. macOS service tests and CLI cross-compile for darwin/arm64 PASS; macOS runtime was not tested. No installed binary replacement or real service restart, no APK/emulator changes, no push/PR/merge. Only PROJECT_STATE.md remains uncommitted, preserving prior entries. Run make build before using the new command from PATH; restarting can interrupt active turns and creates a new journal instance.

## Installed plugin mentions

Updated: 2026-11-15. Branch `feat/mobile-plugin-mentions`, based on `2fccad5`. Host commit `fdc4f5c`; Android commit `dbecc62`. Plan: [installed plugins](docs/superpowers/plans/2026-11-15-mobile-plugin-mentions.md); contract: [references](docs/references-contract.md).

The existing @ picker now includes installed plugins in a separate group with a plug icon. Plugin references retain canonical plugin:// identity and @name markers in session drafts. Host calls read-only plugin/installed, hides uninstalled entries, disables unavailable/policy-blocked plugins and freshly validates exact kind/name/path before native mention dispatch. Skill and plugin references may share a name; eight total selections remain the limit. Unsupported/malformed plugin discovery preserves skills and reports only an issue count. Marketplace installation/removal and external plugin tool/auth management are outside this slice.

Verification on this implementation: Go race/vet PASS; 33 Python integration tests PASS using private temporary binaries; Android 87 unit tests, debug/test APK builds and lint PASS. Catalogue regression fails on baseline code via an overlay and passes on the implementation. Isolated real-provider plugin mention and skill regression both PASS with Codex CLI 0.169.0; the plugin test verifies accepted typed input and a completed assistant response, not external plugin tool execution. Extended picker instrumentation compiles but was not run to preserve the user's active emulator session. Self-review only; no callable delegation tool available.

No installed binary replacement, live supervisor restart, APK install, emulator lifecycle/profile/key changes, push, PR or merge. PROJECT_STATE.md preserves earlier local updates and remains the only uncommitted file. Next: update host and APK when idle, run the plugin picker fixture/device test, then integrate only on request. Current phone/emulator runtime still uses the earlier installed build.

## Reopen dismissed skill suggestions

Updated: 2026-11-14. Branch `fix/reopen-skill-picker`, commit `2fccad5`, based on `1902605`. Closing suggestions remembered the exact text/cursor indefinitely, so clearing and retyping @ stayed dismissed. Composer now clears that dismissal when text or selection changes; unchanged input stays dismissed.

Regression instrumentation failed on the original code at the reopened skill row, then passed after the fix. Coverage includes close, clear/retype @, filtered query back to @, disabled entries, exact metadata, session drafts, chip deletion, email guard and send. Android 86 unit tests, debug/test APK builds and lint PASS. No Go changes or backend restart. APKs installed -r on the existing emulator; real MainActivity reopened after fixture, emulator intentionally left running for user testing. No physical-phone test or push/PR/merge. PROJECT_STATE.md remains the only uncommitted file.

Prior detached runtime restart result was verified during this task: /tmp/modex-bin-restart-result.json reports passed, service active with executable /home/warmdev/.local/bin/modex-host and Android connected. This supersedes the pending runtime statement below.

## Installed host path and requested runtime restart

Updated: 2026-11-14. Branch `fix/installed-host-binary`, commit `1902605` on top of skill feature commits. Build script and Makefile now default to PREFIX/bin/modex-host, matching the service/phone/emulator installed executable. Fresh Android connection forms no longer suggest /tmp; an absolute host path is required. Existing local emulator JSON and on-device connection both verified as /home/warmdev/.local/bin/modex-host with original modex-phone state. No key/history deletion.

Verification: new installer/Make regression tests failed first, then 5 build tests and 6 emulator-install tests PASS; Android 86 unit tests, debug/test APKs and lint PASS. Both binaries installed with the updated build script; APKs installed -r preserving saved data. Pixel was stopped and restarted with the existing AVD under separate modex-pixel.service, intentionally left running for user testing as requested. Emulator journal reset was confirmed, then app stopped to avoid reconnecting before host restart.

The user explicitly requested killing/restarting Modex. Current assistant runs under the same supervisor being restarted, so final restart is prepared as a detached task; do not claim it completed until /tmp/modex-bin-restart-result.json reports passed. The task stops the target service and its installed CLI launchers, restarts from installed bin, opens Android and records service executable/connection verification without secrets. Runtime result remains pending at this snapshot. No push/PR/merge; earlier state content preserved and this file remains uncommitted.

## Mobile skill mentions

Updated: 2026-11-14. Branch `feat/mobile-skill-mentions`, baseline `7efdbf2`; host commit `5943fc0`, Android commit `a58c9b8`. Primary implemented and reviewed. Plan: [mobile skills](docs/superpowers/plans/2026-11-14-mobile-skill-mentions.md); contract: [references](docs/references-contract.md).

Android @ suggestions discover workspace skills through native Codex skills/list, including installed plugin skills. Selection binds exact name/path with the canonical $name marker; per-session saved UI drafts, removable chips, disabled state, refresh and stale-workspace guards are implemented. Host freshly validates up to eight typed references before native turn input. Generic kind metadata is the extension point; MCP/subagent/plugin execution or installation is not implemented.

Checks on the implementation now committed at `a58c9b8`: Go race/vet PASS; 31 Python integration tests PASS without skips using private temporary binaries; Android 86 unit tests, debug/test APK build and lint PASS. Real-provider skill integration PASS with MODEX_SKILL_OK. Emulator picker regression PASS for selection, disabled state, exact metadata, per-session drafts, chip deletion and email guard; screenshots inspected. Final chip-color adjustment was build/unit/lint verified after emulator shutdown.

Owned emulator was stopped; adb devices confirmed empty. Live host and installed binaries were not replaced/restarted. No push/PR/merge/release. Physical-phone, screen-reader, landscape and process-death restoration remain unverified. Earlier local state additions below are preserved; PROJECT_STATE.md remains the only uncommitted file. Next: update host/APK when idle for bradar's device test; integrate only on request.

## Shared backend integration complete

Updated: 2026-11-14. Main is `7efdbf2`. PR [#14](https://github.com/warmdev17/modex/pull/14) merged as `346e56a`; PR [#15](https://github.com/warmdev17/modex/pull/15) merged as `7efdbf2`. Both use merge commits; original commits remain reachable and the main tree matches tested build head `b21ae82` exactly. Both merged local feature branches were deleted; remote branches remain.

Final main CI [43592662525](https://github.com/warmdev17/modex/actions/runs/43592662525) passed all five jobs: Linux, macOS, Windows, Android and packaging. Merged-main local Go race/vet and 31 Python integration tests passed. Initial Python verification lacked /tmp/modex-host; rebuilding private test binaries resolved the setup failure without replacing the live host. Bradar confirmed CLI-to-phone and phone-to-CLI synchronization. Explicit network loss during a running turn remains unverified.

Only PROJECT_STATE.md remains uncommitted, preserving earlier local updates. No host restart, installed binary replacement, device lifecycle change, tag or release during integration. Earlier pending-integration statements below are superseded here.

## Shared supervisor backend and direct local commands

Updated: 2026-11-14. Backend branch `feat/shared-codex-backend`: `8467227`, `ba4a2df`, `3ea56e3`. Build branch `build/install-local-commands`: `31d6f7f`, based on those backend commits. Plan: [shared backend](docs/superpowers/plans/2026-11-14-shared-codex-backend.md). Primary owns implementation/integration; no push/PR/merge/release.

One supervisor-owned Codex app-server now serves Android and the native Codex TUI through a private Unix WebSocket gateway. Disconnect/unsubscribe does not unload sessions or interrupt dispatched turns. Decisions share first-answer-wins delivery, including native JSON-RPC errors. Legacy unsupported pagination uses bounded history slicing; oversized upstream frames are drained and pending calls fail without killing other turns. Never replay uncertain mutations. External unmanaged CLI/Desktop backends remain separate; Windows hosting requires WSL2.

`./scripts/build.sh` builds both commands before installing them into ~/.local/bin, refreshes the existing /tmp/modex-host phone binary and configures PATH only if needed. Make build targets delegate to it. Actual local install succeeded; command -v modex resolves ~/.local/bin/modex. Existing supervisor was NOT restarted, state/profile paths were NOT renamed, and no APK/emulator lifecycle changes were performed. Restart the host only when idle to activate the new backend; managed sessions then open through bare `modex`.

Verification: final backend `3ea56e3` plus build working tree identical to `31d6f7f`: go test -race ./... and go vet ./... PASS; installed binary Python discovery 31 PASS, no skips. Earlier feature validation: real-provider shared native/phone turn and disconnect continuation PASS; Android 81 unit tests, assembleDebug and lintDebug PASS; macOS arm64 CLI/host and Windows amd64 CLI cross-build PASS. Two independent review concerns were confirmed in source and fixed with regression tests (native error delivery; oversized history preserving backend). No second independent review of fixes; actual phone/emulator UI and macOS runtime remain unverified. Existing uncommitted state snapshot is preserved; this state update remains local.

## History features integrated

Updated: 2026-11-14. Active branch `main`, integrated commit `93d6511`. PR [#12](https://github.com/warmdev17/modex/pull/12) merged as `270adef`; PR [#13](https://github.com/warmdev17/modex/pull/13) merged as `93d6511`, both with merge commits. Original commits `3ead74a`, `1c3925e` and `3ce0913` were verified reachable from main; no squash/rebase. The committed tree matches the tested lazy-history head `3ce0913` exactly. Both local and remote feature branches were removed only after ancestry and merged-result checks passed.

Verification: all five hosted jobs passed for both PRs and for final main [run 43544581092](https://github.com/warmdev17/modex/actions/runs/43544581092): Linux/macOS Go checks, Windows supported-client checks, packaging and Android. Local checks on merged `93d6511` also PASS: `go test -race ./...`, `go vet ./...`, JDK 21 Gradle `:app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --offline --no-daemon` (80 unit tests, no failures/errors/skips).

This snapshot is intentionally an uncommitted local documentation update, following the rule not to commit/push solely to update project state; no other working-tree changes. No host restart, APK install, profile change, release tag or publication during integration. The existing emulator remains available with the previously tested APK. Physical-phone and explicit network-failure retry instrumentation remain unverified; earlier milestone sections below retain their evidence and limitations. The earlier no-PR/pending-integration statements below are historical and superseded by this section.

## Automatic older-history loading

Updated: 2026-11-14. Branch `feat/lazy-history-loading`, baseline `1c3925e`. Primary implemented the follow-up to paged history: approaching the first three visible items automatically requests older turns after initial positioning at the latest message. Existing host pagination, cache budget, connection ownership guards and saved host profile remain in use. A single in-flight request is enforced; errors pause automatic retries and expose a retry action. Retry clears the previous error. Timeline loading uses a small overlay rather than inserting a header item.

Stable turn/role/item keys preserve the currently visible message while older data arrives, including continued scrolling during the request. Entries without native IDs retain legacy index keys. The earlier request-time numeric anchor was removed after the delayed-page emulator regression demonstrated a jump when scrolling continued in flight.

Verification on the accompanying implementation working tree: JDK 21 Gradle `:app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --offline --no-daemon` PASS, 80 unit tests, zero failures/errors/skips. Emulator history-scroll regression PASS: async latest positioning, append/tall-message behavior, no early request, automatic request near the beginning, one in-flight request and stable prepend position after continued scrolling. RED on the prior implementation reported zero automatic requests; the first automatic-loading attempt failed the prepend-anchor assertion before stable keys fixed it. No Go changes or repeated Go suites.

Updated APK/test APK installed on existing emulator-5554 using install -r; no emulator started/stopped, profile wiped or host restarted for this Android-only change. The shared supervisor had already been updated on bradar's explicit deployment request, and reconnect after resetting only the old instance cursor was verified as Connected SSH with Greet user history. Physical-phone testing and explicit network-failure retry instrumentation remain unverified. No push/PR/merge/release. Next: bradar can test automatic history loading in the existing emulator.

## Paged history cache and reconnect ownership

Updated: 2026-11-14. Branch `feat/paged-history-cache`, baseline `3d4625f`; host committed as `3ead74a`, Android changes verified in the accompanying commit. Primary owns implementation and integration. Plan: [paged history](docs/superpowers/plans/2026-11-14-paged-history-cache.md); contract: [history](docs/history-contract.md). Supersedes earlier unpaginated history and visible-history-on-disconnect descriptions below.

Host uses native `thread/turns/list`, validates ID/cwd before paging, supports limits 1..50 and preserves legacy unpaged reads. Android loads 10 turns, prepends older pages without moving the reading anchor, and caches at most 10 sessions/4 MiB in the existing encrypted host snapshot. A changed newest-page fingerprint invalidates older cached pages; oversized snapshots skip caching without evicting unrelated sessions. Cache encoding is debounced with persistence.

Disconnect/transport failure/process restoration clears selected chat, messages and local ownership while retaining activity bookkeeping and last-thread target. Reconnect validates resume before showing cache and refreshing history; CLI active_writer leaves an empty landing and explicit Take over. Connection-local request guards ignore late resume/history replies. Captured transports prevent queued commands from moving onto a later connection. Streaming changes win over older snapshots; pagination cannot overwrite active-turn state. Model/effort choices remain persisted per thread.

Verification on host commit + final Android working tree: Go `go test -race ./...` and `go vet ./...` PASS; Python discovery with built host/client binaries 28 PASS; JDK 21 Gradle `:app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --offline --no-daemon` PASS, 80 unit tests, zero failures/errors/skips. Emulator new-chat reducer regression PASS for cached resume, fresh thread, disconnect clearing, late replies and explicit takeover. History-scroll instrumentation PASS for newest positioning, append stability, tall final message and older-page prepend anchor. Initial pagination UI test failures were stale API-33 accessibility cache; refreshing service info fixed the fixture, with no hidden cache-clear API dependency.

Read-only real Codex validation using temporary /tmp/modex-paged-host --serve: newest page 317 messages/822848 JSON bytes; next page 373 messages/1291076 bytes, advancing native cursor. No resume, takeover, provider prompt or interruption of the live CLI. Debug APK and test APK installed with -r on the already-running emulator-5554; no profile/state rename, wipe, or new emulator. Existing supervisor binary/state and running service were not restarted or replaced.

Limitations/next: update both host and Android before phone testing; a single oversized turn can still exceed the upstream frame budget. Physical-phone reconnect, real SSH ownership handoff/process-death lifecycle and the updated continuity SSH fixture were not run (the fixture clears saved profiles). Self-review; no independent reviewer tool available. No push/PR/merge/release. User-started emulator remains running.

## Integration complete

Updated: 2026-11-14. PR [#10](https://github.com/warmdev17/modex/pull/10) merged as `fffc775`; PR [#11](https://github.com/warmdev17/modex/pull/11) merged as `d5d3a4d`, preserving all original commits (no squash/rebase). All five hosted jobs passed on integrated head `1144b10`: Linux, macOS, Windows supported-client checks, packaging and Android. The merged main tree matched that tested head exactly. The only integration conflict was this state file; both milestone summaries were retained. Original commits `a4aa8c9`, `685a585`, `0d2703a`, `379c83a`, `dd25156` and integration merge `1144b10` were verified reachable from main. Three local feature branches and both published feature branches were removed after verification.

This documentation-only snapshot follows integration; no runtime host/emulator changes or release tag were made during the merge task. Earlier unmerged/pending statements below describe their historical milestones. Remaining verification: full takeover of the live CLI and real-provider permission-report persistence across reconnect; large-history SSH loading and subsequent host health were verified as recorded below.

## Shared phone and emulator host

Updated: 2026-11-14. Branch `fix/shared-emulator-host`, baseline `7fecbfb`. Supersedes the separate emulator supervisor setup below: `make emulator-host` now delegates to `make host` without overriding state; both default to `/tmp/modex-host` and `~/.local/state/modex-phone`. Updated emulator documentation and ignored local profile. Existing on-device encrypted profiles remain preserved and require changing the remote state field once if still using modex-emulator. No state migration/deletion or supervisor restart performed.

Verification: default and overridden `make -n emulator-host` resolve the expected binary/state; six `test_install_emulator.py` tests PASS; `git diff --check` PASS. No emulator/device runtime test or saved on-device profile update performed; last ADB inspection showed no attached devices. One client must disconnect before the other attaches. No push/PR/merge in this change.

## Large history relay crash

Updated: 2026-11-14. Branch `fix/large-history-relay`, baseline `379c83a` (includes the pending chat UI fixes). Live diagnosis confirmed `threads/read` for the current Modex session returns 4,804,428 bytes, over the supervisor scanner/Android 4 MiB cap. The supervisor scanner exited and normal cleanup removed endpoint.json after takeover loaded history. This explains the repeatable crash; the earlier deleted-lock orphan incident is separate and its origin remains unknown.

Response budget is now 32 MiB on host and Android; inbound commands remain 4 MiB. EncodeResult reserves 1 KiB for the journal envelope and rejects oversized results as response_too_large instead of killing the supervisor. Replay journal is bounded to 1024 entries/64 MiB. Android buffers SSH input and explains oversize failures. Whole history remains bounded and unpaginated; upstream Codex frames retain their existing 16 MiB cap.

Verified: old binary fails new large-history lifecycle regression (connection EOF, supervisor unavailable). Old Android reader fails 5 MiB test with Frame limit exceeded. Updated Go full race/vet PASS; Python suite 27 pass/1 SSH skip initially, then the skipped SSH test run separately with built client. Android unit tests 76 PASS, debug/test APK builds and lint PASS; added bounded/truncated-frame and next-frame tests. Oversized-result regression verifies next request still succeeds.

Live: updated `/tmp/modex-host` atomically and started it using the exact binary/state in ignored `.modex/emulator.json`, preserving `/home/warmdev/.local/state/modex-phone`, attachments, CLI and all credential settings. Installed APKs with adb install -r on the already-running user emulator; encrypted profile hash unchanged. Read-only large-history instrumentation compared all connection fields against emulator.json, fetched 5,131 messages / 4,865,792 bytes via real SSH, then received successful status. No prompt/provider turn or takeover of this active CLI was performed. User emulator was already running and remains open; this task started no emulator. App reopened afterward; persisted old-instance cursor may need one Reset session cursor. No phone install, PR/push/merge. Files/settings were not renamed or deleted.

## Chat feedback and settings corrections

Updated: 2026-11-14. Branch `fix/chat-feedback-settings`, baseline `7fecbfb`. Separate from unmerged `fix/shared-emulator-host` (`a4aa8c9`). Commits `685a585` (turn outcomes) and `0d2703a` (touch feedback); this milestone covers settings continuity/navigation.

Timer displays Interrupted/Cancelled/Failed instead of Worked for those outcomes; unknown outcomes use Ended. User long press retains Edit/Copy/Select text without a full-row ripple. Model button wraps its content inside the remaining composer space. Connection settings show the saved endpoint/binary/state/fingerprint while connected, with Android Back returning to chat. The last create/resume permission report is saved in the encrypted snapshot alongside the selected session, retained across disconnect/reconnect, and cleared when changing selection/forgetting/resetting. UI labels it as last reported, not current policy, and offers explicit Reload settings only while connected and idle. Older snapshots have no report until a session is opened/reloaded.

Verification: JDK 21 Gradle `:app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --offline --no-daemon` PASS, 74 unit tests with zero failures/errors/skips. Regression added for live interrupted completion and persisted message restore. Pixel_4_XL chat-ui instrumentation PASS including Interrupted timer, user long-press menu, model button bounds and Android Back dismissal; screenshot inspected. Actual app Connection settings -> Android Back -> chat verified via accessibility hierarchy. Updated debug APK ready for `make install-apk`. Emulator launched for this task was stopped afterward; no phone installation, provider turn, host restart, state/profile rename/deletion, push/PR/merge. Encrypted settings continuity reviewed in source; no real-provider reconnect/process-death verification. Impeccable engine unavailable; direct Compose inspection and Pixel checks used instead.

## Release distribution implementation

Updated: 2026-11-13. Branch `feat/release-distribution`, baseline `39b8d2b` (PR #8 merged with all three chat commits preserved). Original approved design commit `a3a31a8` was imported via merge `9ec06d8`, not squashed. Implementation follows [release plan](docs/superpowers/plans/2026-11-13-release-distribution.md) and [design](docs/superpowers/specs/2026-11-13-release-distribution-design.md).

Milestones: `a642f61` native version diagnostics and GoReleaser packages; `cd79135` user service lifecycle and crash ownership; `3d38200` verified POSIX installer; `4b5ce26` shared-lock stopped-state reuse. The accompanying commit adds release signing, package/certificate/version-code checks, PR validation, draft-only tag workflow and maintainer docs. No license chosen, production key provisioned, release tag pushed or release published. Integrated through [PR #9](https://github.com/warmdev17/modex/pull/9) as merge commit `7556660`, preserving the original design and every implementation/CI-fix commit. Hosted run `43434883299` passed all five jobs on `eb9de87`: Linux/macOS full Go suites/race, Windows supported SSH-client checks, packaging, and Android tests/build/lint plus missing-signing rejection. CI fixes use macos-15, request only available Android platform-tools, and generate a disposable debug identity on clean runners. Main's merged tree was verified identical to tested `eb9de87`. Release/design local branches and the release remote branch were removed after reachability checks; main was synchronized with origin/main. This documentation-only integration snapshot follows the merge.

Behavior: Linux/macOS archives include sibling CLI and host; Linux has DEB/RPM, Windows ZIP is SSH client only with WSL2 hosting documented. Per-user systemd/launchd install writes private configuration without starting a host, captures absolute paths/PATH and explicit CODEX_HOME without provider secrets. Start/stop/status/uninstall are explicit; uninstall preserves state. Shared OS flock releases on crash, keeps a persistent private lock inode, rejects legacy markers/live ownership and allows stopped new-format reinstall without touching its endpoint or attachments. Supervisor recovery removes stale endpoints only under exclusive ownership. Installer verifies exact SHA256, uses private staging and streams only the two binary members; replacements are atomic per binary, not a two-file transaction.

Signing/CI: missing signing inputs fail release builds with no debug fallback. Release tag drives versionName; workflow counter + 1000 drives versionCode, checked against all prior release/draft code assets. APK package/version/pinned certificate are verified before artifact upload. Only final draft job has contents-write permission; it runs after both native and APK jobs, aggregates checksums and never overwrites an existing release. Real signing identity must be privately configured in protected GitHub `release` environment; see [release setup](docs/releasing.md). Tests use a disposable key, now deleted; test-signed APK moved out of release build outputs to /tmp.

Verification on implementation tree: Go full race and vet PASS. Python full host/client integration + installer/release checks: 28 tests PASS, no skips. Nine installer tests cover Linux/macOS CPU mapping, HTTP/checksum failures, missing sibling, symlink archive, cleanup and old-binary preservation. Reinstall regression observed RED on prior guard then GREEN for inactive reuse, active refusal and endpoint/attachment preservation. Ownership subprocess exits without cleanup and state can be reclaimed; second owner/legacy marker tests PASS. Generated /tmp systemd unit accepted by systemd-analyze; fake-manager lifecycle tests verify --user routing and active-uninstall refusal. No real service started/stopped. One initial static-check install omitted XDG_CONFIG_HOME and sandbox rejected registration write; rerun used /tmp configuration/state only.

Android JDK 21: testDebugUnitTest, assembleDebug, assembleDebugAndroidTest and lintDebug PASS; 73 JVM tests, no failures/errors/skips. Unsigned assembleRelease fails explicitly as required; disposable-key assembleRelease and apksigner/aapt/certificate verification PASS. GoReleaser 2.8.2 config validation and complete snapshot matrix PASS: six archives, four DEB/RPM, correct binary contents and version diagnostics. actionlint 1.7.7 and git diff --check PASS. Author self-review; no independent reviewer capability used. Cross-builds are not macOS/Windows runtime proof. Hosted PR verification now passes as recorded above. Actual launchd/systemd service lifecycle, production release-tag workflow/signing and phone/emulator runs remain unverified. No emulator started or phone installed, no provider turn, no existing supervisor restart, no GitHub signing-secret changes.

Next: privately configure the release signing identity and protected environment, select license, then prepare a reviewed tag/draft and platform smoke tests before public publication. Existing phone/emulator supervisors and ignored credentials are untouched.

Updated: 2026-11-13. Active branch: `feat/chat-interactions`; prior milestones `756cccc` and `b9944c0`. This snapshot accompanies the single-row thinking follow-up commit.

## Single reasoning status per prompt

Fixed a UI defect reported on phone: each reasoning item rendered its own literal Thinking button with the useful summary collapsed. The display timeline now groups reasoning items by user prompt (turn ID fallback before any user item), preserving commands, replies and canonical persisted messages. Mixed missing/known turn IDs do not duplicate a prompt row. The collapsed label shows the latest nonempty actual summary/title, with muted styling and activity indicator; expanding retains earlier summaries. Empty summaries produce no placeholder row. Scroll counts use the projected timeline so grouping cannot leave the bottom-anchor effect waiting for nonexistent items.

Verified: Android `:app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --offline --no-daemon` PASS, JDK 21, 73 tests, zero failures/errors/skips; `git diff --check` PASS. Four new tests cover grouping across tool output, latest actual label/details, prompt boundaries including legacy IDs, and empty-summary suppression. Initial targeted red failed because the new projection functions were not yet implemented. No host source changes, so Go checks were not repeated. Author source review only. No provider call, APK installation, phone/emulator UI execution, production restart, push, PR or merge. ADB inspection found an existing emulator; this task did not start, modify or stop it. Real summary availability still depends on provider events; no invented model activity text.

## Chat interactions

Implemented compact expandable Codex reasoning summaries, display-only Unicode-safe live text reveal, known per-prompt wall time, and Send/Stop for the exact active turn. Interrupt acknowledgement does not finish a turn; completion does. Missing/stale workers cannot be acquired by Stop. History exposes summaries only and optional timing; raw reasoning content is excluded. Reconnect reconciles uncertain stop state without replaying commands.

Long press opens Edit / Copy / Select text for user messages, Copy / Select text for other messages. Edit loads text into the per-session composer, focuses the keyboard and confirms replacement of a nonempty draft; explicit Send creates a new turn and preserves history. Select text opens a full-screen selectable plain-text snapshot, with Close and Copy all. Clipboard uses canonical text, not the animated display. Existing attachment previews and code copy remain available.

Verified on the uncommitted Android implementation tree and backend `756cccc`: `go test -race ./...`, `go vet ./...`, `git diff --check` PASS. JDK 21 / Gradle `:app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --offline --no-daemon` PASS; 69 JVM tests, zero failures/errors/skips. Initial Android regression was an old test treating reasoning as unsupported; retained unknown-item coverage with an actually unknown type. Gradle needed sandbox socket access. Author source review only. No emulator or phone runtime execution, installation, provider turn, production supervisor restart, push, PR or merge. No emulator was started.

Next: install the built APK when requested, update the host when idle, then verify real long-press gestures, native text selection, draft replacement, summary streaming and Stop during an active/approval-waiting turn. UI interaction runtime and process-level concurrent interrupt coverage remain unverified. Plan: [chat interactions](docs/superpowers/plans/2026-11-13-chat-interactions.md).

Earlier entries below retain their historical branch and verification context.

Updated: 2026-11-13. Integration baseline: `main` at `757c42319f19`; this documentation-only update records the completed integration.

## PR integration and branch cleanup

PRs [#3](https://github.com/warmdev17/modex/pull/3), [#4](https://github.com/warmdev17/modex/pull/4), [#5](https://github.com/warmdev17/modex/pull/5), [#6](https://github.com/warmdev17/modex/pull/6) and [#7](https://github.com/warmdev17/modex/pull/7) were merged using two-parent merge commits. No squash or rebase merge. Every original local branch tip remains reachable from main. Main's complete tree at `757c42319f19` exactly matches tested feature head `1205b61`. Deleted 13 merged local feature branches and 7 merged remote feature branches; only main remained before this temporary documentation branch. Local main and origin/main were synchronized and the working tree clean. The feature sections below retain their original validation context; their earlier no-push/no-merge statements are historical.

Integration verification on `1205b61`: `go test -race ./...` and `go vet ./...` PASS; Python discovery: 17 tests, 8 pass and 9 optional integration fixtures skipped. Android JDK 21: `:app:testDebugUnitTest --rerun :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --offline --no-daemon` PASS, 62 JVM tests without failures/errors/skips. macOS/Windows amd64 CLI and host cross-build PASS; runtime validation Linux only. Primary-agent source review completed; independent reviewer capability unavailable. No new emulator/device execution, paid provider turn, phone installation or production supervisor restart during integration. Emulator dev supervisor and ignored local credentials/config were preserved.

Next: physical-phone validation of multi-session navigation and first-attempt takeover; shared-backend continuation remains deferred.

## Pixel emulator installation and saved SSH profile

Implemented `make emulator-start`, `make emulator` (build and install), `make emulator-install` (reuse APKs), `make emulator-stop` and `make emulator-host` (separate local dev supervisor). Bare `make` remains APK-only, and parallel make cannot install before building. Installer selects an explicit emulator serial plus ro.kernel.qemu verification; never targets a USB phone. An ignored `.modex/emulator.json` references the key file, which is sent through ADB stdin into app-private storage. Test-APK-only instrumentation checks debug/emulator context, imports an unencrypted identity and saves through the existing Keystore-backed SavedHost store. Temporary credential data is removed before reporting success; existing encrypted snapshots are preserved instead of replaced. No production Android source or host protocol changes.

Verified: six Python tests PASS (initial red was missing installer module), including key bounds/encoding, invalid configuration, physical-device rejection and failed-import cleanup/no launch. JDK 21 / Gradle 8.14.3 `:app:assembleDebug :app:assembleDebugAndroidTest :app:testDebugUnitTest :app:lintDebug --offline --no-daemon` PASS; unchanged 62 JVM tests up-to-date, debug runner compiled and lint passed. Real Pixel_4_XL import PASS; UI Connected SSH without form entry PASS; repeat import preserved encrypted snapshot byte-for-byte and left no temporary credential file; reinstall preserved profile and reconnected PASS. `make -n` and `git diff --check` PASS. Pixel started for these tests was stopped and ADB verified emulator-5554 absent. No USB-phone install, prompt/provider turn, production supervisor restart, push, PR or merge. Author self-review.

Runtime prepared for bradar: local profile has the verified laptop host fingerprint, existing authorized unencrypted modex_phone identity path and 10.0.2.2. Credentials are not tracked. Dev supervisor is intentionally left ready at ~/.local/state/modex-emulator; phone state remains separate. Initial reuse of the old phone relay hit an existing replay-gap condition for a cursor-zero client; this work avoids it with an independent dev relay rather than changing journal behavior. Only the empty emulator profile created during this test was replaced when switching to that dev relay. Next: `make emulator-start`, then `make emulator` (or emulator-install for the already-built APK). After a dev host restart, review/reset a stale cursor in the app if requested; never replay uncertain prompts automatically.

Guide: [Pixel setup](docs/emulator.md). Plan: [emulator bootstrap](docs/superpowers/plans/2026-11-13-emulator-bootstrap.md).

## Android multi-session navigation

Opening/creating another conversation now clears selected ID, visible messages and session-specific UI before requesting the host. CLI ownership rejection leaves an empty workspace with the existing takeover option, rather than retaining the previous chat. Navigation no longer depends on selected-session busy state. SessionState tracks active turn IDs independently; live output renders only for the selected conversation, background completion/rejection cannot clear another conversation's busy state, stale completions cannot clear a newer known turn, and older history responses cannot undo newer activity. Active IDs persist in the existing encrypted snapshot; history remains authoritative on the host. Projects and Recents share a compact circular running indicator with accessible label and base tint on selected blue rows. Existing thread-keyed prompts, models, attachments and decisions remain routed by thread.

Verification: JDK 21 / Gradle 8.14.3 `:app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest :app:lintDebug --no-daemon` PASS; 62 JVM tests, zero failures/errors/skips, including five SessionState regressions. `git diff --check` PASS. Wrapper download initially failed inside the sandbox; the authorized build downloaded Gradle and passed. Existing deprecation warnings remain. No physical-phone or emulator execution, APK installation, production host restart, provider turn, push, PR or merge. Backend source unchanged; Go checks not repeated. Author self-review. New chat follow-up: both sidebar and empty-workspace entry points now use Catppuccin Mocha Blue with Base text and icon. The same full Android command passed again on the final styling tree (62 JVM tests, zero failures/errors/skips). No new tests for this color-only change. Next: `make install-apk`, then physical-phone multi-session and CLI-conflict validation.

Plan: [Android multi-session](docs/superpowers/plans/2026-11-13-android-multi-session.md).

Updated: 2026-11-13T03:48+07:00. Branch: `feat/composer-effort-color`; base `862a5c6`. This snapshot accompanies the composer effort-color commit; checks below ran on its identical implementation tree before commit.

## Composer model effort colors

User requested the OpenAI logo, model name and effort label beside upload to change color together with reasoning effort, before any shared-backend implementation. Extracted the existing Catppuccin Mocha slider palette into reasoningColor and reused it as the composer model TextButton content color. ChatGlyph already consumes LocalContentColor, so logo and text inherit one tint; blank effort remains Subtext and disabled state keeps Material disabled colors. Existing per-session/default effort resolution is reused; no new persistence, network or backend behavior. Changed only ChatWorkspace.kt, ModelPicker.kt and this state snapshot.

Verification: Android `:app:assembleDebug :app:testDebugUnitTest :app:lintDebug --offline --no-daemon` PASS using JDK 21 and Gradle 8.14.3. Initial compilation found the missing Color import; added it and all three tasks passed on the final tree. `git diff --check` PASS. APK built, not installed. No emulator or phone visual run, no new tests for simple color reuse, no host restart, push, PR or merge. Shared-backend work remains deferred. Next: `make install-apk` to test colors on the physical phone.


## Takeover shutdown ordering

User reported first takeover returning takeover_uncertain, followed by successful refresh/reopen, and possible interruption of the active computer turn. Confirmed from source that explicit takeover can interrupt active work; it transfers persisted history, not the running computation. Found a reproducible shutdown ordering defect: the launcher waited up to three seconds for the backend while keeping the native TUI paused, and only interrupted the TUI after backend exit. A backend waiting for its client to close therefore reported uncertainty. The independent wait also ignored the request deadline. Two process regressions failed against the original implementation for these exact reasons, then passed after the fix. This reproduces a cause consistent with the screenshot; the exact device event has no retained stage-specific diagnostic proving its individual cause.

Fix: request backend TERM, queue the owned TUI interrupt while it is paused, resume it, then await both actual exit channels under the same request context. Handler budget is ten seconds within the existing twelve-second socket deadline. No acknowledgement before both exits, no automatic takeover retry/prompt replay, and no SIGKILL escalation in takeover. Existing confirmation, same-session inspection, workspace checks and mismatch recovery remain. Source and tests are confined to takeover_unix.go and takeover_unix_test.go.

Verification: full `go test ./...`, `go test -race ./...`, `go vet ./...` PASS; macOS/Windows amd64 host cross-build PASS (runtime Linux only). Disposable real Codex PTY probe PASS for new-session registration, explicit takeover and clean child/registration exit; fixture and private provider copy removed. Tests cover a backend waiting for TUI exit and a backend that does not exit before deadline, including refusal to claim success or escalate to kill. Physical phone retest and real active provider-tool shutdown have not been repeated. Android unchanged, so APK/Gradle/emulator checks not repeated. Author self-review only; no push/PR/merge or deployment of this fix.

Runtime context: the preceding SSH attach failure was traced to a state directory containing only attachments, with no endpoint.json or owner.lock. `make host` was run with the existing /home/warmdev/.local/state/modex-phone and returned supervisor ready. The user's subsequent phone screenshot shows Connected SSH. The running supervisor/launcher for the current conversation has not been stopped or replaced for these fix tests. To apply the correction, close the old launcher when convenient and start a fresh `make cli`; installed CLI users can rebuild with make install-cli and start a new modex cli. Existing Android protocol is unchanged.


## Native CLI session picker

Implemented the approved no-ID workflow: `make cli` or installed `modex cli` lists current-workspace native sessions by name, preview fallback and short ID. Numbered selection, next/previous pages, new conversation and cancellation are supported. Existing `--thread` shortcut and SSH client flags remain compatible. `make install-cli` installs sibling host/client binaries under `~/.local/bin` by default; verification used a disposable /tmp prefix only. New conversations start the real native TUI without synthetic seed prompts, then discover its sole Codex-generated loaded ID and register the same explicit takeover socket. Errors, malformed IDs, cursor loops, observer failures and ambiguous loaded sessions fail closed. Display labels strip terminal controls and are bounded. The Unix client replaces itself with the host so TERM/HUP reach launcher cleanup; Windows refuses this launcher explicitly.

Evidence on the implementation tree: `go test ./...`, `go test -race ./...` and `go vet ./...` PASS. Linux native build and disposable `make install-cli PREFIX=/tmp/modex-picker/install` PASS. Host/client macOS and Windows amd64 cross-build PASS; runtime Linux only. Two real Codex PTY new-conversation probes passed picker selection, automatic native-ID registration, explicit takeover, launcher/backend exit and empty registration root. Initial probes were blocked by sandbox Unix-socket restrictions and then isolated-home login; rerun with socket permission and a private temporary provider configuration passed without a model prompt. Early wrapper cleanup exposed orphan-prone signal routing; process replacement and a subprocess PID regression cover it. Earlier temporary launchers were stopped by exact fixture path. No production supervisor restart, global configuration changes, APK installation or emulator. No push/PR/merge for this picker branch. Author self-review only.

Plan: [CLI picker](docs/superpowers/plans/2026-11-12-cli-session-picker.md). New native conversations appear in persisted session listings only after Codex saves them. Normal unmanaged CLI/Desktop still requires manual close; only a registered launcher can be taken over. Active takeover can interrupt work and does not transfer an in-flight computation. Next: install CLI binaries if desired, run `modex cli` in the project, and confirm the phone handoff; PR/merge awaits user instruction.


## Explicit registered CLI takeover

Implemented the user-approved registered-launcher approach. `make cli THREAD=<exact-native-session-uuid>` runs the real Codex TUI on a dedicated Unix app-server and registers a private CODEX_HOME-scoped control socket. Android offers Take over on phone only for a rejected resume with active_writer, shows the target title and interruption warning, then sends explicitly confirmed threads/takeover. Host checks workspace before release and resumes the same ID once after both owned processes exit. No PID discovery, arbitrary Desktop/CLI killing, fork or prompt replay. Multiple/changed loaded sessions are refused and the paused TUI is resumed. Uncertain outcomes remain manual observation/recovery. Ordinary CLI/Desktop requires manual close; launcher works on Linux/macOS, Windows explicitly unsupported.

Findings verified against installed Codex: standalone Unix listener uses WebSocket, not daemon stdio proxy; native socket path is a symlink, requiring Stat readiness inside the private launcher directory. Added pinned coder/websocket v1.8.14 plus its ISC notice. TERM/HUP cleanup reaps owned children and removes registration. Plan: [CLI takeover](docs/superpowers/plans/2026-11-12-cli-takeover.md); contract: [bridge](docs/bridge-plan.md#explicit-registered-cli-takeover-2026-11-12).

Verification: full go test ./..., go vet ./... and host/bridge/supervisor race PASS on host 3c9b4a5 implementation; Linux/macOS/Windows amd64 cross-build PASS (runtime Linux only). Android assembleDebug/testDebugUnitTest/lintDebug PASS, 57 JVM tests with no failures/errors; assembleDebugAndroidTest PASS on the accompanying Android tree. Python discovery 11 tests, 10 pass and real SSH integration skipped without client binary. Real paid gpt-6.1-sol disposable seed persisted; real native Codex PTY reproduced failures before transport/readiness fixes, then passed active-writer rejection, explicit takeover, native TUI exit, exact same-ID/history recovery and no prompt replay. Separate real native TERM and HUP probes both removed registration and allowed peer same-ID resume. No registered launcher directories left after fixtures. Author self-review only; independent reviewer and physical-phone/UI/emulator execution not performed.

Delivery of the previous milestone: merged through PR #2 as f37dec4, preserving all four original commits. No production supervisor restart, physical-device install or emulator started. Before device testing, rebuild/restart the supervisor when idle and install the debug APK; open the desired existing session through make cli. Active turns/approvals/file writes may be interrupted and background shell processes may survive: takeover resumes persisted history, not an in-flight computation. Stale registration after SIGKILL/crash requires manual inspection. Existing approval/permissions behavior is inherited without a global policy change.

## Phone to desktop writer handoff

User confirmed the project discovery fix and sent messages from the phone, then reported Desktop could not reopen the same session after closing Modex. The persistent bridge app-server retained writer ownership across client disconnect. Real disposable-thread probe confirmed thread/unsubscribe returns unsubscribed yet another app-server still receives active writer; closing the owner permits same-ID resume. Unsubscribe-only prototype was discarded.

Implementation: independent writer app-server per loaded thread, retaining configured executable/args/environment. Supervisor propagates ordered attached/detached state without exposing internal responses in journal. While connected, shell/MCP continuity remains intact. Detach releases persisted idle workers; active turns and approval waits continue offline and release after matching completion only while detached. Empty unpersisted chats stay loaded until their first completed turn. Later sends acquire the same ID without automatic fork/retry; another writer rejects dispatch. Independent decision namespaces preserve routing for simultaneous approval ID collisions and approvals arriving before turn/start acknowledgement. Graceful idle EOF shutdown has a two-second bound; active/uncertain failures keep fail-closed behavior.

Evidence on implementation tree: Go full tests/vet/race PASS; Python discovery 11 tests, 10 pass, real SSH fixture skipped without client binary. Six new handoff process/supervisor tests cover offline completion, independent busy/empty threads, same-ID reacquisition, colliding approval IDs, approval before turn ACK and socket-driven detach, and preservation after uncertain turn acknowledgement. Linux/macOS/Windows host cross-builds PASS; Windows supervisor runtime remains unsupported. Original binary fails the new handoff regression; fixed binary passes. EOF cleanup regression observed failing before CloseIdle implementation. Paid third-party gpt-6.1-sol handoff probe passed two completed turns, same-ID computer resume after each detach, read-only selection/detach release and blocked prompt absent from history. Paid existing provider suite also passed streaming, exact command approval, same-ID resume and new chat. Production supervisor/session untouched; no global config changes, physical-phone installation or emulator. Android code unchanged, so Gradle/device tests not repeated. Self-review only. Before phone retest, rebuild/restart host when idle, then reconnect/reset cursor if required. Use explicit Disconnect because swiping the task can leave foreground SSH connected. No push/merge.

## Native sessions missing from project groups

User reported this exact active CLI session and the Desktop hello session missing from Modex's Modex project. Read-only SQLite metadata and real app-server/host probes confirmed both have cwd /home/warmdev/Workspace/modex, null projectId, no legacy assignment and no explicit projectless exclusion. Host pagination already returned both; Android grouping required explicit membership and consequently omitted them from the project.

Fix: group unassigned sessions by the longest matching root of an existing registered project. Canonical/legacy assignments win; explicit projectless exclusions now survive the bridge as projectless:true and stay outside project groups. Boundary matching avoids sibling-prefix matches, Windows drive paths normalize separators/case, equal-depth ambiguous roots stay in Recents. No invented projects, no remote state/database mutations, no changed session source or archive filters.

Checks on this fix tree: Go regression failed before adding the projectless projection and then passed; full go test ./..., go vet ./... and bridge race PASS. Android assembleDebug/testDebugUnitTest/lintDebug PASS, 56 JVM tests with zero failures. Real read-only Codex versus host comparison over 5-item pages returned identical 16-session sets; four projects loaded over 2-item pages. Exact current session 01a1b731-2a1a-79e0-80fa-c6f24b2bd3b6 and hello 01a1c791-646c-7c13-a7f1-bd07437449f0 both match the registered Modex root; five explicit standalone markers preserved. No resume, prompt, provider turn, active supervisor restart, physical-phone install or emulator used. No push/merge for this fix. Self-review only. Install updated APK and rebuild/restart the host when idle, then Refresh sessions; phone confirmation still pending.

## Per-thread model and reasoning persistence

Model/effort selection now lives in the encrypted per-thread host snapshot and is saved at Use model, before a prompt is sent. Resume/host status cannot overwrite an existing mobile choice. New conversations initialize from configured host settings and advertised default reasoning; the UI removes Keep current and shows the explicit effort or Reasoning unavailable when no value is supplied. First and subsequent turns explicitly use the saved choice without changing global Codex configuration. Forget clears choices; Reset cursor preserves them. Safe host status/thread projections now include configured/reported effort. Plan: [preview/session fixes](docs/superpowers/plans/2026-11-12-preview-session-fixes.md).

Verification on the uncommitted third milestone tree: Android assembleDebug/assembleDebugAndroidTest/testDebugUnitTest/lintDebug PASS (53 JVM tests, no failures); Go full tests/vet and bridge race PASS. Emulator chat/model/effort UI regression PASS. Real pinned loopback SSH + supervisor/fake Codex continuity fixture PASS for two attachments, explicit per-thread model/high effort, disconnect/reconnect and force-stop/fresh-process restoration. The expanded fixture first reproduced an SSH protocol failure: restored image thumbnails opened SFTP during SSH key exchange. Shared file access now requires the exec channel to be connected; preview download waits for an in-progress connection. The same fixture then passed both stages. Temporary raw diagnostic logging was removed.

Paid third-party gpt-6.1-sol/high inference and history reload from a separate fresh host process PASS: correct image/file response, two attachment paths and model/effort preserved. This is host/provider evidence, separate from emulator fake-provider state testing. JSON Mocha viewer fixture PASS with visual screenshot inspection; image/file viewer fixture also PASS. Emulator stopped after testing. Physical phone not installed, production supervisor not restarted, no global configuration changed, no prompts replayed, no push/merge. Self-review only for this follow-up; no independent reviewer tool used. PDF remains Open with. Unknown custom-model effort stays explicitly unavailable rather than borrowing another model's setting. Remote files still need to exist for an uncached preview; the app cache may be evicted by Android.

## Attachment reconnect follow-up

Branch fix/attachment-history-reconnect, base 8f0630d. Reproduced flattened prompt plus file-reference history with failing Go and Android regressions. Host now splits a complete validated reference suffix while preserving prompt bytes; Android also recovers legacy host file references and persists recovered metadata through snapshots. Malformed/trailing content remains visible unchanged. Go full tests/vet and Android assembleDebug/unit/lint PASS. Read-only Codex history probe confirmed separate input parts in current app-server; compatibility regression covers older flattened host output reported on phone. No supervisor restart or user prompt replay.

## JSON preview follow-up

Branch feat/json-preview, base 4d590e6. JSON attachments now format with two-space indentation and Catppuccin Mocha token colors, preserving numeric literals and escape sequences. Invalid, truncated, oversized or overdeep input remains raw text; preview formatting output is bounded. Android assembleDebug, unit tests and lint PASS; emulator verification will be combined with reconnect/model milestones. PDF rendering intentionally remains Open with per user scope.

## Attachment previews and remote folder memory

[Plan](docs/superpowers/plans/2026-11-12-attachment-previews.md). Composer and sent/history messages now display clickable image thumbnails and file tiles. Full-screen image viewer supports pinch zoom; bounded UTF-8 text viewer supports selection; Open with grants temporary read access through a FileProvider limited to preview cache. Images decode at 384px for thumbnails/2048px for full preview. Files download via pinned SFTP into app-private cache, at most 20 MiB/file and approximately 100 MiB total; no external URLs fetched. Uncached previews need the file to remain available remotely. Remote browser is full-screen, starts at SSH home and stores each thread's last successful folder in the encrypted host snapshot, cleared on Forget.

Host history now includes path/name/kind attachment metadata and display_text while preserving legacy filename labels in text. Exact Modex file-reference JSON requires EOF; malformed/trailing text remains ordinary text. Android snapshots preserve original user text even when it matches generated filename labels. Read-only specialist review identified memory/cancellation/text-preservation risks; addressed with separate thumbnail decode bounds, coroutine checks during downloads, separate display_text and strict JSON consumption. Relevant regressions added. Review used source inspection/probes; final fixes verified by builder.

Verification on this feature tree: Go full tests/vet and bridge race PASS; Android assembleDebug/assembleDebugAndroidTest/testDebugUnitTest/lintDebug PASS (48 JVM tests, 0 failures). Emulator viewer fixture PASS for sent/composer thumbnails, image/text preview navigation and FileProvider confinement; screenshots inspected. Full-screen remote picker fixture and existing chat/model/sidebar UI regressions PASS. Real loopback SSH/SFTP fixture PASS with exact preview bytes/cache reuse, cancellation cleanup, home start, per-thread folder isolation and serialized restoration. Paid third-party gpt-6.1-sol turn recognized a red PNG/read a unique file token; real history included both preview paths. Python discovery PASS (2 checks, 3 optional integrations skipped). Emulator shut down after tests; physical phone untouched and production supervisor not restarted.

APK built at the normal debug path; update APK and rebuild/restart host only when idle to get preview metadata on history reload. Physical-phone, external PDF/document apps, Windows/macOS runtime and full accessibility/font-scale matrix unverified. Cache is app-private, not independently encrypted; Android may evict it. Image/file bytes remain outside the journal. No remote upload retention UI or offline archive added. Feature remains local; PR required before merge, no push/merge.

## Composer attachments

[Plan](docs/superpowers/plans/2026-11-12-composer-attachments.md). Implemented + source menu (phone/connected computer), native document picker, SFTP remote browser with multiple selection, removable per-thread draft chips and upload progress/errors. Bundled genuine OpenAI knot vector tinted Mocha Blue replaces the model glyph. Reuses pinned SSH; bounded streaming/atomic uploads in private state/attachments directories. Bridge validates at most 8 readable regular files, 20 MiB each; images use native localImage and files use quoted path references under existing approvals. Attachment-only sends/history supported. No file bytes in journal. ACK clears only submitted draft paths in the origin thread; rejection preserves selections.

Verification on feature tree: Go full suite/vet PASS; Android assembleDebug/assembleDebugAndroidTest/testDebugUnitTest/lintDebug PASS. Emulator chat UI and remote-picker fixtures PASS; source-menu screenshot inspected. Isolated SSH/SFTP integration PASS: exact content, 0700/0600 permissions, partial cleanup, phone URI stream, origin-thread/ACK isolation, literal wildcard/space paths, native image/file mapping. Paid third-party gpt-6.1-sol turn correctly identified a red PNG, read a random token from an attached text file and reloaded history. Python discovery PASS (2 checks, 3 optional integrations skipped). Read-only specialist review found JSch glob expansion; fixed with API-specific escaping and unit/integration regression. Emulator stopped after testing; no physical phone install or production supervisor restart.

Limitations: completed uploads retained (including abandoned drafts), no retention UI; drafts survive rotation/reconnect but not process death. SFTP must be enabled, image capability depends on provider/model. Physical-phone and Windows/macOS runtime unverified. Current Windows supervisor remains unsupported as documented. APK ready at standard debug output. Rebuild/restart host when idle and update APK for device testing. Feature remains local, requires PR before merge; no push/merge authorized for this milestone.

## Minor-change merge workflow

User authorized integrating both local documentation branches. AGENTS.md now requires PRs for functional/quality/security/dependency/build/runtime changes; small nonfunctional instruction/typo/verified-unused-doc changes may merge directly after appropriate checks. Every branch merge preserves original commits with a merge commit, without squash or rebase-merge. Both docs/pr-before-merge and chore/remove-unused-design-system are integrated into main. Verification compares the complete integration diff against c106c0c: only AGENTS.md, this state file and the unused design master change; no runtime/build/test files touched. No build, provider or device tests repeated for this documentation-only integration. Push and removal of merged local branches authorized; confirm final remote and branch status in handoff.

## Unused design artifact cleanup

Removed the unreferenced generated web design master (233 lines, purple/cyan CSS, web fonts and landing-page guidance). Current UI decisions remain in the Mocha chat spec/plan and Android ModexTheme.kt; no unique current requirement was lost. Tracked-file inventory, build manifests, README links, script imports and instrumentation dispatch were inspected. Runtime source, tests, smoke scripts, Gradle wrapper and protocol/design history retained: lack of runtime imports alone does not make these unused. No code/dependency/build changes. Validation: removed-path reference scan and unchanged runtime/build tree comparison; no emulator/device/provider tests needed for this documentation-only cut. Recover the removed artifact from base c106c0c if required. Cleanup commit a79c05e is now integrated into main.

## GitHub publication

User authorized publication to `git@github.com:warmdev17/modex.git`, retaining every commit without squash. Remote inspected empty; all local feature/fix branch tips are ancestors of `1b2ba2e` (38 existing commits). Integrate into main with an explicit merge commit; publish main with all reachable history, keeping local branches. No history rewrite or force push. Pre-publication checks on `1b2ba2e`: Go full suite/vet PASS; Android assembleDebug/testDebugUnitTest/lintDebug offline PASS; Python unittest discovery PASS (2 checks, 3 integration tests skipped without fixture binaries). Pattern scan of 263 historical blobs found no private-key/API-token matches; this is a bounded scan, not a guarantee. Python bytecode ignored at repository root. No emulator started, phone changes, or supervisor restart. Remote push verification belongs to the final handoff.

## Explicit Desktop projects

[Plan](docs/superpowers/plans/2026-11-12-desktop-projects.md), [contract](docs/bridge-plan.md#canonical-desktop-projects-2026-11-12). Host commits `13b3bfb` and `7f18b5f` expose canonical project/list and explicit membership, including bounded read-only compatibility with Desktop legacy assignments using the child CODEX_HOME. Canonical IDs win and projectless exclusions are respected. Android lists registered projects (including empty ones) and groups only explicit members; standalone cwd no longer creates fake projects. Recents remains unchanged. Project discovery errors remain isolated and never hide Recents.

Evidence: full Go tests/vet and bridge race PASS. Real read-only host query returned exactly modex/codjingrank/det/.codex, 13 interactive sessions and 3 explicit memberships. Android assembleDebug/assembleDebugAndroidTest/testDebugUnitTest/lintDebug PASS. Emulator chat-ui fixture PASS: registered folders, standalone ssh-x20 excluded from Projects, model/reasoning and composer regression checks; drawer screenshot visually inspected. Emulator fixture apps replaced after signature mismatch; physical phone untouched. Self-review only. No prompt sent, Desktop state written, supervisor restarted, push or merge. APK ready at normal debug path; host rebuild/restart when idle and APK update required for phone validation. Physical phone runtime, Windows/macOS and older Codex project/list support unverified. Legacy state schema may change; missing/invalid metadata leaves chats in Recents.

## Hide internal review sessions

All-source sidebar discovery had included Guardian approval conversations. Real read-only source inspection confirmed 43 Guardian review threads classified as subAgent.other=guardian. Bridge now requests only cli/vscode/appServer sources in both workspace modes, across all projects/providers. It never filters by title/path and deletes nothing. Real filtered query returned 13 interactive sessions across 11 paths with no internal sources. Regression observed failing before fix; full Go tests/vet PASS. Android unchanged; APK rebuild unnecessary. Running supervisor not restarted; phone refresh requires updated host when idle. Desktop-created cwd under Documents/Codex remains legitimate. No live phone recheck or prompts sent.

## Stable Android debug signing

Phone update rejected new APK certificate ae6929…; installed package on Android guest user had certificate 6c1a50…, matching existing ~/.android/debug.keystore. Re-signed current APK with that existing key and adb install -r on device 24d2ee04 PASS; no uninstall/data clear. Build now pins debug storeFile to user.home/.android/debug.keystore independent of Gradle cache. Keystore remains local and is not committed. Final assembleDebug PASS; generated APK certificate verified matching 6c1a50… old key. App UI not opened automatically; no phone inference recheck.

## Remote project sidebar

[Plan](docs/superpowers/plans/2026-11-11-remote-project-sidebar.md), [contract](docs/bridge-plan.md#remote-project-extension-2026-11-11). Bridge `b73e826` retains default workspace confinement but accepts explicit absolute workspace on thread start/read/resume/turn, with exact ID/cwd validation. All-workspace listing now includes interactive sources across providers (internal-source correction above), ordered by updated_at, excludes archived sessions. Android follows list pages automatically, groups by full cwd with folder icons and collapsible Projects plus Recents, searches titles/paths, selected rows Blue/Base. Selected cwd is stored in existing encrypted snapshot and passed on history/resume/send/new chat. Project folders derive from Codex sessions; desktop-private pinned/empty-project metadata is not imported.

Evidence: Go full tests/vet and bridge race PASS. Read-only real Codex list returned 56 sessions across 16 paths; two-page list and cross-project metadata read PASS. Android assembleDebug/assembleDebugAndroidTest/testDebugUnitTest/lintDebug PASS (40 JVM tests, no failures, lint 0 errors/13 warnings). Emulator sidebar fixture PASS for two project paths and selecting another session, with model/effort and draft regressions; screenshot visually confirms folder icons and Blue/Base selected rows. Fixture needed bounded wait for drawer accessibility after opening. Gradle cache was missing; dependency download/build succeeded. Reviewer agent stream disconnected; self-review fallback. No existing session resumed or sent prompts; host not restarted and physical phone not updated. Cross-project live inference/resume and Windows/macOS runtime not validated.

## Reasoning modal layout fix

Descriptions varied in line count and resized the centered model dialog during slider changes. `ReasoningControl` now reserves the maximum description height at the current width/font scale, with sizing-only copies removed from accessibility semantics. Visible descriptions remain complete. Emulator regression first reproduced a 28px shift in the Use model button; after the fix identical button bounds PASS when changing from long Keep current text to short high description. Android assembleDebug/assembleDebugAndroidTest/testDebugUnitTest/lintDebug PASS (39 JVM tests, no failures). APK rebuilt; no physical-phone installation or font-scale/device matrix checked. No host/protocol changes.

## Model catalog and reasoning

[Plan](docs/superpowers/plans/2026-11-11-model-catalog.md), [contract](docs/bridge-plan.md#model-catalog-and-reasoning-extension-2026-11-11). Bridge catalog/effort milestone `f58e524`. Android requests visible model pages from Codex on connect; errors stay local to the picker and custom model ID remains available. Composer model control is left aligned; New chat uses the same picker. Discrete reasoning slider uses advertised effort strings, with distinct Mocha colors for low/medium/high/xhigh/max/ultra. Changing model clears the queued effort; custom models without advertised efforts retain Codex's current/host setting. Choices send on next explicit prompt and remain scoped to the thread. Selecting Keep current omits effort; it does not reset a previous override.

Evidence: full Go tests/vet and bridge race PASS. Read-only real Codex app-server catalog returned nine models including max/ultra; no provider prompt sent. Android assembleDebug/assembleDebugAndroidTest/testDebugUnitTest/lintDebug PASS (39 JVM tests, no failures; lint 0 errors/12 warnings). Emulator chat-ui fixture PASS: dropdown selection, slider high reaching send, rejected-send draft retention, drawer and code copy. Dialog/composer screenshots visually inspected. First fixture lacked a wait for slider semantics; adding bounded wait passed. An initial concurrent Gradle compilation hit backend/cache errors; sequential rebuild passed. Final reviewer agent disconnected; self-review only. Physical phone and provider inference with chosen effort not tested. No supervisor restart, install on physical phone, push or merge. Rebuild/restart host when idle and install new APK before phone testing.

## Inline agent controls

[Design](docs/superpowers/specs/2026-11-11-inline-agent-controls-design.md), [plan](docs/superpowers/plans/2026-11-11-inline-agent-controls.md). Bridge optional per-turn model override committed at `b554f9a`; muted expandable terminal rows committed at `507a0db`. Composer now queues a validated free-form model ID per session; omission keeps the current model. Approvals and non-secret question inputs render inline for the selected thread, with disabled disconnected/in-flight actions. Question drafts use saved Compose state so lazy-list disposal does not discard answers. File-change acceptance remains disabled pending a diff viewer.

Verification: full Go suite and vet PASS on 2026-11-11; Android assembleDebug/assembleDebugAndroidTest/testDebugUnitTest/lintDebug PASS on the final uncommitted tree (38 JVM tests, zero failures/errors; lint 11 warnings). Earlier emulator fixtures PASS for model selection reaching Send, rejected-send draft retention, inline approvals/questions, selected-thread filtering, disconnected controls, compact terminal expansion/status, history scroll and fresh New chat. Production-context screenshots visually inspected. Final saved-question-state/defensive-guard edits compile; runtime recheck pending because ADB currently has no devices. Independent reviewer failed to start with stream disconnect; self-review fallback. No live model-switch provider turn or physical-phone install in this slice. Existing supervisor was not restarted; per-turn override needs the new host binary before phone testing. No merge/push.

## New-chat empty-history fix

2026-11-11: branch `fix/new-chat-empty-history`, base `dab46a9`. A fresh Codex thread is created successfully but rejects `thread/read includeTurns=true` before its first turn; Android previously issued that read immediately and surfaced the rejection as if creation had failed. New chats now clear the prior transcript/running state and accept a first prompt immediately; resumed sessions still load persisted history.

Checks on the final tree: Android assembleDebug/assembleDebugAndroidTest/testDebugUnitTest/lintDebug PASS. Emulator regression first failed at `New chat must not read unpersisted history`, then PASS after the fix while also checking that resume still requests history. Fresh and persistent-host probes with `gpt-5.6-sol`/Host default both created empty threads; the persistent probe reproduced only the immediate history-read rejection. No provider, keyring, host, or transport configuration changed. Physical-phone New chat is not yet revalidated with the fixed APK.

## Command timeline

2026-11-11: typed chat entries preserve Codex item IDs, separate commentary/final messages and reduce command start/output/completion into one compact expandable card. Running/completed/failed status and exit code are visible; full command/cwd and plain selectable output appear on expansion. Legacy encrypted text history remains readable. Bridge history includes command entries; 64KiB per output and 1MiB total history-output budget with truncation notice. [Plan](docs/superpowers/plans/2026-11-11-command-timeline.md), [contract](docs/bridge-plan.md#command-timeline-extension-2026-11-11).

Checks on final implementation: Go full suite and vet PASS. Android assembleDebug/assembleDebugAndroidTest/testDebugUnitTest/lintDebug PASS, 36 JVM tests, zero failures/errors; lint nine warnings. Emulator fixtures PASS for command running/hidden output/expand/collapse/failed exit and retained output, plus 200-message async history, upward-read preservation and tall-message reload. Expanded/collapsed emulator screenshots visually inspected. Physical phone APK in-place update PASS via adb install -r; data preserved. Device live-provider interaction not revalidated. No live provider turn. Reviewer agent failed with stream disconnect; self-review only. Existing host not restarted: live events work with existing forwarding, but command history requires new host binary/restart when no active work. File changes/MCP tool-specific cards remain outside this slice.

## History scroll fix

2026-11-11: branch `fix/android-history-scroll`, base `71bfc7b`. Opening/reloading history scrolls to its newest content after layout; a tail anchor reaches the bottom of a tall final message. New messages preserve intentional upward reading. Existing chat fixture updated for current permission callbacks.

Checks: assembleDebug/assembleDebugAndroidTest/testDebugUnitTest/lintDebug PASS (31 JVM tests, no failures; lint zero errors, nine warnings). Emulator history-scroll fixture PASS: asynchronous 200-message history, older-reading position unchanged on append, reload with tall final message. Original implementation failed newest-message regression. APK built, not yet installed on physical phone. No Go/provider/host changes. Next: separate command execution cards and message IDs per user's follow-up.

## Connection diagnostics and live host recovery

Laptop IP now 192.168.1.243, phone Wi-Fi 192.168.1.244, sshd listening on 22. Phone ping to laptop PASS. `/tmp/modex-host` had disappeared and supervisor was dead with stale private endpoint/lock. Verified no host process and endpoint refused; removed only stale files, rebuilt and started supervisor. OpenSSH with modex_phone and locally verified ED25519 pin authenticated and attached; provider third-party/model gpt-6.1-sol confirmed. New instance requires phone Reset session cursor; old saved IP must change (Forget host required by current target-change guard). No prompt sent.

Android now reports fixed stage-specific failures for settings/key, SSH network/auth, pin mismatch, remote executable, unavailable/busy supervisor and instance/replay; 4096-byte synchronized stderr capture stays in RAM and is neither displayed nor logged. Explicit disconnect returns quietly; unexpected EOF remains visible. Same SshTransport code on JVM with actual SSH/key/pin PASS for normal attach, wrong fingerprint, missing binary and old-instance rejection. Probe caught shell echo of --cursor misleading replay classifier; fixed with exact known phrases and regression. Final assembleDebug/testDebugUnitTest/lintDebug --offline PASS (31 tests, zero failures/errors); no Go source changed. Self-review only; no physical Android transport rerun.

APK update via make install-apk FAILED: INSTALL_FAILED_UPDATE_INCOMPATIBLE (debug signing certificate differs from installed app). Did not uninstall or delete app data. APK built at normal debug path; need original signing key or explicit user choice before uninstall/reinstall. New diagnostics not on device yet. `/tmp` host remains temporary; permanent install/startup reliability deferred.

## Explicit new-thread permissions

Continuing the Codex companion direction after user evaluated T3. [Plan](docs/superpowers/plans/2026-11-10-thread-permissions.md), [contract](docs/bridge-plan.md#thread-permission-extension-2026-11-10). New chat offers Host default (unchanged inherited permissions), Read-only, Workspace write, Full access with warning. Explicit sandboxed modes request on-request/user, full access never/user. Host policy can reject choices. No global configuration changes; no overrides on resume/turn. Session settings menu projects Codex-returned provider/model/legacy sandbox type/approval/reviewer; missing policy labels remain Unknown. Metadata cleared during switching/reconnect/reset/disconnect and not persisted as authoritative. Settings report create/resume time; full active permission-profile/tool-scope display remains unimplemented. Read-only may request permission escalation, not an immutable no-write guarantee; file approvals cannot be accepted without a diff viewer.

Evidence on final uncommitted milestone: Go suite PASS; bridge race PASS; go vet PASS. Android assembleDebug/testDebugUnitTest/lintDebug --offline PASS, 25 tests/0 failures/0 errors, existing warnings. Go and Kotlin new tests observed fail before implementation. Installed Codex schema validated request/response fields. Real Codex creates returned expected permissions for all three explicit modes; one short paid third-party gpt-6.1-sol marker turn in temporary workspace followed by fresh bridge same-ID resume returned readOnly/on-request/user. Empty-thread resume probe rejected before that; no existing user session touched. Independent reviewer attempted but stream disconnected, so self-review only. New dialog not visually verified on emulator/phone; APK built but not installed, phone supervisor not rebuilt/restarted. Updating host requires intentional stop/start and resetting old journal cursor. Next: physical UI/approval validation, then reliable CLI-to-phone handoff; do not claim control of an already-running CLI process.

## SSH key import troubleshooting

User reports both private-key files fail import after USB transfer. Local originals have OpenSSH headers, no encryption and are below 1 KiB; phone copies not verified. Existing catch-all message incorrectly suggested encryption for every exception. File picker resumes Activity, whose foreground handler can auto-connect a saved profile before import; import rejects connected/connecting state. Disable reconnect via existing disconnect before launching picker, retain state guard with clear guidance, distinguish denied access, unreadable/provider error, empty and oversized file. Key bytes/errors are not logged; import preserves prior key on failed read. No new format validation or passphrase support.

Checks on uncommitted fix: four reader tests PASS (byte preservation, exact limit, empty/oversized, I/O failure), full JVM suite PASS; assembleDebug/lintDebug PASS. First concurrent Gradle invocations collided in Kotlin cache; tests rerun successfully after build finished. Physical phone visible over ADB but picker/import and auto-reconnect race not runtime-verified; APK built, not installed. No Go changes or repeated Go suite.

## Host Make targets

Added `host-build`, `host` (background), `host-attach`, `host-stop`; existing default Android target unchanged. Defaults `/tmp/modex-host`, current workspace, `$HOME/.local/state/modex-phone`, `codex`; overrides documented in README. Atomic binary replacement does not restart existing supervisors or preserve an approval wrapper automatically. Checks on uncommitted milestone: actual build PASS, dry-run with spaced paths/default Android goal PASS, isolated fake-provider Make start/attach/stop lifecycle with spaced workspace/state/Codex paths PASS (loopback permission required). Stop acknowledgement precedes asynchronous endpoint cleanup; fixture waited for removal. No live phone supervisor stopped/restarted; no paid provider or Android checks for this Makefile-only change.

## Current troubleshooting

Phone screenshot confirms the desktop session `01a1b731…` is listed with its preview. Earlier workspace/active-writer explanations were hypotheses, not verified causes. Read-only live probe reproduced the actual history failure: this session's upstream includeTurns response was 4,408,427 bytes (81 turns), exceeding the bridge's 4MiB frame cap and closing its Codex reader. Raised only upstream cap to 16MiB; downstream remains 4MiB. Fixed bridge read this exact session into 281 text messages (167,034 bytes) and then successfully listed all six sessions without resume, fork or prompt. Resume ownership on this currently running session has not been tested. History above either cap remains unsupported pending pagination.

Large-history correction committed at `9022eb7`; user confirmed this session now loads on phone. Existing 16MiB upstream / 4MiB downstream bounds remain.

## Current UI milestone

Approved [UI design](docs/superpowers/specs/2026-11-09-android-chat-ui-design.md) and [implementation plan](docs/superpowers/plans/2026-11-09-mocha-chat-ui.md). Native Catppuccin Mocha blue theme, left conversation drawer with search/refresh/load-more and selected session, compact header, new-chat model dialog, 48dp arrow send control with 10dp corners, matching system bars. Connection settings and approvals preserve existing behavior. Per-session drafts survive drawer switches; ViewModel send reports local acceptance before composer clears. Drafts are UI saved state, not a durable encrypted draft store.

Codex schema/live metadata verified `name` is the user-facing title and `preview` is usually the first question; this session reports `name=modex`. Bridge list forwards bounded optional name; Android prioritizes name, then preview, then untitled + short ID. Workspace filtering unchanged.

CommonMark 0.24.0 + GFM table parser; native Compose headings/emphasis/lists/quotes/safe HTTP(S) links/tables, inline code and fenced/indented code with language label, horizontal scroll, selection and Copy code. HTML remains inert; no remote images, upload, MCP/skill picker, syntax highlighting or math/Mermaid in this slice.

Checks on final production working tree: full Go suite PASS; Android assembleDebug/assembleDebugAndroidTest/testDebugUnitTest/lintDebug PASS (17 unit tests, zero errors; existing warnings). Emulator chat/drawer visually inspected; code copy and session selection PASS. Keyboard screenshot confirms visible draft and reachable blue send control. Extended draft/table smoke PASS: emulator fixture verified code copy, drawer selection, rejected-send draft retained, keyboard composer and Markdown table screenshot. Temporary Gradle cache reset required redownloading dependencies; previous TLS dependency issue recovered. Independent reviewer probe failed with stream disconnect, so self-review only. No physical-phone validation of this new UI yet; no paid model turn. Host binary built at /tmp/modex-ui-host; existing phone supervisor not changed this session.



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

Primary owns integration/state. Backend and reviewer subagent probes for continuity both failed immediately with stream disconnect; no delegated edits/findings accepted. Inline fallback.

Current milestone: black native chat UI, stored text history and encrypted connection/session restoration with a foreground connection service. Host API implemented at `7c93ba5`; Android build/tests/lint and emulator restore checks pass. Plan: [session continuity](docs/superpowers/plans/2026-11-09-session-continuity.md); [history contract](docs/history-contract.md).

Next after this milestone: physical phone verification of saved host + history + multitasking; session titles, full file-diff review, done/decision notifications and push relay/bubbles. Windows supervisor ACL remains outstanding. No billing/license/release decision; no publish/push.

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


## Session continuity milestone

User requested black clean native UI, durable history and connection across app exit/multitasking. Implemented host `threads/read include_turns=true`, exact workspace/ID verification, text-only user/assistant messages and active-turn status; default metadata/resume contracts unchanged. No system/developer/tool details exposed. Frame limit 4MiB still applies; no history pagination. Go tests/race/vet PASS; read existing phone thread returned 15 messages across both roles without resume/prompt.

Android: fixed neutral black theme, gray user bubble/unboxed assistant; encrypted single-host profile/private key/messages/decisions/pending-request IDs/selected thread/journal via Keystore AES-GCM in noBackupFilesDir, atomic write, tamper detection. Forget removes ciphertext/key and invalidates delayed saves. App-owned ViewModel preserves Activity recreation; foreground dataSync service begins on explicit visible connection and provides ongoing notification + Disconnect. Foreground return attaches saved connection if disconnected; no automatic prompt retry. Process restore reads encrypted snapshot then loads stored history. Reset cursor keeps host/key; Forget removes them. Notification permission requested on Android 13+. Services are OS-limited and not restarted after force-stop/reboot; no claim of indefinite background survival.

Evidence before Android commit: Gradle assembleDebug/assembleDebugAndroidTest/testDebugUnitTest/lintDebug PASS (8 tests, lint 0 errors, warnings remain). Emulator storage instrumentation PASS: encrypted round-trip, ciphertext excludes marker, tamper rejected, forget; continuity fixture over native SSH + real supervisor/fake provider PASS for initial stored-history display then force-stop and fresh-process auto-attach/history restoration. Screenshot inspected: black chat with restored user/assistant messages. First attempts hit notification permission dialog and stale fixture profile; fixture initial stage now clears only emulator test profile, grants notification in test setup, then verifies restoration in a distinct process. No paid turn sent in continuity test. Physical phone still needs updated APK and first import once; no device/battery endurance, Android 8 runtime, tablet, landscape, large-font or screen-reader proof. Reviewer unavailable, self-review only.

Phone host was updated/restarted with history-capable `/tmp/modex-host` preserving `/tmp/modex-codex-approval` read-only/on-request/user/gpt-6.1-sol wrapper and same workspace/state dir; no history/config deletion. Instance changed; old phone cursor needs one Reset session cursor. /tmp host/wrapper remain temporary, no boot service installed.
