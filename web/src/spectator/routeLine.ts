import type { SpectatorRun } from '../shared/api/spectator.ts'

// Kanto's eight gyms in the order the route strip draws them. The ninth goal
// is the League / Hall of Fame. Pallet Town is only the visual starting point.
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
  leagueEarned: boolean
  leagueNext: boolean
  completedGoals: number
  goalCount: number
  nextGoal: string
  // 0..1 share of the line between Pallet Town and Indigo League that is filled.
  fill: number
}

function leagueComplete(run: Pick<SpectatorRun, 'player' | 'stats'> | null | undefined): boolean {
  if (run?.stats?.goal_complete) return true
  return (run?.player?.milestones || [])
    .some((milestone) => String(milestone).toLowerCase().includes('hall of fame'))
}

export function kantoRouteLine(run: Pick<SpectatorRun, 'player' | 'stats'> | null | undefined): RouteLine {
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

  const earnedCount = stops.filter((stop) => stop.earned).length
  const leagueEarned = leagueComplete(run)
  const leagueNext = !leagueEarned && earnedCount === KANTO_GYMS.length
  const nextBadge = stops.find((stop) => stop.next)
  const completedGoals = earnedCount + (leagueEarned ? 1 : 0)
  const goalCount = KANTO_GYMS.length + 1

  return {
    stops,
    earnedCount,
    leagueEarned,
    leagueNext,
    completedGoals,
    goalCount,
    nextGoal: nextBadge
      ? `${nextBadge.badge} Badge · ${nextBadge.town}`
      : leagueNext
        ? 'Indigo League · Hall of Fame'
        : leagueEarned
          ? 'Hall of Fame reached'
          : 'Boulder Badge · Pewter',
    fill: leagueEarned ? 1 : (lastEarned + 1) / goalCount
  }
}
