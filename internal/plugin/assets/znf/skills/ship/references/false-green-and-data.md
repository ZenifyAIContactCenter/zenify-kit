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
The condition is written as a command on purpose: it self-disables in a project without `db_read`,
instead of relying on you to read a "skip unless the project matches" note and act on it. That note
was the earlier version of this paragraph, and a note is exactly the form that four separate attempts
today failed to make stick.

### From § 4g: the three data checks
These three assume MongoDB, `tenant_id` and `db_read`, so they are project-specific content in a
global skill. Moving them to a project-level skill was considered and rejected: it needs a
cross-file reference that can go stale — one was created and had to be repaired inside this very
file — and whether a same-named skill at project scope overrides one at user scope is not
verified. The mechanical trigger above achieves the same isolation with neither risk.
