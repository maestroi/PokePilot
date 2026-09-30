import type { SpectatorRun } from '../shared/api/spectator.ts'

// Kanto's eight gyms in the order the route strip draws them. This is Pokémon
// Red/Blue presentation data, so it lives with the spectator UI, not the runtime.
const KANTO_GYMS = [
  { badge: 'Boulder', town: 'Pewter', color: '#a08d78' },
  { badge: 'Cascade', town: 'Cerulean', color: '#4aa3e0' },
  { badge: 'Thunder', town: 'Vermilion', color: '#f0c43a' },
  { badge: 'Rainbow', town: 'Celadon', color: '#7bc96f' },
  { badge: 'Soul', town: 'Fuchsia', color: '#e06aa0' },
  { badge: 'Marsh', town: 'Saffron', color: '#a77bdb' },
  { badge: 'Volcano', town: 'Cinnabar', color: '#ef6a5e' },
  { badge: 'Earth', town: 'Viridian', color: '#5fa35b' }
] as const

export interface RouteStop {
  key: string
  town: string
  badge: string
  color: string
  earned: boolean
  next: boolean
}

export interface RouteLine {
  stops: RouteStop[]
  earnedCount: number
  // 0..1 share of the line between the first and last station that is filled.
  fill: number
}

export function kantoRouteLine(run: Pick<SpectatorRun, 'player'> | null | undefined): RouteLine {
  const owned = (run?.player?.badges || []).map((name) => String(name).toLowerCase())
  let nextTaken = false
  let lastEarned = -1
  const stops = KANTO_GYMS.map((gym, index) => {
    const earned = owned.some((name) => name.includes(gym.badge.toLowerCase()))
    if (earned) lastEarned = index
    const next = !earned && !nextTaken
    if (next) nextTaken = true
    return { key: gym.badge, town: gym.town, badge: gym.badge, color: gym.color, earned, next }
  })
  return {
    stops,
    earnedCount: stops.filter((stop) => stop.earned).length,
    fill: (lastEarned + 1) / (KANTO_GYMS.length + 1)
  }
}
