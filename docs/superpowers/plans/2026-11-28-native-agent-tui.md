# Native agent terminal implementation plan

The fullscreen selector opens the installed Antigravity and OpenCode
interfaces using their native session identities. Antigravity uses one
durable file token backend in the launcher, discovery and ACP. OpenCode
attaches to the existing managed server through a private credential
handoff and imports missing host provider accounts.

- [x] Validate native descriptors behind the authenticated CLI's reserved
  request IDs. Keep passwords out of the public journal and arguments.
- [x] Resolve saved native conversations and launch the vendor interfaces.
  Preserve workspace, model and thinking choices for Antigravity.
- [x] Use durable AGY token storage across fresh shells and ACP launches.
- [x] Adopt registered OpenCode native turns for Android, using existing
  scoped reconciliation and decision handling. Release watches on exit
  and CLI lease teardown.
- [x] Verify actual vendor TUIs, new and existing conversations, fresh
  shells and provider import. Run the affected regression and race checks,
  then install the verified host.
