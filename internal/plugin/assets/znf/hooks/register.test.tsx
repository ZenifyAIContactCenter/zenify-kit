import { expect, mock, test } from 'claude-code/testing'

const CWD = '/w/app'
const LOCAL = `${CWD}/.zenify/nonstop/progress.md`
const GH_FIELDS = 'state,mergeStateStatus,statusCheckRollup,reviewDecision'
const URL_A = 'https://github.com/Z/kit/pull/238'
const URL_B = 'https://github.com/Z/hub/pull/9'

function today(): string {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

const row = (id: string, status: string, evidence = '-', issue = `#${id}`) =>
  `${id} · kit · issue · ${issue} · S · znf:fix · w-${id} · ${status} · ${evidence}`

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
}

// Every hook here stands for the engine beneath the plugin.
function engine(on: any, world: World): Calls {
  const calls: Calls = { open: [], close: [], stat: [], read: [], run: [], copy: [], status: [], toast: [] }
  const panes: { id: string }[] = []
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
    const view = world.gh
      ? await world.gh(e.argv[3])
      : { exitCode: 0, stdout: JSON.stringify({ state: 'OPEN', mergeStateStatus: 'CLEAN', statusCheckRollup: [] }), stderr: '' }
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

const links = async (ui: { findAll: (q: { type: string }) => Promise<{ props: Record<string, unknown> }[]> }) =>
  (await ui.findAll({ type: 'Link' })).map(e => e.props.href)

const ghRuns = (calls: Calls) => calls.run.filter(a => a[0] === 'gh')

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
  const heads = ['RUNNING', 'QUEUED', 'PR', 'done 1 · queued 1 · blocked 1'.slice(0, 19) + '…']
  const at = heads.map(h => all.indexOf(h))
  expect(at.every(i => i >= 0)).toBe(true)
  expect([...at].sort((a, b) => a - b)).toEqual(at)
  expect(all.every(t => t.length <= 20)).toBe(true)
  expect(all.some(t => t.startsWith('⚠ 1'))).toBe(true)
  expect(all.some(t => t.startsWith('this line'))).toBe(true)
  await ui.unmount()

  const human = await $.ui.mount(pane(HUMAN, 20))
  const needs = await texts(human)
  expect(needs[0]).toBe('CẦN ANH')
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
  expect(all).not.toContain('PR')
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
  expect(has(h, /t9 duyệt spec\?/)).toBe(true)
  expect(has(h, /t3 chọn A\/B\?/)).toBe(true)
  expect(h).toEqual(expect.arrayContaining(['Z/hub#9', ' CLEAN']))
  expect(await links(human)).toEqual([URL_B])
  expect(has(h, /^t1/)).toBe(false)
  await human.unmount()

  const progress = await $.ui.mount(pane(PROGRESS))
  const p = await texts(progress)
  expect(has(p, /^t1/)).toBe(true)
  expect(p).toContain('Z/be#12')
  expect(await links(progress)).toEqual(expect.arrayContaining(['https://github.com/Z/be/issues/12', URL_C]))
  expect(has(p, /^t2/)).toBe(true)
  expect(has(p, /CI đỏ \(lint\)/)).toBe(true)
  expect(p).toContain('done 1 · queued 1 · blocked 1')
  expect(has(p, /Z\/hub#9/)).toBe(false)
  await progress.unmount()

  const done = await $.ui.mount(pane(DONE))
  const d = await texts(done)
  expect(has(d, /^t4/)).toBe(true)
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
  expect(p).toEqual(expect.arrayContaining(['Z/hub#9', ' BLOCKED']))
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
