# Stored text history

`threads/read` accepts optional `include_turns: true` (default false). This is read-only and never resumes, forks or starts a turn. Before returning any content, the host verifies thread ID and exact workspace cwd.

List items include `{id,cwd,modelProvider,preview?}`; `preview` is trimmed and bounded to 160 Unicode characters for session labels. `threads/read` returns metadata by default; with `include_turns:true`, its result is `{thread:{id,cwd,modelProvider},messages:[{id,role,text}],active_turn:boolean}`. Roles are `user` or `assistant`; text comes only from userMessage text content and agentMessage text. Tools, developer/system instructions and upstream error metadata are omitted. Nontext user content is omitted. active_turn reflects upstream `inProgress` status. Existing metadata-only reads and thread resume remain unchanged.

Full history is subject to existing 4MiB upstream frame limit; an oversized upstream response fails explicitly, never silently truncates. No history pagination in this slice. Provider credentials remain target-owned. History is sensitive task content intentionally requested by the authenticated user.
