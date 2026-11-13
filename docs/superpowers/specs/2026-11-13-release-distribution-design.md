# Prebuilt releases and host services

Status: proposed for bradar's review; implementation and publication have not started.
Baseline: main `0480c4b`, inspected 2026-11-13.

## Goal

An end user downloads an APK and installs Modex on their laptop/VPS without Go,
Gradle, Make, a source checkout or a compiler. Codex and the SSH server remain
explicit prerequisites on the remote machine. Credentials and agent execution
remain on that machine under the user's account.

## Current implementation

- `cmd/modex-host` already provides `--supervise`, `--attach` and authenticated
  `--stop`. SIGTERM is handled by the supervisor. Reuse these lifecycle paths.
- `cmd/modex` provides SSH attachment and the Unix registered native CLI launcher.
  Both executables must be installed together for `modex cli` to work.
- Linux/macOS supervisor state requires current-user ownership and private modes.
  Windows deliberately refuses persistent supervision pending private ACL support;
  its registered native launcher is also unsupported.
- Android has only debug signing, versionName 0.1.0 and versionCode 1. There are
  no release workflows, release tags, package definitions or selected project license.
- Existing runtime and ignored emulator configuration must remain untouched.

## Approaches

1. Recommended: GitHub Releases, GoReleaser for native archives and DEB/RPM,
   a small download installer, and native per-user service managers. This reuses
   the current Go programs and avoids requiring compilation on user machines.
2. Handwritten archive/DEB/RPM generators would remove a CI build tool but require
   maintaining package metadata, CPU mappings and upgrade/removal behavior ourselves.
3. A desktop installer application or container-only distribution adds another
   product and complicates access to the user's Codex home, projects and SSH.

The first approach is the builder's proposal, not an approved user decision yet.

## Release assets

Proposed initial version: `v0.1.0-alpha.1`, published as a prerelease after review.
Use one release tag for the APK and native assets, with version/commit exposed by
`modex --version` and `modex-host --version` for support diagnostics.

| Platform | CPUs | Assets | Host execution |
| --- | --- | --- | --- |
| Android | Supported by the existing app | Signed release APK | SSH client |
| Linux | amd64, arm64 | TAR.GZ, DEB, RPM | systemd user service; foreground/background fallback |
| macOS | amd64, arm64 | TAR.GZ | launchd LaunchAgent |
| Windows | amd64, arm64 | ZIP containing SSH client | Host/registered CLI run inside WSL2 |

Linux and macOS archives/packages contain both `modex` and `modex-host`.
Windows archives expose the supported SSH client, not a native persistent host.
WSL host discovers WSL Codex sessions; it does not transparently take over a
Windows-native Codex session. Native Windows service/ACL and takeover support
remain separate work and must not be advertised as delivered.

Release assets include SHA-256 checksums and installation/release notes. Do not
invent a license; retain the existing unresolved license decision in documentation.
macOS Developer ID signing/notarization and Windows code signing require separate
credentials and are not prerequisites for producing CLI archives. Document their
actual signing state rather than claiming platform certification.

## Installation

The POSIX installer downloads a selected release, detects Linux/macOS and CPU,
verifies checksums, and installs both sibling binaries to `~/.local/bin` by default.
It needs normal download/archive/checksum utilities, not Go/Python/Node. Unsupported
OS/CPU, missing tools, HTTP failures and checksum mismatch fail explicitly.
Linux builds use CGO disabled to avoid a distro-specific libc dependency.

Support explicit version and destination overrides. Install from a private
temporary directory, clean up on error and replace binaries atomically. Never
execute arbitrary release metadata, source the user's shell configuration, or
silently overwrite a running supervisor's state. Print PATH guidance if needed.
Do not promise every Linux environment: arbitrary Linux can use supported
binaries, but automatic service management requires systemd.

DEB/RPM install binaries into `/usr/bin`. They must not start an agent as root,
select a workspace, create SSH keys, open firewall ports or enable linger from
package hooks. Users opt into their own host service after package installation.
Package removal leaves private user state intact. Upgrades require an explicit
service restart when the user is ready to interrupt active work.

Proposed user flow:

```sh
# Download/install a release, or install its DEB/RPM.
modex --version
modex service install --workspace /absolute/path/to/project
modex service start
modex service status
modex cli
```

The service helper also provides stop and uninstall. Install configures but does
not start/restart a host or delete state. Uninstall removes service registration,
not Codex history, attachment storage or SSH identity. Require an intentional stop
before removing an active registration. Avoid a new remote protocol.

## Service configuration and lifecycle

Run `modex-host --supervise` directly under systemd/launchd. It stays in the
foreground; never put `--background` inside the service manager. Register only
the current user's service, using absolute binary, workspace and Codex paths.
Capture the installation-time PATH and explicit CODEX_HOME when supplied, so
Codex and tools do not depend on an interactive shell startup file. Do not copy
all shell environment variables or persist provider API keys in unit/plist files.
Providers depending on shell-only secrets or a locked desktop keyring require
explicit credential setup; service installation cannot silently unlock a keyring.

Default to a new private state directory `~/.local/state/modex`; allow an explicit
state path. Do not automatically migrate or reuse modex-phone/modex-emulator.
Refuse a conflicting registration or live owner instead of killing it or deleting
locks. Print the exact host binary/state paths for the Android SSH form.

- Linux: systemd user unit under the user's configuration directory. VPS users
  can explicitly enable linger if they want it to run after logout/before login.
  Do not assume systemd or a user bus exists; provide manual host instructions
  when unavailable.
- macOS: LaunchAgent in `~/Library/LaunchAgents`, bootstrapped into the current
  user's logged-in domain. It starts after login, not as a root LaunchDaemon.
  A headless SSH login without the required launchd domain needs explicit guidance.
- Windows: use WSL2 and its Linux install flow; systemd must be enabled in that
  WSL distribution for the automatic service. Document the extra SSH/network
  configuration rather than pretending Windows sshd automatically targets WSL.

Restart only on process failure with bounded backoff. A restart loses the existing
in-memory journal and creates a new instance; never replay prompts or approvals.
Service control and upgrades must state when an active turn can be interrupted.
The service does not replace sshd, pinned host fingerprints or key authentication.

Required lifecycle fix before claiming reliable reboot/crash recovery: current
`claimSupervisorState` uses an O_EXCL marker that survives a crash or reboot.
Introduce a crash-released OS advisory ownership lock for new-format state and
validate/recover its stale endpoint only under exclusive ownership. A legacy
owner marker must still fail closed until its old supervisor is explicitly
stopped; never infer that a process is dead merely because an endpoint is
temporarily unreachable. Test abrupt process death/restart and concurrent owners.
Do not achieve restart by deleting an entire state directory: attachments belong
to the user and must survive. The lock fix belongs to the service milestone.

## Android signing and versioning

Use a dedicated long-lived release keystore, provisioned privately by the owner.
CI consumes protected GitHub Actions signing secrets; never commit the keystore,
passwords or a fallback debug signing key. Missing signing configuration must fail
the release job instead of publishing an unsigned APK.

Keep the public application ID `dev.modex`; derive release versionName from the
tag and require monotonically increasing versionCode. Validate release signing
and package metadata before uploading assets. Later releases must reuse the same
key. The current locally installed debug APK has a different signature; it cannot
be updated in place with a newly signed release. Document that one-time transition
and its app-data consequences; do not automatically uninstall anybody's app.

## CI and publication

Add PR validation for packaging/service tests alongside applicable existing Go
and Android checks. Tag-driven release jobs test before building, use pinned tool
versions, package a consistent tag commit and collect artifacts before release.
Only the publish job needs contents-write permission. Do not expose signing
secrets to fork PRs or run untrusted PR code in a privileged workflow.

Publish the APK and native assets together only after all required jobs pass;
prefer preparing a draft release so a failed matrix does not expose a partially
published version. Checksum files must cover every downloadable installer/binary
asset that the install instructions reference. Publishing the first release is
a separate final step after the implementation, key setup and assets are reviewable.

## Implementation boundaries and verification

Keep coherent milestones: (1) versions/native packaging, (2) service lifecycle,
(3) installer, (4) signed Android release/CI/docs. Split commits by those concerns;
use feature branches and PRs before integration. Do not restart existing hosts,
install on the phone, or publish while preparing the implementation.

Tests should cover service command/escaping generation and private paths;
missing/invalid Codex or workspace; refusal of conflicting ownership; installer
CPU/OS detection, checksum failures, temporary cleanup and safe replacement;
package contents and version mappings; missing Android signing input. Validate
on a disposable service/state fixture, without contacting a paid model provider.
Cross-build on Linux is not macOS/Windows runtime proof: use native CI runners
and report any lifecycle/WSL device coverage still missing. Stop any emulator
started specifically for testing.

## Review required

Approve or revise this distribution design before implementation. In particular,
the proposed Windows support is explicit WSL2 hosting, and automatic services are
per-user rather than root/system-wide. The release signing identity still needs
private provisioning; it must not be pasted into chat.
