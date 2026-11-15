# Installed plugin mentions on mobile

Extend the accepted typed references design without a second discovery endpoint or composer state store.

1. Host: project `plugin/installed` for the thread workspace into plugin references, using `plugin://<id>` identity and `@name` display marker. Omit uninstalled plugins; show policy/disabled entries as unavailable. Revalidate before native `type: mention` dispatch. Keep skills working when plugin discovery fails; report only an issue count.
2. Android: reuse draft bindings, eight-reference limit, grouped picker and native turn parameters. Add a plugin icon and generalize labels. Cover mixed skill/plugin identities and exact marker behavior.
3. Validate host negative paths, Android unit/build/lint, and an opt-in real-provider turn using an already-installed plugin. Keep the current host, saved connection and active emulator session unchanged.

No marketplace installation/removal, auth changes, arbitrary plugin URI execution, new dependency or backend restart. Current Codex documentation marks plugin marketplace mutation APIs as under development. Commit host and Android changes separately; no push or merge without user request.
