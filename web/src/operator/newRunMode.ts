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
  // Validate before queueing a run: the coordinator uses this exact ID.
  if (spec.run_id && !/^[-_a-zA-Z0-9]{3,100}$/.test(spec.run_id)) {
    throw new Error('Full E2E run ID must be 3–100 letters, numbers, hyphens or underscores.')
  }
  return {
    ...spec,
    purpose: 'full_e2e',
    goal: FULL_E2E_GOAL,
    endless: false,
    random_seed: false,
    recovery_profile: 'resilient'
  }
}
