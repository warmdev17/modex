# Live steering implementation plan

**Goal** Match Codex CLI Enter for ordinary active chat turns. Keep Stop available, and use Codex's own delivery timing and transcript events.

- Add a validated Modex `turn/steer` request using the same attachment/reference handling as `turn/start`. Route it to the active thread's backend and pass the exact expected turn ID. Use the CLI's fresh-turn fallback after an explicit native no-active-turn rejection.
- Enable Send in the Android composer once the active turn ID is known. Preserve the draft until acknowledgment and render the user message from the host event. Keep Stop independent and preserve newer drafts.
- Check exact target and input, workspace isolation, completion races and the absence of retries on uncertain delivery. Build and check both host and Android.
