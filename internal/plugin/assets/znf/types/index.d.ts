// The JSON of `zenify track stats --json` (Go `trackstats.Result`); names match the JSON exactly.
// Go emits `null` for an empty slice or map it never filled, so those fields are nullable here.
export type TrackKind = 'cook' | 'fix' | 'hotfix'

export type TrackItem = {
  id: string
  kind: TrackKind
  repo: string
  est: string
  session: string
  cwd: string
  phase: string
  status: 'running' | 'stale' | 'done' | 'cancelled' | 'rollback' | 'pushed'
  startedAt: string
  endedAt: string
  prs: string[] | null
  agentMin: number
  wait: { ask: number; permission: number; idle: number; total: number }
  phases: Record<string, { agentMin: number; waitMin: number }> | null
  tokens: Record<string, { in: number; out: number; cacheRead: number; cacheCreation: number }> | null
}

// One Backlog row; source is manual|github|sdd|stale (Go `trackstats.BacklogItem`).
export type BacklogItem = {
  source: string
  id: string
  repo: string
  est: string
  ref: string
  title: string
  ageDays: number
}

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
    tasks: number
    rounds: number
    prsOpened: number
    merged: number
    mergedError: string
    items: TrackItem[] | null
  }
  running: TrackItem[] | null
  estimate: {
    byKind: Record<string, { n: number; hit: number; low: number; high: number }> | null
    hotfix: { n: number; medianMin: number | null; p90Min: number | null }
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
  backlog: { items: BacklogItem[] | null; ghError: string }
  onboarded: boolean
}

export type TrackTab = 'this' | 'total' | 'estimate' | 'quality' | 'backlog'

declare module 'claude-code' {
  interface PluginState {
    znf: {
      // the last `zenify track stats --json` answer, or why it failed
      stats: Shaped<{ data?: TrackStats; error?: string } | null>
      // the selected tab of the track pane
      tab: Shaped<TrackTab>
      // zenify track: the id this session's turns are recorded under; `ending` once its
      // `zenify track end` ran, unbound at that turn's main-loop end
      bound: Shaped<{ id: string; kind: 'cook' | 'fix' | 'hotfix'; ending: boolean } | null>
      // zenify track: `enabled` of `zenify track stats --json`; false writes nothing
      enabled: Shaped<boolean>
      // zenify track: epoch ms the last main-loop turn ended, until the next prompt reads it
      lastMainEnd: Shaped<number | null>
      // zenify track: main-loop tool_use_ids a person was asked to allow
      personAsks: Shaped<string[]>
    }
  }
}
