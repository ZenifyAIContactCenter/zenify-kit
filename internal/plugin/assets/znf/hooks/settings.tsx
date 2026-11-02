// FR-16: the kit's settings pane, toggled by /znf-settings. Every press runs one `zenify` command
// through $.process.run (no model turn) and reads its output back. It never reads an env file.
import { atom, read, update } from 'claude-code'
import type { EngineInterface as $, Register } from 'claude-code'

import type { SettingsRun, SettingsRuns, SettingsTab } from '../types'

const PANE = { id: 'znf-settings', title: 'znf settings', columns: 44 } as const
const RUN_TIMEOUT_MS = 20000
// FR-16.3: the $.store key; undefined counts as on.
const AUTO_OPEN_KEY = 'trackAutoOpen'

// The Vietnamese UI copy the spec fixes; every other string stays English.
const COPY = {
  opened: 'đã mở pane settings',
  closed: 'đã đóng pane settings',
  loading: 'đang chạy…',
  autoOpen: 'Tự mở pane track khi cook/fix/hotfix/ship: ',
  reviewerNote: 'có hiệu lực ở session mới',
  envHint: 'sửa: ! zenify env set KEY (nhập ẩn trong terminal)',
  toast: 'lỗi settings: ',
} as const

const TABS: { id: SettingsTab; label: string }[] = [
  { id: 'features', label: 'Features' },
  { id: 'track', label: 'Track' },
  { id: 'reviewer', label: 'Reviewer' },
  { id: 'env', label: 'Env' },
]
// The command each output tab shows (FR-16.2/16.4/16.5).
const READ_ARGV: Record<Exclude<SettingsTab, 'track'>, string[]> = {
  features: ['zenify', 'feature'],
  reviewer: ['zenify', 'reviewer'],
  env: ['zenify', 'env', 'list'],
}
const REVIEWERS = ['agent', 'codex'] as const
const FEATURE_LINE = /^(\S+) (on|off)$/

const tab = atom({ plugin: 'znf', key: 'settingsTab' } as const, 'features' as SettingsTab, { shape: 'settings-1' })
const runs = atom({ plugin: 'znf', key: 'settingsRuns' } as const, {} as SettingsRuns, { shape: 'settings-1' })
// A mirror of the stored trackAutoOpen for drawing: $.store reads do not redraw.
const autoOpenShown = atom({ plugin: 'znf', key: 'settingsAutoOpen' } as const, true, { shape: 'settings-1' })

const message = (err: unknown) => (err instanceof Error ? err.message : String(err))
const firstLine = (s: string) => s.trim().split('\n')[0]

// FR-16.3: whether a cook/fix/hotfix/ship prompt opens the track pane.
export async function isAutoOpen($: $): Promise<boolean> {
  return (await $.store.get(AUTO_OPEN_KEY)) !== false
}

// Runs one argv; a non-zero exit or a call that cannot start is the error, never swallowed.
async function run($: $, argv: string[]): Promise<SettingsRun> {
  try {
    const out = await $.process.run(argv, { timeoutMs: RUN_TIMEOUT_MS })
    if (out.exitCode !== 0) return { text: out.stdout, error: firstLine(out.stderr) || `${argv.join(' ')}: exit ${out.exitCode}` }
    return { text: out.stdout }
  } catch (err) {
    return { error: firstLine(message(err)) }
  }
}

async function load($: $, id: SettingsTab): Promise<void> {
  if (id === 'track') {
    const on = await isAutoOpen($)
    await update($, autoOpenShown, () => on)
    return
  }
  const out = await run($, READ_ARGV[id])
  await update($, runs, r => ({ ...r, [id]: out }))
}

async function selectTab($: $, id: SettingsTab): Promise<void> {
  await update($, tab, () => id)
  await load($, id)
}

// Runs a write command, then re-reads the tab; a failed write keeps the last list and adds its ⚠.
async function write($: $, id: Exclude<SettingsTab, 'track'>, argv: string[]): Promise<void> {
  const out = await run($, argv)
  if (out.error !== undefined) {
    await update($, runs, r => ({ ...r, [id]: { text: r[id]?.text, error: out.error } }))
    return
  }
  await load($, id)
}

async function toggleAutoOpen($: $): Promise<void> {
  const next = !(await isAutoOpen($))
  await $.store.set(AUTO_OPEN_KEY, next)
  await update($, autoOpenShown, () => next)
}

// A press whose handler fails (a $.store or $.state error) is toasted, never dropped.
const press = ($: $, work: Promise<void>) => work.catch(err => $.ui.toast(`${COPY.toast}${message(err)}`))

const linesOf = (text: string | undefined) => (text ?? '').split('\n').filter(l => l.trim() !== '')

// FR-16.1: called from the module's one session.start hook (register.tsx).
export async function registerSettingsCommand($: $): Promise<void> {
  await $.command.register({ name: 'znf-settings', description: 'Toggle the zenify kit settings pane' })
}

export const registerSettings: Register = on => {
  on('command.run', { command: 'znf-settings' }, async $ => {
    if ((await $.ui.panes()).some(p => p.id === PANE.id)) {
      await $.ui.close({ id: PANE.id })
      return { text: COPY.closed }
    }
    await $.ui.open({ ...PANE })
    await load($, await read($, tab))
    return { text: COPY.opened }
  })

  on('ui.render', { component: 'Pane', requestId: PANE.id }, async ($, e) => {
    const { Box, Button, Text } = $.ui.resolve(e)
    const selected = await read($, tab)
    const all = await read($, runs)
    const out = selected === 'track' ? undefined : all[selected]
    const errorLine = out?.error !== undefined && <Text color="red" wrap="wrap">{`⚠ ${out.error}`}</Text>
    let body
    if (selected === 'track') {
      const isOn = await read($, autoOpenShown)
      body = (
        <Box flexDirection="row">
          <Text wrap="wrap">{`${COPY.autoOpen}${isOn ? 'on' : 'off'}`}</Text>
          <Button key="autoOpen" label={`→ ${isOn ? 'off' : 'on'}`} plain onPress={() => press($, toggleAutoOpen($))} />
        </Box>
      )
    } else if (!out) {
      body = <Text dimColor>{COPY.loading}</Text>
    } else if (selected === 'features') {
      body = linesOf(out.text).map(l => {
        const m = FEATURE_LINE.exec(l)
        if (!m) return <Text wrap="wrap">{l}</Text>
        const [, key, value] = m
        const flip = value === 'on' ? 'off' : 'on'
        return (
          <Box flexDirection="row">
            <Text>{l}</Text>
            <Text> </Text>
            <Button key={`feature:${key}`} label={`→ ${flip}`} plain onPress={() => press($, write($, 'features', ['zenify', 'feature', key, flip]))} />
          </Box>
        )
      })
    } else if (selected === 'reviewer') {
      body = [
        ...linesOf(out.text).map(l => <Text wrap="wrap">{l}</Text>),
        <Box flexDirection="row">
          {REVIEWERS.map((v, i) => [
            i > 0 && <Text> </Text>,
            <Button key={`reviewer:${v}`} label={v} plain onPress={() => press($, write($, 'reviewer', ['zenify', 'reviewer', 'set', v]))} />,
          ])}
        </Box>,
        <Text dimColor>{COPY.reviewerNote}</Text>,
      ]
    } else {
      body = [...linesOf(out.text).map(l => <Text wrap="wrap">{l}</Text>), <Text dimColor>{COPY.envHint}</Text>]
    }
    return (
      <Box flexDirection="column">
        <Box flexDirection="row">
          {TABS.map((t, i) => [
            i > 0 && <Text dimColor> │ </Text>,
            t.id === selected ? (
              <Text color="claude" bold>{`▸${t.label}`}</Text>
            ) : (
              <Button key={`tab:${t.id}`} label={t.label} plain dimColor onPress={() => press($, selectTab($, t.id))} />
            ),
          ])}
        </Box>
        {body}
        {errorLine}
      </Box>
    )
  })
}
