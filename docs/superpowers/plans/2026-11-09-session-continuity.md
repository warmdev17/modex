# Android session continuity

Authorized by user: clean black native chat UI, stored session history after reconnect, durable connection configuration and usable multitasking. No provider or approval policy changes; never replay a prompt.

1. Host: extend threads/read with include_turns=true, validate workspace before exposing text-only user/assistant history and active status. Existing callers unchanged. Unit tests for boundaries/extraction.
2. Android: encrypted single-host connection/key/session snapshot with Android Keystore AES-GCM in noBackupFilesDir; explicit Forget saved host removes key/ciphertext. Restore config, selected thread and journal with automatic attach when app opens; no prompt retries. Snapshot cursor only with applied state. Surface recovery when supervisor changes.
3. Foreground data-sync service holds the app/session while connected, with persistent notification and disconnect action. App-owned ViewModel preserves state across activity close/reopen. Android OS may still kill/limit service; encrypted restoration recovers observing state.
4. History: load stored messages on session selection and reconnect without clearing existing transcript until successful response. Keep workspace filter and pagination. Label list empty/errors honestly.
5. UI: black surfaces, white text, neutral gray user bubbles; unboxed assistant text, compact header and composer. Preserve selectable text and approval review.
6. Verification: Go boundary tests, Android parser/snapshot tests, build/lint; emulator encrypted-store round-trip and UI light-independent dark rendering. Test local host history with real saved thread read-only. Separate focused commits for host and Android; no push.
