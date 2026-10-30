import { expect, test } from 'claude-code/testing'
import {
  bindLine,
  cwdLine,
  isEndCommand,
  isStartCommand,
  kindOf,
  parseStartId,
  phaseFor,
  phaseLine,
  stdoutOf,
  turnLine,
  unbindLine,
  waitLine,
} from './track'

const ISO = '2026-10-30T09:00:00.000Z'

test('FR-4.1: parseStartId reads only an exact track: <id> line', () => {
  expect(parseStartId('track: cook-261030-a1b2\n')).toBe('cook-261030-a1b2')
  expect(parseStartId('note\ntrack: hotfix-261030-00ff\r\n')).toBe('hotfix-261030-00ff')
  expect(parseStartId('track: fix-261030-beef')).toBe('fix-261030-beef')
})

test('SC-14: parseStartId binds nothing for track: off or a strange stdout', () => {
  expect(parseStartId('track: off\n')).toBeUndefined()
  expect(parseStartId('')).toBeUndefined()
  expect(parseStartId('track: cook-261030-A1B2')).toBeUndefined()
  expect(parseStartId('track: cook-261030-a1b2 extra')).toBeUndefined()
  expect(parseStartId('  track: cook-261030-a1b2')).toBeUndefined()
  expect(parseStartId('track: deploy-261030-a1b2')).toBeUndefined()
  expect(parseStartId('track: cook-2610300-a1b2')).toBeUndefined()
})

test('FR-4.1: stdoutOf reads result.stdout when it is a string, else text', () => {
  expect(stdoutOf({ result: { stdout: 'track: cook-261030-a1b2\n', stderr: '' }, text: 'other' })).toBe('track: cook-261030-a1b2\n')
  expect(stdoutOf({ ref: 'r', result: 'opaque', text: 'track: fix-261030-0000' })).toBe('track: fix-261030-0000')
  expect(stdoutOf({ result: { stdout: 3 }, text: 'track: fix-261030-0000' })).toBe('track: fix-261030-0000')
  expect(stdoutOf({ deny: 'no' })).toBe('')
  expect(stdoutOf(undefined)).toBe('')
})

test('isStartCommand and isEndCommand recognise the track commands', () => {
  expect(isStartCommand('zenify track start --kind cook --repo kit --est -')).toBe(true)
  expect(isStartCommand('cd /w && zenify track start --kind fix --repo be --est S')).toBe(true)
  expect(isStartCommand('zenify track est cook-261030-a1b2 S')).toBe(false)
  expect(isEndCommand('zenify track end cook-261030-a1b2 --pr https://github.com/Z/k/pull/1 --rounds 1')).toBe('cook-261030-a1b2')
  expect(isEndCommand('cd /w && zenify track end "fix-261030-00aa" --rollback --rounds 0')).toBe('fix-261030-00aa')
  expect(isEndCommand('zenify track end $ID --rounds 1')).toBeUndefined()
  expect(isEndCommand('zenify track ship cook-261030-a1b2 --outcome shipped --findings - --rounds 1')).toBeUndefined()
})

test('kindOf reads the kind from the id', () => {
  expect(kindOf('cook-261030-a1b2')).toBe('cook')
  expect(kindOf('fix-261030-a1b2')).toBe('fix')
  expect(kindOf('hotfix-261030-a1b2')).toBe('hotfix')
})

test('FR-8.2: phaseFor maps cook markers, with or without the znf: prefix', () => {
  expect(phaseFor('cook', 'znf:scout')).toBe('scout')
  expect(phaseFor('cook', 'scout')).toBe('scout')
  expect(phaseFor('cook', 'znf:writing-plans')).toBe('plan')
  expect(phaseFor('cook', 'subagent-driven-development')).toBe('implement')
  expect(phaseFor('cook', 'znf:ship')).toBe('ship')
  expect(phaseFor('cook', 'znf:review')).toBeUndefined()
  expect(phaseFor('cook', 'other:scout')).toBeUndefined()
})

test('FR-8.2: phaseFor maps fix and hotfix markers; other skills change nothing', () => {
  for (const kind of ['fix', 'hotfix'] as const) {
    expect(phaseFor(kind, 'scout')).toBe('fix')
    expect(phaseFor(kind, 'znf:scout')).toBe('fix')
    expect(phaseFor(kind, 'znf:ship')).toBe('ship')
    expect(phaseFor(kind, 'znf:writing-plans')).toBeUndefined()
    expect(phaseFor(kind, 'znf:subagent-driven-development')).toBeUndefined()
  }
})

test('the session log lines follow the contract', () => {
  expect(cwdLine(ISO, '/w/app')).toBe(`${ISO} · cwd · /w/app`)
  expect(bindLine(ISO, 'cook-261030-a1b2')).toBe(`${ISO} · bind · cook-261030-a1b2`)
  expect(unbindLine(ISO, 'cook-261030-a1b2')).toBe(`${ISO} · unbind · cook-261030-a1b2`)
  expect(waitLine(ISO, 'cook-261030-a1b2', 40000, 'permission')).toBe(`${ISO} · wait · cook-261030-a1b2 · 40000 · permission`)
  expect(phaseLine(ISO, 'fix-261030-a1b2', 'fix')).toBe(`${ISO} · phase · fix-261030-a1b2 · fix`)
})

test('FR-4.8: turnLine carries the usage, or five dashes without it', () => {
  const usage = { model: 'claude-opus-5-5', input_tokens: 10, output_tokens: 20, cache_read_input_tokens: 300, cache_creation_input_tokens: 4 }
  expect(turnLine(ISO, 'cook-261030-a1b2', 30000, 'main', usage)).toBe(
    `${ISO} · turn · cook-261030-a1b2 · 30000 · main · claude-opus-5-5 · 10 · 20 · 300 · 4`,
  )
  expect(turnLine(ISO, 'cook-261030-a1b2', 1200.6, 'sub', undefined)).toBe(`${ISO} · turn · cook-261030-a1b2 · 1201 · sub · - · - · - · - · -`)
})
