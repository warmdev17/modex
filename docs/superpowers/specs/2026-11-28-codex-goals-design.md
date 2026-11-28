# Codex goals

Approved scope: create a goal, view objective/status/token budget and usage,
pause and resume from Android, synchronized with native Codex.

Use the installed Codex app-server goal API as the source of truth. A Goal
entry in the chat menu opens a compact Mocha dialog. An optional positive
token budget is only sent when specified. Actions await acknowledged server
state, disable duplicate submission and surface uncertain errors without
retrying mutations. Goal refresh only reads state. Native chat opening keeps
Codex's behavior: an existing Active goal can continue on thread resume.
Paused goals remain paused until explicit Resume. The modal explains this.
The user explicitly approved native Active-goal continuation on chat open.
Pause changes goal continuation, not the currently executing turn; Stop
remains separate. Resume is an explicit user action and may run inference.

Approaches: native API preserves CLI behavior and budgets. A Modex-owned
continuation loop would duplicate scheduling and is outside this milestone.
Use capability `goals` only for Codex. Check AGY/OpenCode current protocols,
reject unsupported goal commands clearly and preserve ordinary chat.

Thread/workspace validation and runtime routing must apply to every command
and event. Switching chats/hosts cannot retain another chat's goal. Native
goal updates and clears refresh the matching chat only. No goal creation or
resumption is implied by this feature's implementation request.
