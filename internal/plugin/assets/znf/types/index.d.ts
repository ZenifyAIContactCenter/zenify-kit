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

declare module 'claude-code' {
  interface PluginState {
    znf: {
      ledger: Shaped<Ledger | null>
      ledgerError: Shaped<string | null>
      // PR url -> label from prLabel
      prs: Shaped<Record<string, string>>
      // PR url -> base branch
      bases: Shaped<Record<string, string>>
      // url -> epoch ms until which its merge button is armed
      confirming: Shaped<Record<string, number>>
      // urls whose merge is running
      merging: Shaped<string[]>
      // spinner frame index
      frame: Shaped<number>
      // ids of the expanded task lines
      expanded: Shaped<string[]>
    }
  }
}
