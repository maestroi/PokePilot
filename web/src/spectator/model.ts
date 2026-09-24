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

export {
  goalProgress,
  goalProgressMeta,
  isGoalComplete,
  type GoalProgressMeta
} from './goalProgress'

export function runTitle(run: SpectatorRun): string {
  const lead = run.starter || run.player?.party?.[0]?.name || 'Pokémon Red'
  const badges = run.player?.badges?.length || 0
  const badgeText = badges === 1 ? '1 badge' : `${badges} badges`
  return `${playStyleLabel(run)} · ${lead} · ${badgeText}`
}

export function shortRunID(runID: string): string {
  return runID.length > 16 ? `${runID.slice(0, 16)}…` : runID
}
