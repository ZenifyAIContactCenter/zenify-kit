import type { LedgerTask } from './ledger'

export type PrView = { exitCode: number; stdout: string; stderr: string }

const PR_URL = /https:\/\/github\.com\/([^/\s]+)\/([^/\s]+)\/pull\/(\d+)/g
const RED_CONCLUSIONS = ['FAILURE', 'TIMED_OUT', 'CANCELLED', 'ACTION_REQUIRED']
const RED_STATES = ['FAILURE', 'ERROR']

export function prUrls(tasks: LedgerTask[]): string[] {
  const seen: string[] = []
  for (const t of tasks) {
    for (const m of t.evidence.matchAll(PR_URL)) {
      if (!seen.includes(m[0])) seen.push(m[0])
    }
  }
  return seen
}

type Check = { name?: string; context?: string; conclusion?: string; status?: string; state?: string }

export function prLabel(view: PrView): string {
  if (view.exitCode !== 0) return `gh lỗi: ${view.stderr.split('\n')[0]}`
  let data: { state?: string; mergeStateStatus?: string; statusCheckRollup?: Check[] }
  try {
    data = JSON.parse(view.stdout)
  } catch (e) {
    return `gh lỗi: ${(e as Error).message}`
  }
  if (data.state === 'MERGED') return 'MERGED'
  if (data.state === 'CLOSED') return 'CLOSED'
  const checks = data.statusCheckRollup ?? []
  const red = checks.find(
    c => RED_CONCLUSIONS.includes(c.conclusion ?? '') || RED_STATES.includes(c.state ?? ''),
  )
  if (red) return `CI đỏ (${red.name ?? red.context})`
  if (checks.some(c => (c.status !== undefined && c.status !== 'COMPLETED') || c.state === 'PENDING')) {
    return 'CI chạy'
  }
  return data.mergeStateStatus ?? ''
}

export function isMergeable(label: string): boolean {
  return label === 'CLEAN' || label === 'UNSTABLE'
}

// A merged or closed PR is final: the pane stops showing and polling it.
export function isSettled(label: string): boolean {
  return label === 'MERGED' || label === 'CLOSED'
}

function parseUrl(url: string): { repo: string; n: string } {
  const m = /github\.com\/([^/]+\/[^/]+)\/pull\/(\d+)/.exec(url)
  return { repo: m ? m[1] : '', n: m ? m[2] : '' }
}

export function mergeCommand(url: string): string {
  const { repo, n } = parseUrl(url)
  return `gh pr merge -R ${repo} ${n} --merge`
}

export function prRef(url: string): string {
  const { repo, n } = parseUrl(url)
  return `${repo}#${n}`
}
