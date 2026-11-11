# Android Chat UI Design

## Intent

Give Modex a focused native Android chat surface that feels intentional on a phone: the conversation remains the primary surface, old sessions live in a left drawer, and the composer is easy to reach and comfortable to use. This milestone changes presentation and navigation only; upload, MCP, skills and richer Codex actions come later.

## Goals

- Use a Catppuccin Mocha palette with blue accent and readable contrast.
- Keep the active conversation visible while opening and browsing old sessions.
- Make the session drawer useful: new chat, current session, session list, refresh and loading/empty states.
- Replace the current flat prompt row with a compact composer card, moderate corner radius, clear send action and working/disabled states.
- Preserve existing SSH, history, approval and turn behavior.
- Keep a responsive path for larger screens without introducing a second navigation model yet.

## Non-goals

- Uploading images/files.
- Rendering MCP servers, skills or tool catalogs.
- Changing Codex provider behavior. Optional thread name metadata is included for correct titles.
- Upload/MCP/skills controls remain absent until implemented; use small authored vector icons without a new icon dependency.

## Approaches

1. Replace the chat with a session picker: rejected because it hides the conversation and caused the current multitasking problem.
2. Bottom-sheet session picker: workable, but less discoverable and harder to grow into a navigation surface.
3. **Left navigation drawer over the chat: selected.** It matches the requested mobile interaction, keeps context visible, and can become a persistent rail on larger screens later.

## Visual direction

- Background: Catppuccin Mocha `base`; elevated surfaces use `mantle` and `surface0`.
- Text: `text` for primary content and `subtext1` for supporting labels; error uses `red`.
- Accent: Catppuccin `blue` for active session, focus and send action; avoid gradients and oversized pills.
- Surfaces use restrained 10–14dp corners. User messages may use a slightly larger 14dp bubble; assistant messages remain open on the background.
- Composer uses a single bordered surface with multiline input and a square-ish 44dp send button with a send icon.

## Interaction

- The top bar has a menu button, Modex title/status and connection action.
- Menu opens the drawer; selecting a session closes it and loads history without replacing the chat layout.
- New chat is a primary drawer action; refresh and load-more remain secondary actions.
- The composer send button is disabled when empty, loading or disconnected, and shows a progress indicator while a turn is active.
- Existing approval dialogs and connection form remain functional, receiving only palette and spacing updates where needed.

## Acceptance

- On the emulator, the chat screen shows the active conversation and the drawer opens above it.
- A stored session can be selected from the drawer and its history appears in the same chat surface.
- Empty, loading, working, disconnected and approval states remain legible.
- Android unit tests and debug build pass; a rendered emulator screenshot is inspected at phone size.


## Approved follow-up

Render assistant Markdown and fenced code natively, including code copy and horizontal scrolling. Use actual Codex `name` where available, then preview, then an untitled label with short ID. No HTML execution, remote image fetching or syntax-highlighting claim.
