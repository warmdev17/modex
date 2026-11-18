# Android long-press and popup menu polish

**Goal:** Implement the owner's explicit screenshots/requirements: long-press chat actions, visible Mocha popup surfaces and one consistent action icon family throughout the Android app.
**Architecture:** Reuse the existing Compose menus, ChatGlyph and Mocha palette. One small shared popup/menu-item primitive is permitted because at least seven existing menus must share the same appearance; no new dependencies or backend changes.
**Spec:** Current user request and PROJECT_STATE.md scope/acceptance. This is a refinement, not a new visual world; no extra design approval.

## Task1 — Frontend (coder-fe)

Owned: ChatWorkspace.kt, MessageActions.kt, ConnectionHome.kt, ModelPicker.kt, ModexTheme.kt; optionally one shared popup primitive and affected Android tests. No ViewModel/Go/build/release/state edits.
- [x] Regression-first: actual long hold opens sidebar actions without selecting; normal tap still selects. Archived rows also expose long-press actions. Existing ellipsis-based smoke fixtures are updated, not skipped.
- [x] Replace row ellipsis with combined-clickable/long-click semantics and preserve disabled/running/action/confirmation rules. Retain global toolbar menu and host management entry points.
- [x] Use one consistent clearly outlined/raised opaque Mocha surface for all DropdownMenu callsites. Text >=4.5:1, icons>=3:1 for enabled states. Preserve color semantics, restrained corners and 48dp touch rows. Upload labels must wrap safely.
- [x] Add appropriate leading icons everywhere: Archive, Restore, Fork, Delete; Edit/Remove host; session/connection settings, All hosts, Disconnect; message Edit/Copy/Select text; project choices; model choices/custom ID; upload phone/computer/skills. Extend only existing ChatGlyph family (same24-unit grid/1.8 stroke), consistent same-action glyph mapping, red destructive action text and icon.
- [x] JVM tests/build/lint + instrumented real-touch fixture and screenshots proving sidebar long-press, normal tap, archived restore/delete, upload popup and other popup icon coverage. No user history mutations or new actions from reference screenshot.

## Task2 — Review and delivery (primary, reviewer/tester as useful)

- [x] Review actual diff and rendered menus; repair material findings and reverify.
- [ ] Produce next unused dev.11/code12 APK with established certificate, exact-source tag, checksums, public pre-release and anonymous asset verification. Test direct in-place dev.10/code11→new code12 with encrypted synthetic state. Never uninstall or clear app data.
- [ ] Android-only: no host rebuild/restart. Verify prior dev10 host activation; keep active turns alive.
- [ ] Record tested revision, branch/commits, actual checks/device gaps and remaining dirty state in PROJECT_STATE.md.
- [ ] Continue already authorized PR/merge (merge commits preserving prior stack) then Android-useful slash commands. This new UI request takes priority, not cancellation of those tasks.

Preflight: Task1 owns UI/tests; Task2 owns release/Git/state and consumes frozen source. No file ownership overlap. No host API/interface changes. Existing root and sibling worktree state edits remain outside commits.
