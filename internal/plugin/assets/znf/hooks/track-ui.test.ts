import { expect, test } from 'claude-code/testing'
import type { BacklogItem, TrackItem, TrackStats } from '../types'
import { COPY, FUN_TIPS, STATUS_OFF, TABS, bar, errorStatus, fmtDur, fmtNum, hourBins, modelName, sparkline, stackBar, tipLine, isTrackPrompt, isTrackSkill, parseTrackStats, prLabel, promptSkillOf, ruleText, statusText, tabLines, trackContext, trackSectionOf } from './track-ui'

const A = 'aaaaaaaa-1111-2222-3333-444444444444'
const B = 'bbbbbbbb-5555-6666-7777-888888888888'
const X = 'cook-261030-0a0a'
const Y = 'fix-261030-0b0b'

const item = (over: Partial<TrackItem>): TrackItem => ({
  id: X,
  from: '',
  live: false,
  kind: 'cook',
  repo: 'kit',
  est: 'M',
  session: A,
  cwd: '/w/repos/kit',
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
  tokens: { 'claude-opus-5-5': { in: 100, out: 200, cacheRead: 0, cacheCreation: 0 } },
  ...over,
})

const stats = (over: Partial<TrackStats> = {}): TrackStats => ({
  enabled: true,
  malformed: 0,
  scale: { source: 'default', n: 0, S: 15, M: 45, L: 120 },
  rules: [],
  today: { date: '2026-10-30', plans: {}, started: {}, ended: {}, done: {}, cancelled: {}, rolledBack: {}, wallMin: 0, agentMin: 0, tasks: 0, rounds: 0, prsOpened: 0, merged: 0, prsWaiting: 0, mergedError: '', items: [], dailyPlan: { items: [], estMin: 0, wallMin: 0 }, prOpen: [], prStatusError: '' },
  running: [],
  estimate: { byKind: {}, hotfix: { n: 0, medianMin: null, p90Min: null }, phaseShare: {} },
  quality: { byKind: {}, fix: { n: 0, roundAvg: null, wideRate: null, regressionRate: null } },
  backlog: { items: [], ghError: '', planeError: '' },
  onboarded: true,
  kitchen: { carried: [], lastDay: null, cooked: [], rollingBoil: [], cooking: [], titleError: '' },
  kitUpdate: null,
  ...over,
})

const bl = (source: string, over: Partial<BacklogItem> = {}): BacklogItem => ({
  source,
  id: `${source}-1`,
  repo: 'kit',
  est: 'M',
  ref: '-',
  title: `${source} title`,
  ageDays: 3,
  code: '',
  ...over,
})

const texts = (lines: { text: string }[]) => lines.map(l => l.text)

test('SC-44: contextual tips by priority, one at most', () => {
  const tail = (s: TrackStats, id?: string) => tipLine(s, A, id, true, NOW)
  const backlog = { items: [bl('manual'), bl('github')], ghError: '', planeError: '' }
  const prs = { ...stats().today, prsOpened: 3, merged: 1, prsWaiting: 2 }
  // idle, backlog N > 0
  expect(tail(stats({ backlog }))).toBe('Tip: 2 items in Backlog — type "/znf:track ls"')
  // open id, est not a size, wall >= 5
  expect(tail(stats({ running: [item({ est: '-', wallMin: 5 })] }), X)).toBe('Tip: no estimate yet — type "hey claude estimate this"')
  // wall 4 and no est: no tip
  expect(tail(stats({ running: [item({ est: '-', wallMin: 4 })] }), X)).toBeUndefined()
  // over est: M threshold is scale.M = 45, compared with wallMin
  expect(tail(stats({ running: [item({ est: 'M', wallMin: 50 })] }), X)).toBe('Tip: over est M (50m/45m) — split the work or re-estimate')
  expect(tail(stats({ running: [item({ est: 'M', wallMin: 44, agentMin: 99 })] }), X)).toBeUndefined()
  // PRs waiting
  expect(tail(stats({ today: prs }))).toBe('Tip: 2 PRs waiting to merge — type "hey claude check PR"')
  // backlog beats PRs waiting
  expect(tail(stats({ backlog, today: prs }))).toBe('Tip: 2 items in Backlog — type "/znf:track ls"')
  // over est beats PRs waiting with an open id
  expect(tail(stats({ today: prs, running: [item({ est: 'S', wallMin: 15 })] }), X)).toContain('over est S (15m/15m)')
  // statusText never carries a tip
  expect(statusText(stats({ backlog, today: prs }), A, undefined)).not.toContain('Tip:')
  expect(statusText(stats({ today: prs }), A, undefined)).toBe('track ⏸ · Today: 0/0 · PR 3 (1 merged)')
})

test('SC-45: tips false hides every tip, keeps onboard and the off line', () => {
  const backlog = { items: [bl('manual')], ghError: '', planeError: '' }
  const prs = { ...stats().today, prsOpened: 3, merged: 1, prsWaiting: 2 }
  const noTips = (s: TrackStats, id?: string) => tipLine(s, A, id, false, NOW, true)
  expect(noTips(stats({ backlog }))).toBeUndefined()
  expect(noTips(stats({ today: prs }))).toBeUndefined()
  expect(noTips(stats({ running: [item({ est: '-', wallMin: 5 })] }), X)).toBeUndefined()
  expect(noTips(stats({ running: [item({ est: 'M', wallMin: 95 })] }), X)).toBeUndefined()
  expect(noTips({ ...stats(), onboarded: false })).toBe(COPY.onboard)
  expect(noTips(stats({ enabled: false }))).toBeUndefined()
  expect(statusText(stats({ enabled: false }), A, X, false)).toBe(STATUS_OFF)
  // default is tips on
  expect(tipLine(stats({ backlog }), A, undefined, undefined, NOW)).toBe('Tip: 1 items in Backlog — type "/znf:track ls"')
})

test('FR-11.2/SC-20: enabled false is the off line, verbatim', () => {
  expect(STATUS_OFF).toBe('⏸ track off · enable: /config → znf')
  expect(statusText(stats({ enabled: false }), A, X)).toBe(STATUS_OFF)
})

test('SC-21: the open id of this session shows ▶, id, phase and 12m/M', () => {
  const s = stats({ running: [item({})] })
  const line = statusText(s, A, X)
  expect(line).toBe(`track ▶ · ${X} · plan · 12m/M (agent 12m) · Today: 0/0`)
})

test('FR-11.3: wait shows as wait <n>m; today shows done/started, PR, merged and hit', () => {
  const s = stats({
    running: [item({ wait: { ask: 2, permission: 1, idle: 4, total: 7 } })],
    today: { ...stats().today, started: { cook: 2, fix: 1 }, ended: { fix: 1 }, done: { fix: 1 }, prsOpened: 2, merged: 1, prsWaiting: 1 },
    estimate: { ...stats().estimate, byKind: { cook: { n: 4, hit: 2, low: 1, high: 0 }, fix: { n: 1, hit: 1, low: 0, high: 0 } } },
  })
  expect(statusText(s, A, X)).toBe(`track ▶ · ${X} · plan · 12m/M (agent 12m) · wait 7m · Today: 1/3 · PR 2 (1 merged) · hit 60%`)
})

test('FR-11.3: zero or unknown items are hidden, except Today', () => {
  const s = stats({ running: [item({ phase: '', agentMin: 0, wallMin: 0 })], today: { ...stats().today, prsOpened: 1, prsWaiting: 1 } })
  expect(statusText(s, A, X)).toBe(`track ▶ · ${X} · Today: 0/0 · PR 1`)
})

test('FR-11.3: an id of another session, or none bound, is ⏸', () => {
  const s = stats({ running: [item({ session: B })] })
  expect(statusText(s, A, X)).toBe('track ⏸ · Today: 0/0')
  expect(statusText(stats({ running: [item({})] }), A, undefined)).toBe('track ⏸ · Today: 0/0')
})

test('SC-43: not onboarded and no open id asks to onboard; an open id keeps the normal line', () => {
  const s = stats({ onboarded: false })
  expect(statusText(s, A, undefined)).toBe('track ⏸')
  expect(tipLine(s, A, undefined)).toBe(COPY.onboard)
  expect(statusText({ ...s, running: [item({})] }, A, X)).toBe(`track ▶ · ${X} · plan · 12m/M (agent 12m) · Today: 0/0`)
})

test('FR-11.4: an error is the ⚠ line with its message', () => {
  expect(errorStatus('exit 2')).toBe('⚠ track error: exit 2')
})

test('parseTrackStats takes the stats JSON and fills Go nulls; anything else throws', () => {
  const off = parseTrackStats(JSON.stringify({ ...stats({ enabled: false }), rules: null, running: null }))
  expect(off.enabled).toBe(false)
  expect(off.running).toEqual([])
  expect(() => parseTrackStats('not json')).toThrow()
  expect(() => parseTrackStats('{"x":1}')).toThrow('stats output is not track stats JSON')
})

test('FR-4.1: the first tab is Kitchen', () => {
  expect(TABS[0]).toEqual({ id: 'kitchen', label: 'Kitchen' })
})

// 03:05 local, built from parts so the expectation does not use the code's own formatter.
const COOKED_AT = new Date(2026, 10, 2, 3, 5)
const NOW = Date.UTC(2026, 10, 2, 9, 0, 0)
const kitchenStats = (over: Partial<TrackStats['kitchen']> = {}) =>
  stats({
    kitchen: {
      cooked: [
        { id: X, brief: 'Kitchen pane', status: 'done', endedAt: COOKED_AT.toISOString(), prs: ['https://github.com/o/r/pull/1', 'https://github.com/o/r/pull/2'], merged: true },
      ],
      rollingBoil: [{ session: B, tab: 'Dev track pane ship', brief: 'zenify-kit/x', state: 'permission', since: new Date(NOW - 125_000).toISOString() }],
      titleError: '',
      carried: [],
      cooking: [],
      lastDay: null,
      ...over,
    },
  })

test('SC-8: Kitchen lists Rolling boil then Cooked, identical for any session', () => {
  const s = kitchenStats()
  const mine = tabLines('kitchen', s, A, undefined, NOW)
  expect(tabLines('kitchen', s, B, 'cook-261030-0b0b', NOW)).toEqual(mine)
  expect(mine.slice(3, 11)).toEqual([
    { text: '🔥 Rolling boil', tone: 'bold' },
    { text: '- 🔐 2m Dev track pane ship' },
    { text: `    needs permission · zenify-kit/x · session ${B.slice(0, 8)}`, tone: 'dim' },
    { text: '' },
    { text: '🍳 Cooked', tone: 'bold' },
    { text: '- ✅ 03:05 Kitchen pane' },
    { text: '  🔗 ', href: 'https://github.com/o/r/pull/1', label: 'r#1' },
    { text: '  🔗 ', href: 'https://github.com/o/r/pull/2', label: 'r#2' },
  ])
  expect(mine).toHaveLength(11)
})

test('FR-4.2: an empty tab falls back to the brief, and parseTrackStats defaults tab to empty', () => {
  const s = kitchenStats({ rollingBoil: [{ session: B, tab: '', brief: 'b', state: 'ask', since: new Date(NOW - 45_000).toISOString() }] })
  const lines = tabLines('kitchen', s, A, undefined, NOW)
  expect(texts(lines)).toContain('- ❓ 45s b')
  expect(lines.find(l => l.text.startsWith('    asking · b'))).toEqual({ text: `    asking · b · session ${B.slice(0, 8)}`, tone: 'dim' })
  const raw = { ...stats(), kitchen: { carried: [], lastDay: null, cooked: [], rollingBoil: [{ session: B, brief: 'b', state: 'ask', since: 'x' }], titleError: '' } }
  expect(parseTrackStats(JSON.stringify(raw)).kitchen.rollingBoil?.[0]?.tab).toBe('')
})

const SPIN = '⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
const cookItem = (over: Partial<TrackStats['kitchen']['cooking'][number]> = {}) => ({
  session: B, tab: '', brief: 'zenify-kit/x', id: '', kind: '', phase: '', since: new Date(NOW - 12 * 60_000).toISOString(), ...over,
})
// The frame of line i at a given time: (floor(now / 1000) + i) % 10, spelled out here by hand.
const frameAt = (now: number, i: number) => SPIN[(Math.floor(now / 1000) + i) % 10]

test('SC-21: Cooking spinner frames advance per tick and differ per line', () => {
  const k = kitchenStats({ cooking: [cookItem({ tab: 'Tab one' }), cookItem({ session: A, tab: 'Tab two' })] })
  const at = (now: number) => texts(tabLines('kitchen', k, A, undefined, now)).filter(t => t.startsWith('- ') && /^- [⠋-⠿]/.test(t))
  const first = at(NOW)
  const second = at(NOW + 1000)
  expect(first).toEqual([`- ${frameAt(NOW, 0)} 12m Tab one`, `- ${frameAt(NOW, 1)} 12m Tab two`])
  expect(second).toEqual([`- ${frameAt(NOW + 1000, 0)} 12m Tab one`, `- ${frameAt(NOW + 1000, 1)} 12m Tab two`])
  expect(first[0]?.[2]).not.toBe(second[0]?.[2])
  expect(first[0]?.[2]).not.toBe(first[1]?.[2])
  // The ten frames wrap around.
  expect(at(NOW + 10_000)).toEqual(first)
})

test('FR-12.3: Cooking lines carry the time, tab or brief, kind and phase, then session and id', () => {
  const k = kitchenStats({
    cooking: [
      cookItem({ tab: 'Dev track pane ship', id: 'cook-261103-8d5d', kind: 'cook', phase: 'implement', since: new Date(NOW - 3 * 3600_000).toISOString() }),
      cookItem({ session: A }),
    ],
  })
  const lines = tabLines('kitchen', k, A, undefined, NOW)
  expect(lines[0]).toEqual({ text: '🥘 Cooking', tone: 'bold' })
  expect(lines[1]?.text).toBe(`- ${frameAt(NOW, 0)} 3h Dev track pane ship · cook · implement`)
  expect(lines[2]).toEqual({ text: `    ${B.slice(0, 8)} · cook-261103-8d5d`, tone: 'dim' })
  expect(lines[3]?.text).toBe(`- ${frameAt(NOW, 1)} 12m zenify-kit/x`)
  expect(lines[4]).toEqual({ text: `    ${A.slice(0, 8)}`, tone: 'dim' })
  expect(lines[5]).toEqual({ text: '' })
  expect(lines[6]).toEqual({ text: '🔥 Rolling boil', tone: 'bold' })
})

test('parseTrackStats defaults cooking to an empty list', () => {
  const { kitchen: _drop, ...rest } = stats()
  expect(parseTrackStats(JSON.stringify({ ...rest, kitchen: { cooked: [] } })).kitchen.cooking).toEqual([])
  expect(parseTrackStats(JSON.stringify({ ...rest, kitchen: { cooking: null } })).kitchen.cooking).toEqual([])
})

test('FR-4.2: every cooked status has its icon', () => {
  const at = COOKED_AT.toISOString()
  const cooked = (['done', 'pushed', 'cancelled', 'rollback'] as const).map(status => ({ id: X, brief: 'b', status, endedAt: at, prs: null, merged: true }))
  const lines = texts(tabLines('kitchen', kitchenStats({ cooked }), A, undefined, NOW))
  const cookedLines = lines.slice(lines.indexOf('🍳 Cooked'))
  expect(cookedLines.filter(l => l.startsWith('- ')).slice(0, 4).map(l => l.split(' ')[1])).toEqual(['✅', '🚀', '⛔', '↩️'])
})

test('FR-4.2: a done item shows ✅ only once its PRs are merged; until then ⏳', () => {
  const at = COOKED_AT.toISOString()
  const pr = ['https://github.com/o/r/pull/7']
  const cooked = [
    { id: X, brief: 'open', status: 'done' as const, endedAt: at, prs: pr, merged: false },
    { id: X, brief: 'old', status: 'done' as const, endedAt: at, prs: pr },
    { id: X, brief: 'in', status: 'done' as const, endedAt: at, prs: pr, merged: true },
  ]
  const lines = texts(tabLines('kitchen', kitchenStats({ cooked }), A, undefined, NOW))
  const cookedLines = lines.slice(lines.indexOf('🍳 Cooked')).filter(l => l.startsWith('- '))
  expect(cookedLines.map(l => l.split(' ')[1])).toEqual(['⏳', '⏳', '✅'])
})

test('FR-4.2: prLabel is <repo>#<n>, or the URL when it does not parse', () => {
  expect(prLabel('https://github.com/o/r/pull/1')).toBe('r#1')
  expect(prLabel('https://example.com/x')).toBe('https://example.com/x')
})

test('FR-4.3: empty Kitchen sections say so; ask reads asking', () => {
  const empty = tabLines('kitchen', stats(), A, undefined, NOW)
  expect(empty.slice(0, 8)).toEqual([
    { text: '🥘 Cooking', tone: 'bold' },
    { text: 'no tab is cooking', tone: 'dim' },
    { text: '' },
    { text: '🔥 Rolling boil', tone: 'bold' },
    { text: 'no tab is waiting', tone: 'dim' },
    { text: '' },
    { text: '🍳 Cooked', tone: 'bold' },
    { text: 'nothing finished today', tone: 'dim' },
  ])
  const ask = kitchenStats({ rollingBoil: [{ session: B, tab: '', brief: 'b', state: 'ask', since: new Date(NOW - 45_000).toISOString() }] })
  expect(texts(tabLines('kitchen', ask, A, undefined, NOW))).toContain('- ❓ 45s b')
})

test('FR-5.1: waiting time rounds down to s, m or h', () => {
  const wait = (ms: number) =>
    texts(tabLines('kitchen', kitchenStats({ rollingBoil: [{ session: B, tab: '', brief: 'b', state: 'ask', since: new Date(NOW - ms).toISOString() }] }), A, undefined, NOW))
  expect(wait(59_000)).toContain('- ❓ 59s b')
  expect(wait(60_000)).toContain('- ❓ 1m b')
  expect(wait(125 * 60_000)).toContain('- ❓ 2h b')
})

test('FR-4.2: a title error is a warn line', () => {
  const t = tabLines('kitchen', kitchenStats({ titleError: 'gh: boom' }), A, undefined, NOW)
  expect(t).toContainEqual({ text: '⚠ title error: gh: boom', tone: 'warn' })
})

test('parseTrackStats fills a missing or null kitchen', () => {
  const empty = { carried: [], lastDay: null, cooked: [], rollingBoil: [], cooking: [], titleError: '' }
  const { kitchen: _drop, ...rest } = stats()
  expect(parseTrackStats(JSON.stringify(rest)).kitchen).toEqual(empty)
  expect(parseTrackStats(JSON.stringify({ ...rest, kitchen: { carried: null, lastDay: null, cooked: null, rollingBoil: null, titleError: '' } })).kitchen).toEqual(empty)
})

test('FR-6.2: Estimate shows hit per kind, hotfix time and the agent share by phase', () => {
  const s = stats({
    estimate: {
      byKind: { cook: { n: 6, hit: 3, low: 1, high: 2 } },
      hotfix: { n: 5, medianMin: 15, p90Min: 40 },
      phaseShare: { cook: { plan: 0.25, implement: 0.75 } },
    },
  })
  const t = texts(tabLines('estimate', s, A, X, NOW))
  expect(t).toContain('🎯 Estimate · hit 50% (3/6)')
  expect(t).toContain('█ hit · ▓ longer · ░ shorter')
  expect(t).toContain(`cook   ${'█'.repeat(10)}${'▓'.repeat(3)}${'░'.repeat(7)} 50% ⬆1 ⬇2`)
  expect(t).toContain('hotfix median 15m · p90 40m')
  expect(t).toContain('🧭 Agent time by phase')
  expect(t).toContain(`cook   ${'█'.repeat(15)}${'▓'.repeat(5)}`)
  expect(t).toContain('   implement 75% · plan 25%')
})

test('FR-6.2: Quality shows blocked rate, findings, fix loop, wait share, tokens and the fix block; null is —', () => {
  const s = stats({
    quality: {
      byKind: {
        cook: {
          n: 5,
          shipBlockedRate: 0.4,
          topBlockedStep: 'review',
          findingsAvg: { C: 0, H: 1.5, M: 2, L: 0.25 },
          fixLoopAvg: 1.2,
          ciRedFirstRate: null,
          waitShare: 0.1,
          tokensAvg: { 'claude-opus-5-5': { in: 1000, out: 2000 } },
        },
      },
      fix: { n: 5, roundAvg: 1.5, wideRate: 0.5, regressionRate: null },
    },
  })
  const t = texts(tabLines('quality', s, A, X, NOW))
  expect(t).toContain('cook · 5 items')
  expect(t).toContain('  🟠 high     ████████░░ 1.5')
  expect(t).toContain('  🚧 blocked 40% @review')
  expect(t).toContain('  🔁 fix loop 1.2')
  expect(t).toContain('  ❌ no CI runs recorded')
  expect(t).toContain('  ⏳ wait     ██░░░░░░░░░░░░░░░░░░ 10%')
  expect(t).toContain('  🪙 out/item')
  expect(t).toContain('     opus 2000')
  expect(t).toContain('🩹 fix/hotfix · 5 items')
  expect(t).toContain('  wide 50% · rounds 1.5')
  expect(t.some(l => l.includes('regression test'))).toBe(false)
})

test('FR-10.7/SC-14: the scale footer and rules are on Estimate only', () => {
  const s = stats({ rules: ['cook: est up 1 size', 'fix: est down 1 size'] })
  for (const tab of ['kitchen', 'today', 'quality', 'backlog'] as const) {
    expect(texts(tabLines(tab, s, A, X, NOW)).some(t => t.startsWith('scale ('))).toBe(false)
  }
  const t = texts(tabLines('estimate', s, A, X, NOW))
  expect(t).toContain('scale (default, 0): S<15m M<45m L<2h')
  expect(t).toContain('cook: raise est 1 size')
  expect(t).toContain('fix: lower est 1 size')
  expect(ruleText('hotfix: something new')).toBe('hotfix: something new')
})

test('FR-6.2: Today warns on a merged lookup error and on malformed lines', () => {
  const s = stats({ malformed: 2, today: { ...stats().today, mergedError: 'gh: not logged in' } })
  const t = tabLines('today', s, A, X, NOW)
  expect(t.some(l => l.text === '⚠ merged check error: gh: not logged in' && l.tone === 'warn')).toBe(true)
  expect(t.some(l => l.text === '⚠ 2 malformed log lines' && l.tone === 'warn')).toBe(true)
})

test('FR-6.1: cook/fix/hotfix/ship prompts and skills, with or without znf:, open the pane; others do not', () => {
  expect(isTrackPrompt({ text: '/znf:fix x' })).toBe(true)
  expect(isTrackPrompt({ text: '/cook a feature' })).toBe(true)
  expect(isTrackPrompt({ text: 'go', command: { name: 'znf:ship' } })).toBe(true)
  expect(isTrackPrompt({ text: 'fix the bug' })).toBe(false)
  expect(isTrackPrompt({ text: '/fix-bug' })).toBe(false)
  expect(isTrackPrompt({ text: '/znf:scout x' })).toBe(false)
  expect(isTrackSkill('znf:hotfix')).toBe(true)
  expect(isTrackSkill('ship')).toBe(true)
  expect(isTrackSkill('znf:scout')).toBe(false)
})

test('FR-13.7: Backlog groups by source in manual, plane, github, sdd, stale order, one repo · est · title · age line each', () => {
  const s = stats({
    backlog: {
      ghError: '', planeError: '',
      items: [
        bl('stale', { repo: 'hub', est: 'L', title: 'cook hub (c1)', ageDays: 9 }),
        bl('sdd', { est: '-', title: 'spec A', ageDays: 1 }),
        bl('github', { repo: 'web', est: '-', title: 'issue B', ageDays: 0 }),
        bl('manual', { est: 'S', title: 'task C', ageDays: 5 }),
        bl('manual', { est: 'M', title: 'task D', ageDays: 2 }),
        bl('plane', { repo: 'ZEN', est: '-', title: 'plane E', ageDays: 3 }),
      ],
    },
  })
  expect(texts(tabLines('backlog', s, A, X, NOW)).slice(0, 11)).toEqual([
    '📝 manual',
    'kit · S · task C · 5d',
    'kit · M · task D · 2d',
    '✈️ plane',
    'ZEN · - · plane E · 3d',
    '🐙 github',
    'web · - · issue B · 0d',
    '📒 sdd',
    'kit · - · spec A · 1d',
    '🕸 stale',
    'hub · L · cook hub (c1) · 9d',
  ])
})

test('FR-5.2: a backlog item whose ref or id is an http URL is a Link line, others stay plain', () => {
  const url = 'https://github.com/o/r/issues/1'
  const s = stats({
    backlog: {
      ghError: '', planeError: '',
      items: [
        bl('github', { repo: 'r', est: 'S', title: 'issue one', ageDays: 4, ref: url }),
        bl('github', { repo: 'r', est: 'M', title: 'issue two', ageDays: 1, id: 'https://github.com/o/r/issues/2' }),
        bl('manual', { title: 'plain', ref: 'memory:x' }),
      ],
    },
  })
  const lines = tabLines('backlog', s, A, X, NOW)
  expect(lines).toContainEqual({ text: '', href: url, label: 'r · S · issue one · 4d' })
  expect(lines).toContainEqual({ text: '', href: 'https://github.com/o/r/issues/2', label: 'r · M · issue two · 1d' })
  expect(lines).toContainEqual({ text: 'kit · M · plain · 3d' })
})

test('FR-13.7: an empty backlog says Backlog is empty; a ghError adds a ⚠ github line', () => {
  expect(texts(tabLines('backlog', stats(), A, X, NOW))[0]).toBe('Backlog is empty')
  const t = tabLines('backlog', stats({ backlog: { items: [bl('manual')], ghError: 'gh: not logged in' } }), A, X, NOW)
  expect(t.some(l => l.text === '⚠ github: gh: not logged in' && l.tone === 'warn')).toBe(true)
  const p = tabLines('backlog', stats({ backlog: { items: [bl('manual')], ghError: '', planeError: 'x' } }), A, X, NOW)
  expect(p.some(l => l.text === '⚠ plane: x' && l.tone === 'warn')).toBe(true)
  expect(texts(t)).toContain('kit · M · manual title · 3d')
})

test('parseTrackStats fills a missing backlog so the tab never reads undefined', () => {
  const raw = { ...stats(), backlog: { items: null, ghError: '', planeError: '' } }
  expect(parseTrackStats(JSON.stringify(raw)).backlog.items).toEqual([])
})

test('SC-20: no status string carries a znf: prefix (Claude Code adds its own)', () => {
  const lines = [
    STATUS_OFF,
    errorStatus('x'),
    statusText(stats({ running: [item({})] }), A, X),
    statusText(stats({}), A, undefined),
  ]
  for (const l of lines) expect(l).not.toContain('znf: ')
  expect(errorStatus('x')).toBe('⚠ track error: x')
  expect(statusText(stats({}), A, undefined).startsWith('track ⏸')).toBe(true)
})

test('FR-11.2: a PR merged today shows on the status line even when none was opened today', () => {
  const s = stats({ today: { ...stats().today, prsOpened: 0, merged: 1 } })
  expect(statusText(s, A, undefined)).toContain('PR 0 (1 merged)')
})

test('FR-11.7 (5): the PR tip counts tracked PRs still waiting, not opened minus merged; it hides while this tab has an open item', () => {
  // One PR opened today still waits; one from yesterday merged today: the tip stays for an idle tab.
  const s = stats({ today: { ...stats().today, prsOpened: 1, merged: 1, prsWaiting: 1 } })
  expect(tipLine(s, A, undefined, true, NOW)).toBe('Tip: 1 PRs waiting to merge — type "hey claude check PR"')
  expect(tipLine({ ...s, running: [item({})] }, A, X, true, NOW)).toBeUndefined()
})

const VI = /[À-ỹđĐ]/
const carried = (over: Partial<TrackStats['kitchen']['carried'][number]>) => ({
  id: X, brief: 'Dev track pane ship', kind: 'cook', repo: 'zenify-kit', phase: 'implement', startedAt: new Date(NOW - 2 * 86_400_000).toISOString(),
  wallMin: 2880, stale: false, worktree: 'zenify-kit/track-kitchen', tab: 'Dev tab', session: A, ...over,
})
const noFix = { n: 0, roundAvg: null, wideRate: null, regressionRate: null }

test('SC-5: a kind under 5 samples says need 5 samples on Estimate and Quality; hit hides on the status line', () => {
  const s = stats({
    estimate: { byKind: { fix: { n: 3, hit: 2, low: 1, high: 0 } }, hotfix: { n: 0, medianMin: null, p90Min: null }, phaseShare: {} },
    quality: { byKind: { cook: { n: 2, shipBlockedRate: 0, topBlockedStep: null, findingsAvg: null, fixLoopAvg: null, ciRedFirstRate: null, waitShare: null, tokensAvg: null } }, fix: noFix },
  })
  expect(tabLines('estimate', s, A, X, NOW)).toContainEqual({ text: 'fix: need 5 samples (have 3)', tone: 'dim' })
  expect(texts(tabLines('quality', s, A, X, NOW))).toContain('cook: need 5 samples (have 2)')
  expect(statusText(s, A, undefined)).not.toContain('hit ')
  const five = stats({ estimate: { byKind: { fix: { n: 5, hit: 4, low: 1, high: 0 } }, hotfix: { n: 0, medianMin: null, p90Min: null }, phaseShare: {} } })
  expect(statusText(five, A, undefined)).toContain('hit 80%')
})

test('SC-10: no Vietnamese in any tab, the status line or COPY', () => {
  const s = stats({
    onboarded: false,
    rules: ['cook: est up 1 size'],
    malformed: 1,
    running: [item({}), item({ id: Y, session: B })],
    backlog: { items: [bl('manual'), bl('github'), bl('sdd'), bl('stale')], ghError: 'x' },
    today: { ...stats().today, done: { cook: 1 }, cancelled: { fix: 1 }, tasks: 5, plans: { kit: 1 }, mergedError: 'e' },
    estimate: { byKind: { cook: { n: 6, hit: 3, low: 1, high: 2 } }, hotfix: { n: 5, medianMin: 1, p90Min: 2 }, phaseShare: { cook: { plan: 1 } } },
    kitchen: { ...kitchenStats({ cooking: [cookItem({ id: X, kind: 'cook', phase: 'plan' })] }).kitchen, titleError: 't', carried: [carried({})], lastDay: { date: '2026-10-30', done: 3, cancelled: 1, rolledBack: 0, prsOpened: 2, leftOpen: 2 } },
  })
  for (const t of TABS) for (const l of tabLines(t.id, s, A, X, NOW)) expect(l.text).not.toMatch(VI)
  for (const st of [s, { ...s, onboarded: true }, { ...s, enabled: false }]) {
    expect(statusText(st, A, X)).not.toMatch(VI)
    expect(statusText(st, A, undefined)).not.toMatch(VI)
  }
  // FUN_TIPS is the one Vietnamese UI copy (user decision 2026-11-18, FR-4.8): it is not in COPY, so it is not scanned.
  for (const v of [...Object.values(COPY), STATUS_OFF, errorStatus('x'), ...TABS.map(t => t.label)]) expect(v).not.toMatch(VI)
})

test('SC-12: fmtNum', () => {
  expect(fmtNum(147430)).toBe('147.4k')
  expect(fmtNum(9999)).toBe('9999')
  expect(fmtNum(2_300_000)).toBe('2.3M')
})

test('SC-13: the open item shows wall/est with agent time, and the over-est tip compares wall', () => {
  const line = statusText(stats({ running: [item({ est: 'S', wallMin: 20, agentMin: 8 })] }), A, X)
  expect(line).toContain('20m/S (agent 8m)')
  expect(line).not.toContain('Tip:')
  expect(tipLine(stats({ running: [item({ est: 'S', wallMin: 20, agentMin: 8 })] }), A, X)).toBe('Tip: over est S (20m/15m) — split the work or re-estimate')
})

test('FR-11.4: carried items tip first, before Backlog', () => {
  const s = stats({ kitchen: { ...stats().kitchen, carried: [carried({}), carried({ id: Y })] }, backlog: { items: [bl('manual')], ghError: '', planeError: '' } })
  expect(tipLine(s, A, undefined, true, NOW)).toBe('Tip: 2 items carried over — type "hey claude continue"')
})

test('FR-7.3: a null CI rate with n >= 5 says no CI runs recorded', () => {
  const q = { n: 5, shipBlockedRate: 0.2, topBlockedStep: null, findingsAvg: null, fixLoopAvg: 1, ciRedFirstRate: null, waitShare: 0.1, tokensAvg: null }
  const s = stats({ quality: { byKind: { cook: q }, fix: noFix } })
  expect(texts(tabLines('quality', s, A, X, NOW)).some(l => l.includes('no CI runs recorded'))).toBe(true)
})

test('FR-9.1: titles carry their icon', () => {
  expect(texts(tabLines('estimate', stats(), A, X, NOW))[0]).toMatch(/^🎯 /)
  expect(texts(tabLines('quality', stats(), A, X, NOW))[0]).toMatch(/^🛡 /)
})

test('FR-2.5/10.6: phase share needs n >= 5 for its kind', () => {
  const est = (n: number) => stats({ estimate: { byKind: { cook: { n, hit: n, low: 0, high: 0 } }, hotfix: { n: 0, medianMin: null, p90Min: null }, phaseShare: { cook: { plan: 1 } } } })
  expect(texts(tabLines('estimate', est(3), A, X, NOW)).some(l => l.includes('plan 100%'))).toBe(false)
  const six = texts(tabLines('estimate', est(6), A, X, NOW))
  expect(six[six.indexOf('🧭 Agent time by phase') + 1]).toBe(`cook   ${'█'.repeat(20)}`)
  expect(six[six.indexOf('🧭 Agent time by phase') + 2]).toBe('   plan 100%')
})

test('FR-7.2: all-zero findings draw no severity lines', () => {
  const q = { n: 5, shipBlockedRate: 0, topBlockedStep: null, findingsAvg: { C: 0, H: 0, M: 0, L: 0 }, fixLoopAvg: 0, ciRedFirstRate: 0, waitShare: 0, tokensAvg: null }
  const t = texts(tabLines('quality', stats({ quality: { byKind: { cook: q }, fix: noFix } }), A, X, NOW))
  expect(t.some(l => /^ {2}(🔴|🟠|🟡|🔵)/.test(l))).toBe(false)
})

test('FR-11.3: Kitchen opens with Carried over: the last day, then each unfinished item in two lines', () => {
  const s = stats({
    kitchen: { ...stats().kitchen, carried: [carried({}), carried({ id: Y, stale: true, worktree: '', brief: 'b', tab: '', phase: '' })], lastDay: { date: '2026-10-30', done: 3, cancelled: 1, rolledBack: 0, prsOpened: 2, leftOpen: 2 } },
  })
  const t = texts(tabLines('kitchen', s, A, undefined, NOW))
  expect(t.slice(0, 9)).toEqual([
    '📌 Carried over',
    '📆 Fri 10/30: 3 done · 1 cancelled · 2 PRs · 2 left open',
    '- ⏳ 2d zenify-kit/track-kitchen · cook · implement',
    `    Dev tab · ${X}`,
    '- 🕸 2d b · cook',
    `    aaaaaaaa · ${Y}`,
    '',
    '🥘 Cooking',
    'no tab is cooking',
  ])
  expect(t.indexOf('🔥 Rolling boil')).toBe(10)
  expect(t.indexOf('🍳 Cooked')).toBeGreaterThan(t.indexOf('🔥 Rolling boil'))
})

test('FR-11.3: nothing carried and no last day means no section and no blank line', () => {
  const t = texts(tabLines('kitchen', stats(), A, undefined, NOW))
  expect(t[0]).toBe('🥘 Cooking')
  expect(t[3]).toBe('🔥 Rolling boil')
})

test('FR-7.3: null CI rate and null wait share still say no CI runs recorded; all-zero hides', () => {
  const q = (ci: number | null, wait: number | null) => ({ n: 6, shipBlockedRate: 0.2, topBlockedStep: null, findingsAvg: null, fixLoopAvg: 1, ciRedFirstRate: ci, waitShare: wait, tokensAvg: null })
  const lines = (ci: number | null, wait: number | null) =>
    texts(tabLines('quality', stats({ quality: { byKind: { cook: q(ci, wait) }, fix: noFix } }), A, X, NOW))
  expect(lines(null, null).some(l => l.includes('no CI runs recorded'))).toBe(true)
  expect(lines(null, 0).some(l => l.includes('no CI runs recorded'))).toBe(true)
  expect(lines(0, 0).some(l => l.includes('CI red first 0%'))).toBe(true)
  expect(lines(0, null).some(l => l.includes('⏳'))).toBe(false)
})

test('fmtNum rolls 999999 over to M', () => {
  expect(fmtNum(999_999)).toBe('1.0M')
  expect(fmtNum(999_949)).toBe('999.9k')
})

test('parseTrackStats defaults the fields an older binary lacks', () => {
  const old = {
    enabled: true, malformed: 0, scale: { source: 'default', n: 0, S: 15, M: 45, L: 120 }, rules: null,
    today: { date: '2026-10-30', plans: null, started: null, ended: null, tasks: 0, rounds: 0, prsOpened: 0, merged: 0, prsWaiting: 0, mergedError: '', items: null },
    running: [{ id: X, kind: 'cook', repo: 'kit', est: 'S', session: A, cwd: '/w', phase: 'plan', status: 'running', startedAt: '', endedAt: '', prs: null, agentMin: 3, wait: { ask: 0, permission: 0, idle: 0, total: 0 }, phases: null, tokens: null }],
    estimate: { byKind: null, hotfix: { n: 0, medianMin: null, p90Min: null }, phaseShare: null },
    quality: { byKind: null, fix: noFix }, backlog: { items: null, ghError: '', planeError: '' }, onboarded: true,
    kitchen: { cooked: null, rollingBoil: null, titleError: '' },
  }
  const s = parseTrackStats(JSON.stringify(old))
  expect(s.running?.[0]).toMatchObject({ wallMin: 0, tab: '' })
  expect(s.today).toMatchObject({ done: {}, cancelled: {}, rolledBack: {}, wallMin: 0, agentMin: 0 })
  expect(s.kitchen).toMatchObject({ carried: [], lastDay: null })
  expect(statusText(s, A, X)).not.toContain('undefined')
})

test('rollback reads the same on Today and Carried over', () => {
  const s = stats({
    today: { ...stats().today, rolledBack: { fix: 2 } },
    kitchen: { ...stats().kitchen, lastDay: { date: '2026-10-30', done: 0, cancelled: 0, rolledBack: 2, prsOpened: 0, leftOpen: 0 } },
  })
  expect(texts(tabLines('today', s, A, X, NOW))).toContain('↩️ Rollback 2')
  expect(texts(tabLines('kitchen', s, A, undefined, NOW))).toContain('📆 Fri 10/30: 2 rollback')
})

const noPlanStats = (over: Partial<TrackStats> = {}) => stats({ today: { ...stats().today, dailyPlan: null }, ...over } as Partial<TrackStats>)
const planItem = (status: string, est: string, title: string, code = '') => ({ id: `i-${title}`, code, repo: 'kit', est, title, status, url: '' })
const withPlan = (items: ReturnType<typeof planItem>[], estMin: number, wallMin: number) =>
  stats({ today: { ...stats().today, dailyPlan: { items, estMin, wallMin } } } as Partial<TrackStats>)

test('FR-1.2: Plan row leads with the short code; an empty code keeps the old row', () => {
  const s = withPlan([planItem('todo', 'S', 'a', 'b81e'), planItem('todo', 'S', 'b', '')], 30, 0)
  const t = texts(tabLines('kitchen', s, A, undefined, NOW))
  expect(t.slice(0, 3)).toEqual(['🗓 Plan', '- ☐ b81e · kit · S · a', '- ☐ kit · S · b'])
})

test('FR-1.2: Backlog row leads with the code', () => {
  const s = stats({ backlog: { ghError: '', planeError: '', items: [bl('github', { code: '#581', repo: 'chatting', est: '-', title: 't', ageDays: 0 })] } })
  expect(texts(tabLines('backlog', s, A, X, NOW))).toContain('#581 · chatting · - · t · 0d')
})

test('FR-1.2: a binary without code parses with code empty', () => {
  const s = stats({ backlog: { ghError: '', planeError: '', items: [{ ...bl('manual'), code: undefined } as unknown as BacklogItem] } })
  expect(parseTrackStats(JSON.stringify(s)).backlog.items?.[0].code).toBe('')
  const p = withPlan([{ ...planItem('todo', 'S', 'a'), code: undefined } as unknown as ReturnType<typeof planItem>], 1, 0)
  expect(parseTrackStats(JSON.stringify(p)).today.dailyPlan?.items[0].code).toBe('')
})

test('FR-2.4: Estimate shows the task block like hotfix, gated at 5 samples', () => {
  const est = (n: number) => stats({ estimate: { byKind: {}, hotfix: { n: 0, medianMin: null, p90Min: null }, task: { n, medianMin: 30, p90Min: 50 }, phaseShare: {} } })
  expect(texts(tabLines('estimate', est(5), A, X, NOW))).toContain('task 5 items')
  expect(texts(tabLines('estimate', est(5), A, X, NOW))).toContain('       median 30m · p90 50m')
  expect(texts(tabLines('estimate', est(3), A, X, NOW))).toContain('task: need 5 samples (have 3)')
})

test('FR-3.1: fmtDur renders minutes under an hour, else hours and minutes', () => {
  expect(fmtDur(50)).toBe('50m')
  expect(fmtDur(180)).toBe('3h')
  expect(fmtDur(270)).toBe('4h30m')
})

test('FR-3.1: Kitchen leads with the Plan block before Carried over; null plan hides it', () => {
  const s = withPlan([planItem('todo', 'S', 'a'), planItem('running', 'M', 'b'), planItem('done', 'L', 'c')], 180, 50)
  const t = texts(tabLines('kitchen', { ...s, kitchen: { ...s.kitchen, carried: [carried({})] } }, A, undefined, NOW))
  expect(t.slice(0, 5)).toEqual(['🗓 Plan', '- ☐ kit · S · a', '- ▶ kit · M · b', '- ✅ kit · L · c', 'est 3h · used 50m'])
  expect(t.indexOf('🗓 Plan')).toBeLessThan(t.indexOf('📌 Carried over'))
  expect(texts(tabLines('kitchen', noPlanStats(), A, undefined, NOW))).not.toContain('🗓 Plan')
})

test('SC-6: cooked est to wall shows, with a warning at or past the scale bound', () => {
  const line = (est: string, wallMin: number) => {
    const s = kitchenStats()
    const cooked = [{ ...s.kitchen.cooked[0], est, wallMin }]
    return texts(tabLines('kitchen', { ...s, kitchen: { ...s.kitchen, cooked } }, A, undefined, NOW)).find(l => l.includes('Kitchen pane')) as string
  }
  expect(line('M', 52)).toContain('M→52m ⚠')
  expect(line('M', 45)).toContain('⚠')
  expect(line('M', 30)).toContain('M→30m')
  expect(line('M', 30)).not.toContain('⚠')
  expect(line('-', 30)).not.toContain('→')
})

test('SC-7: Today lists open PRs with CI, review and threads; ready PRs say so', () => {
  const prOpen = [
    { url: 'https://github.com/o/r/pull/1', repo: 'r', number: 1, createdAt: new Date(NOW - 3 * 3600_000).toISOString(), ci: 'success', review: 'approved', threads: 0, ready: true },
    { url: 'https://github.com/o/r/pull/2', repo: 'r', number: 2, createdAt: new Date(NOW - 3 * 3600_000).toISOString(), ci: 'failure', review: 'changes_requested', threads: 2, ready: false },
  ]
  const s = stats({ today: { ...stats().today, prsOpened: 3, merged: 1, prOpen } } as Partial<TrackStats>)
  const lines = tabLines('today', s, A, undefined, NOW)
  expect(texts(lines)).toContain('🔗 PR 3 opened · 1 merged · 0 waiting')
  const row = (n: number) => lines.find(l => l.href?.endsWith(`/pull/${n}`))
  expect(`${row(1)?.text}${row(1)?.label}`).toBe('🔗 r#1 · 3h · ready to merge')
  expect(`${row(2)?.text}${row(2)?.label}`).toContain('CI ❌ · changes requested · 2 threads')
})

test('a PR with unknown status (no createdAt, CI and review "-") renders without NaN', () => {
  const prOpen = [{ url: 'https://github.com/o/r/pull/4', repo: 'r', number: 4, createdAt: '', ci: '-', review: '-', threads: 0, ready: false }]
  const s = stats({ today: { ...stats().today, prOpen } } as Partial<TrackStats>)
  const row = tabLines('today', s, A, undefined, NOW).find(l => l.href?.endsWith('/pull/4'))
  expect(`${row?.text}${row?.label}`).toBe('🔗 r#4 · — · CI - · waiting')
})

test('SC-8: a PR status error is a warning line', () => {
  const s = stats({ today: { ...stats().today, prStatusError: 'x' } } as Partial<TrackStats>)
  expect(texts(tabLines('today', s, A, undefined, NOW))).toContain('⚠ pr status: x')
})

test('SC-9: no-plan tip sits after carried and before backlog; tips=false hides it', () => {
  const noPlan = 'Tip: no plan today — type "/znf:track plan"'
  const tip = (s: TrackStats, tips = true) => tipLine(s, A, undefined, tips, NOW)
  expect(COPY.tipNoPlan).toBe(noPlan)
  expect(tip(noPlanStats())).toBe(noPlan)
  expect(tip(noPlanStats(), false)).toBeUndefined()
  expect(tip(noPlanStats({ kitchen: { ...noPlanStats().kitchen, carried: [carried({})] } }))).toContain('carried over')
  expect(tip(noPlanStats({ backlog: { items: [bl('manual')], ghError: '', planeError: '' } }))).toBe(noPlan)
  expect(tip(withPlan([], 0, 0))).not.toContain('no plan')
  expect(tipLine(stats({ running: [item({})] }), A, X, true, NOW)).toBeUndefined()
})

test('an old binary without dailyPlan, prOpen or est does not break the pane', () => {
  const old = JSON.parse(JSON.stringify(stats()))
  delete old.today.dailyPlan
  delete old.today.prOpen
  delete old.today.prStatusError
  const s = parseTrackStats(JSON.stringify(old))
  expect(s.today).toMatchObject({ dailyPlan: null, prOpen: [], prStatusError: '' })
  for (const t of TABS) tabLines(t.id, s, A, undefined, NOW)
  expect(texts(tabLines('kitchen', s, A, undefined, NOW))).not.toContain('🗓 Plan')
})

test('trackSectionOf names the track.md section of a tracked skill; promptSkillOf reads the prompt skill', () => {
  expect(trackSectionOf('znf:ci-watch')).toBe('ci-watch')
  expect(trackSectionOf('hotfix')).toBe('hotfix')
  expect(trackSectionOf('znf:scout')).toBeUndefined()
  expect(promptSkillOf({ text: '/znf:fix x' })).toBe('znf:fix')
  expect(promptSkillOf({ text: 'fix this', command: { name: 'ship' } })).toBe('ship')
  expect(promptSkillOf({ text: 'fix this' })).toBe('')
  const md = '## Rules\n\n- r\n\n## fix\n\n- f\n'
  expect(trackContext(md, 'fix')).toContain('## Rules\n\n- r\n\n## fix\n\n- f')
  expect(trackContext(md, 'ship')).toBeUndefined()
})

// FR-5.1/FR-5.2, SC-8: the tip points at the PR or item that needs action first.
const pr = (over: Partial<TrackStats['today']['prOpen'][number]>) => ({
  url: 'https://github.com/o/r/pull/12', repo: 'o/r', number: 12, createdAt: new Date(NOW).toISOString(), ci: 'success' as const, review: '-' as const, threads: 0, ready: false, ...over,
})
const withPrs = (prOpen: TrackStats['today']['prOpen'], extra: Partial<TrackStats> = {}) =>
  stats({ today: { ...stats().today, prOpen }, kitchen: { ...stats().kitchen, carried: [carried({})] }, ...extra })

test('SC-8: a PR with red CI is the first tip', () => {
  expect(tipLine(withPrs([pr({ ci: 'failure' })]), A, undefined, true, NOW)).toContain('Tip: PR #12 CI red — type "hey claude fix CI"')
})
test('SC-8: a PR with unresolved threads', () => {
  expect(tipLine(withPrs([pr({ threads: 2 })]), A, undefined, true, NOW)).toContain('Tip: PR #12 has 2 unresolved threads')
})
test('SC-8: two ready PRs beat carried items', () => {
  const s = withPrs([pr({ ready: true }), pr({ number: 13, ready: true })])
  expect(tipLine(s, A, undefined, true, NOW)).toContain('Tip: 2 PRs ready to merge')
})
test('FR-5.2: one ready PR is named', () => {
  expect(tipLine(withPrs([pr({ ready: true })]), A, undefined, true, NOW)).toContain('Tip: PR #12 ready to merge — type "hey claude check PR"')
})
test('SC-8: a carried item open 6 days is flagged', () => {
  const s = stats({ kitchen: { ...stats().kitchen, carried: [carried({ startedAt: new Date(NOW - 6 * 86_400_000).toISOString() })] } })
  expect(tipLine(s, A, undefined, true, NOW)).toContain(`Tip: ${X} open 6 days — continue or cancel it`)
})
test('SC-8: an open item in this tab hides the PR tips', () => {
  const s = withPrs([pr({ ci: 'failure' })], { running: [item({})] })
  expect(tipLine(s, A, X, true, NOW)).toBeUndefined()
})
test('SC-8: no PR and a 1-day carried item keeps the old tip', () => {
  const s = stats({ kitchen: { ...stats().kitchen, carried: [carried({ startedAt: new Date(NOW - 86_400_000).toISOString() })] } })
  expect(tipLine(s, A, undefined, true, NOW)).toContain('Tip: 1 items carried over')
})

// ---- Today charts (track-pane-charts Task 1) ----
const tl = (st: TrackStats, now = NOW) => tabLines('today', st, A, X, now)
const todayWith = (over: Partial<TrackStats['today']>) => stats({ today: { ...stats().today, ...over } })
const local = (h: number, m: number, d = 2) => new Date(2026, 10, d, h, m).toISOString()
const count = (s: string, ch: string) => [...s].filter(c => c === ch).length

test('bar fills round(frac*width) cells and clamps', () => {
  expect(bar(0.5, 20)).toBe('█'.repeat(10) + '░'.repeat(10))
  expect(bar(2, 4)).toBe('████')
  expect(bar(-1, 3)).toBe('░░░')
})

test('stackBar allocates exactly width cells by largest remainder', () => {
  const a = stackBar([6, 0, 13], 20, '█▓░')
  expect(a).toHaveLength(20)
  expect(count(a, '█')).toBe(6)
  expect(count(a, '░')).toBe(14)
  expect(stackBar([1, 1, 1], 20, '█▓░')).toHaveLength(20)
  expect(stackBar([0, 0], 5, '█▓')).toBe('░░░░░')
})

test('sparkline scales to the max and leaves zero as a space', () => {
  expect(sparkline([40, 0, 30])).toBe('█ ▆')
})

test('modelName keeps the family only for claude ids', () => {
  expect(modelName('claude-opus-5-5')).toBe('opus')
  expect(modelName('gpt-x')).toBe('gpt-x')
})

test('fmtDur shows days from 1440 minutes', () => {
  expect(fmtDur(2665)).toBe('1d20h')
  expect(fmtDur(1440)).toBe('1d')
  expect(fmtDur(50)).toBe('50m')
  expect(fmtDur(180)).toBe('3h')
  expect(fmtDur(270)).toBe('4h30m')
})

test('SC-1: Today shows done/started with a bar and per-kind rows, no Running or Plans written', () => {
  const s = todayWith({ started: { cook: 5, fix: 1 }, done: { cook: 3, fix: 1 }, plans: { zenie: 1 } })
  s.running = [item({})]
  const t = texts(tl(s))
  const total = t.find(l => l.startsWith('✅ Done 4/6'))
  expect(total).toContain('67%')
  expect(total).toContain(bar(4 / 6, 20))
  expect(t.find(l => l.trim().startsWith('cook'))).toContain('3/5')
  expect(t.find(l => l.trim().startsWith('fix'))).toContain('1/1')
  expect(t.some(l => l.includes('Plans written') || l.includes('Running'))).toBe(false)
  expect(t[0]).toBe('📅 Today · Fri 10/30')
  expect(texts(tl(stats()))).toContain('✅ Done 0/0')
})

test('Today shows cancelled and rollback only when > 0', () => {
  expect(texts(tl(todayWith({ cancelled: { fix: 2 }, rolledBack: { fix: 1 } })))).toContain('⛔ Cancelled 2 · ↩️ Rollback 1')
  expect(texts(tl(stats())).some(l => l.includes('Cancelled') || l.includes('Rollback'))).toBe(false)
})

test('SC-2: one warn line counts running items over size or open over a day', () => {
  const s = stats({ running: [item({ id: X, est: 'S', wallMin: 15 }), item({ id: Y, est: '-', wallMin: 5, startedAt: new Date(NOW - 30 * 3600_000).toISOString() })] })
  const w = tl(s).filter(l => l.text.includes('running over size'))
  expect(w).toEqual([{ text: '⚠ 2 running over size or >1d → Kitchen', tone: 'warn' }])
  expect(tl(stats({ running: [item({ wallMin: 1, startedAt: new Date(NOW - 3600_000).toISOString() })] })).some(l => l.text.includes('running over'))).toBe(false)
})

test('SC-3: hourBins sums overlapping minutes per local hour and the sparkline renders', () => {
  const now = new Date(2026, 10, 2, 10, 30).getTime()
  const a = item({ id: X, startedAt: local(8, 10), endedAt: local(8, 50), status: 'done' })
  const b = item({ id: Y, startedAt: local(10, 0), endedAt: '' })
  expect(hourBins([a, b], now)).toEqual({ startHour: 8, values: [40, 0, 30] })
  expect(hourBins([], now)).toEqual({ startHour: 0, values: [] })
  const t = texts(tl(todayWith({ items: [a, b] }), now))
  expect(t).toContain('⏱ Time by hour')
  expect(t).toContain('   08 █ ▆ 10')
})

test('SC-3b: an item from yesterday counts only from midnight', () => {
  const now = new Date(2026, 10, 2, 2, 0).getTime()
  const a = item({ startedAt: local(23, 0, 1), endedAt: local(0, 30), status: 'done' })
  const r = hourBins([a], now)
  expect(r.startHour).toBe(0)
  expect(r.values[0]).toBe(30)
})

test('Today shows wall and agent time when > 0', () => {
  expect(texts(tl(todayWith({ wallMin: 2665, agentMin: 90 })))).toContain('   wall 1d20h · agent 1h30m (parallel)')
})

test('SC-4: Today tokens sort by out with bars relative to the max', () => {
  const tok = (out: number, cacheRead = 0) => ({ in: 1, out, cacheRead, cacheCreation: 0 })
  const s = todayWith({ items: [item({ tokens: { 'claude-sonnet-5': tok(50000) } }), item({ tokens: { 'claude-opus-5-5': tok(100000, 2000) } })] })
  const t = texts(tl(s))
  const o = t.findIndex(l => l.includes('opus'))
  const so = t.findIndex(l => l.includes('sonnet'))
  expect(t).toContain("🪙 Tokens · today's items")
  expect(o).toBeGreaterThan(-1)
  expect(o).toBeLessThan(so)
  expect(t[o]).toBe('   opus   ████████████████████ 100.0k')
  expect(t[o + 1]).toBe('          cache 2000')
  expect(count(t[so] ?? '', '█')).toBe(10)
  expect(count(t[so] ?? '', '░')).toBe(10)
})

test('FR-1.7/1.8: SDD and PR summary lines', () => {
  const t = texts(tl(todayWith({ tasks: 3, rounds: 2, done: { cook: 1 }, prsOpened: 2, merged: 1, prsWaiting: 1 })))
  expect(t).toContain('🔁 SDD tasks 3 · rounds 2')
  expect(t).toContain('🔗 PR 2 opened · 1 merged · 1 waiting')
  const none = texts(tl(stats()))
  expect(none.some(l => l.startsWith('🔁') || l.startsWith('🔗 PR'))).toBe(false)
})

test('SC-5: Waiting on you shows the total, a 20-cell stacked bar and hides zero parts', () => {
  const s = todayWith({ items: [item({ wait: { ask: 0, permission: 6, idle: 13, total: 19 } })] })
  const t = texts(tl(s))
  const i = t.indexOf('🙋 Waiting on you 19m')
  expect(i).toBeGreaterThan(-1)
  expect(t[i + 1]?.trim()).toHaveLength(20)
  expect(t[i + 2]).not.toContain('ask')
  expect(t[i + 2]).toBe('   🔐 permission 6m · 💤 idle 13m')
})

const noQ = { n: 5, shipBlockedRate: null, topBlockedStep: null, findingsAvg: null, fixLoopAvg: null, ciRedFirstRate: null, waitShare: null, tokensAvg: null }
const estOf = (byKind: Record<string, { n: number; hit: number; low: number; high: number }>, hotfix = { n: 0, medianMin: null, p90Min: null } as { n: number; medianMin: number | null; p90Min: number | null }, phaseShare: Record<string, Record<string, number>> = {}) =>
  stats({ estimate: { byKind, hotfix, phaseShare } })

test('SC-6: hotfix is a hit row with median and p90 on the next line', () => {
  const t = texts(tabLines('estimate', estOf({ hotfix: { n: 10, hit: 9, low: 1, high: 0 } }, { n: 7, medianMin: 11, p90Min: 59 }), A, X, NOW))
  const i = t.findIndex(l => l.startsWith('hotfix'))
  expect(t.filter(l => l.startsWith('hotfix'))).toHaveLength(1)
  expect(t[i]).toContain('90%')
  expect(t[i + 1]).toBe('       median 11m · p90 59m')
})

test('FR-2.4: hotfix hit and hotfix time gate on their own sample', () => {
  const a = texts(tabLines('estimate', estOf({ hotfix: { n: 10, hit: 9, low: 1, high: 0 } }, { n: 3, medianMin: 11, p90Min: 59 }), A, X, NOW)).filter(l => l.startsWith('hotfix'))
  expect(a).toHaveLength(1)
  expect(a[0]).toContain('90%')
  expect(a[0]).not.toContain('median')
  const b = texts(tabLines('estimate', estOf({ hotfix: { n: 2, hit: 1, low: 1, high: 0 } }, { n: 3, medianMin: 11, p90Min: 59 }), A, X, NOW))
  expect(b).toContain('hotfix: need 5 samples (have 3)')
})

test('SC-7: the title sums hit over cook, fix and hotfix', () => {
  const t = texts(tabLines('estimate', estOf({ cook: { n: 25, hit: 13, low: 2, high: 10 }, fix: { n: 24, hit: 20, low: 2, high: 2 }, hotfix: { n: 10, hit: 9, low: 1, high: 0 } }), A, X, NOW))
  expect(t[0]).toBe('🎯 Estimate · hit 71% (42/59)')
})

test('SC-9: phase line lists the top three, descending', () => {
  const s = estOf({ cook: { n: 5, hit: 3, low: 1, high: 1 } }, undefined, { cook: { design: 0.49, ship: 0.29, implement: 0.18, plan: 0.04 } })
  const row = texts(tabLines('estimate', s, A, X, NOW)).find(l => l.includes('design'))
  expect(row).toBe('   design 49% · ship 29% · implement 18%')
})

test('FR-2.1: under 5 samples in total the title has no hit', () => {
  expect(texts(tabLines('estimate', estOf({ cook: { n: 3, hit: 1, low: 1, high: 1 } }), A, X, NOW))[0]).toBe('🎯 Estimate')
})

test('SC-8: findings are one bar line per severity, scaled to the largest', () => {
  const q = { ...noQ, findingsAvg: { C: 0.037, H: 0.48, M: 2.2, L: 3.9 } }
  const t = texts(tabLines('quality', stats({ quality: { byKind: { cook: q }, fix: noFix } }), A, X, NOW))
  for (const p of ['  🔴 critical', '  🟠 high', '  🟡 medium', '  🔵 low']) expect(t.filter(l => l.startsWith(p))).toHaveLength(1)
  expect(t.find(l => l.startsWith('  🔵 low'))).toContain('██████████ 3.9')
  expect(t.find(l => l.startsWith('  🔴 critical'))).toContain('0.04')
  expect(t.some(l => l.includes('most blocked') || l.includes('—'))).toBe(false)
})

test('SC-4: Quality shows out tokens per item by model', () => {
  const q = { ...noQ, tokensAvg: { 'claude-opus-5-5': { in: 1, out: 72290.8 } } }
  expect(texts(tabLines('quality', stats({ quality: { byKind: { cook: q }, fix: noFix } }), A, X, NOW))).toContain('     opus 72.3k')
})

test('FR-3.3/3.4/3.7: blocked, no CI runs, wait share; empty says no work yet', () => {
  const q = { ...noQ, shipBlockedRate: 0, waitShare: 0.61 }
  const t = texts(tabLines('quality', stats({ quality: { byKind: { cook: q }, fix: noFix } }), A, X, NOW))
  expect(t).toContain('  🚧 blocked 0% · ❌ no CI runs recorded')
  expect(t.find(l => l.startsWith('  ⏳ wait'))).toContain('61%')
  expect(texts(tabLines('quality', stats(), A, X, NOW))).toContain('no work yet')
})

test('FR-3 null: a fix block of nulls prints no dash on any tab', () => {
  const s = stats({ quality: { byKind: { cook: noQ }, fix: { n: 5, roundAvg: null, wideRate: null, regressionRate: null, fixLoopAvg: null } as never } })
  for (const t of TABS) expect(texts(tabLines(t.id, s, A, X, NOW)).some(l => l.includes('—'))).toBe(false)
})

// ---- Context tips (track-pane-charts Task 4, FR-4, SC-11..SC-17) ----
const boil = (over: Partial<NonNullable<TrackStats['kitchen']['rollingBoil']>[number]> = {}) => ({
  session: B, tab: 'Other tab', brief: 'brief', state: 'permission' as const, since: new Date(NOW - 3 * 60_000).toISOString(), ...over,
})

test('SC-11: statusText carries no tip; the PR tip moves to tipLine', () => {
  const s = stats({ today: { ...stats().today, prsWaiting: 2 } })
  expect(statusText(s, A, undefined)).not.toContain('Tip:')
  expect(tipLine(s, A, undefined, true, NOW)).toBe('Tip: 2 PRs waiting to merge — type "hey claude check PR"')
})

test('SC-13: another tab waiting on you beats the PR tip; this own session is ignored', () => {
  const prs = { ...stats().today, prsWaiting: 2 }
  const other = stats({ today: prs, kitchen: { ...stats().kitchen, rollingBoil: [boil()] } } as Partial<TrackStats>)
  expect(tipLine(other, A, undefined, true, NOW)).toBe('Tip: "Other tab" waits on your permission · 3m')
  const own = stats({ today: prs, kitchen: { ...stats().kitchen, rollingBoil: [boil({ session: A })] } } as Partial<TrackStats>)
  expect(tipLine(own, A, undefined, true, NOW)).toBe('Tip: 2 PRs waiting to merge — type "hey claude check PR"')
  // the longest waiter wins, an ask says answer, an empty tab name falls back to the brief
  const two = stats({ kitchen: { ...stats().kitchen, rollingBoil: [boil({ tab: '', brief: 'late', state: 'ask', since: new Date(NOW - 10 * 60_000).toISOString() }), boil()] } } as Partial<TrackStats>)
  expect(tipLine(two, A, X, true, NOW)).toBe('Tip: "late" waits on your answer · 10m')
})

test('SC-14: an open item with an est offers the pane; no tip once it has been shown', () => {
  const s = stats({ running: [item({ est: 'M', wallMin: 10 })], today: { ...stats().today, prsWaiting: 2 } })
  expect(tipLine(s, A, X, true, NOW, true)).toBe('Tip: see today\'s work in the track pane — type "/track-pane"')
  expect(tipLine(s, A, X, true, NOW, false)).toBeUndefined()
})

test('SC-15: the next todo of today\'s plan, then plan done', () => {
  const plan = withPlan([planItem('done', 'S', 'a', 'a1a1'), planItem('todo', 'S', 'fix x', 'b1c2')], 30, 0)
  expect(tipLine(plan, A, undefined, true, NOW)).toBe('Tip: next in plan: b1c2 fix x — type "/znf:track go b1c2"')
  // an item without a code falls back to its id
  expect(tipLine(withPlan([planItem('todo', 'S', 'y')], 15, 0), A, undefined, true, NOW)).toBe('Tip: next in plan: i-y y — type "/znf:track go i-y"')
  const done = withPlan([planItem('done', 'S', 'a'), planItem('done', 'S', 'b')], 30, 20)
  expect(tipLine(done, A, undefined, true, NOW)).toBe('Tip: plan done — type "/znf:track ls"')
  // a running item means the plan is not done yet: no plan tip, it falls through to the fun line
  expect(FUN_TIPS).toContain(tipLine(withPlan([planItem('done', 'S', 'a'), planItem('running', 'S', 'b')], 30, 0), A, undefined, true, NOW) ?? '')
})

test('SC-16: a newer kit release is a tip for an idle tab, after the PR tips', () => {
  const kitUpdate = { current: 'v0.73.0', latest: 'v0.74.0', newer: true }
  expect(tipLine(stats({ kitUpdate }), A, undefined, true, NOW)).toBe('Tip: zenify v0.74.0 is out (running v0.73.0) — run "zenify update"')
  expect(tipLine(stats({ kitUpdate: { ...kitUpdate, newer: false } }), A, undefined, true, NOW)).not.toContain('zenify v0.74.0')
  expect(tipLine(withPrs([pr({ ci: 'failure' })], { kitUpdate }), A, undefined, true, NOW)).toContain('CI red')
  expect(tipLine(stats({ kitUpdate, running: [item({})] }), A, X, true, NOW)).toBeUndefined()
})

test('SC-17: an idle tab with nothing else shows a fun line that changes every 10 minutes', () => {
  const idle = stats()
  const k = 7
  expect(tipLine(idle, A, undefined, true, k * 600_000, false)).toBe(FUN_TIPS[k % FUN_TIPS.length])
  expect(tipLine(idle, A, undefined, true, (k + 1) * 600_000 + 599_999, false)).toBe(FUN_TIPS[(k + 1) % FUN_TIPS.length])
  // the pane invite comes first while the pane has not been shown
  expect(tipLine(idle, A, undefined, true, k * 600_000, true)).toBe('Tip: see today\'s work in the track pane — type "/track-pane"')
  // an open item or tips=false: no fun line
  expect(tipLine(stats({ running: [item({})] }), A, X, true, k * 600_000, false)).toBeUndefined()
  expect(tipLine(idle, A, undefined, false, k * 600_000, false)).toBeUndefined()
  expect(FUN_TIPS.length).toBe(20)
  expect(new Set(FUN_TIPS).size).toBe(FUN_TIPS.length)
  for (const f of FUN_TIPS) expect(f).toMatch(/^\p{Extended_Pictographic}/u)
})

test('ship review: a title with $ patterns renders literally in the plan tip', () => {
  const st = withPlan([planItem('todo', 'S', "cost $$ and $& and $'", 'c0de')], 15, 0)
  expect(tipLine(st, A, undefined, true, NOW)).toBe(`Tip: next in plan: c0de cost $$ and $& and $' — type "/znf:track go c0de"`)
})

test('ship review: Done counts items ended today even when none started today, capped at 100%', () => {
  const none = stats({ today: { ...stats().today, started: {}, done: { cook: 2 } } } as Partial<TrackStats>)
  expect(texts(tl(none))).toContain('✅ Done 2/0')
  const more = stats({ today: { ...stats().today, started: { cook: 1 }, done: { cook: 2 } } } as Partial<TrackStats>)
  expect(texts(tl(more)).find(l => l.startsWith('✅ Done 2/1'))).toContain('100%')
})

test('ship review: an older binary without prsWaiting or cacheRead renders no undefined or NaN', () => {
  const old = item({ tokens: { 'claude-opus-5-5': { in: 1, out: 500 } } as unknown as TrackItem['tokens'] })
  const st = stats({ today: { ...stats().today, prsOpened: 2, prsWaiting: undefined, items: [old] } } as unknown as Partial<TrackStats>)
  const lines = texts(tl(st))
  expect(lines).toContain('🔗 PR 2 opened · 0 merged · 0 waiting')
  for (const l of lines) expect(l).not.toMatch(/undefined|NaN/)
})

// ---- Zenie review r1 (PR #379) ----
// Display width of a line in terminal cells. Rule: emoji and pictographs (U+1F000 and up, U+2600-U+27BF,
// and the emoji-presentation hourglass/clock block U+23E9-U+23F3) count 2; U+FE0F and U+200D count 0;
// everything else, including the block and arrow glyphs used by the charts, counts 1.
const cells = (text: string): number =>
  [...text].reduce((n, ch) => {
    const c = ch.codePointAt(0) ?? 0
    if (c === 0xfe0f || c === 0x200d) return n
    return n + (c >= 0x1f000 || (c >= 0x2600 && c <= 0x27bf) || (c >= 0x23e9 && c <= 0x23f3) ? 2 : 1)
  }, 0)
const PANE_BODY = 40

test('cells counts emoji as 2, the variation selector as 0 and chart glyphs as 1', () => {
  expect(cells('✅ ok')).toBe(5)
  expect(cells('↩️')).toBe(1)
  expect(cells('█▓▒░▁⬆⬇…')).toBe(8)
  expect(cells('🔐')).toBe(2)
})

test('Zenie F1: every Today, Estimate and Quality line fits the 40-cell pane body', () => {
  const tok = (out: number, cacheRead: number) => ({ in: 1, out, cacheRead, cacheCreation: 0 })
  const now = new Date(2026, 10, 2, 17, 30).getTime()
  const running = item({ id: X, est: 'S', wallMin: 30, startedAt: local(9, 0, 1) })
  const s = stats({
    running: [running],
    rules: ['cook: est up 1 size'],
    scale: { source: 'default', n: 120, S: 15, M: 45, L: 120 },
    today: {
      ...stats().today,
      started: { cook: 12, fix: 10, hotfix: 5, task: 3 },
      done: { cook: 10, fix: 9, hotfix: 5, task: 3 },
      cancelled: { fix: 1 },
      rolledBack: { fix: 1 },
      wallMin: 2665,
      agentMin: 90,
      tasks: 7,
      rounds: 12,
      prsOpened: 12,
      merged: 10,
      prsWaiting: 3,
      items: [
        item({ id: Y, startedAt: local(8, 10), endedAt: local(15, 50), status: 'done', wait: { ask: 12, permission: 9, idle: 26, total: 47 }, tokens: { 'claude-opus-5-5': tok(1_234_567, 98_765_432), 'claude-sonnet-5': tok(50000, 2000) } }),
      ],
    },
    estimate: {
      byKind: { cook: { n: 25, hit: 13, low: 2, high: 10 }, fix: { n: 24, hit: 20, low: 2, high: 2 }, hotfix: { n: 10, hit: 9, low: 1, high: 0 } },
      hotfix: { n: 7, medianMin: 665, p90Min: 2665 },
      task: { n: 120, medianMin: 90, p90Min: 165 },
      phaseShare: {
        cook: { design: 0.3, implement: 0.25, ship: 0.2, plan: 0.15, review: 0.1 },
        fix: { implement: 0.4, ship: 0.3, review: 0.2, plan: 0.07, design: 0.03 },
        hotfix: { implement: 0.4, ship: 0.3, review: 0.2, plan: 0.07, design: 0.03 },
      },
    },
    quality: {
      byKind: {
        cook: { n: 25, shipBlockedRate: 1, topBlockedStep: 'behaviour', findingsAvg: { C: 0.04, H: 0.5, M: 2.2, L: 3.9 }, fixLoopAvg: 12.5, ciRedFirstRate: 1, waitShare: 1, tokensAvg: { 'claude-opus-5-5': { in: 1, out: 76543 }, 'claude-sonnet-5': { in: 1, out: 12345 }, 'claude-haiku-5': { in: 1, out: 999 } } },
      },
      fix: { n: 20, roundAvg: 1.5, wideRate: 0.5, regressionRate: 1 },
    },
  })
  const tooLong: string[] = []
  for (const tab of ['today', 'estimate', 'quality'] as const) {
    for (const l of tabLines(tab, s, A, X, now)) {
      if (l.href) continue // PR links keep their pre-existing label
      const text = l.text + (l.label ?? '')
      if (cells(text) > PANE_BODY) tooLong.push(`${tab} (${cells(text)}): ${text}`)
    }
  }
  expect(tooLong).toEqual([])
})

test('Zenie F2: the tokens header says the numbers are for today\'s items', () => {
  const s = todayWith({ items: [item({})] })
  expect(texts(tl(s))).toContain("🪙 Tokens · today's items")
  expect(texts(tl(s)).some(l => l === '🪙 Tokens today')).toBe(false)
})

test('Zenie F3: a running item from yesterday shows in the sparkline once, even with no items today', () => {
  const now = new Date(2026, 10, 2, 2, 30).getTime()
  const r = item({ id: X, startedAt: local(23, 0, 1), endedAt: '' })
  const t = texts(tl(stats({ running: [r] }), now))
  expect(t).toContain('   00 ██▄ 02')
  // the same id in items and running counts once
  const both = texts(tl(stats({ running: [r], today: { ...stats().today, items: [r] } }), now))
  expect(both).toContain('   00 ██▄ 02')
  // a running item left open for more than a day is not counted as work
  const left = item({ id: X, startedAt: new Date(2026, 9, 30, 23, 0).toISOString(), endedAt: '' })
  expect(texts(tl(stats({ running: [left] }), now))).not.toContain('⏱ Time by hour')
})

test('Zenie F4: a span under a minute still draws a sparkline cell', () => {
  const now = new Date(2026, 10, 2, 8, 30).getTime()
  const a = item({ startedAt: new Date(2026, 10, 2, 8, 10, 0).toISOString(), endedAt: new Date(2026, 10, 2, 8, 10, 20).toISOString(), status: 'done' })
  expect(hourBins([a], now).values[0]).toBeGreaterThan(0)
  expect(sparkline(hourBins([a], now).values)[0]).not.toBe(' ')
})
