import type { DeploymentState, ModelDeployment } from '../shared/api/types'

export const DEFAULT_ROLE_FARM = 'farm'
export const DEFAULT_ROLE_EXPERIMENT_A = 'experiment-a'
export const DEFAULT_ROLE_EXPERIMENT_B = 'experiment-b'

function hasDefaultRole(deployment: ModelDeployment, role: string): boolean {
  return (deployment.default_for || []).some((candidate) => candidate.toLowerCase() === role.toLowerCase())
}

const blockedStates = new Set(['unavailable', 'failed', 'busy'])

// servesStrategist is false for choice-only APIs (TypeSafe Jev), which can
// only back the fast decision engine.
export function servesStrategist(deployment: Pick<ModelDeployment, 'protocol'>): boolean {
  return !deployment.protocol || deployment.protocol === 'openai'
}

export function strategistDeployments(deployments: ModelDeployment[]): ModelDeployment[] {
  return deployments.filter(servesStrategist)
}

export function deploymentSelectable(deployment: ModelDeployment): boolean {
  return deployment.enabled !== false && !blockedStates.has(String(deployment.state || '').toLowerCase())
}

export function deploymentStateLabel(deployment: ModelDeployment): string {
  return String(deployment.state || (deployment.enabled ? 'available' : 'disabled'))
}

export function deploymentOptionLabel(deployment: ModelDeployment): string {
  const compute = deployment.compute ? ` · ${deployment.compute}` : ''
  return `${deployment.label || deployment.id}${compute} · ${deploymentStateLabel(deployment)}`
}

export function deploymentIdentity(deployment: ModelDeployment): string {
  return [deployment.model_id, deployment.revision, deployment.quantization].filter(Boolean).join(' · ')
}

export function deploymentStateTone(state: DeploymentState | undefined): 'neutral' | 'info' | 'success' | 'warning' | 'danger' {
  switch (String(state || '').toLowerCase()) {
    case 'ready': return 'success'
    case 'available':
    case 'loading': return 'info'
    case 'busy': return 'warning'
    case 'failed':
    case 'unavailable':
    case 'disabled': return 'danger'
    default: return 'neutral'
  }
}

export function preferredDeployment(deployments: ModelDeployment[], id: string): string {
  const selectable = strategistDeployments(deployments).filter(deploymentSelectable)
  if (selectable.some((deployment) => deployment.id === id)) return id
  return selectable[0]?.id || ''
}

export function defaultFarmDeployment(deployments: ModelDeployment[]): string {
  const selectable = strategistDeployments(deployments).filter(deploymentSelectable)
  return selectable.find((deployment) => hasDefaultRole(deployment, DEFAULT_ROLE_FARM))?.id
    || selectable[0]?.id
    || ''
}

export function defaultExperimentArms(deployments: ModelDeployment[]): [string, string] {
  const enabled = strategistDeployments(deployments).filter((deployment) => deployment.enabled !== false)
  const armA = enabled.find((deployment) => hasDefaultRole(deployment, DEFAULT_ROLE_EXPERIMENT_A))
    || enabled.find((deployment) => hasDefaultRole(deployment, DEFAULT_ROLE_FARM))
    || enabled[0]
  const armB = enabled.find((deployment) => hasDefaultRole(deployment, DEFAULT_ROLE_EXPERIMENT_B) && deployment.id !== armA?.id)
    || enabled.find((deployment) => deployment.id !== armA?.id)
  return [armA?.id || '', armB?.id || '']
}
