import assert from 'node:assert/strict'
import test from 'node:test'
import type { RunSpec } from '../src/shared/api/types'
import { FULL_E2E_GOAL, productionRunSpec, supportsFullE2E } from '../src/operator/newRunMode.ts'

const source: RunSpec = {
  run_id: 'red-campaign-1', game: 'pokemon-red', planner: 'llm',
  starter: 'bulbasaur', seed: 42, goal: 'Earn the Boulder Badge.',
  dest: '', llm_profile: 'auto', llm_deployment: 'strategist',
  play_style: 'speedrun', fps: 480, max_rounds: 0, max_frames: 0,
  risk_tolerance: 'balanced', wild_encounters: 'planner', reasoning_effort: '',
  recovery_profile: 'strict', endless: true, random_seed: true
}

test('Full E2E can only be selected for supported full-campaign games', () => {
  assert.equal(supportsFullE2E(source), true)
  assert.equal(supportsFullE2E({ ...source, game: 'pokemon-gold' }), false)
  assert.equal(supportsFullE2E({ ...source, planner: 'scripted' }), false)
})

test('Full E2E locks a finite Champion campaign, retaining player settings', () => {
  const spec = productionRunSpec(source)
  assert.equal(spec.goal, FULL_E2E_GOAL)
  assert.equal(spec.endless, false)
  assert.equal(spec.random_seed, false)
  assert.equal(spec.recovery_profile, 'resilient')
  assert.equal(spec.starter, 'bulbasaur')
  assert.equal(spec.seed, 42)
  assert.equal(spec.llm_deployment, 'strategist')
  assert.equal(spec.play_style, 'speedrun')
  assert.equal(spec.fps, 480)
  assert.equal(source.endless, true, 'input spec is not mutated')
})

test('Full E2E cannot be submitted with an unsupported planner or game', () => {
  assert.throws(() => productionRunSpec({ ...source, game: 'pokemon-yellow' }), /Pokémon Red/)
  assert.throws(() => productionRunSpec({ ...source, planner: 'scripted' }), /LLM planner/)
})

test('reject invalid custom production run IDs before submitting any game', () => {
  assert.throws(() => productionRunSpec({ ...source, run_id: 'red.1' }), /run ID/)
  assert.throws(() => productionRunSpec({ ...source, run_id: 'a' }), /run ID/)
  assert.doesNotThrow(() => productionRunSpec({ ...source, run_id: '' }), 'API can generate a safe ID')
})
