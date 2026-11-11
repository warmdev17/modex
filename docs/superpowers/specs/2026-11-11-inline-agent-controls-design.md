# Inline agent controls design

## Intent

Make active Codex work feel like one continuous mobile conversation: shell activity stays visible without dominating the transcript, model selection lives beside the prompt it affects, and blocking approvals/questions appear where the user is already watching the agent.

## Decisions

- Render command execution as a muted, compact disclosure row with a terminal glyph, one-line command, small status and a 48dp hit area. Expanded command/output remains selectable and monospace.
- Replace the composer’s generic `Codex` label with a compact model chip. The chip shows the effective or queued model, accepts a model ID, and passes it only on the next `turn/start`; Codex then applies it to that and subsequent turns. An empty selection keeps the current host/session model.
- Render pending command approvals, unsupported file-change approvals, and user-input questions inline after the transcript. Preserve the existing allowlisted decision payloads and actions. Question answers remain local until all required fields are filled and submitted.
- Keep Catppuccin Mocha tokens, native Compose controls, vector Canvas glyphs, 48dp interactive targets, selectable command/detail text, and clear disabled states.

## Approaches

- Chosen: inline activity rows plus inline decision forms. This preserves context, supports multiple pending decisions, and avoids a modal interrupt covering the transcript.
- Rejected: keep modal decisions and only restyle them. It cannot satisfy the requested in-flow inputs.
- Rejected: add a full model catalog backend now. Third-party providers may expose custom IDs; a validated free-form model ID is the smallest control that works across providers. Model discovery can follow as a separate feature.

## Non-goals

No reasoning-effort picker, file diff acceptance, secret user-input fields, MCP-specific cards, provider discovery, or changes to approval policy. The host remains the authority for available models and permissions.
