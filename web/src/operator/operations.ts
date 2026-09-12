import type { DashboardRun } from '../shared/api/types'

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

export function llmProfileLabel(run: DashboardRun): string {
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
