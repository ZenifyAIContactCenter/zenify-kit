import { expect, test } from 'claude-code/testing'
import { parseLedger } from './ledger'

const FIXTURE = [
  '## 2026-10-27',
  't1 · a · b · - · S · znf:fix · w1 · running · -',
  '',
  '## 2026-10-28',
  't2 · kit · user · - · M · znf:cook · w2 · running · -',
  't3 · be · issue · Z/be#1 · S · znf:fix · w3 · blocked · cần chọn A/B',
  't4 · web · pr · - · S · znf:fix · - · deferred until 2026-10-30 · -',
  'Needs you: t9 — duyệt spec?',
  'Ruling: plan approved',
  '| a | b |',
  '|---|---|',
].join('\n')

test('parseLedger reads only the last dated section', async () => {
  const l = parseLedger(FIXTURE)
  expect(l.date).toBe('2026-10-28')
  expect(l.tasks.map(t => t.id)).toEqual(['t2', 't3', 't4'])
  expect(l.tasks[2].status).toBe('deferred')
  expect(l.needs).toEqual([
    { id: 't9', question: 'duyệt spec?' },
    { id: 't3', question: 'cần chọn A/B' },
  ])
  expect(l.malformed.length).toBe(2)
})

test('parseLedger on empty text', async () => {
  expect(parseLedger('')).toEqual({ date: null, tasks: [], needs: [], malformed: [] })
})

const REWRITTEN = [
  '## 2026-10-28',
  'k1 · kit · backlog · Z/kit#10 · S · znf:fix · w1 · done · https://github.com/zenify/kit/pull/11',
  'k2 · kit · user · - · M · znf:cook · w2 · running · -',
  'k3 · be · issue · Z/be#5 · S · znf:fix · w3 · blocked · pick A or B',
  'k4 · web · pr · - · S · znf:fix · - · deferred until 2026-10-30 · -',
  'k5 · hub · backlog · - · L · znf:cook · - · carry-over · -',
  'Needs you: k2 — approve the spec?',
  'Ruling: plan approved',
  'Waiting to merge: https://github.com/zenify/kit/pull/11',
  'Skipped: k6 — duplicate of k1',
].join('\n')

test('parseLedger reads a table section rewritten per ledger-format', async () => {
  const l = parseLedger(REWRITTEN)
  expect(l.tasks.length).toBe(5)
  expect(l.malformed.length).toBe(0)
})
