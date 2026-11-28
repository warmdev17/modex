# Antigravity command activity implementation plan

Show native AGY shell calls in the existing Android command card during a turn
and when reopening the conversation. This is a host-only update to the
Antigravity integration. Permission decisions continue through the current
one-time approval flow.

- [x] In internal/acp, decode content and raw tool payloads by update type.
  Recognize AGY 1.5.17 `run_command` and extract bounded `CommandLine`,
  output text and an optional numeric exit code. Preserve subagent parsing
  and tolerate sparse ACP updates. Ignore unrelated tool content and raw
  fields. Test observed array content and status-only completion.
- [x] In internal/antigravity, route calls by native session, turn and call
  ID. Emit existing `commandExecution` items and persist the projection as
  `role: command` in loaded history. Use one card per invocation. Keep
  terminal states across duplicate or late updates and end unresolved cards
  without inventing an exit result. Test live updates, replay, aborted turns
  and isolation. Include bounded fields in the admission queue budget.
- [x] Run the full Go race suite and vet. On the exact stamped host, verify
  a benign native shell call with explicit one-time approval, live output,
  exit code and cold restart history. Verify shared-runtime routing.

Activation uses the delayed restart after verification.
