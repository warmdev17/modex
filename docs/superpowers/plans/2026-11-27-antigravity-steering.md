# Antigravity follow-ups

**Goal:** Send text to the running Antigravity conversation on Android with the existing Send and Stop controls.

**Runtime evidence:** On AGY 1.5.17, a second `session/prompt` sent after the first assistant chunk ended separately from the original RPC and produced the follow-up marker. Native notifications were scoped to the session. No early reply or user-message echo confirmed acceptance.

## Contract

- Advertise `antigravity_steer` on a Codex supervisor. Android enables active-turn Send for Antigravity only with this key and a known active turn ID. Keep standalone Antigravity prompts and other runtimes unchanged.
- Forward text with `turn/steer`, `thread_id`, exact `turn_id`, `workspace`, and `text`. Reject stale IDs, blank text, files, references, model and reasoning before dispatch. Keep drafts with files available for the next idle turn.
- Use a second `session/prompt` for the same AGY session. Require the expected active local turn and a tested compatible CLI, with no resend or explicit fallback. Treat the result of writing the native request as a delivery acknowledgement, consistent with AGY turn/start. Announce the submitted user message once and return the current Modex `turnId`.
- Keep one active Modex turn across both native responses. Accept at most one in-flight follow-up per session, preserve streamed segments and their order, and wait for both responses before completing. Stop broadcasts the existing session cancel and remains scoped to the active turn. A provider that has already finished when it receives a follow-up can process it as a new native prompt, since ACP exposes no expected native turn field.

## Implementation and verification

- [x] Host owns ACP active-prompt accounting, exact-turn checks, router journal and hello. Tests cover both native reply orders, rejected input, an in-flight follow-up and Stop.
- [x] Android owns capability, composer and acknowledgement guards. Tests cover older hosts, text-only steering, file draft retention and other runtimes.
- [x] Isolated provider and supervisor smoke, Go race/vet, Android unit/lint and emulator upgrade passed. Signed preview 51 is in the main checkout's `.modex/previews/`. Host activation uses the established delayed restart.
