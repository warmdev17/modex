# OpenCode conversation capabilities implementation plan

> **For agentic workers:** Use subagent-driven-development. Follow red, green, review and commit for each task.

**Goal:** Android identifies the agent on SSH attach and only offers conversation controls the connected backend supports.

**Architecture:** Add backend identity and optional-feature flags to the authenticated supervisor ready frame. Apply them to the live Android connection before initial requests and pass that policy to the existing chat UI.

**Tech stack:** Go supervisor, SSH JSON frames, Kotlin and Compose.

**Spec:** [OpenCode integration design](../specs/2026-11-26-opencode-integration-design.md), Stage 2. This plan covers capabilities and identity.

## Global constraints

- Older ready frames without metadata retain existing Codex behavior.
- Supported chat operations, attachments and permission approvals stay available on OpenCode.
- Keep capability state scoped to one live connection. Keep files and drafts when reconnecting or changing backend.

## Task 1: Advertise the backend on attach

- [x] Add a failing test in `internal/supervisor/server_test.go` asserting explicit JSON `backend` and `capabilities` on ready.
- [x] Add optional supervisor metadata and set it in `cmd/modex-host/supervisor.go` based on the selected backend. Use strings `codex` and `opencode`. Capabilities are an array of strings. Codex declares `session_status`, `plan_mode`, `permission_profiles`, `session_lifecycle`, `projects`, `references`, and `takeover`. OpenCode declares an empty array. An unconfigured supervisor omits both fields. `--attach` forwards the ready frame verbatim.
- [x] Verify legacy caller and OpenCode attach behavior with Go unit and command integration coverage. Commit the host milestone.

## Task 2: Apply capabilities to Android chat

- [x] Test parsing explicit OpenCode capabilities, explicit Codex capabilities and a legacy ready frame. Include successive handshakes so a new connection replaces the prior capability set.
- [x] Use the attach metadata in `Protocol.kt`, `SshTransport.kt` and `ModexViewModel.kt` before initial requests. Update `MainActivity.kt`, `ChatWorkspace.kt` and the relevant Compose components to use backend labels and hide unsupported controls. Gate background requests and avoid submitting unsupported plan or reference fields. Preserve core chat, files and approvals.
- [x] Keep flags in memory. Reset archived filtering before listing if lifecycle is absent. Missing backend and capability fields mean legacy Codex. For an explicitly identified backend, absent or malformed capabilities disable optional features. Render OpenCode in the existing composer and header, and hide project sections, archive and per-row actions, permission-profile controls, usage/context, mode controls, takeover and reference picker when their respective flags are absent. Treat old draft reference markers as ordinary text when sending on that connection.
- [x] Verify legacy Codex behavior and the OpenCode chat UI with Android unit tests, lint and UI smoke where available. Commit the Android milestone.

## Review focus

- Attach metadata must derive from the actual supervisor backend, independently of the attach CLI flags.
- Capabilities must reach Android before initial refreshes and reset for replacement connections.
- Cached references must survive and must not block text/file sending to OpenCode.
- Permission profiles and permission approval cards have distinct availability.

## Implementation record

Both tasks are implemented and verified. Independent review found that the
Codex attachment menu hid Skills & plugins until the picker was already open.
The fix separates backend availability from picker visibility. The Android
smoke failed with the missing menu entry before the fix and passed after it.
The OpenCode smoke confirmed the same menu entry remains hidden there.
