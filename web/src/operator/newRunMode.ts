import type { RunSpec } from '../shared/api/types'

export type RunMode = 'standard' | 'full-e2e'
export const FULL_E2E_GAME = 'pokemon-red'
export const FULL_E2E_GOAL = 'Beat the Elite Four and Champion.'

export function supportsFullE2E(spec: Pick<RunSpec, 'game' | 'planner'>): boolean {
  return spec.game === FULL_E2E_GAME && spec.planner === 'llm'
}

// Force a meaningful and finite campaign for a full E2E qualification.
// Preserve the user's starter, seed, model, FPS and other agent settings.
export function productionRunSpec(spec: RunSpec): RunSpec {
  if (!supportsFullE2E(spec)) throw new Error('Full E2E currently requires Pokémon Red with the LLM planner.')
  return {
    ...spec,
    goal: FULL_E2E_GOAL,
    endless: false,
    random_seed: false,
    recovery_profile: 'resilient'
  }
}
