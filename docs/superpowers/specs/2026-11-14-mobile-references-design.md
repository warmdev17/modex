# Mobile prompt references

Implement the requested @ skill picker on native Android. Retain the existing Catppuccin Mocha composer and shared supervisor backend.

Approaches considered: scanning skill directories ourselves would duplicate Codex discovery and miss workspace/plugin rules; text-only name insertion would lose exact skill identity. The builder chooses native skills/list discovery plus structured skill input, using the user's authorization to implement directly.

Typing @ at a token boundary opens a compact searchable picker above the composer. Selecting a skill inserts Codex's canonical $name marker and binds its exact remote path. Selected references appear as removable chips and remain independent per session draft. Deleting a marker removes its binding; removing a chip removes its marker. No popup for email addresses or a noncollapsed selection.

Host references/list projects workspace-scoped skills/list into typed entries with kind, name, path, title, description, scope, enabled and optional plugin ownership. It returns supported_kinds and a count of discovery issues, never upstream error messages or skill bodies. turn/start accepts up to eight references; before dispatch it verifies exact kind/name/path against a fresh enabled catalogue for the session workspace. Unknown kinds fail closed. No skill installation, config writes or arbitrary file reads.

Expansion boundary: the picker and draft binding consume typed references independently of catalogue sources. Each future kind requires a host discovery/input adapter and tests; MCP tools, subagent execution and plugin installation are not interchangeable with skills and must implement their own native contracts. Installed plugin skills work now through Codex discovery; other extension APIs are deferred.

Catalogue requests are scoped to the selected workspace, cleared on disconnect/session switch, and stale replies cannot replace the active catalogue. Refresh is available and skills/changed invalidates discovery. Existing text, attachments, models, approvals and native CLI sharing remain intact. Existing unmanaged sessions and provider credentials are unchanged.
