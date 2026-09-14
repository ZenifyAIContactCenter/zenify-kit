---
title: agent ui-verifier
---

# Agent `ui-verifier`

Generic (project-agnostic) UI verifier — drives a running FE app through the Playwright MCP browser to verify a UI change BOTH functionally AND visually, then returns ONLY a concise verdict + evidence. Use for any project with a frontend when a change renders something (screen, component, modal, layout). Keeps heavy browser output out of the main context and can run in the background while the main session does non-browser work. CAVEAT — the Playwright browser is a single shared instance: never run two browser-driving agents at once, and the main session must not touch Playwright while this agent runs.

**Model:** `sonnet`

## Nguồn

Sinh bởi `zenify docs gen` từ frontmatter agent trong binary.
