# Explicit CLI takeover implementation plan

Goal: let the phone explicitly close a registered desktop CLI and resume its exact native session.

Approved direction: user requested implementation of the previously proposed explicit takeover/registered-launcher design. Execute inline, without another approval round. Reuse Go standard library, Codex Unix WebSocket transport and existing Android error recovery UI.

Safety contract: no PID discovery, pkill, desktop database edits, session forks or prompt replay. Only the launcher's own child processes may be stopped. Unmanaged CLI/Desktop sessions are read-only until manually released. Linux/macOS launcher only; Windows fails explicitly. Confirmation warns about interrupted turns and surviving shell commands. Registration is scoped to CODEX_HOME and thread ID. Reject multiple loaded sessions or changed session ownership. Freeze the owned CLI during ownership validation and release it on refusal; check that the dedicated app-server has exactly the requested thread loaded before stopping it. Do not automatically escalate to SIGKILL on takeover. Resume only after owned backend exit is confirmed. A timed-out request is uncertain and must not be automatically retried.

- [x] Launcher: add --cli --thread ID mode, private per-thread Unix control socket, dedicated Codex server and native TUI, exact loaded-thread verification and bounded release. Test unmanaged, mismatched/multiple loaded threads, explicit confirmation and exit-before-ACK using disposable processes.
- [x] Bridge: threads/takeover validates confirmation and workspace using read-only metadata, calls local launcher, then resumes the same ID once. Test validation and dispatch ordering.
- [x] Android: remember the rejected resume target, offer Take over with warning dialog, clear it on switch/disconnect; persist pending command without auto replay. Treat result like resume; test error copy and target selection.
- [x] Verify: Go test/vet/race, Android build/unit/lint; real disposable Codex server/CLI smoke where environment allows. Never stop this conversation, production host, or physical device. Update PROJECT_STATE and commit coherent verified changes, no push or merge.

Limits: registration requires starting through the new launcher; no retroactive takeover of arbitrary CLI/Desktop. Background terminals may outlive Codex. The launcher does not migrate an in-flight operation; history resumes from persisted state. Existing unmerged fixes remain branch ancestors.

Implementation finding: installed Codex creates a Unix socket symlink, so readiness follows it with Stat inside the owned private directory. Its standalone Unix listener uses WebSocket framing; daemon stdio proxy is incompatible. Added pinned github.com/coder/websocket v1.8.14 rather than hand-writing a protocol parser. Real native PTY fixture now passes ownership rejection, explicit takeover, same-ID/history restoration and no prompt replay.
