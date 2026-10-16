import type { DashboardRun } from '../shared/api/types'

export function archiveWhen(run: DashboardRun): string {
  const seconds = Number(run.ended_at || 0)
  if (!seconds) return '—'
  return new Date(seconds * 1000).toISOString()
}

export function archiveAgo(run: DashboardRun, nowMs = Date.now()): string {
  return timeAgo(Number(run.ended_at || 0), nowMs)
}

export function timeAgo(seconds: number, nowMs = Date.now()): string {
  if (!seconds) return '—'
  const minutes = Math.max(0, Math.round((nowMs / 1000 - seconds) / 60))
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes} min ago`
  const hours = Math.round(minutes / 60)
  if (hours < 48) return `${hours} h ago`
  return `${Math.round(hours / 24)} d ago`
}

export function archiveHow(run: DashboardRun): string {
  return run.planner === 'scripted' ? 'walk' : 'play'
}

export function plannerLabel(run: DashboardRun): string {
  return run.planner === 'scripted' ? 'Scripted' : 'LLM'
}

export function archiveStarter(run: DashboardRun): string {
  const shown = starterPresentation(run)
  if (!shown) return ''
  return shown.detail ? `${shown.species} · ${shown.detail}` : shown.species
}

export function isPokemonRun(run: Pick<DashboardRun, 'game'>): boolean {
  const game = (run.game || '').toLowerCase()
  return !game || game.startsWith('pokemon')
}

// starterPresentation is the Pokémon Oak gave this run. Random experiments
// prefer the resolved species; the request ("random", "random:any") stays as
// a quieter detail. Non-Pokémon games have no starter.
export function starterPresentation(run: DashboardRun): { species: string; detail: string } | null {
  if (!isPokemonRun(run)) return null
  const request = (run.starter || '').trim()
  const resolved = (run.starter_species || '').trim().toLowerCase()
  const game = (run.game || 'pokemon-red').toLowerCase()
  if (resolved) return { species: resolved, detail: starterRequestDetail(request, resolved) }
  if (game === 'pokemon-yellow') return { species: 'pikachu', detail: '' }
  const concrete = concreteStarterRequest(request)
  if (concrete) return { species: concrete, detail: starterRequestDetail(request, concrete) }
  if (run.planner === 'scripted') return { species: 'squirtle', detail: '' }
  if (!request) return { species: 'LLM picks', detail: '' }
  return { species: request, detail: '' }
}

export function partyMonIsStarter(name: string, species: string): boolean {
  const starter = species.trim().toLowerCase()
  if (!starter || starter === 'llm picks' || starter === 'random' || starter.startsWith('random:')) return false
  return name.trim().toLowerCase() === starter
}

function concreteStarterRequest(request: string): string {
  const raw = request.trim().toLowerCase()
  if (!raw || raw === 'random' || raw.startsWith('random:')) return ''
  if (raw.startsWith('fixed:')) return raw.slice('fixed:'.length).trim()
  return raw
}

function starterRequestDetail(request: string, species: string): string {
  const raw = request.trim().toLowerCase()
  if (!raw || raw === species) return ''
  if (raw === 'random' || raw.startsWith('random:') || raw.startsWith('fixed:')) return raw
  return ''
}

export function archiveWhere(run: DashboardRun): string {
  if (run.planner === 'scripted' && run.dest) return run.dest
  const map = Number(run.map || 0).toString(16).padStart(2, '0')
  return `map 0x${map} · ${Number(run.x || 0)},${Number(run.y || 0)}`
}

export function safeIssueURL(run: DashboardRun): string {
  const raw = run.issue?.issue_url || ''
  if (!raw) return ''
  try {
    const parsed = new URL(raw)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.toString() : ''
  } catch {
    return ''
  }
}

/** `run-3d9va38e…98h2`: keeps both ends so similar IDs stay distinguishable. */
export function middleTruncate(value: string, head = 12, tail = 4): string {
  return value.length > head + tail + 1 ? `${value.slice(0, head)}…${value.slice(-tail)}` : value
}

export function modelName(run: DashboardRun): string {
  return run.inference?.label || run.inference?.model_id || String(run.stats?.model || run.llm_profile || '')
}

export function modelID(run: DashboardRun): string {
  return run.inference?.model_id || String(run.stats?.model || run.llm_profile || '')
}

export function deploymentName(run: DashboardRun): string {
  return run.llm_deployment || run.inference?.deployment_id || ''
}

export function computeName(run: DashboardRun): string {
  return run.inference?.compute || ''
}

export function statNumber(run: DashboardRun, key: string): number {
  return Number(run.stats?.[key] || 0)
}

export function plannerSeconds(run: DashboardRun): number {
  const strategic = statNumber(run, 'strategic_seconds')
  if (strategic > 0) return strategic
  const average = statNumber(run, 'avg_seconds')
  const calls = statNumber(run, 'calls')
  return average > 0 && calls > 0 ? average * calls : 0
}

export function rounds(run: DashboardRun): number {
  return statNumber(run, 'rounds') || statNumber(run, 'round')
}

export function strategicCalls(run: DashboardRun): number {
  return statNumber(run, 'strategic_calls') || statNumber(run, 'calls')
}

export function formatInteger(value: number): string {
  return Number.isFinite(value) ? Math.round(value).toLocaleString('en-US') : '—'
}

/** Durations read `1h 04m`, `12m 05s`, `42s` (DESIGN §7). */
export function formatDuration(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '—'
  if (value < 60) return `${value < 10 ? value.toFixed(1) : Math.round(value)}s`
  const total = Math.round(value)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  if (hours) return `${hours}h ${String(minutes).padStart(2, '0')}m`
  return `${minutes}m ${String(total % 60).padStart(2, '0')}s`
}

export function progressLabel(run: DashboardRun): string {
  const current = statNumber(run, 'goal_current')
  const target = statNumber(run, 'goal_target')
  if (target > 0) return `${formatInteger(current)}/${formatInteger(target)}`
  const badges = run.player?.badges?.length || 0
  if (badges > 0) return `${badges} badge${badges === 1 ? '' : 's'}`
  if (run.stats?.goal_complete) return 'Goal complete'
  return 'No structured progress'
}

export function experimentLabel(run: DashboardRun): string {
  if (!run.experiment_id) return ''
  return run.experiment_arm ? `${run.experiment_id} · ${run.experiment_arm}` : run.experiment_id
}
