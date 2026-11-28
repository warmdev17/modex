# Antigravity file diffs implementation plan

Surface completed ACP file edits in the existing Android file change card.
Consume only native diff content with before/after strings. AGY 1.5.17
provider quota blocked the real edit probe. Validate the documented format
in an isolated supervisor with synthetic ACP frames.

The [ACP tool call schema](https://agentclientprotocol.com/protocol/v1/tool-calls#diffs)
defines the diff blocks. The [AGY changelog](https://antigravity.google/docs/changelog)
documents their use for completed edits.

- [x] Decode ACP `content` blocks of type `diff` on completed
  `tool_call` and `tool_call_update`. Reject malformed path or text
  fields, limit payload sizes, and keep other tool paths independent.
- [x] Emit one `item/completed` of type `fileChange` for the current
  session and turn per native call. Reuse the same projection for
  `role: file_change` history on ACP replay. Render bounded line
  differences from the supplied before/after text. Trim shared leading
  and trailing lines and preserve explicit truncation. Never read or
  modify a file using a path from the diff.
- [x] Verify live events, history after restart, duplicate and failed
  edits, and compatibility with signed Android preview 55. Run Go race
  and vet and review before installing the tested host.
