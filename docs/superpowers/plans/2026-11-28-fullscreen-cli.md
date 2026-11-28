# Fullscreen Modex CLI implementation plan

The terminal fills its alternate screen from agent discovery through the
session picker and conversation. Keep the Modex dark and light RGB palettes
and use the available terminal dimensions. In a pipe, keep the existing
line-oriented behavior. The fullscreen workspace operates one session at a
time and keeps the existing Modex conversation identity.

- [x] Implement the framed terminal surface and open the selector only when
  both standard input and output are a terminal.
- [x] Choose an agent and session with arrows and Enter. Escape returns to
  agents or exits. Preserve unavailable-agent explanations, cursors, and
  the selected session's workspace.
- [x] Present shared Antigravity and OpenCode chats with a scrolling
  transcript, a fixed prompt, streaming, and the existing history, Stop,
  and decision flow. Restore the terminal on exit. Hand Codex sessions
  to Codex's own terminal UI.
- [x] Verify keyboard controls, resize, RGB and monochrome, prompt
  submission, phone coexistence, and terminal cleanup in an actual PTY.
  Keep Go 1.23 and preserve the noninteractive tests.
