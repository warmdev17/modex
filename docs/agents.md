# Agent support

[English](agents.md) · [Tiếng Việt](vi/agents.md)

Runtime support is capability-specific. Modex does not claim feature parity between agents. Install and sign in to the runtime on the host before using it; a running bridge alone does not prove provider authentication or inference.

## Connect an agent

1. Install the runtime on the host and complete its normal provider sign-in there.
2. Install Modex and start the host service for the workspace. The default service uses Codex; OpenCode uses its explicit backend setup in the [backend guide](opencode-backend.md).
3. Enable SSH public-key access, verify the host-key fingerprint outside Modex, then add that host in the Android app.
4. Open or create a project and choose the runtime when creating a new chat. Antigravity appears on a Codex-backed connection only after the host probe confirms that its CLI, sign-in, and ACP session recovery are ready.

Existing chats keep their selected runtime and native session. Connecting a host does not bind every project or chat to one agent.

| Runtime | Available in this preview | Known gaps |
| --- | --- | --- |
| Codex | Fullest path: chat, streaming, history, attachments, supported approvals/questions, and Codex-specific controls advertised by the host. | Some native Codex capabilities and operations remain unavailable; see the [protocol docs](history-contract.md) and [design](design.md). |
| OpenCode | Chat, history, model selection, attachments, project context, forms, and tool state. | No Codex-specific plan mode, permission profiles, takeover, references, or full project controls. Session state is temporary per supervisor. |
| Antigravity | Text prompts, default model, streaming, Stop, history, and one-time permissions. | No attachments or steer; no selectable model or full Codex/OpenCode control parity. |

An executable alone does not mean an agent is ready. The current Antigravity picker probe checks its CLI, sign-in, and required ACP capabilities before enabling selection. Codex and OpenCode host startup proves that their bridge initialized, not that provider authentication or inference will succeed. Provider accounts and credentials are configured and verified on the host.

This matrix describes the current preview. The [OpenCode backend implementation snapshot](opencode-backend.md) records an earlier verified revision and may not include newer projects, forms, or tool-history work.
