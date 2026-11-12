# JSON preview, attachment reconnect and session model persistence

Goal: JSON formatting/highlighting in Catppuccin Mocha; restore clickable attachment history after fresh connection; persist explicit per-thread model and effort rather than silently inheriting defaults.

Implement sequentially on stacked feature branches with one coherent commit per item; no push/merge. Keep existing native Compose/Go and encrypted snapshot. No new dependencies, PDF renderer or unrelated restyling.

- [x] JSON: strict bounded formatting of complete valid JSON, preserve string/number values; syntax color keys Blue, strings Green, numbers Peach, booleans Mauve, null Overlay, punctuation Subtext on Base. Invalid/truncated JSON stays readable raw. Unit tests then emulator viewer check.
- [x] Reconnect: inspect actual stored/app-server content boundaries, regression from those shapes, preserve prompt and path metadata without swallowing arbitrary trailing text; verify with separate fresh host process.
- [x] Model/effort: move per-thread choice from transient composable into encrypted VM snapshot, persist at selection (before sending), retain across disconnect/app recreation, isolate threads. Resolve new-thread defaults from host config/model catalog, show actual effort, remove Keep current. Extend safe host status/thread read settings as needed; no credential/config dump. Unknown effort explicitly shown rather than guessed. Unit + emulator state/model controls + real provider tests.
- [x] Final review, Go/Android relevant tests, stop emulator, update PROJECT_STATE. Keep independent commits and report actual validation limitations.
