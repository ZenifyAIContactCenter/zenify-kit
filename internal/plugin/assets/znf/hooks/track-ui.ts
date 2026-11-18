// FR-6.2, FR-11.2–11.4: the pure half of the track pane and status line — what each tab and the
// status line say for one `zenify track stats --json` answer. All UI copy is English (FR-8.1).
import type { KitchenCarried, PRStatus, TrackItem, TrackStats, TrackTab } from '../types'

export type Tab = TrackTab
// A line with `href` is a link: `text` is what stands before it, `label` the clickable text.
export type Line = { text: string; tone?: 'dim' | 'bold' | 'warn' | 'error'; href?: string; label?: string }

export const STATUS_OFF = '⏸ track off · enable: /config → znf'
// The pane's own messages: command replies, the loading line and the toast prefix.
export const COPY = {
  opened: 'track pane opened',
  closed: 'track pane closed',
  loading: 'loading stats…',
  toast: 'track error: ',
  // FR-11.6: the status line's next step for a workspace that is not onboarded yet.
  onboard: 'Tip: onboard the agent to pick work — type "hey claude onboard track"',
  // FR-11.7: contextual tips; {N}, {est}, {min}, {max} are filled by tipFor.
  tipBacklog: 'Tip: {N} items in Backlog — type "/znf:track ls"',
  tipNoEst: 'Tip: no estimate yet — type "hey claude estimate this"',
  tipOver: 'Tip: over est {est} ({min}m/{max}m) — split the work or re-estimate',
  // FR-5.1/FR-5.2: what needs action first; {num}, {N}, {id}, {D} are filled by tipFor.
  tipPrRed: 'Tip: PR #{num} CI red — type "hey claude fix CI"',
  tipPrThreads: 'Tip: PR #{num} has {N} unresolved threads — type "hey claude check PR"',
  tipPrChanges: 'Tip: PR #{num} has changes requested — type "hey claude check PR"',
  tipPrReadyOne: 'Tip: PR #{num} ready to merge — type "hey claude check PR"',
  tipPrReady: 'Tip: {N} PRs ready to merge — type "hey claude check PR"',
  tipStaleSoon: 'Tip: {id} open {D} days — continue or cancel it',
  tipPr: 'Tip: {N} PRs waiting to merge — type "hey claude check PR"',
  // FR-4.5: {tab}, {what}, {T}, {code}, {title}, {latest}, {current} are filled by tipFor.
  tipBoil: 'Tip: "{tab}" waits on your {what} · {T}',
  tipPlanNext: 'Tip: next in plan: {code} {title} — type "/znf:track go {code}"',
  tipPlanDone: 'Tip: plan done — type "/znf:track ls"',
  tipKitUpdate: 'Tip: zenify {latest} is out (running {current}) — run "zenify update"',
  tipPane: 'Tip: see today\'s work in the track pane — type "/track-pane"',
  // FR-11.4 (track-metrics): unfinished work from earlier days.
  tipNoPlan: 'Tip: no plan today — type "/znf:track plan"',
  tipCarried: 'Tip: {N} items carried over — type "hey claude continue"',
  noWorkspace: 'not in a zenify workspace',
  // The main screen reports no clicks: the tabs are reached by keys once the pane holds the keyboard.
  tabKeys: 'ctrl+x tab, then 1–5 to switch tabs',
  // FR-17.3: a /config row that could not be read back from its source.
  configToast: '⚠ /config sync error: ',
} as const
// FR-4.8: the one Vietnamese UI copy, by user decision 2026-11-18; excluded from SC-10.
export const FUN_TIPS: readonly string[] = [
  '🧋 Trà đá không?',
  '💨 Làm bi thuốc lào không?',
  '🍜 Bún chả không anh ei?',
  '☕ Cà phê muối đi',
  '😴 Chợp mắt 15 phút?',
  '🏸 Làm ván cầu lông không?',
  '🍉 Ăn dưa hấu không?',
  '🎮 Một trận rồi làm tiếp?',
  '🌻 Hạt hướng dương tách vỏ đi',
  '🚶 Đi bộ vòng vòng cái nào',
  '🥖 Bánh mì pate không?',
  '🧃 Nước mía không?',
  '🍢 Xiên bẩn không?',
  '🎤 Karaoke tối nay?',
  '🐟 Lướt TikTok 5 phút thôi',
  '🧘 Vươn vai cái nào',
  '🍌 Chuối không?',
  '🍵 Trà chanh chém gió không?',
  '🦐 Bánh tráng trộn không?',
  '🛵 Đi đâu đó hóng gió không?',
]
const FUN_TIP_MS = 600_000
const STATUS_HEAD = 'track'
const ERROR_HEAD = '⚠ track error: '

export const TABS: { id: Tab; label: string }[] = [
  { id: 'kitchen', label: 'Kitchen' },
  { id: 'today', label: 'Today' },
  { id: 'estimate', label: 'Estimate' },
  { id: 'quality', label: 'Quality' },
  { id: 'backlog', label: 'Backlog' },
]
// FR-13.7: the order the Backlog tab groups its sources in.
const BACKLOG_SOURCES = ['manual', 'plane', 'github', 'sdd', 'stale']

// The English rule suffixes of internal/trackstats (RuleUp, RuleDown) and their display copy.
const RULES: Record<string, string> = { 'est up 1 size': 'raise est 1 size', 'est down 1 size': 'lower est 1 size' }

const KINDS = ['cook', 'fix', 'hotfix']
const SIZES = ['S', 'M', 'L']
const SEP = ' · '
// FR-2.4: a figure shows from this many real samples.
const MIN_SAMPLES = 5

// Throws on anything that is not the stats JSON; fills what Go emits as null for an empty field.
export function parseTrackStats(stdout: string): TrackStats {
  const data = JSON.parse(stdout) as Partial<TrackStats> | null
  if (!data || typeof data !== 'object' || typeof data.enabled !== 'boolean') {
    throw new Error('stats output is not track stats JSON')
  }
  const withCode = <T extends { code?: string }>(i: T) => ({ ...i, code: i.code ?? '' })
  const today = {
    ...data.today,
    done: data.today?.done ?? {},
    cancelled: data.today?.cancelled ?? {},
    rolledBack: data.today?.rolledBack ?? {},
    wallMin: data.today?.wallMin ?? 0,
    agentMin: data.today?.agentMin ?? 0,
    dailyPlan: data.today?.dailyPlan ? { ...data.today.dailyPlan, items: data.today.dailyPlan.items.map(withCode) } : null,
    prOpen: data.today?.prOpen ?? [],
    prStatusError: data.today?.prStatusError ?? '',
  }
  const item = (i: TrackItem): TrackItem => ({ ...i, wallMin: i.wallMin ?? 0, tab: i.tab ?? '' })
  const backlog = { ghError: '', planeError: '', ...data.backlog, items: (data.backlog?.items ?? []).map(withCode) }
  const kitchen = {
    titleError: '',
    ...data.kitchen,
    cooked: data.kitchen?.cooked ?? [],
    carried: data.kitchen?.carried ?? [],
    lastDay: data.kitchen?.lastDay ?? null,
    rollingBoil: (data.kitchen?.rollingBoil ?? []).map(b => ({ ...b, tab: b.tab ?? '' })),
    cooking: (data.kitchen?.cooking ?? []).map(c => ({ ...c, tab: c.tab ?? '' })),
  }
  return { ...data, rules: data.rules ?? [], running: (data.running ?? []).map(item), today, backlog, kitchen, kitUpdate: data.kitUpdate ?? null } as TrackStats
}

// The id this session has open: the bound id, running under this session.
export const openItem = (s: TrackStats, session: string, boundId: string | undefined): TrackItem | undefined =>
  boundId === undefined ? undefined : (s.running ?? []).find(i => i.id === boundId && i.session === session)

const sum = (m: Record<string, number> | null | undefined) => Object.values(m ?? {}).reduce((n, v) => n + v, 0)
// FR-7.1: 12.3k and 1.2M from 10000 up.
export const fmtNum = (n: number): string => {
  if (n < 10000) return String(n)
  const k = (n / 1e3).toFixed(1)
  return n < 1e6 && Number(k) < 1000 ? `${k}k` : `${(n / 1e6).toFixed(1)}M`
}
// 50 -> 50m, 180 -> 3h, 270 -> 4h30m, 2665 -> 1d20h.
export const fmtDur = (m: number): string => {
  if (m >= 1440) return `${Math.floor(m / 1440)}d${Math.floor((m % 1440) / 60) > 0 ? `${Math.floor((m % 1440) / 60)}h` : ''}`
  return m < 60 ? `${m}m` : m % 60 === 0 ? `${m / 60}h` : `${Math.floor(m / 60)}h${m % 60}m`
}
const isSize = (est: string) => SIZES.includes(est)
const wallText = (i: TrackItem) => (isSize(i.est) ? `${i.wallMin}m/${i.est}` : `${i.wallMin}m`)

// The workspace's estimate hit rate over every kind, undefined under MIN_SAMPLES samples.
function hitRate(s: TrackStats): number | undefined {
  const byKind = Object.values(s.estimate?.byKind ?? {})
  const n = byKind.reduce((a, k) => a + k.n, 0)
  return n >= MIN_SAMPLES ? byKind.reduce((a, k) => a + k.hit, 0) / n : undefined
}

// FR-4.5: the one tip for this context, by priority; onboard is handled in tipLine.
function tipFor(s: TrackStats, mine: TrackItem | undefined, now: number, session: string, paneInvite: boolean): string | undefined {
  const boil = (s.kitchen?.rollingBoil ?? [])
    .filter((b) => b.session !== session)
    .sort((a, b) => Date.parse(a.since) - Date.parse(b.since))[0]
  if (boil) {
    return COPY.tipBoil
      .replace('{tab}', () => boil.tab || boil.brief)
      .replace('{what}', boil.state === 'ask' ? 'answer' : 'permission')
      .replace('{T}', waited(boil.since, now))
  }
  if (mine) {
    if (!isSize(mine.est) && mine.wallMin >= 5) return COPY.tipNoEst
    if (isSize(mine.est)) {
      const max = s.scale[mine.est as 'S' | 'M' | 'L']
      if (mine.wallMin >= max) {
        return COPY.tipOver.replace('{est}', mine.est).replace('{min}', String(mine.wallMin)).replace('{max}', String(max))
      }
    }
    return paneInvite ? COPY.tipPane : undefined
  }
  const carried = s.kitchen?.carried?.length ?? 0
  const prs = s.today?.prOpen ?? []
  const red = prs.find((p) => p.ci === 'failure')
  if (red) return COPY.tipPrRed.replace('{num}', String(red.number))
  const thr = prs.find((p) => p.threads > 0)
  if (thr) return COPY.tipPrThreads.replace('{num}', String(thr.number)).replace('{N}', String(thr.threads))
  const chg = prs.find((p) => p.review === 'changes_requested')
  if (chg) return COPY.tipPrChanges.replace('{num}', String(chg.number))
  const ready = prs.filter((p) => p.ready)
  const [one] = ready
  if (ready.length === 1 && one) return COPY.tipPrReadyOne.replace('{num}', String(one.number))
  if (ready.length > 1) return COPY.tipPrReady.replace('{N}', String(ready.length))
  if (s.kitUpdate?.newer) return COPY.tipKitUpdate.replace('{latest}', () => s.kitUpdate?.latest ?? '').replace('{current}', () => s.kitUpdate?.current ?? '')
  const DAY = 86_400_000
  const age = (c: { startedAt: string }) => (now - Date.parse(c.startedAt)) / DAY
  const old = (s.kitchen?.carried ?? [])
    .filter((c) => c.stale || age(c) >= 5)
    .sort((a, b) => Date.parse(a.startedAt) - Date.parse(b.startedAt))[0]
  if (old) return COPY.tipStaleSoon.replace('{id}', () => old.id).replace('{D}', String(Math.max(0, Math.floor(age(old)))))
  if (carried > 0) return COPY.tipCarried.replace('{N}', String(carried))
  const plan = s.today?.dailyPlan?.items ?? []
  const next = plan.find((i) => i.status === 'todo')
  if (next) {
    const code = next.code || next.id
    return COPY.tipPlanNext.replaceAll('{code}', () => code).replace('{title}', () => next.title)
  }
  if (plan.length > 0 && !plan.some((i) => i.status === 'running')) return COPY.tipPlanDone
  if (s.onboarded && !s.today?.dailyPlan) return COPY.tipNoPlan
  const n = s.backlog?.items?.length ?? 0
  if (n > 0) return COPY.tipBacklog.replace('{N}', String(n))
  const open = s.today?.prsWaiting ?? 0
  if (open > 0) return COPY.tipPr.replace('{N}', String(open))
  if (paneInvite) return COPY.tipPane
  return FUN_TIPS[Math.floor(now / FUN_TIP_MS) % FUN_TIPS.length]
}

// FR-4.2: the tip the PromptHint row shows, undefined for none. Onboard shows even with tips=false.
export function tipLine(s: TrackStats, session: string, boundId: string | undefined, tips = true, now = Date.now(), paneInvite = false): string | undefined {
  if (!s.enabled) return undefined
  const mine = openItem(s, session, boundId)
  if (!mine && !s.onboarded) return COPY.onboard
  return tips ? tipFor(s, mine, now, session, paneInvite) : undefined
}

// FR-11.2/11.3: an item that is 0 or unknown is hidden, except `Today`.
// FR-4.1: the tip is not part of the line (see tipLine); `tips` and `now` stay for the callers.
export function statusText(s: TrackStats, session: string, boundId: string | undefined, tips = true, now = Date.now()): string {
  if (!s.enabled) return STATUS_OFF
  const mine = openItem(s, session, boundId)
  const t = s.today
  if (!mine && !s.onboarded) return `${STATUS_HEAD} ⏸`
  const hit = hitRate(s)
  const parts: (string | false)[] = [
    `${STATUS_HEAD} ${mine ? '▶' : '⏸'}`,
    !!mine && mine.id,
    !!mine?.phase && mine.phase,
    !!mine?.wallMin && `${wallText(mine)}${mine.agentMin > 0 ? ` (agent ${mine.agentMin}m)` : ''}`,
    !!mine?.wait.total && `wait ${mine.wait.total}m`,
    `Today: ${sum(t.done)}/${sum(t.started)}`,
    (t.prsOpened > 0 || t.merged > 0) && `PR ${t.prsOpened}${t.merged > 0 ? ` (${t.merged} merged)` : ''}`,
    hit !== undefined && `hit ${Math.round(hit * 100)}%`,
  ]
  return parts.filter(Boolean).join(SEP)
}

// FR-11.4.
export const errorStatus = (message: string): string => `${ERROR_HEAD}${message}`

export function ruleText(rule: string): string {
  const at = rule.indexOf(': ')
  const copy = at < 0 ? undefined : RULES[rule.slice(at + 2)]
  return copy ? `${rule.slice(0, at + 2)}${copy}` : rule
}

const head = (text: string): Line => ({ text, tone: 'bold' })
const dim = (text: string): Line => ({ text, tone: 'dim' })
const warn = (text: string): Line => ({ text, tone: 'warn' })
const plain = (text: string): Line => ({ text })

const pad2 = (n: number) => String(n).padStart(2, '0')
const clock = (iso: string) => {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
}
// How long a tab has waited, rounded down: Ns under a minute, Nm under an hour, else Nh.
function waited(sinceIso: string, now: number): string {
  const raw = (now - Date.parse(sinceIso)) / 1000
  if (Number.isNaN(raw)) return '—'
  const sec = Math.max(0, Math.floor(raw))
  return sec < 60 ? `${sec}s` : sec < 3600 ? `${Math.floor(sec / 60)}m` : `${Math.floor(sec / 3600)}h`
}
// Every tab's icons in one place (FR-9.1).
const ICON = {
  cooked: '🍳',
  boil: '🔥',
  cooking: '🥘',
  carried: '📌',
  lastDay: '📆',
  pr: '🔗',
  today: '📅',
  plan: '🗓',
  item: { todo: '☐', running: '▶', done: '✅', cancelled: '⛔' } as Record<string, string>,
  estimate: '🎯',
  quality: '🛡',
  status: { done: '✅', pushed: '🚀', cancelled: '⛔', rollback: '↩️' } as Partial<Record<TrackItem['status'], string>>,
  state: { ask: '❓', permission: '🔐' },
  carry: { open: '⏳', stale: '🕸' },
  // A done item whose PRs are not all merged yet; ✅ waits for the merge.
  unmerged: '⏳',
  source: { manual: '📝', plane: '✈️', github: '🐙', sdd: '📒', stale: '🕸' } as Record<string, string>,
} as const
// `<repo>#<n>` of https://github.com/<owner>/<repo>/pull/<n>; the URL itself when it does not parse.
export const prLabel = (url: string): string => {
  const m = /^https:\/\/github\.com\/[^/]+\/([^/]+)\/pull\/(\d+)/.exec(url)
  return m ? `${m[1]}#${m[2]}` : url
}
// FR-12.3: the braille frames of the Cooking spinner; SPIN_TICK_MS is one frame.
const SPINNER = '⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
// FR-6.3/FR-11.5: how often the watched mtimes are compared (and stats re-run when one changed).
export const WATCH_TICK_MS = 2000
// FR-12.3: how often the Kitchen tab is redrawn while a tab cooks or waits, so the spinner and the
// seconds advance evenly. A redraw runs no command; stats still follow WATCH_TICK_MS.
export const SPIN_TICK_MS = 1000
const BOIL_STATE = { ask: 'asking', permission: 'needs permission' } as const

const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
// `Fri 10/31` of 2026-10-31; the raw date when it does not parse.
function dayLabel(date: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date)
  if (!m) return date
  const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]))
  return `${WEEKDAYS[d.getDay()]} ${m[2]}/${m[3]}`
}
// How long ago an item started: Nm under an hour, Nh under a day, else Nd.
function age(startedAt: string, now: number): string {
  const raw = (now - Date.parse(startedAt)) / 60000
  if (Number.isNaN(raw)) return '—'
  const m = Math.max(0, Math.floor(raw))
  return m < 60 ? `${m}m` : m < 1440 ? `${Math.floor(m / 60)}h` : `${Math.floor(m / 1440)}d`
}
const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

// FR-11.3: the unfinished work and the last working day, above Cooked; nothing when both are empty.
function carriedLines(s: TrackStats, now: number): Line[] {
  const carried: KitchenCarried[] = s.kitchen?.carried ?? []
  const last = s.kitchen?.lastDay
  if (carried.length === 0 && !last) return []
  const lines: Line[] = [head(`${ICON.carried} Carried over`)]
  if (last) {
    const parts = [
      last.done > 0 && `${last.done} done`,
      last.cancelled > 0 && `${last.cancelled} cancelled`,
      last.rolledBack > 0 && `${last.rolledBack} rollback`,
      last.prsOpened > 0 && plural(last.prsOpened, 'PR', 'PRs'),
      last.leftOpen > 0 && `${last.leftOpen} left open`,
    ].filter(Boolean)
    lines.push(plain(`${ICON.lastDay} ${dayLabel(last.date)}${parts.length > 0 ? `: ${parts.join(SEP)}` : ''}`))
  }
  for (const c of carried) {
    const icon = c.stale ? ICON.carry.stale : ICON.carry.open
    lines.push(plain(`- ${icon} ${age(c.startedAt, now)} ${[c.worktree || c.brief, c.kind, c.phase].filter(Boolean).join(SEP)}`))
    lines.push(dim(`    ${c.tab || c.session.slice(0, 8)}${SEP}${c.id}`))
  }
  lines.push(plain(''))
  return lines
}


// FR-12.3: the tabs working right now, one frame apart so they do not turn in step.
function cookingLines(s: TrackStats, now: number): Line[] {
  const cooking = s.kitchen?.cooking ?? []
  const lines: Line[] = [head(`${ICON.cooking} Cooking`)]
  if (cooking.length === 0) lines.push(dim('no tab is cooking'))
  cooking.forEach((c, i) => {
    const frame = SPINNER[(Math.floor(now / SPIN_TICK_MS) + i) % SPINNER.length]
    const more = [c.kind, c.phase].filter(Boolean)
    lines.push(plain(`- ${frame} ${waited(c.since, now)} ${c.tab || c.brief}${more.map(m => SEP + m).join('')}`))
    lines.push(dim(`    ${c.session.slice(0, 8)}${c.id ? SEP + c.id : ''}`))
  })
  lines.push(plain(''))
  return lines
}

// FR-3.1: today's plan above Carried over; nothing without a plan or with an empty one.
function planLines(s: TrackStats): Line[] {
  const p = s.today?.dailyPlan
  if (!p || p.items.length === 0) return []
  const lines: Line[] = [head(`${ICON.plan} Plan`)]
  for (const i of p.items) {
    const row = `- ${ICON.item[i.status] ?? ''} ${[...(i.code ? [i.code] : []), i.repo, i.est, i.title].join(SEP)}`
    lines.push(i.url ? { text: '', href: i.url, label: row } : plain(row))
  }
  lines.push(dim(`est ${fmtDur(p.estMin)}${SEP}used ${fmtDur(p.wallMin)}`), plain(''))
  return lines
}

// FR-3.2: ` · M→52m`, with ⚠ once the wall time reached the size's bound (the rule of the over-est tip).
function cookedEst(s: TrackStats, c: { est?: string; wallMin?: number }): string {
  if (!c.est || !isSize(c.est)) return ''
  const wall = c.wallMin ?? 0
  return `${SEP}${c.est}→${wall}m${wall >= s.scale[c.est as 'S' | 'M' | 'L'] ? ' ⚠' : ''}`
}

// FR-4: the Kitchen tab — the same for every session: what was cooked today, what waits on a person.
function kitchenLines(s: TrackStats, now: number): Line[] {
  const k = s.kitchen
  const cooked = k?.cooked ?? []
  const boil = k?.rollingBoil ?? []
  // Order: what is left, what runs, what waits on you, then what is done.
  const lines: Line[] = [...planLines(s), ...carriedLines(s, now), ...cookingLines(s, now), head(`${ICON.boil} Rolling boil`)]
  if (boil.length === 0) lines.push(dim('no tab is waiting'))
  for (const b of boil) {
    lines.push(plain(`- ${ICON.state[b.state]} ${waited(b.since, now)} ${b.tab || b.brief}`))
    lines.push(dim(`    ${BOIL_STATE[b.state]}${SEP}${b.brief}${SEP}session ${b.session.slice(0, 8)}`))
  }
  lines.push(plain(''), head(`${ICON.cooked} Cooked`))
  if (cooked.length === 0) lines.push(dim('nothing finished today'))
  for (const c of cooked) {
    const icon = c.status === 'done' && !c.merged ? ICON.unmerged : ICON.status[c.status]
    // The icon shows the status and the time comes first, so a long brief is cut at its end only.
    lines.push(plain(`- ${icon ? `${icon} ` : ''}${clock(c.endedAt)} ${c.brief}${cookedEst(s, c)}`))
    for (const pr of c.prs ?? []) lines.push({ text: `  ${ICON.pr} `, href: pr, label: prLabel(pr) })
  }
  if (k?.titleError) lines.push(warn(`⚠ title error: ${k.titleError}`))
  return lines
}

const CI_ICON: Record<string, string> = { success: '✅', failure: '❌', pending: '⏳' }
const REVIEW_COPY: Record<string, string> = { approved: 'approved', changes_requested: 'changes requested' }
// FR-3.3: one open PR as a link line; a ready PR says so instead of its CI and review.
function prLine(pr: PRStatus, now: number): Line {
  const who = `${pr.repo}#${pr.number}${SEP}${age(pr.createdAt, now)}`
  const state = pr.ready
    ? 'ready to merge'
    : [`CI ${CI_ICON[pr.ci] ?? '-'}`, REVIEW_COPY[pr.review] ?? 'waiting', pr.threads > 0 && plural(pr.threads, 'thread', 'threads')].filter(Boolean).join(SEP)
  return { text: `${ICON.pr} `, href: pr.url, label: `${who}${SEP}${state}` }
}

const BAR_W = 20
const SPARKS = '▁▂▃▄▅▆▇█'
const clamp01 = (n: number) => Math.min(1, Math.max(0, n))
// `frac` of `width` cells filled with █, the rest ░.
export const bar = (frac: number, width: number): string => {
  const full = Math.round(clamp01(frac) * width)
  return '█'.repeat(full) + '░'.repeat(width - full)
}
// One cell run per part (chars[i]), `width` cells in all by largest remainder; ░ when every part is 0.
export function stackBar(parts: number[], width: number, chars: string): string {
  const total = parts.reduce((a, v) => a + v, 0)
  if (total <= 0) return '░'.repeat(width)
  const raw = parts.map(v => (v / total) * width)
  const cells = raw.map(Math.floor)
  const order = raw.map((r, i) => ({ i, f: r - Math.floor(r) })).sort((a, b) => b.f - a.f || a.i - b.i)
  const missing = width - cells.reduce((a, v) => a + v, 0)
  const bumped = new Set(order.slice(0, missing).map(o => o.i))
  const glyphs = [...chars]
  return cells.map((n, i) => (glyphs[i] ?? '░').repeat(n + (bumped.has(i) ? 1 : 0))).join('')
}
// A space for 0, else one of ▁…█ scaled to the largest value.
export function sparkline(values: number[]): string {
  const max = Math.max(0, ...values)
  return values.map(v => (v > 0 ? SPARKS[Math.max(0, Math.ceil((v / max) * 8) - 1)] : ' ')).join('')
}
// Minutes of item time per local hour of today, from the earliest start (not before 00:00) to now's hour.
export function hourBins(items: TrackItem[], now: number): { startHour: number; values: number[] } {
  const d = new Date(now)
  const at = (h: number) => new Date(d.getFullYear(), d.getMonth(), d.getDate(), h).getTime()
  const spans = items
    .map(i => ({ from: Date.parse(i.startedAt), to: i.endedAt ? Date.parse(i.endedAt) : now }))
    .filter(x => !Number.isNaN(x.from) && !Number.isNaN(x.to))
  if (spans.length === 0) return { startHour: 0, values: [] }
  const first = Math.min(...spans.map(x => x.from))
  const endHour = d.getHours()
  const startHour = first <= at(0) ? 0 : Math.min(endHour, new Date(first).getHours())
  const values: number[] = []
  for (let h = startHour; h <= endHour && values.length < 24; h++) {
    const lo = at(h)
    const hi = at(h + 1)
    const ms = spans.reduce((a, x) => a + Math.max(0, Math.min(x.to, hi) - Math.max(x.from, lo)), 0)
    values.push(ms / 60000)
  }
  return { startHour, values }
}
// `claude-opus-5-5` -> `opus`; any other id stays.
export const modelName = (id: string): string => /^claude-([a-z]+)-/.exec(id)?.[1] ?? id

// The pane body is 40 terminal cells (44 columns less padding); a longer line loses its tail.
const BODY_W = 40
// Terminal cells of a text: emoji and pictographs (U+1F000+, U+2600-U+27BF, U+23E9-U+23F3) take 2, U+FE0F and U+200D 0, the rest 1.
const cells = (text: string): number =>
  [...text].reduce((n, ch) => {
    const c = ch.codePointAt(0) ?? 0
    if (c === 0xfe0f || c === 0x200d) return n
    return n + (c >= 0x1f000 || (c >= 0x2600 && c <= 0x27bf) || (c >= 0x23e9 && c <= 0x23f3) ? 2 : 1)
  }, 0)
// `parts` joined by SEP under `indent`, broken onto a new line (same indent) when the next part would pass BODY_W.
function packParts(indent: string, parts: string[]): string[] {
  const out: string[] = []
  let cur = ''
  for (const p of parts) {
    const next = cur === '' ? `${indent}${p}` : `${cur}${SEP}${p}`
    if (cur !== '' && cells(next) > BODY_W) {
      out.push(cur)
      cur = `${indent}${p}`
    } else cur = next
  }
  if (cur !== '') out.push(cur)
  return out
}

const KIND_ORDER = ['cook', 'fix', 'hotfix', 'task']
const DAY_MS = 86_400_000
const hh = (n: number) => String(n).padStart(2, '0')

function todayLines(s: TrackStats, _session: string, now: number): Line[] {
  const t = s.today
  const items = t.items ?? []
  const lines: Line[] = [head(`${ICON.today} Today${SEP}${dayLabel(t.date)}`)]
  const started = sum(t.started)
  const done = sum(t.done)
  // done counts items ended today, which may have started on an earlier day, so it can exceed started.
  if (started === 0) lines.push(plain(`✅ Done ${done}/0`))
  else {
    lines.push(plain(`✅ Done ${done}/${started} ${bar(done / started, BAR_W)} ${Math.min(100, Math.round((done / started) * 100))}%`))
    const kinds = Object.keys(t.started ?? {}).filter(k => (t.started?.[k] ?? 0) > 0)
    kinds.sort((a, b) => {
      const ia = KIND_ORDER.indexOf(a)
      const ib = KIND_ORDER.indexOf(b)
      return ia >= 0 || ib >= 0 ? (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib) : a.localeCompare(b)
    })
    for (const k of kinds) {
      const st = t.started?.[k] ?? 0
      const dn = t.done?.[k] ?? 0
      lines.push(plain(`   ${k.padEnd(6)} ${dn}/${st} ${bar(dn / st, BAR_W)}`))
    }
  }
  const cancelled = sum(t.cancelled)
  const rolledBack = sum(t.rolledBack)
  const cr = [cancelled > 0 && `⛔ Cancelled ${cancelled}`, rolledBack > 0 && `↩️ Rollback ${rolledBack}`].filter(Boolean)
  if (cr.length > 0) lines.push(plain(cr.join(SEP)))

  // A running item from an earlier day is only in s.running; hourBins clips its span to today's midnight.
  // Only one started within the last day counts: an older one is left open, not worked on (it has the ⚠ line).
  const seen = new Set(items.map(i => i.id))
  const overnight = (s.running ?? []).filter(i => !seen.has(i.id) && now - Date.parse(i.startedAt) <= DAY_MS)
  const bins = hourBins([...items, ...overnight], now)
  if (bins.values.length > 0) {
    lines.push(plain('⏱ Time by hour'), plain(`   ${hh(bins.startHour)} ${sparkline(bins.values)} ${hh(bins.startHour + bins.values.length - 1)}`))
  }
  if (t.wallMin > 0 || t.agentMin > 0) lines.push(plain(`   wall ${fmtDur(t.wallMin)}${SEP}agent ${fmtDur(t.agentMin)} (parallel)`))

  const wait = { ask: 0, permission: 0, idle: 0, total: 0 }
  for (const i of items) for (const k of ['ask', 'permission', 'idle', 'total'] as const) wait[k] += i.wait?.[k] ?? 0
  if (wait.total > 0) {
    lines.push(plain(`🙋 Waiting on you ${fmtDur(wait.total)}`))
    const parts = [
      wait.ask > 0 && `❓ ask ${fmtDur(wait.ask)}`,
      wait.permission > 0 && `🔐 permission ${fmtDur(wait.permission)}`,
      wait.idle > 0 && `💤 idle ${fmtDur(wait.idle)}`,
    ].filter(Boolean)
    lines.push(plain(`   ${stackBar([wait.ask, wait.permission, wait.idle], BAR_W, '█▓░')}`))
    lines.push(...packParts('   ', parts as string[]).map(plain))
  }

  const tokens: Record<string, { out: number; cacheRead: number }> = {}
  for (const i of items) {
    for (const [m, v] of Object.entries(i.tokens ?? {})) {
      const a = (tokens[m] ??= { out: 0, cacheRead: 0 })
      a.out += v.out ?? 0
      a.cacheRead += v.cacheRead ?? 0
    }
  }
  const models = Object.entries(tokens).sort((a, b) => b[1].out - a[1].out)
  if (models.length > 0) {
    lines.push(plain("🪙 Tokens · today's items"))
    const maxOut = models[0]?.[1].out ?? 0
    for (const [m, v] of models) {
      lines.push(plain(`   ${modelName(m).padEnd(6)} ${bar(maxOut > 0 ? v.out / maxOut : 0, BAR_W)} ${fmtNum(v.out)}`))
      lines.push(dim(`${' '.repeat(10)}cache ${fmtNum(v.cacheRead)}`))
    }
  }

  // FR-1.7: SDD tasks belong to cook work, so they show only after a cook is done today.
  const sdd = [(t.done?.cook ?? 0) > 0 && t.tasks > 0 && `tasks ${fmtNum(t.tasks)}`, t.rounds > 0 && `rounds ${fmtNum(t.rounds)}`].filter(Boolean)
  if (sdd.length > 0) lines.push(plain(`🔁 SDD ${sdd.join(SEP)}`))
  if (t.prsOpened > 0 || t.merged > 0 || t.prsWaiting > 0) {
    lines.push(plain(`${ICON.pr} PR ${t.prsOpened} opened${SEP}${t.merged} merged${SEP}${t.prsWaiting ?? 0} waiting`))
  }
  for (const pr of t.prOpen ?? []) lines.push(prLine(pr, now))
  if (t.prStatusError) lines.push(warn(`⚠ pr status: ${t.prStatusError}`))
  if (t.mergedError) lines.push(warn(`⚠ merged check error: ${t.mergedError}`))
  const scale = s.scale as unknown as Record<string, number>
  const over = (s.running ?? []).filter(i => (isSize(i.est) && i.wallMin >= (scale[i.est] ?? Infinity)) || now - Date.parse(i.startedAt) > DAY_MS).length
  if (over > 0) lines.push(warn(`⚠ ${over} running over size or >1d → Kitchen`))
  if (s.malformed > 0) lines.push(warn(`⚠ ${s.malformed} malformed log lines`))
  return lines
}

// FR-2.4: a figure with fewer than MIN_SAMPLES samples says so, dimmed.
const gate = (n: number, render: () => Line): Line => (n < MIN_SAMPLES ? dim(`need ${MIN_SAMPLES} samples (have ${n})`) : render())
const labelled = (label: string, l: Line): Line => ({ ...l, text: `${label}: ${l.text}` })

const zero = (v: number | null | undefined) => v === null || v === undefined || v === 0
const rate = (v: number) => `${Math.round(v * 100)}%`
const round2 = (v: number) => String(Math.round(v * 100) / 100)
const round1 = (v: number) => String(Math.round(v * 10) / 10)
const present = <T>(v: T | null | undefined): v is T => v !== null && v !== undefined

function estimateLines(s: TrackStats): Line[] {
  const byKind = s.estimate.byKind ?? {}
  const h = s.estimate.hotfix
  const sumN = KINDS.reduce((a, k) => a + (byKind[k]?.n ?? 0), 0)
  const sumHit = KINDS.reduce((a, k) => a + (byKind[k]?.hit ?? 0), 0)
  const title = sumN < MIN_SAMPLES ? `${ICON.estimate} Estimate` : `${ICON.estimate} Estimate${SEP}hit ${rate(sumHit / sumN)} (${sumHit}/${sumN})`
  const lines: Line[] = [head(title)]
  const rowLines: Line[] = []
  let rows = 0
  let charted = false
  for (const k of KINDS) {
    const e = byKind[k]
    const hot = k === 'hotfix' && h.n > 0
    if (!e && !hot) continue
    rows++
    const chart = e && e.n >= MIN_SAMPLES ? `${stackBar([e.hit, e.low, e.high], BAR_W, '█▓░')} ${rate(e.hit / e.n)} ⬆${e.low} ⬇${e.high}` : ''
    if (chart) charted = true
    // The hotfix times go on their own line under the chart row, or on the kind row when there is no chart.
    const times = hot && h.n >= MIN_SAMPLES ? [present(h.medianMin) && `median ${fmtDur(h.medianMin)}`, present(h.p90Min) && `p90 ${fmtDur(h.p90Min)}`].filter(Boolean).join(SEP) : ''
    if (chart) rowLines.push(plain(`${k.padEnd(7)}${chart}`), ...(times ? [plain(`${' '.repeat(7)}${times}`)] : []))
    else if (times) rowLines.push(plain(`${k.padEnd(7)}${times}`))
    else rowLines.push(labelled(k, gate(Math.max(e?.n ?? 0, hot ? h.n : 0), () => plain(''))))
  }
  // An older binary has no task block: skip it.
  const t = s.estimate.task ?? { n: 0, medianMin: null, p90Min: null }
  if (t.n > 0) {
    rows++
    if (t.n < MIN_SAMPLES) rowLines.push(labelled('task', gate(t.n, () => plain(''))))
    else {
      rowLines.push(plain(`task ${t.n} items`))
      const times = [present(t.medianMin) && `median ${fmtDur(t.medianMin)}`, present(t.p90Min) && `p90 ${fmtDur(t.p90Min)}`].filter(Boolean)
      if (times.length > 0) rowLines.push(plain(`${' '.repeat(7)}${times.join(SEP)}`))
    }
  }
  if (charted) lines.push(dim('█ hit · ▓ longer · ░ shorter'))
  lines.push(...rowLines)
  // FR-2.5: the share by phase needs its kind's own sample.
  const share = Object.entries(s.estimate.phaseShare ?? {}).filter(([k]) => (byKind[k]?.n ?? (k === 'hotfix' ? h.n : k === 'task' ? t.n : 0)) >= MIN_SAMPLES)
  if (share.length > 0) lines.push(head('🧭 Agent time by phase'))
  for (const [k, phases] of share) {
    const sorted = Object.entries(phases).sort((a, b) => b[1] - a[1])
    const top = sorted.slice(0, 3)
    const rest = sorted.slice(3).reduce((a, [, v]) => a + v, 0)
    lines.push(plain(`${k.padEnd(7)}${stackBar([...top.map(x => x[1]), rest], BAR_W, '█▓▒░')}`))
    lines.push(...packParts('   ', top.map(([p, v]) => `${p} ${rate(v)}`)).map(plain))
  }
  if (rows === 0) lines.push(dim('no finished work to compare yet'))
  return [...lines, ...footerLines(s)]
}

const SEVERITY = [['🔴', 'critical', 'C'], ['🟠', 'high', 'H'], ['🟡', 'medium', 'M'], ['🔵', 'low', 'L']] as const

function qualityLines(s: TrackStats): Line[] {
  const lines: Line[] = [head(`${ICON.quality} Quality`)]
  const byKind = s.quality.byKind ?? {}
  let rows = 0
  for (const k of KINDS) {
    const q = byKind[k]
    if (!q) continue
    rows++
    if (q.n < MIN_SAMPLES) {
      lines.push(labelled(k, gate(q.n, () => plain(''))))
      continue
    }
    lines.push(head(`${k}${SEP}${q.n} items`))
    const f = q.findingsAvg
    // FR-7.2: all four severities 0 or unknown draw nothing.
    if (f && !(zero(f.C) && zero(f.H) && zero(f.M) && zero(f.L))) {
      const max = Math.max(f.C ?? 0, f.H ?? 0, f.M ?? 0, f.L ?? 0)
      for (const [emoji, label, key] of SEVERITY) {
        const v = f[key] ?? 0
        lines.push(plain(`  ${emoji} ${label.padEnd(8)} ${bar(v / max, 10)} ${round2(v)}`))
      }
    }
    const blocked = [
      present(q.shipBlockedRate) && `🚧 blocked ${rate(q.shipBlockedRate)}${q.topBlockedStep ? ` @${q.topBlockedStep}` : ''}`,
      present(q.fixLoopAvg) && `🔁 fix loop ${round1(q.fixLoopAvg)}`,
      q.ciRedFirstRate === null ? '❌ no CI runs recorded' : present(q.ciRedFirstRate) && `❌ CI red first ${rate(q.ciRedFirstRate)}`,
    ].filter(Boolean)
    lines.push(...packParts('  ', blocked as string[]).map(plain))
    if (present(q.waitShare)) lines.push(plain(`  ⏳ ${'wait'.padEnd(8)} ${bar(q.waitShare, BAR_W)} ${rate(q.waitShare)}`))
    const tok = Object.entries(q.tokensAvg ?? {}).sort((a, b) => b[1].out - a[1].out)
    if (tok.length > 0) lines.push(plain('  🪙 out/item'), ...tok.map(([m, v]) => plain(`     ${modelName(m)} ${fmtNum(Math.round(v.out))}`)))
  }
  const fx = s.quality.fix
  if (fx.n > 0) {
    rows++
    if (fx.n < MIN_SAMPLES) lines.push(labelled('fix/hotfix', gate(fx.n, () => plain(''))))
    else {
      lines.push(head(`🩹 fix/hotfix${SEP}${fx.n} items`))
      if (present(fx.regressionRate)) lines.push(plain(`  🧪 regression ${bar(fx.regressionRate, 16)} ${rate(fx.regressionRate)}`))
      const w = [present(fx.wideRate) && `wide ${rate(fx.wideRate)}`, present(fx.roundAvg) && `rounds ${round1(fx.roundAvg)}`].filter(Boolean)
      if (w.length > 0) lines.push(plain(`  ${w.join(SEP)}`))
    }
  }
  if (rows === 0) lines.push(dim('no work yet'))
  return lines
}

// FR-13.7: items grouped by source, each row `repo · est · title · <age>d`.
function backlogLines(s: TrackStats): Line[] {
  const items = s.backlog?.items ?? []
  const lines: Line[] = []
  if (items.length === 0) lines.push(dim('Backlog is empty'))
  for (const src of BACKLOG_SOURCES) {
    const group = items.filter(i => i.source === src)
    if (group.length === 0) continue
    lines.push(head(`${ICON.source[src] ?? ''} ${src}`.trim()))
    for (const i of group) {
      const row = [...(i.code ? [i.code] : []), i.repo, i.est, i.title, `${i.ageDays}d`].join(SEP)
      // FR-5.2: a URL item is a link, so a truncated line stays clickable.
      const url = [i.ref, i.id].find(v => /^https?:\/\//.test(v ?? ''))
      lines.push(url ? { text: '', href: url, label: row } : plain(row))
    }
  }
  if (s.backlog?.planeError) lines.push(warn(`⚠ plane: ${s.backlog.planeError}`))
  if (s.backlog?.ghError) lines.push(warn(`⚠ github: ${s.backlog.ghError}`))
  return lines
}

// FR-10.7: the S/M/L scale and the calibration rules, under the Estimate tab only.
function footerLines(s: TrackStats): Line[] {
  const sc = s.scale
  return [
    dim(`scale (${sc.source}, ${sc.n}): S<${fmtDur(sc.S)} M<${fmtDur(sc.M)} L<${fmtDur(sc.L)}`),
    ...(s.rules ?? []).map(r => warn(ruleText(r))),
  ]
}

// FR-6.2: the body of one tab.
export function tabLines(tab: Tab, s: TrackStats, session: string, _boundId: string | undefined, now: number): Line[] {
  return tab === 'kitchen'
    ? kitchenLines(s, now)
    : tab === 'today'
      ? todayLines(s, session, now)
      : tab === 'estimate'
        ? estimateLines(s)
        : tab === 'backlog'
          ? backlogLines(s)
          : qualityLines(s)
}

// FR-6.1: `znf:cook|fix|hotfix|ship`, with or without the `znf:` prefix.
const TRACKED = /^(?:znf:)?(?:cook|fix|hotfix|ship)$/
export const isTrackSkill = (name: string): boolean => TRACKED.test(name)
export const isTrackPrompt = (e: { text: string; command?: { name: string } }): boolean =>
  e.command ? isTrackSkill(e.command.name) : /^\/(?:znf:)?(?:cook|fix|hotfix|ship)(?:\s|$)/.test(e.text)

// The track.md section a skill records under: `cook|fix|hotfix|ship|ci-watch`, with or without `znf:`.
export const trackSectionOf = (name: string): string | undefined => /^(?:znf:)?(cook|fix|hotfix|ship|ci-watch)$/.exec(name)?.[1]
// The skill a prompt runs: its slash command, else a leading `/name`.
export const promptSkillOf = (e: { text: string; command?: { name: string } }): string =>
  e.command ? e.command.name : (/^\/(\S+)/.exec(e.text)?.[1] ?? '')

// The `## Rules` and `## <section>` blocks of track.md, each up to the next `## ` heading.
export function trackContext(md: string, section: string): string | undefined {
  const block = (name: string) => {
    const m = new RegExp(`^## ${name}\\n[\\s\\S]*?(?=^## |(?![\\s\\S]))`, 'm').exec(md)
    return m?.[0].trimEnd()
  }
  const rules = block('Rules')
  const own = block(section)
  if (!rules || !own) return undefined
  return `track is on: record this run to \`zenify track\` as below (from skills/_shared/track.md).\n\n${rules}\n\n${own}`
}
