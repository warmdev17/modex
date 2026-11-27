# Antigravity session titles

**Goal:** Show the Antigravity conversation title in the existing Android header and sidebar, updating as AGY names the conversation and retaining it after reconnect.

**Runtime evidence:** AGY 1.5.17 emitted `session_info_update` with a top-level `title` during a synthetic prompt. The matching `session/list` entry returned the same title.

- [x] Host accepts nonblank native titles up to 512 UTF-8 bytes with no control characters. Route them by workspace and known native session. Store the title with the existing private chat record and return it as `thread.name`. Recover the title from the matched native listing on reopen. Send `thread/name/updated` with `threadId` and `threadName` if the title changes during a connected session.
- [x] Android uses the existing session label for list responses, adopts a valid AGY name from a session-open response, and applies the live event only to a known AGY chat. Preserve the current visible name when a snapshot has no title or was requested before the last live title update. Exercise header and sidebar with the real ViewModel and synthetic host responses.
- [x] Verify the native-title flow through an isolated supervisor, including restart. Run Go race and vet, Android unit and lint, and an in-place signed preview upgrade. Store preview 54 under the main checkout's `.modex/previews/`.
- Activate the verified host with the delayed restart at handoff.
