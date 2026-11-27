# Antigravity session controls implementation

**Goal:** Set the model and thinking level per AGY chat, and monitor native subagent invocations in Android.

**Spec:** [Session controls and subagent monitoring](../specs/2026-11-27-antigravity-session-controls-design.md).

**Basis:** Test the shared runtime milestone before integrating these changes into its user-facing path. Keep this stage on its own branch. Use ACP v1 and the stock Android model picker and Agents panel.

### Task 1: Native options and events

- [x] Capture acknowledged select options from session/new, session/load and session/set_config_option. Keep them scoped to loaded ACP sessions. Validate the current catalogue, reject stale choices, and preserve the previous text path when options are absent.
- [x] Decode explicit invocation identities and statuses in typed tool updates. Verify the observed native event shape. Avoid retaining raw prompts, outputs or unrelated tool metadata.

### Task 2: Modex chat session state

- [x] Expose model and thinking options through an additive AGY session-options capability and `threads/options` and `threads/options/set` RPCs. Use `thread_id` and `workspace` for both. The setter takes one `id` (`model` or `thought_level`) and one `value`. Return the complete acknowledged option list after each read/mutation.
- [x] Restore options from the private AGY chat record on first load. Reconcile model before level, with current choices from each native response. Preserve drafts when reconciliation fails and protect active turns from settings changes. Keep older records readable.
- [x] Project native invocation lifecycle to the existing agentActivity envelope scoped to the parent chat. Separate invocation completion from child completion. Use journal events for reconnect and supported history for reload. Do not expose child navigation.

### Task 3: Android and acceptance

- [x] Show only native acknowledged options and labels. Disable changes during busy or pending requests and reject late responses for another chat. Preserve old hosts' text/default-model behavior.
- [x] Display invocation states in Agents without a child transcript link. Keep Codex child navigation and the OpenCode model picker working.
- [x] Verify ACP and host tests, race suite, Android unit tests, lint, a targeted UI fixture and the isolated supervisor flow. Update support matrices and the handoff with observed limitations.
