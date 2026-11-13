# Chat interactions implementation plan

Goal: show Codex reasoning summaries, smoothly reveal live replies, show per-prompt wall time, stop an active turn, and provide long-press message actions.

Accepted interaction contract: long press opens Copy / Select text, plus Edit for user text. Select text opens a full-screen selectable text snapshot. Edit loads text into the composer and sends only on explicit Send as a new turn; no rollback, fork or automatic resend. Preserve existing drafts with a replacement confirmation. Keep Catppuccin Mocha and current attachment behavior.

Architecture: preserve canonical streamed text in the reducer; animation is display-only and skips historical messages. Display only reasoning summaries exposed by Codex. Stop forwards exact thread/turn IDs to the owning worker, awaits turn/completed, never reacquires another writer or clears busy on interrupt acknowledgement. Host history projects turn IDs/durations and reasoning summaries, never raw reasoning content. Timers include waiting/approval time, not CPU time; unavailable historic timing stays unknown.

Execution: primary builder implements inline using ponytail, TDD and executing-plans. PROJECT_STATE.md is the sole progress tracker per project instructions; no competing ledger. Branch feat/chat-interactions starts at main 0480c4b; release design branch is preserved separately. No publishing, production supervisor restart or phone installation in this task.

- [x] Host protocol: failing interruption/history tests; add allowlisted turn/interrupt, owned-worker routing and safe history metadata; Go tests/race/vet; coherent commit.
- [x] Android state: failing summary/timer/stop/reveal tests; add summary reducers, per-turn lifecycle and snapshot compatibility; targeted JVM tests.
- [ ] Android UI: long-press actions and full-screen selection, composer edit/focus and draft replacement, live-only reveal, compact thinking/timer, Send/Stop; unit/build/lint and disposable UI fixture; coherent commit.
- [ ] Review cross-thread stop races, delayed acknowledgements, Unicode, historical replay, selection gestures and preserved drafts. Verify complete branch; update state. If starting an emulator, stop it after testing.

Checks: Go ./... test/race/vet; Android testDebugUnitTest, assembleDebug, assembleDebugAndroidTest and lintDebug under JDK 21. No test that assumes a model always emits reasoning summaries. Self-review if no independent reviewer tool is available.

Protocol reference: https://learn.chatgpt.com/docs/app-server ; installed schema confirms TurnInterruptParams requires threadId/turnId, Turn optionally provides startedAt/completedAt in seconds and durationMs.

Verification milestone: Go race/vet PASS; Android 69 JVM tests, debug APK, instrumentation APK compilation and lint PASS. UI implemented and source-reviewed; no device/emulator gesture execution or concurrent process interrupt fixture was run. No publishing/deployment.
