# Remote project sidebar

User direction: discover all remote Codex sessions and show project folders and recent sessions; selected row Blue background/Base text.

Design: keep existing workspace-scoped API defaults. Add explicit `all_workspaces:true` to thread listing, omitting cwd and provider/source filters. For cross-project reads/resume/turn/start and new threads, accept optional absolute `workspace` and verify Codex-returned cwd exactly matches it. SSH user/CODEX_HOME determines available sessions. Projects derive from full cwd, not filesystem scanning or desktop-private pins. Archived threads remain excluded by Codex default. Phone stores selected workspace with encrypted connection snapshot and passes it on read/resume/send/create. Follow all list cursor pages automatically, with repeated-cursor detection and visible error. Sidebar groups full-path-distinct folders, can collapse, searches session titles and paths, plus Recents; blue/base selected state applies in both locations.

- [x] Bridge optional workspace validation, all-workspace listing, tests retaining default confinement and verifying explicit cross-project reads/resume/send.
- [x] Android thread path metadata, automatic pagination, encrypted selected cwd, folder sidebar and selection colors, unit/fixture coverage.
- [x] Go suite/vet and Android build/tests/lint; read-only real catalog check; emulator checks if available. Update state and commit milestones separately. No host restart or changes to existing sessions.

Verified host: full Go suite/vet, bridge race, real 56 sessions/16 paths, real two-page pagination and cross-project metadata read. Reviewer agent disconnected; self-review fallback. Android build/40 JVM tests/lint pass. Emulator sidebar acceptance PASS; screenshot visually inspected for folder icons and Blue/Base selection.
