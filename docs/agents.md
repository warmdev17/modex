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
| Antigravity | Text prompts, streaming, Stop, history, one-time permissions, and model and thinking selection on compatible hosts. | Attachments and text follow-ups require the development build described below. |

Antigravity must return a model catalog and support ACP session recovery
before the picker enables it. Codex and OpenCode must initialize their
bridges. Each provider applies its own account and quota requirements when
you send a prompt.

This matrix describes the current preview. The [OpenCode backend implementation snapshot](opencode-backend.md) records an earlier verified revision and may not include newer projects, forms, or tool-history work.

## Shared agent chats in development builds

### Goals

On a Codex chat with an updated host and Android app, open the chat menu
and choose **Goal**. The modal creates a native Codex goal with an optional
token budget and shows the objective, status, tokens used and elapsed time.
Pause disables goal continuation; use Stop separately to interrupt the
current turn. Resume explicitly allows Codex to continue working. Goal
state follows native CLI updates and is refreshed when reopening the modal.

The first goal integration targets Codex 0.169.0. OpenCode 2.4.1 has native
goals, including pause/resume/clear, but its goal API is not yet connected
to Modex. Antigravity 1.5.17 does not advertise a goal capability through
ACP. Goal controls are therefore limited to Codex in this milestone;
ordinary OpenCode and Antigravity chat controls keep their existing scope.

### Runtime selection

With an updated host and Android app, open one SSH connection and choose
Codex, OpenCode or Antigravity in **New chat**. The sidebar can filter by
agent. Chats in the same project keep their own agent and history.

The managed OpenCode path requires OpenCode 2.4 or later in the 2.x series.
It keeps its own database in the supervisor state directory. Existing
standalone OpenCode chats and accounts stay in their existing profiles.
Provider access must be configured for the managed runtime on the host.
Verify the requested provider and model in that profile before starting work.

For an interactive OpenCode login, set `supervisor_state` to the state
directory configured in Modex. Run the login as the same host user, after
the first runtime check has initialized the profile.

```sh
profile="$supervisor_state/agent-chats/opencode"
XDG_DATA_HOME="$profile/data" XDG_STATE_HOME="$profile/state" \
  OPENCODE_DB="$profile/data/opencode/opencode.db" opencode auth login --standalone
```

Refresh the agent list after signing in.

## Shared terminal sessions

Run `modex cli` in a terminal. Use ↑/↓ to choose an agent and a session,
Enter to open it, and Esc to go back. A new conversation uses the current
workspace. Codex opens its native terminal UI.

OpenCode and Antigravity open their installed native terminal interfaces.
OpenCode attaches to Modex's managed server and native session. Missing
provider credentials are imported from the host's existing OpenCode
account through its supported export/import commands. Existing accounts
in the managed profile keep their selection.

The OpenCode handoff watches that session for terminal-originated turns,
so Android can receive its activity and answer pending requests. Exiting
the terminal releases the watch while accepted turns keep running.
Antigravity opens the saved native conversation and reserves it for the
native terminal. Close that terminal before using the same chat on Android
to refresh its history and send more prompts.

The UI uses RGB accents for the terminal theme and respects `NO_COLOR`.
Piped input keeps the line-oriented interface. Open a saved session
directly with `modex cli --thread <id>`. For OpenCode or Antigravity, run
from the session's workspace or pass its directory with `--workspace`.

## Antigravity in development builds

Modex runs the Antigravity terminal, model discovery and ACP with
`AGY_CLI_FORCE_FILE_TOKEN_STORAGE=1`. They share AGY's durable token
storage across new shells and host processes. If this storage has never
been signed in, run the following once as the host user, then reopen
the picker.

```sh
AGY_CLI_FORCE_FILE_TOKEN_STORAGE=1 agy acp --login
```

When both the host and the Android app support Antigravity session options,
the chat's model picker shows the model and thinking choices reported by ACP.
Change them while the chat is idle. Modex remembers the confirmed choices for
that chat and checks them against the available options when reopening it.
Older hosts and ACP sessions without options use Antigravity's default model.
Antigravity 1.5.17 lists a new chat for restart recovery after its first
recorded turn.

On compatible builds, the header and Session status show the context window
reported by Antigravity. The count updates as the CLI reports usage and can
be unavailable until the next sample after reopening a native session.

With an updated host, shell commands reported by Antigravity appear in
the chat timeline. Expand a command to see the available output and exit
code. Reopening a chat restores the commands included in native history.

When Antigravity reports a completed file edit with before-and-after
text, expand the file change card to see the changed lines. Reopening
the chat shows the edits included in native history. Large snapshots
may be omitted, and the preview marks clipped diffs.

With matching Modex builds, the title Antigravity generates appears in the
chat header and sidebar and is retained when reconnecting. Older chats recover
their native title when reopened if Antigravity supplies one.

With a compatible host and Android build, use **Attach files** in an
Antigravity chat to send PNG or JPEG images and UTF-8 text files. You can
send a file on its own or include a prompt. Each message can carry up to
eight files and 8 MiB in total. Each text file is limited to 256 KiB.
Antigravity controls model access to image input. After a host restart,
attachment previews are reconstructed from the local file references
included in native history. Some sent files may be absent from that history.

With matching Modex builds and Antigravity CLI 1.5.17, tap Send while a
response is streaming to pass a text prompt to the same session.
Wait until the conversation is idle to send drafts with attachments. Stop
applies to the native session while its prompt requests are still running.

The Agents panel shows subagent invocations reported by Antigravity. An
invocation marked completed means the tool returned. The CLI tested here
reports invocation activity without a separate child transcript.
