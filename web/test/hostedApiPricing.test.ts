import assert from 'node:assert/strict'
import test from 'node:test'
import { hostedApiEquivalentCosts, parseOpenRouterModels } from '../src/shared/hostedApiPricing.ts'

function model(id: string, prompt: string, completion: string, options: Record<string, unknown> = {}) {
  return {
    id, name: id, context_length: 128000, created: 1750000000,
    architecture: { input_modalities: ['text'], output_modalities: ['text'] },
    pricing: { prompt, completion },
    ...options
  }
}

test('OpenRouter text model prices are converted from USD/token to USD/million', () => {
  const models = parseOpenRouterModels({ data: [
    model('anthropic/test', '0.000002', '0.000010'),
    model('openai/small', '0', '0')
  ] })
  assert.equal(models.length, 2)
  assert.deepEqual([models[0].inputPerMillion, models[0].outputPerMillion], [2, 10])
  assert.equal(models[0].provider, 'Anthropic')
  assert.equal(models[0].contextLength, 128000)
  assert.deepEqual([models[1].inputPerMillion, models[1].outputPerMillion], [0, 0])
})

test('filters unpriced, non-text, malformed and duplicate models without changing popularity order', () => {
  const rows = [
    model('openai/first', '0.000001', '0.000002'),
    model('openai/first', '0.000001', '0.000002'),
    model('meta/image-only', '0.000001', '0.000002', { architecture: { input_modalities: ['image'], output_modalities: ['image'] } }),
    model('meta/missing', '0.000001', '', {}),
    model('meta/negative', '-0.000001', '0.000002'),
    model('anthropic/second', '0.000003', '0.000004')
  ]
  assert.deepEqual(parseOpenRouterModels({ data: rows }).map(m => m.id), ['openai/first', 'anthropic/second'])
  assert.throws(() => parseOpenRouterModels({ wrong: [] }), /invalid/)
  assert.throws(() => parseOpenRouterModels({ data: [] }), /no priced text/)
})

test('prices prompt and completion separately using live catalog rates', () => {
  const prices = parseOpenRouterModels({ data: [model('anthropic/test', '0.000002', '0.000010')] })
  const [estimate] = hostedApiEquivalentCosts(43_649_062, 889_935, prices)
  assert.ok(Math.abs(estimate.costUsd - 96.197474) < 1e-9)
})

test('invalid and negative token budgets are zero, but prices remain visible', () => {
  const prices = parseOpenRouterModels({ data: [model('anthropic/test', '0.000002', '0.000010')] })
  assert.deepEqual(hostedApiEquivalentCosts(-100, Number.NaN, prices).map(e => e.costUsd), [0])
})
