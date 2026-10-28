import { atom, read, update } from 'claude-code'
import type { EngineInterface as $, Register, Timer } from 'claude-code'

import { parseLedger } from './ledger'
import type { Ledger } from './ledger'
import { isMergeable, isSettled, mergeCommand, prLabel, prRef, prUrls } from './pr'

const PANE = { id: 'nonstop', title: 'Nonstop', columns: 44 } as const
const LEDGER_TICK_MS = 2000
const PR_TICK_MS = 60000
const GH_TIMEOUT_MS = 20000
const GH_PARALLEL = 4
const GH_FIELDS = 'state,mergeStateStatus,statusCheckRollup,reviewDecision'

// Change the shape tag when Ledger or the PR map changes: it declines what an
// earlier version stored, which outlives a reload.
const ledger = atom({ plugin: 'znf', key: 'ledger' } as const, null, { shape: 'nonstop-1' })
const ledgerError = atom({ plugin: 'znf', key: 'ledgerError' } as const, null, { shape: 'nonstop-1' })
const prs = atom({ plugin: 'znf', key: 'prs' } as const, {}, { shape: 'nonstop-1' })

// Module state: lost on a reload, which also drops the timers; the next open re-arms them.
let timers: Timer[] = []
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

async function viewPr($: $, url: string): Promise<string> {
  try {
    const view = await $.process.run(['gh', 'pr', 'view', url, '--json', GH_FIELDS], { timeoutMs: GH_TIMEOUT_MS })
    return prLabel(view)
  } catch (err) {
    return `gh lỗi: ${message(err).split('\n')[0]}`
  }
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
        const label = await viewPr($, url)
        await update($, prs, map => ({ ...map, [url]: label }))
      }
    }
    await Promise.all(Array.from({ length: Math.min(GH_PARALLEL, queue.length) }, worker))
  } finally {
    isPolling = false
  }
}

function report($: $, err: unknown): void {
  void update($, ledgerError, () => message(err))
}

function stopTimers(): void {
  for (const t of timers) t.cancel()
  timers = []
}

// FR-1.2: the timers start with the first open and stop when the pane closes.
async function openPane($: $): Promise<void> {
  await $.ui.open({ ...PANE })
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

const cut = (s: string, cols: number) => (s.length > cols ? `${s.slice(0, Math.max(0, cols - 1))}…` : s)

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    stopTimers()
    ledgerFile = undefined
    lastMtime = undefined
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
    const isUp = (await $.ui.panes()).some(p => p.id === PANE.id)
    if (isUp) {
      await $.ui.close({ id: PANE.id })
      return { text: 'đã đóng pane nonstop' }
    }
    await show($)
    return { text: 'đã mở pane nonstop' }
  })

  on('ui.close', async ($, e, next) => {
    if (e.id === PANE.id) stopTimers()
    return next(e)
  })

  on('ui.render', { component: 'Pane', requestId: 'nonstop' }, async ($, e) => {
    const { Box, Button, Text } = $.ui.resolve(e)
    const cols = Math.max(1, e.props.bodyColumns)
    const line = (s: string, props: Record<string, unknown> = {}) => <Text {...props}>{cut(s, cols)}</Text>
    const error = await read($, ledgerError)
    const l = await read($, ledger)
    const labels = await read($, prs)

    if (error) return <Box flexDirection="column">{line(`lỗi ledger: ${error}`, { color: 'red' })}</Box>
    if (!l) return <Box flexDirection="column">{line('chưa có ledger nonstop', { dimColor: true })}</Box>

    const count = (s: string) => l.tasks.filter(t => t.status === s).length
    const running = l.tasks.filter(t => t.status === 'running')
    const all = prUrls(l.tasks)
    const urls = all.filter(u => !isSettled(labels[u] ?? ''))
    const merged = all.filter(u => labels[u] === 'MERGED').length
    const firstMergeable = urls.find(u => isMergeable(labels[u] ?? ''))

    return (
      <Box flexDirection="column">
        {running.length > 0 && line('RUNNING', { bold: true })}
        {running.map(t => line(`${t.id} ${t.repo} ${t.skill} ${t.worker}`))}
        {l.needs.length > 0 && line('CẦN ANH', { bold: true, color: 'yellow' })}
        {l.needs.map(n => line(n.id ? `${n.id} ${n.question}` : n.question))}
        {urls.length > 0 && line('PR', { bold: true })}
        {urls.map(url => {
          const label = labels[url] ?? ''
          if (!isMergeable(label)) return line(`${prRef(url)} ${label}`)
          return (
            <Box flexDirection="row">
              {line(`${prRef(url)} ${label}`)}
              <Button
                key={url}
                label="c"
                hotkey={url === firstMergeable ? 'c' : undefined}
                onPress={async press => {
                  await $.ui.copy({ text: mergeCommand(url), surface: press.surface })
                  $.ui.toast('đã copy lệnh merge')
                }}
              />
            </Box>
          )
        })}
        {line(`done ${count('done')} · queued ${count('queued')} · blocked ${count('blocked')}`, { dimColor: true })}
        {merged > 0 && line(`✓ ${merged} PR đã merge`, { dimColor: true })}
        {l.malformed.length > 0 && line(`⚠ ${l.malformed.length} dòng không đúng format`, { color: 'yellow' })}
        {l.malformed.slice(0, 3).map(m => line(m, { dimColor: true }))}
      </Box>
    )
  })
}
