export const meta = {
  name: 'review-changes',
  description: 'Multi-dimensional code review with adversarial verification of findings',
  phases: [
    { title: 'Review', detail: 'Fan-out across dimensions: bugs, security, perf, contracts, types' },
    { title: 'Verify', detail: 'Adversarially verify each CRITICAL/HIGH finding — 2/3 skeptics must confirm to keep it; MEDIUM is returned as advisory without LLM verify' },
    { title: 'Synthesize', detail: 'Merge confirmed findings, rank by severity, produce report' },
  ],
}

// M4d doctrine: preamble no-claim/skeptic tiêm vào mọi reviewer. Absent → '' (back-compat).
const DOCTRINE = args.doctrine ? args.doctrine + '\n\n' : ''

// W3: model scaled by risk — security/contracts carry the shared-contract + auth blast radius (opus);
// bugs/perf/types are well-served by sonnet. Verify skeptics stay opus (bounded set, see below).
const DIMENSIONS = [
  { key: 'bugs', model: 'sonnet', prompt: DOCTRINE + 'Review this diff for CORRECTNESS bugs: wrong field names in dynamic code, off-by-one errors, null/undefined propagation, race conditions, wrong async handling. Return only confirmed bugs with file:line, and for each include evidence: the exact code line content, verbatim, WITHOUT the diff +/- marker. diff:\n\n' + args.diff },
  { key: 'security', model: 'opus', prompt: DOCTRINE + 'Review this diff for SECURITY issues: OWASP Top 10, missing auth/authz, injection (SQL/NoSQL/command), secrets in code, IDOR, insecure defaults. Return only real security issues with severity, and for each include evidence: the exact code line content, verbatim, WITHOUT the diff +/- marker. diff:\n\n' + args.diff },
  { key: 'perf', model: 'sonnet', prompt: DOCTRINE + 'Review this diff for PERFORMANCE issues: N+1 queries, missing indexes, unbounded loops, large in-memory operations. Return only confirmed issues with impact estimate, and for each include evidence: the exact code line content, verbatim, WITHOUT the diff +/- marker. diff:\n\n' + args.diff },
  { key: 'contracts', model: 'opus', prompt: DOCTRINE + 'Review this diff for CONTRACT MISMATCHES: does the response shape match what callers expect? Do DB writes match what readers expect? Are queue/event payloads compatible with consumers? For each finding include evidence: the exact code line content, verbatim, WITHOUT the diff +/- marker. Context: ' + (args.context || 'no extra context') + '\n\ndiff:\n\n' + args.diff },
  { key: 'types', model: 'sonnet', prompt: DOCTRINE + 'Review this diff for TYPE SAFETY issues in dynamic code: field names used without verification, API responses used without checking shape, assumed object structures. For each finding include evidence: the exact code line content, verbatim, WITHOUT the diff +/- marker. diff:\n\n' + args.diff },
]

const FINDING_SCHEMA = {
  type: 'object',
  properties: {
    findings: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          dimension: { type: 'string' },
          severity: { type: 'string', enum: ['CRITICAL', 'HIGH', 'MEDIUM', 'LOW'] },
          title: { type: 'string' },
          file: { type: 'string' },
          line: { type: 'string' },
          issue: { type: 'string' },
          fix: { type: 'string' },
          evidence: { type: 'string', description: 'the exact code line content, verbatim, WITHOUT the diff +/- marker, matching file:line — required when file+line is set' },
        },
        required: ['dimension', 'severity', 'title', 'issue', 'fix'],
      },
    },
  },
  required: ['findings'],
}

const VERDICT_SCHEMA = {
  type: 'object',
  properties: {
    refuted: { type: 'boolean' },
    reason: { type: 'string' },
  },
  required: ['refuted', 'reason'],
}

// Fan-out across dimensions via pipeline; adversarially verify each finding as it comes in
phase('Review')
const dimResults = await pipeline(
  DIMENSIONS,
  d => agent(d.prompt, { label: `review:${d.key}`, phase: 'Review', schema: FINDING_SCHEMA, model: d.model }),
)

const reviewed = dimResults.filter(Boolean).flatMap(r => r.findings)

// Deduplicate by title+file before anything expensive
const dedupBy = list => {
  const seen = new Set()
  return list.filter(f => {
    const key = f.title + '::' + (f.file || '')
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

// W3: only CRITICAL/HIGH earn the 3-skeptic adversarial verify. MEDIUM is returned as
// `advisory` (deduped, not LLM-verified — the engine's mechanical `zenify review-verify`
// still runs on it). LOW stays `lowRisk` as before.
const deduped = dedupBy(reviewed.filter(f => f.severity === 'CRITICAL' || f.severity === 'HIGH'))
const advisory = dedupBy(reviewed.filter(f => f.severity === 'MEDIUM'))
const lowFindings = reviewed.filter(f => f.severity === 'LOW')

if (deduped.length === 0) {
  log(`No CRITICAL/HIGH findings to verify; ${advisory.length} MEDIUM returned as advisory.`)
  return { confirmed: [], advisory, lowRisk: lowFindings, shippable: true, summary: `No CRITICAL/HIGH issues found. ${advisory.length} advisory (MEDIUM).` }
}

log(`${deduped.length} CRITICAL/HIGH findings to verify adversarially (${advisory.length} MEDIUM as advisory)`)

// Adversarial verify: 3 independent skeptics; need ≥2 to confirm (not refute) to keep
phase('Verify')
const verified = await pipeline(
  deduped,
  finding => parallel([
    () => agent(DOCTRINE + `Try to REFUTE this code review finding. Default to refuted=true if uncertain.\nFinding: ${finding.title}\nIssue: ${finding.issue}\nDiff context:\n${args.diff}`, { label: `verify-1:${finding.title.slice(0, 30)}`, phase: 'Verify', schema: VERDICT_SCHEMA, model: 'opus' }),
    () => agent(DOCTRINE + `Try to REFUTE this code review finding. Default to refuted=true if uncertain. Focus on: is this actually reachable/exploitable in this codebase?\nFinding: ${finding.title}\nIssue: ${finding.issue}\nDiff context:\n${args.diff}`, { label: `verify-2:${finding.title.slice(0, 30)}`, phase: 'Verify', schema: VERDICT_SCHEMA, model: 'opus' }),
    () => agent(DOCTRINE + `Try to REFUTE this code review finding. Default to refuted=true if uncertain. Focus on: does the fix actually solve the root cause?\nFinding: ${finding.title}\nIssue: ${finding.issue}\nDiff context:\n${args.diff}`, { label: `verify-3:${finding.title.slice(0, 30)}`, phase: 'Verify', schema: VERDICT_SCHEMA, model: 'opus' }),
  ]).then(votes => {
    const confirmed = votes.filter(Boolean).filter(v => !v.refuted).length
    return { ...finding, confirmed: confirmed >= 2 }
  })
)

phase('Synthesize')
const confirmedFindings = verified.filter(Boolean).filter(f => f.confirmed)

const criticalHigh = confirmedFindings.filter(f => f.severity === 'CRITICAL' || f.severity === 'HIGH')
const shippable = criticalHigh.length === 0

return {
  confirmed: confirmedFindings,
  advisory,
  lowRisk: lowFindings,
  shippable,
  summary: `${confirmedFindings.length} confirmed issues (${criticalHigh.length} critical/high), ${advisory.length} advisory (MEDIUM, not LLM-verified). Shippable: ${shippable ? 'YES' : 'NO — fix CRITICAL/HIGH first'}.`,
}
