# Public Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the internal-manual README with a professional bilingual product page and add a small bilingual Modex documentation set.

**Architecture:** Static Markdown only. English is canonical at the repository root and in `docs`; Vietnamese mirrors it in `README.vi.md` and `docs/vi`, with explicit language links and shared links to existing deep technical references.

**Tech Stack:** GitHub-flavored Markdown, Mermaid.

**Spec:** `docs/superpowers/specs/2026-11-27-public-documentation-design.md`

## Global Constraints

- Preserve all existing developer/protocol documents.
- Add no dependency or generated site.
- Treat dev.38 and all cloud/orchestration work as preview or future work exactly as the spec states.
- Do not claim a project license.

## Review Focus

- Language links must resolve from both root README files and every docs index.
- Install commands must match the current scripts and `modex service` interface.
- Agent capability descriptions must not imply parity across Codex, OpenCode, and Antigravity.
- Architecture must distinguish Android, SSH, supervisor, runtime adapters, and provider-owned credentials.
- Status and roadmap must clearly separate shipped preview behavior from active and future work.

---

### Task 1: Product README pair

**Files:**
- Modify: `README.md`
- Create: `README.vi.md`

**Interfaces:**
- Consumes: current install/release commands and the design spec
- Produces: language-linked entry points into the docs tree

- [x] Replace the protocol-first README with matching English and Vietnamese product pages: hero, release/CI badges, product promise, current capabilities, download, quick start, architecture summary, support matrix, privacy boundary, docs links, roadmap preview, and preview warning.
- [x] Check every relative link and command against the current tree.
- [x] Commit both README files with the directly related docs in Task 2 so no public link lands on a missing page.

### Task 2: Bilingual user and product documentation

**Files:**
- Create: `docs/README.md`
- Create: `docs/vi/README.md`
- Create: `docs/architecture.md`
- Create: `docs/agents.md`
- Create: `docs/status.md`
- Create: `docs/roadmap.md`
- Create: `docs/vi/architecture.md`
- Create: `docs/vi/agents.md`
- Create: `docs/vi/status.md`
- Create: `docs/vi/roadmap.md`

**Interfaces:**
- Consumes: `docs/design.md`, `docs/install.md`, `docs/opencode-backend.md`, `PROJECT_STATE.md`, and current source capability declarations
- Produces: stable public links used by both README files

- [x] Write matching language indexes and four focused topic pages per language.
- [x] Use a Mermaid diagram for the current local/SSH architecture and plain tables for runtime support and status.
- [x] Link detailed install, OpenCode, protocol, mobile-development, and release documents rather than duplicating them.
- [x] Validate Markdown links, language symmetry, forbidden overclaims, and working tree scope with a small standard-library script or shell checks.
- [x] Review the final diff and commit only the documentation scope.

### Task 3: Similar-project map

**Files:**
- Modify: `README.md`
- Modify: `README.vi.md`

**Interfaces:**
- Consumes: official project repositories/documentation and the linked T3 Code telemetry report, issue, and current policy
- Produces: matching English/Vietnamese comparison sections with dated, source-attributed claims

- [x] Add a compact table of selected similar projects with highlights and declared agent compatibility.
- [x] Define the compatibility labels and state that Modex did not independently test those projects.
- [x] Add a neutral T3 Code telemetry note that distinguishes the controversy from the current documented collection, exclusions, and opt-outs.
- [x] Re-run bilingual parity, link, wording, and diff checks before committing.
