# Native Android mode button and status

Accepted revised scope: remove all slash-command UI and interception, including help. A single button above the message editor toggles native Default/Plan mode. A compact context indicator under the chat title opens status details. Preserve existing ordinary UI actions and @ skills/plugins. Compact/review are deferred.

## Contracts and acceptance

`threads/mode` accepts workspace, thread_id, mode (plan/default), model, optional effort. It changes native collaboration settings without starting/resuming a session, prompting, changing permissions, or replaying uncertain requests. Only an owned loaded idle session without pending decisions can change mode. Return the safe session projection.

`threads/status` accepts workspace/thread_id and reads a loaded session without inference. Return session, nullable token_usage and nullable rate_limits. Current context uses native last.totalTokens and modelContextWindow; cumulative total is separate. Missing provider data is unavailable, never zero. Exclude credentials, account IDs, instructions and raw policies/errors. Shared terminal clients keep native schemas; Android events use safe projections.

Android mode button shows acknowledged state, disables during mutation, and switches with one tap; no mode chooser. Fresh unknown mode offers Enable Plan without pretending the current mode is known. Uncertain mutations require a successful refresh before retry. Context indicator preserves host identity and connection status, uses last.totalTokens/modelContextWindow, and opens status details with refresh. Fetch initial status after explicit open/create and update context after turn completion without polling. Fence replies by connection, selection and settings revision so an older status cannot overwrite a mode ACK. Slash input is plain prompt text; @ suggestions retain their bounded layout. Preserve Catppuccin Mocha and existing icon family.

## Execution

1. Backend owner plan_status_backend: protocol/validation/projections, owned worker and shared backend routing/cache/concurrency, focused RED/GREEN tests then race/vet.
2. Android owner plan_status_android: delete slash menu/parser/dispatch/help and obsolete tests; add one mode button and tappable compact context indicator/status dialog using fenced VM integration. Unit and instrumentation cover fresh mode, ACK, uncertainty, stale responses and plain slash prompt dispatch. Keep @/history/attachment responsiveness. No Go or delivery changes.
3. Primary: reconcile contracts, independent whole-branch review, isolated host checks, composer/slash/upgrade emulator checks, docs and state. Commit coherent verified changes.
4. Primary: PR and merge commit, same-signer increasing-code versioned APK and source tag, public prerelease with hashes and source revision. Install tested host and schedule external restart after 30s only as the final operational action; immediately report reset-cursor reconnect instructions.

No real-device availability or paid inference is assumed. Never uninstall/clear saved data; test encrypted synthetic state in place. Existing root and sibling worktree changes remain preserved.
