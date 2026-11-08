# Modex next milestone

This milestone turns the connection-scoped bridge into a recoverable remote session without moving API keys off the target machine.

## Order

1. **Decision protocol**: preserve Codex server requests instead of closing the connection, classify approval/question requests, expose an allowlisted `decision/respond` command, and test replay/timeout behavior with a fake app-server.
2. **Detached host**: add a platform-neutral supervisor contract around `modex-host --serve`; keep workspace and `CODEX_HOME` explicit, write no secrets to logs, and define restart behavior only after transport loss is observable.
3. **SSH transport**: use the system SSH executable first on Linux, Windows and macOS. The mobile client sends framed Modex JSON over an SSH channel; the host remains the only process that starts Codex.
4. **Android native shell**: Kotlin/Compose session list, streaming transcript, approval sheet, connection state, and notification channel. The UI consumes the Modex protocol and never stores provider keys.
5. **Continuous sync**: add a durable host-side event cursor and reconnect handshake. A reconnect must resume observation without replaying an uncertain turn.

## Constraints

- Never bypass Codex writer ownership or silently fork an existing thread.
- Never return raw upstream error text, command output, or credentials to the phone.
- A turn request is not replayed after timeout unless the user explicitly retries it.
- Push notifications require a separate relay when the phone is offline; SSH alone cannot wake a stopped app.
- Real provider inference and approval behavior must be verified against a live Codex build after protocol doubles pass.

## Acceptance slice

The first shippable vertical slice is: Android connects through SSH, lists the exact workspace's existing threads, resumes a same-ID thread when no other writer owns it, streams late events, and surfaces an approval as an explicit user decision. Disconnect/reconnect preserves observation state and never duplicates a turn.
