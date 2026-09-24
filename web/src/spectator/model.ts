import type { SpectatorRun } from '../shared/api/spectator'
import {
  normalizePlayStyle,
  playSpeedLabel as configuredPlaySpeedLabel,
  playStyleLabel,
  playStyleTagline,
  type PlayStyle
} from '../shared/playstyle'
import { preferredRun } from './preferredRun'

export type SpectatorPlayStyle = PlayStyle
export { normalizePlayStyle, playStyleLabel, playStyleTagline, preferredRun }

export type SpectatorTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger'

export function isLiveRun(run: SpectatorRun | null | undefined): boolean {
  return Boolean(run && (run.status === 'running' || run.status === 'leased'))
}

export function splitSpectatorRuns(runs: SpectatorRun[]) {
  const live = [...runs]
    .filter(isLiveRun)
    .sort((a, b) => Number(b.queued_at || 0) - Number(a.queued_at || 0))
  const recent = [...runs]
    .filter((run) => run.status === 'done')
    .sort((a, b) => Number(b.ended_at || 0) - Number(a.ended_at || 0))
  return { live, recent }
}

export function runTone(run: SpectatorRun): SpectatorTone {
  if (run.status === 'running' || run.status === 'leased') return 'success'
  if (run.status === 'queued') return 'warning'
  if (run.replay_ready) return 'info'
  if (run.reason && !run.reason.includes('cancel')) return 'danger'
  return 'neutral'
}

export function runStatusLabel(run: SpectatorRun): string {
  if (isLiveRun(run)) return 'live'
  if (run.status === 'done' && run.replay_ready) return 'replay'
  return (run.highlight || run.reason || run.status || 'unknown').replaceAll('_', ' ')
}

export function playSpeedLabel(run: SpectatorRun): string {
  return configuredPlaySpeedLabel(run)
}

export function routeLabel(run: SpectatorRun): string {
  const route = [run.starter, run.dest].filter(Boolean).join(' → ')
  return route || run.goal || 'Pokémon Red run'
}

export function objectiveLabel(run: SpectatorRun): string {
  return run.stats?.goal_summary || run.goal || run.stop_so_far || 'Waiting for a goal'
}

export function locationLabel(run: SpectatorRun): string {
  const map = Number(run.map || 0).toString(16).padStart(2, '0').toUpperCase()
  return `0x${map} · ${run.x ?? 0},${run.y ?? 0}`
}

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

function isLeagueCompletionGoal(run: SpectatorRun): boolean {
  return normalizePlayStyle(run) === 'speedrun'
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

export function runTitle(run: SpectatorRun): string {
  const lead = run.starter || run.player?.party?.[0]?.name || 'Pokémon Red'
  const badges = run.player?.badges?.length || 0
  const badgeText = badges === 1 ? '1 badge' : `${badges} badges`
  return `${playStyleLabel(run)} · ${lead} · ${badgeText}`
}

export function shortRunID(runID: string): string {
  return runID.length > 16 ? `${runID.slice(0, 16)}…` : runID
}
