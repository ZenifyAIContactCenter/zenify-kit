<!-- Moved verbatim from ship/SKILL.md § 4. Behavioural verification (W4 slim-skills). Read when: the UI verifier stalls, logs itself out, or you doubt why the check runs here and not in /cook. -->

### From § 4a: this is the authoritative UI pass
**This is the authoritative UI pass — the last one, not the only one.** `/ship` is invoked
unconditionally by `/cook`, `/fix` and `/hotfix`, so a rendering change always reaches it without the
step being copied into three pipelines. It runs *here* because it must happen at the **final
fingerprint**: a review fix in step 5 can break layout, and a check taken before that fix certifies a
tree that no longer exists.

`/cook` step 6 also looks, once per rendering task, before writing that task's ledger line. The two do
not overlap: the per-task check tells you **which task** broke a layout, and cannot see a layout broken
three tasks later; this one catches what a *review* fix breaks, and cannot attribute it to a task.
Neither substitutes for the other, and `/fix` and `/hotfix` have no per-task equivalent, so for them
this is genuinely the only look.

### From § 4b: mechanical trigger
The agent exists for exactly this and says why in its own description: it *"keeps heavy browser output
out of the main context"*.

It renders in a pinned Docker image so the baseline is OS-independent.

### From § 4c: login handover to the verifier
Two things this avoids, both of which have actually happened: a verifier stalling for a human because
it could not authenticate, and a verifier being handed an *old* session whose screens are empty by
design, so the check verified nothing.

Snapshots, console dumps and screenshots are the largest volume any step here produces, and it returns
a verdict plus evidence instead.
