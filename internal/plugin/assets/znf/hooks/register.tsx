import { atom, read, update } from 'claude-code'
import type { EngineInterface as $, PluginOptions, Register, Timer } from 'claude-code'

import type { TrackStats } from '../types'
import { isInWorkspace, registerConfig, setInWorkspace, syncConfig, trackPaneOf } from './config'
import { bindLine, cwdLine, isStartCommand, kindOf, parseEndedId, parseStartId, phaseFor, phaseLine, stdoutOf, turnLine, unbindLine, waitLine } from './track'
import type { Kind, Usage, WaitKind } from './track'
import { COPY, SPIN_TICK_MS, STATUS_OFF, TABS, WATCH_TICK_MS, errorStatus, isTrackPrompt, parseTrackStats, statusText, tabLines } from './track-ui'
import type { Line, Tab } from './track-ui'

// FR-6.1: one pane; the five tabs are drawn inside it.
const PANE = { id: 'track', title: 'Track', columns: 44 } as const
const isPane = (id: string) => id === PANE.id
// FR-6.3/FR-11.5: how often the watched mtimes are compared, and the forced re-run for merged PRs.
const MERGED_TICK_MS = 5 * 60000
const STATS_TIMEOUT_MS = 20000
// FR-2.4: how often live/<session>.beat is rewritten. FR-1.4: how often stats re-run while a tab waits.
const BEAT_MS = 10000
const ROLLING_RERUN_MS = 10000

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
// FR-6.4/FR-11.4: the last `zenify track stats --json` answer, or why it failed. The shape tag
// declines what an earlier pane version stored under the same key.
const stats = atom({ plugin: 'znf', key: 'stats' } as const, null as { data?: TrackStats; error?: string } | null, { shape: 'track-1' })
// FR-6.2: the selected tab; `Kitchen` by default. track-3 declines a stored `total` or `this`.
const tab = atom({ plugin: 'znf', key: 'tab' } as const, 'kitchen' as Tab, { shape: 'track-3' })

// Module state: lost on a reload, whose session.start re-arms the timers.
let timers: Timer[] = []
// The watched mtimes and local date of the last stats run; undefined = none since session start.
let lastKey: string | undefined
let isStatting = false
// A forced run asked for while one was in flight; it runs when that one ends. A run that
// bypasses the backlog cache (`--force`) stays that way when queued.
let isRerunWanted = false
let isFreshWanted = false
// FR-1.4: when the last stats run started (epoch ms).
let lastRunAt = 0
// FR-2.2: tool_use_ids of an AskUserQuestion, or of a call a person was asked to allow, still running.
const pendingAsk = new Set<string>()
const pendingPermission = new Set<string>()
// FR-2.2: person asks not yet shown as a dialog, by tool_use_id. Auto mode's classifier settles most
// of them with nobody asked; one waits for a person only once classic.PermissionRequest fires.
const dialogCandidates = new Map<string, { tool: string; input: string; agentId?: string }>()
// FR-12.1: subagents between classic.SubagentStart and SubagentStop; a tab running one is cooking.
const runningAgents = new Set<string>()
// FR-2.1: the state last written to live/<session>.json; undefined = none this session.
type LiveState = 'ask' | 'permission' | 'busy' | 'idle'
let liveState: LiveState | undefined
// The id and `since` last written to the live file: a changed id rewrites it, `since` stays.
let liveId: string | undefined
let liveSince = ''
// This process's run id, written as `run` in the live file. The engine has no crypto, so random plus the clock.
const RUN_ID = `${Math.random().toString(16).slice(2)}${Date.now().toString(16)}`
// FR-12.1: true from a main-loop turn end until the next prompt; a wait (ask, permission) still wins.
let isMainIdle = false
// FR-12.1: a prompt was submitted in this process; kept across session.start (a reload is a new run, a compact is not).
let promptSeen = false
// FR-2.4: the stats' live TTL (30s): the cooking re-run interval.
const LIVE_TTL_MS = 30000

const isMissing = (err: unknown) => /ENOENT/.test(String(err))
const message = (err: unknown) => (err instanceof Error ? err.message : String(err))

// The workspace holding `.zenify/manifest.json` at or above the cwd, as the CLI resolves it
// (internal/cli/workspaceresolve.go, the marker rule); cached per cwd.
// undefined when there is none: a session outside a zenify workspace.
// A cwd moved outside mid-session (an additional working directory such as the knowledge store)
// keeps the workspace found earlier in this process.
let workspaceCache: { cwd: string; dir: string } | undefined
async function findWorkspace($: $): Promise<string | undefined> {
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
    if (!dir) break
  }
  if (!workspaceCache) return undefined
  workspaceCache = { cwd, dir: workspaceCache.dir }
  return workspaceCache.dir
}

async function workspaceDir($: $): Promise<string> {
  const dir = await findWorkspace($)
  if (!dir) throw new Error(`no workspace (.zenify/manifest.json) at or above ${await $.session.cwd()}`)
  return dir
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
    // FR-1.1: other tabs see the new line at their next tick.
    await $.fs.write(`${await workspaceDir($)}/.zenify/track/pulse`, iso(await $.clock.now()))
  })
  // Keeps the chain alive after a failure; the caller still gets the error from `write`.
  appending = write.catch(() => undefined)
  await write
}

// Runs one live-state write at a time, so a slower write never lands after a newer state.
let syncing: Promise<void> = Promise.resolve()

// FR-2.1–2.3: writes live/<session>.json, then pulse, only when the state or the bound id changed.
async function syncLive($: $): Promise<void> {
  const write = syncing.then(async () => {
    // Track may have been turned off while the call was open: write nothing, keep liveState.
    if (!(await isEnabled($))) return
    const state: LiveState = pendingAsk.size ? 'ask' : pendingPermission.size ? 'permission' : isMainIdle && !runningAgents.size ? 'idle' : 'busy'
    const boundId = (await read($, bound))?.id ?? ''
    if (state === liveState && boundId === liveId) return
    const track = `${await workspaceDir($)}/.zenify/track`
    const now = iso(await $.clock.now())
    // A new id under the same state is the same stretch of work: keep `since`.
    const since = state === liveState ? liveSince : now
    const doc = { state, since, cwd: await $.session.cwd(), id: boundId, run: RUN_ID }
    const id = await $.session.id()
    await $.fs.write(`${track}/live/${id}.json`, JSON.stringify(doc))
    // The first beat comes now, not at the 10s timer: stats skip a live file with no beat.
    await $.fs.write(`${track}/live/${id}.beat`, now)
    await $.fs.write(`${track}/pulse`, now)
    liveState = state
    liveId = boundId
    liveSince = since
  })
  // Keeps the chain alive after a failure; the caller still gets the error from `write`.
  syncing = write.catch(() => undefined)
  await write
}

// FR-2.1/FR-12.1: a live file left in ask, permission or busy by another run of this session becomes idle.
async function resetStaleLive($: $): Promise<void> {
  if (!(await isEnabled($))) return
  let text: string
  try {
    text = String(await $.fs.read(`${await workspaceDir($)}/.zenify/track/live/${await $.session.id()}.json`))
  } catch (err) {
    if (isMissing(err)) return
    throw err
  }
  const doc = JSON.parse(text) as { state?: string; run?: string }
  // This process wrote the file (a compact or a re-fired session.start), or a prompt means this run owns it. No await below this check.
  if (promptSeen || doc.run === RUN_ID) return
  // FR-12.1: a file left by an earlier run is not working now; idle, so a resumed tab is no ghost in Cooking.
  if (doc.state === 'ask' || doc.state === 'permission' || doc.state === 'busy') {
    isMainIdle = true
    await syncLive($)
  }
}

// FR-2.2: forgets a finished call; true when it was one a person waited on.
const settle = (useId: string) => [pendingAsk.delete(useId), pendingPermission.delete(useId)].some(Boolean)

// FR-4.5: a main-loop ask a person answers, while bound, is timed as a permission wait at its call end.
async function markPersonAsk($: $, useId: string, isMain: boolean): Promise<void> {
  if (!isMain) return
  await (async () => {
    if (await read($, bound)) await update($, personAsks, ids => [...ids, useId])
  })().catch(timingError($))
}

// The person ask whose dialog just opened, the oldest of the best rank: the same arguments and
// loop, then the same arguments (a call auto mode settled stays a candidate while it runs), then
// the same loop, then any loop (the classic agent_id and the tool.check agentId are not proven equal).
function dialogCandidateOf(tool: string, input: unknown, agentId: string | undefined): string | undefined {
  const args = JSON.stringify(input ?? null)
  const best: (string | undefined)[] = []
  for (const [useId, c] of dialogCandidates) {
    if (c.tool !== tool) continue
    const sameLoop = (c.agentId ?? '') === (agentId ?? '')
    const rank = c.input === args ? (sameLoop ? 0 : 1) : sameLoop ? 2 : 3
    best[rank] ??= useId
  }
  return best.find(Boolean)
}

// FR-2.4: the heartbeat; stats ignore a live file whose .beat is older than 30s.
async function beat($: $): Promise<void> {
  if (!(await isEnabled($))) return
  await $.fs.write(`${await workspaceDir($)}/.zenify/track/live/${await $.session.id()}.beat`, iso(await $.clock.now()))
}

// Called with `enabled` of every successful `zenify track stats --json` run.
export async function setTrackEnabled($: $, on: boolean): Promise<void> {
  const wasOn = await read($, enabled)
  await update($, enabled, () => on)
  // Turned back on: a state skipped while off (an ask answered then) is stale in live/<session>.json.
  // Only a tab that has written one is rewritten; a tab that never asked gets no file.
  if (on && !wasOn && liveState !== undefined) void syncLive($).catch(timingError($))
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
      // A printed id proves the feature is on, even before the first stats run sets `enabled`.
      if (!(await read($, enabled))) await update($, enabled, () => true)
      await update($, bound, () => ({ id, kind: kindOf(id), ending: false }))
      await appendSession($, bindLine(iso(now), id))
      // A bind runs inside a working turn: write busy with the new id, even after a mid-turn
      // session.start cleared liveState.
      await syncLive($)
      // The stats may have run before this bind; this session's open id shows now, not at a tick.
      void refreshStats($, true).catch(statsError($))
      return
    }
  }
  const endedId = tool === 'Bash' ? parseEndedId(stdoutOf(answer)) : undefined
  if (endedId) await update($, bound, b => (b?.id === endedId ? { ...b, ending: true } : b))
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
      void refreshStats($, true).catch(statsError($))
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

const timingError = ($: $) => (err: unknown) => $.ui.toast(`timing error: ${message(err)}`)

const firstLine = (s: string) => s.trim().split('\n')[0]

const localDate = (ms: number) => {
  const d = new Date(ms)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

async function mtimeOf($: $, path: string): Promise<string> {
  try {
    return String((await $.fs.stat(path)).mtimeMs)
  } catch (err) {
    if (!isMissing(err)) throw err
    return 'none'
  }
}

// FR-11.2–11.4: the status line from the last stats answer and this session's bound id.
// FR-11.8: the `tips` option, read once in register (a /config change reloads the module).
let showTips = true

async function updateStatus($: $): Promise<void> {
  const st = await read($, stats)
  if (!st) return
  if (st.error !== undefined || !st.data) {
    $.ui.status(errorStatus(st.error ?? 'no stats'))
    return
  }
  $.ui.status(statusText(st.data, await $.session.id(), (await read($, bound))?.id, showTips))
}

// FR-6.3/FR-11.5/FR-1.2: runs `zenify track stats --json` when log.md, sessions/, this session's own
// log (an append does not touch the directory's mtime), pulse (another tab's state or log line), the
// overlay or the local date changed since the last run, or on `force`. FR-1.4: while the last answer
// has a Rolling boil entry it also re-runs every 10s on an unchanged key. A failure is stored and
// shown (⚠), and is retried when a watched mtime changes or on the next forced run. Outside a
// workspace the pane says so; the status line stays unset.
// `fresh` (FR-13.7, opening the Backlog tab) also passes `--force`, bypassing the 5-minute backlog cache.
async function refreshStats($: $, force = false, fresh = false): Promise<void> {
  if (isStatting) {
    if (force) isRerunWanted = true
    if (fresh) isFreshWanted = true
    return
  }
  isStatting = true
  try {
    let data: TrackStats
    try {
      const ws = await findWorkspace($)
      if (!ws) {
        await update($, stats, () => ({ error: COPY.noWorkspace }))
        return
      }
      const track = `${ws}/.zenify/track`
      const now = await $.clock.now()
      const key = [
        await mtimeOf($, `${track}/log.md`),
        await mtimeOf($, `${track}/sessions`),
        await mtimeOf($, `${track}/sessions/${await $.session.id()}.log`),
        await mtimeOf($, `${track}/pulse`),
        await mtimeOf($, `${ws}/.zenify-overlay.yaml`),
        localDate(now),
      ].join('|')
      if (!force && key === lastKey) {
        const k = (await read($, stats))?.data?.kitchen
        // A live file can expire with no pulse: re-run every 10s while a tab waits on a person, and
        // every live TTL (30s) while tabs only cook, since a cooking beat expires no sooner.
        const every = (k?.rollingBoil?.length ?? 0) > 0 ? ROLLING_RERUN_MS : (k?.cooking?.length ?? 0) > 0 ? LIVE_TTL_MS : 0
        if (every === 0 || now - lastRunAt < every) return
      }
      lastKey = key
      lastRunAt = now
      const argv = ['zenify', 'track', 'stats', '--json', '--workspace', ws, ...(fresh ? ['--force'] : [])]
      const out = await $.process.run(argv, { timeoutMs: STATS_TIMEOUT_MS })
      if (out.exitCode !== 0) throw new Error(firstLine(out.stderr) || `zenify track stats: exit ${out.exitCode}`)
      data = parseTrackStats(out.stdout)
    } catch (err) {
      await update($, stats, () => ({ error: firstLine(message(err)) }))
      await updateStatus($)
      return
    }
    await update($, stats, () => ({ data }))
    if ((await read($, enabled)) !== data.enabled) await setTrackEnabled($, data.enabled)
    await updateStatus($)
  } finally {
    isStatting = false
    // Every exit path, early returns included, runs the queued forced run; the flag is cleared
    // first, so it recurses at most once per queued request.
    if (isRerunWanted) {
      const again = isFreshWanted
      isRerunWanted = false
      isFreshWanted = false
      await refreshStats($, true, again)
    }
  }
}

const statsError = ($: $) => (err: unknown) => $.ui.toast(`${COPY.toast}${message(err)}`)

// FR-12.3: the Cooking spinner turns every SPIN_TICK_MS, so the pane is drawn again while a tab cooks or waits.
async function spinCooking($: $): Promise<void> {
  // Only the Kitchen tab shows a spinner and the waiting seconds.
  const k = (await read($, stats))?.data?.kitchen
  const live = (k?.cooking?.length ?? 0) + (k?.rollingBoil?.length ?? 0)
  if (live > 0 && (await read($, tab)) === 'kitchen' && (await isOpen($))) $.ui.invalidate('ui.render')
}

function stopTimers(): void {
  for (const t of timers) t.cancel()
  timers = []
}

// FR-11.5: the status line follows the stats with the pane closed too, so the timers run for the
// whole session, not only while the pane is open.
function armTimers($: $): void {
  if (timers.length) return
  timers = [
    $.clock.every(WATCH_TICK_MS, () => void refreshStats($).catch(statsError($))),
    $.clock.every(SPIN_TICK_MS, () => void spinCooking($).catch(timingError($))),
    $.clock.every(MERGED_TICK_MS, () => void refreshStats($, true).catch(statsError($))),
    $.clock.every(BEAT_MS, () => void beat($).catch(timingError($))),
  ]
}

// Opens the pane with fresh numbers; the stats run is not awaited so the prompt is not held.
async function show($: $): Promise<void> {
  await $.ui.open({ ...PANE })
  wasShown = true
  void refreshStats($, true).catch(statsError($))
}

// FR-6.1: shown once in this process and closed now means the person closed it (by /track-pane or
// the pane's own close); auto-open leaves it closed, only /track-pane opens it again.
let wasShown = false
async function autoShow($: $): Promise<void> {
  if (wasShown && !(await isOpen($))) return
  await show($)
}

// FR-13.7: opening the Backlog tab reruns stats past the backlog cache; the run is not awaited.
async function selectTab($: $, id: Tab): Promise<void> {
  await update($, tab, () => id)
  if (id === 'backlog') void refreshStats($, true, true).catch(statsError($))
}

const isOpen = async ($: $) => (await $.ui.panes()).some(p => isPane(p.id))

const TONES: Record<NonNullable<Line['tone']>, Record<string, unknown>> = {
  dim: { dimColor: true },
  bold: { bold: true },
  warn: { color: 'yellow' },
  error: { color: 'red' },
}

export const register: Register = (on, options: PluginOptions) => {
  // FR-17.4: auto opens the pane on a tracked prompt, unless the person closed it; command only by /track-pane;
  // off neither, and /track-pane is not registered.
  const trackPane = trackPaneOf(options)
  showTips = options.tips !== false
  // FR-17.2: the /config rows' writes.
  registerConfig(on)
  // FR-11.1: every session in a workspace runs stats and sets the status line; never opens the pane.
  on('session.start', async ($, e, next) => {
    stopTimers()
    // FR-6.1: a person's close holds for one session; a new one (/clear included) auto-opens again.
    wasShown = false
    // FR-18: a start decides from its own cwd; only mid-session callers fall back to this workspace.
    workspaceCache = undefined
    lastKey = undefined
    pendingAsk.clear()
    pendingPermission.clear()
    dialogCandidates.clear()
    // runningAgents is kept: a session.start in this process is a compact or a clear, which a
    // background subagent outlives; a new process starts with an empty set.
    liveState = undefined
    liveId = undefined
    isMainIdle = false
    // FR-18: a failed workspace lookup is a ⚠ toast and counts as outside.
    const inWorkspace = !!(await findWorkspace($).catch(err => (statsError($)(err), undefined)))
    setInWorkspace(inWorkspace)
    $.ui.invalidate('config.describe')
    if (!inWorkspace) {
      // FR-18.3: outside a workspace znf shows nothing: no command, no sync, no stats, no status.
      $.ui.status(undefined)
      return next(e)
    }
    if (trackPane !== 'off') await $.command.register({ name: 'track-pane', description: 'Toggle the zenify track pane' })
    // FR-17.3: not awaited, like the stats run: the reads must not hold the session start.
    void syncConfig($, options).catch(statsError($))
    // Not awaited: the stats run may ask gh for merged PRs and must not hold the session start.
    void refreshStats($, true).catch(statsError($))
    armTimers($)
    // FR-2.1: a resumed session may find a stale ask/permission live file, and the beat keeps it
    // alive. Not awaited; it writes only when the file is there and stale.
    void resetStaleLive($).catch(timingError($))
    return next(e)
  })

  // FR-6.1/FR-17.4: the feature on, a cook/fix/hotfix/ship prompt opens the pane when trackPane is auto.
  on('prompt.submit', async ($, e, next) => {
    promptSeen = true
    if (await isEnabled($)) {
      await recordIdle($).catch(timingError($))
      // FR-12.1: a prompt starts work again.
      isMainIdle = false
      await syncLive($).catch(timingError($))
      if (isInWorkspace() && trackPane === 'auto' && isTrackPrompt(e)) await autoShow($).catch(statsError($))
    }
    return next(e)
  })

  // zenify track (FR-4, FR-8.2): every tool call of a session, timed around `next(e)`; the
  // permission check runs inside it (tool.check below), so a person's ask is within the span.
  on('tool.call', async ($, e, next) => {
    const tool = String(e.tool)
    const command = tool === 'Bash' ? String((e as { command?: unknown }).command ?? '') : ''
    // A start runs through even while `enabled` is still false, so its id is bound (FR-4.1).
    if (!isStartCommand(command) && !(await isEnabled($))) return next(e)
    const useId = e.tool_use_id
    const isMain = !e.agentId
    let started = 0
    try {
      if (tool === 'Skill') await recordPhase($, String((e as { skill?: unknown }).skill ?? ''))
      started = await $.clock.now()
    } catch (err) {
      timingError($)(err)
    }
    // FR-2.2: an AskUserQuestion waits for a person until its call ends.
    if (tool === 'AskUserQuestion' && useId) {
      pendingAsk.add(useId)
      await syncLive($).catch(timingError($))
    }
    try {
      const answer = await next(e)
      await recordCall($, tool, command, isMain, useId, started, answer).catch(timingError($))
      return answer
    } finally {
      if (useId) {
        await update($, personAsks, ids => ids.filter(x => x !== useId)).catch(timingError($))
        dialogCandidates.delete(useId)
        // Answered, allowed, denied or interrupted: back to busy unless another wait is open.
        if (settle(useId)) await syncLive($).catch(timingError($))
      }
    }
  })

  // FR-4.5: a main-loop ask that a person answers (not the host, not a classifier).
  on('tool.check', async ($, e, next) => {
    const verdict = await next(e)
    const useId = e.tool_use_id
    const isPersonAsk = !!useId && verdict.decision === 'ask' && e.approver === 'person'
    if (!isPersonAsk || !(await isEnabled($))) return verdict
    // AskUserQuestion always reaches the person; any other ask waits for its dialog (PermissionRequest).
    if (e.tool === 'AskUserQuestion') await markPersonAsk($, useId, !e.agentId)
    else dialogCandidates.set(useId, { tool: e.tool, input: JSON.stringify(e.input ?? null), agentId: e.agentId })
    return verdict
  })

  // FR-2.2: Claude Code raises PermissionRequest only when it is about to ask the person, so an ask
  // that auto mode's classifier settles never marks the tab as waiting. Any person's dialog, main
  // loop or subagent, bound or not, marks it.
  on('classic.PermissionRequest', async ($, e, next) => {
    const useId = dialogCandidateOf(e.tool_name, e.tool_input, e.agent_id)
    if (useId && (await isEnabled($))) {
      const isMain = !dialogCandidates.get(useId)?.agentId
      dialogCandidates.delete(useId)
      await markPersonAsk($, useId, isMain)
      pendingPermission.add(useId)
      await syncLive($).catch(timingError($))
    }
    return next(e)
  })

  // FR-12.1: a subagent still running (a background one outlives the main turn) keeps the tab cooking.
  on('classic.SubagentStart', async ($, e, next) => {
    runningAgents.add(e.agent_id)
    await syncLive($).catch(timingError($))
    return next(e)
  })
  on('classic.SubagentStop', async ($, e, next) => {
    if (runningAgents.delete(e.agent_id)) await syncLive($).catch(timingError($))
    return next(e)
  })

  on('turn.complete', async ($, e, next) => {
    const result = await next(e)
    if (await isEnabled($)) {
      await recordTurn($, e).catch(timingError($))
      if (!e.agentId) {
        // Safety net: a wait whose tool.call never ran must not leave the tab waiting past the turn.
        pendingAsk.clear()
        pendingPermission.clear()
        // A main-loop ask denied or interrupted before its call ran; a subagent's may still open.
        for (const [useId, c] of dialogCandidates) if (!c.agentId) dialogCandidates.delete(useId)
        // FR-12.1: the main loop is done; a tab waiting for the next prompt is not cooking.
        isMainIdle = true
        await syncLive($).catch(timingError($))
      }
    }
    return result
  })

  on('command.run', { command: 'track-pane' }, async $ => {
    if (await isOpen($)) {
      await $.ui.close({ id: PANE.id })
      return { text: COPY.closed }
    }
    await show($)
    return { text: COPY.opened }
  })

  on('ui.render', { component: 'Pane', requestId: PANE.id }, async ($, e) => {
    const { Box, Button, Link, Text } = $.ui.resolve(e)
    const selected = await read($, tab)
    const st = await read($, stats)
    // A link line is a real OSC 8 hyperlink: the label is short, so truncation never cuts the href.
    const line = (l: Line) => (
      <Text wrap="truncate-end" {...(l.tone ? TONES[l.tone] : {})}>
        {/* An empty Text renders zero rows, so a section gap holds a space. */}
        {l.text || (l.href ? '' : ' ')}
        {l.href && <Link href={l.href} label={l.label} />}
      </Text>
    )
    // FR-6.4: an error or the feature off is said in the pane, never an empty body.
    const body: Line[] = !st
      ? [{ text: COPY.loading, tone: 'dim' }]
      : st.error !== undefined || !st.data
        ? [{ text: errorStatus(st.error ?? 'no stats'), tone: 'error' }]
        : !st.data.enabled
          ? [{ text: STATUS_OFF, tone: 'dim' }]
          : tabLines(selected, st.data, await $.session.id(), (await read($, bound))?.id, await $.clock.now())
    // The main screen reports no clicks, so an unfocused pane there says how to reach the tabs by keys.
    const showTabKeys = e.surface === 'terminal' && e.viewport?.isFullscreen === false && !e.props.isFocused
    return (
      <Box flexDirection="column" paddingX={1}>
        <Box flexDirection="row" marginBottom={showTabKeys ? 0 : 1}>
          {TABS.map((t, i) => [
            i > 0 && <Text dimColor> │ </Text>,
            // A Button has no color prop, so the selected tab is a colored bold Text.
            t.id === selected ? (
              <Text color="claude" bold>{`▸${t.label}`}</Text>
            ) : (
              // The hotkey (its 1-based place) presses it while the pane holds the keyboard.
              <Button key={`tab:${t.id}`} label={t.label} hotkey={String(i + 1)} plain dimColor onPress={() => selectTab($, t.id)} />
            ),
          ])}
        </Box>
        {showTabKeys && (
          <Box marginBottom={1}>
            <Text dimColor wrap="truncate-end">
              {COPY.tabKeys}
            </Text>
          </Box>
        )}
        {body.map(line)}
      </Box>
    )
  })
}
