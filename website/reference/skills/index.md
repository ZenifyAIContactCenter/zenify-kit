---
title: Skill
---

# Skill

Skill `znf:*` gọi bằng `/znf:<name>`; coding skill gọi bằng `/<name>` sau `zenify skills install`.

| Tên | Mô tả |
|---|---|
| [/express-service-patterns](./express-service-patterns) | Use when writing or changing Express (legacy) backend code — controller/service layering, a central model-registry access pattern, a unified response-envelope helper, middleware, and async error handling. |
| [/mongo-data-safety](./mongo-data-safety) | Use when reading or writing MongoDB documents in a multi-tenant, schemaless codebase — covers the tenant filter every query must carry, strict:false silent-write drift, raw-driver bypass, distinct-before-branch, and plural/silent-create collection traps. |
| [/mongoose-modeling](./mongoose-modeling) | Use when designing or changing a Mongoose schema — schema/index design, lean vs populate, discriminators, and the safe order to add a new field (optional → switch writers → backfill → require). |
| [/nestjs-patterns](./nestjs-patterns) | Use when writing or changing NestJS backend code — module/controller structure, DTO + validation pipe, guards/interceptors, @InjectModel model access, DI/service layering, and consistent error envelopes. |
| [/react-patterns](./react-patterns) | Use when writing or changing React (Vite/TanStack) frontend code — component/state structure, list/table rendering gated on a total count, controlled forms, axios interceptor + timeout that exempts blob/arraybuffer downloads, ICU single-brace i18n, and env-driven feature gating. |
| [/service-integration](./service-integration) | Use when changing a cross-service contract — a Redis pub/sub payload, a BullMQ job (mind queue-name env split), or an HTTP shape between services. |
| [/sql-data-safety](./sql-data-safety) | Use when reading or writing SQL through a pooled driver or a thin query client (raw parameterized SQL, not a full ORM mapping) — covers the tenant filter every statement must carry, parameterization against injection, connection-pool and timeout limits, transaction scoping, and schema changes via reviewed migrations. |
| [/znf:analyze](./analyze) | Use to inspect a written spec+plan pair BEFORE implementing — mechanically checks requirement coverage (FR→task), leftover clarification markers, and Brief structure, then adds judgment on SC-testability, necessity, and DB-safety. |
| [/znf:brainstorming](./brainstorming) | You MUST use this before any creative work - creating features, building components, adding functionality, or modifying behavior. |
| [/znf:contract-sweep](./contract-sweep) | Sweep a changed contract across all repos to find producers/consumers and verify no drift. |
| [/znf:cook](./cook) | Full feature pipeline in one command. |
| [/znf:discipline](./discipline) | The house rules for how to work in this workspace — routing by blast radius, no fabrication, verify before claiming done, worktree isolation, memory habits, planning, git safety, grounding external facts, and bounded research fan-out. |
| [/znf:e2e](./e2e) | Use when a plan task's deliverable is a user flow that changes an entity's state — creating, editing, or deleting a ticket, deal, contact, and the like — to decide whether it needs an E2E functional journey and to author one that passes `zenify e2e lint`. |
| [/znf:explain-plan](./explain-plan) | Use when a diff adds or changes a DB query — the mandatory two-tier DB-perf gate. |
| [/znf:finishing-a-development-branch](./finishing-a-development-branch) | Use when implementation is complete, all tests pass, and you need to decide how to integrate the work |
| [/znf:fix](./fix) | Debug and fix a bug. |
| [/znf:gate](./gate) | Cross-repo contract gate for a polyrepo workspace. |
| [/znf:ground](./ground) | Verify real shapes and real values before writing code. |
| [/znf:hotfix](./hotfix) | Handle a live production bug in a polyrepo workspace — diagnose first, then decide the response with the user (revert, disable, or fix forward), and only for a forward fix create an isolated worktree branched from the repo's configured hotfix base ref (never the default feature base). |
| [/znf:onboard-project](./onboard-project) | Bootstrap or map a project so the agent has an accurate system map before doing feature work. |
| [/znf:prune-memory](./prune-memory) | Review and prune auto memory in one batch — remove task logs, dead references, rival memories, and misfiled scopes. |
| [/znf:review-changes](./review-changes) | Multi-dimensional code review with adversarial verification. |
| [/znf:review](./review) | The kit's unified review engine. |
| [/znf:run](./run) | Launch the app and produce real output from the real code path, so a change can be verified rather than asserted. |
| [/znf:scout](./scout) | Map what depends on something before you change it — the reverse question. |
| [/znf:ship](./ship) | Pre-ship gate. |
| [/znf:standards](./standards) | Use after implementing a plan — checks test-traceability: every FR/SC has a real test on disk, not just a testable-shaped SC. |
| [/znf:subagent-driven-development](./subagent-driven-development) | Use when executing implementation plans with independent tasks in the current session |
| [/znf:sweep](./sweep) | Tear down a finished task — stop its dev servers, close its terminal workspaces (when a workspace manager is present), remove its worktrees and branches, across every repo it touched. |
| [/znf:understand-codebase](./understand-codebase) | Build a structural map of an unfamiliar codebase using parallel readers. |
| [/znf:using-zenify-kit](./using-zenify-kit) | Use when starting any conversation - establishes how to find and use the znf kit skills, requiring skill invocation before ANY response including clarifying questions |
| [/znf:writing-plans](./writing-plans) | Use when you have a spec or requirements for a multi-step task, before touching code |
