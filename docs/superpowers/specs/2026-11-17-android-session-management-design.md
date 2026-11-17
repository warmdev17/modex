# Android session management and turn synchronization

Status: approved by owner 2026-11-17 ("ok dứt đê"); implementation authorized. Existing Kotlin/Compose + Go/SSH architecture and Catppuccin Mocha UI are retained.

## Intent and scope

Complete the owner's current batch: disconnect to saved Hosts, explicit connection without reopening the previous conversation, native conversation archive/delete/fork from the sidebar, clear Codex failure/retry/interrupted state, and desktop-to-Android interruption synchronization.

## Approach

Use existing ConnectionHome/HostWorkspace, per-host ModexViewModel, SessionState and the native Codex app-server API. Extend the strict Modex bridge contract for the three operations and archived listing. Do not introduce a second conversation registry or manipulate Codex history files. Native API is the only deletion path: isolated Codex 0.169 evidence confirms deletion rejects a source whose paginated history is referenced by an ordinary fork. A custom file-deletion or database path would bypass that protection.

## Connection navigation

- Explicit Disconnect and connection loss from an open chat return to Hosts, retaining saved profile/key and other host connections. A setup form stays available while configuring/connecting a new host; failed connection is visible in its saved host row.
- Explicit Connect leaves session selection empty and shows the connected host's empty chat/select-session state. No remote conversation is created until New chat or an explicit user action.
- Remove the on-hello auto-selection of lastThread. Selecting a session or an Activity row remains an explicit action. Reconnect does not resend prompts, answers, approvals, or mutations.
- Foregrounding an already-connected host preserves its current selection; an actual reconnect does not automatically select the old session.

## Sidebar conversation management

- Reuse the existing session row and add an accessible overflow menu, including project-group rows and Recents.
- Active listing offers Archive, Fork and Delete. Archived is a sidebar view with clear heading/back to active list, search, refresh and pagination; archived rows offer Restore and Delete. Fork can be offered after restoration if native archived-fork behavior is not verified.
- Archive/restore use canonical thread/archive and thread/unarchive, preserve history and refresh the matching list. Do not automatically replay an uncertain response.
- Delete requires a dialog naming the conversation and explaining permanent deletion. Native dependent-history refusal is displayed clearly; a refused request leaves the row and caches intact. Never fall back to deleting source files.
- Fork uses native thread/fork from the current persisted conversation end, preserving verified workspace/project/provider/model/permission semantics. It opens the returned conversation only after validation; no prompt/turn is automatically started. Do not fork an actively running/pending-decision session silently.
- Fence in-flight actions/list responses by host, selected list mode, target identity and connection generation; prevent double submissions. Do not clear unrelated hosts or pending decisions.

## Codex notices and interruption

- Map structured Codex error information into clear user text, including stream closed before completion/disconnected, retrying/retry exhaustion, context limit, quota/model/auth access and failed turns. Unknown errors retain a safe fallback. Raw credential-bearing upstream errors/URLs are never displayed.
- Retain error/retry notices with the correct host/thread/turn, prevent late old events from overwriting a newer turn, and clear/reconcile stale retry notices on progress/completion.
- Consume supported thread-status notifications and reconcile turn state/history when required. Do not label an idle thread Completed without authoritative turn outcome.
- A desktop/native TUI attached to the same Modex backend must synchronize interruption to Android, including nonselected sessions and after reconnect. Independently owned Codex backends cannot be assumed to share live events; verify the supported observation path and report the actual boundary.
- Interrupted/cancelled turns stop the Android running indicator, keep partial content and show the correct outcome. A stale completion cannot stop newer work or another conversation.

## Verification and delivery

Use regression-first checks for connection selection/navigation, list/mutation validation, deletion refusal and rejected/uncertain actions, notice classification/privacy, and external interruption race handling. Run relevant Go/JVM suites, lint, exact APK emulator UI checks and encrypted in-place upgrade from dev.9/code10. Test backend changes in isolated supervisors and synthetic sessions only. Keep user histories/credentials and active runtime untouched during testing.

Deliver a versioned APK with the established signer and increasing code, source tag, checksums and GitHub development pre-release per the existing phone-development workflow. Local feature commits remain separate from merge authorization. Install a verified host update, then arm one external 30-second restart only as the final operational step; report its interruption risk and Android cursor-reset/reconnect instructions. No host restart for an Android-only result. Physical-phone verification is reported separately.

## Explicit limits

This batch does not reconstruct already deleted history, add background notifications/push, redesign the app, change provider authentication, or auto-resend uncertain actions.
