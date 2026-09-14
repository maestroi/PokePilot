import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  defaultGoalForPlayStyle,
  isPlayStyleRun,
  normalizePlayStyle,
  playSpeedLabel,
  playStyleLabel,
  playStyleTagline,
  policyLabel
} from '../src/shared/playstyle.ts'

test('play style labels match the public spectator names', () => {
  assert.equal(playStyleLabel({ play_style: 'adventure' }), 'Adventure')
  assert.equal(playStyleLabel({ play_style: 'speedrun' }), 'Speedrun')
  assert.equal(playStyleLabel({ play_style: 'completionist' }), 'Completionist')
  assert.equal(playStyleLabel({ play_style: 'team_builder' }), 'Team Builder')
  assert.equal(playStyleLabel({ play_style: 'team-builder' }), 'Team Builder')
  assert.equal(playStyleLabel({}), 'Speedrun')
  assert.equal(playStyleTagline({ play_style: 'adventure' }), 'Natural play · exploration and story')
})

test('play styles have terminal goal defaults', () => {
  assert.equal(defaultGoalForPlayStyle('speedrun'), 'Beat the Elite Four and Champion.')
  assert.equal(defaultGoalForPlayStyle('adventure'), 'Beat the Elite Four and Champion.')
  assert.equal(defaultGoalForPlayStyle('team_builder'), 'Beat the Elite Four and Champion.')
  assert.equal(defaultGoalForPlayStyle('completionist'), 'Complete the obtainable Pokédex.')
})

test('scripted runs are not assigned a play style', () => {
  assert.equal(isPlayStyleRun({ planner: 'scripted', play_style: 'adventure' }), false)
  assert.equal(isPlayStyleRun({ planner: 'llm', play_style: 'adventure' }), true)
  assert.equal(normalizePlayStyle({ play_style: 'adventure' }), 'adventure')
})

test('play speed uses the same public multipliers', () => {
  assert.equal(playSpeedLabel({ fps: 60 }), '1×')
  assert.equal(playSpeedLabel({ fps: 120 }), '2×')
  assert.equal(playSpeedLabel({ fps: 0 }), 'MAX')
  assert.equal(playSpeedLabel({}), 'MAX')
})

test('policy labels stay readable', () => {
  assert.equal(policyLabel('planner'), 'Planner')
  assert.equal(policyLabel(undefined, 'balanced'), 'Balanced')
  assert.equal(policyLabel('fight_every'), 'Fight Every')
})
