# Paged history and reconnect ownership

**Goal:** Cache history per session, fetch recent turns first and older pages on demand, and never treat restored history as permission to send.

**Architecture:** Use Codex `thread/turns/list` with full items, descending pages reversed for display. Keep legacy unpaged reads compatible. Cache bounded session snapshots in the existing encrypted host store. On disconnect/process restore show the empty new-chat landing; remember the last thread only as a target for ownership validation on reconnect. Successful resume can reopen cached content while refreshing the newest page; active_writer leaves the chat empty with explicit takeover. No automatic takeover or prompt replay.

**Contracts:** Paged `threads/read` has include_turns=true, limit (1..50, Android 10), optional cursor. Response includes chronological messages, next_cursor and history_revision for the newest page. A newest-page fingerprint determines whether older cached pages remain valid; changed newest page invalidates older cached pages conservatively. Older pages prepend by stable entry identity. Cursor errors remain explicit; never silently fall back to downloading the entire history. The upstream metadata read validates cwd before accessing pages. Cache is bounded (10 sessions, 4 MiB serialized budget) and never grants ownership.

- [x] Host: native pagination, workspace validation, worker routing, cursor validation and regression tests; commit.
- [x] Android: bounded encrypted per-thread cache and page merge; latest/older loading UI with stable scroll anchor; tests; commit.
- [x] Connection lifecycle: clear visible history/ownership on manual and involuntary disconnect and process restore; reject stale resume/history replies; revalidate last thread on reconnect without auto takeover; tests; commit.
- [x] Verify Go race/vet, Android unit/build/lint, real read-only paged history on configured host; update PROJECT_STATE.md. Never take over the CLI running this implementation or reset emulator data.

Completed on feat/paged-history-cache: host commit 3ead74a; Android cache and lifecycle ship together because cached display must depend on fresh ownership validation. Verification and remaining device limitations are in PROJECT_STATE.md.
