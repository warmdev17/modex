# Roadmap

[English](roadmap.md) · [Tiếng Việt](vi/roadmap.md)

The near-term work is to improve the Android and host preview, close runtime-specific gaps, and make supported behavior clearer. The exact sequence can change as development continues; consult [current status](status.md) for shipped scope.

## Future direction

These are not available features or final commercial commitments:

| Direction | Intended boundary |
| --- | --- |
| **Basic** | A relay to one user's own computer without manual VPN or router port forwarding, one active agent, limited phone uploads, and basic push notifications. The user's computer remains the execution host and the user brings the agent account. |
| **Plus** | The same user-owned execution model with up to three connected hosts, up to five independent active agents, larger upload limits, and richer notifications. It does not include automated cross-agent coordination. |
| **Pro** | A Modex-managed execution machine so the personal computer can be off, plus cross-provider agent orchestration for roles such as planning, implementation, review, and deployment. |

The current local/SSH product remains useful without Modex Cloud. Basic and Plus are relay concepts, not hosted agent execution. Pro is the managed-execution concept. Push notifications are part of this future cloud direction; the current preview does not depend on FCM for ordinary foreground SSH use.

Pricing, billing, quotas, provider-account handling, and commercial terms have not been decided. See the [design overview](design.md) for the current product direction.
