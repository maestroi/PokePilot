import assert from 'node:assert/strict'
import { test } from 'node:test'
import { preferredRun } from '../src/spectator/preferredRun.ts'

test('preferredRun falls back to the newest completed replay when nothing is live', () => {
  const runs = [
    { run_id: 'older-replay', status: 'done', ended_at: 100 },
    { run_id: 'newest-replay', status: 'done', ended_at: 200 }
  ]

  assert.equal(preferredRun(runs)?.run_id, 'newest-replay')
})
