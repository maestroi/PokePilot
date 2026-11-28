import type { RunSpec } from '../shared/api/types'

export type RunMode = 'standard' | 'full-e2e'
export const FULL_E2E_GOAL = 'Beat the Elite Four and Champion.'
// Gold/Silver end at Red on Mt. Silver; their Hall of Fame is not the end.
export const GEN2_FULL_GAME_GOAL = 'progress:gs_mt_silver_red_defeated'

// The goal that means "the whole game" for each Full E2E-capable cartridge.
const FULL_GAME_GOALS: Record<string, string> = {
  'pokemon-red': FULL_E2E_GOAL,
  'pokemon-gold': GEN2_FULL_GAME_GOAL,
  'pokemon-silver': GEN2_FULL_GAME_GOAL
}

export function fullGameGoal(game: string | undefined): string | undefined {
  return FULL_GAME_GOALS[game || '']
}

export function supportsFullE2E(spec: Pick<RunSpec, 'game' | 'planner'>): boolean {
  return fullGameGoal(spec.game) !== undefined && spec.planner === 'llm'
}

// Force a meaningful and finite campaign for a full E2E qualification.
// Preserve the user's starter, seed, model, FPS and other agent settings.
export function productionRunSpec(spec: RunSpec): RunSpec {
  if (!supportsFullE2E(spec)) throw new Error('Full E2E requires Pokémon Red, Gold or Silver with the LLM planner.')
  // Validate before queueing a run: the coordinator uses this exact ID.
  if (spec.run_id && !/^[-_a-zA-Z0-9]{3,100}$/.test(spec.run_id)) {
    throw new Error('Full E2E run ID must be 3–100 letters, numbers, hyphens or underscores.')
  }
  return {
    ...spec,
    purpose: 'full_e2e',
    goal: fullGameGoal(spec.game)!,
    endless: false,
    random_seed: false,
    recovery_profile: 'resilient'
  }
}
