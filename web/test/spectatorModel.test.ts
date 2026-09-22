import assert from 'node:assert/strict'
import { test } from 'node:test'
import { splitSpectatorRuns } from '../src/spectator/model.ts'
import { preferredRun, type SelectableSpectatorRun } from '../src/spectator/preferredRun.ts'

function run(run_id: string, status: string, queued_at: number, featured = false): SelectableSpectatorRun {
  return { run_id, status, queued_at, featured }
}

test('preferredRun keeps an explicitly selected run pinned', () => {
  const runs = [
    run('run-a', 'running', 10),
    run('run-b', 'running', 20, true)
  ]

  assert.equal(preferredRun(runs, 'run-a')?.run_id, 'run-a')
})

test('preferredRun does not switch when a pinned run temporarily disappears', () => {
  const runs = [
    run('run-b', 'running', 20, true),
    run('run-c', 'leased', 30)
  ]

  assert.equal(preferredRun(runs, 'run-a'), null)
})

test('preferredRun chooses the featured run when selection is not pinned', () => {
  const runs = [
    run('run-a', 'running', 30),
    run('run-b', 'running', 20, true),
    run('run-c', 'queued', 40)
  ]

  assert.equal(preferredRun(runs)?.run_id, 'run-b')
})

test('preferredRun still follows the newest live run when nothing is featured', () => {
  const runs = [
    run('run-a', 'running', 10),
    run('run-b', 'running', 20),
    run('run-c', 'queued', 30)
  ]

  assert.equal(preferredRun(runs)?.run_id, 'run-b')
})


test('splitSpectatorRuns only labels running and leased sessions as live', () => {
  const runs = [
    { run_id: 'running', status: 'running', queued_at: 10 },
    { run_id: 'leased', status: 'leased', queued_at: 20 },
    { run_id: 'paused', status: 'paused', queued_at: 30 },
    { run_id: 'queued', status: 'queued', queued_at: 40 },
    { run_id: 'done', status: 'done', ended_at: 50 }
  ]

  const grouped = splitSpectatorRuns(runs)
  assert.deepEqual(grouped.live.map((entry) => entry.run_id), ['leased', 'running'])
  assert.deepEqual(grouped.recent.map((entry) => entry.run_id), ['done'])
})
