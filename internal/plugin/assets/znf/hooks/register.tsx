import { atom, read, update } from 'claude-code'
import type { EngineInterface as $, Register, Timer } from 'claude-code'

import { logLine, slugFromPrompt, taskFromPrompt } from './durations'
import { parseLedger } from './ledger'
import type { Ledger } from './ledger'
import { footerLines, parseStats } from './stats'
import type { NonstopStats } from './stats'
import {
  isFinal,
  isReadyToMerge,
  notReadyReason,
  isSettled,
  issueUrl,
  mergeArgv,
  mergeCommand,
  prBase,
  prHead,
  prLabel,
  prRef,
  prUrls,
  threadCount,
  threadsArgv,
  zenieState,
} from './pr'
import type { Threads } from './pr'

// FR-3.12: one pane; the four tabs are drawn inside it.
const PANE = { id: 'nonstop', title: 'Nonstop', columns: 44 } as const
const isTab = (id: string) => id === PANE.id
// The four panes of the earlier version; opening closes any that survived.
const LEGACY_PANE_IDS = ['nonstop-human', 'nonstop-progress', 'nonstop-pending', 'nonstop-done']
const TABS = [
  { id: 'human', label: 'Need Human' },
  { id: 'progress', label: 'In Progress' },
  { id: 'pending', label: 'Pending' },
  { id: 'done', label: 'Done' },
] as const
const SPINNER = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏']
const SPIN_TICK_MS = 250
const LEDGER_TICK_MS = 2000
const PR_TICK_MS = 60000
const GH_TIMEOUT_MS = 20000
const STATS_TIMEOUT_MS = 20000
const MERGE_TIMEOUT_MS = 60000
const CONFIRM_MS = 5000
const GH_PARALLEL = 4
const GH_FIELDS = 'state,mergeStateStatus,statusCheckRollup,reviewDecision,baseRefName,headRefOid'

// Change the shape tag when Ledger or the PR map changes: it declines what an
// earlier version stored, which outlives a reload.
const ledger = atom({ plugin: 'znf', key: 'ledger' } as const, null, { shape: 'nonstop-3' })
const ledgerError = atom({ plugin: 'znf', key: 'ledgerError' } as const, null, { shape: 'nonstop-1' })
// One entry per PR, written whole so label, Zenie, base and threads always describe one poll.
// `threads`: undefined = being fetched; null = the query failed or was refused.
type PrInfo = { label: string; zenie: string; base?: string; threads?: Threads | null }
const prInfo = atom({ plugin: 'znf', key: 'prInfo' } as const, {} as Record<string, PrInfo>, { shape: 'nonstop-1' })
// url -> epoch ms until which its merge button is armed, and the base branch it was armed for.
// An expiry, not a flag: a reload cancels the disarm timer but keeps this state, so the time
// itself must end the arming.
type Arming = { until: number; base: string }
const confirming = atom({ plugin: 'znf', key: 'confirming' } as const, {}, { shape: 'nonstop-3' })
// Urls whose merge runs now; mirrors `mergeInFlight` so the pane redraws.
const merging = atom({ plugin: 'znf', key: 'merging' } as const, [], { shape: 'nonstop-1' })
// The last `zenify nonstop stats --json` answer, or why it failed.
const stats = atom({ plugin: 'znf', key: 'stats' } as const, null, { shape: 'nonstop-1' })
// The selected tab of the one pane.
const tab = atom({ plugin: 'znf', key: 'tab' } as const, 'human', { shape: 'nonstop-1' })
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
// What the last successful stats run read: ledger dir, estimates.md and durations.log mtimes
// ('none' when absent) and the local date. undefined = none succeeded since the session start.
let lastStatsKey: string | undefined
let isStatting = false
let isRefreshing = false
let isPolling = false

type Loaded = { ledger: Ledger; mtimeMs: number } | { missing: true } | { error: string }

const isMissing = (err: unknown) => /ENOENT/.test(String(err))
const message = (err: unknown) => (err instanceof Error ? err.message : String(err))

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

// FR-3.3: the status line; an item that is 0 or unknown is hidden, except In Progress.
async function updateStatus($: $): Promise<void> {
  const l = await read($, ledger)
  if (!l) {
    $.ui.status(undefined)
    return
  }
  const infos = await read($, prInfo)
  const st = await read($, stats)
  const count = (s: string) => l.tasks.filter(t => t.status === s).length
  const r = count('running')
  const urls = prUrls(l.tasks)
  const label = (u: string) => infos[u]?.label ?? ''
  const ready = urls.filter(u => mergeArgv(u) && isReadyToMerge(label(u), infos[u]?.zenie ?? '—', infos[u]?.threads ?? undefined)).length
  const red = urls.filter(u => label(u).startsWith('CI đỏ')).length
  const threads = urls.reduce((n, u) => (isFinal(label(u)) ? n : n + (infos[u]?.threads?.open ?? 0)), 0)
  const done = count('done')
  // Deferred and carried-over tasks are not part of today's work, so not of its ratio.
  const pending = count('deferred') + count('carry-over')
  const today = st?.data && !st.data.missing ? st.data.today : undefined
  const items = [
    `nonstop ${r > 0 ? '▶' : '⏸'}`,
    `In Progress: ${r}/${r + count('queued')}`,
    l.needs.length > 0 && `Need Human: ${l.needs.length}`,
    pending > 0 && `Pending: ${pending}`,
    done > 0 && `Done: ${done}/${l.tasks.length - pending}`,
    ready > 0 && `Ready: ${ready}`,
    red > 0 && `CI ✗: ${red}`,
    threads > 0 && `Threads: ${threads}`,
    !!today?.points && `${today.points} pts`,
    !!today?.total && `hit ${Math.round((100 * today.hit) / today.total)}%`,
  ]
  $.ui.status(items.filter(Boolean).join(' · '))
}

async function applyLoaded($: $, loaded: Loaded): Promise<void> {
  if ('ledger' in loaded) {
    lastMtime = loaded.mtimeMs
    const tasks = loaded.ledger.tasks
    await update($, ledger, () => loaded.ledger)
    await update($, ledgerError, () => null)
    await updateStatus($)
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

const localDate = (ms: number) => {
  const d = new Date(ms)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

// FR-3.10: the numbers come from the CLI, never from parsing estimates.md here. Runs when
// estimates.md, durations.log or the local date changed since the last successful run (or on
// `force`); an absent file still runs once, and a failed run is retried on the next tick.
async function refreshStats($: $, force = false): Promise<void> {
  if (isStatting) return
  isStatting = true
  try {
    const file = await ledgerPath($)
    if (!file) return
    const dir = file.slice(0, file.lastIndexOf('/'))
    const mtime = async (name: string): Promise<string> => {
      try {
        return String((await $.fs.stat(`${dir}/${name}`)).mtimeMs)
      } catch (err) {
        if (!isMissing(err)) throw err
        return 'none'
      }
    }
    const key = [dir, await mtime('estimates.md'), await mtime('durations.log'), localDate(await $.clock.now())].join('|')
    if (!force && key === lastStatsKey) return
    try {
      const out = await $.process.run(['zenify', 'nonstop', 'stats', '--json', '--dir', dir], { timeoutMs: STATS_TIMEOUT_MS })
      if (out.exitCode !== 0) throw new Error(out.stderr.split('\n')[0] || `exit ${out.exitCode}`)
      const data: NonstopStats = parseStats(out.stdout)
      await update($, stats, () => ({ data }))
      await updateStatus($)
      lastStatsKey = key
    } catch (err) {
      await update($, stats, () => ({ error: message(err).split('\n')[0] }))
    }
  } finally {
    isStatting = false
  }
}

async function viewPr($: $, url: string): Promise<{ label: string; base?: string; head?: string; zenie: string }> {
  try {
    const view = await $.process.run(['gh', 'pr', 'view', url, '--json', GH_FIELDS], { timeoutMs: GH_TIMEOUT_MS })
    return { label: prLabel(view), base: prBase(view), head: prHead(view), zenie: zenieState(view) }
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

// One poll of a PR, stored and returned with what this very poll read (base and head
// undefined when the view failed).
type Polled = { label: string; zenie: string; base?: string; head?: string; threads: Threads | null }

async function pollPr($: $, url: string): Promise<Polled> {
  // Until both answers are in, the PR is not ready: the old threads belong to an older moment.
  await update($, prInfo, map => (map[url] ? { ...map, [url]: { ...map[url], threads: undefined } } : map))
  const { label, base, head, zenie: z } = await viewPr($, url)
  const counted = isSettled(label) ? null : await viewThreads($, url)
  await update($, prInfo, map => ({ ...map, [url]: { label, zenie: z, base: base ?? map[url]?.base, threads: counted } }))
  await updateStatus($)
  return { label, zenie: z, base, head, threads: counted }
}

// FR-2.2: every PR of the last section, at most GH_PARALLEL gh calls at once.
async function pollPrs($: $): Promise<void> {
  if (isPolling) return
  isPolling = true
  try {
    const current = await read($, ledger)
    const known = await read($, prInfo)
    // A closed PR stays polled: it may be reopened.
    const queue = current ? prUrls(current.tasks).filter(url => !isFinal(known[url]?.label ?? '')) : []
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
const omit = <T,>(map: Record<string, T>, url: string): Record<string, T> =>
  Object.fromEntries(Object.entries(map).filter(([u]) => u !== url))

// Checked and added with no await between, so two presses never both merge.
const mergeInFlight = new Set<string>()

// FR-2.7: the first press arms the button for CONFIRM_MS, the second press re-reads the PR and
// merges only if that fresh read is still ready, on the armed base, at the head it just read.
async function pressMerge($: $, url: string): Promise<void> {
  if (mergeInFlight.has(url)) return
  const until = (await read($, confirming))[url]?.until
  const now = await $.clock.now()
  if (until === undefined || now >= until) {
    const base = (await read($, prInfo))[url]?.base
    // The button is drawn only once the base is known.
    if (!base) return
    const expiry = now + CONFIRM_MS
    await update($, confirming, map => ({ ...map, [url]: { until: expiry, base } }))
    // Clears only the arming it was made for, not a later one.
    $.clock.after(
      CONFIRM_MS,
      () =>
        void update($, confirming, map => (map[url]?.until === expiry ? omit(map, url) : map)).catch(err =>
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
    if (still?.until !== until || (await $.clock.now()) >= until) return
    await update($, confirming, map => omit(map, url))
    await update($, merging, urls => [...urls, url])
    const argv = mergeArgv(url)
    if (!argv) {
      $.ui.toast('merge lỗi: URL PR không hợp lệ')
      return
    }
    // The cache may be a minute old: a thread opened or the base changed since the last poll.
    const fresh = await pollPr($, url)
    const reason =
      notReadyReason(fresh.label, fresh.zenie, fresh.threads ?? undefined) ??
      (fresh.base !== still.base ? `base đổi ${still.base} → ${fresh.base ?? '?'}` : undefined) ??
      (fresh.head ? undefined : 'không đọc được head commit')
    if (reason) {
      $.ui.toast(`chưa merge được: ${reason}`)
      return
    }
    const out = await $.process.run([...argv, '--match-head-commit', fresh.head as string], { timeoutMs: MERGE_TIMEOUT_MS })
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
  // FR-3.12: the four-pane version used the ids `nonstop-*`.
  const listed = await $.ui.panes()
  for (const id of LEGACY_PANE_IDS) if (listed.some(p => p.id === id)) await $.ui.close({ id })
  await $.ui.open({ ...PANE })
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
    // FR-3.1: never open the pane here; session.start cannot tell a nonstop restart from an
    // unrelated session. Opening is the skill, the prompt or the command.
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
      await $.ui.close({ id: PANE.id })
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

  on('ui.render', { component: 'Pane', requestId: PANE.id }, async ($, e) => {
    const { Box, Button, Link, Text } = $.ui.resolve(e)
    const cols = Math.max(1, e.props.bodyColumns)
    const id = await read($, tab)
    // One drawn element and the rows it takes, so the body can be cut to the room it has.
    type Item = { node: unknown; h: number }
    const it = (node: unknown, h = 1): Item => ({ node, h })
    const line = (s: string, props: Record<string, unknown> = {}) => <Text {...props}>{cut(s, cols)}</Text>
    const lineItem = (s: string, props: Record<string, unknown> = {}) => it(line(s, props))
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
      it(row([{ href: url, label: prRef(url) }, status ? ` ${status}` : ''], extra))
    const error = await read($, ledgerError)
    const l = await read($, ledger)
    const infos = await read($, prInfo)
    const open = await read($, expanded)
    const armings: Record<string, Arming> = await read($, confirming)
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
    const isArmed = (url: string) => now < (armings[url]?.until ?? 0)
    const mergeLabel = (url: string) =>
      inMerge.includes(url) ? 'đang merge…' : isArmed(url) ? `xác nhận → ${armings[url].base}?` : 'merge'
    // FR-3.7: the expanded lines are indented and wrapped, never cut.
    const detailRows = (lines: string[]) =>
      lines.reduce((n, s) => n + Math.max(1, Math.ceil((s.length + 2) / cols)), 0)
    const detail = (lines: string[]) =>
      lines.map(s => (
        <Text wrap="wrap" dimColor>
          {`  ${s}`}
        </Text>
      ))
    const fields = (t: (typeof l.tasks)[number]) =>
      [t.repo, t.source, t.issue, t.size, t.skill, t.worker, t.status].join(' · ')
    const prefix = (priority: string): string => withPriority(priority, '')
    const taskLine = (t: (typeof l.tasks)[number], tail: string, lead = ''): Item => {
      const href = issueUrl(t.issue)
      const issue: Part[] = href ? [' ', { href, label: t.issue }] : []
      const extra = open.includes(t.id) ? [fields(t), t.evidence] : []
      return it(
        <Box flexDirection="column">
          {row([lead + prefix(t.priority), { taskId: t.id }, ` ${t.repo}`, ...issue, ` ${tail}`])}
          {extra.length > 0 && detail(extra)}
        </Box>,
        1 + detailRows(extra),
      )
    }
    const needLine = (n: { id: string; question: string; priority: string }): Item => {
      if (!n.id) return lineItem(n.question)
      const task = l.tasks.find(t => t.id === n.id)
      const extra = open.includes(n.id) ? (task ? [fields(task), n.question] : [n.question]) : []
      return it(
        <Box flexDirection="column">
          {row([prefix(n.priority), { taskId: n.id }, ` ${n.question}`])}
          {extra.length > 0 && detail(extra)}
        </Box>,
        1 + detailRows(extra),
      )
    }

    const priorityOf = (taskId: string) => l.tasks.find(t => t.id === taskId)?.priority ?? '-'
    const needs = byPriority(l.needs.map(n => ({ ...n, priority: priorityOf(n.id) })))

    const humanItems = (): Item[] => [
      ...(needs.length > 0 ? [lineItem('CẦN ANH', { bold: true, color: 'yellow' })] : []),
      ...needs.map(needLine),
      ...(mergeable.length > 0 ? [lineItem('CHỜ MERGE', { bold: true })] : []),
      ...mergeable.map(url =>
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
        ),
      ),
    ]

    const pendingItems = (): Item[] => {
      const deferred = [...by('deferred')].sort((a, b) => (a.until ?? '').localeCompare(b.until ?? ''))
      const carried = by('carry-over')
      return [
        ...(deferred.length > 0 ? [lineItem('HOÃN', { bold: true })] : []),
        ...deferred.map(t => taskLine(t, `tới ${t.until ?? '?'}`)),
        ...(carried.length > 0 ? [lineItem('CARRY-OVER', { bold: true })] : []),
        ...carried.map(t => taskLine(t, '')),
      ]
    }

    const done = by('done')
    const doneItems = (): Item[] => [
      ...(done.length > 0 ? [lineItem('DONE', { bold: true })] : []),
      ...done.map(t => taskLine(t, t.evidence)),
      ...(settled.length > 0 ? [lineItem('PR', { bold: true })] : []),
      ...settled.map(url => prRow(url, label(url))),
    ]

    const progressItems = async (): Promise<Item[]> => {
      const running = by('running')
      const queued = by('queued')
      const spin = SPINNER[await read($, frame)]
      const isIdle = !running.length && !queued.length && !watching.length && !l.malformed.length
      return [
        running.length > 0 ? lineItem(`▶ ${spin} đang chạy ${running.length} worker`) : lineItem('⏸ tạm dừng', { dimColor: true }),
        ...(isIdle ? [lineItem('trống', { dimColor: true })] : []),
        ...(running.length > 0 ? [lineItem('RUNNING', { bold: true })] : []),
        ...running.map(t => taskLine(t, `${t.skill} ${t.worker}`, `${spin} `)),
        ...(queued.length > 0 ? [lineItem('QUEUED', { bold: true })] : []),
        ...queued.map(t => taskLine(t, `${t.skill} ${t.worker}`)),
        ...(watching.length > 0 ? [lineItem('ĐANG WATCH', { bold: true })] : []),
        ...watching.map(url => prRow(url, watchText(url))),
        lineItem(`done ${done.length} · queued ${queued.length} · blocked ${by('blocked').length}`, { dimColor: true }),
        ...(l.malformed.length > 0 ? [lineItem(`⚠ ${l.malformed.length} dòng không đúng format`, { color: 'yellow' })] : []),
        ...l.malformed.slice(0, 3).map(m => lineItem(m, { dimColor: true })),
      ]
    }

    const items = id === 'human' ? humanItems() : id === 'pending' ? pendingItems() : id === 'done' ? doneItems() : await progressItems()
    if (!items.length) items.push(lineItem('trống', { dimColor: true }))

    // FR-3.12: the fixed footer; its lines come before the body so the body knows its room.
    // Today's ledger tasks done, empty when the ledger is another day's, so stale days never warn.
    const doneToday = st?.data && l.date === st.data.today.date ? done.map(t => t.id) : []
    const footer = st?.error
      ? [{ text: `⚠ lỗi thống kê: ${st.error}`, color: 'red' }]
      : st?.data
        ? footerLines(st.data, doneToday).map(f => ({ text: f.text, ...(f.warn ? { color: 'yellow' } : {}) }))
        : []
    const rowsTotal = e.props.scroll?.bodyRows ?? e.viewport?.rows ?? 30
    const room = Math.max(2, rowsTotal - 1 - 1 - footer.length)
    const total = items.reduce((n, i) => n + i.h, 0)
    let shown = items
    let hidden = 0
    if (total > room) {
      let used = 0
      shown = items.filter(i => (used + i.h <= room - 1 ? ((used += i.h), true) : ((used = room), false)))
      hidden = total - shown.reduce((n, i) => n + i.h, 0)
    }

    return (
      <Box flexDirection="column">
        <Box flexDirection="row">
          {TABS.map((t, i) => (
            <Button
              key={`tab:${t.id}`}
              label={t.label}
              plain
              hotkey={String(i + 1)}
              variant={t.id === id ? 'primary' : undefined}
              onPress={() => update($, tab, () => t.id).then(() => undefined)}
            />
          ))}
        </Box>
        {shown.map(i => i.node)}
        {hidden > 0 && line(`+${hidden} dòng`, { dimColor: true })}
        {line('─ Thống kê '.padEnd(cols, '─'), { dimColor: true })}
        {footer.map(f => line(f.text, f.color ? { color: f.color } : {}))}
      </Box>
    )
  })
}
