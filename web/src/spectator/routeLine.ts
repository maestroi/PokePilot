import type { SpectatorRun } from '../shared/api/spectator.ts'

interface GymStop {
  badge: string
  town: string
  color: string
}

interface RouteProfile {
  startTown: string
  leagueTown: string
  gyms: readonly GymStop[]
}

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
  { badge: 'Zephyr', town: 'Violet', color: '#8fa7c5' },
  { badge: 'Hive', town: 'Azalea', color: '#c7a64a' },
  { badge: 'Plain', town: 'Goldenrod', color: '#d79ac1' },
  { badge: 'Fog', town: 'Ecruteak', color: '#a77bdb' },
  { badge: 'Storm', town: 'Cianwood', color: '#6596c8' },
  { badge: 'Mineral', town: 'Olivine', color: '#a9b3be' },
  { badge: 'Glacier', town: 'Mahogany', color: '#7fc7d9' },
  { badge: 'Rising', town: 'Blackthorn', color: '#6f78c9' }
] as const

const KANTO_PROFILE: RouteProfile = {
  startTown: 'Pallet Town',
  leagueTown: 'Indigo League',
  gyms: KANTO_GYMS
}

const JOHTO_PROFILE: RouteProfile = {
  startTown: 'New Bark Town',
  leagueTown: 'Indigo Plateau',
  gyms: JOHTO_GYMS
}

export interface RouteStop {
  key: string
  town: string
  badge: string
  color: string
  earned: boolean
  next: boolean
}

export interface RouteLine {
  startTown: string
  leagueTown: string
  stops: RouteStop[]
  earnedCount: number
  leagueEarned: boolean
  leagueNext: boolean
  completedGoals: number
  goalCount: number
  nextGoal: string
  // 0..1 share of the line between the starting town and League that is filled.
  fill: number
}

function isGen2Game(game: string | undefined): boolean {
  const normalized = String(game || '').trim().toLowerCase()
  return normalized === 'pokemon-gold'
    || normalized === 'gold'
    || normalized === 'pokemon-silver'
    || normalized === 'silver'
}

function profileForGame(game: string | undefined): RouteProfile {
  return isGen2Game(game) ? JOHTO_PROFILE : KANTO_PROFILE
}

function leagueComplete(run: Pick<SpectatorRun, 'player' | 'stats'> | null | undefined): boolean {
  if (run?.stats?.goal_complete) return true
  return (run?.player?.milestones || [])
    .some((milestone) => String(milestone).toLowerCase().includes('hall of fame'))
}

function buildRouteLine(
  run: Pick<SpectatorRun, 'player' | 'stats'> | null | undefined,
  profile: RouteProfile
): RouteLine {
  const owned = (run?.player?.badges || []).map((name) => String(name).toLowerCase())
  let nextTaken = false
  let lastEarned = -1
  const stops = profile.gyms.map((gym, index) => {
    const earned = owned.some((name) => name.includes(gym.badge.toLowerCase()))
    if (earned) lastEarned = index
    const next = !earned && !nextTaken
    if (next) nextTaken = true
    return { key: gym.badge, town: gym.town, badge: gym.badge, color: gym.color, earned, next }
  })

  const earnedCount = stops.filter((stop) => stop.earned).length
  const leagueEarned = leagueComplete(run)
  const leagueNext = !leagueEarned && earnedCount === profile.gyms.length
  const nextBadge = stops.find((stop) => stop.next)
  const completedGoals = earnedCount + (leagueEarned ? 1 : 0)
  const goalCount = profile.gyms.length + 1

  return {
    startTown: profile.startTown,
    leagueTown: profile.leagueTown,
    stops,
    earnedCount,
    leagueEarned,
    leagueNext,
    completedGoals,
    goalCount,
    nextGoal: nextBadge
      ? `${nextBadge.badge} Badge · ${nextBadge.town}`
      : leagueNext
        ? `${profile.leagueTown} · Hall of Fame`
        : leagueEarned
          ? 'Hall of Fame reached'
          : `${profile.gyms[0].badge} Badge · ${profile.gyms[0].town}`,
    fill: leagueEarned ? 1 : (lastEarned + 1) / goalCount
  }
}

export function kantoRouteLine(run: Pick<SpectatorRun, 'player' | 'stats'> | null | undefined): RouteLine {
  return buildRouteLine(run, KANTO_PROFILE)
}

export function gameRouteLine(
  run: Pick<SpectatorRun, 'game' | 'player' | 'stats'> | null | undefined
): RouteLine {
  return buildRouteLine(run, profileForGame(run?.game))
}
