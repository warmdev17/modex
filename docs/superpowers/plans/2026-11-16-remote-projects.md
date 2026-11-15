# Remote project creation

Goal: create a canonical Codex project from a chosen remote folder and start chats in that project's root, independently of the supervisor workspace.

Use the existing full-screen SFTP browser in folder mode, starting at home. Allow explicit single-child folder creation with no shell execution. Register the selected existing directory through `projects/create` -> Codex `project/create` with an idempotency key. New-chat settings select an existing project or the current/default workspace; pass both workspace and canonical project ID to `thread/start`. Keep existing permissions/model choices unchanged.

- [x] Add bridge validation and projection tests, observe failure, implement bounded project creation and project-aware thread start.
- [x] Reuse the native browser for directory selection/creation; add sidebar entry and project selection to new-chat settings. Test validation and reducer behavior.
- [x] Run Go/Android checks, isolated SFTP/UI and in-place upgrade checks; inspect the diff, commit the coherent feature.
- [ ] Publish a signed versioned development APK with a source tag and checksum. Stage the host binary, leaving live activation to an idle boundary. Do not merge without a new request.

No Git initialization, repository cloning, deleting folders, or automatic mutation replay. An empty project remains valid if the user cancels chat creation. Uncertain results require refreshing the project list before retrying.
