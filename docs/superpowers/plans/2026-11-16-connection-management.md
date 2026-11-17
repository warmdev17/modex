# Connection management implementation plan

Goal: Native Android Home with saved hosts, active connections and Settings, retaining simultaneous independent SSH connections.

Architecture: retain existing per-host ModexViewModel/transport, keyed by stable device-local profile IDs; persist the profile index encrypted with SavedHost. Existing host.enc remains the legacy host profile so no credentials or cursor need moving. One aggregate foreground service stays alive while any host is connecting/connected. UI selection must never route a callback to another host.

- [x] Add encrypted profile catalog and independent keyed ViewModels. Test parsing, legacy migration, names, storage separation, aggregate service lifecycle and profile deletion.
- [x] Add Home / Connections / Settings native Mocha navigation and host editor; keep existing chat navigation and back stack; per-host reconnect/reset/forget actions.
- [x] Verify JVM tests, lint, build and emulator migration/multi-host/state upgrade; commit coherent milestones.
- [ ] Publish versioned APK and source tag with unchanged certificate and increasing versionCode. No host restart for this Android-only change.

Scope: name/add/edit/delete saved hosts; explicit connect/disconnect; independent keys, cursors and pending decisions. Global Settings provides a real reconnect-on-open toggle and app version. No cloud sync, port forwarding, shared keychain or automatic command replay.

Evidence: 100 JVM tests and lint PASS. Disposable emulator upgrade 7→8 and legacy encrypted host migration PASS. Two isolated real SSH supervisors/fake provider PASS, including model isolation, one disconnect preserving the other foreground service, Home/Connections/Settings taps, opening both chats and process restore/removal. Initial UI assertions used isClickable and immediate reads; physical touches plus bounded UI waits resolved the fixture failures without product navigation changes. Self-review; no independent reviewer tool used. Physical phone and simultaneous live-provider approvals not tested.
