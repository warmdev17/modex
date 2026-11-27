# Public Documentation Design

## Intent

Turn Modex's repository front page into a clear product introduction and give
users a small, trustworthy documentation set that explains the system without
requiring them to read protocol notes or implementation plans.

## Goals

- Make English the default public language and provide an equivalent Vietnamese
  option from every public landing page.
- Lead with what Modex does, how to try it, what is supported today, and the
  local-first security boundary.
- Document architecture, connecting agent runtimes, current delivery status,
  and future direction, including the Basic/Plus/Pro Modex Cloud distinction.
- Give readers a sourced, dated map of comparable mobile/remote agent projects,
  their highlights and declared agent compatibility, including a neutral T3
  Code telemetry note that links the report, primary issue and current policy.
- Keep claims aligned with the source tree and the dev.38 release.

## Non-goals and anti-goals

- Do not redesign the product, add a documentation framework, or add runtime
  code and dependencies.
- Do not present preview builds as production-ready, claim native Windows host
  support, invent a project license, or describe future cloud/orchestration
  work as shipped.
- Do not delete the existing developer and protocol documents.
- Do not turn self-reported competitor compatibility into Modex test results or
  present a privacy controversy without the project's current policy and scope.

## Approaches

1. Keep one very long README and translate it inline. Rejected: it preserves
   the current manual-like front page and makes both languages difficult to scan.
2. Use a concise product README plus a small bilingual documentation tree.
   Selected: it follows the product-first hierarchy used by Atuin and
   NectarLink while keeping Bluetui's direct installation and usage sections.
3. Add a generated documentation website. Rejected: no public docs host or
   maintenance need justifies another toolchain.

## Structure

- `README.md`: default English product landing page.
- `README.vi.md`: equivalent Vietnamese landing page.
- `docs/README.md` and `docs/vi/README.md`: English-default and Vietnamese
  documentation indexes.
- `docs/{architecture,agents,status,roadmap}.md`: default English technical and
  product documents.
- `docs/vi/{architecture,agents,status,roadmap}.md`: Vietnamese equivalents.

The landing pages use the same section order and cross-link at the top. The
documentation pages carry visible language switches and link to the existing
detailed developer references where appropriate.

## Accuracy constraints

- Codex, OpenCode, and Antigravity support must be described by capability, not
  by a blanket parity claim.
- Host execution and provider credentials remain on the user's machine in the
  current local/SSH model.
- Android is currently a preview distributed through GitHub pre-releases.
- Native host operation is Linux/macOS; Windows use is through WSL2 for now.
- Modex Cloud and Pro cross-agent orchestration are future direction only.
