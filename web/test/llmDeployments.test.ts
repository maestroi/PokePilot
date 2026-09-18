import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  DEFAULT_ROLE_EXPERIMENT_A,
  DEFAULT_ROLE_EXPERIMENT_B,
  defaultExperimentArms,
  defaultFarmDeployment,
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
    deployment({ id: 'primary', state: 'busy', compute: 'RX 7900 XTX', model_id: 'qwen3.5-9b' }),
    deployment({ id: 'secondary', state: 'ready', compute: 'RTX 4090', model_id: 'qwen3.5-4b' })
  ]
  assert.equal(preferredDeployment(rows, 'primary'), 'secondary')
  assert.match(deploymentOptionLabel(rows[0]), /busy/)
})

test('experiment defaults come from registry roles, not model size or hardware labels', () => {
  const [a, b] = defaultExperimentArms([
    deployment({ id: 'cpu', label: 'CPU', model_id: 'qwen3.5-4b', compute: 'CPU' }),
    deployment({ id: 'cloud', label: 'Cloud model', model_id: 'future-model', compute: 'cloud', default_for: [DEFAULT_ROLE_EXPERIMENT_B] }),
    deployment({ id: 'local-primary', label: 'Local primary', model_id: 'qwen3.5-9b', compute: 'RX 7900 XTX', default_for: [DEFAULT_ROLE_EXPERIMENT_A, 'farm'] })
  ])
  assert.equal(a, 'local-primary')
  assert.equal(b, 'cloud')
})


test('normal runs prefer the registry farm role', () => {
  const rows = [
    deployment({ id: 'secondary', default_for: ['experiment-b'] }),
    deployment({ id: 'primary', default_for: ['farm'] })
  ]
  assert.equal(defaultFarmDeployment(rows), 'primary')
})
