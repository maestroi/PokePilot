import assert from 'node:assert/strict'
import test from 'node:test'
import {
  bugGroupRuns,
  deleteRuns,
  eligibleRuns,
  lineageBlockedRuns,
  matchingRunsForGroups,
  normalizeFailureDetail
} from '../src/operator/runCleanup.ts'

test('cleanup excludes resume-protected runs but reports them separately', () => {
  const runs = [
    { run_id: 'old', status: 'done', ended_at: 100 },
    { run_id: 'protected', status: 'done', ended_at: 90, resume_protected: true },
    { run_id: 'recent', status: 'done', ended_at: 990 }
  ]
  assert.deepEqual(eligibleRuns(runs, 1000, 100).map((run) => run.run_id), ['old'])
  assert.deepEqual(lineageBlockedRuns(runs, 1000, 100).map((run) => run.run_id), ['protected'])
})

test('failure cleanup normalizes volatile ids and addresses', () => {
  const pattern = normalizeFailureDetail('skill Traverse failed at 0xDEAD frame 12345')
  const runs = [
    { run_id: 'a', status: 'done', reason: 'error', ended_at: 1, detail: 'skill Traverse failed at 0xBEEF frame 99999' },
    { run_id: 'b', status: 'done', reason: 'error', ended_at: 2, detail: 'something else' }
  ]
  assert.deepEqual(bugGroupRuns(runs, pattern).map((run) => run.run_id), ['a'])
})

// A composed objective-failure pattern is
// normalizeDetail(objective + " | " + error)[:128] + " | map=xx" — longer
// than any normalizeFailureDetail(run.detail) — so an exact comparison
// selected no runs. The shared failure-id marker is the identity that makes
// the Failures "Delete selected" action select them again.
test('failure cleanup matches a composed objective-failure group by failure-id', () => {
  const marker = 'hfhmhabpkfeiljbjdkcciamajgiehelcallfhlonokpeapokbahfnafeegkcipgl'
  const groups = [
    {
      key: 'hfhmhabpkfeiljbjd',
      count: 11,
      pattern: `recover from repeated objective failures | failure recovery budget was exhausted: failure-id:${marker.slice(0, 28)} | map=06`,
      example: `recover from repeated objective failures: failure recovery budget was exhausted: failure-id:${marker} progress blocked route_prerequisite_missing`
    }
  ]
  const runs = [
    { run_id: 'marker', status: 'done', reason: 'failed', ended_at: 20, detail: `failure-id:${marker} progress blocked` },
    { run_id: 'other-marker', status: 'done', reason: 'failed', ended_at: 10, detail: `failure-id:${'a'.repeat(64)} progress blocked` },
    { run_id: 'clean-finish', status: 'done', reason: 'done', ended_at: 30, detail: `failure-id:${marker} progress blocked` },
    { run_id: 'protected', status: 'done', reason: 'failed', resume_protected: true, ended_at: 40, detail: `failure-id:${marker} progress blocked` }
  ]

  assert.deepEqual(matchingRunsForGroups(runs, groups).map((run) => run.run_id), ['marker'])
})

test('bulk deletion treats active resume-lineage rejection as kept, not failed', async () => {
  const result = await deleteRuns(['ok', 'keep', 'bad'], async (id) => {
    if (id === 'keep') throw new Error('run is still required by an active resume lineage')
    if (id === 'bad') throw new Error('storage unavailable')
  }, 2)

  assert.deepEqual(result.deletedIds, ['ok'])
  assert.deepEqual(result.skippedIds, ['keep'])
  assert.deepEqual(result.failures, [{ id: 'bad', error: 'storage unavailable' }])
})
