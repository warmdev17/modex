# CLI session picker

User-approved scope: `modex cli` lists native sessions by title in the current workspace, supports pagination, cancellation and a new conversation, and registers the real Codex TUI for explicit phone takeover without copying IDs. Existing `--cli --thread` and SSH client remain compatible. No Android changes or new dependencies.

1. Test then implement the host picker using the existing bridge list projection: names before previews, numbered selections, next/previous pages, new conversation and safe cancellation. Fail closed on list errors and invalid IDs; strip terminal controls from display labels.
2. Test then extend the Unix launcher: a new native TUI creates its own session; discover its single loaded ID and expose the same private owner socket. Preserve ownership checks and clean up children/registrations on every exit. Windows still refuses explicitly.
3. Add `modex cli` dispatch with sibling host resolution and workspace defaulting to cwd, preserve legacy SSH flags, and update `make cli`, installation and README. Tests cover argument validation and launching arguments.
4. Verify full Go tests/vet/race, cross-build host/client, and disposable real native CLI picker/new-session/takeover lifecycle where available. Update PROJECT_STATE and commit coherent changes; do not push/merge or restart production supervisor.

Review focus: pagination errors never become new sessions; malformed IDs never launch; terminal escapes never render; new-session discovery never kills an unrelated client; quitting before a new ID cleans up the native TUI and backend.
