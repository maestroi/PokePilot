import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { SpectatorRun } from '../src/shared/api/spectator.ts'
import {
  formatReplayDuration,
  replayResult,
  replayRuns,
  replayRuntimeSeconds,
  replaySearchText,
  sortReplays
} from '../src/replays/model.ts'

function run(overrides: Partial<SpectatorRun> = {}): SpectatorRun {
  return {
    run_id: 'run-a',
    status: 'done',
    replay_ready: true,
    queued_at: 100,
    ended_at: 225,
    llm_profile: 'qwen-9b',
    play_style: 'speedrunner',
    starter: 'Squirtle',
    player: { money: 0, party: [], badges: ['Boulder Badge'], dex_owned: 4, dex_total: 151 },
    ...overrides
  }
}

test('replayRuns keeps only ready completed runs and orders newest first', () => {
  const got = replayRuns([
    run({ run_id: 'old', ended_at: 200 }),
    run({ run_id: 'live', status: 'running', ended_at: undefined }),
    run({ run_id: 'missing', replay_ready: false, ended_at: 400 }),
    run({ run_id: 'new', ended_at: 300 })
  ])
  assert.deepEqual(got.map((item) => item.run_id), ['new', 'old'])
})

test('replay helpers expose searchable metadata and human runtime', () => {
  const item = run({ goal: 'Earn the Boulder Badge', highlight: 'goal complete' })
  assert.equal(replayRuntimeSeconds(item), 125)
  assert.equal(formatReplayDuration(125), '2m 5s')
  assert.equal(replayResult(item), 'goal complete')
  const haystack = replaySearchText(item)
  for (const needle of ['qwen-9b', 'speedrunner', 'squirtle', 'boulder badge']) {
    assert.ok(haystack.includes(needle))
  }
})

test('sortReplays supports fastest and most badges', () => {
  const slow = run({ run_id: 'slow', queued_at: 100, ended_at: 400, player: { money: 0, party: [], badges: ['A', 'B'] } })
  const fast = run({ run_id: 'fast', queued_at: 100, ended_at: 180, player: { money: 0, party: [], badges: ['A'] } })
  assert.equal(sortReplays([slow, fast], 'runtime')[0]?.run_id, 'fast')
  assert.equal(sortReplays([fast, slow], 'badges')[0]?.run_id, 'slow')
})
