# Command timeline implementation plan

Goal: show individual Codex messages and compact expandable command cards, including running/completed/failed state, full command and output on demand.

Architecture: Android uses typed chat entries keyed by Codex item ID. Reduce item start/delta/completion events into one entry, preserving timeline order. Bridge history adds command entries alongside existing text messages; no execution or approval policy changes. Restore older saved text messages compatibly. Output stays plain selectable text, not executable markup.

User direction: compact command line and arrow expansion, native Mocha UI. Builder choices: collapsed by default, 48dp header touch target, status text as well as color, bounded 64KiB output per command with explicit truncation label. This is an execution timeline, not an OS process monitor.

- [x] Add failing JVM tests for separate agent IDs, command lifecycle/output, selected-thread validation, save/load compatibility and output cap; add Go history regression.
- [x] Implement typed entries/reducer and history serialization; preserve all existing callers and encrypted saved data.
- [x] Add native expandable command card with accessible toggle and plain scrollable output.
- [x] Build/JVM/lint/Go checks; emulator fixture verifies collapse/expand and history scrolling. Review final diff and record evidence in PROJECT_STATE.md; commit coherent milestone.
