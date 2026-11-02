import { expect, test } from 'claude-code/testing'
import type { BacklogItem, TrackItem, TrackStats } from '../types'
import { STATUS_OFF, errorStatus, isTrackPrompt, isTrackSkill, parseTrackStats, ruleText, statusText, tabLines } from './track-ui'

const A = 'aaaaaaaa-1111-2222-3333-444444444444'
const B = 'bbbbbbbb-5555-6666-7777-888888888888'
const X = 'cook-261030-0a0a'
const Y = 'fix-261030-0b0b'

const item = (over: Partial<TrackItem>): TrackItem => ({
  id: X,
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
  wait: { ask: 0, permission: 0, idle: 0, total: 0 },
  phases: { plan: { agentMin: 12, waitMin: 0 } },
  tokens: { 'claude-opus-5-5': { in: 100, out: 200, cacheRead: 0, cacheCreation: 0 } },
  ...over,
})

const stats = (over: Partial<TrackStats> = {}): TrackStats => ({
  enabled: true,
  malformed: 0,
  scale: { source: 'default', n: 0, S: 30, M: 90, L: 240 },
  rules: [],
  today: { date: '2026-10-30', plans: {}, started: {}, ended: {}, tasks: 0, rounds: 0, prsOpened: 0, merged: 0, mergedError: '', items: [] },
  running: [],
  estimate: { byKind: {}, hotfix: { n: 0, medianMin: null, p90Min: null }, phaseShare: {} },
  quality: { byKind: {}, fix: { n: 0, roundAvg: null, wideRate: null, regressionRate: null } },
  backlog: { items: [], ghError: '' },
  onboarded: true,
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
  ...over,
})

const texts = (lines: { text: string }[]) => lines.map(l => l.text)

test('SC-44: contextual tips by priority, one at most', () => {
  const tail = (s: TrackStats, id?: string) => statusText(s, A, id)
  const backlog = { items: [bl('manual'), bl('github')], ghError: '' }
  const prs = { ...stats().today, prsOpened: 3, merged: 1 }
  // (2) no open id, backlog N > 0
  expect(tail(stats({ backlog }))).toBe('track ⏸ · Hôm nay: 0/0 · Tip: còn 2 việc trong Backlog — gõ "hey claude lấy việc tiếp"')
  // (3) open id, est not a size, agentMin >= 5
  const noEst = stats({ running: [item({ est: '-', agentMin: 5 })] })
  expect(tail(noEst, X).endsWith(' · Tip: chưa có estimate — gõ "hey claude est việc này"')).toBe(true)
  // agentMin 4 and no est: no tip (3)
  expect(tail(stats({ running: [item({ est: '-', agentMin: 4 })] }), X)).not.toContain('Tip:')
  // (4) over est: M threshold is scale.M = 90
  const over = stats({ running: [item({ est: 'M', agentMin: 95 })] })
  expect(tail(over, X).endsWith(' · Tip: đã vượt est M (95m/90m) — cân nhắc tách việc hoặc est lại')).toBe(true)
  expect(tail(stats({ running: [item({ est: 'M', agentMin: 89 })] }), X)).not.toContain('Tip:')
  // (5) PRs waiting
  expect(tail(stats({ today: prs }))).toBe('track ⏸ · Hôm nay: 0/0 · PR 3 (1 merged) · Tip: 2 PR chờ merge — gõ "hey claude check PR"')
  // (2) beats (5)
  expect(tail(stats({ backlog, today: prs }))).toContain('Tip: còn 2 việc trong Backlog')
  expect(tail(stats({ backlog, today: prs }))).not.toContain('chờ merge')
  // (4) beats (5) with an open id
  expect(tail(stats({ today: prs, running: [item({ est: 'S', agentMin: 30 })] }), X)).toContain('đã vượt est S (30m/30m)')
  // none
  expect(tail(stats())).not.toContain('Tip:')
})

test('FR-11.2/SC-20: enabled false is the off line, verbatim', () => {
  expect(STATUS_OFF).toBe('⏸ track off · bật: /config → znf')
  expect(statusText(stats({ enabled: false }), A, X)).toBe(STATUS_OFF)
})

test('SC-21: the open id of this session shows ▶, id, phase and 12m/M', () => {
  const s = stats({ running: [item({})] })
  const line = statusText(s, A, X)
  expect(line).toBe(`track ▶ · ${X} · plan · 12m/M · Hôm nay: 0/0`)
})

test('FR-11.3: wait shows as chờ <n>m; today shows ended/started, PR, merged and hit', () => {
  const s = stats({
    running: [item({ wait: { ask: 2, permission: 1, idle: 4, total: 7 } })],
    today: { ...stats().today, started: { cook: 2, fix: 1 }, ended: { fix: 1 }, prsOpened: 2, merged: 1 },
    estimate: { ...stats().estimate, byKind: { cook: { n: 3, hit: 2, low: 1, high: 0 }, fix: { n: 1, hit: 1, low: 0, high: 0 } } },
  })
  expect(statusText(s, A, X)).toBe(`track ▶ · ${X} · plan · 12m/M · chờ 7m · Hôm nay: 1/3 · PR 2 (1 merged) · hit 75% · Tip: 1 PR chờ merge — gõ "hey claude check PR"`)
})

test('FR-11.3: zero or unknown items are hidden, except Hôm nay', () => {
  const s = stats({ running: [item({ phase: '', agentMin: 0 })], today: { ...stats().today, prsOpened: 1 } })
  expect(statusText(s, A, X)).toBe(`track ▶ · ${X} · Hôm nay: 0/0 · PR 1 · Tip: 1 PR chờ merge — gõ "hey claude check PR"`)
})

test('FR-11.3: an id of another session, or none bound, is ⏸', () => {
  const s = stats({ running: [item({ session: B })] })
  expect(statusText(s, A, X)).toBe('track ⏸ · Hôm nay: 0/0')
  expect(statusText(stats({ running: [item({})] }), A, undefined)).toBe('track ⏸ · Hôm nay: 0/0')
})

test('SC-43: not onboarded and no open id asks to onboard; an open id keeps the normal line', () => {
  const s = stats({ onboarded: false })
  expect(statusText(s, A, undefined)).toBe('track ⏸ · Tip: Bạn có thể onboard cho agent lấy công việc bằng cách gõ "hey claude onboard track"')
  expect(statusText({ ...s, running: [item({})] }, A, X)).toBe(`track ▶ · ${X} · plan · 12m/M · Hôm nay: 0/0`)
})

test('FR-11.4: an error is the ⚠ line with its message', () => {
  expect(errorStatus('exit 2')).toBe('⚠ track lỗi: exit 2')
})

test('parseTrackStats takes the stats JSON and fills Go nulls; anything else throws', () => {
  const off = parseTrackStats(JSON.stringify({ ...stats({ enabled: false }), rules: null, running: null }))
  expect(off.enabled).toBe(false)
  expect(off.running).toEqual([])
  expect(() => parseTrackStats('not json')).toThrow()
  expect(() => parseTrackStats('{"x":1}')).toThrow('stats output is not track stats JSON')
})

test('SC-11: Tab này shows this session open id; Tổng shows both, grouped by session, mine tagged', () => {
  const s = stats({ running: [item({}), item({ id: Y, kind: 'fix', session: B, cwd: '/w/repos/hub', phase: 'fix' })] })
  const mine = texts(tabLines('this', s, A, X)).join('\n')
  expect(mine).toContain(X)
  expect(mine.includes(Y)).toBe(false)
  const all = texts(tabLines('total', s, A, X))
  expect(all.some(t => t.includes(X))).toBe(true)
  expect(all.some(t => t.includes(Y))).toBe(true)
  expect(all).toContain('aaaaaaaa kit (tab này)')
  expect(all).toContain('bbbbbbbb hub')
})

test('FR-6.2: Tab này lists kind/repo/est, phases, tokens, PRs and the ids done today in this session', () => {
  const done = item({ id: 'fix-261030-0c0c', kind: 'fix', status: 'done', agentMin: 20, est: 'S', prs: ['https://github.com/Z/kit/pull/7'] })
  const s = stats({ running: [item({ prs: ['https://github.com/Z/kit/pull/9'] })], today: { ...stats().today, items: [item({}), done] } })
  const t = texts(tabLines('this', s, A, X))
  expect(t).toContain(`${X} · cook · kit · est M`)
  expect(t).toContain('giai đoạn: plan')
  expect(t).toContain('agent 12m / M (S<30m M<90m L<240m)')
  expect(t).toContain('  plan: agent 12m · chờ 0m')
  expect(t).toContain('  claude-opus-5-5: in 100 · out 200')
  expect(t).toContain('PR https://github.com/Z/kit/pull/9')
  expect(t).toContain('fix-261030-0c0c · done · 20m/S')
})

test('FR-6.2: Tab này with no open id says so', () => {
  expect(texts(tabLines('this', stats(), A, undefined))).toContain('chưa có id đang mở')
})

test('FR-6.2: Estimate shows hit per kind, hotfix time and the agent share by phase', () => {
  const s = stats({
    estimate: {
      byKind: { cook: { n: 4, hit: 3, low: 1, high: 0 } },
      hotfix: { n: 2, medianMin: 15, p90Min: 40 },
      phaseShare: { cook: { plan: 0.25, implement: 0.75 } },
    },
  })
  const t = texts(tabLines('estimate', s, A, X))
  expect(t).toContain('cook: hit 3/4 · thấp 1 · cao 0')
  expect(t).toContain('hotfix: 2 việc · trung vị 15m · p90 40m')
  expect(t).toContain('cook: plan 25% · implement 75%')
})

test('FR-6.2: Chất lượng shows blocked rate, findings, fix loop, wait share, tokens and the fix block; null is —', () => {
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
      fix: { n: 3, roundAvg: 1.5, wideRate: 0.5, regressionRate: null },
    },
  })
  const t = texts(tabLines('quality', s, A, X))
  expect(t).toContain('cook (5 việc): ship blocked 40% · chặn nhiều nhất: review')
  expect(t).toContain('  finding C/H/M/L: 0/1.5/2/0.3 · fix loop 1.2')
  expect(t).toContain('  CI đỏ lần đầu — · chờ 10%')
  expect(t).toContain('  claude-opus-5-5: in 1000 · out 2000')
  expect(t).toContain('fix/hotfix (3 việc): ROUND 1.5 · wide 50% · regression test —')
})

test('FR-6.2: every tab ends with the scale footer and the rules in Vietnamese', () => {
  const s = stats({ rules: ['cook: est up 1 size', 'fix: est down 1 size'] })
  for (const tab of ['this', 'total', 'estimate', 'quality'] as const) {
    const t = texts(tabLines(tab, s, A, X))
    expect(t).toContain('thang (default, 0 việc): S<30m M<90m L<240m')
    expect(t).toContain('cook: tăng est 1 size')
    expect(t).toContain('fix: giảm est 1 size')
  }
  expect(ruleText('hotfix: something new')).toBe('hotfix: something new')
})

test('FR-6.2: Tổng warns on a merged lookup error and on malformed lines', () => {
  const s = stats({ malformed: 2, today: { ...stats().today, mergedError: 'gh: not logged in' } })
  const t = tabLines('total', s, A, X)
  expect(t.some(l => l.text === '⚠ merged lỗi: gh: not logged in' && l.tone === 'warn')).toBe(true)
  expect(t.some(l => l.text === '⚠ 2 dòng log sai format' && l.tone === 'warn')).toBe(true)
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

test('FR-13.7: Backlog groups by source in manual, github, sdd, stale order, one repo · est · title · age line each', () => {
  const s = stats({
    backlog: {
      ghError: '',
      items: [
        bl('stale', { repo: 'hub', est: 'L', title: 'cook hub (c1)', ageDays: 9 }),
        bl('sdd', { est: '-', title: 'spec A', ageDays: 1 }),
        bl('github', { repo: 'web', est: '-', title: 'issue B', ageDays: 0 }),
        bl('manual', { est: 'S', title: 'task C', ageDays: 5 }),
        bl('manual', { est: 'M', title: 'task D', ageDays: 2 }),
      ],
    },
  })
  expect(texts(tabLines('backlog', s, A, X)).slice(0, 9)).toEqual([
    'manual',
    'kit · S · task C · 5d',
    'kit · M · task D · 2d',
    'github',
    'web · - · issue B · 0d',
    'sdd',
    'kit · - · spec A · 1d',
    'stale',
    'hub · L · cook hub (c1) · 9d',
  ])
})

test('FR-13.7: an empty backlog says Backlog trống; a ghError adds a ⚠ github line', () => {
  expect(texts(tabLines('backlog', stats(), A, X))[0]).toBe('Backlog trống')
  const t = tabLines('backlog', stats({ backlog: { items: [bl('manual')], ghError: 'gh: not logged in' } }), A, X)
  expect(t.some(l => l.text === '⚠ github: gh: not logged in' && l.tone === 'warn')).toBe(true)
  expect(texts(t)).toContain('kit · M · manual title · 3d')
})

test('FR-14.5: the footer names the history scale source and its sample size', () => {
  const s = stats({ scale: { source: 'history', n: 25, S: 20, M: 60, L: 150 } })
  expect(texts(tabLines('backlog', s, A, X))).toContain('thang (history, 25 việc): S<20m M<60m L<150m')
})

test('parseTrackStats fills a missing backlog so the tab never reads undefined', () => {
  const raw = { ...stats(), backlog: { items: null, ghError: '' } }
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
  expect(errorStatus('x')).toBe('⚠ track lỗi: x')
  expect(statusText(stats({}), A, undefined).startsWith('track ⏸')).toBe(true)
})
