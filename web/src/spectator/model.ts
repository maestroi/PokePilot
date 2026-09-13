import type { SpectatorRun } from '../shared/api/spectator'

export type SpectatorTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger'
export type SpectatorPlayStyle = 'speedrun' | 'adventure' | 'completionist' | 'team_builder'

export function isLiveRun(run: SpectatorRun | null | undefined): boolean {
  return Boolean(run && (run.status === 'running' || run.status === 'leased'))
}

function newest(runs: SpectatorRun[], field: 'queued_at' | 'ended_at'): SpectatorRun | null {
  return [...runs].sort((a, b) => Number(b[field] || 0) - Number(a[field] || 0))[0] || null
}

export function preferredRun(runs: SpectatorRun[], selectedRunID = ''): SpectatorRun | null {
  if (!runs.length) return null
  if (selectedRunID) {
    const selected = runs.find((run) => run.run_id === selectedRunID)
    if (selected) return selected
  }
  return newest(runs.filter((run) => run.status === 'running'), 'queued_at')
    || newest(runs.filter((run) => run.status === 'leased'), 'queued_at')
    || newest(runs.filter((run) => run.status === 'queued'), 'queued_at')
    || newest(runs.filter((run) => run.status === 'done'), 'ended_at')
    || runs[runs.length - 1]
    || null
}

export function splitSpectatorRuns(runs: SpectatorRun[]) {
  const live = [...runs]
    .filter((run) => run.status !== 'done')
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

export function goalProgress(run: SpectatorRun): number {
  const stats = run.stats
  if (!stats) return 0
  if (stats.goal_complete) return 100
  const target = Number(stats.goal_target || 0)
  if (target <= 0) return 0
  return Math.max(0, Math.min(100, 100 * Number(stats.goal_current || 0) / target))
}

export function normalizePlayStyle(run: SpectatorRun | null | undefined): SpectatorPlayStyle {
  switch ((run?.play_style || '').trim().toLowerCase()) {
    case 'adventure':
      return 'adventure'
    case 'completionist':
      return 'completionist'
    case 'team_builder':
    case 'team-builder':
    case 'teambuilder':
      return 'team_builder'
    default:
      return 'speedrun'
  }
}

export function playStyleLabel(run: SpectatorRun | null | undefined): string {
  switch (normalizePlayStyle(run)) {
    case 'adventure':
      return 'Adventure'
    case 'completionist':
      return 'Completionist'
    case 'team_builder':
      return 'Team Builder'
    default:
      return 'Speedrun'
  }
}

export function playStyleTagline(run: SpectatorRun | null | undefined): string {
  switch (normalizePlayStyle(run)) {
    case 'adventure':
      return 'Natural play · exploration and story'
    case 'completionist':
      return 'Optional content · items and interactions'
    case 'team_builder':
      return 'Party growth · catches and training'
    default:
      return 'Progression first · minimal detours'
  }
}

export function playSpeedLabel(run: SpectatorRun | null | undefined): string {
  const fps = Number(run?.fps ?? 0)
  if (fps <= 0) return 'MAX'
  const multiple = fps / 60
  if (Number.isInteger(multiple)) return `${multiple}×`
  return `${multiple.toFixed(1).replace(/\.0$/, '')}×`
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
