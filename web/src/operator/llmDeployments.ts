import type { DeploymentState, ModelDeployment } from '../shared/api/types'

export const DEFAULT_ARM_A = 'qwen38-27b-7900'
export const DEFAULT_ARM_B = 'qwen35-4b-4090'

const blockedStates = new Set(['unavailable', 'failed', 'busy'])

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
  const selectable = deployments.filter(deploymentSelectable)
  if (selectable.some((deployment) => deployment.id === id)) return id
  return selectable[0]?.id || ''
}

export function defaultExperimentArms(deployments: ModelDeployment[]): [string, string] {
  const enabled = deployments.filter((deployment) => deployment.enabled !== false)
  const armA = enabled.find((deployment) => deployment.id === DEFAULT_ARM_A)
    || enabled.find((deployment) => /27b/i.test(`${deployment.model_id} ${deployment.label}`) && /7900/i.test(`${deployment.compute} ${deployment.label}`))
    || enabled[0]
  const armB = enabled.find((deployment) => deployment.id === DEFAULT_ARM_B)
    || enabled.find((deployment) => /4b/i.test(`${deployment.model_id} ${deployment.label}`) && /4090/i.test(`${deployment.compute} ${deployment.label}`))
    || enabled.find((deployment) => deployment.id !== armA?.id)
  return [armA?.id || '', armB?.id || '']
}
