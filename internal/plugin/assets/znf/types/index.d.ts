export type LedgerTask = {
  id: string
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
    }
  }
}
