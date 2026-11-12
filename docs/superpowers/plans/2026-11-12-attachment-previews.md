# Attachment previews and full-screen remote picker

Goal: tappable image/file previews in composer and sent/history messages; full-screen remote selection starting at SSH home and remembering the last folder per thread.

Use the existing Mocha palette, quiet file tiles and 112dp image thumbnails. Images open in a full-screen zoomable viewer. UTF-8 text previews are bounded; other files use Android ACTION_VIEW through a narrowly scoped FileProvider. Reuse pinned SSH/SFTP for bounded private cache downloads (20 MiB/file, ~100 MiB cache), no external image URLs and no new libraries. Keep remote attachment paths in encrypted history metadata, never bytes in the journal. The remote picker occupies the entire app with fixed header/footer and a scrollable listing; remember only successful directory navigation per host/thread in the encrypted connection snapshot, defaulting to SFTP home.

- [x] Host: regression for image/file metadata on history reload; parse only validated Modex file-reference content; preserve existing text-only clients.
- [x] Android: JSON round-trip tests, bounded preview download/cache, image/text/full-screen viewer, read-only FileProvider and clickable composer/message tiles.
- [x] Remote picker: full screen, home default and per-thread last-directory persistence; integration tests for download and folder isolation.
- [x] Verify Go tests/vet, Android build/unit/lint, sequential emulator preview/picker/SFTP fixtures; stop emulator. Update PROJECT_STATE and coherent commits; no push/merge.

Limits: OS viewer support varies for binary formats; files must remain readable on the remote host for uncached previews. Cache is app-private and bounded, not encrypted separately. No PDF renderer dependency, HEIC conversion, external web downloads or remote file editing.
