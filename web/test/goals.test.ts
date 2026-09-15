import assert from 'node:assert/strict'
import test from 'node:test'
import { GOAL_OPTIONS, nextGoalForPlayStyle } from '../src/shared/goals.ts'

test('shared goal presets include structured stop targets and free play', () => {
  assert.equal(GOAL_OPTIONS[0], 'Earn the Boulder Badge.')
  assert.ok(GOAL_OPTIONS.includes('Beat the Elite Four and Champion.'))
  assert.ok(GOAL_OPTIONS.includes('Complete the obtainable Pokédex.'))
  assert.ok(GOAL_OPTIONS.includes(''))
  assert.equal(new Set(GOAL_OPTIONS).size, GOAL_OPTIONS.length)
})

test('explicitly selected goal survives later play-style changes', () => {
  const boulder = 'Earn the Boulder Badge.'
  assert.equal(nextGoalForPlayStyle(boulder, 'adventure', true), boulder)
  assert.equal(nextGoalForPlayStyle(boulder, 'completionist', true), boulder)
})

test('untouched goal still follows play-style defaults', () => {
  assert.equal(nextGoalForPlayStyle('', 'adventure', false), 'Beat the Elite Four and Champion.')
  assert.equal(nextGoalForPlayStyle('', 'completionist', false), 'Complete the obtainable Pokédex.')
})
