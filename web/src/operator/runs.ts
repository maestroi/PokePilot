import type { DashboardRun } from '../shared/api/types'

export function archiveWhen(run: DashboardRun): string {
  const seconds = Number(run.ended_at || 0)
  if (!seconds) return '—'
  return new Date(seconds * 1000).toLocaleString()
}

export function archiveHow(run: DashboardRun): string {
  return run.planner === 'scripted' ? 'walk' : 'play'
}

export function archiveStarter(run: DashboardRun): string {
  return run.starter || (run.planner === 'scripted' ? 'squirtle' : 'LLM picks')
}

export function archiveWhere(run: DashboardRun): string {
  if (run.planner === 'scripted' && run.dest) return run.dest
  const map = Number(run.map || 0).toString(16).padStart(2, '0')
  return `map 0x${map} · ${Number(run.x || 0)},${Number(run.y || 0)}`
}

export function archiveOutcome(run: DashboardRun): string {
  return run.detail || run.reason || 'done'
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

export function legacyRunURL(runID: string): string {
  const url = new URL('/', window.location.origin)
  url.searchParams.set('run', runID)
  url.hash = 'live'
  return url.toString()
}
