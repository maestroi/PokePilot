import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('../src/operator/PairedExperimentPanel.vue', import.meta.url), 'utf8')
const types = readFileSync(new URL('../src/shared/api/types.ts', import.meta.url), 'utf8')

test('paired experiment pins game and surfaces comparability exclusions', () => {
  assert.match(source, /game: 'pokemon-red'/)
  assert.match(source, /v-model="form\.game"/)
  assert.match(source, /game: form\.game/)
  assert.match(source, /Comparable/)
  assert.match(source, /Excluded non-comparable seeds/)
  assert.match(source, /Excluded pairs never contribute to the aggregate metrics above/)
  assert.match(source, /rom_identity/)
  assert.match(source, /deploymentIdentity\('arm_a'\)/)
  assert.match(types, /comparable_pairs\?: number/)
  assert.match(types, /excluded_pairs\?: number/)
  assert.match(types, /pairs\?: ExperimentPairResult\[\]/)
})

test('paired experiment exposes quality and inference benchmark metrics', () => {
  for (const label of [
    'Goal success',
    'Median rounds / frames to goal',
    'Blackouts / objective failures',
    'Stagnation / plan-exhaustion replans',
    'Plan steps produced / executed / skipped',
    'Plan execution fraction',
    'Strategic avg / p50 / p95',
    'Planner time',
    'Prefill / decode TPS',
    'Prompt / completion tokens'
  ]) {
    assert.ok(source.includes(label), `missing metric ${label}`)
  }
})
