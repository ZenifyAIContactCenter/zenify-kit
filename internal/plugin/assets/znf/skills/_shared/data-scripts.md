# Data scripts — shared reference

A data script reads or changes records in a shared database: a backfill, a repair, a one-off
export. The agent writes it, dry-runs it, and runs it through `zenify data run`. The command hands
the script credentials from the broker, so nobody pastes a credential and nobody has to run
anything by hand. The credentials are long-lived and held only in the child process's memory; do
not print them.

Hard rules:

- never ask the user to run a script, a `!` command or anything else on your behalf.
- never ask the user to paste a credential, and never put one in a script, a prompt or a file.
- never read .zenify/.env — it holds personal keys, not data credentials. Do not use an app .env
  for credentials either; the broker is the only source.

## Writing a data script

Save it under `docs/scripts/<repo>/` (`<repo>` = the repo whose data it changes). The write log
lands next to it.

1. **`--dry-run` writes nothing.** Without the flag the script writes; with it the script only
   reads, prints counts, and prints a small sample of the records it would change. The dry-run
   must also be safe when the credentials it receives could write.
2. **Never swallow a database error.** A write that fails during a dry-run must make the script
   exit non-zero. A `try/catch` that logs and carries on turns a broken dry-run into a green one.
3. **Before-image first.** Before changing a record, append its full previous state to a JSONL
   file (one record per line, next to the script). That file is the way back.
4. **`--restore <file>`.** The script accepts a before-image JSONL and puts every record back
   exactly as it was. Write and dry-run this mode too; an untested restore is not a rollback.
5. **Tenant filter.** On a multi-tenant collection or table every query and every update carries
   the tenant filter, taken from an argument, never hard-coded and never "all tenants" by default.
6. **Paginate by `_id`.** Read in batches ordered by `_id` and resume from the last one
   (`_id > last`); never `skip` over a large set, and never load a whole collection into memory.
7. **Read credentials from the environment only**, by these names: `MONGO_URL`, `MYSQL_URL`,
   `REDIS_URL`. Do not accept a connection string as an argument and do not print one.
8. **Write the command down.** The `--write` run must be the same command minus --dry-run: same
   interpreter, same script path, same arguments, same working directory. If the dry-run needs
   an extra argument the write does not, the two commands differ and the gate refuses the write.
9. **Name the script file directly in argv** (`node docs/scripts/<repo>/x.js`), never through
   `sh -c` or `npm run`: the gate hashes only files named in argv, so indirection hides the
   script from it.
10. **Keep outputs out of argv.** A file named in argv that the dry-run creates or changes
    changes the key, and the `--write` run then refuses. Write outputs to a path the command
    line does not name.
11. **A prd sample shows ids and counts only**, never customer PII fields (names, phones,
    emails, message text). Output masking covers credentials only, not PII.

## Running it

```bash
zenify data run --stg -- node docs/scripts/<repo>/fix-tags.js --tenant T --dry-run
zenify data run --stg --write -- node docs/scripts/<repo>/fix-tags.js --tenant T
```

- One of `--stg` or `--prd` is required. `--write` asks for write credentials. `--lumi` adds the
  second service's variables (`LUMI_MYSQL_URL`, `QDRANT_URL`, `LUMI_APP_URL`).
- The command needs a broker and network access, so run it with the sandbox **off**
  (`dangerouslyDisableSandbox`); inside the sandbox it fails to connect, and that failure is not
  a reason to look for another way in.
- The child process gets `MONGO_URL`, `MYSQL_URL`, `REDIS_URL` (and the `--lumi` names). Any copy
  of those names inherited from the shell, and every `*_PRD` variable, is stripped first.
- Output is masked: credential values and hosts are replaced before they reach you. Still do not
  print `process.env` or a connection string.
- The child's exit code is the command's exit code. Errors from the command itself are printed as
  `error: data run: …`.
- A `--write` run logs the masked output to `docs/scripts/<repo>/<script>.<UTC>.<env>.write.log`
  when the script is under `docs/scripts/<repo>/`; otherwise under `.zenify/data-runs/`. Keep the
  log with the script and quote the counts from it, not from memory.

### Order of work

1. Write the script and a small local check of its pure logic (the filter it builds, the
   batching) if there is any.
2. Dry-run on `--stg`. Read the counts and the sample; check they match what the task claims.
3. `--write` on `--stg`. Re-run the dry-run: it should now report zero remaining.
4. Only then, and only if the task needs it, the production sequence below.

## The prd gate

Production data is the user's call, not the agent's.

1. Dry-run with `--prd`. Do not add `--write`.
2. Report the dry-run summary to the user: how many records, which tenants, the sample, the
   before-image path and the exact `--write` command you will run.
3. **Wait for the user's `"ok"`.** Silence, a thumbs-up on something else, or "looks fine" about
   the plan is not an `"ok"` for this command. If the user changes the scope, dry-run again.
4. Run the `--prd --write` command unchanged, then report the counts from the write log and the
   path of the before-image file.

## When the command refuses

| You see | Meaning | What to do |
|---|---|---|
| `prd needs role lead (yours: dev)`, exit 1 (broker code `lead_only`) | Production credentials are for role lead only | Stop. Tell the user; a lead runs the production step, or the user changes the plan. Do not hunt for other credentials. |
| exit 7 (`--write needs a dry-run of the same command first`) | No successful dry-run of this exact command for this env (and the same `--lumi` setting) in the last 60 minutes, or it was already used | Run the dry-run the message prints, then the write. A write consumes its dry-run: another write needs another dry-run. |
| `broker_misconfigured` | The operator has not configured write credentials for that env | Report it to the user as an operator task. Do not try another path. |
| `broker chưa hỗ trợ /creds — cần nâng cấp broker`, exit 1 | The broker answered 404: it is older than `zenify data run` and has no `/creds` endpoint | Report it to the user as an operator task (upgrade the broker). Do not fall back to a credential file. |
| `data run needs a broker` | The workspace manifest declares no broker | Report it; do not fall back to a credential file. |
| any other non-zero exit | The script itself failed | Read the masked output, fix the script, dry-run again. |

## Limits

- The dry-run gate is a trip-wire against mistakes, not a security boundary. It stops "wrote
  without looking"; it does not stop a determined caller, so it never replaces the user's `"ok"`
  on production.
- The matching key covers the argv (minus the exact token `--dry-run`), the working directory and
  the content of the script files named in it. It does **not** cover modules the script imports,
  stdin, or other environment. Edit an imported helper after the dry-run and the write still
  passes; dry-run again yourself.
- Masking skips strings shorter than 6 characters, so a short password or token can appear in
  clear text. Keep them out of output.
- With `--lumi` the second service's credentials are the same in the dry-run and the write; there
  is no read-only variant. The script's own `--dry-run` is the only thing keeping it from writing,
  so check that path before the first run.
- Dry-run records last 60 minutes and are single-use.

See also `evidence-sources.md` for reading logs and metrics, and `knowledge-doctrine.md` for where
files go.
