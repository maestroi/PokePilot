import assert from 'node:assert/strict'
import test from 'node:test'
import { GOAL_OPTIONS } from '../src/shared/goals.ts'

test('Boulder remains a first-class selectable benchmark goal', () => {
  assert.equal(GOAL_OPTIONS.includes('Earn the Boulder Badge.'), true)
})
