// FR-4, FR-7.3, FR-8.2: the pure half of the zenify track hook — which Bash call binds or ends
// an id, which Skill call changes the phase, and the sessions/<id>.log lines it writes.

export type Kind = 'cook' | 'fix' | 'hotfix'
export type Phase = 'design' | 'diagnose' | 'scout' | 'plan' | 'implement' | 'fix' | 'ship'
export type WaitKind = 'ask' | 'permission' | 'idle'
export type Usage = {
  model: string
  input_tokens: number
  output_tokens: number
  cache_read_input_tokens: number
  cache_creation_input_tokens: number
}

const ID = '(?:cook|fix|hotfix)-\\d{6}-[0-9a-f]{4}'
const START_LINE = new RegExp(`^track: (${ID})$`)
const ENDED_LINE = new RegExp(`^track: ended (${ID})$`)

// FR-4.1: only a line that is exactly `track: <id>` binds; `track: off` never does.
export function parseStartId(stdout: string): string | undefined {
  for (const line of stdout.split('\n')) {
    const m = START_LINE.exec(line.replace(/\r$/, ''))
    if (m) return m[1]
  }
  return undefined
}

// FR-4.1: core answers `{ ref, result, text }`; Bash's result may carry `stdout`, else read `text`.
export function stdoutOf(answer: unknown): string {
  if (!answer || typeof answer !== 'object') return ''
  const { result, text } = answer as { result?: unknown; text?: unknown }
  const stdout = result && typeof result === 'object' ? (result as { stdout?: unknown }).stdout : undefined
  if (typeof stdout === 'string') return stdout
  return typeof text === 'string' ? text : ''
}

export const isStartCommand = (command: string): boolean => /\bzenify\s+track\s+start\b/.test(command)

// The id a successful `zenify track end` closed: only its `track: ended <id>` stdout line
// counts, so a rejected end, or a script that merely contains the command, unbinds nothing.
export function parseEndedId(stdout: string): string | undefined {
  for (const line of stdout.split('\n')) {
    const m = ENDED_LINE.exec(line.replace(/\r$/, ''))
    if (m) return m[1]
  }
  return undefined
}

export const kindOf = (id: string): Kind => id.slice(0, id.indexOf('-')) as Kind

const PHASES: Record<Kind, Record<string, Phase>> = {
  cook: { scout: 'scout', 'writing-plans': 'plan', 'subagent-driven-development': 'implement', ship: 'ship' },
  fix: { scout: 'fix', ship: 'ship' },
  hotfix: { scout: 'fix', ship: 'ship' },
}

// FR-8.2: the skill name matches with or without `znf:` (hotfix calls `Skill(scout)`).
export function phaseFor(kind: Kind, skill: string): Phase | undefined {
  const name = skill.startsWith('znf:') ? skill.slice(4) : skill
  return Object.prototype.hasOwnProperty.call(PHASES[kind], name) ? PHASES[kind][name] : undefined
}

const join = (...fields: (string | number)[]) => fields.join(' · ')
const ms = (n: number) => Math.max(0, Math.round(n))

export const cwdLine = (iso: string, path: string) => join(iso, 'cwd', path)
export const bindLine = (iso: string, id: string) => join(iso, 'bind', id)
export const unbindLine = (iso: string, id: string) => join(iso, 'unbind', id)
export const phaseLine = (iso: string, id: string, phase: Phase) => join(iso, 'phase', id, phase)
export const waitLine = (iso: string, id: string, n: number, kind: WaitKind) => join(iso, 'wait', id, ms(n), kind)

// FR-4.2/FR-4.8: no usage (an interrupt, an API error) leaves the five token fields `-`.
export function turnLine(iso: string, id: string, n: number, loop: 'main' | 'sub', usage: Usage | undefined): string {
  const tokens = usage
    ? [usage.model, usage.input_tokens, usage.output_tokens, usage.cache_read_input_tokens, usage.cache_creation_input_tokens]
    : ['-', '-', '-', '-', '-']
  return join(iso, 'turn', id, ms(n), loop, ...tokens)
}
