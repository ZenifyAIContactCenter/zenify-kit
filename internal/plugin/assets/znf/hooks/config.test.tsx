import { expect, mock, test } from 'claude-code/testing'

const CWD = '/w/app'
const WS_MARKER = '/w/.zenify/manifest.json'
const SESSION = 'aaaaaaaa-1111-2222-3333-444444444444'
// FR-17.2/17.3: the only argv the config rows may run (stats runs are the track pane's).
const ALLOWED = [
  'zenify feature',
  'zenify feature track on',
  'zenify feature track off',
  'zenify feature github.issues on',
  'zenify feature github.issues off',
  'zenify reviewer',
  'zenify reviewer set agent',
  'zenify reviewer set codex',
]

type Run = { exitCode: number; stdout: string; stderr: string }
const ok = (stdout: string): Run => ({ exitCode: 0, stdout, stderr: '' })
const fail = (stderr: string): Run => ({ exitCode: 1, stdout: '', stderr })

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
  // Joined argv that cannot start (the call rejects with this reason).
  rejects?: Record<string, string>
}

type Calls = { run: string[]; set: { key: string; value: unknown }[]; toast: string[] }

// Every hook here stands for the engine beneath the plugin.
function engine(on: any, world: World): Calls {
  const calls: Calls = { run: [], set: [], toast: [] }
  on('command.register', (_: unknown, e: { name: string }) => ({ value: { command: e.name } }))
  on('session.start', (_: unknown, e: { cwd: string }) => ({ cwd: e.cwd }))
  on('session.cwd', () => ({ value: CWD }))
  on('session.id', () => ({ value: SESSION }))
  on('ui.status', () => ({ value: undefined }))
  on('ui.toast', (_: unknown, e: { text: string }) => {
    calls.toast.push(e.text)
    return { value: undefined }
  })
  on('fs.stat', (_: unknown, e: { path: string }) =>
    e.path === WS_MARKER ? { value: { kind: 'file', size: 1, mtimeMs: 1, isLink: false } } : { deny: `ENOENT: no such file, stat '${e.path}'` },
  )
  on('fs.read', (_: unknown, e: { path: string }) => ({ deny: `ENOENT: no such file, open '${e.path}'` }))
  // The writer beneath every config.set hook: the row takes the value.
  on('config.set', (_: unknown, e: { key: string; value: unknown }) => {
    calls.set.push({ key: e.key, value: e.value })
    return { value: e.value }
  })
  on('process.run', (_: unknown, e: { argv: string[] }) => {
    if (e.argv[1] === 'track') return { value: ok(STATS) }
    const key = e.argv.join(' ')
    calls.run.push(key)
    if (world.rejects?.[key]) return { deny: world.rejects[key] }
    return { value: world.runs[key] ?? { exitCode: 127, stdout: '', stderr: `unexpected ${key}` } }
  })
  return calls
}

// The rows as the defaults hold them, matching the sources below unless a test says otherwise.
const SYNCED = { 'zenify feature': ok('github.issues off\ntrack off\n'), 'zenify reviewer': ok('agent (ZNF_REVIEWER=agent)\n') }

async function start($: any, clock: { settle: () => Promise<void> }) {
  await $.session.start({ cwd: CWD, source: 'startup' } as any)
  await clock.settle()
}

test('SC-41: setting znf.track to true runs `zenify feature track on` and the set goes on', async ($, on) => {
  const calls = engine(on, { runs: { ...SYNCED, 'zenify feature track on': ok('track on\n') } })
  const clock = mock.clock(on)
  await start($, clock)
  calls.run.length = 0
  const res = await $.config.set({ key: 'znf.track', value: true })
  expect(calls.run).toEqual(['zenify feature track on'])
  expect(res).toEqual({ value: true })
  expect(calls.set).toEqual([{ key: 'znf.track', value: true }])
})

test('SC-41: a failing `zenify feature track on` denies the set with its stderr', async ($, on) => {
  const calls = engine(on, { runs: { ...SYNCED, 'zenify feature track on': fail('overlay: permission denied\n') } })
  const clock = mock.clock(on)
  await start($, clock)
  const res = await $.config.set({ key: 'znf.track', value: true })
  expect(res).toEqual({ deny: 'overlay: permission denied' })
  expect(calls.set).toEqual([])
})

test('FR-17.2: znf.githubIssues false runs `zenify feature github.issues off`', async ($, on) => {
  const calls = engine(on, { runs: { ...SYNCED, 'zenify feature github.issues off': ok('github.issues off\n') } })
  const clock = mock.clock(on)
  await start($, clock)
  calls.run.length = 0
  expect(await $.config.set({ key: 'znf.githubIssues', value: false })).toEqual({ value: false })
  expect(calls.run).toEqual(['zenify feature github.issues off'])
})

test('FR-17.2: znf.reviewer codex runs `zenify reviewer set codex`; a command that cannot start denies', async ($, on) => {
  const calls = engine(on, {
    runs: { ...SYNCED, 'zenify reviewer set codex': ok('ZNF_REVIEWER=codex\n') },
    rejects: { 'zenify reviewer set agent': 'spawn zenify ENOENT' },
  })
  const clock = mock.clock(on)
  await start($, clock)
  calls.run.length = 0
  expect(await $.config.set({ key: 'znf.reviewer', value: 'codex' })).toEqual({ value: 'codex' })
  const denied = (await $.config.set({ key: 'znf.reviewer', value: 'agent' })) as { deny?: string }
  expect(denied.deny).toContain('spawn zenify ENOENT')
  expect(calls.run).toEqual(['zenify reviewer set codex', 'zenify reviewer set agent'])
})

test('FR-17.4: znf.trackPane runs no command', async ($, on) => {
  const calls = engine(on, { runs: SYNCED })
  const clock = mock.clock(on)
  await start($, clock)
  calls.run.length = 0
  expect(await $.config.set({ key: 'znf.trackPane', value: 'command' })).toEqual({ value: 'command' })
  expect(calls.run).toEqual([])
})

test('SC-41: `zenify feature` says track on while the option is false: session start sets znf.track true', async ($, on) => {
  const calls = engine(on, { runs: { ...SYNCED, 'zenify feature': ok('github.issues off\ntrack on\n') } })
  const clock = mock.clock(on)
  await start($, clock)
  expect(calls.toast).toEqual([])
  expect(calls.set).toEqual([{ key: 'znf.track', value: true }])
  expect(calls.run).toEqual(['zenify feature', 'zenify reviewer'])
  for (const argv of calls.run) expect(ALLOWED).toContain(argv)
})

test('FR-17.3: rows in step with their sources are left alone', { options: { track: true, githubIssues: true, reviewer: 'codex' } }, async ($, on) => {
  const calls = engine(on, { runs: { 'zenify feature': ok('github.issues on\ntrack on\n'), 'zenify reviewer': ok('codex\n') } })
  const clock = mock.clock(on)
  await $.session.start({ cwd: CWD, source: 'startup' } as any)
  await clock.settle()
  expect(calls.run).toEqual(['zenify feature', 'zenify reviewer'])
  expect(calls.set).toEqual([])
})

test('FR-17.3: a reviewer source of codex sets znf.reviewer codex', { options: { reviewer: 'agent' } }, async ($, on) => {
  const calls = engine(on, { runs: { ...SYNCED, 'zenify reviewer': ok('codex\n') } })
  const clock = mock.clock(on)
  await start($, clock)
  expect(calls.set).toEqual([{ key: 'znf.reviewer', value: 'codex' }])
})

test('FR-17.3: a failing read is a ⚠ toast and no row changes', async ($, on) => {
  const calls = engine(on, { runs: { 'zenify feature': fail('overlay: bad yaml\n'), 'zenify reviewer': ok('codex\n') } })
  const clock = mock.clock(on)
  await start($, clock)
  expect(calls.toast.some(t => t.startsWith('⚠') && t.includes('overlay: bad yaml'))).toBe(true)
  expect(calls.set).toEqual([{ key: 'znf.reviewer', value: 'codex' }])
})
