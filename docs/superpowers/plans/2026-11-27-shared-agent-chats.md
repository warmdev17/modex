# Shared Agent Chats Implementation Plan

**Goal:** Create and reopen Codex, OpenCode and Antigravity chats in a shared project over one SSH connection.

**Architecture:** Extend the existing Codex-backed supervisor with an isolated managed OpenCode runtime. Keep Modex project membership and conversation identifiers stable at the bridge.

**Global constraints:** Preserve startup and wire compatibility of dedicated OpenCode supervisors and existing clients. Host credentials stay on host. No automatic import, takeover, or rerouting across agents. No changes to the live supervisor during tests.

### Task 1: OpenCode host runtime and common routing

- [x] Own cmd/modex-host and internal/opencode for stage 1. Use existing OpenCode APIs/events and keep the dedicated backend on its existing code path. Optional OpenCode startup failure must leave Codex and AGY accessible. Support custom binary path for the managed route via --opencode on a Codex supervisor; check default PATH without extra flags.
- [x] Advertise ready flag runtime_routing alongside agent_discovery on the upgraded Codex supervisor only. agents/list returns data:[{id,label,ready,reason?,capabilities?:[string]}] for codex, opencode and antigravity. AGY retains existing readiness. OpenCode requires compatible runtime, API and at least one available provider-qualified model, using sanitized reason if no model is configured. Accept optional workspace to check readiness for a selected project.
- [x] Support agent on threads/start, threads/list and models/list. Omitted threads/start remains Codex. Omitted threads/list preserves existing Codex+AGY behavior for old clients. Explicit lists return per-agent pages with existing {data,next_cursor}. Codex uses existing cursor. AGY and OpenCode paginate registered chats using stable ID order. Use shared Modex projects/list and validate project/root before optional-runtime creation. Outside projects retains current standalone behavior.
- [x] OpenCode Modex IDs use oc: + random UUID and private persistent records mapping ID to native session, canonical workspace and Modex project. thread.agent is explicit in new routed results. Keep OpenCode child data/state in checked private persistent directories under supervisor-owned agent-chats, with an explicit private OPENCODE_DB. Host configuration and environment authentication remain available. A saved account requires native OpenCode sign-in in that managed profile. Do not import, link or copy existing account or session stores.
- [x] Rewrite native OpenCode thread/decision identities at the adapter boundary in results and events. Scope events to registered chats, verify exact workspace and active turn, keep pending decisions isolated, and replay once after the existing connection reset. Support current OpenCode history, prompt, attachments, steer, Stop, approvals/forms. Reject unsupported Codex-only controls. Treat stale/unknown IDs as errors and avoid fallback across agents.
- [x] Test routing, scoped events, missing runtime, legacy compatibility, private state, restart and same-project isolation. Use fake runtime for regression and report exact tests.

### Task 2: Android shared chat surfaces

- [x] Own Android main source and affected unit tests. Consume runtime_routing to enable the upgraded protocol. Parse all three runtime IDs. Dedicated OpenCode remains the default for an old OpenCode connection. Preserve legacy Codex+AGY picker on hosts without runtime_routing.
- [x] Keep Add host SSH-only. Use selected agent and workspace to scope readiness/catalogue requests in New chat. Reject late results from the previous host/agent/workspace. When opening a chat, use its metadata to scope models, capabilities, composer, and menu actions.
- [x] In shared mode, request explicit per-agent thread pages. Maintain independent cursors and request/generation state. Merge by ID without dropping successful agent results if another agent errors. Default filter All and offer Codex/OpenCode/Antigravity. Show per-row agent label and only one row per chat across Projects and Recents. Filter and search compose. Preserve existing Codex archived behavior.
- [x] Verify Kotlin tests and debug build/lint. Cover legacy modes, catalogue isolation, filters and safe optional agent selection.

### Task 3: Integration and handoff

- [x] Exercise one shared connection against isolated supervisor, plus host and Android regression suites. Verify restart persistence, separate concurrent turns and stale-event isolation. Test signed upgrade with synthetic state. Record actual limitations and update bilingual agent support and project state.
- [x] Implement stage 2 on a separate branch and integrate the verified stage 1 result into it. See the [Antigravity session-controls plan](2026-11-27-antigravity-session-controls.md).
