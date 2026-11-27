# Antigravity in Android New chat

## Goal

On an existing Modex host connection, create a Codex or Antigravity chat
from New chat. An Antigravity chat supports text, streamed responses, Stop,
history, reconnect and one-time tool permission decisions.

## Approach

Extend the normal Modex connection with runtime discovery and a runtime
argument on chat creation. The host keeps each Antigravity chat's Modex ID,
workspace and native session reference in private supervisor state. Requests
for existing chats follow their stored runtime. Codex remains the default for
clients that omit the runtime argument. Codex list cursors remain independent
from the Antigravity chat list.

A second host connection or a global agent switch would leave selection tied
to transport. Per-chat routing lets both runtimes operate on the same host
and preserves the workspace in project New chat.

## Behavior

- Show Antigravity readiness from a host capability probe. Missing CLI,
  incompatible ACP and unavailable authentication get actionable reasons.
  An ACP handshake alone does not certify model access.
- Acknowledge the Modex turn separately from its streamed text and terminal
  state. Stop matches the active turn. Unknown transport outcomes preserve
  drafts and are never retried automatically.
- Reuse the inline approval card for native one-time decisions. Apply only
  an option offered for the same pending request and active turn. Unsupported
  requests use ACP's cancelled outcome.
- Reconnect through the existing host journal. After restart, restore the
  Modex chat ID and load its native history in the recorded workspace.
- Keep existing Codex and OpenCode flows compatible. The first Antigravity
  preview uses text, the host's default model and its own advertised
  capabilities. Model selection, attachments and cross-agent workflows are
  outside this milestone.

## Verification

Exercise the runtime picker on an Android emulator and use an isolated
supervisor for both ACP fixture tests and a short real Antigravity turn.
Cover disconnected streaming, restart recovery, Stop and permission denial
without moving the running Codex or OpenCode Preview services.
