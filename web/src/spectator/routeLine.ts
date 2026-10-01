import type { SpectatorRun } from '../shared/api/spectator.ts'

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

const JOHTO_GYMS = [
  { badge: 'Zephyr', town: 'Violet', color: '#8fb7cc' },
  { badge: 'Hive', town: 'Azalea', color: '#d5b659' },
  { badge: 'Plain', town: 'Goldenrod', color: '#d7a9c7' },
  { badge: 'Fog', town: 'Ecruteak', color: '#9f83c8' },
  { badge: 'Storm', town: 'Cianwood', color: '#d89055' },
  { badge: 'Mineral', town: 'Olivine', color: '#b5bcc4' },
  { badge: 'Glacier', town: 'Mahogany', color: '#79b8d8' },
  { badge: 'Rising', town: 'Blackthorn', color: '#5f78b9' }
] as const

type GymDefinition = { badge: string, town: string, color: string }

export interface RouteStop {
  key: string
  town: string
  badge: string
  color: string
  earned: boolean
  next: boolean
}

export interface RouteLine {
  region: 'Kanto' | 'Johto'
  startTown: string
  leagueTown: string
  leagueGoal: string
  stops: RouteStop[]
  earnedCount: number
  leagueEarned: boolean
  leagueNext: boolean
  completedGoals: number
  goalCount: number
  nextGoal: string
  fill: number
}

function leagueComplete(run: Pick<SpectatorRun, 'player' | 'stats'> | null | undefined): boolean {
  if (run?.stats?.goal_complete) return true
  return (run?.player?.milestones || [])
    .some((milestone) => String(milestone).toLowerCase().includes('hall of fame'))
}

function buildRouteLine(
  run: Pick<SpectatorRun, 'player' | 'stats'> | null | undefined,
  gyms: readonly GymDefinition[],
  region: 'Kanto' | 'Johto',
  startTown: string
): RouteLine {
  const owned = (run?.player?.badges || []).map((name) => String(name).toLowerCase())
  let nextTaken = false
  let lastEarned = -1
  const stops = gyms.map((gym, index) => {
    const earned = owned.some((name) => name.includes(gym.badge.toLowerCase()))
    if (earned) lastEarned = index
    const next = !earned && !nextTaken
    if (next) nextTaken = true
    return { key: gym.badge, town: gym.town, badge: gym.badge, color: gym.color, earned, next }
  })

  const earnedCount = stops.filter((stop) => stop.earned).length
  const leagueEarned = leagueComplete(run)
  const leagueNext = !leagueEarned && earnedCount === gyms.length
  const nextBadge = stops.find((stop) => stop.next)
  const completedGoals = earnedCount + (leagueEarned ? 1 : 0)
  const goalCount = gyms.length + 1

  return {
    region,
    startTown,
    leagueTown: 'Indigo League',
    leagueGoal: 'Hall of Fame',
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
          : `${gyms[0]?.badge || 'First'} Badge · ${gyms[0]?.town || startTown}`,
    fill: leagueEarned ? 1 : (lastEarned + 1) / goalCount
  }
}

export function kantoRouteLine(run: Pick<SpectatorRun, 'player' | 'stats'> | null | undefined): RouteLine {
  return buildRouteLine(run, KANTO_GYMS, 'Kanto', 'Pallet Town')
}

export function johtoRouteLine(run: Pick<SpectatorRun, 'player' | 'stats'> | null | undefined): RouteLine {
  return buildRouteLine(run, JOHTO_GYMS, 'Johto', 'New Bark Town')
}

export function pokemonRouteLine(run: Pick<SpectatorRun, 'game' | 'player' | 'stats'> | null | undefined): RouteLine {
  const game = String(run?.game || '').trim().toLowerCase()
  return game === 'pokemon-gold' || game === 'gold' || game === 'pokemon-silver' || game === 'silver'
    ? johtoRouteLine(run)
    : kantoRouteLine(run)
}
