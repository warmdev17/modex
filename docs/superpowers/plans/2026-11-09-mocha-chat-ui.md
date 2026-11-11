# Mocha Chat UI Implementation Plan

**Goal:** Deliver the approved left drawer, restrained Mocha blue composer and native Markdown/code rendering, with actual Codex session names.
**Architecture:** Preserve the existing ViewModel/SSH protocol. List metadata gains optional Codex `name`. Compose chat shell and Markdown view are separated from the connection form and approvals.
**Spec:** ../specs/2026-11-09-android-chat-ui-design.md, with Markdown/code and actual title requested in the latest message.

## Constraints
- Existing workspace validation and no automatic prompt replay/fork remain.
- Catppuccin Mocha base #1e1e2e, mantle #181825, surface0 #313244, text #cdd6f4, blue #89b4fa; 10–14dp corners and 48dp touch targets.
- No inactive upload/MCP/skill controls; these are later features.
- Markdown uses CommonMark parsing with GFM tables, native Compose selectable text, safe HTTP(S) links, fenced code scroll and copy. HTML is inert text; no WebView or image fetching.

## Tasks
1. Session names: add regression for optional upstream `name` in list, retain preview fallback; JVM title fallback tests. Run failing then passing tests. Commit backend/Android label behavior.
2. UI: native drawer with loaded-session search, selected row, new chat dialog, refresh/load more, host footer; compact header and restrained composer with arrow send. Keep disconnected form and approvals functional. Add dedicated theme/shell files; replace old inline picker.
3. Markdown: CommonMark parser and native block/inline rendering; tests for fenced code, paragraphs, nested list, table, links, incomplete streaming fence and literal HTML. Code stays verbatim and copyable. No syntax coloring claim.
4. Verify: full Go tests; Android unit tests/build/lint; emulator screenshots chat/drawer/keyboard with fixture Markdown and actual SSH continuity if available. Review and commit coherent UI with related documentation/state. No push.

## Review focus
- Busy/loading/disconnected state must prevent prompt loss and accidental session switches.
- Back closes drawer, keyboard insets leave composer visible; touch targets and overflow on phone.
- Named/unnamed session fallback must not change IDs or workspace authorization.
- Streaming fence/list parsing must preserve code bytes and avoid active HTML/network images.
- Long code/table horizontally scroll instead of widening chat; scrolling up should not force user to bottom on every token.

## Progress
- Approved UI direction and incremental features reused; no new approval required.
