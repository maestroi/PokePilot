import type { SpectatorRun } from './api/spectator'

export type PublicCapability = 'live' | 'replay' | 'worldMap' | 'stats'

const DEFAULT_CAPABILITIES: PublicCapability[] = ['live', 'replay']

const GAME_CAPABILITY_PROFILES: Record<string, PublicCapability[]> = {
  'pokemon-red': ['live', 'replay', 'worldMap', 'stats'],
  'pokemon-blue': ['live', 'replay', 'stats'],
  'pokemon-yellow': ['live', 'replay', 'stats'],
  'pokemon-gold': ['live', 'replay', 'stats'],
  'pokemon-silver': ['live', 'replay', 'stats'],
  red: ['live', 'replay', 'worldMap', 'stats'],
  blue: ['live', 'replay', 'stats'],
  yellow: ['live', 'replay', 'stats'],
  gold: ['live', 'replay', 'stats'],
  silver: ['live', 'replay', 'stats'],
  tetris: ['live', 'replay', 'stats']
}

function isPublicCapability(value: string): value is PublicCapability {
  return value === 'live' || value === 'replay' || value === 'worldMap' || value === 'stats'
}

function normalizeCapabilities(values: readonly string[]): PublicCapability[] {
  return [...new Set(values.filter(isPublicCapability))]
}

export function publicCapabilitiesForRun(run?: Pick<SpectatorRun, 'game' | 'game_state' | 'public_capabilities'> | null): PublicCapability[] {
  const advertised = normalizeCapabilities(run?.public_capabilities || [])
  if (advertised.length) return advertised

  const game = (run?.game || '').trim().toLowerCase()
  const stateKind = (run?.game_state?.kind || '').trim().toLowerCase()
  if (stateKind === 'tetris') return [...GAME_CAPABILITY_PROFILES.tetris]

  return [...(GAME_CAPABILITY_PROFILES[game] || DEFAULT_CAPABILITIES)]
}

export function supportsPublicCapability(
  run: Pick<SpectatorRun, 'game' | 'game_state' | 'public_capabilities'> | null | undefined,
  capability: PublicCapability
): boolean {
  return publicCapabilitiesForRun(run).includes(capability)
}
