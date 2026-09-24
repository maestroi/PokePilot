import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { SpectatorRun } from '../src/shared/api/spectator.ts'
import { goalProgress, goalProgressMeta } from '../src/spectator/model.ts'

function run(overrides: Partial<SpectatorRun> = {}): SpectatorRun {
  return {
    run_id: 'run-1',
    status: 'running',
    play_style: 'speedrun',
    goal: 'Beat the Elite Four and Champion and reach the Hall of Fame',
    player: {
      money: 0,
      badges: [],
      party: []
    },
    stats: {
      round: 0,
      rounds_left: 0,
      calls: 0,
      rounds: 0,
      rejected: 0,
      repeats: 0,
      last_seconds: 0,
      avg_seconds: 0,
      goal_summary: 'Beat the Elite Four and Champion and reach the Hall of Fame',
      goal_current: 0,
      goal_target: 8,
      goal_complete: false
    },
    ...overrides
  }
}

test('speedrun badge completion does not render as full goal completion', () => {
  const value = run({
    player: {
      money: 0,
      badges: ['Boulder', 'Cascade', 'Thunder', 'Rainbow', 'Soul', 'Marsh', 'Volcano', 'Earth'],
      party: []
    },
    stats: {
      ...run().stats!,
      goal_current: 8,
      goal_target: 8,
      goal_complete: false
    }
  })

  assert.equal(goalProgress(value), 88)
  assert.equal(goalProgressMeta(value).complete, false)
  assert.match(goalProgressMeta(value).detail, /Elite Four/)
})

test('league goal reaches 100 only when the structured goal is complete', () => {
  const value = run({
    player: {
      money: 0,
      badges: ['Boulder', 'Cascade', 'Thunder', 'Rainbow', 'Soul', 'Marsh', 'Volcano', 'Earth'],
      party: []
    },
    stats: {
      ...run().stats!,
      goal_current: 8,
      goal_target: 8,
      goal_complete: true
    }
  })

  assert.equal(goalProgress(value), 100)
  assert.equal(goalProgressMeta(value).complete, true)
})

test('Hall of Fame milestone also marks a league goal complete', () => {
  const value = run({
    player: {
      money: 0,
      badges: ['Boulder', 'Cascade', 'Thunder', 'Rainbow', 'Soul', 'Marsh', 'Volcano', 'Earth'],
      party: [],
      milestones: ['Hall of Fame']
    }
  })

  assert.equal(goalProgress(value), 100)
})

test('non-league goals keep using structured goal current and target', () => {
  const value = run({
    play_style: 'completionist',
    goal: 'Complete the Pokédex',
    stats: {
      ...run().stats!,
      goal_summary: 'Complete the Pokédex',
      goal_current: 75,
      goal_target: 150,
      goal_complete: false
    }
  })

  assert.equal(goalProgress(value), 50)
  assert.equal(goalProgressMeta(value).label, '75/150')
})
