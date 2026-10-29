import { expect, mock, test } from 'claude-code/testing'

const CWD = '/w/app'
const LOCAL = `${CWD}/.zenify/nonstop/progress.md`
const GH_FIELDS = 'state,mergeStateStatus,statusCheckRollup,reviewDecision,baseRefName,headRefOid'
const HEAD = 'abc123'
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
  // Holds the review-threads query until this resolves (read on each call).
  holdThreads?: Promise<void>
  // The answer to the review-threads graphql call, by PR number; defaults to no threads.
  threads?: (n: string) => { exitCode: number; stdout: string; stderr: string }
  // Panes already open before the plugin acts (the legacy single pane).
  openPanes?: string[]
  // The answer to `zenify nonstop stats`; defaults to a missing estimates.md.
  stats?: { exitCode: number; stdout: string; stderr: string }
  // mtime of estimates.md next to the ledger; absent = the file does not exist.
  estimatesMtime?: number
  // mtime of durations.log next to the ledger; absent = the file does not exist.
  durationsMtime?: number
  gh?: (url: string) => Promise<{ exitCode: number; stdout: string; stderr: string }>
  // The ids `agent.spawn` answers, in order.
  agentIds?: string[]
  // durations.log next to the ledger; absent = the file does not exist.
  durations?: string
  writeError?: string
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
  write: { path: string; text: string }[]
}

// One slurped page, as `gh api graphql --paginate --slurp` answers.
const threadsOf = (open: number, total: number) => () => ({
  exitCode: 0,
  stdout: JSON.stringify([
    {
      data: {
        repository: {
          pullRequest: {
            reviewThreads: {
              nodes: Array.from({ length: total }, (_, i) => ({ isResolved: i >= open })),
              pageInfo: { hasNextPage: false, endCursor: null },
            },
          },
        },
      },
    },
  ]),
  stderr: '',
})

const statsJson = (over: Record<string, unknown> = {}) =>
  JSON.stringify({
    missing: false,
    scale: { source: 'default', n: 7, S: 5, M: 20, L: 60 },
    today: { date: today(), hit: 0, total: 0, over: 0, under: 0, points: 0, reviewMinutes: 0, ids: [] },
    week: { hitRate: null, pointsPerDay: null, reviewShare: null },
    tiers: { T1: 0, T2: 0, T3: 0 },
    rules: [],
    malformed: 0,
    ...over,
  })
const zenifyRuns = (calls: Calls) => calls.run.filter(a => a[0] === 'zenify')

// Every hook here stands for the engine beneath the plugin.
function engine(on: any, world: World): Calls {
  const calls: Calls = { open: [], close: [], stat: [], read: [], run: [], copy: [], status: [], toast: [], merges: [], graphql: [], write: [] }
  const panes: { id: string }[] = (world.openPanes ?? []).map(id => ({ id }))
  on('command.register', () => ({ value: { command: 'nonstop-pane' } }))
  on('session.start', (_: unknown, e: { cwd: string }) => ({ cwd: e.cwd }))
  on('session.cwd', () => ({ value: CWD }))
  on('prompt.submit', (_: unknown, e: { text: string }) => ({ text: e.text }))
  on('tool.call', () => ({ result: {} }))
  on('agent.spawn', () => ({ model: 'sonnet', agentId: world.agentIds?.shift() }))
  on('turn.complete', (_: unknown, e: { answer: string }) => ({ text: e.answer }))
  on('fs.write', (_: unknown, e: { path: string; text: string }) => {
    if (world.writeError) return { deny: world.writeError }
    calls.write.push({ path: e.path, text: e.text })
    world.durations = e.text
    return { value: undefined }
  })
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
    if (e.path.endsWith('/estimates.md')) {
      if (world.estimatesMtime === undefined) return { deny: `ENOENT: no such file, stat '${e.path}'` }
      return { value: { kind: 'file', size: 1, mtimeMs: world.estimatesMtime, isLink: false } }
    }
    if (e.path.endsWith('/durations.log')) {
      if (world.durationsMtime === undefined) return { deny: `ENOENT: no such file, stat '${e.path}'` }
      return { value: { kind: 'file', size: 1, mtimeMs: world.durationsMtime, isLink: false } }
    }
    // With gitDir set the cwd is a worktree: the ledger lives only in the main checkout.
    const isLedger = world.gitDir && !world.isLocal ? e.path.endsWith('/.znf/nonstop/progress.md') : e.path === LOCAL
    if (!world.file || !isLedger) return { deny: `ENOENT: no such file, stat '${e.path}'` }
    return { value: { kind: 'file', size: world.file.text.length, mtimeMs: world.file.mtimeMs, isLink: false } }
  })
  on('fs.read', (_: unknown, e: { path: string }) => {
    calls.read.push(e.path)
    if (e.path.endsWith('/durations.log')) {
      if (world.durations === undefined) return { deny: `ENOENT: no such file, open '${e.path}'` }
      return { value: world.durations }
    }
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
    if (e.argv[0] === 'zenify') return { value: world.stats ?? { exitCode: 0, stdout: statsJson({ missing: true }), stderr: '' } }
    if (e.argv[0] === 'gh' && e.argv[2] === 'merge') {
      calls.merges.push([...e.argv])
      if (world.holdMerge) await world.holdMerge
      if (world.mergeRejects) return { deny: world.mergeRejects }
      return { value: world.merge ?? { exitCode: 0, stdout: '', stderr: '' } }
    }
    if (e.argv[0] === 'gh' && e.argv[1] === 'api') {
      calls.graphql.push([...e.argv])
      if (world.holdThreads) await world.holdThreads
      const query = e.argv.find(a => a.startsWith('query=')) ?? ''
      return { value: (world.threads ?? threadsOf(0, 0))(/number:(\d+)/.exec(query)?.[1] ?? '') }
    }
    const view = world.gh
      ? await world.gh(e.argv[3])
      : { exitCode: 0, stdout: JSON.stringify({ state: 'OPEN', mergeStateStatus: 'CLEAN', statusCheckRollup: [{ name: 'Zenie Review', status: 'COMPLETED', conclusion: 'SUCCESS' }], baseRefName: 'main', headRefOid: HEAD }), stderr: '' }
    return { value: view }
  })
  return calls
}

const HUMAN = 'human'
const PROGRESS = 'progress'
const PENDING = 'pending'
const DONE = 'done'
const PANE = { id: 'nonstop', title: 'Nonstop', columns: 44 }
const LEGACY = ['nonstop-human', 'nonstop-progress', 'nonstop-pending', 'nonstop-done']

const pane = (bodyColumns = 44, bodyRows = 40) =>
  ({
    plugin: 'znf',
    component: 'Pane',
    requestId: 'nonstop',
    surface: 'terminal',
    props: {
      title: 'Nonstop',
      isFocused: false,
      bodyColumns,
      placement: 'dock',
      scroll: { offset: 0, bodyRows },
      view: {},
    },
  }) as const

// Mounts the one pane and selects `tab` (the state is shared, so every mount shows it).
async function mountTab($: any, tab: string, bodyColumns = 44, bodyRows = 40) {
  const ui = await $.ui.mount(pane(bodyColumns, bodyRows))
  await ui.press({ key: `tab:${tab}` })
  return ui
}

// Text and Link cells in document order: a PR row is a Link (the ref) then a Text (the status).
const allTexts = async (ui: { findAll: (q: { type?: string }) => Promise<{ type: string; text?: string }[]> }) =>
  (await ui.findAll({})).filter(e => (e.type === 'Text' || e.type === 'Link') && e.text !== ' │ ').map(e => e.text ?? '')

// The body only: everything above the footer's separator line.
const texts = async (ui: Parameters<typeof allTexts>[0]) => {
  const all = await allTexts(ui)
  const sep = all.findIndex(t => t.startsWith('─ Thống kê'))
  return sep < 0 ? all : all.slice(0, sep)
}

// The text of every row Box: one drawn line each, whatever its cells.
const rows = async (ui: { findAll: (q: { type: string }) => Promise<{ text: string; props: Record<string, unknown> }[]> }) =>
  (await ui.findAll({ type: 'Box' })).filter(b => b.props.flexDirection === 'row').map(b => b.text).slice(1)

const links = async (ui: { findAll: (q: { type: string }) => Promise<{ props: Record<string, unknown> }[]> }) =>
  (await ui.findAll({ type: 'Link' })).filter(e => e.props.label !== 'ⓘ').map(e => e.props.href)

const ghRuns = (calls: Calls) => calls.run.filter(a => a[0] === 'gh' && a[2] === 'view')

test('SC-7: only a nonstop prompt or Skill call opens the pane', async ($, on) => {
  const calls = engine(on, { file: null })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: 'hello' })
  await clock.settle()
  expect(calls.open).toHaveLength(0)

  await $.prompt.submit({ text: '/znf:nonstop continue', command: { name: 'znf:nonstop', args: 'continue' } })
  await clock.settle()
  expect(calls.open).toEqual([PANE])

  await $.tool.call({ tool: 'Skill', skill: 'znf:nonstop' })
  await clock.settle()
  expect(calls.open).toHaveLength(2)
})

test('SC-5: a ledger edit shows after the 2s tick; an unchanged mtime is not re-read', async ($, on) => {
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 100 } }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()

  const ui = await mountTab($, PROGRESS)
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

  const ui = await mountTab($, HUMAN)
  expect(await texts(ui)).toEqual(expect.arrayContaining(['Z/kit#238', ' CLEAN']))
  expect(await links(ui)).toContain(URL_A)
  await ui.press({ key: URL_A })
  expect(calls.copy).toEqual(['gh pr merge -R Z/kit 238 --merge'])
  expect(calls.toast).toEqual(['đã copy lệnh merge'])
  await ui.unmount()
})

test('FR-3.3: a ledger without PRs or stats shows only In Progress, with the pause icon at 0 running', async ($, on) => {
  const calls = engine(on, { file: { text: ledgerText([row('t3', 'queued')]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  expect(calls.status.at(-1)).toBe('nonstop ⏸ · In Progress: 0/1')
})

test('FR-3.3: the status line lists every non-zero item', async ($, on) => {
  const URL_C = 'https://github.com/Z/kit/pull/5'
  const world: World = {
    file: {
      text: ledgerText([
        row('d1', 'done', URL_A),
        row('d2', 'done', URL_B),
        row('d3', 'done', URL_C),
        row('d4', 'done'),
        row('d5', 'done'),
        row('r1', 'running'),
        row('q1', 'queued'),
        row('b1', 'blocked', 'chọn gì?'),
        row('w1', 'deferred until 2099-01-01'),
        row('c1', 'carry-over'),
        'Needs you: x1 — một',
        'Needs you: x2 — hai',
        'Needs you: x3 — ba',
        'Needs you: x4 — bốn',
      ]),
      mtimeMs: 1,
    },
    gh: async url => ({
      exitCode: 0,
      stdout: JSON.stringify(
        url === URL_C
          ? { state: 'OPEN', mergeStateStatus: 'BLOCKED', statusCheckRollup: [{ name: 'build', status: 'COMPLETED', conclusion: 'FAILURE' }] }
          : { state: 'OPEN', mergeStateStatus: 'CLEAN', statusCheckRollup: [{ name: 'Zenie Review', status: 'COMPLETED', conclusion: 'SUCCESS' }], baseRefName: 'main', headRefOid: HEAD },
      ),
      stderr: '',
    }),
    threads: n => (n === '5' ? threadsOf(3, 4)() : threadsOf(0, 0)()),
    stats: {
      exitCode: 0,
      stdout: statsJson({ today: { date: today(), hit: 5, total: 7, over: 1, under: 1, points: 11, reviewMinutes: 0, ids: [] } }),
      stderr: '',
    },
    estimatesMtime: 5,
  }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  expect(calls.status.at(-1)).toBe(
    'nonstop ▶ · In Progress: 1/2 · Need Human: 5 · Pending: 2 · Done: 5/8 · Ready: 2 · CI ✗: 1 · Threads: 3 · 11 pts · hit 71%',
  )
})

test('FR-1.1: no ledger on either path shows the empty state and clears the status', async ($, on) => {
  const calls = engine(on, { file: null, gitDir: '/w/repo/.git' })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane())
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
  const ui = await $.ui.mount(pane())
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
  const ui = await mountTab($, PROGRESS)
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

  const ui = await mountTab($, PROGRESS, 20)
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

  const human = await mountTab($, HUMAN, 20)
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
  const ui = await mountTab($, PROGRESS)
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
          : view('OPEN', 'CLEAN', [{ name: 'Zenie Review', status: 'COMPLETED', conclusion: 'SUCCESS' }]),
  }
  engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()

  const has = (all: string[], re: RegExp) => all.some(t => re.test(t))
  const human = await mountTab($, HUMAN)
  const h = await texts(human)
  const hr = await rows(human)
  expect(has(hr, /t9 duyệt spec\?/)).toBe(true)
  expect(has(hr, /t3 chọn A\/B\?/)).toBe(true)
  expect(h).toEqual(expect.arrayContaining(['Z/hub#9', ' CLEAN']))
  expect(await links(human)).toEqual([URL_B])
  expect(has(hr, /^t1/)).toBe(false)
  await human.unmount()

  const progress = await mountTab($, PROGRESS)
  const p = await texts(progress)
  const pr = await rows(progress)
  expect(has(pr, /^(⠋ )?t1/)).toBe(true)
  expect(p).toContain('Z/be#12')
  expect(await links(progress)).toEqual(expect.arrayContaining(['https://github.com/Z/be/issues/12', URL_C]))
  expect(has(pr, /^(⠋ )?t2/)).toBe(true)
  expect(has(p, /CI đỏ \(lint\)/)).toBe(true)
  expect(p).toContain('done 1 · queued 1 · blocked 1')
  expect(has(await rows(progress), /^Z\/hub#9 CLEAN/)).toBe(true)
  await progress.unmount()

  const done = await mountTab($, DONE)
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
    const ui = await mountTab($, id)
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

test('FR-3.1: session.start never opens the pane, even on a live day; re-entry does', async ($, on) => {
  const calls = engine(on, { file: { text: ledgerText(BASE), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.session.start({ cwd: CWD, surface: 'terminal', isInteractive: true })
  await clock.settle()
  const reads = calls.read.length
  await clock.advance(60000)
  expect(calls.open).toHaveLength(0)
  expect(ghRuns(calls)).toHaveLength(0)
  expect(calls.read).toHaveLength(reads)
  await $.prompt.submit({ text: '/znf:nonstop continue' })
  await clock.settle()
  expect(calls.open).toEqual([PANE])
})

test('FR-1.2: the nonstop-pane command toggles; closing stops the timers', async ($, on) => {
  const world: World = { file: { text: ledgerText([row('t1', 'running', URL_A)]), mtimeMs: 1 } }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await $.session.start({ cwd: CWD, surface: 'terminal', isInteractive: true })
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  await $.command.run({ command: 'nonstop-pane', args: '' })
  expect(calls.close.map(c => (c as { id: string }).id)).toEqual(['nonstop'])

  const reads = calls.read.length
  const gh = ghRuns(calls).length
  world.file = { text: ledgerText([row('t1', 'running', URL_A), row('t2', 'running')]), mtimeMs: 2 }
  await clock.advance(60000)
  expect(calls.read).toHaveLength(reads)
  expect(ghRuns(calls)).toHaveLength(gh)

  await $.command.run({ command: 'nonstop-pane', args: '' })
  await clock.settle()
  expect(calls.open).toHaveLength(2)
})

test('FR-1.2: a tab still open keeps the timers running', async ($, on) => {
  const world: World = { file: { text: ledgerText([row('t1', 'running')]), mtimeMs: 1 }, keepOpen: 'nonstop' }
  const calls = engine(on, world)
  const clock = mock.clock(on)
  await $.session.start({ cwd: CWD, surface: 'terminal', isInteractive: true })
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  await $.command.run({ command: 'nonstop-pane', args: '' })
  expect(calls.close).toHaveLength(1)

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
  const ui = await mountTab($, HUMAN)
  expect(await texts(ui)).toEqual(expect.arrayContaining(['Z/kit#238', ' CLEAN']))
  await ui.unmount()
})

test('FR-1.1: a local ledger created after session start is found', async ($, on) => {
  const world: World = { file: null, gitDir: '/w/repo/.git' }
  engine(on, world)
  const clock = mock.clock(on)
  await $.session.start({ cwd: CWD, surface: 'terminal', isInteractive: true })
  await clock.settle()
  world.file = { text: ledgerText(BASE), mtimeMs: 5 }
  world.isLocal = true
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await mountTab($, PROGRESS)
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

  const progress = await mountTab($, PROGRESS)
  const p = await texts(progress)
  expect(p.some(t => t.includes('Z/kit#238') || t.includes('Z/web#7'))).toBe(false)
  expect(p).toEqual(expect.arrayContaining(['Z/hub#9', ' BLOCKED · Zenie — · thread 0/0']))
  await progress.unmount()

  const done = await mountTab($, DONE)
  const d = await texts(done)
  expect(d).toEqual(expect.arrayContaining(['Z/kit#238', ' MERGED', 'Z/web#7', ' CLOSED']))
  expect(await links(done)).toEqual([URL_A, URL_C])
  expect(d.some(t => t.includes('Z/hub#9'))).toBe(false)
  await done.unmount()

  await clock.advance(60000)
  expect(ghRuns(calls).slice(3).map(a => a[3])).toEqual([URL_B, URL_C])
})

test('a closed PR reopened before the next poll leaves Done and is watched again', async ($, on) => {
  let state = 'CLOSED'
  const world: World = {
    file: { text: ledgerText([row('t1', 'running', URL_A)]), mtimeMs: 1 },
    gh: async () => ({
      exitCode: 0,
      stdout: JSON.stringify({ state, mergeStateStatus: 'BLOCKED', statusCheckRollup: [], baseRefName: 'main', headRefOid: HEAD }),
      stderr: '',
    }),
  }
  engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const done = await mountTab($, DONE)
  expect(await texts(done)).toEqual(expect.arrayContaining(['Z/kit#238', ' CLOSED']))
  await done.unmount()

  state = 'OPEN'
  await clock.advance(60000)
  const after = await mountTab($, DONE)
  expect((await texts(after)).some(t => t.includes('Z/kit#238'))).toBe(false)
  await after.unmount()
  const progress = await mountTab($, PROGRESS)
  expect(await texts(progress)).toEqual(expect.arrayContaining(['Z/kit#238', ' BLOCKED · Zenie — · thread 0/0']))
  await progress.unmount()
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

  const progress = await mountTab($, PROGRESS)
  const p = (await rows(progress)).map(t => t.replace(/^⠋ /, '')).filter(t => /^(P\d )?t\d/.test(t))
  expect(p.map(t => t.split(' kit')[0])).toEqual(['P0 t2', 'P2 t1', 't3'])
  await progress.unmount()

  const human = await mountTab($, HUMAN)
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

  const ui = await mountTab($, DONE)
  const has = async (re: RegExp) => (await ui.find({ type: 'Text', text: re })) !== undefined
  expect((await rows(ui))[0].endsWith('…')).toBe(true)
  expect(await has(new RegExp(evidence))).toBe(false)

  await ui.press({ key: 'task:done:t1' })
  expect(await has(new RegExp(evidence))).toBe(true)
  expect(await has(/kit · issue ·/)).toBe(true)

  await ui.press({ key: 'task:done:t1' })
  expect(await has(new RegExp(evidence))).toBe(false)
  await ui.unmount()
})

const HINT = '▾ chi tiết dài, cần pane cao hơn'

test('an expanded task taller than the room stays reachable in collapsed form', async ($, on) => {
  const evidence = `ev-${'e'.repeat(397)}`
  engine(on, { file: { text: ledgerText([row('t1', 'done', evidence)]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()

  const ui = await mountTab($, DONE, 44, 14)
  const hint = async () => (await ui.find({ type: 'Text', text: HINT })) !== undefined
  await ui.press({ key: 'task:done:t1' })
  expect(await ui.find({ type: 'Button', key: 'task:done:t1' })).toBeDefined()
  expect(await hint()).toBe(true)

  await ui.press({ key: 'task:done:t1' })
  expect(await hint()).toBe(false)
  expect(await ui.find({ type: 'Button', key: 'task:done:t1' })).toBeDefined()
  await ui.unmount()
})

test('an expanded task shows in full when the room is tall enough', async ($, on) => {
  const evidence = `ev-${'e'.repeat(397)}`
  engine(on, { file: { text: ledgerText([row('t1', 'done', evidence)]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()

  const ui = await mountTab($, DONE, 44, 60)
  await ui.press({ key: 'task:done:t1' })
  expect(await ui.find({ type: 'Text', text: new RegExp(evidence) })).toBeDefined()
  expect(await ui.find({ type: 'Text', text: HINT })).toBeUndefined()
  await ui.unmount()
})

test('FR-3.7: a Needs you line without a task expands to its full question', async ($, on) => {
  const question = `q-${'q'.repeat(100)}`
  engine(on, { file: { text: ledgerText([row('t1', 'running'), `Needs you: t9 — ${question}`]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()

  const ui = await mountTab($, HUMAN)
  const full = () => ui.find({ type: 'Text', text: new RegExp(`^ +${question}$`) })
  expect(await full()).toBeUndefined()
  await ui.press({ key: 'task:human:t9' })
  expect(await full()).toBeDefined()
  await ui.unmount()
})

const first = async (ui: Parameters<typeof texts>[0]) => (await texts(ui))[0]

test('FR-3.8: a running task spins the indicator every 250 ms', async ($, on) => {
  engine(on, { file: { text: ledgerText([row('t1', 'running')]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await mountTab($, PROGRESS)
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
  const ui = await mountTab($, PROGRESS)
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
  const ui = await mountTab($, PROGRESS)
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
  const ui = await mountTab($, PROGRESS)
  const before = await first(ui)
  await clock.advance(1000)
  expect(await first(ui)).toBe(before)
  await ui.unmount()
})

test('FR-3.12: opening the pane closes the four legacy tab panes', async ($, on) => {
  const calls = engine(on, { file: null, openPanes: LEGACY })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  expect(calls.close.map(c => (c as { id: string }).id)).toEqual(LEGACY)
})

test('FR-3.7: a needs line sharing its id with a task expands to the fields and the full question', async ($, on) => {
  const question = `q-${'q'.repeat(100)}`
  engine(on, { file: { text: ledgerText([row('t1', 'running'), `Needs you: t1 — ${question}`]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await mountTab($, HUMAN)
  await ui.press({ key: 'task:human:t1' })
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
  const ui = await mountTab($, PROGRESS)
  expect(['▶ ⠧ đang chạy 1 worker', '▶ ⠇ đang chạy 1 worker']).toContain(await first(ui))
  await ui.unmount()
})

const MERGE_ARGV = ['gh', 'pr', 'merge', '-R', 'Z/kit', '238', '--merge', '--match-head-commit', HEAD]
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
  const ui = await mountTab($, HUMAN)
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
  const ui = await mountTab($, HUMAN)
  await ui.press({ key: `merge:${URL_A}` })
  await clock.advance(5000)
  expect((await mergeButton(ui))?.props.label).toBe('merge')
  expect(calls.merges).toEqual([])
  await ui.unmount()
})

test('FR-2.7: the second press runs gh pr merge once, toasts and re-polls into Done', async ($, on) => {
  const { calls } = await mergeWorld(on, {
    gh: async () => ({
      exitCode: 0,
      stdout: JSON.stringify({
        state: calls.merges.length ? 'MERGED' : 'OPEN',
        mergeStateStatus: 'CLEAN',
        statusCheckRollup: [{ name: 'Zenie Review', status: 'COMPLETED', conclusion: 'SUCCESS' }],
        baseRefName: 'main',
        headRefOid: HEAD,
      }),
      stderr: '',
    }),
  })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await mountTab($, HUMAN)
  await ui.press({ key: `merge:${URL_A}` })
  await ui.press({ key: `merge:${URL_A}` })
  await clock.settle()
  expect(calls.merges).toEqual([MERGE_ARGV])
  expect(calls.toast).toContain('đã merge Z/kit#238')
  expect(await mergeButton(ui)).toBeUndefined()
  await ui.unmount()

  const done = await mountTab($, DONE)
  expect(await texts(done)).toEqual(expect.arrayContaining(['Z/kit#238', ' MERGED']))
  await done.unmount()
})

test('FR-2.7: a thread opened after the last poll is seen at the second press and nothing merges', async ($, on) => {
  let open = 0
  const { calls } = await mergeWorld(on, { gh: async () => viewWith(READY_CHECKS), threads: n => threadsOf(open, 1)(n) })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await mountTab($, HUMAN)
  await ui.press({ key: `merge:${URL_A}` })
  open = 1
  await ui.press({ key: `merge:${URL_A}` })
  await clock.settle()
  expect(calls.merges).toEqual([])
  expect(calls.toast).toContain('chưa merge được: còn 1 thread mở')
  expect(await mergeButton(ui)).toBeUndefined()
  await ui.unmount()
})

test('FR-2.7: a base branch changed after the arming is seen at the second press and nothing merges', async ($, on) => {
  let base = 'main'
  const { calls } = await mergeWorld(on, { gh: async () => viewWith(READY_CHECKS, base) })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await mountTab($, HUMAN)
  await ui.press({ key: `merge:${URL_A}` })
  expect((await mergeButton(ui))?.props.label).toBe('xác nhận → main?')
  base = 'staging'
  await ui.press({ key: `merge:${URL_A}` })
  await clock.settle()
  expect(calls.merges).toEqual([])
  expect(calls.toast).toContain('chưa merge được: base đổi main → staging')
  await ui.unmount()
})

test('FR-2.7: a failed fresh read at the second press merges nothing and is toasted', async ($, on) => {
  let fail = false
  const { calls } = await mergeWorld(on, {
    gh: async () => (fail ? { exitCode: 1, stdout: '', stderr: 'HTTP 502\nmore' } : viewWith(READY_CHECKS)),
  })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await mountTab($, HUMAN)
  await ui.press({ key: `merge:${URL_A}` })
  fail = true
  await ui.press({ key: `merge:${URL_A}` })
  await clock.settle()
  expect(calls.merges).toEqual([])
  expect(calls.toast).toContain('chưa merge được: gh lỗi: HTTP 502')
  await ui.unmount()
})

test('FR-2.7: a failed merge toasts the first stderr line and the PR stays in Need Human', async ($, on) => {
  const { calls } = await mergeWorld(on, {
    merge: { exitCode: 1, stdout: '', stderr: 'GraphQL: Pull request is not mergeable\nmore' },
  })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await mountTab($, HUMAN)
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
  const ui = await mountTab($, HUMAN)
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
    const ui = await mountTab($, id)
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
  const ui = await mountTab($, HUMAN)
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
  const ui = await mountTab($, HUMAN)
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
  const ui = await mountTab($, HUMAN)
  expect(await mergeButton(ui)).toBeUndefined()
  await ui.unmount()
})

test('FR-2.7: the button row draws no stray comma', async ($, on) => {
  await mergeWorld(on)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await mountTab($, HUMAN)
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
  const ui = await mountTab($, HUMAN)
  await ui.press({ key: `merge:${URL_A}` })
  expect((await mergeButton(ui))?.props.label).toBe('xác nhận → main?')
  now += 5001
  await ui.unmount()
  const fresh = await mountTab($, HUMAN)
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
  const ui = await mountTab($, HUMAN)
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
  const ui = await mountTab($, HUMAN)
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
  stdout: JSON.stringify({ state: 'OPEN', mergeStateStatus: 'CLEAN', statusCheckRollup: checks, baseRefName: base, headRefOid: HEAD }),
  stderr: '',
})

test('FR-2.8: CLEAN, Zenie pass and no open thread is ready, with a merge button', async ($, on) => {
  await mergeWorld(on, { gh: async () => viewWith(READY_CHECKS), threads: threadsOf(0, 5) })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const human = await mountTab($, HUMAN)
  expect(await texts(human)).toContain('CHỜ MERGE')
  expect(await mergeButton(human)).toBeDefined()
  await human.unmount()
  const progress = await mountTab($, PROGRESS)
  expect(await texts(progress)).toContain('ĐANG WATCH')
  expect(await watchOf(progress)).toEqual(['Z/kit#238 CLEAN · Zenie pass · thread 0/5'])
  await progress.unmount()
})

test('FR-2.8: open threads keep a CLEAN PR out of CHỜ MERGE and show thread 2/5', async ($, on) => {
  await mergeWorld(on, { gh: async () => viewWith(READY_CHECKS), threads: threadsOf(2, 5) })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const human = await mountTab($, HUMAN)
  expect(await texts(human)).not.toContain('CHỜ MERGE')
  expect(await mergeButton(human)).toBeUndefined()
  await human.unmount()
  const progress = await mountTab($, PROGRESS)
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
  const human = await mountTab($, HUMAN)
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
  const human = await mountTab($, HUMAN)
  expect(await mergeButton(human)).toBeUndefined()
  await human.unmount()
  const progress = await mountTab($, PROGRESS)
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

test('FR-2.8: a Zenie check that is absent is not ready', async ($, on) => {
  await mergeWorld(on, { gh: async () => viewWith([]), threads: threadsOf(0, 0) })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const human = await mountTab($, HUMAN)
  expect(await mergeButton(human)).toBeUndefined()
  await human.unmount()
})

test('FR-2.8: the PR is not ready while a poll waits for its threads, and stays not ready with open threads', async ($, on) => {
  const { world } = await mergeWorld(on, { gh: async () => viewWith(READY_CHECKS), threads: threadsOf(0, 3) })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const human = await mountTab($, HUMAN)
  expect(await mergeButton(human)).toBeDefined()

  let release: () => void = () => {}
  world.holdThreads = new Promise<void>(r => (release = r))
  world.threads = threadsOf(2, 3)
  await clock.advance(60000)
  expect(await mergeButton(human)).toBeUndefined()
  expect(await texts(human)).not.toContain('CHỜ MERGE')

  release()
  await clock.settle()
  expect(await mergeButton(human)).toBeUndefined()
  await human.unmount()
})

test('FR-2.8: the merge button is gone once a poll finds an open thread, and nothing merged', async ($, on) => {
  let open = 0
  const { calls } = await mergeWorld(on, { gh: async () => viewWith(READY_CHECKS), threads: n => threadsOf(open, 1)(n) })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const human = await mountTab($, HUMAN)
  expect(await mergeButton(human)).toBeDefined()
  open = 1
  await clock.advance(60000)
  expect(await mergeButton(human)).toBeUndefined()
  expect(calls.merges).toEqual([])
  await human.unmount()
})

test('FR-2.8: the merge button is gone once Zenie stops passing, and nothing merged', async ($, on) => {
  let checks: unknown[] = READY_CHECKS
  const { calls } = await mergeWorld(on, { gh: async () => viewWith(checks) })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const human = await mountTab($, HUMAN)
  expect(await mergeButton(human)).toBeDefined()
  checks = [{ name: 'Zenie Review', status: 'COMPLETED', conclusion: 'FAILURE' }]
  await clock.advance(60000)
  expect(await mergeButton(human)).toBeUndefined()
  expect(calls.merges).toEqual([])
  await human.unmount()
})

test('FR-2.8: before the first threads answer the row shows thread … and the PR is not ready', async ($, on) => {
  let release: () => void = () => {}
  const holdThreads = new Promise<void>(r => (release = r))
  await mergeWorld(on, { gh: async () => viewWith(READY_CHECKS), holdThreads })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const progress = await mountTab($, PROGRESS)
  expect((await watchOf(progress))[0]).toContain('thread …')
  await progress.unmount()
  const human = await mountTab($, HUMAN)
  expect(await mergeButton(human)).toBeUndefined()
  await human.unmount()
  release()
  await clock.settle()
})

const openDone = async ($: any, on: any, world: World, now?: number) => {
  const calls = engine(on, world)
  const clock = mock.clock(on, now === undefined ? {} : { now })
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  return { calls, clock }
}

test('FR-3.10: the Done tab shows the learned scale, today and week lines, and a rule', async ($, on) => {
  const stats = statsJson({
    scale: { source: 'learned', n: 31, S: 4, M: 12, L: 40 },
    today: { date: today(), hit: 3, total: 5, over: 1, under: 1, points: 11, reviewMinutes: 7, ids: ['t1'] },
    week: { hitRate: 0.714, pointsPerDay: 9.25, reviewShare: 0.334 },
    tiers: { T1: 4, T2: 2, T3: 1 },
    rules: ['hub fix: est lên 1 size'],
  })
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 1 }, stats: { exitCode: 0, stdout: stats, stderr: '' }, estimatesMtime: 5 }
  const { calls } = await openDone($, on, world)
  const ui = await mountTab($, DONE)
  const t = await allTexts(ui)
  expect(t).toContain('thang (học từ 31): S<4m · M<12m · L<40m')
  expect(t).toContain('hôm nay: đúng 3/5 · 1 cao · 1 thấp · 11 điểm')
  expect(t).toContain('review 7 phút (33%) · T1 4 T2 2 T3 1')
  expect(t).toContain('7 ngày: đúng 71% · 9.3 điểm/ngày')
  // calibration rules are not in the footer
  expect(t.some(x => x.includes('hub fix'))).toBe(false)
  expect(zenifyRuns(calls)[0]).toEqual(['zenify', 'nonstop', 'stats', '--json', '--dir', `${CWD}/.zenify/nonstop`])
  await ui.unmount()
})

test('FR-3.10: default scale line and null week render as dashes', async ($, on) => {
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 1 }, stats: { exitCode: 0, stdout: statsJson(), stderr: '' } }
  await openDone($, on, world)
  const ui = await mountTab($, DONE)
  const t = await allTexts(ui)
  expect(t).toContain('thang (mặc định 7/20): S<5m · M<20m · L<60m')
  expect(t).toContain('7 ngày: đúng — · — điểm/ngày')
  expect(t).toContain('review 0 phút · T1 0 T2 0 T3 0')
  await ui.unmount()
})

test('FR-3.10: a done task with no estimate line is named; malformed lines are counted', async ($, on) => {
  const stats = statsJson({ today: { date: today(), hit: 1, total: 1, over: 0, under: 0, points: 1, reviewMinutes: 0, ids: ['t1'] }, malformed: 2 })
  const world: World = {
    file: { text: ledgerText([row('t1', 'done'), row('t4', 'done')]), mtimeMs: 1 },
    stats: { exitCode: 0, stdout: stats, stderr: '' },
  }
  await openDone($, on, world)
  const ui = await mountTab($, DONE)
  const t = await allTexts(ui)
  // one warning line, the unrecorded task wins over the malformed count
  expect(t).toContain('⚠ 1 task done chưa ghi estimate: t4')
  expect(t.some(x => x.includes('sai format'))).toBe(false)
  await ui.unmount()
})

test('FR-3.10: missing estimates.md reads "chưa có estimates.md"', async ($, on) => {
  await openDone($, on, { file: { text: ledgerText(BASE), mtimeMs: 1 } })
  const ui = await mountTab($, DONE)
  expect(await allTexts(ui)).toContain('chưa có estimates.md')
  await ui.unmount()
})

test('FR-3.10: a failing stats command shows its first stderr line, not a swallowed error', async ($, on) => {
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 1 }, stats: { exitCode: 1, stdout: '', stderr: 'boom: cannot read\nmore' } }
  await openDone($, on, world)
  const ui = await mountTab($, DONE)
  expect(await allTexts(ui)).toContain('⚠ lỗi thống kê: boom: cannot read')
  await ui.unmount()
})

test('FR-3.10: output that is not stats JSON reports the parse failure', async ($, on) => {
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 1 }, stats: { exitCode: 0, stdout: 'not json', stderr: '' } }
  await openDone($, on, world)
  const ui = await mountTab($, DONE)
  expect((await allTexts(ui)).some(x => x.startsWith('⚠ lỗi thống kê: '))).toBe(true)
  await ui.unmount()
})

test('FR-3.10: stats re-run on a 2s tick only when estimates.md mtime changed', async ($, on) => {
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 1 }, estimatesMtime: 10 }
  const { calls, clock } = await openDone($, on, world)
  expect(zenifyRuns(calls)).toHaveLength(1)
  await clock.advance(2000)
  await clock.advance(2000)
  expect(zenifyRuns(calls)).toHaveLength(1)
  world.estimatesMtime = 20
  await clock.advance(2000)
  expect(zenifyRuns(calls)).toHaveLength(2)
  await clock.advance(2000)
  expect(zenifyRuns(calls)).toHaveLength(2)
})

test('FR-3.10: stats re-run when durations.log mtime changes', async ($, on) => {
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 1 }, estimatesMtime: 10 }
  const { calls, clock } = await openDone($, on, world)
  await clock.advance(2000)
  expect(zenifyRuns(calls)).toHaveLength(1)
  world.durationsMtime = 30
  await clock.advance(2000)
  expect(zenifyRuns(calls)).toHaveLength(2)
  await clock.advance(2000)
  expect(zenifyRuns(calls)).toHaveLength(2)
  world.durationsMtime = 40
  await clock.advance(2000)
  expect(zenifyRuns(calls)).toHaveLength(3)
})

test('FR-3.10: stats re-run when the local date changes', async ($, on) => {
  const midnight = new Date()
  midnight.setHours(24, 0, 0, 0)
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 1 }, estimatesMtime: 10 }
  // Opened 3 s before local midnight: the second tick is the next day.
  const { calls, clock } = await openDone($, on, world, midnight.getTime() - 3000)
  await clock.advance(2000)
  expect(zenifyRuns(calls)).toHaveLength(1)
  await clock.advance(2000)
  expect(zenifyRuns(calls)).toHaveLength(2)
  await clock.advance(2000)
  expect(zenifyRuns(calls)).toHaveLength(2)
})

test('FR-3.10: a failed stats run is retried on the next tick', async ($, on) => {
  const world: World = {
    file: { text: ledgerText(BASE), mtimeMs: 1 },
    estimatesMtime: 10,
    stats: { exitCode: 1, stdout: '', stderr: 'boom' },
  }
  const { calls, clock } = await openDone($, on, world)
  expect(zenifyRuns(calls)).toHaveLength(1)
  world.stats = { exitCode: 0, stdout: statsJson(), stderr: '' }
  await clock.advance(2000)
  expect(zenifyRuns(calls)).toHaveLength(2)
  await clock.advance(2000)
  expect(zenifyRuns(calls)).toHaveLength(2)
})

test('FR-3.10: a plain session makes no zenify call', async ($, on) => {
  const calls = engine(on, { file: { text: ledgerText([row('t1', 'done')]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.session.start({ cwd: CWD, surface: 'terminal', isInteractive: true })
  await clock.settle()
  await clock.advance(4000)
  expect(zenifyRuns(calls)).toHaveLength(0)
})

const LOG = `${CWD}/.zenify/nonstop/durations.log`
const WORKER_PROMPT = [
  'Task:       t14 — fix login redirect, source issue, size S',
  'Repo:       /w/repos/app · worktree slug: fix-login (git fetch origin; wt new fix-login --type fix --base main)',
].join('\n')
const REVIEW_PROMPT = 'Review the diff. WT=/w/repos/app/.worktrees/fix-login'
const ended = (agentId: string, durationMs: number) =>
  ({ answer: 'ok', durationMs, isAborted: false, turnId: `turn-${agentId}-${durationMs}`, agentId, reason: 'answer' }) as any
const logLines = (world: World) => (world.durations ?? '').split('\n').filter(Boolean)
const ISO = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z · /

test('FR-6.1: a worker run logs work, its reviewer run logs review, a resumed run logs again', async ($, on) => {
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 1 }, agentIds: ['a-work', 'a-review', 'a-work'] }
  engine(on, world)
  mock.clock(on)
  await $.agent.spawn({ prompt: WORKER_PROMPT })
  await $.turn.complete(ended('a-work', 245000))
  await $.agent.spawn({ prompt: REVIEW_PROMPT })
  await $.turn.complete(ended('a-review', 60000))
  // A resume re-spawns the same agent with the resuming message, which names the worktree.
  await $.agent.spawn({ prompt: 'Fix the findings in /w/repos/app/.worktrees/fix-login', resume: { agentId: 'a-work' } } as any)
  await $.turn.complete(ended('a-work', 30000))
  const lines = logLines(world)
  expect(lines).toHaveLength(3)
  expect(lines.every(l => ISO.test(l))).toBe(true)
  expect(lines.map(l => l.replace(ISO, ''))).toEqual(['t14 · work · 245000', 't14 · review · 60000', 't14 · work · 30000'])
})

test('FR-6.1: lines are appended to an existing durations.log next to the ledger', async ($, on) => {
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 1 }, agentIds: ['a1'], durations: 'old line\n' }
  const calls = engine(on, world)
  mock.clock(on)
  await $.agent.spawn({ prompt: WORKER_PROMPT })
  await $.turn.complete(ended('a1', 245000))
  expect(calls.write.map(w => w.path)).toEqual([LOG])
  expect(logLines(world)[0]).toBe('old line')
  expect(logLines(world)[1]).toMatch(/ · t14 · work · 245000$/)
})

test('FR-6.1: a reviewer of an unknown worktree logs unknown; a plain prompt and the main loop log nothing', async ($, on) => {
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 1 }, agentIds: ['a-rev', 'a-other'] }
  const calls = engine(on, world)
  mock.clock(on)
  await $.agent.spawn({ prompt: 'Review WT=/w/repos/app/.worktrees/other-slug' })
  await $.turn.complete(ended('a-rev', 1000))
  await $.agent.spawn({ prompt: 'Find where login is handled' })
  await $.turn.complete(ended('a-other', 2000))
  await $.turn.complete({ ...ended('x', 3000), agentId: undefined })
  expect(calls.write).toHaveLength(1)
  expect(logLines(world).map(l => l.replace(ISO, ''))).toEqual(['unknown · review · 1000'])
})

test('FR-6.1: a failed write is toasted, not swallowed', async ($, on) => {
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 1 }, agentIds: ['a1'], writeError: 'EACCES: permission denied' }
  const calls = engine(on, world)
  mock.clock(on)
  await $.agent.spawn({ prompt: WORKER_PROMPT })
  await $.turn.complete(ended('a1', 245000))
  expect(calls.toast).toHaveLength(1)
  expect(calls.toast[0].startsWith('lỗi đo thời gian: ') && calls.toast[0].includes('EACCES: permission denied')).toBe(true)
})

test('FR-6.1: a plain session with no ledger writes nothing', async ($, on) => {
  const world: World = { file: null, agentIds: ['a1'] }
  const calls = engine(on, world)
  mock.clock(on)
  await $.agent.spawn({ prompt: WORKER_PROMPT })
  await $.turn.complete(ended('a1', 245000))
  expect(calls.write).toHaveLength(0)
  expect(calls.toast).toHaveLength(0)
})

test('FR-1.2: a nonstop tab left open across session.start keeps refreshing', async ($, on) => {
  const world: World = { file: { text: ledgerText([row('t1', 'done')]), mtimeMs: 1 }, openPanes: ['nonstop'] }
  engine(on, world)
  const clock = mock.clock(on)
  await $.session.start({ cwd: CWD, surface: 'terminal', isInteractive: true })
  await clock.settle()
  world.file = { text: ledgerText([row('t1', 'done'), row('t9', 'running')]), mtimeMs: 2 }
  await clock.advance(2000)
  const ui = await mountTab($, PROGRESS)
  expect((await ui.find({ type: 'Text', text: /t9/ }))?.text).toContain('t9')
  await ui.unmount()
})

test('FR-2.7: a PR url with an unsafe repo name shows no merge or copy button', async ($, on) => {
  const BAD = 'https://github.com/Z/ki"t/pull/5'
  const world: World = { file: { text: ledgerText([row('t1', 'running', BAD)]), mtimeMs: 1 }, gh: async () => viewWith(READY_CHECKS), threads: threadsOf(0, 5) }
  engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const human = await mountTab($, HUMAN)
  expect(await mergeButton(human)).toBeUndefined()
  expect(await texts(human)).not.toContain('CHỜ MERGE')
  await human.unmount()
})

const defer = (id: string, until: string, priority = '-') => row(id, `deferred until ${until}`, '-', `#${id}`, priority)

test('FR-3.11: Pending lists deferred by date then carry-over by priority', async ($, on) => {
  engine(on, {
    file: {
      text: ledgerText([
        defer('d2', '2026-11-02', 'P0'),
        defer('d1', '2026-10-30', 'P2'),
        row('c2', 'carry-over', '-', '#c2', 'P2'),
        row('c0', 'carry-over', '-', '#c0', 'P0'),
        row('t1', 'running'),
      ]),
      mtimeMs: 1,
    },
  })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await mountTab($, PENDING)
  const r = await rows(ui)
  const t = await texts(ui)
  expect(t).toContain('HOÃN')
  expect(t).toContain('CARRY-OVER')
  const at = (s: string) => r.findIndex(x => x.includes(s))
  expect(r[at('d1')]).toContain('tới 2026-10-30')
  expect(r[at('d2')]).toContain('tới 2026-11-02')
  expect(at('d1')).toBeLessThan(at('d2'))
  expect(r[at('c0')]).not.toContain('tới')
  expect(at('c0')).toBeLessThan(at('c2'))
  expect(r.some(x => x.includes('t1'))).toBe(false)
  await ui.unmount()
})

test('FR-3.11: Pending shows only the sections that have tasks', async ($, on) => {
  engine(on, { file: { text: ledgerText([row('c1', 'carry-over'), row('t1', 'running')]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await mountTab($, PENDING)
  const t = await texts(ui)
  expect(t).toContain('CARRY-OVER')
  expect(t).not.toContain('HOÃN')
  await ui.unmount()
})

test('FR-3.11: Pending with nothing deferred or carried over says trống', async ($, on) => {
  engine(on, { file: { text: ledgerText([row('t1', 'running')]), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await mountTab($, PENDING)
  expect(await texts(ui)).toEqual(['trống'])
  await ui.unmount()
})

test('FR-3.12: the tab row marks the selected tab with ▸, dims the others, and pressing a tab switches body', async ($, on) => {
  engine(on, { file: { text: ledgerText([row('t1', 'queued'), row('t2', 'deferred until 2099-01-01'), 'Needs you: x — hỏi']), mtimeMs: 1 } })
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const ui = await $.ui.mount(pane())
  const header = async () => (await ui.findAll({ type: 'Box' })).filter(b => b.props.flexDirection === 'row')[0].text
  const tabs = async () => (await ui.findAll({ type: 'Button' })).filter(b => String(b.props.key).startsWith('tab:'))
  expect(await header()).toBe('▸Need Human │ In Progress │ Pending │ Done')
  expect((await tabs()).map(b => [b.props.key, !!b.props.dimColor])).toEqual([
    ['tab:human', false],
    ['tab:progress', true],
    ['tab:pending', true],
    ['tab:done', true],
  ])
  expect((await tabs()).every(b => b.props.plain && b.props.hotkey === undefined && b.props.variant === undefined)).toBe(true)
  expect(await texts(ui)).toContain('CẦN ANH')
  await ui.press({ key: 'tab:pending' })
  expect(await header()).toBe('Need Human │ In Progress │ ▸Pending │ Done')
  expect((await tabs()).filter(b => !b.props.dimColor).map(b => b.props.key)).toEqual(['tab:pending'])
  expect(await texts(ui)).toContain('HOÃN')
  expect(await texts(ui)).not.toContain('CẦN ANH')
  await ui.unmount()
})

test('FR-3.12: a failing stats run shows the ⚠ lỗi thống kê line in the footer under every tab', async ($, on) => {
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 1 }, stats: { exitCode: 1, stdout: '', stderr: 'boom: no\nmore' } }
  engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  for (const tab of [HUMAN, DONE]) {
    const ui = await mountTab($, tab)
    const all = await allTexts(ui)
    expect(all.slice(all.findIndex(t => t.startsWith('─ Thống kê ')) + 3)).toEqual(['⚠ lỗi thống kê: boom: no'])
    await ui.unmount()
  }
})

test('FR-3.12: the Thống kê footer sits under every tab with the scale line', async ($, on) => {
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 1 }, stats: { exitCode: 0, stdout: statsJson(), stderr: '' }, estimatesMtime: 5 }
  engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  for (const tab of [HUMAN, PROGRESS, PENDING, DONE]) {
    const ui = await mountTab($, tab)
    const all = await allTexts(ui)
    const sep = all.findIndex(t => t.startsWith('─ Thống kê '))
    expect(sep).toBeGreaterThanOrEqual(0)
    expect(all.slice(sep, sep + 3).join('')).toHaveLength(44)
    expect(all.slice(sep + 3)).toEqual([
      'thang (mặc định 7/20): S<5m · M<20m · L<60m',
      'hôm nay: đúng 0/0 · 0 cao · 0 thấp · 0 điểm',
      '7 ngày: đúng — · — điểm/ngày',
      'review 0 phút · T1 0 T2 0 T3 0',
      '⚠ 1 task done chưa ghi estimate: t1',
    ])
    await ui.unmount()
  }
})

test('FR-3.12: a body taller than its room is cut to room - 1 rows and ends with +N dòng', async ($, on) => {
  const many = Array.from({ length: 12 }, (_, i) => row(`q${i}`, 'queued'))
  const world: World = { file: { text: ledgerText(many), mtimeMs: 1 }, stats: { exitCode: 0, stdout: statsJson(), stderr: '' }, estimatesMtime: 5 }
  engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  // 14 rows: header 1 + separator 1 + 4 footer lines = room 8; the body is 1 + 1 + 12 + 1 = 15 rows
  const ui = await mountTab($, PROGRESS, 44, 14)
  const body = await texts(ui)
  const taskRows = (await rows(ui)).filter(t => t.startsWith('q'))
  // 7 shown rows: the running line, QUEUED and 5 tasks
  expect(taskRows).toHaveLength(5)
  expect(body.at(-1)).toBe(`+${15 - 7} dòng`)
  await ui.unmount()
})

test('FR-3.13: the stats footer separator carries an ⓘ link to the metrics definitions, within the width', async ($, on) => {
  const world: World = { file: { text: ledgerText(BASE), mtimeMs: 1 }, stats: { exitCode: 0, stdout: statsJson(), stderr: '' }, estimatesMtime: 5 }
  engine(on, world)
  const clock = mock.clock(on)
  await $.prompt.submit({ text: '/znf:nonstop' })
  await clock.settle()
  const url = 'https://github.com/ZenifyAIContactCenter/zenify-knowledge/blob/main/reference/zenify-kit/nonstop-metrics.md'
  for (const id of [PROGRESS, HUMAN, DONE]) {
    const ui = await mountTab($, id, 44)
    const info = (await ui.findAll({ type: 'Link' })).filter(l => l.props.label === 'ⓘ')
    expect(info.map(l => l.props.href)).toEqual([url])
    const sep = (await rows(ui)).find(t => t.startsWith('─ Thống kê'))
    expect(sep).toBeDefined()
    expect(sep!.length).toBeLessThanOrEqual(44)
    await ui.unmount()
  }
})
