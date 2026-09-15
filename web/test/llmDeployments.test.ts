import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  DEFAULT_ARM_A,
  DEFAULT_ARM_B,
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
    deployment({ id: DEFAULT_ARM_A, state: 'busy', compute: 'RX 7900 XTX', model_id: 'qwen3.8-27b' }),
    deployment({ id: DEFAULT_ARM_B, state: 'ready', compute: 'RTX 4090', model_id: 'qwen3.5-4b' })
  ]
  assert.equal(preferredDeployment(rows, DEFAULT_ARM_A), DEFAULT_ARM_B)
  assert.match(deploymentOptionLabel(rows[0]), /busy/)
})

test('Brock defaults pick 27B on 7900 against 4B on 4090', () => {
  const [a, b] = defaultExperimentArms([
    deployment({ id: 'cpu', label: 'CPU', model_id: 'qwen3.5-4b', compute: 'CPU' }),
    deployment({ id: DEFAULT_ARM_B, label: 'Qwen 3.5 4B · RTX 4090', model_id: 'qwen3.5-4b', compute: 'RTX 4090' }),
    deployment({ id: DEFAULT_ARM_A, label: 'Qwen 3.8 27B · 7900 XTX', model_id: 'qwen3.8-27b', compute: 'RX 7900 XTX' })
  ])
  assert.equal(a, DEFAULT_ARM_A)
  assert.equal(b, DEFAULT_ARM_B)
})
