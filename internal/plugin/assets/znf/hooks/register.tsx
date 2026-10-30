import { atom, read, update } from 'claude-code'
import type { EngineInterface as $, Register, Timer } from 'claude-code'

import { bindLine, cwdLine, isEndCommand, isStartCommand, kindOf, parseStartId, phaseFor, phaseLine, stdoutOf, turnLine, unbindLine, waitLine } from './track'
import type { Kind, Usage, WaitKind } from './track'
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
// FR-4.1: the id this session's turns are recorded under; `ending` once its `zenify track end`
// ran, so the turn that ran it is written before the unbind (SC-22).
type Bound = { id: string; kind: Kind; ending: boolean }
const bound = atom({ plugin: 'znf', key: 'bound' } as const, null as Bound | null, { shape: 'track-1' })
// FR-2.3: false writes nothing. Set from `zenify track stats --json` -> `enabled` (setTrackEnabled).
const enabled = atom({ plugin: 'znf', key: 'enabled' } as const, false, { shape: 'track-1' })
// FR-4.6: when the last main-loop turn ended (epoch ms), until the next prompt reads it.
const lastMainEnd = atom({ plugin: 'znf', key: 'lastMainEnd' } as const, null as number | null, { shape: 'track-1' })
// FR-4.5: main-loop tool_use_ids a person was asked to allow.
const personAsks = atom({ plugin: 'znf', key: 'personAsks' } as const, [] as string[], { shape: 'track-1' })

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

// FR-3.13: the metrics definitions file of the workspace whose `.zenify/nonstop` holds the
// ledger. Read at runtime, never named in the plugin: undefined when the ledger is elsewhere or
// the file is absent. Cached per ledger dir so the render does not stat on every tick.
const WS_LEDGER_SUFFIX = '/.zenify/nonstop'
let metricsCache: { dir: string; path: string | undefined } | undefined
async function metricsPath($: $): Promise<string | undefined> {
  let dir: string | undefined
  try {
    dir = await ledgerDir($)
  } catch {
    return undefined
  }
  if (!dir || !dir.endsWith(WS_LEDGER_SUFFIX)) return undefined
  if (metricsCache?.dir === dir) return metricsCache.path
  const candidate = `${dir.slice(0, -WS_LEDGER_SUFFIX.length)}/docs/reference/zenify-kit/nonstop-metrics.md`
  let path: string | undefined
  try {
    const st = await $.fs.stat(candidate)
    path = st.kind === 'file' ? candidate : undefined
  } catch (err) {
    // ENOENT means the workspace has no metrics file; anything else is shown once (the cache below).
    if (!isMissing(err)) $.ui.toast(`lỗi pane: ${message(err)}`)
    path = undefined
  }
  metricsCache = { dir, path }
  return path
}

// The workspace holding `.zenify/manifest.json` at or above the cwd, as the CLI resolves it
// (internal/cli/workspaceresolve.go, the marker rule); cached per cwd.
let workspaceCache: { cwd: string; dir: string } | undefined
async function workspaceDir($: $): Promise<string> {
  const cwd: string = await $.session.cwd()
  if (workspaceCache?.cwd === cwd) return workspaceCache.dir
  for (let dir = cwd.replace(/\/+$/, ''); ; dir = dir.slice(0, dir.lastIndexOf('/'))) {
    try {
      await $.fs.stat(`${dir}/.zenify/manifest.json`)
      workspaceCache = { cwd, dir }
      return dir
    } catch (err) {
      if (!isMissing(err)) throw err
    }
    if (!dir) throw new Error(`no workspace (.zenify/manifest.json) at or above ${cwd}`)
  }
}

// Runs one append at a time: there is no fs append, so each is a read and a whole write.
let appending: Promise<void> = Promise.resolve()

const iso = (ms: number) => new Date(ms).toISOString()

// FR-4.2/FR-7.3: appends to this session's own file; a new file starts with the cwd line.
export async function appendSession($: $, line: string): Promise<void> {
  const file = `${await workspaceDir($)}/.zenify/track/sessions/${await $.session.id()}.log`
  const write = appending.then(async () => {
    let text: string
    try {
      text = String(await $.fs.read(file))
    } catch (err) {
      if (!isMissing(err)) throw err
      text = `${cwdLine(iso(await $.clock.now()), await $.session.cwd())}\n`
    }
    await $.fs.write(file, `${text}${text && !text.endsWith('\n') ? '\n' : ''}${line}\n`)
  })
  // Keeps the chain alive after a failure; the caller still gets the error from `write`.
  appending = write.catch(() => undefined)
  await write
}

// Task 6 calls this with `stats.enabled` from `zenify track stats --json`.
export async function setTrackEnabled($: $, on: boolean): Promise<void> {
  await update($, enabled, () => on)
}

// FR-2.3: whether track records; a failed read is toasted and counts as off.
const isEnabled = ($: $): Promise<boolean> => read($, enabled).catch(err => (timingError($)(err), false))

// FR-8.2: a Skill marker of the bound id's kind starts its phase.
async function recordPhase($: $, skill: string): Promise<void> {
  const b = await read($, bound)
  const phase = b && phaseFor(b.kind, skill)
  if (b && phase) await appendSession($, phaseLine(iso(await $.clock.now()), b.id, phase))
}

// FR-4.1/4.4/4.5: after a call — bind from a start's stdout, else a main-loop wait.
async function recordCall($: $, tool: string, command: string, isMain: boolean, useId: string | undefined, started: number, answer: unknown): Promise<void> {
  const now = await $.clock.now()
  if (tool === 'Bash' && isStartCommand(command)) {
    const id = parseStartId(stdoutOf(answer))
    if (id) {
      await update($, bound, () => ({ id, kind: kindOf(id), ending: false }))
      await appendSession($, bindLine(iso(now), id))
      return
    }
  }
  const b = await read($, bound)
  const asked = !!useId && (await read($, personAsks)).includes(useId)
  const kind: WaitKind | undefined = asked ? 'permission' : tool === 'AskUserQuestion' ? 'ask' : undefined
  if (b && isMain && kind) await appendSession($, waitLine(iso(now), b.id, now - started, kind))
}

// FR-4.2/4.8, SC-22: the turn line first, then the unbind of an id whose end ran this turn.
async function recordTurn($: $, e: { durationMs: number; agentId?: string; usage?: Usage }): Promise<void> {
  const now = await $.clock.now()
  const b = await read($, bound)
  if (b) {
    await appendSession($, turnLine(iso(now), b.id, e.durationMs, e.agentId ? 'sub' : 'main', e.usage))
    if (!e.agentId && b.ending) {
      await appendSession($, unbindLine(iso(now), b.id))
      await update($, bound, () => null)
    }
  }
  if (!e.agentId) await update($, lastMainEnd, () => now)
}

// FR-4.6: the time from the last main-loop turn end to this prompt, once per turn end.
async function recordIdle($: $): Promise<void> {
  const last = await read($, lastMainEnd)
  if (last === null) return
  await update($, lastMainEnd, () => null)
  const b = await read($, bound)
  const now = await $.clock.now()
  if (b) await appendSession($, waitLine(iso(now), b.id, now - last, 'idle'))
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
    if (await isEnabled($)) await recordIdle($).catch(timingError($))
    if (isNonstopPrompt(e)) await show($)
    return next(e)
  })

  on('tool.call', { tool: 'Skill' }, async ($, e, next) => {
    if ((e as { skill?: string }).skill === 'znf:nonstop') await show($)
    return next(e)
  })

  // zenify track (FR-4, FR-8.2): every tool call of a session, timed around `next(e)`; the
  // permission check runs inside it (tool.check below), so a person's ask is within the span.
  on('tool.call', async ($, e, next) => {
    if (!(await isEnabled($))) return next(e)
    const tool = String(e.tool)
    const command = tool === 'Bash' ? String((e as { command?: unknown }).command ?? '') : ''
    const useId = e.tool_use_id
    const isMain = !e.agentId
    let started = 0
    try {
      if (tool === 'Skill') await recordPhase($, String((e as { skill?: unknown }).skill ?? ''))
      const endId = command ? isEndCommand(command) : undefined
      if (endId) await update($, bound, b => (b?.id === endId ? { ...b, ending: true } : b))
      started = await $.clock.now()
    } catch (err) {
      timingError($)(err)
    }
    try {
      const answer = await next(e)
      await recordCall($, tool, command, isMain, useId, started, answer).catch(timingError($))
      return answer
    } finally {
      if (useId) await update($, personAsks, ids => ids.filter(x => x !== useId)).catch(timingError($))
    }
  })

  // FR-4.5: a main-loop ask that a person answers (not the host, not a classifier).
  on('tool.check', async ($, e, next) => {
    const verdict = await next(e)
    const useId = e.tool_use_id
    if (useId && !e.agentId && verdict.decision === 'ask' && e.approver === 'person') {
      await (async () => {
        if ((await read($, enabled)) && (await read($, bound))) await update($, personAsks, ids => [...ids, useId])
      })().catch(timingError($))
    }
    return verdict
  })

  on('turn.complete', async ($, e, next) => {
    const result = await next(e)
    if (await isEnabled($)) await recordTurn($, e).catch(timingError($))
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
    type Item = { node: unknown; h: number; collapsed?: Item }
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
    // An expanded item that outgrows the room is drawn as its head row plus this hint, so its id
    // button stays on screen and the user can still collapse it.
    const withCollapsed = (full: Item, head: unknown): Item =>
      full.h === 1
        ? full
        : {
            ...full,
            collapsed: it(
              <Box flexDirection="column">
                {head}
                {line('▾ chi tiết dài, cần pane cao hơn', { dimColor: true })}
              </Box>,
              2,
            ),
          }
    const taskLine = (t: (typeof l.tasks)[number], tail: string, lead = ''): Item => {
      const href = issueUrl(t.issue)
      const issue: Part[] = href ? [' ', { href, label: t.issue }] : []
      const extra = open.includes(t.id) ? [fields(t), t.evidence] : []
      const head = row([lead + prefix(t.priority), { taskId: t.id }, ` ${t.repo}`, ...issue, ` ${tail}`])
      return withCollapsed(
        it(
          <Box flexDirection="column">
            {head}
            {extra.length > 0 && detail(extra)}
          </Box>,
          1 + detailRows(extra),
        ),
        head,
      )
    }
    const needLine = (n: { id: string; question: string; priority: string }): Item => {
      if (!n.id) return lineItem(n.question)
      const task = l.tasks.find(t => t.id === n.id)
      const extra = open.includes(n.id) ? (task ? [fields(task), n.question] : [n.question]) : []
      const head = row([prefix(n.priority), { taskId: n.id }, ` ${n.question}`])
      return withCollapsed(
        it(
          <Box flexDirection="column">
            {head}
            {extra.length > 0 && detail(extra)}
          </Box>,
          1 + detailRows(extra),
        ),
        head,
      )
    }

    const priorityOf = (taskId: string) => l.tasks.find(t => t.id === taskId)?.priority ?? '-'
    const needs = byPriority(l.needs.map(n => ({ ...n, priority: priorityOf(n.id) })))

    const humanItems = (): Item[] => [
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
    const metrics = footer.length > 0 ? await metricsPath($) : undefined
    const rowsTotal = e.props.scroll?.bodyRows ?? e.viewport?.rows ?? 30
    const room = Math.max(2, rowsTotal - 1 - 1 - footer.length - (metrics ? 1 : 0))
    const total = items.reduce((n, i) => n + i.h, 0)
    let shown = items
    let hidden = 0
    if (total > room) {
      // Walk in order: an item that fits is shown, else its collapsed form if that fits, else stop.
      // The first item is always shown so the body is never empty.
      let used = 0
      shown = []
      for (const [n, i] of items.entries()) {
        const pick = used + i.h <= room - 1 ? i : i.collapsed && used + i.collapsed.h <= room - 1 ? i.collapsed : n === 0 ? (i.collapsed ?? i) : undefined
        if (!pick) break
        shown.push(pick)
        used += pick.h
      }
      hidden = total - shown.reduce((n, i) => n + i.h, 0)
    }

    return (
      <Box flexDirection="column">
        <Box flexDirection="row">
          {TABS.map((t, i) => [
            i > 0 && <Text dimColor> │ </Text>,
            // A Button has no color prop, so the selected tab is a colored bold Text, not a Button.
            t.id === id ? (
              <Text color="claude" bold>{`▸${t.label}`}</Text>
            ) : (
              <Button
                key={`tab:${t.id}`}
                label={t.label}
                plain
                dimColor
                onPress={() => update($, tab, () => t.id).then(() => undefined)}
              />
            ),
          ])}
        </Box>
        {shown.map(i => i.node)}
        {hidden > 0 && line(`+${hidden} dòng`, { dimColor: true })}
        {line('─ Thống kê '.padEnd(cols, '─'), { dimColor: true })}
        {footer.map(f => line(f.text, f.color ? { color: f.color } : {}))}
        {metrics && <Link href={`file://${metrics}`} label="Xem thêm thông tin" />}
      </Box>
    )
  })
}
