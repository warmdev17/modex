# OpenCode integration design

## Goal

Make OpenCode a first-class Modex backend in ordered stages, starting with a
safe core that the existing Android client can enter and control. Add parity
and convenience only after the core lifecycle is trustworthy.

## Stages

1. **Core correctness and entrypoints**
   - Remove the decision/event lock inversion.
   - Let Android create a standalone chat in `~/Documents/Codex`, matching the
     existing Modex contract.
   - Reject stale Stop requests by exact active turn id.
   - Map `turn/steer` to OpenCode prompt delivery `steer`; do not invent a
     second queue or start a second Modex turn.
2. **Conversation parity**
   - Expose backend capabilities to Android and hide or disable unsupported
     controls.
   - Complete model, attachment, permission and form behavior using OpenCode's
     native contracts.
3. **Continuity and navigation**
   - Persist OpenCode state across host restarts.
   - Add supported session lifecycle operations such as archive, delete and
     fork, then verify history/resume behavior.
4. **Utilities**
   - Add projects, references, plan mode and agent-oriented conveniences only
     where OpenCode exposes a stable contract.
   - Keep cross-agent orchestration separate from basic single-agent chat.

Each stage must be independently usable and verified before the next stage
widens the surface.

## Stage 1 contracts

### Locking

`Decisions` must never invoke the synthesizer turn lookup while holding the
decision mutex. The existing synthesizer completion callback may resolve
decisions while holding the synthesizer mutex; eliminating the reverse edge
removes the deadlock cycle without adding a new synchronization layer.

### Standalone chats

`threads/start` with `standalone: true` creates the OpenCode session in
`$HOME/Documents/Codex`, creating that directory with private permissions when
needed. A standalone request cannot also select a workspace or project.

### Stop

`turn/interrupt` carries the requested `turn_id` through the server/client seam.
Immediately before calling OpenCode interrupt, the bridge compares it with the
currently active synthesized turn. Missing, completed or different turns are
rejected without sending an interrupt request.

### Live follow-up

`turn/steer` uses the same validated text, attachments and model fields as
`turn/start`, plus the exact expected `turn_id`. The bridge verifies that turn
is still active, submits OpenCode's native prompt with `delivery: "steer"`, and
returns the original active turn id. It must not call `StartTurn`, because a
steer belongs to the current Modex turn.

If admission is uncertain, Modex reports an error and retains the Android
draft; it does not retry automatically.

## Non-goals for Stage 1

- Android visual changes or a new backend picker.
- A Modex-owned follow-up queue.
- Persistent OpenCode state across host restarts.
- Projects, references, plan mode, archive/delete/fork or cross-agent work.
