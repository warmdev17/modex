# Thread permission modes

Continue the agreed Codex companion direction with explicit new-thread permissions.
Implement inline; preserve existing session/provider/workspace ownership checks.

- Extend `threads/start` with optional `permission_mode`: `host-default` (omit overrides), `read-only`, `workspace-write`, `full-access`. Reject all other values/types before upstream. Explicit modes route approvals to the user; sandboxed modes use on-request; full access uses never and requires a visible UI warning.
- Return only safe effective model/provider/approval/sandbox metadata from Codex create/resume responses. Never infer effective permissions from the requested mode. Missing metadata is unknown; do not change permissions on resume or turn start.
- Add native radio choices to new-chat dialog, reset to host default every time it opens, and show returned session settings in a compact details dialog. Clear settings during session switches, reconnect and reset so stale permissions are not attributed to another session. Restored history without resume has unknown permissions.
- Tests first: Go valid modes, invalid/null/unknown values, unchanged default request, response projection without unrelated fields; Android request serialization and metadata formatting. Run Go suite plus Android JVM tests/build/lint sequentially. Provider schema from installed Codex is the contract reference; live paid test only if needed to resolve behavior.
- Update bridge contract/README/state and commit one coherent feature after reviewing the diff. No deployment, automatic host restart or device installation.
