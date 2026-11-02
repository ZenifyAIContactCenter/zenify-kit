// FR-17: the kit's settings as /config rows (manifest userConfig). The rows mirror the real
// sources: features in .zenify-overlay.yaml (`zenify feature`), the reviewer in env.ZNF_REVIEWER
// (`zenify reviewer`). A change runs the writing command; session start pulls the sources back.
import type { EngineInterface as $, ConfigSetInput, ConfigValue, On, PluginOptions } from 'claude-code'

import { COPY } from './track-ui'

const RUN_TIMEOUT_MS = 20000
// Row field -> `zenify feature` key (FR-17.1).
const FEATURES: Record<string, string> = { track: 'track', githubIssues: 'github.issues' }
const REVIEWERS = ['agent', 'codex']
const FEATURE_LINE = /^(\S+) (on|off)$/

export type TrackPane = 'auto' | 'command' | 'off'

// FR-17.4: a value outside the options reads as unset, so the default `auto`.
export const trackPaneOf = (options: PluginOptions): TrackPane =>
  options.trackPane === 'command' || options.trackPane === 'off' ? options.trackPane : 'auto'

const message = (err: unknown) => (err instanceof Error ? err.message : String(err))

// Runs one argv; a non-zero exit or a call that cannot start throws with the stderr.
async function run($: $, argv: string[]): Promise<string> {
  const out = await $.process.run(argv, { timeoutMs: RUN_TIMEOUT_MS })
  if (out.exitCode !== 0) throw new Error(out.stderr.trim() || `${argv.join(' ')}: exit ${out.exitCode}`)
  return out.stdout
}

// The command a row change runs, or undefined for a row with no outside source (trackPane).
function writeArgv(field: string, value: unknown): string[] | undefined {
  const feature = FEATURES[field]
  if (feature) return ['zenify', 'feature', feature, value ? 'on' : 'off']
  if (field === 'reviewer') return ['zenify', 'reviewer', 'set', String(value)]
  return undefined
}

// The keys align is setting: they copy a source's value, so the config.set hook writes nothing
// back. A session skips this plugin's own hooks for its $.config.set; `claude plugin test` runs
// them and stamps no `origin`, so the key marks align's set. A change to the same key that names
// another origin (a person in /config, the bridge, another plugin) is still written.
const aligning = new Set<string>()
// The latest value of each znf key changed by anyone but align since the running sync began its
// reads: align leaves those keys, and puts the value back if its own set landed after it, so a
// stale read never overwrites a newer change.
const changed = new Map<string, ConfigValue>()

const isAlign = (e: ConfigSetInput) =>
  aligning.has(e.key) && (!e.origin || (e.origin.kind === 'plugin' && e.origin.name === 'znf'))

// FR-17.2: a znf row change runs its command first; a failure denies it, the row keeps its value.
export function registerConfig(on: On): void {
  on('config.set', async ($, e, next) => {
    if (!e.key.startsWith('znf.') || isAlign(e)) return next(e)
    const argv = writeArgv(e.key.slice(4), e.value)
    if (argv) {
      try {
        await run($, argv)
      } catch (err) {
        return { deny: message(err) }
      }
    }
    const res = await next(e)
    if (res.deny === undefined) changed.set(e.key, e.value)
    return res
  })
}

// Sets one row to its source's value when they differ; a refused set is an error.
async function align($: $, options: PluginOptions, field: string, value: boolean | string): Promise<void> {
  const key = `znf.${field}`
  if (options[field] === value || changed.has(key)) return
  aligning.add(key)
  try {
    const res = await $.config.set({ key, value })
    if (res.deny !== undefined) throw new Error(res.deny)
    // A change made while this set was in flight: its value wins. Still under `aligning`, so its
    // command, already run by that change, is not run again.
    if (changed.has(key)) {
      const again = await $.config.set({ key, value: changed.get(key)! })
      if (again.deny !== undefined) throw new Error(again.deny)
    }
  } finally {
    aligning.delete(key)
  }
}

// FR-17.3: pulls each row back to its source. `inWorkspace` false skips the features, which
// belong to a workspace. Each failure is a ⚠ toast and leaves its row alone.
export async function syncConfig($: $, options: PluginOptions, inWorkspace: boolean): Promise<void> {
  const toast = (err: unknown) => $.ui.toast(`${COPY.configToast}${message(err)}`)
  changed.clear()
  if (inWorkspace) {
    await (async () => {
      const on = new Map<string, boolean>()
      for (const l of (await run($, ['zenify', 'feature'])).split('\n')) {
        const m = FEATURE_LINE.exec(l.trim())
        if (m) on.set(m[1], m[2] === 'on')
      }
      for (const [field, key] of Object.entries(FEATURES)) {
        if (!on.has(key)) throw new Error(`zenify feature: no ${key} line`)
        await align($, options, field, on.get(key)!)
      }
    })().catch(toast)
  }
  await (async () => {
    const engine = (await run($, ['zenify', 'reviewer'])).trim().split(/\s/)[0]
    if (!REVIEWERS.includes(engine)) throw new Error(`zenify reviewer: unexpected output ${JSON.stringify(engine)}`)
    await align($, options, 'reviewer', engine)
  })().catch(toast)
}
