# Compact mode label and matching Send color

Accepted: move mode control outside the input border, directly above it, left aligned; small label without filled background or border, 48dp minimum touch target. Default label/Send blue #89b4fa; Plan label/Send pink #f5c2e7 with dark Send glyph. Keep acknowledged mode/uncertainty semantics. Pending changes disabled, Stop/Stopping remains red and disabled Send neutral. Header/context/host identity stay unchanged. No backend/API change or host restart.

1. Add real-touch UI regression for Send colors and mode bounds outside composer, observe RED on incumbent Plan blue/inside border.
2. Add reusable Mocha.Pink token, relocate mode label to composer wrapper outside Surface and switch Send color from acknowledged mode. Preserve bounded reference panel and scrolling fix. Reuse modeControl/handlers.
3. Run unit/lint/release+test builds; controls/pending/failure/Stop/reference+IME checks, narrow screen/large text visual checks; independent review. Same-signer increasing-code APK, exact encrypted in-place upgrade, source tag and public hashes. PR/merge preserves commit; primary owns PROJECT_STATE. No host install/restart.

Narrow/large-font verification ruling: thumbnail previews could consume the reference list and shrink Send. While IME is open use compact filename chips with the same preview/remove callbacks; temporarily hide previews during reference selection without modifying attachments. Verify actual clickable Send and a scrolled reference choice retain48dp.
