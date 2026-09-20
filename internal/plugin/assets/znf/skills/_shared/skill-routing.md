# Skill routing — which domain skill a task must invoke

Read by `znf:writing-plans` (to fill `_Skills:` per task), `znf:ground` (to propose the list),
and the SDD implementer prompt (to invoke before the first edit). One table, one owner.

## Signal → skill

Pick every row whose signal appears in the task's files or diff. Several rows may match.

| Signal in the task / diff | Skill |
|---|---|
| Reads or writes a Mongo document: `.find(`, `.aggregate(`, `.collection(`, `$match`/`$lookup` stages, `updateOne`/`insertMany` | `znf:mongo-data-safety` |
| SQL through a pool or query builder: `knex`, `mysql2`, `pool.query`, raw `SELECT`/`UPDATE` strings, `DESCRIBE` | `znf:sql-data-safety` |
| Declares or changes a Mongoose schema, index or model: `new Schema(`, `mongoose.model(`, `@Schema()`, `@Prop(`, `schemas/*`, a backfill | `znf:mongoose-modeling` |
| Crosses a service boundary: Redis pub/sub `publish`/`subscribe`, a BullMQ queue/job, an HTTP call between services, a change-stream event | `znf:service-integration` |
| Express controller / service / middleware / route file (`app/controller`, `app/services`, `app/routes`, `router.get(`) | `znf:express-service-patterns` |
| NestJS module / controller / provider / guard / pipe (`@Module(`, `@Controller(`, `@Injectable(`, `@InjectModel(`) | `znf:nestjs-patterns` |
| React component, hook, form or route (`.tsx`/`.jsx`, `useState`, `useQuery`, a form schema) | `znf:react-patterns` |
| Any edit inside repo X that has `X/.claude/skills/<x>-conventions/` | `<x>-conventions` (e.g. `be-conventions`) |
| None of the above (Go, shell, docs, config, kit assets) | `none` |

Write the result as one bullet in the task's Interfaces block, under `_Requirements:`:

```
- `_Skills: znf:mongo-data-safety, be-conventions_`
- `_Skills: none_`
```

`zenify analyze` flags a task with no `_Skills:` line (`missing-skills`, HIGH) and a value that is
neither `none`, `znf:<skill>` nor `<repo>-conventions` (`unknown-skill`, HIGH). Advisory.

## "Unknown skill" — the skill is not registered yet

`znf:*` skills are user-scoped and registered at session start. A `<repo>-conventions` skill lives
in the repo and registers lazily: the harness discovers `repos/<repo>/.claude/skills` on the
first Read or Edit of a **project file** under that repo (a Read of a path under `.claude/` does
not count). So, in order:

1. Read one real source file of the repo (its `package.json`, an entry point).
2. Invoke the skill. Still `Unknown skill` → Read `repos/<repo>/.claude/skills/<x>-conventions/SKILL.md`
   directly; its content is the guidance, the registration is only a shortcut.
3. Record in the task report which skills were invoked, or "no skills routed" for `none`.

## Measuring that routing works

Count skill invocations across a workspace's transcripts (adjust the project dir):

```bash
grep -rhoE '"name":"Skill","input":\{"skill":"[^"]+"' ~/.claude/projects/<project-dir> \
  | sed -E 's/.*"skill":"//; s/"$//' | sort | uniq -c | sort -rn
```

Domain-skill rows at zero after a real `/cook` means the routing above is not being followed —
fix the routing, not the skill content.
