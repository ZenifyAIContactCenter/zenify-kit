import { atom, read, update } from 'claude-code'
import type { EngineInterface as $, Register, Timer } from 'claude-code'

import { logLine, slugFromPrompt, taskFromPrompt } from './durations'
import { parseLedger } from './ledger'
import type { Ledger } from './ledger'
import { parseStats, statsLines } from './stats'
import type { NonstopStats } from './stats'
import {
  isReadyToMerge,
  notReadyReason,
  isSettled,
  issueUrl,
  mergeArgv,
  mergeCommand,
  prBase,
  prLabel,
  prRef,
  prUrls,
  threadCount,
  threadsArgv,
  zenieState,
} from './pr'
import type { Threads } from './pr'

const PANES = [
  { id: 'nonstop-human', title: 'Need Human', columns: 44 },
  { id: 'nonstop-progress', title: 'In Progress', columns: 44 },
  { id: 'nonstop-pending', title: 'Pending', columns: 44 },
  { id: 'nonstop-done', title: 'Done', columns: 44 },
] as const
const isTab = (id: string) => PANES.some(p => p.id === id)
const LEGACY_PANE_ID = 'nonstop'
const SPINNER = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏']
const SPIN_TICK_MS = 250
const LEDGER_TICK_MS = 2000
const PR_TICK_MS = 60000
const GH_TIMEOUT_MS = 20000
const STATS_TIMEOUT_MS = 20000
const MERGE_TIMEOUT_MS = 60000
const CONFIRM_MS = 5000
const GH_PARALLEL = 4
const GH_FIELDS = 'state,mergeStateStatus,statusCheckRollup,reviewDecision,baseRefName'

// Change the shape tag when Ledger or the PR map changes: it declines what an
// earlier version stored, which outlives a reload.
const ledger = atom({ plugin: 'znf', key: 'ledger' } as const, null, { shape: 'nonstop-3' })
const ledgerError = atom({ plugin: 'znf', key: 'ledgerError' } as const, null, { shape: 'nonstop-1' })
// One entry per PR, written whole so label, Zenie, base and threads always describe one poll.
// `threads`: undefined = being fetched; null = the query failed or was refused.
type PrInfo = { label: string; zenie: string; base?: string; threads?: Threads | null }
const prInfo = atom({ plugin: 'znf', key: 'prInfo' } as const, {} as Record<string, PrInfo>, { shape: 'nonstop-1' })
// url -> epoch ms until which its merge button is armed. An expiry, not a flag: a reload
// cancels the disarm timer but keeps this state, so the time itself must end the arming.
const confirming = atom({ plugin: 'znf', key: 'confirming' } as const, {}, { shape: 'nonstop-2' })
// Urls whose merge runs now; mirrors `mergeInFlight` so the pane redraws.
const merging = atom({ plugin: 'znf', key: 'merging' } as const, [], { shape: 'nonstop-1' })
// The last `zenify nonstop stats --json` answer, or why it failed.
const stats = atom({ plugin: 'znf', key: 'stats' } as const, null, { shape: 'nonstop-1' })
const frame = atom({ plugin: 'znf', key: 'frame' } as const, 0, { shape: 'nonstop-1' })
// Ids of the task lines the user expanded; shared by the tabs.
const expanded = atom({ plugin: 'znf', key: 'expanded' } as const, [], { shape: 'nonstop-1' })
// FR-6.1: subagent id -> the task its runs are logged under, and worktree slug -> task id.
// State, not module variables: a worker outlives a reload of this module.
const runTags = atom({ plugin: 'znf', key: 'runTags' } as const, {}, { shape: 'nonstop-1' })
const slugTasks = atom({ plugin: 'znf', key: 'slugTasks' } as const, {}, { shape: 'nonstop-1' })

// Module state: lost on a reload, which also drops the timers; the next open re-arms them.
let timers: Timer[] = []
let spinTimer: Timer | undefined
let ledgerFile: string | undefined
let lastMtime: number | undefined
// mtime of estimates.md at the last stats run; 'missing' when it was absent; undefined = never ran.
let lastStatsKey: number | 'missing' | undefined
let isStatting = false
let isRefreshing = false
let isPolling = false

type Loaded = { ledger: Ledger; mtimeMs: number } | { missing: true } | { error: string }

const isMissing = (err: unknown) => /ENOENT/.test(String(err))
const message = (err: unknown) => (err instanceof Error ? err.message : String(err))

function today(): string {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

// FR-1.1: the cwd's own ledger, else the main checkout's (right from a worktree too).
async function ledgerPath($: $): Promise<string | undefined> {
  if (ledgerFile) return ledgerFile
  const cwd: string = await $.session.cwd()
  const local = `${cwd}/.zenify/nonstop/progress.md`
  try {
    await $.fs.stat(local)
    return (ledgerFile = local)
  } catch (err) {
    if (!isMissing(err)) throw err
  }
  const git = await $.process.run(['git', 'rev-parse', '--path-format=absolute', '--git-common-dir'], { cwd })
  // Not a git checkout: the local path was the only candidate.
  if (git.exitCode !== 0) return undefined
  const common = git.stdout.trim().replace(/\/+$/, '')
  return (ledgerFile = `${common.slice(0, common.lastIndexOf('/'))}/.znf/nonstop/progress.md`)
}

// Stats the ledger and reads it unless its mtime equals `since`.
async function load($: $, since?: number): Promise<Loaded | 'unchanged'> {
  try {
    const file = await ledgerPath($)
    if (!file) return { missing: true }
    const stat = await $.fs.stat(file)
    if (stat.mtimeMs === since) return 'unchanged'
    const text = await $.fs.read(file)
    return { ledger: parseLedger(String(text)), mtimeMs: stat.mtimeMs }
  } catch (err) {
    if (!isMissing(err)) return { error: message(err) }
    // FR-1.1 "if it exists": forget the path so the next load looks at the cwd's ledger again.
    ledgerFile = undefined
    return { missing: true }
  }
}

async function apply($: $, loaded: Loaded): Promise<void> {
  await applyLoaded($, loaded)
  await syncSpin($)
}

async function applyLoaded($: $, loaded: Loaded): Promise<void> {
  if ('ledger' in loaded) {
    lastMtime = loaded.mtimeMs
    const tasks = loaded.ledger.tasks
    const count = (s: string) => tasks.filter(t => t.status === s).length
    const r = count('running')
    await update($, ledger, () => loaded.ledger)
    await update($, ledgerError, () => null)
    $.ui.status(`nonstop ${r}/${r + count('queued')} ▶ · ${count('blocked')} cần anh`)
    return
  }
  lastMtime = undefined
  if ('missing' in loaded) {
    await update($, ledger, () => null)
    await update($, ledgerError, () => null)
    $.ui.status(undefined)
    return
  }
  await update($, ledgerError, () => loaded.error)
}

async function refresh($: $, force = false): Promise<void> {
  if (isRefreshing) return
  isRefreshing = true
  try {
    const loaded = await load($, force ? undefined : lastMtime)
    if (loaded !== 'unchanged') await apply($, loaded)
  } finally {
    isRefreshing = false
  }
}

// FR-3.10: the numbers come from the CLI, never from parsing estimates.md here. Runs when the
// file's mtime changed since the last run (or on `force`); an absent file still runs once.
async function refreshStats($: $, force = false): Promise<void> {
  if (isStatting) return
  isStatting = true
  try {
    const file = await ledgerPath($)
    if (!file) return
    const dir = file.slice(0, file.lastIndexOf('/'))
    let key: number | 'missing'
    try {
      key = (await $.fs.stat(`${dir}/estimates.md`)).mtimeMs
    } catch (err) {
      if (!isMissing(err)) throw err
      key = 'missing'
    }
    if (!force && key === lastStatsKey) return
    lastStatsKey = key
    try {
      const out = await $.process.run(['zenify', 'nonstop', 'stats', '--json', '--dir', dir], { timeoutMs: STATS_TIMEOUT_MS })
      if (out.exitCode !== 0) throw new Error(out.stderr.split('\n')[0] || `exit ${out.exitCode}`)
      const data: NonstopStats = parseStats(out.stdout)
      await update($, stats, () => ({ data }))
    } catch (err) {
      await update($, stats, () => ({ error: message(err).split('\n')[0] }))
    }
  } finally {
    isStatting = false
  }
}

async function viewPr($: $, url: string): Promise<{ label: string; base?: string; zenie: string }> {
  try {
    const view = await $.process.run(['gh', 'pr', 'view', url, '--json', GH_FIELDS], { timeoutMs: GH_TIMEOUT_MS })
    return { label: prLabel(view), base: prBase(view), zenie: zenieState(view) }
  } catch (err) {
    return { label: `gh lỗi: ${message(err).split('\n')[0]}`, zenie: '—' }
  }
}

// FR-2.8: the open and total review threads of one PR; null when the query fails.
async function viewThreads($: $, url: string): Promise<Threads | null> {
  const argv = threadsArgv(url)
  if (!argv) return null
  try {
    const out = await $.process.run(argv, { timeoutMs: GH_TIMEOUT_MS })
    return threadCount(out) ?? null
  } catch {
    return null
  }
}

async function pollPr($: $, url: string): Promise<void> {
  // Until both answers are in, the PR is not ready: the old threads belong to an older moment.
  await update($, prInfo, map => (map[url] ? { ...map, [url]: { ...map[url], threads: undefined } } : map))
  const { label, base, zenie: z } = await viewPr($, url)
  const counted = isSettled(label) ? null : await viewThreads($, url)
  await update($, prInfo, map => ({ ...map, [url]: { label, zenie: z, base: base ?? map[url]?.base, threads: counted } }))
}

// FR-2.2: every PR of the last section, at most GH_PARALLEL gh calls at once.
async function pollPrs($: $): Promise<void> {
  if (isPolling) return
  isPolling = true
  try {
    const current = await read($, ledger)
    const known = await read($, prInfo)
    const queue = current ? prUrls(current.tasks).filter(url => !isSettled(known[url]?.label ?? '')) : []
    const worker = async () => {
      for (let url = queue.shift(); url; url = queue.shift()) {
        await pollPr($, url)
      }
    }
    await Promise.all(Array.from({ length: Math.min(GH_PARALLEL, queue.length) }, worker))
  } finally {
    isPolling = false
  }
}

const without = (urls: string[], url: string) => urls.filter(u => u !== url)
const omit = (map: Record<string, number>, url: string) => Object.fromEntries(Object.entries(map).filter(([u]) => u !== url))

// Checked and added with no await between, so two presses never both merge.
const mergeInFlight = new Set<string>()

// FR-2.7: the first press arms the button for CONFIRM_MS, the second press merges.
async function pressMerge($: $, url: string): Promise<void> {
  if (mergeInFlight.has(url)) return
  const until = (await read($, confirming))[url]
  const now = await $.clock.now()
  if (until === undefined || now >= until) {
    const expiry = now + CONFIRM_MS
    await update($, confirming, map => ({ ...map, [url]: expiry }))
    // Clears only the arming it was made for, not a later one.
    $.clock.after(
      CONFIRM_MS,
      () =>
        void update($, confirming, map => (map[url] === expiry ? omit(map, url) : map)).catch(err =>
          $.ui.toast(`lỗi pane: ${message(err)}`),
        ),
    )
    return
  }
  if (mergeInFlight.has(url)) return
  mergeInFlight.add(url)
  try {
    // `until` may be stale: a merge that finished meanwhile cleared the arming. Confirm it
    // still stands, then clear it, before gh runs.
    const still = (await read($, confirming))[url]
    if (still !== until || (await $.clock.now()) >= until) return
    await update($, confirming, map => omit(map, url))
    // The arming may predate a poll that found an open thread or a failing Zenie.
    const info = (await read($, prInfo))[url]
    const reason = notReadyReason(info?.label ?? '', info?.zenie ?? '—', info?.threads ?? undefined)
    if (reason) {
      $.ui.toast(`chưa merge được: ${reason}`)
      return
    }
    await update($, merging, urls => [...urls, url])
    const argv = mergeArgv(url)
    if (!argv) {
      $.ui.toast('merge lỗi: URL PR không hợp lệ')
      return
    }
    const out = await $.process.run(argv, { timeoutMs: MERGE_TIMEOUT_MS })
    if (out.exitCode !== 0) {
      $.ui.toast(`merge lỗi: ${out.stderr.split('\n')[0] || `exit ${out.exitCode}`}`)
      return
    }
    $.ui.toast(`đã merge ${prRef(url)}`)
  } catch (err) {
    $.ui.toast(`merge lỗi: ${message(err)}`)
    return
  } finally {
    mergeInFlight.delete(url)
    await update($, merging, urls => without(urls, url))
  }
  try {
    await pollPr($, url)
  } catch (err) {
    $.ui.toast(`đã merge, nhưng không đọc lại PR: ${message(err)}`)
  }
}

// The directory of the resolved ledger when the ledger exists; undefined in a plain session.
async function ledgerDir($: $): Promise<string | undefined> {
  const file = await ledgerPath($)
  if (!file) return undefined
  try {
    await $.fs.stat(file)
  } catch (err) {
    if (isMissing(err)) return undefined
    throw err
  }
  return file.slice(0, file.lastIndexOf('/'))
}

// FR-6.1: a worker brief tags its agent `work` and maps its slug to the task; a prompt naming a
// worktree tags its agent `review` of that slug's task. Any other agent is not tracked.
async function tagRun($: $, agentId: string, prompt: string): Promise<void> {
  const { id, slug } = taskFromPrompt(prompt)
  const reviewed = id ? undefined : slugFromPrompt(prompt)
  if (!id && !reviewed) return
  if (!(await ledgerDir($))) return
  if (id) {
    await update($, runTags, tags => ({ ...tags, [agentId]: { id, kind: 'work' } }))
    if (slug) await update($, slugTasks, map => ({ ...map, [slug]: id }))
    return
  }
  const task = (await read($, slugTasks))[reviewed as string] ?? 'unknown'
  await update($, runTags, tags => ({ ...tags, [agentId]: { id: task, kind: 'review' } }))
}

// Runs one append at a time: there is no fs append, so each is a read and a whole write.
let appending: Promise<void> = Promise.resolve()

async function logRun($: $, agentId: string, ms: number): Promise<void> {
  const tag = (await read($, runTags))[agentId]
  if (!tag) return
  const dir = await ledgerDir($)
  if (!dir) return
  const line = logLine(new Date(await $.clock.now()).toISOString(), tag.id, tag.kind, ms)
  const write = appending.then(async () => {
    const file = `${dir}/durations.log`
    let text = ''
    try {
      text = String(await $.fs.read(file))
    } catch (err) {
      if (!isMissing(err)) throw err
    }
    await $.fs.write(file, `${text}${text && !text.endsWith('\n') ? '\n' : ''}${line}\n`)
  })
  appending = write.catch(() => undefined)
  await write
}

const timingError = ($: $) => (err: unknown) => $.ui.toast(`lỗi đo thời gian: ${message(err)}`)

function report($: $, err: unknown): void {
  void update($, ledgerError, () => message(err))
}

function stopTimers(): void {
  for (const t of timers) t.cancel()
  timers = []
  spinTimer?.cancel()
  spinTimer = undefined
}

// FR-3.8: the spinner timer runs only while a nonstop tab is open and a task is running.
async function syncSpin($: $): Promise<void> {
  const current = await read($, ledger)
  const failed = await read($, ledgerError)
  const isRunning = !failed && !!current?.tasks.some(t => t.status === 'running')
  const isOpen = (await $.ui.panes()).some(p => isTab(p.id))
  const isWanted = isRunning && isOpen
  if (isWanted && !spinTimer) {
    spinTimer = $.clock.every(
      SPIN_TICK_MS,
      () => void update($, frame, f => (f + 1) % SPINNER.length).catch(err => report($, err)),
    )
  } else if (!isWanted && spinTimer) {
    spinTimer.cancel()
    spinTimer = undefined
  }
}

// FR-1.2: the timers start with the first open and stop when the pane closes.
async function openPane($: $): Promise<void> {
  // FR-3.9: the one-pane version used the id `nonstop`.
  if ((await $.ui.panes()).some(p => p.id === LEGACY_PANE_ID)) await $.ui.close({ id: LEGACY_PANE_ID })
  for (const p of PANES) await $.ui.open({ ...p })
  await syncSpin($)
  await refreshStats($, true).catch(err => report($, err))
  armTimers($)
}

function armTimers($: $): void {
  if (timers.length) return
  timers = [
    $.clock.every(
      LEDGER_TICK_MS,
      () => void refresh($).then(() => refreshStats($)).catch(err => report($, err)),
    ),
    $.clock.every(PR_TICK_MS, () => void pollPrs($).catch(err => report($, err))),
  ]
  // The first PR labels come now, not a full tick after the open.
  void pollPrs($).catch(err => report($, err))
}

async function show($: $): Promise<void> {
  await refresh($, true)
  await openPane($)
}

const isNonstopPrompt = (e: { text: string; command?: { name: string } }) =>
  e.command?.name === 'znf:nonstop' || /^\/?znf:nonstop\b/.test(e.text)

const RANKS = ['P0', 'P1', 'P2', 'P3']
const rank = (p: string) => (RANKS.includes(p) ? RANKS.indexOf(p) : RANKS.length)
// Stable: equal ranks keep the ledger order.
const byPriority = <T extends { priority: string }>(items: T[]): T[] =>
  [...items].sort((a, b) => rank(a.priority) - rank(b.priority))
const withPriority = (priority: string, text: string) => (priority && priority !== '-' ? `${priority} ${text}` : text)

const cut = (s: string, cols: number) => (s.length > cols ? `${s.slice(0, Math.max(0, cols - 1))}…` : s)

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    stopTimers()
    ledgerFile = undefined
    lastMtime = undefined
    lastStatsKey = undefined
    // A reload keeps shaped state but not the in-flight set; a plain session start (resume,
    // /clear, compact) keeps both, so `merging` is reset to what is really running.
    await update($, merging, () => [...mergeInFlight])
    await update($, confirming, () => ({}))
    await $.command.register({
      name: 'nonstop-pane',
      description: 'Toggle the nonstop orchestrator pane',
    })
    // FR-3.1: reopen only a nonstop day that is still running (re-entry after a restart).
    const loaded = await load($)
    if (loaded !== 'unchanged' && 'ledger' in loaded) {
      const l = loaded.ledger
      if (l.date === today() && l.tasks.some(t => t.status === 'running')) {
        await apply($, loaded)
        await openPane($)
      }
    }
    // A nonstop tab that survived the restart must not stay frozen; a plain session arms nothing.
    if (!timers.length && (await $.ui.panes()).some(p => isTab(p.id))) {
      await refresh($, true)
      await syncSpin($)
      armTimers($)
    }
    return next(e)
  })

  on('prompt.submit', async ($, e, next) => {
    if (isNonstopPrompt(e)) await show($)
    return next(e)
  })

  on('tool.call', { tool: 'Skill' }, async ($, e, next) => {
    if ((e as { skill?: string }).skill === 'znf:nonstop') await show($)
    return next(e)
  })

  // FR-6.1: tagged on spawn, not on the Agent tool call: a foreground agent's turn ends before
  // its tool call answers. A resume keeps the tag its first spawn got.
  on('agent.spawn', async ($, e, next) => {
    const result = await next(e)
    if (!e.resume && result.agentId) await tagRun($, result.agentId, e.prompt).catch(timingError($))
    return result
  })

  // Each run of a subagent's loop is one turn; its durationMs is that run's wall time.
  on('turn.complete', async ($, e, next) => {
    const result = await next(e)
    if (e.agentId) await logRun($, e.agentId, e.durationMs).catch(timingError($))
    return result
  })

  on('command.run', { command: 'nonstop-pane' }, async $ => {
    const isUp = (await $.ui.panes()).some(p => isTab(p.id))
    if (isUp) {
      for (const p of PANES) await $.ui.close({ id: p.id })
      return { text: 'đã đóng pane nonstop' }
    }
    await show($)
    return { text: 'đã mở pane nonstop' }
  })

  on('ui.close', async ($, e, next) => {
    const result = await next(e)
    if (isTab(e.id) && !(await $.ui.panes()).some(p => isTab(p.id))) stopTimers()
    else await syncSpin($)
    return result
  })

  for (const { id } of PANES) {
    on('ui.render', { component: 'Pane', requestId: id }, async ($, e) => {
      const { Box, Button, Link, Text } = $.ui.resolve(e)
      const cols = Math.max(1, e.props.bodyColumns)
      const line = (s: string, props: Record<string, unknown> = {}) => <Text {...props}>{cut(s, cols)}</Text>
      // A row of plain text and links; the text pieces are cut so the row fits the width.
      type Part = string | { href: string; label: string } | { taskId: string }
      const row = (parts: Part[], extra: unknown = null) => {
        let left = cols
        const cells = parts.map(p => {
          if (typeof p !== 'string' && 'taskId' in p) {
            left -= p.taskId.length
            return (
              <Button
                key={`task:${id}:${p.taskId}`}
                label={p.taskId}
                plain
                onPress={async () => {
                  await update($, expanded, ids =>
                    ids.includes(p.taskId) ? ids.filter(x => x !== p.taskId) : [...ids, p.taskId],
                  )
                }}
              />
            )
          }
          if (typeof p !== 'string') {
            left -= p.label.length
            return <Link href={p.href} label={p.label} />
          }
          const text = cut(p, Math.max(0, left))
          left -= text.length
          return <Text>{text}</Text>
        })
        return (
          <Box flexDirection="row">
            {cells}
            {extra}
          </Box>
        )
      }
      const prRow = (url: string, status: string, extra: unknown = null) =>
        row([{ href: url, label: prRef(url) }, status ? ` ${status}` : ''], extra)
      const empty = () => <Box flexDirection="column">{line('trống', { dimColor: true })}</Box>
      const error = await read($, ledgerError)
      const l = await read($, ledger)
      const infos = await read($, prInfo)
      const open = await read($, expanded)
      const armedUntil = await read($, confirming)
      const inMerge = await read($, merging)
      const now = await $.clock.now()
      const st = await read($, stats)

      if (error) return <Box flexDirection="column">{line(`lỗi ledger: ${error}`, { color: 'red' })}</Box>
      if (!l) return <Box flexDirection="column">{line('chưa có ledger nonstop', { dimColor: true })}</Box>

      const label = (u: string) => infos[u]?.label ?? ''
      const zenieOf = (u: string) => infos[u]?.zenie ?? '—'
      const countOf = (u: string) => infos[u]?.threads
      const baseOf = (u: string) => infos[u]?.base
      const urls = prUrls(l.tasks)
      const mergeable = urls.filter(u => mergeArgv(u) && isReadyToMerge(label(u), zenieOf(u), countOf(u) ?? undefined))
      const watching = urls.filter(u => !isSettled(label(u)))
      const threadText = (u: string) => {
        const c = countOf(u)
        return c === undefined ? '…' : c === null ? '?' : `${c.open}/${c.total}`
      }
      const watchText = (u: string) => `${label(u)} · Zenie ${zenieOf(u)} · thread ${threadText(u)}`
      const settled = urls.filter(u => isSettled(label(u)))
      const by = (s: string) => byPriority(l.tasks.filter(t => t.status === s))
      const isArmed = (url: string) => now < (armedUntil[url] ?? 0)
      const mergeLabel = (url: string) =>
        inMerge.includes(url) ? 'đang merge…' : isArmed(url) ? `xác nhận → ${baseOf(url)}?` : 'merge'
      // FR-3.7: the expanded lines are indented and wrapped, never cut.
      const detail = (lines: string[]) =>
        lines.map(s => (
          <Text wrap="wrap" dimColor>
            {`  ${s}`}
          </Text>
        ))
      const fields = (t: (typeof l.tasks)[number]) =>
        [t.repo, t.source, t.issue, t.size, t.skill, t.worker, t.status].join(' · ')
      const prefix = (priority: string): string => withPriority(priority, '')
      const taskLine = (t: (typeof l.tasks)[number], tail: string, lead = '') => {
        const href = issueUrl(t.issue)
        const issue: Part[] = href ? [' ', { href, label: t.issue }] : []
        return (
          <Box flexDirection="column">
            {row([lead + prefix(t.priority), { taskId: t.id }, ` ${t.repo}`, ...issue, ` ${tail}`])}
            {open.includes(t.id) && detail([fields(t), t.evidence])}
          </Box>
        )
      }
      const needLine = (n: { id: string; question: string; priority: string }) => {
        if (!n.id) return line(n.question)
        const task = l.tasks.find(t => t.id === n.id)
        return (
          <Box flexDirection="column">
            {row([prefix(n.priority), { taskId: n.id }, ` ${n.question}`])}
            {open.includes(n.id) && detail(task ? [fields(task), n.question] : [n.question])}
          </Box>
        )
      }

      const priorityOf = (taskId: string) => l.tasks.find(t => t.id === taskId)?.priority ?? '-'
      const needs = byPriority(l.needs.map(n => ({ ...n, priority: priorityOf(n.id) })))

      if (id === 'nonstop-human') {
        if (!needs.length && !mergeable.length) return empty()
        return (
          <Box flexDirection="column">
            {needs.length > 0 && line('CẦN ANH', { bold: true, color: 'yellow' })}
            {needs.map(needLine)}
            {mergeable.length > 0 && line('CHỜ MERGE', { bold: true })}
            {mergeable.map(url => (
              prRow(
                url,
                label(url),
                <Box flexDirection="row">
                <Button
                  key={url}
                  label="c"
                  hotkey={url === mergeable[0] ? 'c' : undefined}
                  onPress={async press => {
                    await $.ui.copy({ text: mergeCommand(url) ?? '', surface: press.surface })
                    $.ui.toast('đã copy lệnh merge')
                  }}
                />
                {baseOf(url) && (
                  <Button
                    key={`merge:${url}`}
                    label={mergeLabel(url)}
                    variant={isArmed(url) ? 'primary' : undefined}
                    onPress={() => pressMerge($, url).catch(err => $.ui.toast(`lỗi pane: ${message(err)}`))}
                  />
                )}
                </Box>,
              )
            ))}
          </Box>
        )
      }

      if (id === 'nonstop-pending') {
        const deferred = [...by('deferred')].sort((a, b) => (a.until ?? '').localeCompare(b.until ?? ''))
        const carried = by('carry-over')
        if (!deferred.length && !carried.length) return empty()
        return (
          <Box flexDirection="column">
            {deferred.length > 0 && line('HOÃN', { bold: true })}
            {deferred.map(t => taskLine(t, `tới ${t.until ?? '?'}`))}
            {carried.length > 0 && line('CARRY-OVER', { bold: true })}
            {carried.map(t => taskLine(t, ''))}
          </Box>
        )
      }

      if (id === 'nonstop-done') {
        const done = by('done')
        if (!done.length && !settled.length && (!st || st.data?.missing)) return empty()
        // Ledger tasks done today; empty when the ledger is another day's, so stale days never warn.
        const doneToday = st?.data && l.date === st.data.today.date ? done.map(t => t.id) : []
        return (
          <Box flexDirection="column">
            {done.length > 0 && line('DONE', { bold: true })}
            {done.map(t => taskLine(t, t.evidence))}
            {settled.length > 0 && line('PR', { bold: true })}
            {settled.map(url => prRow(url, label(url)))}
            {st && line('THỐNG KÊ', { bold: true })}
            {st?.error && (
              <Text wrap="wrap" color="red">
                {`lỗi thống kê: ${st.error}`}
              </Text>
            )}
            {st?.data &&
              statsLines(st.data, doneToday).map(s =>
                s.cut ? (
                  line(s.text, s.warn ? { color: 'yellow' } : {})
                ) : (
                  <Text wrap="wrap" {...(s.warn ? { color: 'yellow' } : {})}>
                    {s.text}
                  </Text>
                ),
              )}
          </Box>
        )
      }

      const running = by('running')
      const queued = by('queued')
      const spin = SPINNER[await read($, frame)]
      const isIdle = !running.length && !queued.length && !watching.length && !l.malformed.length
      return (
        <Box flexDirection="column">
          {running.length > 0 ? (
            <Text>{`▶ ${spin} đang chạy ${running.length} worker`}</Text>
          ) : (
            <Text dimColor>⏸ tạm dừng</Text>
          )}
          {isIdle && line('trống', { dimColor: true })}
          {running.length > 0 && line('RUNNING', { bold: true })}
          {running.map(t => taskLine(t, `${t.skill} ${t.worker}`, `${spin} `))}
          {queued.length > 0 && line('QUEUED', { bold: true })}
          {queued.map(t => taskLine(t, `${t.skill} ${t.worker}`))}
          {watching.length > 0 && line('ĐANG WATCH', { bold: true })}
          {watching.map(url => prRow(url, watchText(url)))}
          {line(`done ${by('done').length} · queued ${queued.length} · blocked ${by('blocked').length}`, { dimColor: true })}
          {l.malformed.length > 0 && line(`⚠ ${l.malformed.length} dòng không đúng format`, { color: 'yellow' })}
          {l.malformed.slice(0, 3).map(m => line(m, { dimColor: true }))}
        </Box>
      )
    })
  }
}
