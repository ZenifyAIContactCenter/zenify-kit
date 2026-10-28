import { expect, test } from 'claude-code/testing'
import type { LedgerTask } from './ledger'
import { isMergeable, isReadyToMerge, issueUrl, notReadyReason, mergeArgv, mergeCommand, prBase, threadCount, threadsArgv, zenieState, prLabel, prRef, prUrls } from './pr'

const view = (o: object) => ({ exitCode: 0, stdout: JSON.stringify(o), stderr: '' })
const run = (name: string, conclusion: string, status = 'COMPLETED') => ({
  __typename: 'CheckRun', name, conclusion, status,
})

test('prLabel follows the FR-2.3 order', async () => {
  const cases = [
    view({ state: 'MERGED', mergeStateStatus: 'CLEAN', statusCheckRollup: [] }),
    view({ state: 'CLOSED', mergeStateStatus: 'CLEAN', statusCheckRollup: [] }),
    view({ state: 'OPEN', mergeStateStatus: 'BLOCKED', statusCheckRollup: [run('build', 'SUCCESS'), run('lint', 'FAILURE')] }),
    view({ state: 'OPEN', mergeStateStatus: 'BLOCKED', statusCheckRollup: [run('build', 'SUCCESS'), run('test', '', 'IN_PROGRESS')] }),
    view({ state: 'OPEN', mergeStateStatus: 'CLEAN', statusCheckRollup: [run('build', 'SUCCESS')] }),
    view({ state: 'OPEN', mergeStateStatus: 'UNSTABLE', statusCheckRollup: [run('build', 'SUCCESS')] }),
    { exitCode: 1, stdout: '', stderr: 'HTTP 404\nnot found' },
  ]
  expect(cases.map(prLabel)).toEqual(['MERGED', 'CLOSED', 'CI đỏ (lint)', 'CI chạy', 'CLEAN', 'UNSTABLE', 'gh lỗi: HTTP 404'])
})

test('prLabel handles StatusContext, BLOCKED, other states and bad JSON', async () => {
  expect(prLabel(view({ state: 'OPEN', mergeStateStatus: 'BLOCKED', statusCheckRollup: [{ __typename: 'StatusContext', context: 'ci/jenkins', state: 'FAILURE' }] }))).toBe('CI đỏ (ci/jenkins)')
  expect(prLabel(view({ state: 'OPEN', mergeStateStatus: 'BLOCKED', statusCheckRollup: [{ __typename: 'StatusContext', context: 'ci/x', state: 'PENDING' }] }))).toBe('CI chạy')
  expect(prLabel(view({ state: 'OPEN', mergeStateStatus: 'BLOCKED', statusCheckRollup: [] }))).toBe('BLOCKED')
  expect(prLabel(view({ state: 'OPEN', mergeStateStatus: 'BEHIND', statusCheckRollup: [] }))).toBe('BEHIND')
  expect(prLabel({ exitCode: 0, stdout: 'not json', stderr: '' }).startsWith('gh lỗi: ')).toBe(true)
})

test('isMergeable, mergeCommand, prRef', async () => {
  expect(isMergeable('CLEAN')).toBe(true)
  expect(isMergeable('UNSTABLE')).toBe(true)
  expect(isMergeable('BLOCKED')).toBe(false)
  expect(mergeCommand('https://github.com/Z/kit/pull/238')).toBe('gh pr merge -R Z/kit 238 --merge')
  expect(prRef('https://github.com/Z/kit/pull/238')).toBe('Z/kit#238')
})

test('prUrls dedupes and keeps order', async () => {
  const t = (evidence: string) => ({ evidence }) as LedgerTask
  expect(prUrls([
    t('PR https://github.com/Z/kit/pull/238 ok'),
    t('https://github.com/Z/kit/pull/238'),
    t('hub#1075 and https://github.com/Z/be/pull/7'),
  ])).toEqual(['https://github.com/Z/kit/pull/238', 'https://github.com/Z/be/pull/7'])
})

test('issueUrl turns owner/repo#N into an issue URL and rejects the rest', () => {
  expect(issueUrl('Z/be#12')).toBe('https://github.com/Z/be/issues/12')
  expect(issueUrl('-')).toBeUndefined()
  expect(issueUrl('#12')).toBeUndefined()
})

test('mergeArgv builds the gh pr merge argv for a PR url', () => {
  expect(mergeArgv('https://github.com/Z/kit/pull/238')).toEqual(['gh', 'pr', 'merge', '-R', 'Z/kit', '238', '--merge'])
})

test('prBase reads baseRefName and is undefined on a failure or bad JSON', () => {
  expect(prBase(view({ state: 'OPEN', baseRefName: 'staging' }))).toBe('staging')
  expect(prBase({ exitCode: 1, stdout: '', stderr: 'x' })).toBeUndefined()
  expect(prBase({ exitCode: 0, stdout: 'nope', stderr: '' })).toBeUndefined()
})

const zenie = (check: object | null) =>
  view({ statusCheckRollup: check ? [{ name: 'lint', conclusion: 'SUCCESS', status: 'COMPLETED' }, check] : [] })

test('zenieState reads the Zenie Review check run', () => {
  expect(zenieState(zenie({ name: 'Zenie Review', status: 'IN_PROGRESS', conclusion: '' }))).toBe('đang chạy')
  expect(zenieState(zenie({ name: 'Zenie Review', status: 'COMPLETED', conclusion: 'SUCCESS' }))).toBe('pass')
  expect(zenieState(zenie({ name: 'Zenie Review', status: 'COMPLETED', conclusion: 'FAILURE' }))).toBe('fail')
  expect(zenieState(zenie(null))).toBe('—')
  expect(zenieState({ exitCode: 1, stdout: '', stderr: 'x' })).toBe('—')
  expect(zenieState({ exitCode: 0, stdout: 'nope', stderr: '' })).toBe('—')
})

const graphql = (nodes: unknown) =>
  view({ data: { repository: { pullRequest: { reviewThreads: { nodes } } } } })

test('threadCount counts open and total review threads', () => {
  expect(threadCount(graphql([{ isResolved: true }, { isResolved: false }, { isResolved: false }]))).toEqual({ open: 2, total: 3 })
  expect(threadCount(graphql([]))).toEqual({ open: 0, total: 0 })
  expect(threadCount({ exitCode: 1, stdout: '', stderr: 'x' })).toBeUndefined()
  expect(threadCount({ exitCode: 0, stdout: 'nope', stderr: '' })).toBeUndefined()
  expect(threadCount(view({ data: {} }))).toBeUndefined()
})

test('threadsArgv queries the review threads of the PR', () => {
  expect(threadsArgv('https://github.com/Z/kit/pull/238')).toEqual([
    'gh',
    'api',
    'graphql',
    '-f',
    'query=query{repository(owner:"Z",name:"kit"){pullRequest(number:238){reviewThreads(first:100){pageInfo{hasNextPage}nodes{isResolved}}}}}',
  ])
})

test('isReadyToMerge needs a mergeable label, Zenie pass or none, and no open thread', () => {
  expect(isReadyToMerge('CLEAN', 'pass', { open: 0, total: 3 })).toBe(true)
  expect(isReadyToMerge('UNSTABLE', 'pass', { open: 0, total: 0 })).toBe(true)
  expect(isReadyToMerge('CLEAN', '—', { open: 0, total: 0 })).toBe(false)
  expect(isReadyToMerge('CLEAN', 'pass', { open: 1, total: 3 })).toBe(false)
  expect(isReadyToMerge('CLEAN', 'đang chạy', { open: 0, total: 0 })).toBe(false)
  expect(isReadyToMerge('CLEAN', 'fail', { open: 0, total: 0 })).toBe(false)
  expect(isReadyToMerge('CLEAN', 'pass', undefined)).toBe(false)
  expect(isReadyToMerge('BLOCKED', 'pass', { open: 0, total: 0 })).toBe(false)
})

test('threadsArgv refuses an owner or repo outside [A-Za-z0-9_.-]', () => {
  expect(threadsArgv('https://github.com/Z/ki"t/pull/5')).toBeUndefined()
  expect(threadsArgv('https://github.com/Z}/kit/pull/5')).toBeUndefined()
  expect(threadsArgv('https://github.com/my-org/my.repo_1/pull/5')).toBeDefined()
})

test('threadCount is undefined when more than one page of threads exists', () => {
  const paged = view({
    data: { repository: { pullRequest: { reviewThreads: { pageInfo: { hasNextPage: true }, nodes: [{ isResolved: true }] } } } },
  })
  expect(threadCount(paged)).toBeUndefined()
})

test('notReadyReason names the first failing condition', () => {
  expect(notReadyReason('CLEAN', 'pass', { open: 0, total: 1 })).toBeUndefined()
  expect(notReadyReason('BLOCKED', 'pass', { open: 0, total: 1 })).toBe('BLOCKED')
  expect(notReadyReason('CLEAN', 'fail', { open: 0, total: 1 })).toBe('Zenie fail')
  expect(notReadyReason('CLEAN', 'pass', { open: 2, total: 5 })).toBe('còn 2 thread mở')
  expect(notReadyReason('CLEAN', 'pass', undefined)).toBe('thread ?')
})
