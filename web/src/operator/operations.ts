import type { DashboardRun, DashboardStats } from '../shared/api/types'
import { playSpeedLabel } from '../shared/playstyle'
import { GAME_BOY_FPS, formatRunSpeed, runTimingSummary } from '../shared/runTiming'

export type StatusTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger'

export function partitionOperationRuns(runs: DashboardRun[], recentLimit = 12) {
  const waiting = runs
    .filter((run) => run.status === 'queued' || run.status === 'leased')
    .sort((a, b) => Number(a.queued_at || 0) - Number(b.queued_at || 0))
  const active = runs
    .filter((run) => run.status === 'running')
    .sort((a, b) => Number(a.queued_at || 0) - Number(b.queued_at || 0))
  const recent = runs
    .filter((run) => run.status === 'done')
    .sort((a, b) => Number(b.ended_at || 0) - Number(a.ended_at || 0))
    .slice(0, Math.max(0, recentLimit))
  return { waiting, active, recent }
}

export function shortID(value: string | undefined, length = 10): string {
  const text = String(value || '')
  return text.length > length ? `${text.slice(0, length)}…` : text || '—'
}

export function shortRevision(value: string | undefined): string {
  return String(value || '').slice(0, 7) || '—'
}

export function goalLabel(run: DashboardRun): string {
  return (run.goal || run.dest || '').trim() || 'Free play'
}

export function gameTitle(game: string | undefined): string {
  switch ((game || 'pokemon-red').toLowerCase()) {
    case 'pokemon-blue': return 'Pokémon Blue'
    case 'pokemon-yellow': return 'Pokémon Yellow'
    case 'pokemon-red':
    default: return 'Pokémon Red'
  }
}

export function llmProfileLabel(run: DashboardRun): string {
  const identity = run.inference
  if (identity?.label) {
    return identity.compute ? `${identity.label}` : identity.label
  }
  if (run.llm_deployment) return run.llm_deployment
  switch ((run.llm_profile || '').toLowerCase()) {
    case 'auto': return '7900 XTX · default · CPU after 120s'
    case 'gpu': return 'RTX 4090 · manual'
    case 'default': return 'CPU only · manual'
    default: return run.planner === 'llm' ? 'Endpoint default' : 'Scripted'
  }
}

export function outcomeLabel(run: DashboardRun): string {
  return (run.reason || run.status || 'done').trim().replaceAll('_', ' ')
}

export function statusTone(status: string | undefined): StatusTone {
  switch ((status || '').toLowerCase()) {
    case 'running': return 'success'
    case 'leased': return 'info'
    case 'queued': return 'warning'
    case 'done': return 'neutral'
    case 'error':
    case 'failed':
    case 'lost': return 'danger'
    default: return 'neutral'
  }
}

export function isLiveStatus(status: string | undefined): boolean {
  const value = (status || '').toLowerCase()
  return value === 'running' || value === 'leased'
}

export function gameMediaLabel(status: string | undefined): string {
  switch ((status || '').toLowerCase()) {
    case 'running': return 'Live'
    case 'leased': return 'Leased · starting'
    case 'queued': return 'Waiting for a worker'
    case 'done': return 'Ended · last frame'
    default: return 'Last recorded frame'
  }
}

export function railStatusLabel(run: Pick<DashboardRun, 'status' | 'reason'>): string {
  if (run.status === 'done') return 'ended'
  return run.status || 'unknown'
}

export function railFacts(run: DashboardRun): string {
  if (run.status === 'queued' || run.status === 'leased') return 'waiting for a worker'
  const timing = runTimingSummary(run)
  if (run.status === 'done') {
    const reason = (run.reason || 'ended').trim().replaceAll('_', ' ')
    const age = run.ended_at ? ageLabel(run.ended_at) : ''
    const ended = age ? `ended · ${reason} · ${age} ago` : `ended · ${reason}`
    return timing ? `${ended} · ${timing}` : ended
  }
  const fps = fpsLabel(run)
  return `live${timing ? ` · ${timing}` : ''} · frame ${run.frame ?? 0}${fps ? ` · ${fps}` : ''} · attempt ${run.attempts ?? 0}`
}

export function outcomeTone(run: DashboardRun): StatusTone {
  const outcome = (run.reason || '').toLowerCase()
  if (!outcome || outcome === 'done' || outcome.includes('complete') || outcome.includes('success')) return 'success'
  if (outcome.includes('cancel')) return 'neutral'
  if (outcome.includes('limit') || outcome.includes('timeout')) return 'warning'
  return 'danger'
}

export function ageLabel(unixSeconds: number | undefined, nowSeconds = Date.now() / 1000): string {
  if (!unixSeconds) return '—'
  const seconds = Math.max(0, Math.round(nowSeconds - unixSeconds))
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 48) return `${hours}h ${minutes % 60}m`
  return `${Math.floor(hours / 24)}d ${hours % 24}h`
}

export function formatFrame(frame: number | undefined): string {
  return Number(frame || 0).toLocaleString()
}

export function howText(run: DashboardRun): string {
  return run.planner === 'scripted' ? 'walk to a place' : 'play the game'
}

export function starterLabel(run: DashboardRun): string {
  return run.starter || (run.planner === 'scripted' ? 'squirtle' : 'LLM picks')
}

export function reasoningEffortLabel(run: DashboardRun): string {
  switch ((run.reasoning_effort || '').toLowerCase()) {
    case 'off': return 'Off (no thinking)'
    case 'low': return 'Low'
    case 'medium': return 'Medium'
    case 'high': return 'High (slowest)'
    default: return 'Auto (endpoint default)'
  }
}

// decisionEngineLabel describes the run's fast typed-decision selection. A run
// without one uses the runner default, which is off in the shipped stack.
export function decisionEngineLabel(run: Pick<DashboardRun, 'decision_engine'>): string {
  const engine = run.decision_engine
  if (!engine || engine.backend === 'off' || engine.mode === 'off') return 'Off'
  const legacyName = engine.backend === 'jev' ? 'TypeSafe Jev' : 'Local System-1'
  const name = engine.inference?.label || engine.deployment || legacyName
  const mode = engine.mode === 'shadow' ? 'shadow' : 'active'
  const uses = [
    engine.battles ? 'battles' : '',
    engine.objectives ? 'objectives' : '',
    engine.failures ? 'recovery' : ''
  ].filter(Boolean)
  const confidence = engine.min_confidence ? ` · ≥${engine.min_confidence.toFixed(2)}` : ''
  return `${name} · ${mode} · ${uses.length ? uses.join(' + ') : 'no features'}${confidence}`
}

export function tileLabel(run: Pick<DashboardRun, 'map' | 'x' | 'y'>): string {
  const map = `0x${Number(run.map || 0).toString(16).padStart(2, '0')}`
  return `${map} (${Number(run.x || 0)},${Number(run.y || 0)})`
}

export function formatWhen(unixSeconds: number | undefined, nowSeconds = Date.now() / 1000): string {
  const value = Number(unixSeconds || 0)
  if (!value) return ''
  const elapsed = Math.max(0, Math.round(nowSeconds - value))
  const relative = elapsed >= 86400
    ? `${Math.floor(elapsed / 86400)}d ago`
    : elapsed >= 3600
      ? `${Math.floor(elapsed / 3600)}h ago`
      : elapsed >= 60
        ? `${Math.floor(elapsed / 60)}m ago`
        : elapsed >= 5
          ? `${elapsed}s ago`
          : 'just now'
  const clock = new Date(value * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  return `${clock} ${relative}`
}

export function fpsLabel(run: DashboardRun): string {
  if (run.status === 'done' && run.ended_at && Number(run.frame || 0) > 0) {
    const duration = Number(run.ended_at) - Number(run.queued_at || 0)
    if (duration > 1) {
      const framesPerSecond = Number(run.frame) / duration
      return `${formatRunSpeed(framesPerSecond / GAME_BOY_FPS)} avg · ${framesPerSecond.toFixed(1)} FPS`
    }
  }

  const speed = playSpeedLabel(run)
  if (speed === 'MAX') return 'MAX · uncapped'
  return `${speed} target · ${Number(run.fps || 0)} FPS`
}

export function statsLine(run: DashboardRun): string {
  const stats = run.stats
  if (!stats || run.planner === 'scripted') return ''
  const left = stats.rounds_left ? ` (${stats.rounds_left} left)` : ''
  return `round ${stats.round ?? '—'}${left} · rep ${stats.repeats ?? 0}/${stats.rounds ?? 0} · call ${Number(stats.avg_seconds || 0).toFixed(1)}s avg`
}

export function statNumber(stats: DashboardStats | undefined, key: string): number {
  return Number(stats?.[key] || 0)
}
