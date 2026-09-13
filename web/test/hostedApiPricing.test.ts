import assert from 'node:assert/strict'
import test from 'node:test'
import { hostedApiEquivalentCosts } from '../src/shared/hostedApiPricing.ts'

test('hosted API equivalents price prompt and completion tokens independently', () => {
  const estimates = hostedApiEquivalentCosts(43_649_062, 889_935)
  const byModel = Object.fromEntries(estimates.map((estimate) => [estimate.model, estimate.costUsd]))

  assert.ok(Math.abs(byModel['Claude Sonnet 5'] - 96.197474) < 1e-9)
  assert.ok(Math.abs(byModel['GPT-5.6 Sol'] - 192.394948) < 1e-9)
  assert.ok(Math.abs(byModel['Claude Opus 5'] - 240.493685) < 1e-9)
})

test('hosted API equivalents treat invalid or negative telemetry as zero', () => {
  const estimates = hostedApiEquivalentCosts(-100, Number.NaN)
  assert.deepEqual(estimates.map((estimate) => estimate.costUsd), [0, 0, 0])
})
