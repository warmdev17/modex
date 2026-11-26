# OpenCode Stage 1 core implementation plan

**Spec:** `docs/superpowers/specs/2026-11-26-opencode-integration-design.md`

**Goal:** Make the existing OpenCode backend safe and compatible with the
current Android core chat flow before adding parity or utilities.

## Global Constraints

- Preserve Codex as the default backend and preserve all existing protocol
  responses outside OpenCode-specific paths.
- Follow TDD for every behavior change: record the focused failing command and
  expected failure before production edits, then the focused passing command
  and full `go test ./...` result.
- Do not add dependencies or a new queue/state machine.
- Do not edit Android in this stage; its existing `standalone`, Stop and
  `turn/steer` requests are the client contract.
- Every commit is scoped to one task and includes its tests.

## Task 1: Remove the OpenCode decision lock inversion

**Owner:** backend implementer

**Files:** `internal/opencode/decisions.go`,
`internal/opencode/decisions_test.go`, and only directly required event tests.

Move synthesizer turn-id lookup outside `Decisions.mu` for both SSE admission
and permission reconciliation, then pass the captured id into the locked
helper. Add a deterministic regression test that would block under the old
`Decisions.mu -> Synthesizer.mu` edge while completion resolves decisions in
the opposite direction. Preserve decision ordering, caps and emitted payloads.

**Acceptance:** focused decision tests demonstrate RED then GREEN; the race
detector for `./internal/opencode` completes without a deadlock.

## Task 2: Support standalone OpenCode chats

**Owner:** backend implementer

**Files:** `internal/opencode/server.go`, `internal/opencode/server_test.go`.

Accept `threads/start` with `standalone: true`, resolve
`$HOME/Documents/Codex`, create it privately, and pass that absolute path to
session creation. Reject standalone combined with a nonempty workspace or a
project id. Keep ordinary workspace creation unchanged.

**Acceptance:** tests cover directory creation, request projection and invalid
parameter combinations; focused server tests demonstrate RED then GREEN.

## Task 3: Make Stop stale-safe and add native live follow-up

**Owner:** backend implementer

**Files:** `internal/opencode/server.go`, `internal/opencode/client.go`,
`internal/opencode/events.go`, `cmd/modex-host/opencode_serve.go`, their focused
tests, and no Android files.

Add one active-turn snapshot method on the synthesizer. Carry the expected
turn id through `turn/interrupt` and reject stale or completed targets before
the OpenCode interrupt HTTP call. Add `turn/steer` validation by reusing the
existing prompt input parsing, carry its expected turn id through the call
seam, and submit `/api/session/{id}/prompt` with `delivery: "steer"`. Return
the expected active Modex turn id and do not call `StartTurn` for a steer.

Tests must cover exact-turn Stop, stale Stop with no HTTP request, steer request
body and response, stale steer with no HTTP request, and synthesizer behavior
showing a steer does not replace the active turn.

**Acceptance:** focused client/server/bridge tests demonstrate RED then GREEN;
`go test -race ./...` and `go vet ./...` pass.

## Task 4: Verify and document the Stage 1 boundary

**Owner:** primary agent for `PROJECT_STATE.md`; read-only tester/reviewer for
verification.

Run the full Go race suite and vet, build `modex-host`, and exercise an isolated
OpenCode 2.4.1 supervisor flow covering standalone creation, initial prompt,
native steer and exact-turn interrupt without using an existing user session.
Update OpenCode documentation and `PROJECT_STATE.md` with verified behavior,
remaining stages and explicit test gaps. Do not publish, merge or restart the
live host in this task.

**Acceptance:** independent whole-branch review has no open critical/important
finding; state and documentation match the verified source revision.
