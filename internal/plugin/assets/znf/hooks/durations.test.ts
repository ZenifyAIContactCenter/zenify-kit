import { expect, test } from 'claude-code/testing'
import { logLine, slugFromPrompt, taskFromPrompt } from './durations'

const BRIEF = [
  'Task:       t14 — fix login redirect, source issue, size S',
  'Repo:       /w/repos/app · worktree slug: fix-login (git fetch origin; wt new fix-login --type fix --base main)',
  'Skill:      run Skill(znf:fix) for this task',
].join('\n')

test('taskFromPrompt reads the task id and the worktree slug of a worker brief', () => {
  expect(taskFromPrompt(BRIEF)).toEqual({ id: 't14', slug: 'fix-login' })
})

test('taskFromPrompt finds nothing in a prompt without a Task line', () => {
  expect(taskFromPrompt('Review the diff at /w/repos/app/.worktrees/fix-login')).toEqual({})
})

test('taskFromPrompt reads the id when the brief has no slug', () => {
  expect(taskFromPrompt('Task: t2 — spike')).toEqual({ id: 't2' })
})

test('slugFromPrompt reads the .worktrees path segment of a reviewer prompt', () => {
  expect(slugFromPrompt('WT=/w/repos/app/.worktrees/fix-login\nReview the diff.')).toBe('fix-login')
  expect(slugFromPrompt('Read /w/repos/app/.worktrees/fix-login/src/a.ts')).toBe('fix-login')
  expect(slugFromPrompt('Review the diff in /w/repos/app')).toBeUndefined()
})

test('slugFromPrompt stops at a quote or backtick around the path', () => {
  expect(slugFromPrompt('WT="/w/repos/app/.worktrees/fix-login"\nReview.')).toBe('fix-login')
  expect(slugFromPrompt("WT='/w/repos/app/.worktrees/fix-login'")).toBe('fix-login')
  expect(slugFromPrompt('Read `/w/repos/app/.worktrees/fix-login`.')).toBe('fix-login')
})

test('logLine joins time, task id, kind and ms with middle dots', () => {
  expect(logLine('2026-10-28T09:00:00.000Z', 't14', 'work', 245000)).toBe('2026-10-28T09:00:00.000Z · t14 · work · 245000')
})
