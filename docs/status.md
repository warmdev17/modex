# Current status

[English](status.md) · [Tiếng Việt](vi/status.md)

Modex `v0.1.0-dev.38` is a development preview, not production-ready software. The Android package is `dev.modex` and supports Android 8.0 (API 26) or later.

| Area | Current state |
| --- | --- |
| Android client | Native Jetpack Compose app distributed as a development pre-release APK. |
| Host | Linux and macOS native operations; Windows host use through WSL2. |
| Transport | SSH connection to a host supervisor and bounded event journal. |
| Agents | Codex is the fullest path. OpenCode and Antigravity have narrower, documented capabilities. |
| Credentials | Provider credentials and agent execution remain on the host. |
| Cloud and orchestration | Future direction only; unavailable in this preview. |
| License | No project license has been chosen. |

See the [install guide](install.md) for current download and platform instructions and the [agent support matrix](agents.md) for capability limits. Do not treat development builds as a production signing or support channel.

## Already working in the preview

- Native Android chat over SSH with reconnectable host journals.
- Codex conversations, streaming, history, supported approvals/questions, attachments, plan controls, and read-only subagent activity.
- OpenCode conversations, models, projects, forms, tool state, and history within its documented limits.
- Antigravity text conversations, streaming, Stop, history, and one-time permissions using its default model.

## Current focus

- Harden OpenCode and Antigravity behavior against their native CLIs and close the highest-value parity gaps.
- Improve setup, recovery messages, physical-device UX, accessibility, and release verification.
- Keep the runtime capability model agent-neutral so a host connection does not lock every chat to one agent.

## Preparing next

- More reliable background completion and action-required notifications.
- Broader host installation and lifecycle support, including a native Windows host path when its private-state protections are ready.
- The relay and managed-execution foundations described in the [roadmap](roadmap.md); they are not part of the current preview.
