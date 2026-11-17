# Android session management implementation plan

> **For agentic workers:** Use subagent-driven-development, no recursive delegation. Primary owns integration, PROJECT_STATE.md and release.

**Goal:** Deliver the approved connection/sidebar/notice/interruption batch and then honor the owner's added PR/merge and Android slash-command work.
**Architecture:** Extend the existing strict Go bridge and existing per-host Kotlin state/Compose components. Canonical Codex APIs own conversation storage and delete dependency validation. No automatic mutation retry.
**Tech Stack:** Go, Kotlin/Compose, SSH, installed Codex 0.169 native RPC, Gradle 8.14.3/JDK21.
**Spec:** ../specs/2026-11-17-android-session-management-design.md (owner approved).

## Global constraints

- Preserve Mocha/blue and compact 48–56dp headers; no new dependency.
- Preserve existing encrypted host/key/history, user conversations and active backend while testing.
- Strictly validate workspace/IDs before mutations. Delete confirm:true is mandatory; canonical native delete only.
- Fork never starts a goal/turn automatically: excludeTurns:true,deferGoalContinuation:true. Verify returned workspace, provider and effective permissions.
- Fence responses by identity/list mode/connection; uncertain actions require inspection, never automatic replay.
- No raw credential-bearing errors/URLs in Android. Preserve native TUI protocol and original Codex event forwarding.
- Keep root checkout's historical state uncommitted and independent; primary alone writes this worktree's PROJECT_STATE.md as the sole ledger.
- Latest owner authorization includes development release publication, host activation, then PR/merge (merge commit preserving original commits), then a separate Android-useful slash-command feature. No squash/rebase.

## Shared API contracts (binding)

Existing Modex envelope {id,method,params} and safe errors remain.
- threads/list: existing + archived:boolean=false; result {data,next_cursor} unchanged. archived true lists archived only.
- threads/archive: {thread_id,workspace?} -> {}. Native thread/archive {threadId}.
- threads/unarchive: {thread_id,workspace?} -> {thread: existing Thread projection}. Native thread/unarchive.
- threads/delete: {thread_id,workspace?,confirm:true} -> {}. Native thread/delete; dependency refusal becomes category text thread_has_dependents, with no deletion fallback.
- threads/fork: {thread_id,workspace?} -> existing projected session response (thread/model/permissions). Native ordinary thread/fork with deferGoalContinuation:true and excludeTurns:true. Open returned thread only after successful validation, without inference.
- New notice object: {category:string,message:string,retrying:boolean,http_status?:number}. Only allowlisted fixed messages; no raw details.
- Mobile error events add params.notice; mobile turn/completed adds params.turn.notice when error exists. Existing raw error payloads must be sanitized for mobile, while native TUI forwarding remains unchanged.
- History user/turn message projection adds optional turn_notice using the same notice shape. Existing clients ignore it. Metadata read retains authoritative thread status/active turn as needed for non-resuming reconciliation.
- Native lifecycle/status events retain their documented native names/identity fields for Android reconciliation.

## Review focus

1. Late list/action replies after a mode/host/connection switch never mutate the wrong view (FE regression).
2. Missing delete confirmation or workspace mismatch rejects before mutation, native dependency refusal keeps data (BE unit/native fixture + FE UI).
3. Fork copies provider/permissions and defers inherited goals (BE native fixture).
4. Retry/terminal error privacy and old-turn events cannot stop or overwrite newer work (BE projection + FE reducer tests).
5. Disconnect/foreground/process restore preserve credentials but no implicit selection; other hosts remain online (FE instrumentation).

## Task 1 — Backend commands and safe turn notices (coder-be)

Owned: internal/bridge/{protocol.go,server.go,turn_metadata.go,new focused lifecycle/notice helpers and tests}, internal/diagnostic/{rpc.go,new notice helper and tests}, cmd/modex-host/shared_bridge_test.go for interruption coverage. No Kotlin/PROJECT_STATE edits.
- [ ] Add failing command tests for exact mappings, native refusals, missing confirmation, unknown fields, bad workspace, returned fork provider/permissions and deferred goal.
- [ ] Implement the shared contracts above using existing projection/validation patterns; add safe dependency/missing-source categories.
- [ ] Add failing live-event/history notice projection/privacy tests, then implement the normalized notice shape without altering native TUI errors/events.
- [ ] Add shared-backend synthetic native interruption→mobile events regression.
- [ ] Run umask 022; GOCACHE=/tmp/modex-go-cache go test -race ./... and go vet ./...; report real versus fake evidence. Primary commits explicit paths after review.

## Task 2 — Android navigation, lifecycle menus and synchronized notices (coder-fe, independent Kotlin ownership)

Owned: android/app/src/main/java/dev/modex and relevant android test/androidTest files only. No Go/build configuration/PROJECT_STATE edits. Primary handles versioning/build/release.
- [ ] Add failing JVM/state tests for no auto-resume, notices/retry/terminal/stale completion/status reconciliation, list response fencing and lifecycle decisions.
- [ ] Remove automatic lastThread selection at connection. Navigate chat disconnect/loss back to Hosts while preserving profiles and other remotes. Explicit Activity/session selection still works.
- [ ] Add accessible session overflow actions and Archived/Restore view with correctly scoped pagination/search/action progress. Delete confirmation and dependency refusal retain original entries/caches. Fork opens only successful validated result; no prompt sent.
- [ ] Consume normalized notice fields, preserve in encrypted ChatEntry snapshots and show clear inline turn/retry/outcome text. Old-host known-error fallback remains safe.
- [ ] Consume native status/closed notifications by authoritative bounded history reconciliation; keep newer-turn guards. Never infer interrupted from idle alone or resume for observation. Reconcile selected active sessions/foreground as supported; report independent-backend limits.
- [ ] Update continuity/reconnect fixtures to explicitly select after connecting. Add real-touch UI fixture for connection return/blank selection/sidebar menus/turn notices.
- [ ] Run affected/full JVM tests, assembleDebug/AndroidTest/lint using existing Gradle8.14.3/JDK21/caches; report screenshots/test selectors. Primary commits explicit paths after review.

## Task 3 — Review, integration, delivery and host activation (primary + reviewer/tester)

- [ ] Read-only reviewer checks each BE/FE deliverable against this plan, including scopes/contract/privacy/races; owners fix findings before final integration.
- [ ] Primary updates docs/bridge-plan.md and docs/mobile-development.md from implementation, and the sole PROJECT_STATE ledger. Inspect combined diff and run Go/Python/Android checks appropriate to actual changes.
- [ ] Test native archive/restore/delete/fork and native interruption through isolated supervisor with synthetic sessions. No user thread mutations. Test external-process observation independently and report actual coverage.
- [ ] Produce dev.10 or next unused release with versionCode>10, same certificate; download exact dev.9 APK, seed encrypted synthetic state, upgrade in-place and run affected UI checks on the distributable. Inspect screenshots; stop owned emulator only.
- [ ] Publish source tag, APK, code, host archives, checksums, verification notes and downloadable GitHub pre-release. Verify remote refs/assets and install exact tested binaries with rollback backup.
- [ ] Per latest user sequencing, activate one delayed external host restart after phase-one delivery, verify it independently where the current backend is not owned by that service, then create PR/merge using merge commit. Never interrupt this backend or discard pending work. If a turn boundary is necessary, preserve exact next steps in PROJECT_STATE.

## Task 4 — Android slash commands (subsequent feature)

- [ ] After phase-one delivery/activation and PR/merge, inspect current native Codex slash-command behavior/API and existing Android controls. Produce a concrete include/exclude table; omit /keymap,/vim and terminal-only commands.
- [ ] Implement the smallest Android command picker triggered by /, mapping only supported actions to real Android/native operations. Never submit a decorative/no-op command as a prompt. Destructive commands retain explicit confirmation.
- [ ] Regression tests, UI verification, new versioned APK delivery and separate PR/merge for this feature; update state and final evidence. No repeated design approval: owner explicitly authorized continuing and primary settling routine details.
