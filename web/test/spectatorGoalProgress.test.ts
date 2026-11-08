import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { SpectatorRun } from '../src/shared/api/spectator.ts'
import { goalProgress, goalProgressMeta, isGoalComplete } from '../src/spectator/goalProgress.ts'

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

test('Gold and Silver Hall of Fame goal tracks Johto badges through 8/9', () => {
  for (const game of ['pokemon-gold', 'pokemon-silver']) {
    const badges = ['Zephyr', 'Hive', 'Plain', 'Fog', 'Storm', 'Mineral', 'Glacier', 'Rising']
    const before = run({
      game,
      goal: 'elite-four',
      play_style: 'adventure',
      player: { money: 0, badges, party: [] },
      stats: { ...run().stats!, goal_current: 8, goal_target: 9, goal_complete: false }
    })
    assert.equal(isGoalComplete(before), false)
    assert.equal(goalProgress(before), 88)
    assert.match(goalProgressMeta(before).detail, /Hall of Fame/)
    const completed = run({
      ...before,
      stats: { ...before.stats!, goal_current: 9, goal_target: 9, goal_complete: true }
    })
    assert.equal(goalProgress(completed), 100)
    assert.equal(goalProgressMeta(completed).complete, true)
  }
})

test('Gen2 badge and frontier goals are not treated as League goals', () => {
  const badges = ['Zephyr', 'Hive', 'Plain', 'Fog', 'Storm', 'Mineral', 'Glacier', 'Rising']
  const badgeGoal = run({
    game: 'pokemon-gold',
    goal: 'Earn 8 badges.',
    player: { money: 0, badges, party: [], milestones: ['Hall of Fame'] },
    stats: { ...run().stats!, goal_current: 7, goal_target: 8, goal_complete: false }
  })
  assert.equal(isGoalComplete(badgeGoal), false)
  assert.equal(goalProgress(badgeGoal), 87.5)

  const frontier = run({
    game: 'pokemon-silver',
    goal: 'Play through the current supported Gen 2 frontier.',
    stats: { ...run().stats!, goal_current: 0, goal_target: 1, goal_complete: false }
  })
  assert.equal(goalProgress(frontier), 0)
  assert.equal(goalProgressMeta(frontier).label, '0/1')
})
