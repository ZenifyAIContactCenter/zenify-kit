// The JSON of `zenify track stats --json` (Go `trackstats.Result`); names match the JSON exactly.
// Go emits `null` for an empty slice or map it never filled, so those fields are nullable here.
export type TrackKind = 'cook' | 'fix' | 'hotfix' | 'task'

export type TrackItem = {
  id: string
  from: string
  // set only on `running`; false everywhere else
  live: boolean
  kind: TrackKind
  repo: string
  est: string
  session: string
  cwd: string
  // the tab's title; empty when unknown
  tab: string
  phase: string
  status: 'running' | 'stale' | 'done' | 'cancelled' | 'rollback' | 'pushed'
  startedAt: string
  endedAt: string
  prs: string[] | null
  agentMin: number
  wallMin: number
  wait: { ask: number; permission: number; idle: number; total: number }
  phases: Record<string, { agentMin: number; waitMin: number }> | null
  tokens: Record<string, { in: number; out: number; cacheRead: number; cacheCreation: number }> | null
}

// One Backlog row; source is manual|plane|github|sdd|stale (Go `trackstats.BacklogItem`).
export type BacklogItem = {
  source: string
  code: string
  id: string
  repo: string
  est: string
  ref: string
  title: string
  ageDays: number
}

// One item that ended today, workspace-wide (Go `trackstats.CookedItem`).
// `merged`: every PR of the item is merged (absent from an older binary: read as not merged).
export type KitchenCooked = { id: string; brief: string; status: TrackItem['status']; endedAt: string; prs: string[] | null; merged?: boolean; est?: string; wallMin?: number }
// One line of today's plan (Go `trackstats.DailyPlanItem`).
export type DailyPlanItem = { id: string; code: string; repo: string; est: string; title: string; status: 'todo' | 'running' | 'done' | 'cancelled'; url: string }
export type DailyPlan = { items: DailyPlanItem[]; estMin: number; wallMin: number }
// One open PR with its CI and review state (Go `trackstats.PRStatus`); '-' = unknown.
export type PRStatus = {
  url: string
  repo: string
  number: number
  createdAt: string
  ci: 'success' | 'failure' | 'pending' | '-'
  review: 'approved' | 'changes_requested' | 'review_required' | '-'
  threads: number
  ready: boolean
}
// One live tab waiting for a person (Go `trackstats.BoilItem`).
export type KitchenBoil = { session: string; tab: string; brief: string; state: 'ask' | 'permission'; since: string }

// One live tab that is working right now (Go `trackstats.CookingItem`).
export type KitchenCooking = { session: string; tab: string; brief: string; id: string; kind: string; phase: string; since: string }

// One item started before today and not ended (Go `trackstats.CarriedItem`).
export type KitchenCarried = {
  id: string
  brief: string
  kind: string
  repo: string
  phase: string
  startedAt: string
  wallMin: number
  stale: boolean
  worktree: string
  tab: string
  session: string
}
// The last working day before today (Go `trackstats.LastDay`).
export type KitchenLastDay = { date: string; done: number; cancelled: number; rolledBack: number; prsOpened: number; leftOpen: number }

export type TrackStats = {
  enabled: boolean
  malformed: number
  scale: { source: string; n: number; S: number; M: number; L: number }
  rules: string[] | null
  today: {
    date: string
    plans: Record<string, number> | null
    started: Record<string, number> | null
    ended: Record<string, number> | null
    done: Record<string, number> | null
    cancelled: Record<string, number> | null
    rolledBack: Record<string, number> | null
    wallMin: number
    agentMin: number
    tasks: number
    rounds: number
    prsOpened: number
    merged: number
    prsWaiting: number
    mergedError: string
    dailyPlan: DailyPlan | null
    prOpen: PRStatus[]
    prStatusError: string
    items: TrackItem[] | null
  }
  running: TrackItem[] | null
  estimate: {
    byKind: Record<string, { n: number; hit: number; low: number; high: number }> | null
    hotfix: { n: number; medianMin: number | null; p90Min: number | null }
    task?: { n: number; medianMin: number | null; p90Min: number | null }
    phaseShare: Record<string, Record<string, number>> | null
  }
  quality: {
    byKind: Record<
      string,
      {
        n: number
        shipBlockedRate: number | null
        topBlockedStep: string | null
        findingsAvg: { C: number; H: number; M: number; L: number } | null
        fixLoopAvg: number | null
        ciRedFirstRate: number | null
        waitShare: number | null
        tokensAvg: Record<string, { in: number; out: number }> | null
      }
    > | null
    fix: { n: number; roundAvg: number | null; wideRate: number | null; regressionRate: number | null }
  }
  backlog: { items: BacklogItem[] | null; ghError: string; planeError: string }
  onboarded: boolean
  kitchen: {
    carried: KitchenCarried[] | null
    lastDay: KitchenLastDay | null
    cooked: KitchenCooked[] | null
    rollingBoil: KitchenBoil[] | null
    cooking: KitchenCooking[] | null
    titleError: string
  }
}

export type TrackTab = 'kitchen' | 'today' | 'estimate' | 'quality' | 'backlog'

declare module 'claude-code' {
  interface PluginState {
    znf: {
      // the last `zenify track stats --json` answer, or why it failed; `loading` while a run is out
      stats: Shaped<{ data?: TrackStats; error?: string; loading?: boolean } | null>
      // the selected tab of the track pane
      tab: Shaped<TrackTab>
      // zenify track: the id this session's turns are recorded under; `ending` once its
      // `zenify track end` ran, unbound at that turn's main-loop end
      bound: Shaped<{ id: string; kind: TrackKind; ending: boolean } | null>
      // zenify track: `enabled` of `zenify track stats --json`; false writes nothing
      enabled: Shaped<boolean>
      // zenify track: epoch ms the last main-loop turn ended, until the next prompt reads it
      lastMainEnd: Shaped<number | null>
      // zenify track: main-loop tool_use_ids a person was asked to allow
      personAsks: Shaped<string[]>
    }
  }
}
