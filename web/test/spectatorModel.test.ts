import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { SpectatorRun } from '../src/shared/api/spectator.ts'
import { preferredRun } from '../src/spectator/model.ts'

function run(run_id: string, status: string, queued_at: number): SpectatorRun {
  return { run_id, status, queued_at }
}

test('preferredRun keeps an explicitly selected run pinned', () => {
  const runs = [
    run('run-a', 'running', 10),
    run('run-b', 'running', 20)
  ]

  assert.equal(preferredRun(runs, 'run-a')?.run_id, 'run-a')
})

test('preferredRun does not switch when a pinned run temporarily disappears', () => {
  const runs = [
    run('run-b', 'running', 20),
    run('run-c', 'leased', 30)
  ]

  assert.equal(preferredRun(runs, 'run-a'), null)
})

test('preferredRun still follows the newest live run when selection is not pinned', () => {
  const runs = [
    run('run-a', 'running', 10),
    run('run-b', 'running', 20),
    run('run-c', 'queued', 30)
  ]

  assert.equal(preferredRun(runs)?.run_id, 'run-b')
})
