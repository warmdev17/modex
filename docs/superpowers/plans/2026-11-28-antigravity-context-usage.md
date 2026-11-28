# Antigravity context usage

Show the native context window in the existing Android header and Session status. AGY 1.5.17 emitted a `usage_update` for 0 / 1,048,576 followed by 15,709 / 1,048,576 in a synthetic ACP prompt.

- [x] Host advertises `antigravity_context_usage` in the Codex supervisor hello. Parse only numeric `used` and `size` from native `usage_update`, require `used >= 0` and `size > 0`. Scope updates to the known session and workspace. Use existing `thread/tokenUsage/updated` with `threadId`, `turnId` when known and `tokenUsage: {last:{totalTokens:used},modelContextWindow:size}`. Deduplicate repeated samples. Return the latest sample as `token_usage` from `threads/status`. Reset context when the model changes or when the native session is retired. A cold host may need a new ACP sample.
- [x] Android requests read-only session status for AGY when the hello advertises the feature. Use live values only for the open AGY thread and protect them from a stale refresh response. Show context with the existing header and status dialog. Keep the AGY dialog focused on context and workspace, with no Codex quota or policy rows.
- [x] Verify isolation and invalid samples on Go fixtures, test real ACP through a temporary supervisor, verify Android UI and signed upgrade from preview 54 to 55, and stage the private APK in the main checkout.
