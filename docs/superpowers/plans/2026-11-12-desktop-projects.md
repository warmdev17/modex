# Desktop project membership correction

Projects must come from explicit Codex Desktop project metadata, never infer a project from every session cwd. Keep Recents unchanged. Prefer app-server project/list if exposed; otherwise read only the relevant Desktop state fields with bounds and projection, honoring CODEX_HOME. Include registered empty projects and explicit projectless exclusions. On missing/unsupported Desktop metadata, show no fabricated Projects, leave all chats accessible through Recents. Do not edit Desktop files or discard sessions.

- [x] Host project metadata projection with parser tests and live read-only check.
- [x] Android registered project grouping, keep standalone chats only in Recents; tests/build.
- [x] Update state and commit verified milestones, no supervisor restart.

Verified: full Go suite/vet/bridge race, real read-only project and membership probe, Android build/unit tests/lint, emulator chat-ui fixture and screenshot. No physical-phone validation or supervisor restart. Host milestones 13b3bfb/7f18b5f; Android milestone recorded by Git.
