import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { DashboardRun, TriageGroup } from '../shared/api/types'
import {
  AGE_OPTIONS,
  DEFAULT_AGE_SECONDS,
  FAILURE_PATTERN_CAP,
  ageDescription,
  bugGroupRuns,
  cleanupResultText,
  deleteRuns,
  eligibleRuns,
  groupPattern,
  isLiveLineageError,
  isResolvedGroup,
  matchingRunsForGroups,
  normalizeFailureDetail
} from './runCleanup.ts'

function run(partial: Partial<DashboardRun> & Pick<DashboardRun, 'run_id'>): DashboardRun {
  return {
    status: 'done',
    reason: 'error',
    ...partial
  }
}

test('normalizeFailureDetail matches pokewall triage normalization', () => {
  assert.equal(
    normalizeFailureDetail('still on map 0x0c at (10,35) after 180 frames'),
    'still on map <hex> at (<n>,<n>) after <n> frames'
  )
  assert.equal(
    normalizeFailureDetail('still on map 0x21 at (4,22) after 240 frames'),
    'still on map <hex> at (<n>,<n>) after <n> frames'
  )
  assert.equal(normalizeFailureDetail('x'.repeat(FAILURE_PATTERN_CAP + 20)).length, FAILURE_PATTERN_CAP)
})

test('bugGroupRuns selects every finished error/lost run in one triage pattern', () => {
  const pattern = normalizeFailureDetail('still on map 0x0c at (10,35)')
  const runs = [
    run({ run_id: 'new-error', detail: 'still on map 0x21 at (4,22)', ended_at: 30 }),
    run({ run_id: 'old-lost', reason: 'lost', detail: 'still on map 0x0c at (10,35)', ended_at: 10 }),
    run({ run_id: 'other-bug', detail: 'agent: blacked out', ended_at: 20 }),
    run({ run_id: 'success', reason: 'done', detail: 'still on map 0x21 at (4,22)', ended_at: 40 }),
    run({ run_id: 'active', status: 'running', detail: 'still on map 0x21 at (4,22)', ended_at: 0 })
  ]

  assert.deepEqual(
    bugGroupRuns(runs, pattern).map((item) => item.run_id),
    ['old-lost', 'new-error']
  )
  assert.deepEqual(bugGroupRuns(runs, ''), [])
})

test('matchingRunsForGroups unions several resolved issues without duplicating runs', () => {
  const groups: TriageGroup[] = [
    { key: 'stall', pattern: normalizeFailureDetail('still on map 0x0c at (10,35)'), count: 2 },
    { key: 'blackout', pattern: 'agent: blacked out', count: 1 }
  ]
  const runs = [
    run({ run_id: 'a', detail: 'still on map 0x0c at (10,35)', ended_at: 10 }),
    run({ run_id: 'b', detail: 'still on map 0x21 at (4,22)', ended_at: 20 }),
    run({ run_id: 'c', detail: 'agent: blacked out', ended_at: 30 })
  ]

  assert.deepEqual(
    matchingRunsForGroups(runs, groups).map((item) => item.run_id),
    ['a', 'b', 'c']
  )
})

// A composed objective-failure pattern is
// normalizeDetail(objective + " | " + error)[:128] + " | map=xx" — 137 chars,
// longer than any normalizeFailureDetail(run.detail) — so the old exact
// comparison selected nothing and "Delete selected" was a no-op on every
// resolved group the wall produced from objective_failures.
test('matchingRunsForGroups matches composed objective-failure groups by failure-id', () => {
  const marker = 'hfhmhabpkfeiljbjdkcciamajgiehelcallfhlonokpeapokbahfnafeegkcipgl'
  const groups: TriageGroup[] = [
    {
      key: 'hfhmhabpkfeiljbjd',
      count: 11,
      pattern: `recover from repeated objective failures | failure recovery budget was exhausted: failure-id:${marker.slice(0, 28)} | map=06`,
      example: `recover from repeated objective failures: failure recovery budget was exhausted: failure-id:${marker} progress blocked route_prerequisite_missing`,
      issue: { status: 'resolved', resolution: 'fixed' }
    }
  ]
  const runs = [
    run({ run_id: 'marker', reason: 'failed', detail: `failure-id:${marker} progress blocked route_prerequisite_missing`, ended_at: 20 }),
    run({ run_id: 'other-marker', reason: 'failed', detail: `failure-id:${'a'.repeat(64)} progress blocked no_route`, ended_at: 10 }),
    run({ run_id: 'clean-finish', reason: 'done', detail: `failure-id:${marker} progress blocked`, ended_at: 30 }),
    run({ run_id: 'protected', reason: 'failed', detail: `failure-id:${marker} progress blocked`, resume_protected: true, ended_at: 40 })
  ]

  assert.deepEqual(
    matchingRunsForGroups(runs, groups).map((item) => item.run_id),
    ['marker']
  )
})

test('bugGroupRuns still matches a legacy normalized pattern without a marker', () => {
  const pattern = normalizeFailureDetail('still on map 0x0c at (10,35)')
  const runs = [
    run({ run_id: 'legacy', reason: 'failed', detail: 'still on map 0x21 at (4,22)', ended_at: 10 })
  ]
  assert.deepEqual(bugGroupRuns(runs, pattern).map((item) => item.run_id), ['legacy'])
})

test('groupPattern falls back to normalized detail when the wall omitted pattern', () => {
  assert.equal(
    groupPattern({ pattern: '', detail: 'still on map 0x0c at (10,35)', key: 'x', count: 1 }),
    'still on map <hex> at (<n>,<n>)'
  )
})

test('isResolvedGroup mirrors the actionable issue lifecycle', () => {
  assert.equal(isResolvedGroup({ key: 'unlinked', count: 1 }), false)
  assert.equal(isResolvedGroup({ key: 'open', count: 1, issue: { status: 'open' } }), false)
  assert.equal(isResolvedGroup({ key: 'reopened-stale', count: 1, issue: { status: 'reopened', resolution: 'fixed' } }), false)
  assert.equal(isResolvedGroup({ key: 'resolved', count: 1, issue: { status: 'resolved' } }), true)
  assert.equal(isResolvedGroup({ key: 'closed', count: 1, issue: { status: 'closed' } }), true)
  assert.equal(isResolvedGroup({ key: 'completed', count: 1, issue: { status: 'completed' } }), true)
  assert.equal(isResolvedGroup({ key: 'fixed', count: 1, issue: { resolution: 'fixed' } }), true)
  assert.equal(isResolvedGroup({ key: 'not-planned', count: 1, issue: { status: 'closed', resolution: 'not_planned' } }), true)
})

test('eligibleRuns skips runs still required by a live resume lineage', () => {
  const now = 10_000
  const runs = [
    run({ run_id: 'old-free', ended_at: 1_000 }),
    run({ run_id: 'old-protected', ended_at: 1_000, resume_protected: true }),
    run({ run_id: 'fresh', ended_at: 9_000 })
  ]
  assert.deepEqual(
    eligibleRuns(runs, now, 3_600).map((item) => item.run_id),
    ['old-free']
  )
})

test('isLiveLineageError recognizes the wall 409', () => {
  assert.equal(isLiveLineageError('run is still required by an active resume lineage: run-1'), true)
  assert.equal(isLiveLineageError('s3 unavailable'), false)
})

test('eligibleRuns selects only finished runs older than the cutoff', () => {
  const now = 10_000
  const runs = [
    run({ run_id: 'old', ended_at: 1_000 }),
    run({ run_id: 'edge', ended_at: 6_400 }),
    run({ run_id: 'fresh', ended_at: 6_401 }),
    run({ run_id: 'active', status: 'running', ended_at: 1_000 }),
    run({ run_id: 'queued', status: 'queued', ended_at: 1_000 }),
    run({ run_id: 'no-end', ended_at: 0 })
  ]

  assert.deepEqual(
    eligibleRuns(runs, now, 3_600).map((item) => item.run_id),
    ['old', 'edge']
  )
})

test('eligibleRuns rejects invalid clocks and thresholds', () => {
  const runs = [run({ run_id: 'old', ended_at: 1 })]
  assert.deepEqual(eligibleRuns(runs, 0, 60), [])
  assert.deepEqual(eligibleRuns(runs, 100, 0), [])
  assert.deepEqual(eligibleRuns(null, 100, 60), [])
})

test('ageDescription names the cleanup presets', () => {
  assert.equal(DEFAULT_AGE_SECONDS, 7 * 24 * 60 * 60)
  assert.equal(AGE_OPTIONS.length, 5)
  assert.equal(ageDescription(60 * 60), '1 hour')
  assert.equal(ageDescription(6 * 60 * 60), '6 hours')
  assert.equal(ageDescription(24 * 60 * 60), '24 hours')
  assert.equal(ageDescription(7 * 24 * 60 * 60), '7 days')
  assert.equal(ageDescription(30 * 24 * 60 * 60), '30 days')
})

test('deleteRuns bounds concurrency and reports partial failures', async () => {
  const ids = ['a', 'b', 'c', 'd', 'e', 'f']
  let active = 0
  let maxActive = 0
  const progress: Array<{ completed: number, deleted: number, failed: number }> = []

  const result = await deleteRuns(ids, async (id) => {
    active++
    maxActive = Math.max(maxActive, active)
    await new Promise((resolve) => setTimeout(resolve, 5))
    active--
    if (id === 'c' || id === 'f') throw new Error(`cannot delete ${id}`)
  }, 3, (state) => progress.push({ ...state }))

  assert.ok(maxActive <= 3, `maximum concurrency was ${maxActive}`)
  assert.deepEqual([...result.deletedIds].sort(), ['a', 'b', 'd', 'e'])
  assert.deepEqual(result.failures.map((failure) => failure.id).sort(), ['c', 'f'])
  assert.equal(progress.at(-1)?.completed, ids.length)
  assert.equal(progress.at(-1)?.deleted, 4)
  assert.equal(progress.at(-1)?.failed, 2)
  assert.equal(progress.at(-1)?.skipped, 0)
  assert.match(cleanupResultText(result), /4 deleted · 2 failed/)
})

test('deleteRuns counts live-lineage 409s as skipped, not failed', async () => {
  const result = await deleteRuns(['keep', 'gone'], async (id) => {
    if (id === 'keep') throw new Error('run is still required by an active resume lineage: keep')
  }, 2)

  assert.deepEqual(result.deletedIds, ['gone'])
  assert.deepEqual(result.skippedIds, ['keep'])
  assert.deepEqual(result.failures, [])
  assert.match(cleanupResultText(result), /kept because a live endless run/)
})
