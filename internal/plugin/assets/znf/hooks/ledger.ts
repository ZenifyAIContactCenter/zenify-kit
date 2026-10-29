import type { Ledger, LedgerTask } from '../types'

export type { Ledger, LedgerTask }

const DATE_HEADING = /^## (\d{4}-\d{2}-\d{2})\s*$/
const DEFERRED = /^deferred until (\d{4}-\d{2}-\d{2})/
const NEEDS = /^Needs you:\s*(.*)$/
const KNOWN_LABEL = /^[A-Z][A-Za-z ]*:|^Batch \d/

function parseTask(line: string): LedgerTask | null {
  const f = line.split(' · ').map(s => s.trim())
  // 10 fields carry a priority after the id; the legacy 9-field line has none.
  if (f.length !== 9 && f.length !== 10) return null
  if (f.length === 9) f.splice(1, 0, '-')
  const until = DEFERRED.exec(f[8])?.[1]
  return {
    id: f[0],
    priority: f[1],
    repo: f[2],
    source: f[3],
    issue: f[4],
    size: f[5],
    skill: f[6],
    worker: f[7],
    status: f[8].split(/\s+/)[0],
    ...(until && { until }),
    evidence: f[9],
  }
}

export function parseLedger(text: string): Ledger {
  const lines = text.split('\n')
  let start = -1
  let date: string | null = null
  lines.forEach((line, i) => {
    const m = DATE_HEADING.exec(line)
    if (m) {
      start = i
      date = m[1]
    }
  })
  const out: Ledger = { date, tasks: [], needs: [], malformed: [] }
  if (start < 0) return out

  const asked: { id: string; question: string }[] = []
  for (const raw of lines.slice(start + 1)) {
    const line = raw.trim()
    if (!line || line.startsWith('#') || line.startsWith('<!--')) continue
    const task = parseTask(line)
    if (task) {
      out.tasks.push(task)
      continue
    }
    const n = NEEDS.exec(line)
    if (n) {
      const i = n[1].indexOf(' — ')
      asked.push(
        i < 0
          ? { id: '', question: n[1].trim() }
          : { id: n[1].slice(0, i).trim(), question: n[1].slice(i + 3).trim() },
      )
      continue
    }
    if (KNOWN_LABEL.test(line)) continue
    out.malformed.push(line)
  }
  const blocked = out.tasks
    .filter(t => t.status === 'blocked')
    .map(t => ({ id: t.id, question: t.evidence }))
  out.needs = [...asked, ...blocked]
  return out
}
