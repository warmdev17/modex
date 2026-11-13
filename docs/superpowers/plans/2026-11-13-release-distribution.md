# Release distribution implementation

Approved spec: ../specs/2026-11-13-release-distribution-design.md. Implement inline; PROJECT_STATE.md is the single progress tracker. Work on feat/release-distribution, do not deploy, publish or change existing service state. Keep original design commit and separate coherent implementation commits.

1. Native delivery: internal/version shared by both command entrypoints; GoReleaser archives and DEB/RPM with matching names consumed by installer. Check version output, Linux/macOS/Windows cross-builds and package configuration.
2. Service lifecycle: crash-released flock on a persistent private lock inode, legacy owner.lock refusal, recover endpoint only under lock. subprocess death/exclusivity tests. modex service install/start/status/stop/uninstall with systemd user or launchd, exact argv quoting, private state, explicit lifecycle. Test generated configuration and refusal paths; preserve live supervisors.
3. POSIX installer: explicit version, OS/CPU detection, private temp area, HTTPS download, exact SHA256 verification, stage both binaries before atomic per-file replacement, no service restart. Fake-network tests for success, corruption and unsupported platform; shell syntax check.
4. Android and CI: opt-in release signing with required inputs, monotonic tag build version code, verified APK signature/metadata, PR verification and tag draft release assembling all assets with checksums. Document key provisioning and platform limits; test unsigned release rejection, normal debug checks, dry-run packaging where tooling permits. No actual release/tag/key provisioning.

Final checks: Go race/vet, installer unit tests, Android tests/debug/lint and signed release via disposable test-only key if possible, native cross-builds, workflow/config validation. Report native-platform runtime and hosted-CI gaps. Author review when specialist tools unavailable.

Implementation complete locally across native packaging, user service/ownership, installer and signed-release/CI milestones. Review correction: shared ownership permits safe reinstall into stopped new-format state; regression verified RED/GREEN. Current verification and limitations are recorded only in PROJECT_STATE.md. Production keys, hosted workflow runs, native macOS/Windows lifecycle and publication are separate steps; no live installation performed.
