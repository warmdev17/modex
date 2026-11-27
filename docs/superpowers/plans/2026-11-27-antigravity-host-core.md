# Antigravity CLI Host Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Connect Modex Host to Antigravity CLI over ACP and verify the text-session lifecycle.

**Architecture:** Reuse the host's bounded stdio RPC transport with an explicit ACP envelope. An ACP client scopes session operations to a single workspace. The host exposes a local probe and keeps the production chat path separate.

**Tech Stack:** Go, ACP v1 over NDJSON, Antigravity CLI 1.5.17.

**Spec:** [Host core design](../specs/2026-11-27-antigravity-host-core-design.md).

## Constraints

- Keep Codex framing and behavior compatible. Use the installed CLI and its existing credentials.
- Report model authentication only from an explicit ACP result. Handshake success alone certifies protocol compatibility.
- Forward decimal JSON-RPC error codes internally and keep raw provider errors out of public output.
- Fail closed for tool permission and unavailable or stale session/turn operations. Never replay prompts.

## Tasks

- [x] Add a fixture ACP executable and tests for handshake, workspace-filtered list/load, prompt deltas, cancellation, permission rejection, and authentication errors. Observe missing APIs failing compilation.
- [x] Implement ACP framing in `internal/bridge/process.go`, safe error codes in `internal/diagnostic/rpc.go`, and native operations in `internal/acp/client.go`. Run the ACP and existing Codex transport suites.
- [x] Add `modex-host --probe-antigravity` in `cmd/modex-host`. Use the same workspace validation as the existing host modes. Report detected protocol and capabilities without starting a turn.
- [x] Run Go race and vet, build the candidate, run isolated supervisor checks, and verify installed `agy` through the new probe. Record findings in `PROJECT_STATE.md`.
