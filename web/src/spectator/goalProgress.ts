import type { SpectatorRun } from '../shared/api/spectator'

export interface GoalProgressMeta {
  percent: number
  label: string
  detail: string
  complete: boolean
}

function goalSearchText(run: SpectatorRun): string {
  return [run.goal, run.stats?.goal_summary, run.stop_so_far]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()
}

function isSpeedrun(run: SpectatorRun): boolean {
  switch ((run.play_style || '').trim().toLowerCase()) {
    case 'adventure':
    case 'completionist':
    case 'team_builder':
    case 'team-builder':
    case 'teambuilder':
      return false
    default:
      return true
  }
}

const GEN2_FULL_GAME_GOAL = 'progress:gs_mt_silver_red_defeated'

function isLeagueCompletionGoal(run: SpectatorRun): boolean {
  const goal = (run.goal || '').trim().toLowerCase()
  // A selected structured goal wins over play-style heuristics. Gen2's
  // supported frontier or 8-badge preset is not a League completion goal.
  if (goal) return /elite[- ]four|champion|hall[- ]of[- ]fame/.test(goal)
  return isSpeedrun(run) || /elite four|champion|hall of fame/.test(goalSearchText(run))
}

function hasMilestone(run: SpectatorRun, phrase: string): boolean {
  return (run.player?.milestones || [])
    .some((milestone) => milestone.toLowerCase().includes(phrase.toLowerCase()))
}

export function isGoalComplete(run: SpectatorRun): boolean {
  if (run.stats?.goal_complete) return true
  // Milestones provide a fallback for a delayed stats snapshot, but may not
  // override another goal such as badges:8 or the Gen2 supported frontier.
  return isLeagueCompletionGoal(run) &&
    (run.player?.badges?.length || 0) >= 8 &&
    hasMilestone(run, 'hall of fame')
}

export function goalProgressMeta(run: SpectatorRun): GoalProgressMeta {
  const stats = run.stats
  const badges = run.player?.badges?.length || 0

  if (isGoalComplete(run)) {
    return {
      percent: 100,
      label: 'Goal complete',
      detail: stats?.goal_summary || run.goal || 'Run objective complete',
      complete: true
    }
  }

  // A full-game league run is not complete at badge 8. Reserve the last part
  // of the meter for Victory Road, the Elite Four, the Champion, and Hall of
  // Fame so 8/8 never visually reads as a finished run.
  if (isLeagueCompletionGoal(run)) {
    const badgePercent = Math.min(88, (Math.min(8, badges) / 8) * 88)
    return {
      percent: badgePercent,
      label: `${badges}/8 badges earned`,
      detail: badges >= 8
        ? 'Next: Elite Four, Champion & Hall of Fame'
        : 'Full-game goal in progress',
      complete: false
    }
  }

  // Gold/Silver full game: 16 badges, then Red. The cartridge goal itself is
  // a single event, so show badge progress and keep the end for Red.
  if ((run.goal || '').trim().toLowerCase() === GEN2_FULL_GAME_GOAL) {
    return {
      percent: Math.min(94, (Math.min(16, badges) / 16) * 94),
      label: `${badges}/16 badges earned`,
      detail: badges >= 16 ? 'Next: Red on Mt. Silver' : 'Full-game goal in progress',
      complete: false
    }
  }

  const target = Number(stats?.goal_target || 0)
  if (target <= 0) {
    return {
      percent: 0,
      label: 'Goal in progress',
      detail: stats?.goal_summary || run.goal || 'Waiting for structured goal progress',
      complete: false
    }
  }

  const current = Number(stats?.goal_current || 0)
  return {
    percent: Math.max(0, Math.min(99, 100 * current / target)),
    label: `${current}/${target}`,
    detail: stats?.goal_summary || run.goal || 'Goal in progress',
    complete: false
  }
}

export function goalProgress(run: SpectatorRun): number {
  return goalProgressMeta(run).percent
}
