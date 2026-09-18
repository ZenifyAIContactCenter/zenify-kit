<!-- Moved verbatim from ship/SKILL.md § 4. Behavioural verification (W4 slim-skills). Read when: a negative result ("no match", "OK", 0 tests) is about to let you proceed, or you wonder why the three data checks are global. -->

### From § 4d: keep the output you keep small, but keep it real
An agent would hand back its *summary* of the output, and step 2 exists precisely to look at the
output; moving the evidence one hop away to save context trades the wrong thing.

### From § 4e: a clean check is not yet evidence
This targets one asymmetry that keeps producing false greens, and it has a documented instance
outside this project: a CI provider's instrumentation returned `null` instead of a file, the null
"appeared to be a valid response", the pipeline **registered the failures as successes**, no alert
fired, and it was caught 3.5 hours later by someone reading a dashboard.

### From § 4f: trigger the data checks mechanically
The condition is written as a command on purpose: it self-disables in a project without `zenify db-read`,
instead of relying on you to read a "skip unless the project matches" note and act on it. That note
was the earlier version of this paragraph, and a note is exactly the form that four separate attempts
today failed to make stick.

### From § 4g: the three data checks
These three assume MongoDB, `tenant_id` and `zenify db-read`, so they are project-specific content in a
global skill. Moving them to a project-level skill was considered and rejected: it needs a
cross-file reference that can go stale — one was created and had to be repaired inside this very
file — and whether a same-named skill at project scope overrides one at user scope is not
verified. The mechanical trigger above achieves the same isolation with neither risk.

### The confirmation table (moved verbatim from § 4e)
A positive result carries its own content — "FAIL", "error", "3 matches" means something standing
alone. A *negative* one — "no match", "0 results", "OK", "nothing found", "no diff" — that lets you
**proceed** does not, until a second independent mechanism agrees:

| The check said | Confirm it with |
|---|---|
| a repo-wide sweep found 0 hits | `rg`, never `grep -R`; plus one count against a file known to contain a hit |
| the setting/config was applied | measure the effect (pixels, `getBoundingClientRect`, real output) — not by re-reading the config |
| a wrapper tool: "not found" / "none" / "not a repo" | run the underlying tool directly |
| a connection failed | `nc -z <host> <port>` first — separate network from credential before theorising. **Sandbox disabled, and say so:** inside it `nc`/`curl` call every port closed. Local port → `lsof -nP -iTCP:<port> -sTCP:LISTEN` |

### Two of the three data checks (moved from § 4)
- **A tenant-scoped query → assert the negative**: run tenant B's context against a tenant-A row and
  expect **zero rows**. A query that returns the right rows for the right tenant proves nothing about
  the wrong one.
- **Pagination → not `skip`/`OFFSET` at depth**; keyset pagination (`WHERE id > last_seen`) stays flat
  as the offset grows, `skip` does not.

### The mechanical trigger for the three data checks (moved verbatim from § 4)
Both commands true → run the three checks; either false → say which, and which the diff could not
trigger.

```bash
command -v zenify >/dev/null || echo "no zenify db-read on PATH — these three do not apply here"
git diff HEAD | rg -c '\.find\(|\.aggregate\(|\.skip\(|OFFSET|findOne\(|updateMany\('
```
