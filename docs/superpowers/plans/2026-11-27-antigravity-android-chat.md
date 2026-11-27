# Antigravity Android Chat Implementation Plan

> Implement the following tasks against the existing Modex host and Android protocol.

**Goal** — Choose Antigravity in New chat and use text, Stop, history and one-time approvals.

**Spec** — [Antigravity Android chat](../specs/2026-11-27-antigravity-android-chat-design.md).

- [ ] Extend the ACP client to deliver typed, session-scoped permission requests with a bounded one-shot reply. Verify approval, rejection and expiry with the fixture.
- [ ] Implement the host Antigravity chat handler. Persist a Modex-generated ID, workspace, project ID and native ACP reference, and project existing Modex thread, turn, history and decision envelopes. Verify reconnect, session routing, denied options and exact-turn cancellation.
- [ ] Mount the handler on the existing Codex host bridge and supervisor. Keep existing methods and cursors compatible. Report runtime discovery as `agents/list → {data:[{id,label,ready,reason?}]}`. Use `threads/start` with optional `agent:"antigravity"`; return `thread.agent` in AGY thread metadata. Append stored AGY chats on the first unarchived Codex list page.
- [ ] Add the runtime choice to the Android New chat UI and apply capability gating to the selected conversation. Use the existing text transcript, Stop button and inline decision cards. Verify the picker and legacy fallback, plus AGY-specific chat behavior.
- [ ] Build and run isolated supervisor and Android checks, exercise real AGY with one short text turn, and verify restart recovery. Record exact source and evidence before delivering.
