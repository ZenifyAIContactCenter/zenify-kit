export type LedgerTask = {
  id: string
  priority: string
  repo: string
  source: string
  issue: string
  size: string
  skill: string
  worker: string
  status: string
  evidence: string
}

export type Ledger = {
  date: string | null
  tasks: LedgerTask[]
  needs: { id: string; question: string }[]
  malformed: string[]
}

// The JSON of `zenify nonstop stats --json`.
export type NonstopStats = {
  missing: boolean
  scale: { source: 'learned' | 'default'; n: number; S: number; M: number; L: number }
  today: { date: string; hit: number; total: number; over: number; under: number; points: number; ids: string[] }
  week: { hitRate: number | null; pointsPerDay: number | null }
  rules: string[]
  malformed: number
}

declare module 'claude-code' {
  interface PluginState {
    znf: {
      ledger: Shaped<Ledger | null>
      ledgerError: Shaped<string | null>
      // the last `zenify nonstop stats` answer, or why it failed
      stats: Shaped<{ data?: NonstopStats; error?: string } | null>
      // PR url -> one poll's label, Zenie state, base branch and review threads (undefined: being
      // fetched; null: the query failed)
      prInfo: Shaped<
        Record<
          string,
          {
            label: string
            zenie: string
            base?: string
            threads?: { open: number; total: number } | null
          }
        >
      >
      // url -> epoch ms until which its merge button is armed
      confirming: Shaped<Record<string, number>>
      // urls whose merge is running
      merging: Shaped<string[]>
      // spinner frame index
      frame: Shaped<number>
      // ids of the expanded task lines
      expanded: Shaped<string[]>
      // subagent id -> the nonstop task its runs are logged under (durations.log)
      runTags: Shaped<Record<string, { id: string; kind: 'work' | 'review' }>>
      // worktree slug -> nonstop task id, from the worker briefs
      slugTasks: Shaped<Record<string, string>>
    }
  }
}
