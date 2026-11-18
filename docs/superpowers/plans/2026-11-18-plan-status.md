# Native Android plan and status

Accepted scope: `/help`, `/plan`, `/status` only. Remove the previous slash shortcuts to existing UI actions; preserve those UI actions. Compact/review are deferred.

## Contracts and acceptance

`threads/mode` accepts workspace, thread_id, mode (plan/default), model, optional effort. It changes native collaboration settings without starting/resuming a session, prompting, changing permissions, or replaying uncertain requests. Only an owned loaded idle session without pending decisions can change mode. Return the safe session projection.

`threads/status` accepts workspace/thread_id and reads a loaded session without inference. Return session, nullable token_usage and nullable rate_limits. Current context uses native last.totalTokens and modelContextWindow; cumulative total is separate. Missing provider data is unavailable, never zero. Exclude credentials, account IDs, instructions and raw policies/errors. Shared terminal clients keep native schemas; Android events use safe projections.

Android dialogs show native acknowledged mode and actual status, refresh explicitly, and fence late replies by selected session and connection. Preserve the composer responsiveness fix and Catppuccin Mocha icon family.

## Execution

1. Backend owner plan_status_backend: protocol/validation/projections, owned worker and shared backend routing/cache/concurrency, focused RED/GREEN tests then race/vet.
2. Android owner plan_status_android: replace command menu and callers, mode/status dialogs and fenced VM integration; unit and instrumentation fixtures, unit/lint/build checks. No Go or delivery changes.
3. Primary: reconcile contracts, independent whole-branch review, isolated host checks, composer/slash/upgrade emulator checks, docs and state. Commit coherent verified changes.
4. Primary: PR and merge commit, same-signer increasing-code versioned APK and source tag, public prerelease with hashes and source revision. Install tested host and schedule external restart after 30s only as the final operational action; immediately report reset-cursor reconnect instructions.

No real-device availability or paid inference is assumed. Never uninstall/clear saved data; test encrypted synthetic state in place. Existing root and sibling worktree changes remain preserved.
