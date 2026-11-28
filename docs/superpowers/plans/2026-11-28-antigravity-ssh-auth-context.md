# Antigravity SSH auth context implementation plan

A live A/B on AGY 1.5.17 used Gemini 4 Hydrogen Medium and the same prompt.
In the host service environment, ACP returned Individual quota reached.
With the real SSH_CONNECTION supplied by an open SSH terminal on the same
host, the prompt succeeded. The service launches ACP outside the SSH
process, so it loses a signal that affects AGY's local versus SSH path.

- [x] Carry SSH_CONNECTION from the remote attach helper in the authenticated
  private supervisor hello. Validate the OpenSSH four-field form. Pass it
  through the internal attachment callback without echoing it in public
  ready frames, results, or journal. Reserve the lifecycle command against
  input forwarded from the remote application.
- [x] Use the current attachment's context only for new Antigravity
  processes, including readiness. Keep existing ACP processes and active
  turns through detach and reconnect. Keep the inherited process
  environment where an older or local attach has no context. Never replace
  user credentials, launch a login, or automatically resend prompts.
- [x] Prove the regression with a fake ACP supervisor fixture plus a real
  OpenSSH loopback with isolated state and the installed AGY, if available.
  Verify raw address data stays private and unrelated runtimes pass. Install
  the exact tested binary and use the delayed host restart. Android remains
  compatible through the existing remote attach command.
