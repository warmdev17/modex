# Development from a phone

The owner can ask the agent to implement and verify changes remotely, then download a versioned APK from a GitHub pre-release. A browser download and Android's package installer work without ADB or access to the computer. Installing requires the user's confirmation; there is no silent installation.

## Delivering Android updates

1. Finish a coherent commit and run the affected tests, lint and UI regressions. Preserve unrelated working-tree changes and the normal PR rules.
2. Build an explicitly versioned APK using the established private signing identity. Verify `dev.modex`, versionName, versionCode and certificate using `scripts/verify_release_apk.py`. Use `scripts/check_release_code.py` to reject an existing release/tag or a reused versionCode.
3. Test upgrading the previous APK with `adb install -r`, without uninstalling or clearing data. The `upgrade` instrumentation preview seeds encrypted fixture state at the previous version and reads it after the upgrade. Verify the retained model, cursor and synthetic host/key, then run affected UI previews on the actual distributable APK.
4. Create a development tag, publish a GitHub pre-release with the APK, `android-version-code.txt`, checksum and verification notes, then return a direct download link. Never publish a signing key, SSH profile, state directory or test-only APK.
5. On the phone, download the APK, allow the browser to install applications if Android requests it, install the update and reopen Modex. The established signer permits an in-place update. The Android process can close during installation, but that is not a host restart.

The current owner installation uses the local development certificate. A development pre-release must retain that identity and be labelled accordingly; it is not a newly provisioned production signing channel. The local keystore stays on the computer. The production `v*` workflow separately requires protected release signing secrets and a pinned certificate. Do not create a production tag before those prerequisites exist. Development tags use `v<version>-dev.<iteration>`; the production workflow excludes `v*-dev.*` so these tags do not trigger an unconfigured production signing job.

A source tag may identify a verified feature commit; this does not merge it to main. Keep every original commit when integration is requested. A later production versionCode must exceed every earlier development release code too.

The existing remote file picker also transfers files using SFTP, and the present APK fits its 20 MiB limit. GitHub/browser delivery is the bootstrap route because the current app does not declare permission to install unknown packages. Opening an APK as an ordinary attachment is not a verified installer workflow. APKs or profiles must never be added to Git as a workaround.

## Service continuity today

Android-only changes do not need a host rebuild or service restart. SSH loss and Android process replacement are separate from the server-side Codex turn. Do not restart a working host just to distribute an APK.

The current service runs `modex-host --supervise`, which owns a persistent bridge and its Codex app-server. Stopping the supervisor closes that bridge and stops the backend. `modex service restart` delegates to systemd/launchd and can interrupt a running turn or pending question. It is not a hot reload.

The Android client currently reconnects on foreground return, but it does not implement a general background reconnect loop or automatic recovery across a new supervisor journal instance. A drain-only command would therefore not, by itself, complete the phone-only upgrade experience.

Journal retention is separate from a service failure. In the updated Android/host protocol, choose Connection settings → Reset session cursor, then Connect / reconnect to recover an expired replay without restarting the host. The host returns a fresh checkpoint; Android requires an explicitly cleared journal before accepting it, reloads the selected session, and reconciles pending approvals/questions. Unconfirmed prompts and decisions are never resent. Both ends need this protocol update: an older host can still reject cursor 0 after retention, and an older Android client cannot consume the new checkpoint. This recovery does not make upgrading/restarting the host non-disruptive.

Until those mechanisms are implemented and tested, build/test host changes in a private temporary workspace and stage the new binary without restarting the active service. Never claim that waiting an arbitrary number of seconds or observing no output proves the host is idle. Never replay an uncertain prompt or approval after a restart.

## Needed for host upgrades without lost work

Keep transport replacement separate from engine replacement:

- A persistent engine must own the Codex processes, journal, pending approvals/questions and session ownership. A replaceable gateway may reconnect to it; restarting that gateway must not cancel an engine turn.
- Updating the engine itself requires a drain: block new turns atomically, continue delivering output and user decisions, wait for all active turns and in-flight dispatches to settle, then activate the new engine. A waiting approval is active work, not an idle engine. Cancellation of an update must release the drain without terminating work.
- Android needs bounded reconnect with backoff, explicit connection state, journal-instance reconciliation and a fresh session/ownership check. Only reads and attachment handshakes may retry automatically; user prompts, approvals, answers and file mutations must not replay.
- Installing the first version of this architecture on the current runtime needs a deliberately idle activation boundary. It cannot retroactively preserve an active backend owned by the process being stopped. Keep a usable old APK/binary and report this bootstrap limit explicitly.

Acceptance requires isolated integration tests for active streaming, shell tools, pending approval/question, concurrent CLI/mobile clients, reconnect, rejected versions and failed activation. Engine PID/turn identity must survive gateway restart. Engine replacement must either wait for a verified drain or refuse; a best-effort sleep followed by kill is not acceptable. A transport reconnect can still briefly show reconnecting, even when no agent work is lost.

## Creating projects from Android

Open the sidebar and choose **New project**. The full-screen remote folder browser starts at SSH home; open a folder, type a path, or explicitly create one child with **New folder**. Choose **Use this folder**, set the project name and confirm. The project is stored in Codex's canonical project registry, so refreshing or reconnecting discovers it again.

Use the new-chat icon beside a project to start a conversation in its root. New conversation settings also offer a project picker and the host's default workspace. Project identity and the chosen root travel together; changing projects does not change the supervisor's default workspace. Model, effort and permission controls keep their existing behavior.

Both the new APK and updated host are needed. Folder operations require SFTP access for the existing SSH account. No Git repository is initialized, no existing files are replaced, and uncertain create requests are never replayed automatically. Refresh and inspect the folder/project list before retrying an uncertain creation.

The main New chat action starts outside projects in the remote account Documents/Codex folder. It does not inherit the selected conversation workspace or project. Use the square-and-pencil action beside a project for project chats. The standalone request requires an updated host; stage its binary and activate only at an idle boundary.

## Managing Android connections

The Home screen contains **Hosts**, **Activity** and **Settings**. Hosts lists encrypted saved profiles; add a host, set its SSH address/account, verified fingerprint, remote host binary and supervisor state directory, import its key, then Save host or Connect. Existing single-host installations retain their original encrypted host/key/cursor as My computer; migration does not rewrite those credentials.

Activity shows running conversations and pending questions/approvals across remotes, with waiting items first. Offline cached items explicitly require reconnecting to verify their status. Selecting a row opens that host and conversation; no prompt or answer is automatically submitted. Hosts manages saved remotes. All navigation/settings headers use a compact 56dp toolbar. Open switches to that host's chat; Android Back or All hosts returns Home without disconnecting other hosts. Chat headers include the host name. Each profile has an independent SSH transport, journal cursor, history, model/effort and pending decisions. Disconnect affects only that profile; the foreground notification Disconnect all action affects all profiles. A manually disconnected profile is not automatically reopened during that app process. Settings controls reconnect-on-foreground; after process replacement, enabled reconnect restores saved hosts without replaying prompts or decisions.

Use the host menu to edit or remove a profile. Disconnect before changing SSH fields or keys. Changing a remote target clears that profile's old history/cursor before connecting. Removing a profile deletes its saved device key/state, while remote files and Codex conversations remain. If a supervisor has restarted, disconnect that host, open its Connection settings, use Reset session cursor and reconnect; other remotes can remain online.

The aggregate foreground service remains subject to Android dataSync service limits; simultaneous remote connections do not bypass OS background restrictions. Profiles and SSH keys remain device-local, Keystore encrypted and excluded from Android backup. There is no cloud profile sync or shared keychain in this milestone.
