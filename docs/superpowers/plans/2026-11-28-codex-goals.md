# Codex Goals Implementation Plan

> **For agentic workers:** Use subagent-driven-development for scoped tasks.

**Goal:** Manage native Codex goals from Android and verify runtime boundaries.
**Architecture:** Host validates and maps commands to Codex native API;
Android consumes acknowledged state and live goal events.
**Tech Stack:** Go, Kotlin, Jetpack Compose, existing SSH journal.
**Spec:** docs/superpowers/specs/2026-11-28-codex-goals-design.md

## Global Constraints

- No new dependencies or client-side goal scheduler.
- Only Codex advertises `goals`; unsupported runtimes fail explicitly.
- Commands `threads/goal/get`, `threads/goal/set`, `threads/goal/clear` use
  `thread_id`, optional `workspace`; set accepts `objective`, `status`
  (`active`/`paused`), optional positive `token_budget`.
- Responses `{goal: null|{thread_id,objective,status,token_budget,tokens_used,
  time_used_seconds,created_at,updated_at}}`; clear responds `{cleared: bool}`.
- Events `thread/goal/updated` and `thread/goal/cleared` retain native Codex
  camelCase payloads, matching existing event conventions.
- Mutations use native origin `user`, never replay automatically.

## Review Focus

Wrong workspace/runtime, reconnect without mutation, stale response after
chat switch, external CLI updates/clear, malformed budgets/status.

### Task 1: Host goal protocol

Owner backend. Files internal/bridge goal command/projection/event tests,
cmd/modex-host routing/capabilities and native lifecycle test as needed.
Produces the Global Constraints contract for Android.

- [ ] Add failing tests for get/set/clear mapping, `origin:user`, validation,
  wrong workspace, unknown keys/status/budget and unsupported agent.
- [ ] Implement goal projection and workspace-scoped command mapping.
- [ ] Relay native update/clear events and advertise Codex-only `goals`.
- [ ] Run focused tests, full Go race suite/vet and commit explicit paths.

### Task 2: Android goal management

Owner frontend, all Android production and test files. Consumes Task 1
contract. Use existing capability model, request tracking, session switching
and chat menu/dialog patterns. Add a focused Goal model/UI file as needed.

- [ ] Add failing tests for goal parsing, budget validation, capability gating
  and lifecycle updates including clear, stale responses and reconnect.
- [ ] Add Goal menu/dialog with create/objective/budget form, usage/status,
  Pause/Resume and clear. Disable action while pending/disconnected, refresh
  after uncertain failure without retrying mutation. Explain Pause vs Stop.
- [ ] Wire ViewModel state/actions/events and MainActivity callbacks.
- [ ] Run Android unit tests, lint and build including affected UI smoke.

### Task 3: Integration and compatibility

Owner primary. Read actual diff, independent reviewer of whole feature.

- [ ] Exercise real installed Codex goal get/set paused/clear in isolated
  supervisor without provider inference, test shared-runtime rejection.
- [ ] Inspect AGY/OpenCode installed API support and document actual limits.
- [ ] Verify APK identity/signature/upgrade and stage project preview if built.
- [ ] Update PROJECT_STATE with commands/results and any delivery gaps.
