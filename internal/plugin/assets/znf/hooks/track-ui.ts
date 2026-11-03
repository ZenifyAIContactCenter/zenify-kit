// FR-6.2, FR-11.2–11.4: the pure half of the track pane and status line — what each tab and the
// status line say for one `zenify track stats --json` answer. The Vietnamese strings are the UI
// copy the spec fixes; every other string stays English.
import type { TrackItem, TrackStats, TrackTab } from '../types'

export type Tab = TrackTab
export type Line = { text: string; tone?: 'dim' | 'bold' | 'warn' | 'error' }

export const STATUS_OFF = '⏸ track off · bật: /config → znf'
// The pane's own messages: command replies, the loading line and the toast prefix.
export const COPY = {
  opened: 'đã mở pane track',
  closed: 'đã đóng pane track',
  loading: 'đang đọc số liệu…',
  toast: 'lỗi track: ',
  // FR-11.6: the status line's next step for a workspace that is not onboarded yet.
  onboard: 'Tip: Bạn có thể onboard cho agent lấy công việc bằng cách gõ "hey claude onboard track"',
  // FR-11.7: contextual tips; {N}, {est}, {min}, {max} are filled by tipFor.
  tipBacklog: 'Tip: còn {N} việc trong Backlog — gõ "hey claude lấy việc tiếp"',
  tipNoEst: 'Tip: chưa có estimate — gõ "hey claude est việc này"',
  tipOver: 'Tip: đã vượt est {est} ({min}m/{max}m) — cân nhắc tách việc hoặc est lại',
  tipPr: 'Tip: {N} PR chờ merge — gõ "hey claude check PR"',
  noWorkspace: 'không ở trong workspace zenify',
  // FR-17.3: a /config row that could not be read back from its source.
  configToast: '⚠ lỗi đồng bộ /config: ',
} as const
const STATUS_HEAD = 'track'
const ERROR_HEAD = '⚠ track lỗi: '

export const TABS: { id: Tab; label: string }[] = [
  { id: 'kitchen', label: 'Kitchen' },
  { id: 'total', label: 'Tổng' },
  { id: 'estimate', label: 'Estimate' },
  { id: 'quality', label: 'Chất lượng' },
  { id: 'backlog', label: 'Backlog' },
]
// FR-13.7: the order the Backlog tab groups its sources in.
const BACKLOG_SOURCES = ['manual', 'github', 'sdd', 'stale']

// The English rule suffixes of internal/trackstats (RuleUp, RuleDown) and their display copy.
const RULES: Record<string, string> = { 'est up 1 size': 'tăng est 1 size', 'est down 1 size': 'giảm est 1 size' }

const KINDS = ['cook', 'fix', 'hotfix']
const SIZES = ['S', 'M', 'L']
const SEP = ' · '

// Throws on anything that is not the stats JSON; fills what Go emits as null for an empty field.
export function parseTrackStats(stdout: string): TrackStats {
  const data = JSON.parse(stdout) as Partial<TrackStats> | null
  if (!data || typeof data !== 'object' || typeof data.enabled !== 'boolean') {
    throw new Error('stats output is not track stats JSON')
  }
  const backlog = { ghError: '', ...data.backlog, items: data.backlog?.items ?? [] }
  const kitchen = {
    titleError: '',
    ...data.kitchen,
    cooked: data.kitchen?.cooked ?? [],
    rollingBoil: (data.kitchen?.rollingBoil ?? []).map(b => ({ ...b, tab: b.tab ?? '' })),
  }
  return { ...data, rules: data.rules ?? [], running: data.running ?? [], backlog, kitchen } as TrackStats
}

// The id this session has open: the bound id, running under this session.
export const openItem = (s: TrackStats, session: string, boundId: string | undefined): TrackItem | undefined =>
  boundId === undefined ? undefined : (s.running ?? []).find(i => i.id === boundId && i.session === session)

const sum = (m: Record<string, number> | null | undefined) => Object.values(m ?? {}).reduce((n, v) => n + v, 0)
const pct = (v: number | null | undefined) => (v === null || v === undefined ? '—' : `${Math.round(v * 100)}%`)
const num = (v: number | null | undefined) => (v === null || v === undefined ? '—' : String(Math.round(v * 10) / 10))
const min = (v: number | null | undefined) => (v === null || v === undefined ? '—' : `${v}m`)
const isSize = (est: string) => SIZES.includes(est)
const agentText = (i: TrackItem) => (isSize(i.est) ? `${i.agentMin}m/${i.est}` : `${i.agentMin}m`)
const base = (path: string) => path.replace(/\/+$/, '').split('/').pop() || path

// The workspace's estimate hit rate over every kind, undefined with no sample.
function hitRate(s: TrackStats): number | undefined {
  const byKind = Object.values(s.estimate?.byKind ?? {})
  const n = byKind.reduce((a, k) => a + k.n, 0)
  return n > 0 ? byKind.reduce((a, k) => a + k.hit, 0) / n : undefined
}

// FR-11.7 (2)-(5): the one tip for this context, by priority; (1) onboard is handled in statusText.
function tipFor(s: TrackStats, mine: TrackItem | undefined): string | undefined {
  const n = s.backlog?.items?.length ?? 0
  if (!mine && n > 0) return COPY.tipBacklog.replace('{N}', String(n))
  if (mine && !isSize(mine.est) && mine.agentMin >= 5) return COPY.tipNoEst
  if (mine && isSize(mine.est)) {
    const max = s.scale[mine.est as 'S' | 'M' | 'L']
    if (mine.agentMin >= max) {
      return COPY.tipOver.replace('{est}', mine.est).replace('{min}', String(mine.agentMin)).replace('{max}', String(max))
    }
  }
  const open = s.today.prsWaiting ?? 0
  if (open > 0) return COPY.tipPr.replace('{N}', String(open))
  return undefined
}

// FR-11.2/11.3: an item that is 0 or unknown is hidden, except `Hôm nay`.
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
    !!mine?.agentMin && agentText(mine),
    !!mine?.wait.total && `chờ ${mine.wait.total}m`,
    `Hôm nay: ${sum(t.ended)}/${sum(t.started)}`,
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
// The Kitchen tab's icons in one place, so the later English/copy switch touches one spot.
const KITCHEN_ICON = {
  cooked: '🍽',
  boil: '🔥',
  pr: '🔗',
  status: { done: '✅', pushed: '🚀', cancelled: '⛔', rollback: '↩️' } as Partial<Record<TrackItem['status'], string>>,
  state: { ask: '❓', permission: '🔐' },
} as const
const BOIL_STATE = { ask: 'đang hỏi', permission: 'chờ quyền' } as const

// FR-4: the Kitchen tab — the same for every session: what was cooked today, what waits on a person.
function kitchenLines(s: TrackStats, now: number): Line[] {
  const k = s.kitchen
  const cooked = k?.cooked ?? []
  const boil = k?.rollingBoil ?? []
  const lines: Line[] = [head(`${KITCHEN_ICON.cooked} Cooked`)]
  if (cooked.length === 0) lines.push(dim('chưa có việc xong hôm nay'))
  for (const c of cooked) {
    const icon = KITCHEN_ICON.status[c.status]
    lines.push(plain(`- ${icon ? `${icon} ` : ''}${[c.brief, c.status, clock(c.endedAt)].join(SEP)}`))
    for (const pr of c.prs ?? []) lines.push(plain(`  ${KITCHEN_ICON.pr} ${pr}`))
  }
  lines.push(plain(''), head(`${KITCHEN_ICON.boil} Rolling boil`))
  if (boil.length === 0) lines.push(dim('không tab nào đang chờ'))
  for (const b of boil) {
    lines.push(plain(`- ${KITCHEN_ICON.state[b.state]} ${[b.tab || b.brief, BOIL_STATE[b.state], waited(b.since, now)].join(SEP)}`))
    lines.push(dim(`    ${b.brief}${SEP}session ${b.session.slice(0, 8)}`))
  }
  if (k?.titleError) lines.push(warn(`⚠ title lỗi: ${k.titleError}`))
  return lines
}

function totalLines(s: TrackStats, session: string): Line[] {
  const t = s.today
  const lines: Line[] = [
    head(`Hôm nay ${t.date}`),
    plain(`xong/bắt đầu ${sum(t.ended)}/${sum(t.started)}${SEP}task ${t.tasks}${SEP}vòng ${t.rounds}`),
    plain(`PR ${t.prsOpened} (${t.merged} merged)`),
  ]
  const plans = Object.entries(t.plans ?? {})
  if (plans.length > 0) lines.push(plain(`plan: ${plans.map(([repo, n]) => `${repo} ${n}`).join(SEP)}`))
  if (t.mergedError) lines.push(warn(`⚠ merged lỗi: ${t.mergedError}`))
  if (s.malformed > 0) lines.push(warn(`⚠ ${s.malformed} dòng log sai format`))
  lines.push(head('Đang chạy'))
  const running = s.running ?? []
  if (running.length === 0) lines.push(dim('không có việc đang chạy'))
  const sessions = [...new Set(running.map(i => i.session))]
  for (const sid of sessions) {
    const items = running.filter(i => i.session === sid)
    lines.push(plain(`${sid.slice(0, 8)} ${base(items[0]?.cwd ?? '')}${sid === session ? ' (tab này)' : ''}`))
    for (const i of items) {
      const tail = [i.phase, agentText(i), i.status === 'stale' ? 'stale' : ''].filter(Boolean)
      lines.push(plain(`  ${[i.id, ...tail].join(SEP)}`))
    }
  }
  return lines
}

function estimateLines(s: TrackStats): Line[] {
  const lines: Line[] = []
  const byKind = s.estimate.byKind ?? {}
  for (const k of KINDS) {
    const e = byKind[k]
    if (e) lines.push(plain(`${k}: hit ${e.hit}/${e.n}${SEP}thấp ${e.low}${SEP}cao ${e.high}`))
  }
  const h = s.estimate.hotfix
  if (h.n > 0) lines.push(plain(`hotfix: ${h.n} việc${SEP}trung vị ${min(h.medianMin)}${SEP}p90 ${min(h.p90Min)}`))
  const share = Object.entries(s.estimate.phaseShare ?? {})
  if (share.length > 0) lines.push(head('agent theo giai đoạn'))
  for (const [k, phases] of share) {
    lines.push(plain(`${k}: ${Object.entries(phases).map(([p, v]) => `${p} ${pct(v)}`).join(SEP)}`))
  }
  if (lines.length === 0) lines.push(dim('chưa có việc đã xong để so estimate'))
  return lines
}

function qualityLines(s: TrackStats): Line[] {
  const lines: Line[] = []
  const byKind = s.quality.byKind ?? {}
  for (const k of KINDS) {
    const q = byKind[k]
    if (!q) continue
    const f = q.findingsAvg
    lines.push(head(`${k} (${q.n} việc): ship blocked ${pct(q.shipBlockedRate)}${SEP}chặn nhiều nhất: ${q.topBlockedStep ?? '—'}`))
    lines.push(plain(`  finding C/H/M/L: ${f ? [f.C, f.H, f.M, f.L].map(num).join('/') : '—'}${SEP}fix loop ${num(q.fixLoopAvg)}`))
    lines.push(plain(`  CI đỏ lần đầu ${pct(q.ciRedFirstRate)}${SEP}chờ ${pct(q.waitShare)}`))
    for (const [model, t] of Object.entries(q.tokensAvg ?? {})) lines.push(plain(`  ${model}: in ${num(t.in)}${SEP}out ${num(t.out)}`))
  }
  const fx = s.quality.fix
  if (fx.n > 0) {
    lines.push(head(`fix/hotfix (${fx.n} việc): ROUND ${num(fx.roundAvg)}${SEP}wide ${pct(fx.wideRate)}${SEP}regression test ${pct(fx.regressionRate)}`))
  }
  if (lines.length === 0) lines.push(dim('chưa có việc nào'))
  return lines
}

// FR-13.7: items grouped by source, each row `repo · est · title · <age>d`.
function backlogLines(s: TrackStats): Line[] {
  const items = s.backlog?.items ?? []
  const lines: Line[] = []
  if (items.length === 0) lines.push(dim('Backlog trống'))
  for (const src of BACKLOG_SOURCES) {
    const group = items.filter(i => i.source === src)
    if (group.length === 0) continue
    lines.push(head(src))
    for (const i of group) lines.push(plain([i.repo, i.est, i.title, `${i.ageDays}d`].join(SEP)))
  }
  if (s.backlog?.ghError) lines.push(warn(`⚠ github: ${s.backlog.ghError}`))
  return lines
}

// FR-6.2: the footer under every tab — the S/M/L scale and the calibration rules.
function footerLines(s: TrackStats): Line[] {
  const sc = s.scale
  return [
    dim(`thang (${sc.source}, ${sc.n} việc): S<${sc.S}m M<${sc.M}m L<${sc.L}m`),
    ...(s.rules ?? []).map(r => warn(ruleText(r))),
  ]
}

// FR-6.2: the body of one tab, then the footer.
export function tabLines(tab: Tab, s: TrackStats, session: string, boundId: string | undefined, now: number): Line[] {
  const body =
    tab === 'kitchen' ? kitchenLines(s, now) : tab === 'total' ? totalLines(s, session) : tab === 'estimate'
          ? estimateLines(s)
          : tab === 'backlog'
            ? backlogLines(s)
            : qualityLines(s)
  return [...body, ...footerLines(s)]
}

// FR-6.1: `znf:cook|fix|hotfix|ship`, with or without the `znf:` prefix.
const TRACKED = /^(?:znf:)?(?:cook|fix|hotfix|ship)$/
export const isTrackSkill = (name: string): boolean => TRACKED.test(name)
export const isTrackPrompt = (e: { text: string; command?: { name: string } }): boolean =>
  e.command ? isTrackSkill(e.command.name) : /^\/(?:znf:)?(?:cook|fix|hotfix|ship)(?:\s|$)/.test(e.text)
