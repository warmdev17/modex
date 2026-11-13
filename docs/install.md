# Install prebuilt Modex

The release pipeline prepares **draft releases**. No public binary release is
implied by this guide; use these steps once an owner has published a reviewed tag.
The project's own license decision is still pending. Third-party notices are in
`docs/licenses/` and this delivery work does not choose a project license.

## Android

Download `modex-VERSION.apk` and `checksums.txt` from the same GitHub release.
Install the APK normally. Release updates use the same signing key. An existing
debug installation has a different signature and cannot upgrade directly to the
release APK. Preserve any information you need before a deliberate uninstall;
uninstalling deletes the saved SSH profile, private key and local cache. Modex
never uninstalls it automatically. Remote Codex history remains on the host.

## Linux and macOS

Configure Codex (including your provider) and an SSH server for your normal user.
No compiler, Go, Make, Python or Node is required to install the prebuilt binaries.
Download `install.sh` from a reviewed release, inspect it, then run:

```sh
sh install.sh --version v0.1.0-alpha.1
# Optional: --prefix /absolute/path/to/bin
~/.local/bin/modex --version
~/.local/bin/modex service install --workspace /absolute/path/to/project
~/.local/bin/modex service start
~/.local/bin/modex service status
```

Use an **actually published** version in place of the example. The installer
accepts Linux/macOS amd64/arm64, requires curl, tar and sha256sum or shasum, checks
the exact archive against the release checksum, and installs both sibling binaries
under `~/.local/bin`. Add that directory to PATH if needed. Checksums detect
corruption; trust also depends on the GitHub repository and HTTPS connection.
Each binary replacement is atomic, but the pair is not a filesystem transaction:
avoid launching the CLI during an upgrade. The installer does not restart a host.

DEB/RPM packages are an alternative on the matching Linux architecture:

```sh
sudo apt install ./modex_VERSION_amd64.deb
# or: sudo dnf install ./modex_VERSION_amd64.rpm
modex service install --workspace /absolute/path/to/project
modex service start
```

Packages contain `/usr/bin/modex` and `/usr/bin/modex-host`. No package hook starts
an agent, modifies SSH/firewall settings, enables linger or deletes user data.
macOS archives are not Developer ID signed/notarized; Windows executables are not
Authenticode signed. Download only trusted releases and follow your OS's security
controls; no certification is claimed.

## User service lifecycle

`modex service install` writes configuration but does not start/restart the host.
It prints the absolute binary/state paths to enter on Android. Default state:
`~/.local/state/modex`; existing phone/emulator state is not migrated. Optional
install flags: `--state-dir`, `--host-binary`, `--codex`, `--codex-home`.
Workspace/Codex paths must exist. PATH and explicit CODEX_HOME are captured; API
keys and arbitrary environment variables are not copied into the service file.
Set up provider credentials using Codex before starting; a locked desktop keyring
or shell-only secret may need manual setup in the service environment.

Linux uses a **systemd user** unit. `start` enables it for future user logins;
`stop` disables it and stops the host. On a VPS, optionally use
`loginctl enable-linger YOUR_USER` if you deliberately want it to survive logout
and start before login. This is not enabled for you. Without systemd/user bus,
run `modex-host --supervise --workspace /absolute/project --state-dir /private/state`
in your existing process manager, or use `--background` explicitly.

macOS uses a LaunchAgent in `~/Library/LaunchAgents` and the logged-in GUI domain.
`start` bootstraps it; `stop` boots it out for the current login. The installed plist
loads again at the next login; uninstall to remove that registration permanently.
Headless SSH sessions without the GUI launchd domain must use a foreground host
under their own process manager. No root LaunchDaemon is installed.

```sh
# Stopping interrupts active work. Choose an idle time.
modex service stop
modex service uninstall
# Uninstall removes registration, not state, attachments, Codex history or keys.
```

For upgrade: stop when safe, install the new binaries, then start again. For
configuration changes, stop/uninstall first, then install using the same new-format
state path. Installation checks exclusive ownership and preserves attachments; it
refuses a live owner or legacy state. Normal start/restart reuses configured state.
Crash-released advisory locking allows new-format state recovery after process
death while retaining attachments. A legacy empty `owner.lock` or unidentified
endpoint fails closed: explicitly stop the old supervisor before migration, or
choose a fresh state directory. Never delete a lock merely because attach failed.
Restart creates a new journal instance; reconnect and observe, do not replay
uncertain prompts or approvals. Native service managers restart failures with
backoff; Linux additionally limits repeated failures to five per minute.

## Windows

The Windows amd64/arm64 ZIP contains **the SSH client only** (`modex.exe`), usable
with the Windows OpenSSH client. For hosting or `modex cli`, install Codex and the
Linux binaries **inside WSL2**; use its own SSH server/network endpoint. Enable
systemd inside that WSL distribution if you want the automatic service. Windows
sshd does not automatically enter WSL. WSL discovers WSL Codex sessions, not
Windows-native Codex Desktop sessions. Native Windows host service/ACL and CLI
takeover support are not implemented.
