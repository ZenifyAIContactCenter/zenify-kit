// FR-6.2, FR-11.2–11.4: the pure half of the track pane and status line — what each tab and the
// status line say for one `zenify track stats --json` answer. All UI copy is English (FR-8.1).
import type { KitchenCarried, TrackItem, TrackStats, TrackTab } from '../types'

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
  tipBacklog: 'Tip: {N} items in Backlog — type "hey claude next task"',
  tipNoEst: 'Tip: no estimate yet — type "hey claude estimate this"',
  tipOver: 'Tip: over est {est} ({min}m/{max}m) — split the work or re-estimate',
  tipPr: 'Tip: {N} PRs waiting to merge — type "hey claude check PR"',
  // FR-11.4 (track-metrics): unfinished work from earlier days.
  tipCarried: 'Tip: {N} items carried over — type "hey claude continue"',
  noWorkspace: 'not in a zenify workspace',
  // The main screen reports no clicks: the tabs are reached by keys once the pane holds the keyboard.
  tabKeys: 'ctrl+x tab, then 1–5 to switch tabs',
  // FR-17.3: a /config row that could not be read back from its source.
  configToast: '⚠ /config sync error: ',
} as const
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
const BACKLOG_SOURCES = ['manual', 'github', 'sdd', 'stale']

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
  const today = {
    ...data.today,
    done: data.today?.done ?? {},
    cancelled: data.today?.cancelled ?? {},
    rolledBack: data.today?.rolledBack ?? {},
    wallMin: data.today?.wallMin ?? 0,
    agentMin: data.today?.agentMin ?? 0,
  }
  const item = (i: TrackItem): TrackItem => ({ ...i, wallMin: i.wallMin ?? 0, tab: i.tab ?? '' })
  const backlog = { ghError: '', ...data.backlog, items: data.backlog?.items ?? [] }
  const kitchen = {
    titleError: '',
    ...data.kitchen,
    cooked: data.kitchen?.cooked ?? [],
    carried: data.kitchen?.carried ?? [],
    lastDay: data.kitchen?.lastDay ?? null,
    rollingBoil: (data.kitchen?.rollingBoil ?? []).map(b => ({ ...b, tab: b.tab ?? '' })),
    cooking: (data.kitchen?.cooking ?? []).map(c => ({ ...c, tab: c.tab ?? '' })),
  }
  return { ...data, rules: data.rules ?? [], running: (data.running ?? []).map(item), today, backlog, kitchen } as TrackStats
}

// The id this session has open: the bound id, running under this session.
export const openItem = (s: TrackStats, session: string, boundId: string | undefined): TrackItem | undefined =>
  boundId === undefined ? undefined : (s.running ?? []).find(i => i.id === boundId && i.session === session)

const sum = (m: Record<string, number> | null | undefined) => Object.values(m ?? {}).reduce((n, v) => n + v, 0)
const pct = (v: number | null | undefined) => (v === null || v === undefined ? '—' : `${Math.round(v * 100)}%`)
const num = (v: number | null | undefined) => (v === null || v === undefined ? '—' : String(Math.round(v * 10) / 10))
// FR-7.1: 12.3k and 1.2M from 10000 up.
export const fmtNum = (n: number): string => {
  if (n < 10000) return String(n)
  const k = (n / 1e3).toFixed(1)
  return n < 1e6 && Number(k) < 1000 ? `${k}k` : `${(n / 1e6).toFixed(1)}M`
}
const min = (v: number | null | undefined) => (v === null || v === undefined ? '—' : `${v}m`)
const isSize = (est: string) => SIZES.includes(est)
const wallText = (i: TrackItem) => (isSize(i.est) ? `${i.wallMin}m/${i.est}` : `${i.wallMin}m`)
const base = (path: string) => path.replace(/\/+$/, '').split('/').pop() || path

// The workspace's estimate hit rate over every kind, undefined under MIN_SAMPLES samples.
function hitRate(s: TrackStats): number | undefined {
  const byKind = Object.values(s.estimate?.byKind ?? {})
  const n = byKind.reduce((a, k) => a + k.n, 0)
  return n >= MIN_SAMPLES ? byKind.reduce((a, k) => a + k.hit, 0) / n : undefined
}

// FR-11.7 (2)-(5): the one tip for this context, by priority; (1) onboard is handled in statusText.
function tipFor(s: TrackStats, mine: TrackItem | undefined): string | undefined {
  const carried = s.kitchen?.carried?.length ?? 0
  if (!mine && carried > 0) return COPY.tipCarried.replace('{N}', String(carried))
  const n = s.backlog?.items?.length ?? 0
  if (!mine && n > 0) return COPY.tipBacklog.replace('{N}', String(n))
  if (mine && !isSize(mine.est) && mine.wallMin >= 5) return COPY.tipNoEst
  if (mine && isSize(mine.est)) {
    const max = s.scale[mine.est as 'S' | 'M' | 'L']
    if (mine.wallMin >= max) {
      return COPY.tipOver.replace('{est}', mine.est).replace('{min}', String(mine.wallMin)).replace('{max}', String(max))
    }
  }
  const open = s.today.prsWaiting ?? 0
  if (open > 0) return COPY.tipPr.replace('{N}', String(open))
  return undefined
}

// FR-11.2/11.3: an item that is 0 or unknown is hidden, except `Today`.
// FR-11.8: tips=false drops only the FR-11.7 (2)-(5) tips; onboard and the off line stay.
export function statusText(s: TrackStats, session: string, boundId: string | undefined, tips = true): string {
  if (!s.enabled) return STATUS_OFF
  const mine = openItem(s, session, boundId)
  const t = s.today
  if (!mine && !s.onboarded) return `${STATUS_HEAD} ⏸${SEP}${COPY.onboard}`
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
    (tips && tipFor(s, mine)) || false,
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
  running: '🏃',
  estimate: '🎯',
  quality: '🛡',
  status: { done: '✅', pushed: '🚀', cancelled: '⛔', rollback: '↩️' } as Partial<Record<TrackItem['status'], string>>,
  state: { ask: '❓', permission: '🔐' },
  carry: { open: '⏳', stale: '🕸' },
  source: { manual: '📝', github: '🐙', sdd: '📒', stale: '🕸' } as Record<string, string>,
} as const
// `<repo>#<n>` of https://github.com/<owner>/<repo>/pull/<n>; the URL itself when it does not parse.
export const prLabel = (url: string): string => {
  const m = /^https:\/\/github\.com\/[^/]+\/([^/]+)\/pull\/(\d+)/.exec(url)
  return m ? `${m[1]}#${m[2]}` : url
}
// FR-12.3: the braille frames of the Cooking spinner; WATCH_TICK_MS is one frame.
const SPINNER = '⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
// FR-6.3/FR-11.5: how often the watched mtimes are compared; also the length of one spinner frame.
export const WATCH_TICK_MS = 2000
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
    const frame = SPINNER[(Math.floor(now / WATCH_TICK_MS) + i) % SPINNER.length]
    const more = [c.kind, c.phase].filter(Boolean)
    lines.push(plain(`- ${frame} ${waited(c.since, now)} ${c.tab || c.brief}${more.map(m => SEP + m).join('')}`))
    lines.push(dim(`    ${c.session.slice(0, 8)}${c.id ? SEP + c.id : ''}`))
  })
  lines.push(plain(''))
  return lines
}

// FR-4: the Kitchen tab — the same for every session: what was cooked today, what waits on a person.
function kitchenLines(s: TrackStats, now: number): Line[] {
  const k = s.kitchen
  const cooked = k?.cooked ?? []
  const boil = k?.rollingBoil ?? []
  // Order: what is left, what runs, what waits on you, then what is done.
  const lines: Line[] = [...carriedLines(s, now), ...cookingLines(s, now), head(`${ICON.boil} Rolling boil`)]
  if (boil.length === 0) lines.push(dim('no tab is waiting'))
  for (const b of boil) {
    lines.push(plain(`- ${ICON.state[b.state]} ${waited(b.since, now)} ${b.tab || b.brief}`))
    lines.push(dim(`    ${BOIL_STATE[b.state]}${SEP}${b.brief}${SEP}session ${b.session.slice(0, 8)}`))
  }
  lines.push(plain(''), head(`${ICON.cooked} Cooked`))
  if (cooked.length === 0) lines.push(dim('nothing finished today'))
  for (const c of cooked) {
    const icon = ICON.status[c.status]
    // The icon shows the status and the time comes first, so a long brief is cut at its end only.
    lines.push(plain(`- ${icon ? `${icon} ` : ''}${clock(c.endedAt)} ${c.brief}`))
    for (const pr of c.prs ?? []) lines.push({ text: `  ${ICON.pr} `, href: pr, label: prLabel(pr) })
  }
  if (k?.titleError) lines.push(warn(`⚠ title error: ${k.titleError}`))
  return lines
}

function todayLines(s: TrackStats, session: string): Line[] {
  const t = s.today
  const cancelled = sum(t.cancelled)
  const rolledBack = sum(t.rolledBack)
  const lines: Line[] = [
    head(`${ICON.today} Today ${t.date}`),
    plain(
      [
        `Done ${sum(t.done)}`,
        `started ${sum(t.started)}`,
        cancelled > 0 && `Cancelled ${cancelled}`,
        rolledBack > 0 && `Rollback ${rolledBack}`,
      ]
        .filter(Boolean)
        .join(SEP),
    ),
  ]
  if (t.wallMin > 0 || t.agentMin > 0) lines.push(plain(`Time ${t.wallMin}m${SEP}agent ${t.agentMin}m`))
  // FR-1.4: SDD tasks and rounds belong to cook work, so they show only after a cook is done today.
  if ((t.done?.cook ?? 0) > 0) lines.push(plain(`SDD tasks ${fmtNum(t.tasks)}`))
  if (t.rounds > 0) lines.push(plain(`Rounds ${fmtNum(t.rounds)}`))
  if (t.prsOpened > 0 || t.merged > 0) lines.push(plain(`PR ${t.prsOpened} (${t.merged} merged)`))
  const plans = Object.entries(t.plans ?? {})
  if (plans.length > 0) lines.push(plain(`Plans written today: ${plans.map(([repo, n]) => `${repo} ${n}`).join(SEP)}`))
  if (t.mergedError) lines.push(warn(`⚠ merged check error: ${t.mergedError}`))
  if (s.malformed > 0) lines.push(warn(`⚠ ${s.malformed} malformed log lines`))
  lines.push(head(`${ICON.running} Running`))
  const running = s.running ?? []
  if (running.length === 0) lines.push(dim('nothing running'))
  const sessions = [...new Set(running.map(i => i.session))]
  for (const sid of sessions) {
    const items = running.filter(i => i.session === sid)
    lines.push(plain(`${items[0]?.tab || sid.slice(0, 8)} ${base(items[0]?.cwd ?? '')}${sid === session ? ' (this tab)' : ''}`))
    for (const i of items) {
      const tail = [i.phase, wallText(i), `agent ${i.agentMin}m`, i.status === 'stale' ? 'stale' : ''].filter(Boolean)
      lines.push(plain(`  ${[i.id, ...tail].join(SEP)}`))
    }
  }
  return lines
}

// FR-2.4: a figure with fewer than MIN_SAMPLES samples says so, dimmed.
const gate = (n: number, render: () => Line): Line => (n < MIN_SAMPLES ? dim(`need ${MIN_SAMPLES} samples (have ${n})`) : render())
const labelled = (label: string, l: Line): Line => ({ ...l, text: `${label}: ${l.text}` })

function estimateLines(s: TrackStats): Line[] {
  const lines: Line[] = [head(`${ICON.estimate} Estimate`)]
  const byKind = s.estimate.byKind ?? {}
  let rows = 0
  for (const k of KINDS) {
    const e = byKind[k]
    if (!e) continue
    rows++
    lines.push(labelled(k, gate(e.n, () => plain(`hit ${e.hit}/${e.n}${SEP}under ${e.low}${SEP}over ${e.high}`))))
  }
  const h = s.estimate.hotfix
  if (h.n > 0) {
    rows++
    lines.push(labelled('hotfix', gate(h.n, () => plain(`${h.n} items${SEP}median ${min(h.medianMin)}${SEP}p90 ${min(h.p90Min)}`))))
  }
  // FR-2.5: the share by phase needs its kind's own sample.
  const share = Object.entries(s.estimate.phaseShare ?? {}).filter(([k]) => (byKind[k]?.n ?? (k === 'hotfix' ? h.n : 0)) >= MIN_SAMPLES)
  if (share.length > 0) lines.push(head('agent time by phase'))
  for (const [k, phases] of share) {
    lines.push(plain(`${k}: ${Object.entries(phases).map(([p, v]) => `${p} ${pct(v)}`).join(SEP)}`))
  }
  if (rows === 0) lines.push(dim('no finished work to compare yet'))
  return [...lines, ...footerLines(s)]
}

const zero = (v: number | null | undefined) => v === null || v === undefined || v === 0

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
    const f = q.findingsAvg
    lines.push(head(`${k} (${q.n} items): ship blocked ${pct(q.shipBlockedRate)}${SEP}most blocked at: ${q.topBlockedStep ?? '—'}`))
    // FR-7.2: a sub-line whose figures are all 0 or unknown is hidden.
    if (f && !(zero(f.C) && zero(f.H) && zero(f.M) && zero(f.L)) || !zero(q.fixLoopAvg)) {
      lines.push(plain(`  finding C/H/M/L: ${f ? [f.C, f.H, f.M, f.L].map(num).join('/') : '—'}${SEP}fix loop ${num(q.fixLoopAvg)}`))
    }
    if (q.ciRedFirstRate === null || !zero(q.ciRedFirstRate) || !zero(q.waitShare)) {
      const ci = q.ciRedFirstRate === null ? 'no CI runs recorded' : pct(q.ciRedFirstRate)
      lines.push(plain(`  CI red first: ${ci}${SEP}wait ${pct(q.waitShare)}`))
    }
  }
  const fx = s.quality.fix
  if (fx.n > 0) {
    rows++
    lines.push(
      fx.n < MIN_SAMPLES
        ? labelled('fix/hotfix', gate(fx.n, () => plain('')))
        : head(`fix/hotfix (${fx.n} items): ROUND ${num(fx.roundAvg)}${SEP}wide ${pct(fx.wideRate)}${SEP}regression test ${pct(fx.regressionRate)}`),
    )
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
      const row = [i.repo, i.est, i.title, `${i.ageDays}d`].join(SEP)
      // FR-5.2: a URL item is a link, so a truncated line stays clickable.
      const url = [i.ref, i.id].find(v => /^https?:\/\//.test(v ?? ''))
      lines.push(url ? { text: '', href: url, label: row } : plain(row))
    }
  }
  if (s.backlog?.ghError) lines.push(warn(`⚠ github: ${s.backlog.ghError}`))
  return lines
}

// FR-10.7: the S/M/L scale and the calibration rules, under the Estimate tab only.
function footerLines(s: TrackStats): Line[] {
  const sc = s.scale
  return [
    dim(`scale (${sc.source}, ${sc.n} items): S<${sc.S}m M<${sc.M}m L<${sc.L}m`),
    ...(s.rules ?? []).map(r => warn(ruleText(r))),
  ]
}

// FR-6.2: the body of one tab.
export function tabLines(tab: Tab, s: TrackStats, session: string, _boundId: string | undefined, now: number): Line[] {
  return tab === 'kitchen'
    ? kitchenLines(s, now)
    : tab === 'today'
      ? todayLines(s, session)
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
