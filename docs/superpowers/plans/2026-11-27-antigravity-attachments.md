# Antigravity prompt attachments implementation

> Use subagent-driven development for the host and Android tasks. Primary owns integration, the live ACP probe, and project state.

**Goal:** Send screenshots and UTF-8 files from the existing Modex attachment picker in an Antigravity chat.

**Architecture:** SSH/SFTP remains the file transport. Android sends selected host paths in the existing turn request and the host reads the file privately to build native ACP content blocks. The journal and Android timeline carry attachment metadata.

**Tech stack:** Go, ACP v1, Jetpack Compose.

## Global constraints

- Keep Codex and OpenCode behavior, Antigravity text-only compatibility, turn acknowledgements, Stop and permission flow.
- Accept PNG/JPEG images and UTF-8 text files, including messages with attachments only. Allow up to eight files, 8 MiB total, and 256 KiB per text file. Reject unreadable, empty, unsupported or oversized selections before dispatch with fixed actionable errors.
- Gate Android support on `antigravity_attachments` in the Codex host hello. For each ACP process, check `promptCapabilities.image` and `promptCapabilities.embeddedContext` before using the corresponding block.
- Use `attachments:[{path,kind}]` with `kind` `file` or `image`. Live confirmation and history use `attachments:[{path,name,kind}]`. Never forward file bytes in journal/history/diagnostics. Replay attachment metadata only from native local file URIs when available.
- Verify against the installed AGY in an isolated workspace and supervisor. Stage any private APK in the main checkout's `.modex/previews/`.

### Task 1: Host attachment delivery

- [x] Cover capability discovery, mixed/attachment-only native prompt shapes, refusal before dispatch, live metadata and local-only replay projection with Go tests.
- [x] Extend the ACP client with typed prompt blocks while preserving text-only callers. Read selected files once using bounded reads and authoritative MIME/UTF-8 checks. Keep ACP-native content separate from Modex-visible metadata.
- [x] Route attachments in Antigravity's existing turn/start and advertise `antigravity_attachments` in the Codex supervisor hello. Preserve exact routing and session isolation.
- [x] Run Go tests and vet. Report limits and native history behavior.

### Task 2: Android attachment composer

- [x] Cover the new hello capability and attachment-only serialization, while asserting Antigravity omits model, reasoning and references and older host gating remains.
- [x] Enable the existing attachment picker, chips, upload progress and errors for supported Antigravity chats. Preserve phone/host file selection. Keep the prompt and attachments for rejected or unconfirmed starts and clear only acknowledged submission content. Add a narrow UI smoke for sending a file-only turn.
- [x] Run Android unit tests, lint and affected UI smoke.

### Task 3: Integration

- [x] Probe with a synthetic PNG and text file. Check real ACP response and replay, isolated supervisor flow and Android installation/upgrade.
- [x] Update the bilingual support guide and commit the verified feature. Record delivery evidence and host activation in PROJECT_STATE.md.
