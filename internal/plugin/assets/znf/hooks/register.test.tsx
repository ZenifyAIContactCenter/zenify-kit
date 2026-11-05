import { expect, mock, test } from 'claude-code/testing'
import { COPY, TABS } from './track-ui'

const CWD = '/w/app'
const WS = '/w'
// The workspace marker sits one level above the cwd; session logs go under it.
const WS_MARKER = '/w/.zenify/manifest.json'
const SESSION = 'aaaaaaaa-1111-2222-3333-444444444444'
const OTHER = 'bbbbbbbb-5555-6666-7777-888888888888'
const SESSION_LOG = `/w/.zenify/track/sessions/${SESSION}.log`
const LOG_MD = '/w/.zenify/track/log.md'
const SESSIONS_DIR = '/w/.zenify/track/sessions'
const OVERLAY = '/w/.zenify-overlay.yaml'
const PULSE = '/w/.zenify/track/pulse'
const LIVE = `/w/.zenify/track/live/${SESSION}.json`
const BEAT = `/w/.zenify/track/live/${SESSION}.beat`
const STATS_ARGV = ['zenify', 'track', 'stats', '--json', '--workspace', WS]
const PANE = { id: 'track', title: 'Track', columns: 44 }
const OFF = '⏸ track off · enable: /config → znf'
const MIN = 60000

type Run = { exitCode: number; stdout: string; stderr: string }

type World = {
  // The answer to `zenify track stats --json`; defaults to an enabled, empty result.
  stats?: Run
  // `zenify track stats` cannot start.
  statsRejects?: string
  // A stored `tab` atom value as the kit keeps a shaped atom: {shape, value}.
  storedTab?: { shape: string; value: string }
  // mtimeMs by path for the watched files; absent = ENOENT.
  mtimes?: Record<string, number>
  // No `.zenify/manifest.json` at or above the cwd.
  noWorkspace?: boolean
  // Files the plugin wrote, by path (session logs live here).
  files?: Record<string, string>
  // Answers the test's tool.call beneath the plugin; defaults to an empty result.
  toolCall?: (e: any) => unknown
  // The verdict beneath the plugin's tool.check; defaults to allow.
  verdict?: 'allow' | 'ask' | 'deny'
  // Forces the plugin's `enabled` atom read; undefined = the atom as the plugin wrote it.
  trackEnabled?: boolean
  writeError?: string
  // Holds the stat of log.md until this resolves (a stats run in flight).
  holdLogStat?: Promise<void>
  // Holds the answer of a read of this session's live json after its content is taken (a read in flight).
  holdLiveRead?: Promise<void>
}

type Calls = {
  registered: string[]
  open: unknown[]
  close: unknown[]
  run: string[][]
  status: (string | undefined)[]
  toast: string[]
  write: { path: string; text: string }[]
}

const item = (over: Record<string, unknown> = {}) => ({
  id: 'cook-261030-c0de',
  kind: 'cook',
  repo: 'kit',
  est: 'M',
  session: SESSION,
  cwd: '/w/repos/app',
  phase: 'plan',
  status: 'running',
  startedAt: '2026-10-30T01:00:00Z',
  endedAt: '',
  prs: null,
  agentMin: 12,
  wallMin: 12,
  tab: '',
  wait: { ask: 0, permission: 0, idle: 0, total: 0 },
  phases: { plan: { agentMin: 12, waitMin: 0 } },
  tokens: {},
  ...over,
})

const statsJson = (over: Record<string, unknown> = {}) =>
  JSON.stringify({
    enabled: true,
    malformed: 0,
    scale: { source: 'default', n: 0, S: 30, M: 90, L: 240 },
    rules: [],
    today: { date: '2026-10-30', plans: {}, started: {}, ended: {}, tasks: 0, rounds: 0, prsOpened: 0, merged: 0, mergedError: '', items: [], dailyPlan: { items: [], estMin: 0, wallMin: 0 }, prOpen: [], prStatusError: '' },
    running: [],
    estimate: { byKind: {}, hotfix: { n: 0, medianMin: null, p90Min: null }, phaseShare: {} },
    quality: { byKind: {}, fix: { n: 0, roundAvg: null, wideRate: null, regressionRate: null } },
    backlog: { items: [], ghError: '' },
    onboarded: true,
    ...over,
  })
const ok = (stdout: string): Run => ({ exitCode: 0, stdout, stderr: '' })
const statsRuns = (calls: Calls) => calls.run.filter(a => a[0] === 'zenify' && a[1] === 'track')
const lastStatus = (calls: Calls) => calls.status.at(-1)

// Every hook here stands for the engine beneath the plugin.
function engine(on: any, world: World): Calls {
  const calls: Calls = { registered: [], open: [], close: [], run: [], status: [], toast: [], write: [] }
  const panes: { id: string }[] = []
  on('command.register', (_: unknown, e: { name: string }) => {
    calls.registered.push(e.name)
    return { value: { command: e.name } }
  })
  on('session.start', (_: unknown, e: { cwd: string }) => ({ cwd: e.cwd }))
  on('session.cwd', () => ({ value: CWD }))
  on('prompt.submit', (_: unknown, e: { text: string }) => ({ text: e.text }))
  on('session.id', () => ({ value: SESSION }))
  on('tool.call', async (_: unknown, e: any) => (world.toolCall ? await world.toolCall(e) : { result: {} }))
  on('tool.check', () => ({ decision: world.verdict ?? 'allow' }))
  // A test's $ has no state noun: answer the `enabled` atom's read as the kit stores a shaped atom.
  on('state.get', (_: unknown, e: { plugin: string; key: string }, next: any) =>
    e.plugin === 'znf' && e.key === 'enabled' && world.trackEnabled !== undefined
      ? { value: { value: { shape: 'track-1', value: world.trackEnabled }, version: 1 } }
      : e.plugin === 'znf' && e.key === 'tab' && world.storedTab !== undefined
        ? { value: { value: world.storedTab, version: 1 } }
        : next(e),
  )
  on('turn.complete', (_: unknown, e: { answer: string }) => ({ text: e.answer }))
  on('classic.PermissionRequest', () => ({}))
  on('classic.SubagentStart', () => ({}))
  on('classic.SubagentStop', () => ({}))
  // FR-17.3: the writer beneath the session-start sync of the /config rows.
  on('config.set', (_: unknown, e: { value: unknown }) => ({ value: e.value }))
  on('fs.write', (_: unknown, e: { path: string; text: string }) => {
    if (world.writeError) return { deny: world.writeError }
    calls.write.push({ path: e.path, text: e.text })
    world.files = { ...world.files, [e.path]: e.text }
    return { value: undefined }
  })
  on('ui.open', (_: unknown, e: { id: string }) => {
    calls.open.push(e)
    if (!panes.some(p => p.id === e.id)) panes.push({ id: e.id })
    return { value: { isPlaced: true } }
  })
  on('ui.close', (_: unknown, e: { id: string }) => {
    calls.close.push(e)
    const at = panes.findIndex(p => p.id === e.id)
    if (at >= 0) panes.splice(at, 1)
    return { value: undefined }
  })
  on('ui.panes', () => ({
    value: panes.map(p => ({ id: p.id, title: 'Track', isShown: true, isFocused: false, isPlaced: true })),
  }))
  on('ui.status', (_: unknown, e: { text: string | undefined }) => {
    calls.status.push(e.text)
    return { value: undefined }
  })
  on('ui.toast', (_: unknown, e: { text: string }) => {
    calls.toast.push(e.text)
    return { value: undefined }
  })
  on('fs.stat', async (_: unknown, e: { path: string }) => {
    if (e.path === LOG_MD && world.holdLogStat) await world.holdLogStat
    if (e.path === WS_MARKER && !world.noWorkspace) return { value: { kind: 'file', size: 1, mtimeMs: 1, isLink: false } }
    const m = world.mtimes?.[e.path]
    if (m === undefined) return { deny: `ENOENT: no such file, stat '${e.path}'` }
    return { value: { kind: e.path === SESSIONS_DIR ? 'directory' : 'file', size: 1, mtimeMs: m, isLink: false } }
  })
  on('fs.read', async (_: unknown, e: { path: string }) => {
    const text = world.files?.[e.path]
    if (e.path === LIVE && world.holdLiveRead) await world.holdLiveRead
    if (text === undefined) return { deny: `ENOENT: no such file, open '${e.path}'` }
    return { value: text }
  })
  on('process.run', async (_: unknown, e: { argv: string[] }) => {
    calls.run.push([...e.argv])
    // FR-17.3: the sources the /config rows are synced from, in step with the defaults.
    if (e.argv.join(' ') === 'zenify feature') return { value: ok('github.issues off\ntrack off\n') }
    if (e.argv.join(' ') === 'zenify reviewer get') return { value: ok('agent\n') }
    if (e.argv[0] === 'zenify') {
      if (world.statsRejects) return { deny: world.statsRejects }
      return { value: world.stats ?? ok(statsJson()) }
    }
    return { value: { exitCode: 127, stdout: '', stderr: `unexpected ${e.argv[0]}` } }
  })
  return calls
}

const pane = (bodyColumns = 44, bodyRows = 40) =>
  ({
    plugin: 'znf',
    component: 'Pane',
    requestId: 'track',
    surface: 'terminal',
    props: {
      title: 'Track',
      isFocused: false,
      bodyColumns,
      placement: 'dock',
      scroll: { offset: 0, bodyRows },
      view: {},
    },
  }) as const

// Mounts the pane on the terminal; tabs are switched by pressing `tab:<id>` on it.
const mountTab = ($: any) => $.ui.mount(pane())

const allTexts = async (ui: { findAll: (q: { type?: string }) => Promise<{ type: string; text?: string }[]> }) =>
  (await ui.findAll({})).filter(e => e.type === 'Text' && e.text !== ' │ ').map(e => e.text ?? '')

async function start($: any, clock: { settle: () => Promise<void> }) {
  await $.session.start({ cwd: CWD, source: 'startup' } as any)
  await clock.settle()
}

test('SC-20: feature off at session start sets the off status line and opens no pane', async ($, on) => {
  const calls = engine(on, { stats: ok(statsJson({ enabled: false, rules: null, running: null })) })
  const clock = mock.clock(on)
  await start($, clock)
  expect(statsRuns(calls)).toEqual([STATS_ARGV])
  expect(lastStatus(calls)).toBe(OFF)
  expect(calls.open).toHaveLength(0)
})

test('FR-11.1: feature on, session start still opens no pane and shows the ⏸ line', async ($, on) => {
  const calls = engine(on, {})
  const clock = mock.clock(on)
  await start($, clock)
  expect(lastStatus(calls)).toBe('track ⏸ · Today: 0/0')
  expect(calls.open).toHaveLength(0)
})

test('FR-11.1: outside a zenify workspace nothing runs and no status is set', async ($, on) => {
  const calls = engine(on, { noWorkspace: true })
  const clock = mock.clock(on)
  await start($, clock)
  expect(statsRuns(calls)).toHaveLength(0)
  expect(calls.status.filter(Boolean)).toHaveLength(0)
  // FR-18.3: no /track-pane, no row sync (no process at all), the status line cleared.
  expect(calls.registered).toEqual([])
  expect(calls.run).toEqual([])
  expect(calls.status).toEqual([undefined])
})

test('FR-18.3: outside a workspace a cook prompt does not open the pane', async ($, on) => {
  const calls = engine(on, { noWorkspace: true, trackEnabled: true })
  const clock = mock.clock(on)
  await start($, clock)
  await $.prompt.submit({ text: 'cook add a thing' } as any)
  expect(calls.open).toHaveLength(0)
})

test('FR-18.3: inside a workspace /track-pane is registered', async ($, on) => {
  const calls = engine(on, {})
  const clock = mock.clock(on)
  await start($, clock)
  expect(calls.registered).toEqual(['track-pane'])
})

test('SC-21: this session bound to a cook id in plan, agent 12m est M: status has ▶, id, plan, 12m/M', async ($, on) => {
  const world: World = { mtimes: { [SESSIONS_DIR]: 1 } }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  world.toolCall = () => ({ result: { stdout: 'track: cook-261030-c0de\n', stderr: '' }, text: '' })
  await $.tool.call({ tool: 'Bash', command: 'zenify track start --kind cook --repo kit --est M' })
  world.toolCall = undefined
  world.stats = ok(statsJson({ running: [item()] }))
  world.mtimes = { [SESSIONS_DIR]: 2 }
  await clock.advance(2000)
  const line = lastStatus(calls) ?? ''
  expect(line.startsWith('track ▶')).toBe(true)
  expect(line).toContain('cook-261030-c0de')
  expect(line).toContain('plan')
  expect(line).toContain('12m/M')
})

test('SC-21/FR-11.5: a bind after the stats run shows ▶ with the id at once, no tick needed', async ($, on) => {
  const world: World = {}
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  expect(lastStatus(calls)).toBe('track ⏸ · Today: 0/0')
  // The CLI wrote log.md and the watcher already ran; only the bind is left.
  world.stats = ok(statsJson({ running: [item()] }))
  world.toolCall = () => ({ result: { stdout: 'track: cook-261030-c0de\n', stderr: '' }, text: '' })
  await $.tool.call({ tool: 'Bash', command: 'zenify track start --kind cook --repo kit --est M' })
  await clock.settle()
  expect(statsRuns(calls)).toHaveLength(2)
  expect((lastStatus(calls) ?? '').startsWith('track ▶ · cook-261030-c0de')).toBe(true)
})

test('a bind while a tick run is in flight is not lost when that run ends on an unchanged key', async ($, on) => {
  const world: World = {}
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  expect(statsRuns(calls)).toHaveLength(1)
  let release = () => {}
  world.holdLogStat = new Promise<void>(r => (release = r))
  await clock.advance(2000)
  // The tick's run is held on its stat; the bind asks for a forced run meanwhile.
  world.stats = ok(statsJson({ running: [item()] }))
  world.toolCall = () => ({ result: { stdout: 'track: cook-261030-c0de\n', stderr: '' }, text: '' })
  await $.tool.call({ tool: 'Bash', command: 'zenify track start --kind cook --repo kit --est M' })
  world.toolCall = undefined
  expect(statsRuns(calls)).toHaveLength(1)
  world.holdLogStat = undefined
  release()
  await clock.settle()
  expect(statsRuns(calls)).toHaveLength(2)
  expect((lastStatus(calls) ?? '').startsWith('track ▶ · cook-261030-c0de')).toBe(true)
})

test('FR-11.5: an unbind re-runs stats at once', async ($, on) => {
  const world: World = {}
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  world.toolCall = () => ({ result: { stdout: 'track: cook-261030-c0de\n', stderr: '' }, text: '' })
  await $.tool.call({ tool: 'Bash', command: 'zenify track start --kind cook --repo kit --est M' })
  world.toolCall = () => ({ result: { stdout: 'track: ended cook-261030-c0de\n', stderr: '' }, text: '' })
  await $.tool.call({ tool: 'Bash', command: 'zenify track end cook-261030-c0de --pr https://github.com/Z/kit/pull/1 --rounds 1' })
  world.toolCall = undefined
  await clock.settle()
  const before = statsRuns(calls).length
  await $.turn.complete({ answer: 'ok', durationMs: 1000, isAborted: false, turnId: 't-1', reason: 'answer' } as any)
  await clock.settle()
  expect(statsRuns(calls)).toHaveLength(before + 1)
})

test('FR-6.3: an append to this session own log re-runs stats at the next tick', async ($, on) => {
  const world: World = { mtimes: { [SESSIONS_DIR]: 1, [SESSION_LOG]: 1 } }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  await clock.advance(2000)
  expect(statsRuns(calls)).toHaveLength(1)
  world.mtimes = { [SESSIONS_DIR]: 1, [SESSION_LOG]: 2 }
  await clock.advance(2000)
  expect(statsRuns(calls)).toHaveLength(2)
})

test('FR-6.4: outside a workspace track-pane says so instead of loading forever; no status line', async ($, on) => {
  const calls = engine(on, { noWorkspace: true })
  const clock = mock.clock(on)
  await start($, clock)
  await $.command.run({ command: 'track-pane', args: '' } as any)
  await clock.settle()
  const ui = await mountTab($)
  expect(await allTexts(ui)).toContain('⚠ track error: not in a zenify workspace')
  expect(statsRuns(calls)).toHaveLength(0)
  expect(calls.status.filter(Boolean)).toHaveLength(0)
})

test('FR-11.3: an open id of another session leaves this one at ⏸', async ($, on) => {
  const calls = engine(on, { stats: ok(statsJson({ running: [item({ session: OTHER })] })) })
  const clock = mock.clock(on)
  await start($, clock)
  expect(lastStatus(calls)).toBe('track ⏸ · Today: 0/0')
})

test('FR-11.4: a failing stats run is the ⚠ status line with its first stderr line', async ($, on) => {
  const calls = engine(on, { stats: { exitCode: 1, stdout: '', stderr: 'parse log.md: boom\nmore' } })
  const clock = mock.clock(on)
  await start($, clock)
  expect(lastStatus(calls)).toBe('⚠ track error: parse log.md: boom')
})

test('FR-11.4: stats output that is not JSON is the ⚠ line, not swallowed', async ($, on) => {
  const calls = engine(on, { stats: ok('track: off') })
  const clock = mock.clock(on)
  await start($, clock)
  expect((lastStatus(calls) ?? '').startsWith('⚠ track error: ')).toBe(true)
})

test('FR-11.4: a stats command that cannot start is the ⚠ line', async ($, on) => {
  const calls = engine(on, { statsRejects: 'ENOENT: zenify not found' })
  const clock = mock.clock(on)
  await start($, clock)
  expect((lastStatus(calls) ?? '').startsWith('⚠ track error: ')).toBe(true)
  expect(lastStatus(calls)).toContain('zenify not found')
})

test('FR-11.5: an overlay mtime change re-runs stats, so feature on shows at once; no change runs nothing', async ($, on) => {
  const world: World = { stats: ok(statsJson({ enabled: false })), mtimes: { [OVERLAY]: 1 } }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  await clock.advance(6000)
  expect(statsRuns(calls)).toHaveLength(1)
  world.stats = ok(statsJson())
  world.mtimes = { [OVERLAY]: 2 }
  await clock.advance(2000)
  expect(statsRuns(calls)).toHaveLength(2)
  expect(lastStatus(calls)).toBe('track ⏸ · Today: 0/0')
})

test('FR-6.3: log.md or sessions/ mtime changes re-run stats with the pane closed', async ($, on) => {
  const world: World = { mtimes: { [LOG_MD]: 1, [SESSIONS_DIR]: 1 } }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  world.mtimes = { [LOG_MD]: 2, [SESSIONS_DIR]: 1 }
  await clock.advance(2000)
  expect(statsRuns(calls)).toHaveLength(2)
  world.mtimes = { [LOG_MD]: 2, [SESSIONS_DIR]: 5 }
  await clock.advance(2000)
  expect(statsRuns(calls)).toHaveLength(3)
  expect(calls.open).toHaveLength(0)
})

test('FR-6.3: stats re-run every 5 minutes for merged PRs and when the local date changes', async ($, on) => {
  const world: World = {}
  const calls = engine(on, world)
  const clock = mock.clock(on, { now: new Date(2026, 9, 30, 23, 59, 57).getTime() })
  await start($, clock)
  await clock.advance(4000)
  expect(statsRuns(calls)).toHaveLength(2)
  await clock.advance(5 * MIN)
  expect(statsRuns(calls)).toHaveLength(3)
})

test('FR-2.3: the enabled flag comes from the stats run: on records, off does not', async ($, on) => {
  const world: World = {}
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  world.toolCall = () => ({ result: { stdout: 'track: fix-261030-a1b2\n', stderr: '' }, text: '' })
  await $.tool.call({ tool: 'Bash', command: 'zenify track start --kind fix --repo kit --est -' })
  // The bind line, then the live file with the bound id (FR-12.1: a bind runs inside a working turn).
  expect(calls.write.map(w => w.path)).toEqual([SESSION_LOG, PULSE, LIVE, BEAT, PULSE])
})

test('FR-4.1: a start that printed an id binds even while enabled is still false (feature turned on in the same command)', async ($, on) => {
  const world: World = { stats: ok(statsJson({ enabled: false })) }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  world.toolCall = () => ({ result: { stdout: 'track: fix-261030-a1b2\n', stderr: '' }, text: '' })
  await $.tool.call({ tool: 'Bash', command: 'zenify feature track on && zenify track start --kind fix --repo kit --est -' })
  expect(calls.write.some(w => w.text.includes('bind · fix-261030-a1b2'))).toBe(true)
})

test('FR-4.1: a rejected track end (no confirmation line) keeps the binding', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  mock.clock(on)
  await bindTo($, world, COOK_ID, START_COOK)
  world.toolCall = () => ({ result: { stdout: '', stderr: 'error: pass exactly one of --pr, --branch, --rollback, --cancelled' }, text: '' })
  await $.tool.call({ tool: 'Bash', command: `zenify track end ${COOK_ID} --rounds 1` })
  world.toolCall = undefined
  await $.turn.complete(turnEnd(30000))
  await $.turn.complete(turnEnd(9000))
  expect(fields(world).filter(f => f.startsWith('unbind'))).toEqual([])
  expect(fields(world).filter(f => f.startsWith('turn'))).toHaveLength(2)
})

test('SC-8: feature on, a /znf:fix prompt opens the track pane', async ($, on) => {
  const calls = engine(on, {})
  const clock = mock.clock(on)
  await start($, clock)
  await $.prompt.submit({ text: 'hello' })
  expect(calls.open).toHaveLength(0)
  await $.prompt.submit({ text: '/znf:fix x', command: { name: 'znf:fix', args: 'x' } } as any)
  await clock.settle()
  expect(calls.open).toEqual([PANE])
})

test('SC-8: feature off, a /znf:fix prompt opens nothing', async ($, on) => {
  const calls = engine(on, { stats: ok(statsJson({ enabled: false })) })
  const clock = mock.clock(on)
  await start($, clock)
  await $.prompt.submit({ text: '/znf:fix x', command: { name: 'znf:fix', args: 'x' } } as any)
  await clock.settle()
  expect(calls.open).toHaveLength(0)
})

test('FR-6.1: a Skill call of cook, hotfix or ship, with or without znf:, opens the pane; scout does not', async ($, on) => {
  const calls = engine(on, {})
  const clock = mock.clock(on)
  await start($, clock)
  await $.tool.call({ tool: 'Skill', skill: 'znf:scout' } as any)
  expect(calls.open).toHaveLength(0)
  await $.tool.call({ tool: 'Skill', skill: 'hotfix' } as any)
  await clock.settle()
  expect(calls.open).toEqual([PANE])
})

test('SC-45: options tips false: the status line carries no contextual tip', { options: { tips: false } }, async ($, on) => {
  const calls = engine(on, { stats: ok(statsJson({ today: { ...JSON.parse(statsJson()).today, prsOpened: 2, merged: 0, prsWaiting: 2 } })) })
  const clock = mock.clock(on)
  await start($, clock)
  expect(lastStatus(calls)).toBe('track ⏸ · Today: 0/0 · PR 2')
})

test('SC-45: tips default: the status line carries the tip', async ($, on) => {
  const calls = engine(on, { stats: ok(statsJson({ today: { ...JSON.parse(statsJson()).today, prsOpened: 2, merged: 0, prsWaiting: 2 } })) })
  const clock = mock.clock(on)
  await start($, clock)
  expect(lastStatus(calls)).toContain('Tip: 2 PRs waiting to merge')
})

test('SC-41: trackPane off: a /znf:fix prompt opens no pane and /track-pane is not registered', { options: { trackPane: 'off' } }, async ($, on) => {
  const calls = engine(on, {})
  const clock = mock.clock(on)
  await start($, clock)
  await $.prompt.submit({ text: '/znf:fix x', command: { name: 'znf:fix', args: 'x' } } as any)
  await $.tool.call({ tool: 'Skill', skill: 'znf:fix' } as any)
  await clock.settle()
  expect(calls.open).toHaveLength(0)
  expect(calls.registered).not.toContain('track-pane')
})

test('SC-41: trackPane command: a /znf:fix prompt opens no pane; /track-pane opens it', { options: { trackPane: 'command' } }, async ($, on) => {
  const calls = engine(on, {})
  const clock = mock.clock(on)
  await start($, clock)
  await $.prompt.submit({ text: '/znf:fix x', command: { name: 'znf:fix', args: 'x' } } as any)
  await $.tool.call({ tool: 'Skill', skill: 'znf:fix' } as any)
  await clock.settle()
  expect(calls.open).toHaveLength(0)
  expect(calls.registered).toContain('track-pane')
  await $.command.run({ command: 'track-pane', args: '' } as any)
  expect(calls.open).toEqual([PANE])
})

test('FR-17.5: track-pane is the only command registered', async ($, on) => {
  const calls = engine(on, {})
  const clock = mock.clock(on)
  await start($, clock)
  expect(calls.registered).toEqual(['track-pane'])
})

test('FR-6.1: the track-pane command toggles the pane', async ($, on) => {
  const calls = engine(on, {})
  const clock = mock.clock(on)
  await start($, clock)
  await $.command.run({ command: 'track-pane', args: '' } as any)
  expect(calls.open).toEqual([PANE])
  await $.command.run({ command: 'track-pane', args: '' } as any)
  expect(calls.close.map((c: any) => c.id)).toEqual(['track'])
})

test('SC-11: Kitchen is the default tab; Today has x and y, A tagged (this tab)', async ($, on) => {
  const x = item()
  const y = item({ id: 'fix-261030-0b0b', kind: 'fix', session: OTHER, cwd: '/w/repos/hub', phase: 'fix' })
  const world: World = {}
  engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  world.toolCall = () => ({ result: { stdout: `track: ${x.id}\n`, stderr: '' }, text: '' })
  await $.tool.call({ tool: 'Bash', command: 'zenify track start --kind cook --repo kit --est M' })
  world.toolCall = undefined
  world.stats = ok(statsJson({ running: [x, y] }))
  await $.command.run({ command: 'track-pane', args: '' } as any)
  await clock.settle()
  const ui = await mountTab($)
  expect(await allTexts(ui)).toContain('▸Kitchen')
  await ui.press({ key: 'tab:today' })
  const all = await allTexts(ui)
  expect(all).toContain('▸Today')
  expect(all.some(t => t.includes(x.id))).toBe(true)
  expect(all.some(t => t.includes(y.id))).toBe(true)
  expect(all).toContain('aaaaaaaa app (this tab)')
  expect(all).toContain('bbbbbbbb hub')
})

test('FR-10.8: a tab stored by the old shape (total) opens as Kitchen', async ($, on) => {
  const world: World = { storedTab: { shape: 'track-2', value: 'total' } }
  engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  await $.command.run({ command: 'track-pane', args: '' } as any)
  const ui = await mountTab($)
  expect(await allTexts(ui)).toContain('▸Kitchen')
})

test('FR-6.2: the tab row has the five tabs; Estimate and Quality switch the body', async ($, on) => {
  engine(on, {})
  const clock = mock.clock(on)
  await start($, clock)
  await $.command.run({ command: 'track-pane', args: '' } as any)
  const ui = await mountTab($)
  const buttons = (await ui.findAll({ type: 'Button' })).map((b: any) => b.props.label)
  expect(buttons).toEqual(['Today', 'Estimate', 'Quality', 'Backlog'])
  await ui.press({ key: 'tab:estimate' })
  expect(await allTexts(ui)).toContain('▸Estimate')
  await ui.press({ key: 'tab:quality' })
  expect(await allTexts(ui)).toContain('▸Quality')
  expect(await allTexts(ui)).toContain('no work yet')
})

test('tab keys: each tab Button carries its 1-based place as hotkey', async ($, on) => {
  engine(on, {})
  const clock = mock.clock(on)
  await start($, clock)
  await $.command.run({ command: 'track-pane', args: '' } as any)
  const ui = await mountTab($)
  const keys = async () => (await ui.findAll({ type: 'Button' })).map((b: any) => [b.props.label, b.props.hotkey])
  expect(await keys()).toEqual([['Today', '2'], ['Estimate', '3'], ['Quality', '4'], ['Backlog', '5']])
  await ui.press({ key: 'tab:quality' })
  expect(await keys()).toEqual([['Kitchen', '1'], ['Today', '2'], ['Estimate', '3'], ['Backlog', '5']])
})

test('tab keys: the key hint shows on the main screen while the pane is unfocused, never in fullscreen or focused', async ($, on) => {
  engine(on, {})
  const clock = mock.clock(on)
  await start($, clock)
  await $.command.run({ command: 'track-pane', args: '' } as any)
  const at = (isFullscreen: boolean | undefined, isFocused: boolean) => {
    const base = pane()
    return { ...base, ...(isFullscreen === undefined ? {} : { viewport: { columns: 200, rows: 50, isFullscreen } }), props: { ...base.props, isFocused, placement: 'inline' } }
  }
  const hint = async (isFullscreen: boolean | undefined, isFocused: boolean) => {
    const ui = await $.ui.mount(at(isFullscreen, isFocused))
    const shown = (await allTexts(ui)).includes(COPY.tabKeys)
    await ui.unmount()
    return shown
  }
  expect(await hint(false, false)).toBe(true)
  expect(await hint(false, true)).toBe(false)
  expect(await hint(true, false)).toBe(false)
  expect(await hint(undefined, false)).toBe(false)
})

test('FR-6.4: a failing stats run shows its message in the pane, not an empty pane', async ($, on) => {
  const world: World = {}
  engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  await $.command.run({ command: 'track-pane', args: '' } as any)
  world.stats = { exitCode: 2, stdout: '', stderr: 'read sessions: EACCES' }
  world.mtimes = { [LOG_MD]: 9 }
  await clock.advance(2000)
  const ui = await mountTab($)
  expect(await allTexts(ui)).toContain('⚠ track error: read sessions: EACCES')
})

test('FR-6.1: the pane opened by command with the feature off says how to turn it on', async ($, on) => {
  engine(on, { stats: ok(statsJson({ enabled: false })) })
  const clock = mock.clock(on)
  await start($, clock)
  await $.command.run({ command: 'track-pane', args: '' } as any)
  const ui = await mountTab($)
  expect(await allTexts(ui)).toContain(OFF)
})

const FIX_ID = 'fix-261030-a1b2'
const COOK_ID = 'cook-261030-c0de'
const START_FIX = 'zenify track start --kind fix --repo kit --est -'
const START_COOK = 'zenify track start --kind cook --repo kit --est -'
const ISO_AT = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z · /
const sessionLines = (world: World) => (world.files?.[SESSION_LOG] ?? '').split('\n').filter(Boolean)
const fields = (world: World) => sessionLines(world).map(l => l.replace(ISO_AT, ''))
const turnEnd = (durationMs: number, agentId?: string, usage?: unknown) =>
  ({ answer: 'ok', durationMs, isAborted: false, turnId: `turn-${durationMs}`, reason: 'answer', ...(agentId ? { agentId } : {}), ...(usage ? { usage } : {}) }) as any
// The Bash answer of `zenify track start`, as core gives it ({ stdout }) or as text only.
const stdoutAnswer = (stdout: string) => ({ result: { stdout, stderr: '' }, text: stdout })
const textAnswer = (text: string) => ({ ref: 'r1', result: undefined, text })

function trackWorld(over: Partial<World> = {}): World {
  return { trackEnabled: true, ...over }
}

// Runs `zenify track start` in the main loop and answers its stdout.
async function bindTo($: any, world: World, id: string, command = START_FIX) {
  world.toolCall = () => stdoutAnswer(`track: ${id}\n`)
  await $.tool.call({ tool: 'Bash', command })
  world.toolCall = undefined
}

test('FR-2.3: with track disabled nothing is written, not even a bind', async ($, on) => {
  const world = trackWorld({ trackEnabled: false })
  const calls = engine(on, world)
  mock.clock(on)
  // With the feature off the CLI answers `track: off`, never an id.
  world.toolCall = () => stdoutAnswer('track: off\n')
  await $.tool.call({ tool: 'Bash', command: START_FIX })
  world.toolCall = undefined
  await $.turn.complete(turnEnd(30000))
  await $.prompt.submit({ text: 'next' })
  expect(calls.write).toHaveLength(0)
  expect(calls.toast).toEqual([])
})

test('SC-14: a Bash stdout of track: off binds nothing', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on)
  world.toolCall = () => stdoutAnswer('track: off\n')
  await $.tool.call({ tool: 'Bash', command: START_FIX })
  world.toolCall = undefined
  await $.turn.complete(turnEnd(30000))
  // No session log line; the main turn end only marks the live file idle (FR-12.1).
  expect(calls.write.map(w => w.path)).toEqual([LIVE, BEAT, PULSE])
})

test('FR-4.1/FR-7.3: a start stdout binds the id into this session file, the cwd first', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on)
  await bindTo($, world, FIX_ID)
  expect(calls.write.map(w => w.path)).toEqual([SESSION_LOG, PULSE, LIVE, BEAT, PULSE])
  expect(sessionLines(world).every(l => ISO_AT.test(l))).toBe(true)
  expect(fields(world)).toEqual([`cwd · ${CWD}`, `bind · ${FIX_ID}`])
})

test('FR-4.1: a start answer with only text binds too', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  mock.clock(on)
  world.toolCall = () => textAnswer(`track: ${COOK_ID}`)
  await $.tool.call({ tool: 'Bash', command: START_COOK })
  world.toolCall = undefined
  expect(fields(world)).toEqual([`cwd · ${CWD}`, `bind · ${COOK_ID}`])
})

test('SC-22: the turn that ran track end is written before the unbind', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  mock.clock(on)
  await bindTo($, world, COOK_ID, START_COOK)
  world.toolCall = () => stdoutAnswer(`track: ended ${COOK_ID}\n`)
  await $.tool.call({ tool: 'Bash', command: `zenify track end ${COOK_ID} --pr https://github.com/Z/kit/pull/1 --rounds 1` })
  world.toolCall = undefined
  // A subagent turn inside the ending turn still counts for the id.
  await $.turn.complete(turnEnd(5000, 'a-1'))
  await $.turn.complete(turnEnd(30000))
  await $.turn.complete(turnEnd(9000))
  expect(fields(world)).toEqual([
    `cwd · ${CWD}`,
    `bind · ${COOK_ID}`,
    `turn · ${COOK_ID} · 5000 · sub · - · - · - · - · -`,
    `turn · ${COOK_ID} · 30000 · main · - · - · - · - · -`,
    `unbind · ${COOK_ID}`,
  ])
})

test('FR-4.8: a turn with usage carries model and the four token counts', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  mock.clock(on)
  await bindTo($, world, FIX_ID)
  const usage = { model: 'claude-opus-5-5', input_tokens: 12, output_tokens: 34, cache_read_input_tokens: 5600, cache_creation_input_tokens: 78 }
  await $.turn.complete(turnEnd(20000, undefined, usage))
  expect(fields(world).at(-1)).toBe(`turn · ${FIX_ID} · 20000 · main · claude-opus-5-5 · 12 · 34 · 5600 · 78`)
})

test('SC-19: an ask a person answers counts the whole tool call as a permission wait', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  const clock = mock.clock(on)
  await bindTo($, world, FIX_ID)
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'rm -rf build' }, tool_use_id: 'tu-1', approver: 'person' } as any)
  await $.classic.PermissionRequest({ tool_name: 'Bash', tool_input: { command: 'rm -rf build' } } as any)
  world.verdict = undefined
  world.toolCall = async () => {
    await clock.sleep(40000)
    return { result: { stdout: '', stderr: '' } }
  }
  const call = $.tool.call({ tool: 'Bash', command: 'rm -rf build', tool_use_id: 'tu-1' } as any)
  await clock.advance(40000)
  await call
  expect(fields(world).at(-1)).toBe(`wait · ${FIX_ID} · 40000 · permission`)
})

test('SC-24: an ask the host answers is no wait', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  const clock = mock.clock(on)
  await bindTo($, world, FIX_ID)
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'rm -rf build' }, tool_use_id: 'tu-2', approver: 'host' } as any)
  world.verdict = undefined
  world.toolCall = async () => {
    await clock.sleep(40000)
    return { result: { stdout: '', stderr: '' } }
  }
  const call = $.tool.call({ tool: 'Bash', command: 'rm -rf build', tool_use_id: 'tu-2' } as any)
  await clock.advance(40000)
  await call
  expect(fields(world).some(l => l.startsWith('wait'))).toBe(false)
})

test('FR-4.4: AskUserQuestion in the main loop is an ask wait; in a subagent it is none', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  const clock = mock.clock(on)
  await bindTo($, world, FIX_ID)
  world.toolCall = async () => {
    await clock.sleep(7000)
    return { result: { answers: {} } }
  }
  const ask = { tool: 'AskUserQuestion', questions: [{ question: 'Which?', header: 'Pick', multiSelect: false, options: [{ label: 'a', description: 'a' }, { label: 'b', description: 'b' }] }] }
  let call = $.tool.call(ask as any)
  await clock.advance(7000)
  await call
  call = $.tool.call({ ...ask, agentId: 'a-1' } as any)
  await clock.advance(7000)
  await call
  expect(fields(world).filter(l => l.startsWith('wait'))).toEqual([`wait · ${FIX_ID} · 7000 · ask`])
})

test('FR-4.6: a prompt after a main turn is an idle wait from that turn end', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  const clock = mock.clock(on)
  await bindTo($, world, FIX_ID)
  // No main turn has ended yet: no idle.
  await $.prompt.submit({ text: 'first' })
  await $.turn.complete(turnEnd(1000))
  await clock.advance(90000)
  await $.prompt.submit({ text: 'go on' })
  // A second prompt before any new turn end adds nothing.
  await $.prompt.submit({ text: 'and this' })
  expect(fields(world).filter(l => l.startsWith('wait'))).toEqual([`wait · ${FIX_ID} · 90000 · idle`])
})

test('FR-8.2: Skill(scout) and Skill(znf:scout) under a fix id mark the fix phase; other skills none', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  mock.clock(on)
  await bindTo($, world, FIX_ID)
  await $.tool.call({ tool: 'Skill', skill: 'scout' } as any)
  await $.tool.call({ tool: 'Skill', skill: 'znf:scout' } as any)
  await $.tool.call({ tool: 'Skill', skill: 'znf:writing-plans' } as any)
  await $.tool.call({ tool: 'Skill', skill: 'znf:ship' } as any)
  expect(fields(world).filter(l => l.startsWith('phase'))).toEqual([
    `phase · ${FIX_ID} · fix`,
    `phase · ${FIX_ID} · fix`,
    `phase · ${FIX_ID} · ship`,
  ])
})

test('FR-8.2: cook markers map to scout, plan, implement and ship', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  mock.clock(on)
  await bindTo($, world, COOK_ID, START_COOK)
  for (const skill of ['znf:scout', 'znf:writing-plans', 'znf:subagent-driven-development', 'znf:review', 'znf:ship']) {
    await $.tool.call({ tool: 'Skill', skill } as any)
  }
  expect(fields(world).filter(l => l.startsWith('phase'))).toEqual(
    ['scout', 'plan', 'implement', 'ship'].map(p => `phase · ${COOK_ID} · ${p}`),
  )
})

test('FR-4.2: with no bound id a turn writes nothing', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on)
  await $.turn.complete(turnEnd(30000))
  await $.tool.call({ tool: 'Skill', skill: 'znf:scout' } as any)
  // The main turn end still tells the live file the tab is idle (FR-12.1); no session log line.
  expect(calls.write.filter(w => !w.path.startsWith('/w/.zenify/track/live/') && w.path !== PULSE)).toHaveLength(0)
})

test('a failed session write is toasted, not swallowed', async ($, on) => {
  const world = trackWorld({ writeError: 'EACCES: permission denied' })
  const calls = engine(on, world)
  mock.clock(on)
  await bindTo($, world, FIX_ID)
  expect(calls.toast).toHaveLength(1)
  expect(calls.toast[0].startsWith('timing error: ') && calls.toast[0].includes('EACCES: permission denied')).toBe(true)
})

const FORCE_ARGV = [...STATS_ARGV, '--force']

test('FR-13.7: opening the Backlog tab reruns stats with --force; other runs and tabs do not', async ($, on) => {
  const backlog = { items: [{ source: 'manual', id: 'b1', repo: 'kit', est: 'S', ref: '-', title: 'fix the thing', ageDays: 4 }], ghError: 'gh: rate limited' }
  const calls = engine(on, { stats: ok(statsJson({ backlog })) })
  const clock = mock.clock(on)
  await start($, clock)
  await $.command.run({ command: 'track-pane', args: '' } as any)
  await clock.settle()
  expect(statsRuns(calls).every(a => !a.includes('--force'))).toBe(true)
  const before = statsRuns(calls).length
  const ui = await mountTab($)
  expect((await ui.findAll({ type: 'Button' })).map((b: any) => b.props.label)).toEqual(['Today', 'Estimate', 'Quality', 'Backlog'])
  await ui.press({ key: 'tab:estimate' })
  await clock.settle()
  expect(statsRuns(calls)).toHaveLength(before)
  await ui.press({ key: 'tab:backlog' })
  await clock.settle()
  expect(statsRuns(calls).slice(before)).toEqual([FORCE_ARGV])
  const t = await allTexts(ui)
  expect(t).toContain('▸Backlog')
  expect(t).toContain('kit · S · fix the thing · 4d')
  expect(t).toContain('⚠ github: gh: rate limited')
})

// FR-1/FR-2: live tab state, heartbeat and pulse.
const T0 = Date.UTC(2026, 10, 2, 3, 0, 0)
const T0_ISO = '2026-11-02T03:00:00.000Z'
const ASK = { tool: 'AskUserQuestion', questions: [{ question: 'Which?', header: 'Pick', multiSelect: false, options: [{ label: 'a', description: 'a' }, { label: 'b', description: 'b' }] }] }
// The live json without `run`, which is random per process; liveRun reads it.
const liveDoc = (world: World) => {
  const { run: _run, ...doc } = JSON.parse(world.files?.[LIVE] ?? 'null')
  return doc
}
const liveRun = (world: World): string => JSON.parse(world.files?.[LIVE] ?? 'null').run
const liveWrites = (calls: Calls) => calls.write.filter(w => w.path === LIVE)
const trackWrites = (calls: Calls) => calls.write.filter(w => w.path.startsWith('/w/.zenify/track/live/') || w.path === PULSE)

test('SC-2: AskUserQuestion writes ask then busy, with pulse; a throw still ends busy', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on, { now: T0 })
  let during: { live: unknown; pulse: string | undefined } | undefined
  world.toolCall = () => {
    during = { live: liveDoc(world), pulse: world.files?.[PULSE] }
    return { result: { answers: {} } }
  }
  await $.tool.call({ ...ASK, tool_use_id: 'u1' } as any)
  expect(during).toEqual({ live: { state: 'ask', since: T0_ISO, cwd: CWD, id: '' }, pulse: T0_ISO })
  expect(liveDoc(world)).toEqual({ state: 'busy', since: T0_ISO, cwd: CWD, id: '' })
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['ask', 'busy'])
  // A hook beneath that throws is skipped, so `next(e)` rejects (as an interrupt does); the
  // rejection reaches the caller and the tab still goes back to busy.
  world.toolCall = () => {
    throw new Error('interrupted')
  }
  let thrown = ''
  try {
    await $.tool.call({ ...ASK, tool_use_id: 'u3' } as any)
  } catch (err) {
    thrown = String(err)
  }
  expect(thrown).toContain('no implementation for tool.call')
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['ask', 'busy', 'ask', 'busy'])
})

test('SC-2 / Review Focus 2: a plain tool call, even the first of the session, writes no live file or pulse', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on)
  await $.tool.call({ tool: 'Read', file_path: '/w/app/a.ts', tool_use_id: 'r1' } as any)
  await $.tool.call({ tool: 'Bash', command: 'ls', tool_use_id: 'r2' } as any)
  expect(trackWrites(calls)).toEqual([])
})

test('SC-3: a main-loop ask a person answers is permission until its call ends; unbound id is empty', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on, { now: T0 })
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'rm -rf build' }, tool_use_id: 'p1', approver: 'person' } as any)
  await $.classic.PermissionRequest({ tool_name: 'Bash', tool_input: { command: 'rm -rf build' } } as any)
  world.verdict = undefined
  expect(liveDoc(world)).toEqual({ state: 'permission', since: T0_ISO, cwd: CWD, id: '' })
  expect(world.files?.[PULSE]).toBe(T0_ISO)
  await $.tool.call({ tool: 'Bash', command: 'rm -rf build', tool_use_id: 'p1' } as any)
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['permission', 'busy'])
})

test('SC-12: a subagent ask a person answers is permission too, with the bound id', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on, { now: T0 })
  await bindTo($, world, FIX_ID)
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'rm -rf build' }, tool_use_id: 'p2', agentId: 'a-1', approver: 'person' } as any)
  await $.classic.PermissionRequest({ tool_name: 'Bash', tool_input: { command: 'rm -rf build' }, agent_id: 'a-1' } as any)
  world.verdict = undefined
  expect(liveDoc(world)).toEqual({ state: 'permission', since: T0_ISO, cwd: CWD, id: FIX_ID })
  await $.tool.call({ tool: 'Bash', command: 'rm -rf build', tool_use_id: 'p2', agentId: 'a-1' } as any)
  // busy first: the bind wrote the live file with the id before the ask.
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['busy', 'permission', 'busy'])
})

test('SC-3: an ask the host answers writes no live state', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on)
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'rm -rf build' }, tool_use_id: 'h1', approver: 'host' } as any)
  world.verdict = undefined
  await $.tool.call({ tool: 'Bash', command: 'rm -rf build', tool_use_id: 'h1' } as any)
  expect(trackWrites(calls)).toEqual([])
})

test('Review Focus 4: an ask and a subagent permission overlap; busy only once both end', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on)
  const seen: string[] = []
  let checked!: () => void
  const checkedP = new Promise<void>(r => (checked = r))
  let release!: () => void
  const gate = new Promise<void>(r => (release = r))
  let u2Call: Promise<unknown> | undefined
  // As in real use, u2's permission check runs inside u2's own tool.call; u2 is still waiting
  // when u1 (the ask) ends.
  world.toolCall = async (e: any) => {
    if (e.tool_use_id === 'u1') {
      u2Call = $.tool.call({ tool: 'Bash', command: 'rm x', tool_use_id: 'u2', agentId: 'a-1' } as any)
      await checkedP
      return { result: {} }
    }
    world.verdict = 'ask'
    await $.tool.check({ tool: 'Bash', input: { command: 'rm x' }, tool_use_id: 'u2', agentId: 'a-1', approver: 'person' } as any)
    await $.classic.PermissionRequest({ tool_name: 'Bash', tool_input: { command: 'rm x' }, agent_id: 'a-1' } as any)
    world.verdict = undefined
    seen.push(liveDoc(world).state)
    checked()
    await gate
    return { result: {} }
  }
  await $.tool.call({ ...ASK, tool_use_id: 'u1' } as any)
  seen.push(liveDoc(world).state)
  release()
  await u2Call
  seen.push(liveDoc(world).state)
  expect(seen).toEqual(['ask', 'permission', 'busy'])
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['ask', 'permission', 'busy'])
})

test('FR-2.3: track turned off while an ask is open writes nothing when the tool ends', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on, { now: T0 })
  world.toolCall = () => {
    world.trackEnabled = false
    return { result: {} }
  }
  await $.tool.call({ ...ASK, tool_use_id: 'u1' } as any)
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['ask'])
  expect(calls.write.filter(w => w.path === PULSE)).toHaveLength(1)
})

const withMtimes = (pulse: number) => ({ [LOG_MD]: 1, [SESSIONS_DIR]: 1, [PULSE]: pulse })

test('FR-2.3: track turned off during an ask and back on rewrites the stale ask as busy', async ($, on) => {
  const world: World = { mtimes: withMtimes(1) }
  const calls = engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await start($, clock)
  world.toolCall = async () => {
    // The stats run reports track off while the ask is open.
    world.stats = ok(statsJson({ enabled: false }))
    world.mtimes = withMtimes(2)
    await clock.advance(2000)
    return { result: {} }
  }
  await $.tool.call({ ...ASK, tool_use_id: 'u1' } as any)
  expect(liveDoc(world).state).toBe('ask')
  world.stats = undefined
  world.mtimes = withMtimes(3)
  await clock.advance(2000)
  await clock.settle()
  expect(liveDoc(world).state).toBe('busy')
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['ask', 'busy'])
})

test('FR-2.3: turning track back on writes nothing for a tab that never wrote a live file', async ($, on) => {
  const world: World = { mtimes: withMtimes(1) }
  const calls = engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await start($, clock)
  world.stats = ok(statsJson({ enabled: false }))
  world.mtimes = withMtimes(2)
  await clock.advance(2000)
  world.stats = undefined
  world.mtimes = withMtimes(3)
  await clock.advance(2000)
  await clock.settle()
  expect(liveWrites(calls)).toEqual([])
})

test('SC-13: track off, AskUserQuestion writes no live file or pulse', async ($, on) => {
  const world = trackWorld({ trackEnabled: false })
  const calls = engine(on, world)
  mock.clock(on)
  await $.tool.call({ ...ASK, tool_use_id: 'u1' } as any)
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'rm x' }, tool_use_id: 'p1', approver: 'person' } as any)
  await $.classic.PermissionRequest({ tool_name: 'Bash', tool_input: { command: 'rm x' } } as any)
  await $.classic.SubagentStart({ agent_id: 'a-1', agent_type: 'Explore' } as any)
  await $.tool.call({ tool: 'Bash', command: 'rm x', tool_use_id: 'p1' } as any)
  expect(trackWrites(calls)).toEqual([])
})

for (const state of ['ask', 'permission', 'busy'])
test(`FR-2.1/FR-12.1: session start overwrites a stale ${state} live file with idle and touches pulse`, async ($, on) => {
  const world = trackWorld({ files: { [LIVE]: JSON.stringify({ state, since: 'x', cwd: CWD, id: '' }) } })
  const calls = engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await start($, clock)
  expect(liveDoc(world)).toEqual({ state: 'idle', since: T0_ISO, cwd: CWD, id: '' })
  expect(world.files?.[PULSE]).toBe(T0_ISO)
  expect(liveWrites(calls)).toHaveLength(1)
})

test('FR-2.1: session start with no live file writes no live file, beat or pulse', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await start($, clock)
  expect(calls.write.filter(w => w.path === LIVE || w.path === PULSE)).toEqual([])
})

test('FR-2.4: the first ask writes .beat before pulse', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on, { now: T0 })
  await $.tool.call({ ...ASK, tool_use_id: 'u1' } as any)
  const paths = calls.write.map(w => w.path)
  expect(paths.indexOf(BEAT)).toBeGreaterThanOrEqual(0)
  expect(paths.indexOf(BEAT)).toBeLessThan(paths.indexOf(PULSE))
  expect(world.files?.[BEAT]).toBe(T0_ISO)
})

test('FR-2.4: every 10s the heartbeat rewrites live/<session>.beat, never pulse', async ($, on) => {
  const world: World = {}
  const calls = engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await start($, clock)
  await clock.advance(9000)
  expect(calls.write.filter(w => w.path === BEAT)).toEqual([])
  await clock.advance(1000)
  expect(calls.write.filter(w => w.path === BEAT)).toEqual([{ path: BEAT, text: '2026-11-02T03:00:10.000Z' }])
  await clock.advance(10000)
  expect(calls.write.filter(w => w.path === BEAT)).toHaveLength(2)
  expect(calls.write.filter(w => w.path === PULSE)).toEqual([])
})

test('FR-2.4: track off, no heartbeat', async ($, on) => {
  const world: World = { stats: ok(statsJson({ enabled: false })) }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  await clock.advance(30000)
  expect(trackWrites(calls)).toEqual([])
})

test('SC-1: a session log append writes pulse; a pulse mtime change alone re-runs stats at the next tick', async ($, on) => {
  const world: World = { trackEnabled: true, mtimes: { [LOG_MD]: 1, [SESSIONS_DIR]: 1, [PULSE]: 1 } }
  const calls = engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await start($, clock)
  await bindTo($, world, FIX_ID)
  await clock.settle()
  const logAt = calls.write.findIndex(w => w.path === SESSION_LOG)
  expect(logAt).toBeGreaterThanOrEqual(0)
  expect(calls.write.findIndex((w, i) => i > logAt && w.path === PULSE)).toBeGreaterThan(logAt)
  const before = statsRuns(calls).length
  await clock.advance(2000)
  expect(statsRuns(calls)).toHaveLength(before)
  world.mtimes = { [LOG_MD]: 1, [SESSIONS_DIR]: 1, [PULSE]: 2 }
  await clock.advance(2000)
  expect(statsRuns(calls)).toHaveLength(before + 1)
  expect(statsRuns(calls).at(-1)).toEqual(STATS_ARGV)
})

test('SC-11: with a Rolling boil entry stats re-run every 10s on an unchanged key; with none they do not', async ($, on) => {
  const boil = { cooked: [], rollingBoil: [{ session: OTHER, brief: 'x', state: 'ask', since: T0_ISO }], titleError: '' }
  const world: World = { stats: ok(statsJson({ kitchen: boil })) }
  const calls = engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await start($, clock)
  expect(statsRuns(calls)).toHaveLength(1)
  await clock.advance(8000)
  expect(statsRuns(calls)).toHaveLength(1)
  world.stats = ok(statsJson({ kitchen: { ...boil, rollingBoil: [] } }))
  await clock.advance(2000)
  expect(statsRuns(calls)).toEqual([STATS_ARGV, STATS_ARGV])
  await clock.advance(20000)
  expect(statsRuns(calls)).toHaveLength(2)
})

test('stuck state: a person ask whose call never ran is cleared to idle at the main turn end', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on)
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'rm x' }, tool_use_id: 'u3', approver: 'person' } as any)
  await $.classic.PermissionRequest({ tool_name: 'Bash', tool_input: { command: 'rm x' } } as any)
  world.verdict = undefined
  expect(liveDoc(world).state).toBe('permission')
  await $.turn.complete(turnEnd(5000, 'a-1'))
  expect(liveDoc(world).state).toBe('permission')
  await $.turn.complete(turnEnd(9000))
  expect(liveDoc(world).state).toBe('idle')
  // A later main turn end with nothing pending writes nothing more.
  await $.turn.complete(turnEnd(1000))
  expect(liveWrites(calls)).toHaveLength(2)
})

test('a failed live write is toasted and the tool still answers', async ($, on) => {
  const world = trackWorld({ writeError: 'EACCES: permission denied' })
  const calls = engine(on, world)
  mock.clock(on)
  world.toolCall = () => ({ result: { answers: { q: 'a' } } })
  const answer: any = await $.tool.call({ ...ASK, tool_use_id: 'u1' } as any)
  expect(answer.result).toEqual({ answers: { q: 'a' } })
  // Two toasts: the ask write fails when the call starts, and the busy write fails when it ends.
  expect(calls.toast).toHaveLength(2)
  expect(calls.toast.every(t => t.startsWith('timing error: ') && t.includes('EACCES: permission denied'))).toBe(true)
})

test('FR-4.2: a PR line is a Link with the full href and the short label', async ($, on) => {
  const url = 'https://github.com/o/r/pull/1'
  const world: World = { stats: ok(statsJson({ kitchen: { cooked: [{ id: 'c1', brief: 'b', status: 'done', endedAt: '2026-11-02T03:05:00Z', prs: [url] }], rollingBoil: [], titleError: '' } })) }
  engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  await $.command.run({ command: 'track-pane', args: '' } as any)
  await clock.settle()
  const ui = await mountTab($)
  const links = (await ui.findAll({ type: 'Link' })) as any[]
  expect(links).toHaveLength(1)
  expect(links[0].props.href).toBe(url)
  expect(links[0].props.label).toBe('r#1')
})

test('pane spacing: blank section lines keep their height, the body is padded and set off from the tabs', async ($, on) => {
  const world: World = { stats: ok(statsJson({ kitchen: { cooked: [], rollingBoil: [], titleError: '' } })) }
  engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  await $.command.run({ command: 'track-pane', args: '' } as any)
  await clock.settle()
  const ui = await mountTab($)
  const texts = (await ui.findAll({ type: 'Text' })) as any[]
  // An empty Text renders zero rows in the terminal, so a section gap must hold a space.
  expect(texts.some(t => t.text === '')).toBe(false)
  expect(texts.filter(t => t.text === ' ').length).toBeGreaterThan(0)
  const boxes = (await ui.findAll({ type: 'Box' })) as any[]
  expect(boxes[0].props.paddingX).toBe(1)
  expect(boxes[1].props.marginBottom).toBe(1)
})

test('SC-9: every pane content Text truncates at the pane width', async ($, on) => {
  const world: World = { stats: ok(statsJson({ running: [item()], kitchen: { cooked: [{ id: 'c1', brief: 'x'.repeat(300), status: 'done', endedAt: '2026-11-02T03:05:00Z', prs: null }], rollingBoil: [], titleError: '' } })) }
  engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  await $.command.run({ command: 'track-pane', args: '' } as any)
  await clock.settle()
  const ui = await mountTab($)
  for (const tabId of ['kitchen', 'today', 'estimate', 'quality', 'backlog']) {
    if (tabId !== 'kitchen') await ui.press({ key: `tab:${tabId}` })
    // The tab row holds the separators and the selected label, `▸` plus a TABS label; a content line
    // that merely starts with ▸ is not one of them and is still checked.
    const tabLabels = TABS.map(t => `▸${t.label}`)
    const body = ((await ui.findAll({ type: 'Text' })) as any[]).filter(t => t.text !== ' │ ' && !tabLabels.includes(String(t.text)))
    expect(body.length).toBeGreaterThan(0)
    for (const t of body) expect(t.props.wrap).toBe('truncate-end')
  }
})

test('SC-20: a main turn end writes idle, the next prompt writes busy; a subagent turn end changes nothing', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await $.prompt.submit({ text: 'go' })
  expect(liveDoc(world)).toEqual({ state: 'busy', since: T0_ISO, cwd: CWD, id: '' })
  await clock.advance(1000)
  await $.turn.complete(turnEnd(5000, 'a-1'))
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['busy'])
  await $.turn.complete(turnEnd(9000))
  expect(liveDoc(world).state).toBe('idle')
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['busy', 'idle'])
  // A subagent turn end after the main one does not flip idle back.
  await $.turn.complete(turnEnd(2000, 'a-2'))
  await $.prompt.submit({ text: 'next' })
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['busy', 'idle', 'busy'])
})

test('SC-20: a permission ask while idle wins over idle, and idle returns once it is answered', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on, { now: T0 })
  await $.prompt.submit({ text: 'go' })
  await $.turn.complete(turnEnd(9000))
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'rm x' }, tool_use_id: 'u9', approver: 'person' } as any)
  await $.classic.PermissionRequest({ tool_name: 'Bash', tool_input: { command: 'rm x' } } as any)
  world.verdict = undefined
  expect(liveDoc(world).state).toBe('permission')
  await $.tool.call({ tool: 'Bash', command: 'rm x', tool_use_id: 'u9' } as any)
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['busy', 'idle', 'permission', 'idle'])
})

test('SC-20: ask and permission win over idle until they are answered or the turn ends', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on, { now: T0 })
  await $.prompt.submit({ text: 'go' })
  world.toolCall = () => ({ result: { answers: {} } })
  await $.tool.call({ ...ASK, tool_use_id: 'u1' } as any)
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['busy', 'ask', 'busy'])
  // Idle waits for the ask to be answered: ask has priority over idle.
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'rm x' }, tool_use_id: 'u2', approver: 'person' } as any)
  await $.classic.PermissionRequest({ tool_name: 'Bash', tool_input: { command: 'rm x' } } as any)
  world.verdict = undefined
  await $.turn.complete(turnEnd(9000))
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['busy', 'ask', 'busy', 'permission', 'idle'])
})

test('SC-21: the open pane redraws every second while a tab is cooking, so the spinner turns', async ($, on) => {
  const since = new Date(T0 - 12 * MIN).toISOString()
  const cooking = [
    { session: OTHER, tab: 'Tab one', brief: 'x', id: '', kind: '', phase: '', since },
    { session: SESSION, tab: 'Tab two', brief: 'y', id: '', kind: '', phase: '', since },
  ]
  const world: World = { stats: ok(statsJson({ kitchen: { carried: [], lastDay: null, cooked: [], rollingBoil: [], cooking, titleError: '' } })) }
  engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await start($, clock)
  await $.command.run({ command: 'track-pane', args: '' } as any)
  await clock.settle()
  const ui = await mountTab($)
  const spin = async () => (await allTexts(ui)).filter(t => /^- [⠋-⠿] 12m/.test(t))
  const first = await spin()
  expect(first).toHaveLength(2)
  await clock.advance(1000)
  const second = await spin()
  expect(second).toHaveLength(2)
  expect(second[0]?.[2]).not.toBe(first[0]?.[2])
  expect(second[1]?.[2]).not.toBe(first[1]?.[2])
  expect(second[0]?.[2]).not.toBe(second[1]?.[2])
  expect(first[0]?.[2]).not.toBe(first[1]?.[2])
})

test('a cooking tab whose beat expired drops out on the 30s re-run with no pulse change', async ($, on) => {
  const cooking = [{ session: OTHER, tab: 'T', brief: 'x', id: '', kind: '', phase: '', since: T0_ISO }]
  const kitchen = { carried: [], lastDay: null, cooked: [], rollingBoil: [], cooking, titleError: '' }
  const world: World = { stats: ok(statsJson({ kitchen })) }
  const calls = engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await start($, clock)
  expect(statsRuns(calls)).toHaveLength(1)
  // Only cooking, no tab waiting: a beat expires after the 30s live TTL, so no 10s re-run.
  await clock.advance(28000)
  expect(statsRuns(calls)).toHaveLength(1)
  world.stats = ok(statsJson({ kitchen: { ...kitchen, cooking: [] } }))
  await clock.advance(2000)
  expect(statsRuns(calls)).toHaveLength(2)
  await clock.advance(60000)
  expect(statsRuns(calls)).toHaveLength(2)
})

test('a prompt that lands while the stale-file reset is reading keeps busy; the reset never writes idle over it', async ($, on) => {
  let release = () => {}
  const hold = new Promise<void>(r => (release = r))
  const world = trackWorld({ files: { [LIVE]: JSON.stringify({ state: 'ask', since: 'x', cwd: CWD, id: '' }) }, holdLiveRead: hold })
  const calls = engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await $.session.start({ cwd: CWD, source: 'resume' } as any)
  await $.prompt.submit({ text: 'go' })
  expect(liveDoc(world).state).toBe('busy')
  release()
  await clock.settle()
  expect(liveDoc(world).state).toBe('busy')
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state).at(-1)).toBe('busy')
})

test('a stale busy file of another run with a 5s beat becomes idle', async ($, on) => {
  const world = trackWorld({ files: { [LIVE]: JSON.stringify({ state: 'busy', since: 'x', cwd: CWD, id: '', run: 'old-run' }) }, mtimes: { [BEAT]: T0 - 5000 } })
  engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await start($, clock)
  expect(liveDoc(world).state).toBe('idle')
})

test('a live file with this run id is untouched by a second session start (compact)', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await $.tool.call({ ...ASK, tool_use_id: 'u1' } as any)
  const run = liveRun(world)
  expect(run).toMatch(/\S/)
  world.toolCall = undefined
  await $.session.start({ cwd: CWD, source: 'compact' } as any)
  await clock.settle()
  expect(liveRun(world)).toBe(run)
  expect(liveDoc(world).state).toBe('busy')
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['ask', 'busy'])
})

test('start binds an id: the live file gets the id, state stays busy and since does not move', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await $.prompt.submit({ text: 'go' })
  expect(liveDoc(world)).toEqual({ state: 'busy', since: T0_ISO, cwd: CWD, id: '' })
  await clock.advance(5000)
  await bindTo($, world, FIX_ID)
  await clock.settle()
  expect(liveDoc(world)).toEqual({ state: 'busy', since: T0_ISO, cwd: CWD, id: FIX_ID })
  expect(liveWrites(calls)).toHaveLength(2)
})

test('a bind after a mid-turn session.start still writes the id to the live file', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await $.prompt.submit({ text: 'go' })
  // An auto-compact fires session.start inside the turn: no new prompt follows.
  await $.session.start({ cwd: CWD, source: 'compact' } as any)
  await clock.settle()
  await bindTo($, world, FIX_ID)
  await clock.settle()
  expect(liveDoc(world)).toEqual({ state: 'busy', since: T0_ISO, cwd: CWD, id: FIX_ID })
})

test('a stale busy file with a beat older than 30s still becomes idle', async ($, on) => {
  const world = trackWorld({ files: { [LIVE]: JSON.stringify({ state: 'busy', since: 'x', cwd: CWD, id: '' }) }, mtimes: { [BEAT]: T0 - 31000 } })
  engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await start($, clock)
  expect(liveDoc(world).state).toBe('idle')
})

// A person's ask whose dialog opens: core's check asks a person, then the engine raises
// PermissionRequest as it is about to show the dialog.
async function dialogAsk($: any, world: World, check: Record<string, unknown>) {
  world.verdict = 'ask'
  await $.tool.check({ approver: 'person', ...check } as any)
  world.verdict = undefined
  const { tool, input, agentId } = check as { tool: string; input: unknown; agentId?: string }
  await $.classic.PermissionRequest({ tool_name: tool, tool_input: input, ...(agentId ? { agent_id: agentId } : {}) } as any)
}

test('FR-2.2: a person ask that auto mode settles (no dialog, no PermissionRequest) is no wait and no permission state', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on, { now: T0 })
  await bindTo($, world, FIX_ID)
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'rm -rf build' }, tool_use_id: 'au-1', approver: 'person' } as any)
  world.verdict = undefined
  await $.tool.call({ tool: 'Bash', command: 'rm -rf build', tool_use_id: 'au-1' } as any)
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['busy'])
  expect(fields(world).some(l => l.startsWith('wait'))).toBe(false)
})

test('FR-2.2: the permission dialog opening (PermissionRequest) is the permission state, until its call ends', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on, { now: T0 })
  await dialogAsk($, world, { tool: 'Bash', input: { command: 'rm -rf build' }, tool_use_id: 'dl-1' })
  expect(liveDoc(world)).toEqual({ state: 'permission', since: T0_ISO, cwd: CWD, id: '' })
  await $.tool.call({ tool: 'Bash', command: 'rm -rf build', tool_use_id: 'dl-1' } as any)
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['permission', 'busy'])
})

test('FR-12.1: a main turn end while a subagent runs stays busy; the last subagent stop writes idle', async ($, on) => {
  const world = trackWorld()
  const calls = engine(on, world)
  mock.clock(on, { now: T0 })
  await $.prompt.submit({ text: 'go' })
  await $.classic.SubagentStart({ agent_id: 'bg-1', agent_type: 'general-purpose' } as any)
  await $.classic.SubagentStart({ agent_id: 'bg-2', agent_type: 'Explore' } as any)
  await $.turn.complete(turnEnd(9000))
  expect(liveDoc(world).state).toBe('busy')
  await $.classic.SubagentStop({ agent_id: 'bg-1', agent_type: 'general-purpose', agent_transcript_path: '/t', stop_hook_active: false } as any)
  expect(liveDoc(world).state).toBe('busy')
  await $.classic.SubagentStop({ agent_id: 'bg-2', agent_type: 'Explore', agent_transcript_path: '/t', stop_hook_active: false } as any)
  expect(liveWrites(calls).map(w => JSON.parse(w.text).state)).toEqual(['busy', 'idle'])
})

test('FR-4.5: two same-tool asks: the first dialog marks the oldest, so its whole call is the permission wait', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  const clock = mock.clock(on)
  await bindTo($, world, FIX_ID)
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'rm a' }, tool_use_id: 'two-a', approver: 'person' } as any)
  await $.tool.check({ tool: 'Bash', input: { command: 'rm b' }, tool_use_id: 'two-b', approver: 'person' } as any)
  world.verdict = undefined
  await $.classic.PermissionRequest({ tool_name: 'Bash', tool_input: { command: 'rm a' } } as any)
  world.toolCall = async () => {
    await clock.sleep(30000)
    return { result: { stdout: '', stderr: '' } }
  }
  const call = $.tool.call({ tool: 'Bash', command: 'rm a', tool_use_id: 'two-a' } as any)
  await clock.advance(30000)
  await call
  expect(fields(world).at(-1)).toBe(`wait · ${FIX_ID} · 30000 · permission`)
})

test('FR-2.2: a main ask that never ran is swept at the main turn end, so a later dialog marks the live call', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  mock.clock(on, { now: T0 })
  await $.prompt.submit({ text: 'go' })
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'rm old' }, tool_use_id: 'stale-1', approver: 'person' } as any)
  world.verdict = undefined
  await $.turn.complete(turnEnd(9000))
  await $.prompt.submit({ text: 'again' })
  await dialogAsk($, world, { tool: 'Bash', input: { command: 'rm new' }, tool_use_id: 'live-1' })
  expect(liveDoc(world).state).toBe('permission')
  await $.tool.call({ tool: 'Bash', command: 'rm new', tool_use_id: 'live-1' } as any)
  // live-1 was the marked id: its call end settles the wait.
  expect(liveDoc(world).state).toBe('busy')
})

test('FR-2.2: a subagent dialog whose agent_id differs from the check still marks that ask', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  mock.clock(on, { now: T0 })
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'rm x' }, tool_use_id: 'sub-1', agentId: 'loop-7', approver: 'person' } as any)
  world.verdict = undefined
  await $.classic.PermissionRequest({ tool_name: 'Bash', tool_input: { command: 'rm x' }, agent_id: 'agent-7' } as any)
  expect(liveDoc(world).state).toBe('permission')
  await $.tool.call({ tool: 'Bash', command: 'rm x', tool_use_id: 'sub-1', agentId: 'loop-7' } as any)
  expect(liveDoc(world).state).toBe('busy')
})

test('FR-12.1: a compact (session.start in the same process) keeps a running background subagent cooking', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  const clock = mock.clock(on, { now: T0 })
  await $.prompt.submit({ text: 'go' })
  await $.classic.SubagentStart({ agent_id: 'bg-9', agent_type: 'general-purpose' } as any)
  await start($, clock)
  await $.turn.complete(turnEnd(9000))
  expect(liveDoc(world).state).toBe('busy')
  await $.classic.SubagentStop({ agent_id: 'bg-9', agent_type: 'general-purpose', agent_transcript_path: '/t', stop_hook_active: false } as any)
  expect(liveDoc(world).state).toBe('idle')
})

test('FR-2.2: a long call auto mode settled does not swallow a later real dialog on the same tool', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  mock.clock(on, { now: T0 })
  // long-1: a person ask the classifier settled; its call is still running (its candidate lingers).
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'sleep 600' }, tool_use_id: 'long-1', approver: 'person' } as any)
  world.verdict = undefined
  await dialogAsk($, world, { tool: 'Bash', input: { command: 'rm -rf build' }, tool_use_id: 'real-1' })
  expect(liveDoc(world).state).toBe('permission')
  await $.tool.call({ tool: 'Bash', command: 'sleep 600', tool_use_id: 'long-1' } as any)
  // The settled call ending leaves the open dialog waiting.
  expect(liveDoc(world).state).toBe('permission')
  await $.tool.call({ tool: 'Bash', command: 'rm -rf build', tool_use_id: 'real-1' } as any)
  expect(liveDoc(world).state).toBe('busy')
})

test('FR-2.2: same arguments in a subagent and the main loop: the main dialog marks the main ask', async ($, on) => {
  const world = trackWorld()
  engine(on, world)
  mock.clock(on, { now: T0 })
  world.verdict = 'ask'
  await $.tool.check({ tool: 'Bash', input: { command: 'sleep 600' }, tool_use_id: 'sub-s', agentId: 'a-1', approver: 'person' } as any)
  world.verdict = undefined
  await dialogAsk($, world, { tool: 'Bash', input: { command: 'sleep 600' }, tool_use_id: 'main-s' })
  expect(liveDoc(world).state).toBe('permission')
  // The subagent call ending does not settle the main dialog.
  await $.tool.call({ tool: 'Bash', command: 'sleep 600', tool_use_id: 'sub-s', agentId: 'a-1' } as any)
  expect(liveDoc(world).state).toBe('permission')
  await $.tool.call({ tool: 'Bash', command: 'sleep 600', tool_use_id: 'main-s' } as any)
  expect(liveDoc(world).state).toBe('busy')
})
