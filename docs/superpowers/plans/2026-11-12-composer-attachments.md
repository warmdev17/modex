# Composer attachments implementation plan

Goal: + offers From this phone / From connected computer; select files/images, review removable draft chips, send real Codex inputs. Replace the model glyph with a bundled OpenAI vector tinted Mocha Blue. User specified native Android and both sources; transport/limits below are builder choices within that request.

Design: reuse the authenticated, pinned SSH session with a separate SFTP channel. Browse remote account-readable paths, starting at the selected workspace. Phone files stream through SAF and SFTP to private unique directories beneath the configured supervisor state/attachments. Atomic rename only after bounded upload. Do not send file bytes through the journal. Retain sent uploads because stored Codex conversations reference them. Unsent drafts survive rotation/reconnect in the app ViewModel, not app process death. No automatic upload or prompt retries.

Contract: turn/start gains optional attachments:[{path,kind:"image"|"file"}] (max 8, 20MiB/file, absolute regular files). Text may be blank only when attachments exist. Host validates paths and types before upstream calls; images use native localImage, other files add JSON-encoded user-selected path references for Codex tools, subject to existing sandbox/approvals. No generic remote-write API. File permission/provider/model errors remain visible without exposing raw errors. No provider keys leave the target.

UI: + menu has exactly two sources; phone uses Android document picker without storage permissions. Remote dialog shows path input, Up, directories/files, bounded listing and loading/error states. Composer chips show name, image/file glyph and removal; progress/error stays local to attachments. Disable sending while upload is incomplete; do not move a picker result into another thread after switching. Keep model control left aligned beside the separate + action. No thumbnail downloader, document conversion, billing/backend or broad storage permissions.

- [ ] Bridge: write failing tests for native image/file mapping, attachment-only turn, invalid/missing/oversized files and history image labels; implement validation and contract docs; full Go suite/vet/race; commit.
- [ ] Android transfer: add bounded stream/naming/model helpers with JVM tests, separate SFTP channel listing/private atomic uploads; draft/progress state keyed by thread; preserve existing SSH behavior.
- [ ] Android UI: menu, SAF picker, remote browser, removable chips, tinted OpenAI vector; wire real send; UI fixture; build/unit tests/lint.
- [ ] Verify with ephemeral file/image inputs and emulator fixture where possible. Always stop the emulator started for this task after tests. Review whole diff; update PROJECT_STATE.md and commit coherent milestones. Feature requires PR before merging; do not merge/push without authorization.

Review focus: stale picker/upload completion after host/thread change; partial uploads and disconnect; directory/symlink/nonregular inputs; channel cancellation; attachment-only history; model-provider image support; Windows SFTP path translation (runtime may remain unverified).
