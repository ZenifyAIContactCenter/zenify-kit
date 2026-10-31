# Output contract — worker and verifier

Both formats are fixed so that the verifier can parse the worker file mechanically and the lead
can build the findings table without re-reading pages.

## Worker file (`worker-<n>.md`)

```
# <sub-question, one line>

Done when: <criterion copied from the prompt>

## Findings

According to https://example.org/docs/config: "The `timeout` option defaults to 30 seconds and
applies per request."
→ The library's default request timeout is 30 s.

According to https://example.org/changelog#v4: "v4.0 removed the `legacyMode` flag."
→ `legacyMode` no longer exists from v4.

## Not found

not found: pricing per seat — tried "example pricing", https://example.org/pricing (403)

## Calls used: 9/12
```

The Step 0 worker (`worker-0.md`) uses the same format; a vendor with nothing relevant is a gap
line that names the keywords searched: `not found: <vendor> feature for <keywords> — tried <llms.txt URL>, <changelog URL>`.

Rules: one blank line between blocks; the quote is copied verbatim, at most three sentences;
the claim is one sentence; a gap always names what was tried.

## First-party worker — extra brief lines (Step 0)

```
For each vendor: fetch its docs index (`<docs-root>/llms.txt`, else the sitemap) and its
changelog / release notes for the last 6 months. Search both for each keyword. Every page the
index lists as relevant to a keyword must be fetched — never left unfetched.
Vendors: <list> · Keywords: <list>
Per vendor, record what its own feature covers (with quote) or "none found — keywords: <list>".
Cap reached → one `unfetched: <URL> — <keyword>` line per relevant page left unfetched
(never a `not found` line: the lead dispatches a follow-up on `unfetched:` only).
```

## Verifier file (`verify.md`)

```
worker-1.md:1 | https://example.org/docs/config | VERIFIED
worker-1.md:2 | https://example.org/changelog#v4 | QUOTE-MISMATCH
worker-2.md:1 | https://example.net/blog/post | DEAD-URL

totals: VERIFIED 1 · QUOTE-MISMATCH 1 · DEAD-URL 1 · NOT-FETCHED 0
```

Status meanings: `VERIFIED` quote found on the fetched page (verbatim, or equal after collapsing
whitespace and punctuation) · `QUOTE-MISMATCH` page fetched, quote absent · `DEAD-URL` fetch
failed · `NOT-FETCHED` verifier ran out of cap or time.

## Worker return (at most 40 lines)

```
claims: 6 · not found: 1 · file: /tmp/znf-research-<slug>/worker-1.md
1. <claim> — <URL>
2. <claim> — <URL>
3. <claim> — <URL>
```

## Lead return (at most 40 lines)

```
file: docs/research/<repo>/2026-09-20-<slug>.md
question: <sentence> · done when: <criterion>
first-party: <vendor: what its own feature covers> | none found — keywords: <list> | N/A — <why>
sub-questions: <n> · workers: <n> sonnet (incl. worker-0, worker-0b) + 1–2 haiku verifiers · est. <USD>
verified 11 · flagged 2 (1 QUOTE-MISMATCH, 1 DEAD-URL) · not found 3
conclusion:
- <line>
- <line>
```
