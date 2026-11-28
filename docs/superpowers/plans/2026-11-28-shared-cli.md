# Shared Modex CLI implementation plan

The terminal starts with the runtime picker and then the selected runtime's
workspace sessions. The phone and terminal operate on the same supervisor
sessions. Existing direct Codex resume still opens the native TUI.

- [x] Build a keyboard-operated terminal picker with a framed layout,
  adaptive light/dark RGB palette, monochrome for NO_COLOR and redirected
  output, safe display of names, back/cancel, paging and new conversation.
- [x] Expose an authenticated local CLI relay on the existing loopback
  supervisor endpoint. Its independent connection must coexist with the
  phone lease. Forward only expected operations, keep CLI results outside
  the mobile journal, and fan out live events from the same runtime.
- [x] Wire agent-scoped selection and shared Antigravity/OpenCode chat:
  create/resume, history, new prompt, live responses, Stop, permissions,
  and disconnect without stopping the supervisor. Keep Codex's native TUI.
- [x] Verify framed output in a terminal, phone/CLI coexistence, same
  session identity, replies in both directions, reconnect and unaffected
  Codex behavior. Build and verify the host and client binaries.
