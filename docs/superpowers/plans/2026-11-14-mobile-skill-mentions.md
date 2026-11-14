# Mobile skill mentions implementation plan

> Execute inline with executing-plans; preserve existing local state notes.

**Goal:** Invoke remote Codex skills from Android via @ suggestions and exact typed inputs.
**Architecture:** Workspace-scoped references/list wraps native skills/list; turn/start validates structured references. Android separates reference data/token operations from the picker and ViewModel lifecycle.
**Stack:** Existing Go bridge and Kotlin Jetpack Compose; no new dependencies.
**Spec:** ../specs/2026-11-14-mobile-references-design.md

## Constraints and review focus
- Reject forged paths, disabled references and unsupported kinds before turn dispatch.
- Ignore stale catalogue replies after switching workspace or disconnecting.
- Preserve cursor/suffix, avoid email triggers, handle duplicate names and deleting markers.
- Keep session drafts independent and preserve bindings through saved UI state.
- Never expose raw discovery errors or automatically modify host configuration.

## Task 1: Host discovery and invocation
- [x] Write failing projection, validation and structured turn-input tests in internal/bridge/references_test.go.
- [x] Add references/list to protocol and implement internal/bridge/references.go; integrate with server.go.
- [x] Verify Go race/vet and a read-only real Codex catalogue.
- [x] Commit the coherent host contract, tests and design documentation.

## Task 2: Android composer and lifecycle
- [x] Add failing pure tests for catalogue, token boundaries, replacement/removal and scoped request state.
- [x] Implement PromptReference.kt and ReferencePicker.kt, integrate ChatWorkspace.kt, SessionSettings.kt, ModexViewModel.kt and MainActivity.kt.
- [x] Verify unit tests, debug/test APK builds and lint; inspect on an available emulator without wiping saved state.
- [x] Review final diff, document contract/limits and commit Android implementation.

## Verification

Host commit `5943fc0`: Go race/vet PASS. Real-provider opt-in test
`MODEX_LIVE_SKILLS_CODEX=codex go test ./cmd/modex-host -run '^TestLiveMobileSkillReference$' -count=1 -v`
PASS: workspace catalogue, typed native skill input, completed response MODEX_SKILL_OK.
Final Android implementation: 86 unit tests, assembleDebug, assembleDebugAndroidTest
and lintDebug PASS with JDK21. Emulator picker regression PASS for discovery,
disabled state, exact binding, independent session drafts, deletion, email guard
and send. Picker/selected screenshots inspected. A final chip-color-only adjustment
was build/unit/lint verified; emulator was already stopped. Python discovery with
private host/client binaries: 31 PASS, no skips.

No physical-phone, large-font, landscape or screen-reader runtime validation.
No independent reviewer was available; primary reviewed the source/contracts.
MCP/subagent/plugin execution beyond installed plugin skills remains deferred.
No live host restart/binary replacement, push, PR, merge or release.
Owned emulator was stopped and adb devices confirmed empty.
