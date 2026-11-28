# Fast project-scoped CLI picker

User objective: faster agent/session selection, sessions only from the folder
running `modex cli`, recognizable agent icons and a polished terminal UI.

Keep the current two-step keyboard picker and native vendor TUI launch.
Use current dependencies and palette. New CLI-only readiness checks must
validate runtime compatibility/auth exactly as existing discovery. Codex can
be ready from its initialized backend; other agents remain Checking until
their own probe finishes. Slow discovery must not block selecting Codex or
loading its sessions. No synthetic ready status based on executable alone.

## Tasks

- [ ] Backend owner: private `cli/agents/check` accepts `{workspace,agent}`
  and returns `{data:[one verified agent]}`. Bound concurrency to three
  discovery requests, isolate from serialized actions, reject public clients.
  Reuse probes, make AGY respect selected canonical workspace/request context.
  Allow `workspace` on per-agent `threads/list` with canonical scoping and
  preserve Android `all_workspaces` behavior. Add regression tests first.
- [ ] Primary: initial fullscreen rows show Codex ready and others Checking;
  per-agent checks update independently. No change in native launch paths.
  CLI list explicitly requests its canonical cwd and no all-workspace flag.
  Defensively filter foreign workspace rows. Cache pages for this picker
  invocation with explicit Refresh, retry page errors in place, preserve
  cancellation and stale-result guards. TTY icons use portable terminal
  glyphs, with ASCII fallback for dumb/non-UTF8 terminals, always named labels.
  Show project identity, selected-agent details, ready/checking/unavailable,
  new-chat and empty states with consistent focus/footer spacing.
- [ ] Independent review and verification: Go race/vet, controlled delayed
  readiness proving Codex/session selection doesn't wait for AGY, scope
  tests with mixed roots/cursors/symlink cwd, real PTY keyboard/resize/NO_COLOR
  and native launch. Measure before/after using same fixture and real binary.
  Install exact tested host and schedule delayed restart only at final step.

Checks must prove project filtering for all three agents, not only Codex.
No Android/APK change, no unsolicited PR/merge/release.
