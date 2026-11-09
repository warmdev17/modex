# OpenSSH transport slice

Goal: attach to a target-owned running supervisor over SSH without moving provider credentials or token to the client. Use installed OpenSSH, not a new SSH dependency. Existing accepted transport plan selected the system executable first.

## Contract and scope

- `cmd/modex`: flags for target, absolute POSIX remote host/state paths, optional port/identity/known-hosts, supervisor cursor/instance and OpenSSH binary. No automatic deployment or supervisor startup.
- `internal/sshtransport`: validate destination/options and construct strict noninteractive no-PTY SSH arguments. Quote every dynamic remote shell value. Host-key verification stays strict; cursor requires matching explicit instance.
- JSON stdin/stdout passes through OpenSSH to `modex-host --attach`. No automatic reconnect or prompt resend. Generic stderr error only; use ordinary terminal SSH for diagnostics.
- Client builds on Linux/Windows/macOS; remote shell contract POSIX only. Windows supervisor targets and Android native SSH remain separate tasks.

## Verification

- Tests observed RED before implementation; GREEN for option/target validation, injection-resistant quoting, strict host key and authentication flags, cursor requirements and optional identity/port/known-hosts.
- Actual temporary sshd with ephemeral keys: exact JSON round-trip and metacharacter paths PASS; mismatched known host key rejected.
- Same test drives real supervisor with fake provider via SSH twice, preserving instance/cursor and confirming target status; stop and lock cleanup PASS.
- No system SSH config, user authorized_keys or actual server deployments changed. No service installed or keys committed.
