<!-- Moved verbatim from fix/SKILL.md § Step 6 (token-diet). Read when: a negative result (0 hits, OK, not found) is about to let you proceed. -->

**A clean check is not yet evidence.** A positive result carries its own content. A *negative* one — "no match", "0 results", "OK", "nothing found", "no diff" — that lets you **proceed** does not, until a second independent mechanism agrees:

| The check said | Confirm it with |
|---|---|
| a repo-wide sweep found 0 hits | `rg`, never `grep -R`; plus one count against a file you know contains a hit |
| the config was applied | measure the effect (pixels, `getBoundingClientRect`, real output) — not by re-reading the config |
| a wrapper tool: "not found" / "not a repo" / "none" | run the underlying tool directly (`git worktree list`, not the wrapper's view of it) |
| a connection failed | `nc -z <host> <port>` first — separate network from credential. **Run it with the sandbox disabled and say so:** inside the sandbox `nc` and `curl` report *every* port closed. For a local port use `lsof -nP -iTCP:<port> -sTCP:LISTEN` |
