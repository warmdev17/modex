# Git workflow

- Before editing, inspect the current branch, working tree and relevant diff. Preserve existing user changes and work from other sessions.
- Start a separate branch for each feature or substantial action, including major bug fixes, refactors and infrastructure changes. Use descriptive names such as `feat/approval-flow`, `fix/thread-resume`, `refactor/event-reader` or `docs/git-workflow`.
- Do not implement features or substantial actions directly on `main` or `master`. Small follow-up changes belonging to the same feature may stay on its branch.
- Keep unrelated work on separate branches. If the working tree already contains unrelated changes, isolate the new work in a worktree or otherwise preserve those changes before proceeding. Never discard, silently stash or bundle them into the new task.
- Commit after each coherent, verified milestone. Each commit must have one clear purpose; split unrelated features, fixes, refactors and formatting into separate commits.
- Keep implementation, its necessary tests and directly related documentation together when they form one coherent change. Do not split a change into commits that knowingly leave the project broken.
- Use clear commit messages describing the concrete change, for example `feat(bridge): handle one-time approval responses`. Avoid vague messages such as `update`, `misc fixes` or `changes`.
- Before committing, review the staged diff, check for secrets and unintended files, and run checks appropriate to the change. Stage explicit paths or hunks; do not use blanket staging when unrelated changes are present.
- Create a pull request before merging features or changes that affect project logic, quality, security, dependencies, build or runtime behavior. Include a summary of the changes and verification results.
- Small changes that do not affect project logic or quality, such as instruction edits, typo fixes or removal of verified unused documentation, may be merged directly without a pull request after reviewing the diff and running appropriate checks.
- For both pull requests and direct branch merges, use a merge commit that preserves every original commit. Do not squash or rebase-merge.
- Report the branch, commits, checks and any remaining uncommitted changes when handing off implementation work.
- Local branches and commits are part of the implementation workflow. These rules do not authorize pushing, merging, publishing or rewriting shared history without the user's instruction.

# Phone-only development delivery

- During an owner-authorized mobile development session, finishing a verified milestone includes a versioned Android APK, a source tag, and a downloadable GitHub pre-release link; a local build or Actions artifact alone is not delivery. Keep the existing PR/merge rules.
- Verify package identity, the established signing certificate and an increasing versionCode before publishing. Never rotate the signing identity, uninstall the app or clear its data to work around an update failure. Keep signing keys private; never put them in Git or release assets.
- Test an in-place APK upgrade with encrypted synthetic state, plus the affected unit, lint and UI checks. Report physical-device gaps separately. Publish checksums and identify the exact source revision.
- Android-only updates must not restart modex-host. Installing an APK may restart the Android process; keep server-side turns alive and restore the saved connection afterward.
- Test host changes in an isolated supervisor before deployment. A successful test does not make an ordinary service restart non-disruptive: the current service owns the Codex backend. Do not restart it from its own active turn or discard pending decisions.
- Until a tested drain/reconnect or persistent-engine handover exists, stage host upgrades and explain the remaining activation step instead of promising uninterrupted restarts. See docs/mobile-development.md.
