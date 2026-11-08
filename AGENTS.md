# Git workflow

- Before editing, inspect the current branch, working tree and relevant diff. Preserve existing user changes and work from other sessions.
- Start a separate branch for each feature or substantial action, including major bug fixes, refactors and infrastructure changes. Use descriptive names such as `feat/approval-flow`, `fix/thread-resume`, `refactor/event-reader` or `docs/git-workflow`.
- Do not implement features or substantial actions directly on `main` or `master`. Small follow-up changes belonging to the same feature may stay on its branch.
- Keep unrelated work on separate branches. If the working tree already contains unrelated changes, isolate the new work in a worktree or otherwise preserve those changes before proceeding. Never discard, silently stash or bundle them into the new task.
- Commit after each coherent, verified milestone. Each commit must have one clear purpose; split unrelated features, fixes, refactors and formatting into separate commits.
- Keep implementation, its necessary tests and directly related documentation together when they form one coherent change. Do not split a change into commits that knowingly leave the project broken.
- Use clear commit messages describing the concrete change, for example `feat(bridge): handle one-time approval responses`. Avoid vague messages such as `update`, `misc fixes` or `changes`.
- Before committing, review the staged diff, check for secrets and unintended files, and run checks appropriate to the change. Stage explicit paths or hunks; do not use blanket staging when unrelated changes are present.
- Report the branch, commits, checks and any remaining uncommitted changes when handing off implementation work.
- Local branches and commits are part of the implementation workflow. These rules do not authorize pushing, merging, publishing or rewriting shared history without the user's instruction.
