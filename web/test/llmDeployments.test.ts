import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  defaultExperimentArms,
  deploymentOptionLabel,
  deploymentSelectable,
  preferredDeployment
} from '../src/operator/llmDeployments.ts'
import type { ModelDeployment } from '../src/shared/api/types.ts'

function deployment(partial: Partial<ModelDeployment> & Pick<ModelDeployment, 'id'>): ModelDeployment {
  return {
    label: partial.label || partial.id,
    model_id: partial.model_id || partial.id,
    compute: partial.compute || '',
    endpoint: partial.endpoint || 'http://example/v1',
    api_model: partial.api_model || 'model',
    enabled: partial.enabled ?? true,
    state: partial.state || 'ready',
    ...partial
  }
}

test('busy failed and unavailable deployments are not immediately selectable', () => {
  assert.equal(deploymentSelectable(deployment({ id: 'ready', state: 'ready' })), true)
  assert.equal(deploymentSelectable(deployment({ id: 'available', state: 'available' })), true)
  assert.equal(deploymentSelectable(deployment({ id: 'loading', state: 'loading' })), true)
  assert.equal(deploymentSelectable(deployment({ id: 'busy', state: 'busy' })), false)
  assert.equal(deploymentSelectable(deployment({ id: 'failed', state: 'failed' })), false)
  assert.equal(deploymentSelectable(deployment({ id: 'unavailable', state: 'unavailable' })), false)
  assert.equal(deploymentSelectable(deployment({ id: 'disabled', enabled: false, state: 'ready' })), false)
})

test('preferred deployment keeps a selectable choice and falls back', () => {
  const rows = [
    deployment({ id: 'farm-7900', state: 'busy', compute: 'RX 7900 XTX', model_id: 'qwen3.5-9b', default: true }),
    deployment({ id: 'coding-4090', state: 'ready', compute: 'RTX 4090', model_id: 'qwen3.5-4b' })
  ]
  assert.equal(preferredDeployment(rows, 'farm-7900'), 'coding-4090')
  assert.match(deploymentOptionLabel(rows[0]), /busy/)
})

test('experiment defaults follow registry default and prefer a different compute for B', () => {
  const [a, b] = defaultExperimentArms([
    deployment({ id: 'cpu', label: 'CPU', model_id: 'qwen3.5-4b', compute: 'CPU' }),
    deployment({ id: 'coding-4090', label: 'Qwen 3.5 4B · RTX 4090', model_id: 'qwen3.5-4b', compute: 'RTX 4090' }),
    deployment({ id: 'farm-7900', label: 'Farm · RX 7900 XTX', model_id: 'qwen3.5-9b', compute: 'RX 7900 XTX', default: true })
  ])
  assert.equal(a, 'farm-7900')
  assert.equal(b, 'cpu')
})

test('preferred deployment falls back to registry default without model-name heuristics', () => {
  const rows = [
    deployment({ id: 'other', compute: 'RTX 4090' }),
    deployment({ id: 'farm', compute: 'RX 7900 XTX', default: true })
  ]
  assert.equal(preferredDeployment(rows, ''), 'farm')
})
