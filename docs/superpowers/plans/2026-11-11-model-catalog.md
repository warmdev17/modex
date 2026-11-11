# Model catalog and reasoning controls

Goal: select Codex-discovered models and reasoning effort from a left-aligned Android composer control.
Architecture: bridge `models/list` projects Codex `model/list` with cursor pagination; Android loads pages at connect. No provider HTTP endpoint or credentials on phone. A custom ID fallback remains available. Only advertised reasoning efforts appear as colored discrete slider stops; unknown models inherit host effort. Choices apply on next turn, never resend prompts.

- [x] Bridge: model catalog projection/pagination, validated optional advertised effort string mapped to turn/start effort; Go regression tests.
- [x] Android: catalog parsing, connect pagination and isolated errors, model dropdown/custom ID, reasoning slider and left alignment; JVM and UI fixture coverage.
- [x] Verify Go/Android builds, review, update PROJECT_STATE and commit coherent milestones. No supervisor restart or provider prompt required.

Acceptance: supported effort travels with selected model; unsupported/custom model selection clears stale effort; catalog failure leaves manual entry functional. Existing thread permission behavior remains unchanged. New chat uses the same model picker; reasoning is chosen in the composer before first send.

Verified: Go suite/vet/bridge race; Android build/JVM/lint; emulator model selection + reasoning high reaches send. Real read-only catalog returns nine models. Review agent stream disconnected; self-review fallback. Physical-phone and paid inference not checked.
