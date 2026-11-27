# Antigravity CLI host core

The first milestone gives Modex Host a tested connection to Google Antigravity
CLI in the selected workspace. Android and agent selection at New chat follow
after the host can open a session and carry a complete turn.

## Protocol

- Use `agy acp` and ACP v1 over stdio. ACP exposes native sessions,
  streaming updates, cancellation and permission requests on one connection.
- Headless `agy --input-format stream-json --output-format stream-json`
  supports sequential prompts and streamed text. Its input contract excludes
  permission responses. ACP also provides session discovery and history.

The test baseline is installed `agy 1.5.17`. The adapter checks the negotiated
protocol and advertised capabilities. The executable and credentials belong
to the host user. Missing authentication gets its own readiness result.

## First milestone

1. Start and reap `agy acp` inside the verified workspace. Test a bounded
   handshake with the real binary and report the new runtime through a local
   host probe.
2. Add host operations for native session listing, creation, history loading,
   text prompts, streamed updates and cancellation. Native session IDs are
   backend references. The future chat router owns Modex conversation IDs.
3. Confirm successful completion with the native prompt response. Match Stop
   to the active turn. Treat transport loss as uncertain and leave retries to
   the caller. Fail closed when a tool needs permission from a client.
4. Verify streaming, cancellation, session isolation and permission handling
   with a synthetic ACP subprocess. Exercise readiness against the installed
   CLI in an isolated workspace without spending model quota.

Keep the existing Codex and OpenCode host paths separate from this probe.
The current milestone includes no Android APK or production service change.
The playback of tool activity, permission UI, model selection and per-chat
routing belong to the subsequent Modex integration milestone.
