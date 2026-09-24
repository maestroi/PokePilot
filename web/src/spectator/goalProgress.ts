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

function isLeagueCompletionGoal(run: SpectatorRun): boolean {
  return isSpeedrun(run)
    || /elite four|champion|hall of fame/.test(goalSearchText(run))
}

function hasMilestone(run: SpectatorRun, phrase: string): boolean {
  return (run.player?.milestones || [])
    .some((milestone) => milestone.toLowerCase().includes(phrase.toLowerCase()))
}

export function isGoalComplete(run: SpectatorRun): boolean {
  return Boolean(run.stats?.goal_complete || hasMilestone(run, 'hall of fame'))
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
