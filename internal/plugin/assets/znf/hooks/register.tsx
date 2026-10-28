import { atom, read, update } from 'claude-code'
import type { EngineInterface as $, Register, Timer } from 'claude-code'

import { parseLedger } from './ledger'
import type { Ledger } from './ledger'
import { isMergeable, isSettled, issueUrl, mergeArgv, mergeCommand, prBase, prLabel, prRef, prUrls } from './pr'

const PANES = [
  { id: 'nonstop-human', title: 'Need Human', columns: 44 },
  { id: 'nonstop-progress', title: 'In Progress', columns: 44 },
  { id: 'nonstop-done', title: 'Done', columns: 44 },
] as const
const isTab = (id: string) => PANES.some(p => p.id === id)
const LEGACY_PANE_ID = 'nonstop'
const SPINNER = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏']
const SPIN_TICK_MS = 250
const LEDGER_TICK_MS = 2000
const PR_TICK_MS = 60000
const GH_TIMEOUT_MS = 20000
const MERGE_TIMEOUT_MS = 60000
const CONFIRM_MS = 5000
const GH_PARALLEL = 4
const GH_FIELDS = 'state,mergeStateStatus,statusCheckRollup,reviewDecision,baseRefName'

// Change the shape tag when Ledger or the PR map changes: it declines what an
// earlier version stored, which outlives a reload.
const ledger = atom({ plugin: 'znf', key: 'ledger' } as const, null, { shape: 'nonstop-2' })
const ledgerError = atom({ plugin: 'znf', key: 'ledgerError' } as const, null, { shape: 'nonstop-1' })
const prs = atom({ plugin: 'znf', key: 'prs' } as const, {}, { shape: 'nonstop-1' })
const bases = atom({ plugin: 'znf', key: 'bases' } as const, {}, { shape: 'nonstop-1' })
// url -> epoch ms until which its merge button is armed. An expiry, not a flag: a reload
// cancels the disarm timer but keeps this state, so the time itself must end the arming.
const confirming = atom({ plugin: 'znf', key: 'confirming' } as const, {}, { shape: 'nonstop-2' })
// Urls whose merge runs now; mirrors `mergeInFlight` so the pane redraws.
const merging = atom({ plugin: 'znf', key: 'merging' } as const, [], { shape: 'nonstop-1' })
const frame = atom({ plugin: 'znf', key: 'frame' } as const, 0, { shape: 'nonstop-1' })
// Ids of the task lines the user expanded; shared by the three tabs.
const expanded = atom({ plugin: 'znf', key: 'expanded' } as const, [], { shape: 'nonstop-1' })

// Module state: lost on a reload, which also drops the timers; the next open re-arms them.
let timers: Timer[] = []
let spinTimer: Timer | undefined
let ledgerFile: string | undefined
let lastMtime: number | undefined
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

async function viewPr($: $, url: string): Promise<{ label: string; base?: string }> {
  try {
    const view = await $.process.run(['gh', 'pr', 'view', url, '--json', GH_FIELDS], { timeoutMs: GH_TIMEOUT_MS })
    return { label: prLabel(view), base: prBase(view) }
  } catch (err) {
    return { label: `gh lỗi: ${message(err).split('\n')[0]}` }
  }
}

async function pollPr($: $, url: string): Promise<void> {
  const { label, base } = await viewPr($, url)
  await update($, prs, map => ({ ...map, [url]: label }))
  if (base) await update($, bases, map => ({ ...map, [url]: base }))
}

// FR-2.2: every PR of the last section, at most GH_PARALLEL gh calls at once.
async function pollPrs($: $): Promise<void> {
  if (isPolling) return
  isPolling = true
  try {
    const current = await read($, ledger)
    const known = await read($, prs)
    const queue = current ? prUrls(current.tasks).filter(url => !isSettled(known[url] ?? '')) : []
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
    if (still !== until || now >= until) return
    await update($, confirming, map => omit(map, url))
    await update($, merging, urls => [...urls, url])
    const out = await $.process.run(mergeArgv(url), { timeoutMs: MERGE_TIMEOUT_MS })
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
  if (timers.length) return
  timers = [
    $.clock.every(LEDGER_TICK_MS, () => void refresh($).catch(err => report($, err))),
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
    // A reload keeps shaped state but drops timers and the in-flight set: start the merge UI clean.
    mergeInFlight.clear()
    await update($, merging, () => [])
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
      const labels = await read($, prs)
      const open = await read($, expanded)
      const armedUntil = await read($, confirming)
      const inMerge = await read($, merging)
      const now = await $.clock.now()
      const baseOf = await read($, bases)

      if (error) return <Box flexDirection="column">{line(`lỗi ledger: ${error}`, { color: 'red' })}</Box>
      if (!l) return <Box flexDirection="column">{line('chưa có ledger nonstop', { dimColor: true })}</Box>

      const label = (u: string) => labels[u] ?? ''
      const urls = prUrls(l.tasks)
      const mergeable = urls.filter(u => isMergeable(label(u)))
      const active = urls.filter(u => !isSettled(label(u)) && !isMergeable(label(u)))
      const settled = urls.filter(u => isSettled(label(u)))
      const by = (s: string) => byPriority(l.tasks.filter(t => t.status === s))
      const isArmed = (url: string) => now < (armedUntil[url] ?? 0)
      const mergeLabel = (url: string) =>
        inMerge.includes(url) ? 'đang merge…' : isArmed(url) ? `xác nhận → ${baseOf[url]}?` : 'merge'
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
                    await $.ui.copy({ text: mergeCommand(url), surface: press.surface })
                    $.ui.toast('đã copy lệnh merge')
                  }}
                />
                {baseOf[url] && (
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

      if (id === 'nonstop-done') {
        const done = by('done')
        if (!done.length && !settled.length) return empty()
        return (
          <Box flexDirection="column">
            {done.length > 0 && line('DONE', { bold: true })}
            {done.map(t => taskLine(t, t.evidence))}
            {settled.length > 0 && line('PR', { bold: true })}
            {settled.map(url => prRow(url, label(url)))}
          </Box>
        )
      }

      const running = by('running')
      const queued = by('queued')
      const spin = SPINNER[await read($, frame)]
      const isIdle = !running.length && !queued.length && !active.length && !l.malformed.length
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
          {active.length > 0 && line('PR', { bold: true })}
          {active.map(url => prRow(url, label(url)))}
          {line(`done ${by('done').length} · queued ${queued.length} · blocked ${by('blocked').length}`, { dimColor: true })}
          {l.malformed.length > 0 && line(`⚠ ${l.malformed.length} dòng không đúng format`, { color: 'yellow' })}
          {l.malformed.slice(0, 3).map(m => line(m, { dimColor: true }))}
        </Box>
      )
    })
  }
}
