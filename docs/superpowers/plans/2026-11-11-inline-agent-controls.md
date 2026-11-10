# Inline Agent Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add compact command activity, real per-turn model selection, and inline approvals/questions to the Android chat.

**Architecture:** Extend the existing allowlisted bridge `turn/start` contract with one optional model string. Keep UI state in the ViewModel/Compose layer and reuse the existing decision protocol without changing security semantics.

**Tech Stack:** Go bridge, Kotlin, Jetpack Compose, org.json, existing Android instrumentation.

**Spec:** `docs/superpowers/specs/2026-11-11-inline-agent-controls-design.md`

## Global Constraints

- No new dependency or provider/config mutation.
- Preserve 48dp touch targets and selectable command details.
- Never expose raw upstream errors or add unsupported decision responses.

## Review Focus

- Blank, whitespace, multiline, null and oversized model values must not reach Codex.
- Omitting a model must preserve the current session model.
- Decisions for another thread must not appear in the selected transcript.
- Multiple required questions must keep answers until one explicit submit.
- File changes must remain unapprovable until a diff viewer exists.

---

### Task 1: Optional per-turn model contract

**Files:**
- Modify: `internal/bridge/server.go`
- Test: `internal/bridge/server_test.go`
- Modify: `android/app/src/main/java/dev/modex/SessionSettings.kt`
- Modify: `android/app/src/main/java/dev/modex/ModexViewModel.kt`
- Test: `android/app/src/test/java/dev/modex/SessionSettingsTest.kt`

- [ ] Write Go and Kotlin tests for optional valid model and invalid values.
- [ ] Run focused tests and verify RED.
- [ ] Forward a validated optional model on `turn/start`; serialize it from Android and track effective model from create/resume.
- [ ] Run focused tests and verify GREEN.

### Task 2: Compact command activity

**Files:**
- Modify: `android/app/src/main/java/dev/modex/CommandCard.kt`
- Test: `android/app/src/androidTest/java/dev/modex/ChatUiSmoke.kt`

- [ ] Tighten the instrumentation expectations for compact terminal activity and hidden output.
- [ ] Verify the old UI fails the new visual/semantic fixture.
- [ ] Replace the card with a muted disclosure row while preserving expansion, status and selectable details.
- [ ] Run the command timeline fixture.

### Task 3: Composer model chip and inline decisions

**Files:**
- Modify: `android/app/src/main/java/dev/modex/ChatWorkspace.kt`
- Modify: `android/app/src/main/java/dev/modex/MainActivity.kt`
- Create: `android/app/src/main/java/dev/modex/InlineDecisionCard.kt`
- Test: `android/app/src/androidTest/java/dev/modex/ChatUiSmoke.kt`
- Test: `android/app/src/androidTest/java/dev/modex/ApprovalSmoke.kt`

- [ ] Extend fixtures to require the model chip, inline approval actions and inline question inputs; verify RED.
- [ ] Wire queued model selection into send and render only selected-thread decisions inline.
- [ ] Move current decision form/actions from the modal into `InlineDecisionCard` with unchanged response JSON.
- [ ] Run UI fixtures, full Android checks, Go suite/vet, inspect screenshots and update `PROJECT_STATE.md`.
