# Shared Codex backend and direct command installation

User-authorized implementation: keep a turn running while native CLI and Android
control the same session; install both binaries into PATH after a local build.

## Design

The supervisor owns one persistent Codex app-server. Android keeps its existing
authenticated SSH/journal protocol. A private Unix WebSocket gateway presents
the native app-server protocol to `codex --remote`; it forwards requests to that
same process. Client disconnect never closes that process. Existing standalone
Codex writers still require explicit legacy takeover; do not signal other CLIs.

Reuse the existing stdio Process, decision validation, WebSocket dependency and
state ownership checks. This avoids replacing Android's protocol, copying
history or exposing an unauthenticated network listener. Keep direct `--serve`
and legacy takeover compatibility. No Claude/OpenCode implementation here.

## Tasks

1. Test the shared gateway using two native clients and the existing bridge:
   one backend, shared thread, streamed turn and approval from either client,
   first answer wins, disconnect retains work, bounded slow consumers,
   uninitialized clients denied, private socket and clean shutdown.
2. Implement the gateway and enable it for supervisor-owned bridges. Publish
   its socket in the private endpoint; native clients subscribe by thread.
   Shared request timeouts report uncertainty without killing the backend.
3. Route `modex cli` to that endpoint, preserving the picker and explicit thread
   shortcut. Use the existing phone state by default; allow --state-dir. Bare
   `modex` opens the manager. No CLI launcher owns or shuts down the backend.
4. On a separate build branch, add local build/install into ~/.local/bin and
   ensure the user shell can resolve it. Preserve /tmp/modex-host and the phone's
   state. Do not silently restart the active supervisor. Document direct
   service and CLI commands and the update step for an existing old host.

## Verification

Run RED/GREEN gateway and launcher regressions, full Go race/vet, existing Python
host/client tests, Unix build plus Windows client cross-build, and a real Codex
temporary-state smoke test. Use no existing session, profile or credentials in
fixtures. Review the branch diff before coherent commits; no push/merge/release
without a new request. PROJECT_STATE.md remains the single continuity snapshot.

## Review focus

- No request-ID collision between native clients or Android.
- No duplicate approval dispatch or implicit approval on disconnect.
- No backend cancellation when a client disconnects or misses a reply.
- No default public native listener, unsafe socket, or credentials in logs.
- Existing SSH attach exclusivity applies only to Android, not native clients.
