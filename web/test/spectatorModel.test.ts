import assert from 'node:assert/strict'
import { test } from 'node:test'
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
