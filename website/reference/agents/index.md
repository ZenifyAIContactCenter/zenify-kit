---
title: Agent
---

# Agent

Agent được skill dispatch qua Agent tool; bạn không gọi trực tiếp.

| Tên | Mô tả |
|---|---|
| [code-reviewer](./code-reviewer) | Independent code reviewer with a fresh context — no memory of writing the code. |
| [scout](./scout) | Discovery agent for the reverse question — given something you are about to change, find what depends on it. |
| [ui-verifier](./ui-verifier) | Generic (project-agnostic) UI verifier — drives a running FE app through the Playwright MCP browser to verify a UI change BOTH functionally AND visually, then returns ONLY a concise verdict + evidence. |
