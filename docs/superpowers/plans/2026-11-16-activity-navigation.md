# Compact navigation and Activity

User requirements: keep three distinct tabs; remove oversized headers from Home and host settings; persist this as a project UI rule. Builder choice: Hosts / Activity / Settings. Activity groups running conversations and pending approvals/questions across saved remotes; waiting rows come first and cached offline state is explicitly labelled. It opens the exact host/thread, without disconnecting other transports or automatically answering anything. It does not fabricate completed-work history from a host list.

- Implement and test pure activity aggregation (deduplication, waiting priority, unscoped decisions, stable cross-host identity).
- Replace duplicate Connections UI with compact Activity list. Use a single 56dp app bar and titleMedium on Home and host settings; remove repeated brand/subtitle and oversized form intro.
- Test UI row routing, compact toolbar bounds, empty state and live two-host navigation. Build/lint/JVM tests, upgrade exact APK preserving encrypted state and established signer.
- Commit separately from instruction changes; publish Android development APK/tag. No host restart.
