import { expect, mock, test } from 'claude-code/testing'

const CWD = '/w/app'
const LOCAL = `${CWD}/.zenify/nonstop/progress.md`
const GH_FIELDS = 'state,mergeStateStatus,statusCheckRollup,reviewDecision,baseRefName'
const URL_A = 'https://github.com/Z/kit/pull/238'
const URL_B = 'https://github.com/Z/hub/pull/9'

function today(): string {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

const row = (id: string, status: string, evidence = '-', issue = `#${id}`, priority = '-') =>
  `${id} · ${priority} · kit · issue · ${issue} · S · znf:fix · w-${id} · ${status} · ${evidence}`

function ledgerText(rows: string[], date = today()): string {
  return [`## 2026-01-01`, row('old', 'running'), '', `## ${date}`, ...rows].join('\n')
}

const BASE = [row('t1', 'done'), row('t2', 'running'), row('t3', 'queued'), row('t4', 'blocked', 'chọn schema nào?')]

type World = {
  file: { text: string; mtimeMs: number } | null
  statError?: string
  readError?: string
  gitDir?: string
  // The ledger sits at the cwd's own path even inside a git checkout.
  isLocal?: boolean
  // A pane id the engine keeps open when asked to close it (a tab that outlives the others).
  keepOpen?: string
  // The answer to `gh pr merge`; defaults to success.
  merge?: { exitCode: number; stdout: string; stderr: string }
  mergeRejects?: string
  // Holds `gh pr merge` until this resolves.
  holdMerge?: Promise<void>
  // The answer to the review-threads graphql call, by PR number; defaults to no threads.
  threads?: (n: string) => { exitCode: number; stdout: string; stderr: string }
  // Panes already open before the plugin acts (the legacy single pane).
  openPanes?: string[]
  gh?: (url: string) => Promise<{ exitCode: number; stdout: string; stderr: string }>
}

type Calls = {
  open: unknown[]
  close: unknown[]
  stat: string[]
  read: string[]
  run: string[][]
  copy: string[]
  status: (string | undefined)[]
  toast: string[]
  merges: string[][]
  graphql: string[][]
}

const threadsOf = (open: number, total: number) => () => ({
  exitCode: 0,
  stdout: JSON.stringify({
    data: {
      repository: {
        pullRequest: {
          reviewThreads: { nodes: Array.from({ length: total }, (_, i) => ({ isResolved: i >= open })) },
        },
      },
    },
  }),
  stderr: '',
})

// Every hook here stands for the engine beneath the plugin.
function engine(on: any, world: World): Calls {
  const calls: Calls = { open: [], close: [], stat: [], read: [], run: [], copy: [], status: [], toast: [], merges: [], graphql: [] }
  const panes: { id: string }[] = (world.openPanes ?? []).map(id => ({ id }))
  on('command.register', () => ({ value: { command: 'nonstop-pane' } }))
  on('session.start', (_: unknown, e: { cwd: string }) => ({ cwd: e.cwd }))
  on('session.cwd', () => ({ value: CWD }))
  on('prompt.submit', (_: unknown, e: { text: string }) => ({ text: e.text }))
  on('tool.call', () => ({ result: {} }))
  on('ui.open', (_: unknown, e: { id: string }) => {
    calls.open.push(e)
    if (!panes.some(p => p.id === e.id)) panes.push({ id: e.id })
    return { value: { isPlaced: true } }
  })
  on('ui.close', (_: unknown, e: { id: string }) => {
    calls.close.push(e)
    if (e.id !== world.keepOpen) panes.splice(panes.findIndex(p => p.id === e.id), 1)
    return { value: undefined }
  })
  on('ui.panes', () => ({
    value: panes.map(p => ({ id: p.id, title: 'Nonstop', isShown: true, isFocused: false, isPlaced: true })),
  }))
  on('ui.status', (_: unknown, e: { text: string | undefined }) => {
    calls.status.push(e.text)
    return { value: undefined }
  })
  on('ui.toast', (_: unknown, e: { text: string }) => {
    calls.toast.push(e.text)
    return { value: undefined }
  })
  on('ui.copy', (_: unknown, e: { text: string }) => {
    calls.copy.push(e.text)
    return { value: { isCopied: true } }
  })
  on('fs.stat', (_: unknown, e: { path: string }) => {
    calls.stat.push(e.path)
    if (world.statError) return { deny: world.statError }
    // With gitDir set the cwd is a worktree: the ledger lives only in the main checkout.
    const isLedger = world.gitDir && !world.isLocal ? e.path.endsWith('/.znf/nonstop/progress.md') : e.path === LOCAL
    if (!world.file || !isLedger) return { deny: `ENOENT: no such file, stat '${e.path}'` }
    return { value: { kind: 'file', size: world.file.text.length, mtimeMs: world.file.mtimeMs, isLink: false } }
  })
  on('fs.read', (_: unknown, e: { path: string }) => {
    calls.read.push(e.path)
    if (world.readError) return { deny: world.readError }
    if (!world.file) return { deny: `ENOENT: no such file, open '${e.path}'` }
    return { value: world.file.text }
  })
  on('process.run', async (_: unknown, e: { argv: string[] }) => {
    calls.run.push([...e.argv])
    if (e.argv[0] === 'git') {
      if (!world.gitDir) return { value: { exitCode: 128, stdout: '', stderr: 'fatal: not a git repository' } }
      return { value: { exitCode: 0, stdout: `${world.gitDir}\n`, stderr: '' } }
    }
    if (e.argv[0] === 'gh' && e.argv[2] === 'merge') {
      calls.merges.push([...e.argv])
      if (world.holdMerge) await world.holdMerge
      if (world.mergeRejects) return { deny: world.mergeRejects }
      return { value: world.merge ?? { exitCode: 0, stdout: '', stderr: '' } }
    }
    if (e.argv[0] === 'gh' && e.argv[1] === 'api') {
      calls.graphql.push([...e.argv])
      return { value: (world.threads ?? threadsOf(0, 0))(/number:(\d+)/.exec(e.argv[4])?.[1] ?? '') }
    }
    const view = world.gh
      ? await world.gh(e.argv[3])
      : { exitCode: 0, stdout: JSON.stringify({ state: 'OPEN', mergeStateStatus: 'CLEAN', statusCheckRollup: [], baseRefName: 'main' }), stderr: '' }
    return { value: view }
  })
  return calls
}

const HUMAN = 'nonstop-human'
const PROGRESS = 'nonstop-progress'
const DONE = 'nonstop-done'
const THREE = [
  { id: HUMAN, title: 'Need Human', columns: 44 },
  { id: PROGRESS, title: 'In Progress', columns: 44 },
  { id: DONE, title: 'Done', columns: 44 },
]

const pane = (requestId: string, bodyColumns = 44) =>
  ({
    plugin: 'znf',
    component: 'Pane',
    requestId,
    surface: 'terminal',
    props: {
      title: 'Nonstop',
      isFocused: false,
      bodyColumns,
      placement: 'dock',
      scroll: { offset: 0, bodyRows: 40 },
      view: {},
    },
  }) as const

// Text and Link cells in document order: a PR row is a Link (the ref) then a Text (the status).
const texts = async (ui: { findAll: (q: { type?: string }) => Promise<{ type: string; text?: string }[]> }) =>
  (await ui.findAll({})).filter(e => e.type === 'Text' || e.type === 'Link').map(e => e.text ?? '')

// The text of every row Box: one drawn line each, whatever its cells.
const rows = async (ui: { findAll: (q: { type: string }) => Promise<{ text: string; props: Record<string, unknown> }[]> }) =>
  (await ui.findAll({ type: 'Box' })).filter(b => b.props.flexDirection === 'row').map(b => b.text)

const links = async (ui: { findAll: (q: { type: string }) => Promise<{ props: Record<string, unknown> }[]> }) =>
  (await ui.findAll({ type: 'Link' })).map(e => e.props.href)

const ghRuns = (calls: Calls) => calls.run.filter(a => a[0] === 'gh' && a[2] === 'view')

test('SC-7: only a nonstop prompt or Skill call opens the pane', async ($, on) => {
  const calls = engine(on, { file: null })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: 'hello' })
  await clock.settle()
  expect(calls.open).toHaveLength(0)

  await $.prompt.submit({ text: '/znf:nonstop continue', command: { name: 'znf:nonstop', args: 'continue' } })
  await clock.settle()
  expect(calls.open).toEqual(THREE)

  await $.tool.call({ tool: 'Skill', skill: 'znf:nonstop' })
  await clock.settle()
  expect(calls.open).toHaveLength(6)
  expect(calls.open[3]).toMatchObject({ id: HUMAN })
})

test('SC-5: a ledger edit shows after the 2s tick; an unchanged mtime is not re-read', async ($, on) => {
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 100 } }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()

  const ui = await $.ui.mount(pane(PROGRESS))
  expect((await ui.find({ type: 'Text', text: /t2/ }))?.text).toContain('t2')
  expect(await ui.find({ type: 'Text', text: /t5/ })).toBeUndefined()

  world.file = { text: ledgerText([...BASE, row('t5', 'running')]), mtimeMs: 200 }
  await clock.advance(2000)
  expect((await ui.find({ type: 'Text', text: /t5/ }))?.text).toContain('t5')

  const reads = calls.read.length
  await clock.advance(2000)
  expect(calls.read).toHaveLength(reads)
  await ui.unmount()
})

test('SC-6: the 60s tick runs gh pr view per URL; [c] copies the merge command', async ($, on) => {
  const world: World = { file: { text: ledgerText([row('t1', 'done', URL_A), row('t2', 'running', URL_B)]), mtimeMs: 1 } }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  expect(ghRuns(calls)).toHaveLength(2)

  await clock.advance(60000)
  const runs = ghRuns(calls).slice(2)
  expect(runs).toHaveLength(2)
  expect(runs.map(a => a.slice(0, 6)).sort()).toEqual(
    [URL_A, URL_B].map(u => ['gh', 'pr', 'view', u, '--json', GH_FIELDS]).sort(),
  )

  const ui = await $.ui.mount(pane(HUMAN))
  expect(await texts(ui)).toEqual(expect.arrayContaining(['Z/kit#238', ' CLEAN']))
  expect(await links(ui)).toContain(URL_A)
  await ui.press({ key: URL_A })
  expect(calls.copy).toEqual(['gh pr merge -R Z/kit 238 --merge'])
  expect(calls.toast).toEqual(['đã copy lệnh merge'])
  await ui.unmount()
})

test('FR-3.3: the status line counts running, queued and blocked', async ($, on) => {
  const calls = engine(on, { file: { text: ledgerText(BASE), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  expect(calls.status.at(-1)).toBe('nonstop 1/2 ▶ · 1 cần anh')
})

test('FR-1.1: no ledger on either path shows the empty state and clears the status', async ($, on) => {
  const calls = engine(on, { file: null, gitDir: '/w/repo/.git' })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(PROGRESS))
  expect(await texts(ui)).toEqual(['chưa có ledger nonstop'])
  expect(calls.status.at(-1)).toBeUndefined()
  expect(calls.status.length).toBeGreaterThan(0)
  await ui.unmount()
})

test('FR-1.2: a read error other than missing is shown, not swallowed', async ($, on) => {
  engine(on, { file: { text: ledgerText(BASE), mtimeMs: 1 }, readError: 'EACCES: permission denied' })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(PROGRESS))
  expect((await ui.find({ type: 'Text', text: /EACCES/ }))?.text).toContain('EACCES')
  await ui.unmount()
})

test('FR-1.1: from a worktree the ledger is found in the main checkout', async ($, on) => {
  const calls = engine(on, { file: { text: ledgerText(BASE), mtimeMs: 1 }, gitDir: '/w/repo/.git' })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  expect(calls.stat[0]).toBe(LOCAL)
  expect(calls.stat).toContain('/w/repo/.znf/nonstop/progress.md')
  expect(calls.run.find(a => a[0] === 'git')).toEqual(['git', 'rev-parse', '--path-format=absolute', '--git-common-dir'])
  const ui = await $.ui.mount(pane(PROGRESS))
  expect((await ui.find({ type: 'Text', text: /t2/ }))?.text).toContain('t2')
  await ui.unmount()
})

test('FR-2.2: at most 4 gh calls run at once', async ($, on) => {
  const urls = [1, 2, 3, 4, 5, 6].map(n => `https://github.com/Z/kit/pull/${n}`)
  let pending = 0
  let peak = 0
  const held: (() => void)[] = []
  const world: World = {
    file: { text: ledgerText(urls.map((u, i) => row(`p${i}`, 'done', u))), mtimeMs: 1 },
    gh: () => {
      pending++
      peak = Math.max(peak, pending)
      return new Promise(resolve =>
        held.push(() => {
          pending--
          resolve({ exitCode: 0, stdout: JSON.stringify({ state: 'MERGED' }), stderr: '' })
        }),
      )
    },
  }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  expect(ghRuns(calls)).toHaveLength(4)
  while (held.length) {
    held.shift()!()
    await clock.settle()
  }
  expect(ghRuns(calls)).toHaveLength(6)
  expect(peak).toBe(4)
})

test('FR-3.2/FR-3.4: sections in order, every line cut to bodyColumns', async ($, on) => {
  const long = 'x'.repeat(80)
  const world: World = {
    file: {
      text: ledgerText([
        ...BASE,
        row('t6', 'running', URL_B),
        `Needs you: t9 — ${long}`,
        'this line is not a task',
      ]),
      mtimeMs: 1,
    },
    gh: async () => ({ exitCode: 0, stdout: JSON.stringify({ state: 'OPEN', mergeStateStatus: 'BLOCKED', statusCheckRollup: [] }), stderr: '' }),
  }
  engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  await clock.advance(60000)

  const ui = await $.ui.mount(pane(PROGRESS, 20))
  const all = await texts(ui)
  const heads = ['RUNNING', 'QUEUED', 'ĐANG WATCH', 'done 1 · queued 1 · blocked 1'.slice(0, 19) + '…']
  const at = heads.map(h => all.indexOf(h))
  expect(at.every(i => i >= 0)).toBe(true)
  expect([...at].sort((a, b) => a - b)).toEqual(at)
  // The indicator line is never cut; every other line fits.
  expect(all.slice(1).every(t => t.length <= 20)).toBe(true)
  expect(all.some(t => t.startsWith('⚠ 1'))).toBe(true)
  expect(all.some(t => t.startsWith('this line'))).toBe(true)
  await ui.unmount()

  const human = await $.ui.mount(pane(HUMAN, 20))
  expect((await texts(human))[0]).toBe('CẦN ANH')
  const needs = await rows(human)
  expect(needs.some(t => t.startsWith('t9') && t.endsWith('…'))).toBe(true)
  expect(needs.every(t => t.length <= 20)).toBe(true)
  await human.unmount()
})

test('FR-3.2: an empty section is hidden', async ($, on) => {
  engine(on, { file: { text: ledgerText([row('t1', 'done'), row('t3', 'queued')]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(PROGRESS))
  const all = await texts(ui)
  expect(all).not.toContain('RUNNING')
  expect(all).not.toContain('ĐANG WATCH')
  expect(all).toContain('QUEUED')
  expect(all).toContain('done 1 · queued 1 · blocked 0')
  await ui.unmount()
})

test('FR-3.2: each tab shows only its own content', async ($, on) => {
  const URL_C = 'https://github.com/Z/web/pull/7'
  const view = (state: string, merge = 'UNKNOWN', checks: unknown[] = []) => ({
    exitCode: 0,
    stdout: JSON.stringify({ state, mergeStateStatus: merge, statusCheckRollup: checks }),
    stderr: '',
  })
  const world: World = {
    file: {
      text: ledgerText([
        row('t1', 'running', URL_B, 'Z/be#12'),
        row('t2', 'queued', URL_C),
        row('t3', 'blocked', 'chọn A/B?'),
        row('t4', 'done', URL_A),
        'Needs you: t9 — duyệt spec?',
      ]),
      mtimeMs: 1,
    },
    gh: async url =>
      url === URL_A
        ? view('MERGED')
        : url === URL_C
          ? view('OPEN', 'BLOCKED', [{ name: 'lint', conclusion: 'FAILURE', status: 'COMPLETED' }])
          : view('OPEN', 'CLEAN'),
  }
  engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()

  const has = (all: string[], re: RegExp) => all.some(t => re.test(t))
  const human = await $.ui.mount(pane(HUMAN))
  const h = await texts(human)
  const hr = await rows(human)
  expect(has(hr, /t9 duyệt spec\?/)).toBe(true)
  expect(has(hr, /t3 chọn A\/B\?/)).toBe(true)
  expect(h).toEqual(expect.arrayContaining(['Z/hub#9', ' CLEAN']))
  expect(await links(human)).toEqual([URL_B])
  expect(has(hr, /^t1/)).toBe(false)
  await human.unmount()

  const progress = await $.ui.mount(pane(PROGRESS))
  const p = await texts(progress)
  const pr = await rows(progress)
  expect(has(pr, /^(⠋ )?t1/)).toBe(true)
  expect(p).toContain('Z/be#12')
  expect(await links(progress)).toEqual(expect.arrayContaining(['https://github.com/Z/be/issues/12', URL_C]))
  expect(has(pr, /^(⠋ )?t2/)).toBe(true)
  expect(has(p, /CI đỏ \(lint\)/)).toBe(true)
  expect(p).toContain('done 1 · queued 1 · blocked 1')
  expect(has(p, /Z\/hub#9 CLEAN/)).toBe(false)
  await progress.unmount()

  const done = await $.ui.mount(pane(DONE))
  const d = await texts(done)
  expect(has(await rows(done), /^t4/)).toBe(true)
  expect(d).toEqual(expect.arrayContaining(['Z/kit#238', ' MERGED']))
  await done.unmount()
})

test('FR-3.2: an empty tab shows trống', async ($, on) => {
  engine(on, { file: { text: ledgerText([row('t1', 'running')]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  for (const id of [HUMAN, DONE]) {
    const ui = await $.ui.mount(pane(id))
    expect(await texts(ui)).toEqual(['trống'])
    await ui.unmount()
  }
})

test('FR-1.2: a normal session polls nothing', async ($, on) => {
  const calls = engine(on, { file: { text: ledgerText([row('t1', 'done', URL_A)]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.session.start({ cwd: CWD, surface: 'terminal', isInteractive: true })
  await clock.settle()
  await clock.advance(60000)
  expect(calls.open).toHaveLength(0)
  expect(ghRuns(calls)).toHaveLength(0)
  expect(calls.read.length).toBeLessThanOrEqual(1)
})

test('FR-3.1: session.start reopens the pane on a live day', async ($, on) => {
  const calls = engine(on, { file: { text: ledgerText(BASE), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.session.start({ cwd: CWD, surface: 'terminal', isInteractive: true })
  await clock.settle()
  expect(calls.open).toEqual(THREE)
})

test('FR-3.1: session.start stays closed when today has no running task', async ($, on) => {
  const calls = engine(on, { file: { text: ledgerText([row('t1', 'done'), row('t2', 'carry-over')]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.session.start({ cwd: CWD, surface: 'terminal', isInteractive: true })
  await clock.settle()
  expect(calls.open).toHaveLength(0)
})

test('FR-1.2: the nonstop-pane command toggles; closing stops the timers', async ($, on) => {
  const world: World = { file: { text: ledgerText([row('t1', 'running', URL_A)]), mtimeMs: 1 } }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await $.session.start({ cwd: CWD, surface: 'terminal', isInteractive: true })
  await clock.settle()
  await $.command.run({ command: 'nonstop-pane', args: '' })
  expect(calls.close.map(c => (c as { id: string }).id)).toEqual(THREE.map(p => p.id))

  const reads = calls.read.length
  const gh = ghRuns(calls).length
  world.file = { text: ledgerText([row('t1', 'running', URL_A), row('t2', 'running')]), mtimeMs: 2 }
  await clock.advance(60000)
  expect(calls.read).toHaveLength(reads)
  expect(ghRuns(calls)).toHaveLength(gh)

  await $.command.run({ command: 'nonstop-pane', args: '' })
  await clock.settle()
  expect(calls.open).toHaveLength(6)
})

test('FR-1.2: a tab still open keeps the timers running', async ($, on) => {
  const world: World = { file: { text: ledgerText([row('t1', 'running')]), mtimeMs: 1 }, keepOpen: DONE }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await $.session.start({ cwd: CWD, surface: 'terminal', isInteractive: true })
  await clock.settle()
  await $.command.run({ command: 'nonstop-pane', args: '' })
  expect(calls.close).toHaveLength(3)

  world.file = { text: ledgerText([row('t1', 'running'), row('t2', 'running')]), mtimeMs: 2 }
  const reads = calls.read.length
  await clock.advance(2000)
  expect(calls.read.length).toBeGreaterThan(reads)
})

test('FR-2.2: opening the pane polls its PRs at once', async ($, on) => {
  const calls = engine(on, { file: { text: ledgerText([row('t1', 'running', URL_A)]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  expect(ghRuns(calls)).toEqual([['gh', 'pr', 'view', URL_A, '--json', GH_FIELDS]])
  const ui = await $.ui.mount(pane(HUMAN))
  expect(await texts(ui)).toEqual(expect.arrayContaining(['Z/kit#238', ' CLEAN']))
  await ui.unmount()
})

test('FR-1.1: a local ledger created after session start is found', async ($, on) => {
  const world: World = { file: null, gitDir: '/w/repo/.git' }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  // Neither path has a ledger yet: session.start falls back to the main checkout's.
  await $.session.start({ cwd: CWD, surface: 'terminal', isInteractive: true })
  await clock.settle()
  expect(calls.stat).toContain('/w/repo/.znf/nonstop/progress.md')
  world.file = { text: ledgerText(BASE), mtimeMs: 5 }
  world.isLocal = true
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(PROGRESS))
  expect((await ui.find({ type: 'Text', text: /t2/ }))?.text).toContain('t2')
  await ui.unmount()
})

test('a settled PR appears only in Done and is not polled again', async ($, on) => {
  const URL_C = 'https://github.com/Z/web/pull/7'
  const view = (state: string, merge = 'UNKNOWN') => ({
    exitCode: 0,
    stdout: JSON.stringify({ state, mergeStateStatus: merge, statusCheckRollup: [] }),
    stderr: '',
  })
  const world: World = {
    file: { text: ledgerText([row('t1', 'done', URL_A), row('t2', 'running', URL_B), row('t3', 'done', URL_C)]), mtimeMs: 1 },
    gh: async url => (url === URL_A ? view('MERGED') : url === URL_C ? view('CLOSED') : view('OPEN', 'BLOCKED')),
  }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  expect(ghRuns(calls)).toHaveLength(3)

  const progress = await $.ui.mount(pane(PROGRESS))
  const p = await texts(progress)
  expect(p.some(t => t.includes('Z/kit#238') || t.includes('Z/web#7'))).toBe(false)
  expect(p).toEqual(expect.arrayContaining(['Z/hub#9', ' BLOCKED · Zenie — · thread 0/0']))
  await progress.unmount()

  const done = await $.ui.mount(pane(DONE))
  const d = await texts(done)
  expect(d).toEqual(expect.arrayContaining(['Z/kit#238', ' MERGED', 'Z/web#7', ' CLOSED']))
  expect(await links(done)).toEqual([URL_A, URL_C])
  expect(d.some(t => t.includes('Z/hub#9'))).toBe(false)
  await done.unmount()

  await clock.advance(60000)
  expect(ghRuns(calls).slice(3).map(a => a[3])).toEqual([URL_B])
})

test('FR-3.6: each section sorts by priority and prefixes it; a legacy 9-field line has none', async ($, on) => {
  const legacy = 't3 · kit · issue · #t3 · S · znf:fix · w-t3 · running · -'
  engine(on, {
    file: {
      text: ledgerText([
        row('t1', 'running', '-', '#t1', 'P2'),
        row('t2', 'running', '-', '#t2', 'P0'),
        legacy,
        'Needs you: t2 — duyệt spec?',
        'Needs you: t8 — không rõ?',
      ]),
      mtimeMs: 1,
    },
  })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()

  const progress = await $.ui.mount(pane(PROGRESS))
  const p = (await rows(progress)).map(t => t.replace(/^⠋ /, '')).filter(t => /^(P\d )?t\d/.test(t))
  expect(p.map(t => t.split(' kit')[0])).toEqual(['P0 t2', 'P2 t1', 't3'])
  await progress.unmount()

  const human = await $.ui.mount(pane(HUMAN))
  const h = await rows(human)
  expect(h.filter(t => /t\d/.test(t))).toEqual(['P0 t2 duyệt spec?', 't8 không rõ?'])
  await human.unmount()
})

test('FR-3.7: pressing a task id expands its full line, pressing again collapses it', async ($, on) => {
  const evidence = `ev-${'e'.repeat(116)}`
  engine(on, { file: { text: ledgerText([row('t1', 'done', evidence)]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()

  const ui = await $.ui.mount(pane(DONE))
  const has = async (re: RegExp) => (await ui.find({ type: 'Text', text: re })) !== undefined
  expect((await rows(ui))[0].endsWith('…')).toBe(true)
  expect(await has(new RegExp(evidence))).toBe(false)

  await ui.press({ key: 'task:nonstop-done:t1' })
  expect(await has(new RegExp(evidence))).toBe(true)
  expect(await has(/kit · issue ·/)).toBe(true)

  await ui.press({ key: 'task:nonstop-done:t1' })
  expect(await has(new RegExp(evidence))).toBe(false)
  await ui.unmount()
})

test('FR-3.7: a Needs you line without a task expands to its full question', async ($, on) => {
  const question = `q-${'q'.repeat(100)}`
  engine(on, { file: { text: ledgerText([row('t1', 'running'), `Needs you: t9 — ${question}`]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()

  const ui = await $.ui.mount(pane(HUMAN))
  const full = () => ui.find({ type: 'Text', text: new RegExp(`^ +${question}$`) })
  expect(await full()).toBeUndefined()
  await ui.press({ key: 'task:nonstop-human:t9' })
  expect(await full()).toBeDefined()
  await ui.unmount()
})

const first = async (ui: Parameters<typeof texts>[0]) => (await texts(ui))[0]

test('FR-3.8: a running task spins the indicator every 250 ms', async ($, on) => {
  engine(on, { file: { text: ledgerText([row('t1', 'running')]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(PROGRESS))
  expect(await first(ui)).toBe('▶ ⠋ đang chạy 1 worker')
  expect((await texts(ui)).some(t => t.startsWith('⠋ '))).toBe(true)
  await clock.advance(250)
  expect(await first(ui)).toBe('▶ ⠙ đang chạy 1 worker')
  await clock.advance(500)
  expect(await first(ui)).toBe('▶ ⠸ đang chạy 1 worker')
  await ui.unmount()
})

test('FR-3.8: with no running task it shows the pause line and never spins', async ($, on) => {
  engine(on, { file: { text: ledgerText([row('t1', 'queued')]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(PROGRESS))
  expect(await first(ui)).toBe('⏸ tạm dừng')
  await clock.advance(1000)
  expect(await first(ui)).toBe('⏸ tạm dừng')
  await ui.unmount()
})

test('FR-3.8: the spinner stops when the last running task finishes', async ($, on) => {
  const world: World = { file: { text: ledgerText([row('t1', 'running')]), mtimeMs: 1 } }
  engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(PROGRESS))
  world.file = { text: ledgerText([row('t1', 'done')]), mtimeMs: 2 }
  await clock.advance(2000)
  expect(await first(ui)).toBe('⏸ tạm dừng')
  const text = await texts(ui)
  await clock.advance(1000)
  expect(await texts(ui)).toEqual(text)
  await ui.unmount()
})

test('FR-3.8: closing all tabs stops the spinner', async ($, on) => {
  const world: World = { file: { text: ledgerText([row('t1', 'running')]), mtimeMs: 1 } }
  engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  await $.command.run({ command: 'nonstop-pane', args: '' })
  const ui = await $.ui.mount(pane(PROGRESS))
  const before = await first(ui)
  await clock.advance(1000)
  expect(await first(ui)).toBe(before)
  await ui.unmount()
})

test('FR-3.9: opening the pane closes the legacy nonstop pane', async ($, on) => {
  const calls = engine(on, { file: null, openPanes: ['nonstop'] })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  expect(calls.close).toContainEqual(expect.objectContaining({ id: 'nonstop' }))
})

test('FR-3.7: a needs line sharing its id with a task expands to the fields and the full question', async ($, on) => {
  const question = `q-${'q'.repeat(100)}`
  engine(on, { file: { text: ledgerText([row('t1', 'running'), `Needs you: t1 — ${question}`]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(HUMAN))
  await ui.press({ key: 'task:nonstop-human:t1' })
  expect(await ui.find({ type: 'Text', text: new RegExp(`^ +${question}$`) })).toBeDefined()
  expect(await ui.find({ type: 'Text', text: /kit · issue ·/ })).toBeDefined()
  await ui.unmount()
})

test('FR-3.8: the spinner stops when the ledger goes missing', async ($, on) => {
  const world: World = { file: { text: ledgerText([row('t1', 'running')]), mtimeMs: 1 } }
  engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  // 2s of spinning (frame 7 or 8), then the ledger vanishes at the 2s tick.
  world.file = null
  await clock.advance(2000)
  await clock.advance(1250)
  // The ledger returns; the next ledger tick (t=4s) re-arms the spinner. A timer that
  // kept running through the gap would sit near frame 5 or 6 by now.
  world.file = { text: ledgerText([row('t1', 'running')]), mtimeMs: 9 }
  await clock.advance(750)
  const ui = await $.ui.mount(pane(PROGRESS))
  expect(['▶ ⠧ đang chạy 1 worker', '▶ ⠇ đang chạy 1 worker']).toContain(await first(ui))
  await ui.unmount()
})

const MERGE_ARGV = ['gh', 'pr', 'merge', '-R', 'Z/kit', '238', '--merge']
const mergeButton = (ui: Parameters<typeof texts>[0] & { find: (q: { type: string; key: string }) => Promise<{ props: Record<string, unknown> } | undefined> }) =>
  ui.find({ type: 'Button', key: `merge:${URL_A}` })

async function mergeWorld(on: any, extra: Partial<World> = {}) {
  const world: World = { file: { text: ledgerText([row('t1', 'running', URL_A)]), mtimeMs: 1 }, ...extra }
  const calls = engine(on, world)
  return { world, calls }
}

test('FR-2.7: one press arms the merge button with the base branch and merges nothing', async ($, on) => {
  const { calls } = await mergeWorld(on)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(HUMAN))
  expect((await mergeButton(ui))?.props.label).toBe('merge')
  await ui.press({ key: `merge:${URL_A}` })
  expect((await mergeButton(ui))?.props.label).toBe('xác nhận → main?')
  expect(calls.merges).toEqual([])
  await ui.unmount()
})

test('FR-2.7: an armed button disarms after 5 s without merging', async ($, on) => {
  const { calls } = await mergeWorld(on)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(HUMAN))
  await ui.press({ key: `merge:${URL_A}` })
  await clock.advance(5000)
  expect((await mergeButton(ui))?.props.label).toBe('merge')
  expect(calls.merges).toEqual([])
  await ui.unmount()
})

test('FR-2.7: the second press runs gh pr merge once, toasts and re-polls into Done', async ($, on) => {
  let state = 'OPEN'
  const { calls } = await mergeWorld(on, {
    gh: async () => ({
      exitCode: 0,
      stdout: JSON.stringify({ state, mergeStateStatus: 'CLEAN', statusCheckRollup: [], baseRefName: 'main' }),
      stderr: '',
    }),
  })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(HUMAN))
  await ui.press({ key: `merge:${URL_A}` })
  state = 'MERGED'
  await ui.press({ key: `merge:${URL_A}` })
  await clock.settle()
  expect(calls.merges).toEqual([MERGE_ARGV])
  expect(calls.toast).toContain('đã merge Z/kit#238')
  expect(await mergeButton(ui)).toBeUndefined()
  await ui.unmount()

  const done = await $.ui.mount(pane(DONE))
  expect(await texts(done)).toEqual(expect.arrayContaining(['Z/kit#238', ' MERGED']))
  await done.unmount()
})

test('FR-2.7: a failed merge toasts the first stderr line and the PR stays in Need Human', async ($, on) => {
  const { calls } = await mergeWorld(on, {
    merge: { exitCode: 1, stdout: '', stderr: 'GraphQL: Pull request is not mergeable\nmore' },
  })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(HUMAN))
  await ui.press({ key: `merge:${URL_A}` })
  await ui.press({ key: `merge:${URL_A}` })
  await clock.settle()
  expect(calls.toast).toContain('merge lỗi: GraphQL: Pull request is not mergeable')
  expect((await mergeButton(ui))?.props.label).toBe('merge')
  await ui.unmount()
})

test('FR-2.7: a rejected merge call is toasted, not swallowed', async ($, on) => {
  const { calls } = await mergeWorld(on, { mergeRejects: 'spawn gh ENOENT' })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(HUMAN))
  await ui.press({ key: `merge:${URL_A}` })
  await ui.press({ key: `merge:${URL_A}` })
  await clock.settle()
  expect(calls.toast.some(t => t.startsWith('merge lỗi: ') && t.includes('spawn gh ENOENT'))).toBe(true)
  await ui.unmount()
})

test('FR-2.7: a PR that is not mergeable has no merge button', async ($, on) => {
  await mergeWorld(on, {
    gh: async () => ({
      exitCode: 0,
      stdout: JSON.stringify({
        state: 'OPEN',
        mergeStateStatus: 'BLOCKED',
        statusCheckRollup: [{ name: 'lint', conclusion: 'FAILURE', status: 'COMPLETED' }],
        baseRefName: 'main',
      }),
      stderr: '',
    }),
  })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  for (const id of [HUMAN, PROGRESS]) {
    const ui = await $.ui.mount(pane(id))
    expect(await mergeButton(ui)).toBeUndefined()
    await ui.unmount()
  }
})

test('FR-2.7: an arming whose timer never fired still expires after 5 s', async ($, on) => {
  const { calls } = await mergeWorld(on)
  // A hand-driven clock whose timers never fire (a hot reload cancels pending waits).
  let now = 1_000_000
  on('clock.now', () => ({ value: now }))
  on('clock.after', () => new Promise(() => {}))
  on('clock.every', () => new Promise(() => {}))
  await $.prompt.submit({ text: '/znf:nonstop' })
  await new Promise(r => setTimeout(r, 20))
  const ui = await $.ui.mount(pane(HUMAN))
  await ui.press({ key: `merge:${URL_A}` })
  expect((await mergeButton(ui))?.props.label).toBe('xác nhận → main?')
  now += 5001
  await ui.press({ key: `merge:${URL_A}` })
  expect(calls.merges).toEqual([])
  expect((await mergeButton(ui))?.props.label).toBe('xác nhận → main?')
  await ui.unmount()
})

test('FR-2.7: two concurrent confirming presses merge once; a press while merging does nothing', async ($, on) => {
  let release: () => void = () => {}
  const holdMerge = new Promise<void>(r => (release = r))
  const { calls } = await mergeWorld(on, { holdMerge })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(HUMAN))
  await ui.press({ key: `merge:${URL_A}` })
  const both = Promise.all([ui.press({ key: `merge:${URL_A}` }), ui.press({ key: `merge:${URL_A}` })])
  await clock.settle()
  await ui.press({ key: `merge:${URL_A}` })
  expect((await mergeButton(ui))?.props.label).toBe('đang merge…')
  release()
  await both
  await clock.settle()
  expect(calls.merges).toEqual([MERGE_ARGV])
  await ui.unmount()
})

test('FR-2.7: no merge button until the base branch is known', async ($, on) => {
  await mergeWorld(on, {
    gh: async () => ({
      exitCode: 0,
      stdout: JSON.stringify({ state: 'OPEN', mergeStateStatus: 'CLEAN', statusCheckRollup: [] }),
      stderr: '',
    }),
  })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(HUMAN))
  expect(await mergeButton(ui)).toBeUndefined()
  await ui.unmount()
})

test('FR-2.7: the button row draws no stray comma', async ($, on) => {
  await mergeWorld(on)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(HUMAN))
  expect((await rows(ui)).some(t => t.includes(','))).toBe(false)
  await ui.unmount()
})

test('FR-2.7: an expired arming draws as merge without a new press', async ($, on) => {
  await mergeWorld(on)
  let now = 1_000_000
  on('clock.now', () => ({ value: now }))
  on('clock.after', () => new Promise(() => {}))
  on('clock.every', () => new Promise(() => {}))
  await $.prompt.submit({ text: '/znf:nonstop' })
  await new Promise(r => setTimeout(r, 20))
  const ui = await $.ui.mount(pane(HUMAN))
  await ui.press({ key: `merge:${URL_A}` })
  expect((await mergeButton(ui))?.props.label).toBe('xác nhận → main?')
  now += 5001
  await ui.unmount()
  const fresh = await $.ui.mount(pane(HUMAN))
  expect((await mergeButton(fresh))?.props.label).toBe('merge')
  await fresh.unmount()
})

test('FR-2.7: a press that read the arming before another merge finished does not merge again', async ($, on) => {
  const { calls } = await mergeWorld(on)
  let now = 1_000_000
  let held = false
  let reads = 0
  let letB: () => void = () => {}
  const gateB = new Promise<void>(r => (letB = r))
  on('clock.now', async () => {
    if (held && ++reads === 2) await gateB
    return { value: now }
  })
  on('clock.after', () => new Promise(() => {}))
  on('clock.every', () => new Promise(() => {}))
  await $.prompt.submit({ text: '/znf:nonstop' })
  await new Promise(r => setTimeout(r, 20))
  const ui = await $.ui.mount(pane(HUMAN))
  await ui.press({ key: `merge:${URL_A}` })
  held = true
  // Press B reads the clock second and waits there; press A merges meanwhile.
  const a = ui.press({ key: `merge:${URL_A}` })
  const b = ui.press({ key: `merge:${URL_A}` })
  await new Promise(r => setTimeout(r, 30))
  letB()
  await Promise.all([a, b])
  await new Promise(r => setTimeout(r, 30))
  expect(calls.merges).toHaveLength(1)
  await ui.unmount()
})

test('FR-2.7: a session start while a merge runs keeps the lock and the merging label', async ($, on) => {
  let release: () => void = () => {}
  const holdMerge = new Promise<void>(r => (release = r))
  const { calls } = await mergeWorld(on, { holdMerge })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane(HUMAN))
  await ui.press({ key: `merge:${URL_A}` })
  const merge = ui.press({ key: `merge:${URL_A}` })
  await clock.settle()
  expect((await mergeButton(ui))?.props.label).toBe('đang merge…')

  await $.session.start({ cwd: CWD, surface: 'terminal', isInteractive: true })
  await clock.settle()
  expect((await mergeButton(ui))?.props.label).toBe('đang merge…')
  await ui.press({ key: `merge:${URL_A}` })
  await ui.press({ key: `merge:${URL_A}` })
  await clock.settle()
  expect(calls.merges).toEqual([MERGE_ARGV])

  release()
  await merge
  await clock.settle()
  expect(calls.merges).toEqual([MERGE_ARGV])
  await ui.unmount()
})

const watchOf = async (ui: Parameters<typeof rows>[0]) => (await rows(ui)).filter(t => t.startsWith('Z/kit#238'))

const READY_CHECKS = [{ name: 'Zenie Review', status: 'COMPLETED', conclusion: 'SUCCESS' }]
const viewWith = (checks: unknown[], base = 'main') => ({
  exitCode: 0,
  stdout: JSON.stringify({ state: 'OPEN', mergeStateStatus: 'CLEAN', statusCheckRollup: checks, baseRefName: base }),
  stderr: '',
})

test('FR-2.8: CLEAN, Zenie pass and no open thread is ready, with a merge button', async ($, on) => {
  await mergeWorld(on, { gh: async () => viewWith(READY_CHECKS), threads: threadsOf(0, 5) })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const human = await $.ui.mount(pane(HUMAN))
  expect(await texts(human)).toContain('CHỜ MERGE')
  expect(await mergeButton(human)).toBeDefined()
  await human.unmount()
  const progress = await $.ui.mount(pane(PROGRESS))
  expect(await texts(progress)).toContain('ĐANG WATCH')
  expect(await watchOf(progress)).toEqual(['Z/kit#238 CLEAN · Zenie pass · thread 0/5'])
  await progress.unmount()
})

test('FR-2.8: open threads keep a CLEAN PR out of CHỜ MERGE and show thread 2/5', async ($, on) => {
  await mergeWorld(on, { gh: async () => viewWith(READY_CHECKS), threads: threadsOf(2, 5) })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const human = await $.ui.mount(pane(HUMAN))
  expect(await texts(human)).not.toContain('CHỜ MERGE')
  expect(await mergeButton(human)).toBeUndefined()
  await human.unmount()
  const progress = await $.ui.mount(pane(PROGRESS))
  expect((await watchOf(progress))[0]).toContain('thread 2/5')
  await progress.unmount()
})

test('FR-2.8: a Zenie check still running is not ready', async ($, on) => {
  await mergeWorld(on, {
    gh: async () => viewWith([{ name: 'Zenie Review', status: 'IN_PROGRESS', conclusion: '' }]),
    threads: threadsOf(0, 0),
  })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const human = await $.ui.mount(pane(HUMAN))
  expect(await mergeButton(human)).toBeUndefined()
  await human.unmount()
})

test('FR-2.8: a graphql error shows thread ? and keeps the PR out of CHỜ MERGE', async ($, on) => {
  await mergeWorld(on, {
    gh: async () => viewWith(READY_CHECKS),
    threads: () => ({ exitCode: 1, stdout: '', stderr: 'boom' }),
  })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const human = await $.ui.mount(pane(HUMAN))
  expect(await mergeButton(human)).toBeUndefined()
  await human.unmount()
  const progress = await $.ui.mount(pane(PROGRESS))
  expect((await watchOf(progress))[0]).toContain('thread ?')
  await progress.unmount()
})

test('FR-2.8: each poll runs gh pr view and the threads query per open PR, none for a settled one', async ($, on) => {
  const URL_C = 'https://github.com/Z/web/pull/7'
  let aState = 'OPEN'
  const calls = engine(on, {
    file: { text: ledgerText([row('t1', 'running', URL_A), row('t2', 'running', URL_C)]), mtimeMs: 1 },
    gh: async url => ({
      exitCode: 0,
      stdout: JSON.stringify({ state: url === URL_A ? aState : 'OPEN', mergeStateStatus: 'BLOCKED', statusCheckRollup: [] }),
      stderr: '',
    }),
  })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  expect(ghRuns(calls)).toHaveLength(2)
  expect(calls.graphql).toHaveLength(2)

  aState = 'MERGED'
  await clock.advance(60000)
  expect(ghRuns(calls)).toHaveLength(4)
  expect(calls.graphql).toHaveLength(3)

  await clock.advance(60000)
  expect(ghRuns(calls)).toHaveLength(5)
  expect(calls.graphql).toHaveLength(4)
})
