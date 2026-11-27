# Antigravity stage 3: session controls and subagent monitoring

## Goal

Select a model and thinking level in an Antigravity chat on Android. Follow
subagent activity from the same chat while keeping each setting and activity
record scoped to its parent conversation.

## Approach

Use capabilities and events from the existing `agy acp` connection. A global
CLI default would affect unrelated chats. Reading the CLI's private logs or
guessing which child owns an event from its timing cannot reliably identify
child sessions, especially with overlapping calls.

### Session controls

- Read the model and thinking choices returned by ACP for the selected
  session. Display the acknowledged values in the existing Android model
  picker. The account and provider decide which choices are available.
- Send a session-scoped `session/set_config_option` while the chat is idle.
  Validate against that session's latest choices and display the values
  returned by AGY. Take model-dependent changes in thinking options from the
  same response. Preserve the draft on failures or reconnect.
- The host remembers acknowledged choices by Modex chat ID. On native
  session recovery, reconcile them with the current ACP options before the
  next prompt. Show the current native value if a saved value is unavailable.
  If a transient ACP failure prevents confirming the selection, retain the
  draft and report the failure so the user can retry before sending.
  Older chats without saved choices follow the native session defaults.
- Keep the existing default-model path available for a host or CLI that
  supplies no usable session options. Codex and OpenCode retain their own
  selectors and behavior.

### Subagent monitoring

- Project explicitly identified native `invoke_subagent` activity into the
  existing Android Agents surface. Track native call identity within the
  parent conversation and reflect supported call states as they arrive. If
  AGY reports that the invocation completed, label it as invocation completion
  unless AGY also confirms the child's terminal status.
- Keep observed events available across a phone reconnect from the supervisor
  journal. After journal expiry or host restart, recover activity that native
  history provides. This stage adds no separate durable activity log. Never
  infer a running child from a stale launch, attribute unrelated tools to a
  child, or journal raw tool parameters, prompts or output to build the
  activity summary.
- This milestone shows invocation activity in the Agents surface. AGY child
  navigation is a follow-up that requires an observed native child identity
  and a matching stream. Existing Codex child navigation remains available.

## Evidence and acceptance

- Local `agy 1.5.17` negotiated ACP v1. `session/new` reported `model` and
  `thought_level` as select options. Setting `thought_level` updated one
  session without changing another. An invalid value returned `-32602`.
  Loading the test's empty session on a fresh process returned its original
  thinking level, so the host cannot assume settings always survive restart.
- A short read-only subagent turn completed in an isolated workspace. ACP
  delivered a parent tool start and completion with role and model fields in
  the input, and delivered no child-scoped event in that run. Loading that
  session on a fresh process replayed one tool call on the parent. This
  evidence supports monitoring the invocation. A child transcript needs
  separate verification against a CLI that provides child-scoped events.
- Before implementing the tool projection, capture a sanitized start/update
  fixture that pins the native tool discriminator and correlation fields.
  Key live calls by parent session, turn and call ID, and match updates only
  to calls already identified as subagent invocations.
- Exercise model and thinking negotiation, stale options, individual chat
  settings, reconnect, host restart, and legacy fallback. Test tool-call
  deduplication, interruption, parent isolation and reconnect. On Android,
  verify the picker and Agents surface alongside the existing Codex behavior.

See [Antigravity ACP client behavior](https://antigravity.google/docs/ide/extensions/acp/)
and the [previous Modex milestone](2026-11-27-antigravity-android-chat-design.md).
