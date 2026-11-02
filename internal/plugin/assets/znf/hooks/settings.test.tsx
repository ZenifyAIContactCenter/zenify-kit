import { expect, mock, test } from 'claude-code/testing'

const CWD = '/w/app'
const WS_MARKER = '/w/.zenify/manifest.json'
const SESSION = 'aaaaaaaa-1111-2222-3333-444444444444'
const TRACK_PANE = { id: 'track', title: 'Track', columns: 44 }
const SETTINGS_PANE = { id: 'znf-settings', title: 'znf settings', columns: 44 }
// FR-16.1–16.5: the only argv the settings pane may run.
const ALLOWED = [
  'zenify feature',
  'zenify feature track on',
  'zenify feature track off',
  'zenify reviewer',
  'zenify reviewer set agent',
  'zenify reviewer set codex',
  'zenify env list',
]

type Run = { exitCode: number; stdout: string; stderr: string }
const ok = (stdout: string): Run => ({ exitCode: 0, stdout, stderr: '' })

const STATS = JSON.stringify({
  enabled: true,
  malformed: 0,
  scale: { source: 'default', n: 0, S: 30, M: 90, L: 240 },
  rules: [],
  today: { date: '2026-11-02', plans: {}, started: {}, ended: {}, tasks: 0, rounds: 0, prsOpened: 0, merged: 0, mergedError: '', items: [] },
  running: [],
  estimate: { byKind: {}, hotfix: { n: 0, medianMin: null, p90Min: null }, phaseShare: {} },
  quality: { byKind: {}, fix: { n: 0, roundAvg: null, wideRate: null, regressionRate: null } },
  backlog: { items: [], ghError: '' },
  onboarded: true,
})

type World = {
  // Answers by the joined argv; a missing entry exits 127.
  runs: Record<string, Run>
  store: Record<string, unknown>
  // Joined argv that cannot start (the call rejects with this reason).
  rejects?: Record<string, string>
}

type Calls = { open: unknown[]; close: string[]; run: string[] }

function engine(on: any, world: World): Calls {
  const calls: Calls = { open: [], close: [], run: [] }
  const panes: string[] = []
  on('command.register', (_: unknown, e: { name: string }) => ({ value: { command: e.name } }))
  on('session.start', (_: unknown, e: { cwd: string }) => ({ cwd: e.cwd }))
  on('session.cwd', () => ({ value: CWD }))
  on('session.id', () => ({ value: SESSION }))
  on('prompt.submit', (_: unknown, e: { text: string }) => ({ text: e.text }))
  on('ui.open', (_: unknown, e: { id: string }) => {
    calls.open.push(e)
    if (!panes.includes(e.id)) panes.push(e.id)
    return { value: { isPlaced: true } }
  })
  on('ui.close', (_: unknown, e: { id: string }) => {
    calls.close.push(e.id)
    const at = panes.indexOf(e.id)
    if (at >= 0) panes.splice(at, 1)
    return { value: undefined }
  })
  on('ui.panes', () => ({ value: panes.map(id => ({ id, title: id, isShown: true, isFocused: false, isPlaced: true })) }))
  on('ui.status', () => ({ value: undefined }))
  on('ui.toast', () => ({ value: undefined }))
  on('fs.stat', (_: unknown, e: { path: string }) =>
    e.path === WS_MARKER ? { value: { kind: 'file', size: 1, mtimeMs: 1, isLink: false } } : { deny: `ENOENT: no such file, stat '${e.path}'` },
  )
  on('fs.read', (_: unknown, e: { path: string }) => ({ deny: `ENOENT: no such file, open '${e.path}'` }))
  on('process.run', (_: unknown, e: { argv: string[] }) => {
    if (e.argv[1] === 'track') return { value: ok(STATS) }
    const key = e.argv.join(' ')
    calls.run.push(key)
    if (world.rejects?.[key]) return { deny: world.rejects[key] }
    return { value: world.runs[key] ?? { exitCode: 127, stdout: '', stderr: `unexpected ${key}` } }
  })
  on('store.get', (_: unknown, e: { key: string }) => ({ value: world.store[e.key] }))
  on('store.set', (_: unknown, e: { key: string; value: unknown }) => {
    world.store[e.key] = e.value
    return { value: undefined }
  })
  return calls
}

const mountSettings = ($: any) =>
  $.ui.mount({
    plugin: 'znf',
    component: 'Pane',
    requestId: SETTINGS_PANE.id,
    surface: 'terminal',
    props: { title: SETTINGS_PANE.title, isFocused: false, bodyColumns: 44, placement: 'dock', scroll: { offset: 0, bodyRows: 40 }, view: {} },
  } as const)

const texts = async (ui: any) => (await ui.findAll({})).filter((e: any) => e.type === 'Text').map((e: any) => e.text ?? '')
const labels = async (ui: any) => (await ui.findAll({ type: 'Button' })).map((b: any) => b.props.label)

async function start($: any, clock: { settle: () => Promise<void> }) {
  await $.session.start({ cwd: CWD, source: 'startup' } as any)
  await clock.settle()
}

async function openSettings($: any, clock: { settle: () => Promise<void> }) {
  await $.command.run({ command: 'znf-settings', args: '' } as any)
  await clock.settle()
  return mountSettings($)
}

test('FR-16.1: /znf-settings toggles its own pane with the four tabs', async ($, on) => {
  const calls = engine(on, { runs: { 'zenify feature': ok('track off\n') }, store: {} })
  const clock = mock.clock(on)
  await start($, clock)
  const ui = await openSettings($, clock)
  expect(calls.open).toEqual([SETTINGS_PANE])
  const all = await texts(ui)
  expect(all).toContain('▸Features')
  expect(await labels(ui)).toEqual(['Track', 'Reviewer', 'Env', '→ on'])
  await $.command.run({ command: 'znf-settings', args: '' } as any)
  expect(calls.close).toEqual([SETTINGS_PANE.id])
})

test('SC-39: pressing the button of `track off` runs `zenify feature track on` and the row reads `track on`', async ($, on) => {
  const world: World = { runs: { 'zenify feature': ok('track off\n'), 'zenify feature track on': ok('track on\n') }, store: {} }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  const ui = await openSettings($, clock)
  expect(await texts(ui)).toContain('track off')
  world.runs['zenify feature'] = ok('track on\n')
  await ui.press({ key: 'feature:track' })
  expect(calls.run).toEqual(['zenify feature', 'zenify feature track on', 'zenify feature'])
  expect(await texts(ui)).toContain('track on')
  expect(await labels(ui)).toContain('→ off')
})

test('SC-39: a failing toggle shows a ⚠ line with its stderr', async ($, on) => {
  const world: World = {
    runs: { 'zenify feature': ok('track off\n'), 'zenify feature track on': { exitCode: 1, stdout: '', stderr: 'overlay: permission denied\n' } },
    store: {},
  }
  engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  const ui = await openSettings($, clock)
  await ui.press({ key: 'feature:track' })
  const all = await texts(ui)
  expect(all).toContain('⚠ overlay: permission denied')
  expect(all).toContain('track off')
})

test('SC-39: the Track tab button stores trackAutoOpen=false and a /znf:fix prompt then opens no track pane', async ($, on) => {
  const world: World = { runs: { 'zenify feature': ok('track on\n') }, store: {} }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  const ui = await openSettings($, clock)
  await ui.press({ key: 'tab:track' })
  expect(await texts(ui)).toContain('Tự mở pane track khi cook/fix/hotfix/ship: on')
  await ui.press({ key: 'autoOpen' })
  expect(world.store.trackAutoOpen).toBe(false)
  expect(await texts(ui)).toContain('Tự mở pane track khi cook/fix/hotfix/ship: off')
  await $.prompt.submit({ text: '/znf:fix x', command: { name: 'znf:fix', args: 'x' } } as any)
  await clock.settle()
  expect(calls.open).toEqual([SETTINGS_PANE])
})

test('SC-39: trackAutoOpen unset, a /znf:fix prompt opens the track pane as before', async ($, on) => {
  const calls = engine(on, { runs: {}, store: {} })
  const clock = mock.clock(on)
  await start($, clock)
  await $.prompt.submit({ text: '/znf:fix x', command: { name: 'znf:fix', args: 'x' } } as any)
  await clock.settle()
  expect(calls.open).toEqual([TRACK_PANE])
})

test('FR-16.4: the Reviewer tab shows `zenify reviewer` and its codex button runs `zenify reviewer set codex`', async ($, on) => {
  const world: World = {
    runs: { 'zenify feature': ok('track on\n'), 'zenify reviewer': ok('agent (ZNF_REVIEWER=agent)\n'), 'zenify reviewer set codex': ok('ZNF_REVIEWER=codex\n') },
    store: {},
  }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await start($, clock)
  const ui = await openSettings($, clock)
  await ui.press({ key: 'tab:reviewer' })
  expect(await texts(ui)).toContain('agent (ZNF_REVIEWER=agent)')
  expect(await texts(ui)).toContain('có hiệu lực ở session mới')
  world.runs['zenify reviewer'] = ok('codex\n')
  await ui.press({ key: 'reviewer:codex' })
  expect(calls.run.slice(-2)).toEqual(['zenify reviewer set codex', 'zenify reviewer'])
  expect(await texts(ui)).toContain('codex')
})

test('FR-16.5: the Env tab shows `zenify env list` verbatim, with the edit hint, and nothing outside the five argv runs', async ($, on) => {
  const list = 'GITHUB_TOKEN set manifest secret\nAPP_ENV set manifest stg  app env\n'
  const calls = engine(on, { runs: { 'zenify feature': ok('track on\n'), 'zenify env list': ok(list) }, store: {} })
  const clock = mock.clock(on)
  await start($, clock)
  const ui = await openSettings($, clock)
  await ui.press({ key: 'tab:env' })
  const all = await texts(ui)
  expect(all).toContain('GITHUB_TOKEN set manifest secret')
  expect(all).toContain('APP_ENV set manifest stg  app env')
  expect(all).toContain('sửa: ! zenify env set KEY (nhập ẩn trong terminal)')
  await ui.press({ key: 'tab:reviewer' })
  await ui.press({ key: 'tab:track' })
  await ui.press({ key: 'tab:features' })
  for (const argv of calls.run) expect(ALLOWED).toContain(argv)
})

test('a command that cannot start shows ⚠ in its tab', async ($, on) => {
  engine(on, { runs: {}, store: {}, rejects: { 'zenify env list': 'spawn zenify ENOENT' } })
  const clock = mock.clock(on)
  await start($, clock)
  const ui = await openSettings($, clock)
  await ui.press({ key: 'tab:env' })
  expect((await texts(ui)).some((t: string) => t.startsWith('⚠') && t.includes('spawn zenify ENOENT'))).toBe(true)
})
