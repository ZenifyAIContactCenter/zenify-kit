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

const SAFE_NAME = /^[A-Za-z0-9_.-]+$/

export function mergeCommand(url: string): string | undefined {
  const argv = mergeArgv(url)
  return argv && `gh pr merge -R ${argv[4]} ${argv[5]} --merge`
}

export function prRef(url: string): string {
  const { repo, n } = parseUrl(url)
  return `${repo}#${n}`
}

// `owner/repo#N` (the ledger's issue field) -> its GitHub issue URL; anything else has none.
export function issueUrl(issue: string): string | undefined {
  const m = /^([^/\s#]+\/[^/\s#]+)#(\d+)$/.exec(issue)
  return m ? `https://github.com/${m[1]}/issues/${m[2]}` : undefined
}

// undefined when owner or repo holds a character outside SAFE_NAME or the number is not digits.
export function mergeArgv(url: string): string[] | undefined {
  const { repo, n } = parseUrl(url)
  const [owner, name] = repo.split('/')
  if (!SAFE_NAME.test(owner ?? '') || !SAFE_NAME.test(name ?? '') || !/^\d+$/.test(n)) return undefined
  return ['gh', 'pr', 'merge', '-R', repo, n, '--merge']
}

// The branch the PR merges into, from `gh pr view --json baseRefName`.
export function prBase(view: PrView): string | undefined {
  if (view.exitCode !== 0) return undefined
  try {
    return (JSON.parse(view.stdout) as { baseRefName?: string }).baseRefName
  } catch {
    return undefined
  }
}

export type Zenie = 'đang chạy' | 'pass' | 'fail' | '—'

// The `Zenie Review` check run in statusCheckRollup.
export function zenieState(view: PrView): Zenie {
  if (view.exitCode !== 0) return '—'
  let data: { statusCheckRollup?: Check[] }
  try {
    data = JSON.parse(view.stdout)
  } catch {
    return '—'
  }
  const check = (data.statusCheckRollup ?? []).find(c => (c.name ?? c.context) === 'Zenie Review')
  if (!check) return '—'
  if (check.status !== 'COMPLETED') return 'đang chạy'
  return check.conclusion === 'SUCCESS' ? 'pass' : 'fail'
}

export type Threads = { open: number; total: number }

// undefined when owner or repo holds a character that could break out of the query.
export function threadsArgv(url: string): string[] | undefined {
  const { repo, n } = parseUrl(url)
  const [owner, name] = repo.split('/')
  if (!SAFE_NAME.test(owner ?? '') || !SAFE_NAME.test(name ?? '') || !/^\d+$/.test(n)) return undefined
  const query = `query{repository(owner:"${owner}",name:"${name}"){pullRequest(number:${n}){reviewThreads(first:100){pageInfo{hasNextPage}nodes{isResolved}}}}}`
  return ['gh', 'api', 'graphql', '-f', `query=${query}`]
}

// Open and total review threads from the threadsArgv answer; undefined when unreadable
// or when there is a second page (the count would be partial).
export function threadCount(result: PrView): Threads | undefined {
  if (result.exitCode !== 0) return undefined
  try {
    const list = JSON.parse(result.stdout)?.data?.repository?.pullRequest?.reviewThreads
    if (!Array.isArray(list?.nodes) || list.pageInfo?.hasNextPage) return undefined
    return { open: list.nodes.filter((t: { isResolved?: boolean }) => !t.isResolved).length, total: list.nodes.length }
  } catch {
    return undefined
  }
}

// Why a PR may not be merged yet, or undefined when it may. Zenie must have passed.
export function notReadyReason(label: string, zenie: string, threads: Threads | undefined): string | undefined {
  if (!isMergeable(label)) return label || 'chưa poll'
  if (zenie !== 'pass') return `Zenie ${zenie}`
  if (threads === undefined) return 'thread ?'
  if (threads.open > 0) return `còn ${threads.open} thread mở`
  return undefined
}

export function isReadyToMerge(label: string, zenie: string, threads: Threads | undefined): boolean {
  return notReadyReason(label, zenie, threads) === undefined
}
