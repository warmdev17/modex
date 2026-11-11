# Stored text history

`threads/read` accepts optional `include_turns: true` (default false). This is read-only and never resumes, forks or starts a turn. Before returning any content, the host verifies thread ID and exact workspace cwd.

List items include `{id,cwd,modelProvider,name?,preview?}`; `name` (the actual Codex title) and `preview` (usually the first user message) are trimmed and bounded to 160 Unicode characters each. The Android label prioritizes name. `threads/read` returns metadata by default; with `include_turns:true`, its result is `{thread:{id,cwd,modelProvider},messages:[{id,role,text}],active_turn:boolean}`. Roles are `user` or `assistant`; text comes only from userMessage text content and agentMessage text. Tools, developer/system instructions and upstream error metadata are omitted. Nontext user content is omitted. active_turn reflects upstream `inProgress` status. Existing metadata-only reads and thread resume remain unchanged.

Upstream Codex frames are bounded to 16MiB because stored turns also contain tool output before projection. The downstream bridge/mobile frame limit remains 4MiB; neither history nor tool output is silently truncated. No history pagination in this slice. Provider credentials remain target-owned. History is sensitive task content intentionally requested by the authenticated user.

## Session model and reasoning

Host status exposes only the allowlisted configured provider/model and optional `effort` from `model_reasoning_effort`. Thread metadata may include `model` and `reasoningEffort`; start/resume results also expose optional top-level `effort`. Missing values remain absent rather than being guessed. Android stores a separate model/effort choice per thread in its encrypted host snapshot, persists it at selection, and gives it precedence over later host defaults or resume responses. New chats resolve host defaults and advertised default effort, while explicit turn overrides leave global Codex configuration unchanged. Forget clears the stored choices; cursor reset preserves them. No prompt is replayed automatically.
