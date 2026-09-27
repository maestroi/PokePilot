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

export function isTetrisRun(run: SpectatorRun | null | undefined): boolean {
  return Boolean(run && (run.game === 'tetris' || run.game_state?.kind === 'tetris'))
}

export function gameTitle(run: SpectatorRun): string {
  if (isTetrisRun(run)) return 'Tetris'
  switch ((run.game || '').toLowerCase()) {
    case 'pokemon-blue': return 'Pokémon Blue'
    case 'pokemon-yellow': return 'Pokémon Yellow'
    default: return 'Pokémon Red'
  }
}

function titleCase(value: string): string {
  return value
    .replaceAll('_', ' ')
    .replaceAll('-', ' ')
    .replace(/\b\w/g, (letter) => letter.toUpperCase())
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
  if (isTetrisRun(run)) {
    const state = run.game_state
    if (run.goal) return run.goal
    const mode = state?.mode ? titleCase(state.mode) : 'Autonomous play'
    return `${mode} · ${Number(state?.lines_cleared || 0)} lines`
  }
  const route = [run.starter, run.dest].filter(Boolean).join(' → ')
  return route || run.goal || `${gameTitle(run)} run`
}

export function objectiveLabel(run: SpectatorRun): string {
  return run.stats?.goal_summary || run.goal || run.stop_so_far || 'Waiting for a goal'
}

export function locationLabel(run: SpectatorRun): string {
  if (isTetrisRun(run)) {
    const state = run.game_state
    const level = Number(state?.level || 0)
    const lines = Number(state?.lines_cleared || 0)
    if (state?.game_over) return `Game over · ${lines} lines`
    if (state?.complete) return `Goal complete · ${lines} lines`
    if (state?.paused) return `Paused · level ${level}`
    return `Level ${level} · ${lines} lines`
  }
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
  if (isTetrisRun(run)) {
    const score = Number(run.game_state?.score || 0).toLocaleString()
    const lines = Number(run.game_state?.lines_cleared || 0)
    return `Tetris · ${score} pts · ${lines} lines`
  }
  const lead = run.starter || run.player?.party?.[0]?.name || gameTitle(run)
  const badges = run.player?.badges?.length || 0
  const badgeText = badges === 1 ? '1 badge' : `${badges} badges`
  return `${playStyleLabel(run)} · ${lead} · ${badgeText}`
}

export function shortRunID(runID: string): string {
  return runID.length > 16 ? `${runID.slice(0, 16)}…` : runID
}
