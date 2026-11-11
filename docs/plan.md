# First host diagnostic implementation plan

Execution: single primary agent. This records the scoped initial milestone, not a full release plan.

Goal: prove that an installed Codex app-server can provide effective configuration and stored workspace sessions without invoking a model.

Architecture: Go standard library CLI, explicit workspace, stdio JSON-RPC to target Codex. Dependencies: none. Design: [design.md](design.md).

- [x] Generate the installed Codex protocol schemas outside the repository and inspect initialize/config/read/thread/list.
- [x] Test allowlisted configuration summaries and malformed responses.
- [x] Test response ID matching, RPC failure redaction and oversized frame rejection.
- [x] Implement a time-bounded child process diagnostic, preserving inherited environment and rejecting unsupported profiles explicitly.
- [x] Smoke-test a real Codex app-server with an isolated third-party configuration and no inference.
- [x] Verify cross compilation for Linux, Windows and macOS and record actual checks.

Review focus: credential-bearing responses; oversized output; wrong request IDs; explicit workspace validation; unsupported options. Follow-up tests should exercise child-process timeout/failure and CLI argument validation through the executable boundary before expanding this probe into a host service.
