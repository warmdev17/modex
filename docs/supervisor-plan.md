# Background supervisor design and implementation plan

User authorized inline implementation and delegated routine design choices. Reuse the existing stdio bridge as a child process: the supervisor keeps its stdin open across clients. Never replay submitted commands or auto-restart Codex after uncertain failure.

## Contract

- `--background --workspace PATH --state-dir PATH`: launch detached supervisor and wait for readiness. Supervisor launches this executable with `--serve` and preserves explicit workspace/Codex/home.
- `--supervise`: foreground supervisor process for service managers; same state-directory contract.
- `--attach --state-dir PATH --cursor N`: authenticate to loopback endpoint from private state file; stream journal entries with monotonic `seq` and original bridge frame as `message`. Send ordinary Modex JSON lines unchanged. One active controlling connection, reject a second.
- `--stop --state-dir PATH`: authenticated graceful stop; never kill a PID found in a file.
- Supervisor uses a random token, private directory/file (0700/0600 on Unix), exclusive owner marker, loopback TCP only, authentication timeout, 4 MiB frame cap, bounded 1024-event/16 MiB memory journal. Refuse unsafe state directories. Windows private state requires explicit OS ACL validation before advertising supported detached mode.
- Cursor before retention window gets explicit gap error. Cursor ahead of journal gets rejected. No silent data loss or fabricated exactly-once transport. Client transport loss after submission is an uncertain outcome: observe journal, never resend automatically.
- No on-disk transcript, provider keys, raw stderr or crash restart. Reconnect replay includes all bridge results/events within retention. Process restart resets journal and invalidates endpoint/token; old cursors must not be reused across supervisor instances.

## Tasks

1. Journal + authenticated reconnect server: write failing tests for replay, gaps, auth, competing clients and child lifetime; implement bounded journal and supervisor control transport.
2. CLI and detached launch: route mutually exclusive modes; preserve existing serve/diagnostic; private state directory, exclusive ownership, readiness and cleanup; tests on fake child, no destructive stale-lock recovery.
3. Verify disconnect/reattach with fake child and real provider turn; run Go tests/race/vet/cross-builds. Document limitations; commit coherent tests/lifecycle implementation separately from live evidence.

## Review focus

Slow/disconnected client must not block child output; invalid auth cannot read journal or write commands; shutdown removes only owner state; duplicate supervisor cannot replace a live endpoint; bounded replay reports gaps; stderr never forwarded. No subagents per user instruction.

## Completed evidence

- Journal/server/state tests passed under `go test -race ./...`; vet passed.
- Linux/macOS/Windows builds passed; Windows supervisor modes fail closed pending ACL implementation.
- Linux black-box fake-process tests passed for detached startup, auth, exclusive attachment, offline completion replay, CLI EOF detach, duplicate ownership, graceful stop, app-server reaping and startup-failure cleanup.
- Paid `third-party / gpt-6.1-sol` test passed: one submitted turn, client detached, same-instance cursor reconnect, exact streamed reply and completion, stop/state cleanup. No prompt replay.
- Linux runtime only. No auto-restart/boot service, durable disk journal or Windows ACL support. Self-review only; no subagents per user instruction.
