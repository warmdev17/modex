# Android slash commands implementation plan

**Goal:** Typing `/` offers only commands that perform real Android actions, without sending command tokens as model prompts.
**Architecture:** Reuse ChatWorkspace dialogs, sidebar, lifecycle callbacks, clipboard and the reference picker. Keep a small pure command matcher/catalog; no new host APIs, provider calls, dependencies or command framework. Consume the menu-polish shared surface and glyphs once that branch lands.
**Spec:** Owner's authorized follow-up after PR/merge; existing Codex 0.169.0 and Modex capability reconnaissance. The owner already asked to finish autonomously. Primary owns design/plan/integration and PROJECT_STATE; frontend specialist owns UI/parser/tests. No separate agent journals.

## Decisions and constraints

Use a local action palette, rather than forwarding slash strings through turn/start (Codex TUI dispatches commands locally; app-server prompts do not). A full terminal command clone would advertise actions Android cannot perform. Initial useful set: `/help` (show command help), `/model` (existing next-turn model/effort dialog), `/new` (existing create dialog), `/resume` (session sidebar), `/fork`, `/archive`, `/delete` (existing lifecycle/confirmation), `/skills` (existing skills/plugins picker), `/copy` (latest completed assistant reply). Each description states its Android behavior. Preserve same-set Mocha glyphs/surface, 48dp rows, narrow/keyboard layout.

Do not advertise `/vim`, `/keymap`, `/raw`, `/tui`, `/title`, `/statusline`, `/theme`, terminal-only features, unsupported login/provider/MCP/settings controls. `/stop` means background terminal stop in CLI and must not be mapped to interrupt. `/compact`, `/review`, `/plan`, `/goal` are useful future candidates but omitted until actual worker routing, turn/busy tracking and API support exist. Do not pretend readonly saved permission reports are live `/permissions` or full token-usage `/status`.

Match only a whole single-token leading slash query, case insensitive; do not trigger inside prose, code, multiline messages, file paths or reference markers. Selecting an action or submitting an exact supported command dispatches locally, never onSend. Partial queries show filtered choices. Dismiss leaves draft untouched. Executing consumes only command input; unrelated session drafts, attachments/references remain under existing ownership. No automatic command replay. Unknown slash input is not claimed to execute a native command.

Use existing enable/gating rules: connected/history loaded; `/model` requires selected idle session; lifecycle must target selected thread in current active list and preserve action/busy/pending-decision guards and delete confirmation; copy only when an eligible completed assistant reply exists. Hide/disable unavailable choices with honest labels; do not bypass guards by sending a command as text. `/skills` opens existing picker directly without injecting a fake reference. Normal messages retain existing Send behavior.

## Task 1 — primary inline (coder-fe usage quota unavailable)

Owned: new SlashCommands.kt + SlashCommandsTest.kt; minimal ChatWorkspace.kt and existing reference-picker integration as necessary; one focused instrumented slash preview and TransportSmoke selector. No ViewModel/Go/build config, release, state or Git writes. The pure matcher/catalog and its tests are independent and may be implemented first. Integration touching existing UI waits until primary integrates the frozen menu-polish commit. This avoids overlapping source ownership.

- [x] Write and run failing matcher/gating tests: bare slash/all, prefix/case, exact command, embedded slash/path/multiline/no match; meaningful context guards. Prefer plain pure matcher tests and runtime checks over a test-only duplicate dispatcher.
- [x] Implement minimal catalog/matcher and local action routing, reuse dialogs and picker. Include icons, descriptions, no unsupported commands, no prompt dispatch on exact/selected command.
- [x] Exercise real touch and text entry for `/`, filtering, keyboard layout, model/help/new/resume and cancel preserving draft, `/skills`, lifecycle delete cancel/confirmation and no accidental model request. Use synthetic in-memory callbacks only.
- [x] Run affected JVM/build/lint and instrumented preview; source freeze/report. Primary commits after independent review.

## Task 2 — primary delivery/integration

- [ ] Independent review against action semantics, guards, draft/context switching and one-set popup styling; fix and reverify.
- [ ] Coherent commit, PR, final-head CI and merge commit preserving history.
- [ ] Next unused dev12/code13 APK, established signer; direct encrypted synthetic upgrade from dev11/code12, affected menus/slash tests on actual distributable, exact-source tag/checksums/public prerelease and anonymous-download hash verification. No user-device clearing/uninstalling.
- [ ] Record exact tested revision and physical-device gaps. Any separate host preflight-fix deployment stays last operational step and uses the standing external 30-second restart authorization; never restart from active backend.
