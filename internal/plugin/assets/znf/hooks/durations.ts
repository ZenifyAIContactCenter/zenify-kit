// FR-6.1: which nonstop task a subagent run belongs to, and the durations.log line it writes.

// A worker brief: `Task:       <id> — ...` and `worktree slug: <slug>`.
export function taskFromPrompt(prompt: string): { id?: string; slug?: string } {
  const id = /^Task:\s+(\S+)/m.exec(prompt)?.[1]
  if (!id) return {}
  const slug = /worktree slug:\s*(\S+)/.exec(prompt)?.[1]
  return slug ? { id, slug } : { id }
}

// A reviewer prompt names the worktree path: `.../.worktrees/<slug>`, maybe quoted.
export function slugFromPrompt(prompt: string): string | undefined {
  return /\.worktrees\/([^/\s"'`]+)/.exec(prompt)?.[1]
}

export function logLine(iso: string, id: string, kind: 'work' | 'review', ms: number): string {
  return `${iso} · ${id} · ${kind} · ${ms}`
}
