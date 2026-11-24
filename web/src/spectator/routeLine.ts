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
  // The final step after the gyms and its exact milestone label.
  finale: string
  finaleGoals: readonly string[]
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

const LEAGUE_GOALS = ['elite-four', 'hall-of-fame', 'progress:gs_league_hall_of_fame'] as const

const KANTO_PROFILE: RouteProfile = {
  startTown: 'Pallet Town',
  leagueTown: 'Indigo League',
  gyms: KANTO_GYMS,
  finale: 'Hall of Fame',
  finaleGoals: LEAGUE_GOALS
}

const JOHTO_PROFILE: RouteProfile = {
  startTown: 'New Bark Town',
  leagueTown: 'Indigo Plateau',
  gyms: JOHTO_GYMS,
  finale: 'Hall of Fame',
  finaleGoals: LEAGUE_GOALS
}

// Gold/Silver after the Hall of Fame: Kanto badges in the order the Gen2
// controller earns them, ending at Red. Hall of Fame is not game completion.
const GEN2_POSTGAME_PROFILE: RouteProfile = {
  startTown: 'Vermilion City',
  leagueTown: 'Mt. Silver',
  gyms: [
    KANTO_GYMS[2], KANTO_GYMS[5], KANTO_GYMS[3], KANTO_GYMS[4],
    KANTO_GYMS[1], KANTO_GYMS[0], KANTO_GYMS[6], KANTO_GYMS[7]
  ],
  finale: 'Red',
  finaleGoals: ['progress:gs_mt_silver_red_defeated']
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
  finale: string
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

function hasMilestone(run: Pick<SpectatorRun, 'player'> | null | undefined, label: string): boolean {
  return (run?.player?.milestones || []).some((m) => String(m).trim().toLowerCase() === label.toLowerCase())
}

function profileForGame(run: Pick<SpectatorRun, 'game' | 'player'> | null | undefined): RouteProfile {
  if (!isGen2Game(run?.game)) return KANTO_PROFILE
  return hasMilestone(run, 'Hall of Fame') ? GEN2_POSTGAME_PROFILE : JOHTO_PROFILE
}

function leagueComplete(run: Pick<SpectatorRun, 'goal' | 'player' | 'stats'> | null | undefined, profile: RouteProfile): boolean {
  // An eight-badge or supported-frontier goal can complete without beating
  // the League. Do not turn generic goal_complete into Hall of Fame proof.
  if (hasMilestone(run, profile.finale)) return true
  const goal = String(run?.goal || '').trim().toLowerCase()
  const finaleGoal = profile.finaleGoals.includes(goal)
    || (profile.finale === 'Hall of Fame' && /elite four|champion|hall of fame/.test(goal))
  return Boolean(finaleGoal && run?.stats?.goal_complete)
}

function buildRouteLine(
  run: Pick<SpectatorRun, 'goal' | 'player' | 'stats'> | null | undefined,
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
  const leagueEarned = leagueComplete(run, profile)
  const leagueNext = !leagueEarned && earnedCount === profile.gyms.length
  const nextBadge = stops.find((stop) => stop.next)
  const completedGoals = earnedCount + (leagueEarned ? 1 : 0)
  const goalCount = profile.gyms.length + 1

  return {
    startTown: profile.startTown,
    leagueTown: profile.leagueTown,
    finale: profile.finale,
    stops,
    earnedCount,
    leagueEarned,
    leagueNext,
    completedGoals,
    goalCount,
    nextGoal: nextBadge
      ? `${nextBadge.badge} Badge · ${nextBadge.town}`
      : leagueNext
        ? `${profile.leagueTown} · ${profile.finale}`
        : leagueEarned
          ? `${profile.finale} ${profile.finale === 'Red' ? 'defeated' : 'reached'}`
          : `${profile.gyms[0].badge} Badge · ${profile.gyms[0].town}`,
    fill: leagueEarned ? 1 : (lastEarned + 1) / goalCount
  }
}

export function kantoRouteLine(run: Pick<SpectatorRun, 'goal' | 'player' | 'stats'> | null | undefined): RouteLine {
  return buildRouteLine(run, KANTO_PROFILE)
}

export function gameRouteLine(
  run: Pick<SpectatorRun, 'game' | 'goal' | 'player' | 'stats'> | null | undefined
): RouteLine {
  return buildRouteLine(run, profileForGame(run))
}
