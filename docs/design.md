# Modex initial design

Status: provisional builder proposal, 2026-11-08. User authorized beginning work; Go and this milestone order are builder choices, not previously accepted stack decisions.

## Intent

Continue the user's actual Codex environment from a native Android phone, including custom providers, workspace configuration, skills and MCP. A separate agent that merely uses the same model is insufficient.

## Requirements already discussed

Android native; Linux/Windows/macOS hosts at first public release; explicit workspace; existing sessions; continuous synchronization; completion and decision notifications; optional chat bubbles; public distribution. A minimal push backend is acceptable. Monetization and license remain undecided.

## Approaches

1. Parse terminal output: easy initial transport, fragile structured state and approvals. Rejected as the product foundation.
2. Codex app-server over an SSH-owned host bridge: structured history/events/questions, target-owned credentials. Builder's provisional choice.
3. Replace Codex with a custom agent: increases behavioral differences and conflicts with preserving the environment.

## First milestone and boundaries

Use Go standard library only for a local host diagnostic. Require an existing explicit workspace; inherit the environment, optionally override CODEX_HOME; launch a short-lived app-server over stdio. Initialize, config/read, thread/list. Emit an allowlisted summary; no model turn or thread mutation. Bound frames and total execution time. No remote listener or remote shell API yet.

Named profiles must fail explicitly until the installed Codex exposes a verified supported selection path. Configuration summary proves loading only, not authentication, connectivity or inference. Cross compilation proves build portability only, not Windows/macOS runtime support.

## Next milestones

Establish profile semantics and history pagination; validate custom provider inference in an isolated test project; implement supervised host lifecycle and safe reconnect; Android SSH connection and session browser; streaming/approvals with pending-decision reconciliation; push relay and device pairing. The CLI and Desktop may own a thread exclusively: investigate supported live attachment without promising seamless concurrency or bypassing ownership locks.

The diagnostic is an initial probe. Its short-lived process is not the final session supervisor. Do not publish keys, raw errors or raw configuration. No license, billing, deployment or commits are implicit in beginning development.
