<!-- Moved verbatim from subagent-driven-development/SKILL.md § Setup (W4 slim-skills). Read when: you lost your place after compaction, are tempted to `git clean`, or are about to write "the scan is clean" without a table. -->

Conversation memory does not survive compaction. In real sessions,
controllers that lost their place have re-dispatched entire completed task
sequences — the single most expensive failure observed. Track progress in
a ledger file, not only in todos.

The ledger is your recovery map: the commits it names exist in git even
when your context no longer remembers creating them. After compaction,
trust the ledger and `git log` over your own recollection.

`git clean -fdx` will destroy the workspace (it's git-ignored scratch); if
that happens, recover from `git log`.

One row for every pair of tasks
that share a file or an interface: the two tasks, what one produces against
what the other consumes, and what you found. One row for every task: whether
its own text agrees with itself — the tests it specifies against the code it
specifies, the files it creates against the files it later touches. "The scan
is clean" without those rows is not a scan you ran.

rulings made without one are provisional.

The review loop remains the net for conflicts that only emerge from
implementation.
