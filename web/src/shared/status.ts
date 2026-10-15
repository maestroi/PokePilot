// One presentation for every backend status the operator console shows.
// Status is never colour alone: each value gets a glyph and a word as well
// (docs/design/DESIGN.md §4.3). Unknown values stay visible as a neutral ■
// with their raw label, so a new backend state never disappears.

export type StatusTone = 'ok' | 'warn' | 'danger' | 'accent' | 'neutral'
export type StatusPresentation = { label: string; tone: StatusTone; glyph: string; live?: boolean }

type RunLike = {
  status?: string
  reason?: string
  stats?: { goal_complete?: boolean } | null
}

const FAILURE_REASONS = new Set(['error', 'failed', 'stuck', 'objective_failed', 'stagnation', 'blackout', 'lost', 'crashed'])

function humanize(value: string): string {
  const text = value.trim().replaceAll('_', ' ')
  return text ? text[0].toUpperCase() + text.slice(1) : 'Unknown'
}

export function presentOutcome(run: RunLike): StatusPresentation {
  const reason = (run.reason || '').toLowerCase()
  if (run.stats?.goal_complete || reason === 'done' || reason === 'goal' || reason === 'goal_complete') {
    return { label: 'Goal complete', tone: 'ok', glyph: '✓' }
  }
  if (FAILURE_REASONS.has(reason)) return { label: humanize(reason), tone: 'danger', glyph: '✕' }
  if (!reason) return { label: 'Ended', tone: 'neutral', glyph: '■' }
  return { label: humanize(reason), tone: 'neutral', glyph: '■' }
}

export function presentRunStatus(run: RunLike): StatusPresentation {
  const status = (run.status || '').toLowerCase()
  switch (status) {
    case 'running': return { label: 'Running', tone: 'ok', glyph: '●', live: true }
    case 'paused': return { label: 'Paused', tone: 'warn', glyph: '‖' }
    case 'queued': return { label: 'Queued', tone: 'neutral', glyph: '○' }
    case 'leased': return { label: 'Starting', tone: 'accent', glyph: '◐' }
    case 'done': return presentOutcome(run)
    case 'error':
    case 'failed':
    case 'lost': return { label: humanize(status), tone: 'danger', glyph: '✕' }
    default: return { label: humanize(status || 'unknown'), tone: 'neutral', glyph: '■' }
  }
}

export function presentReplay(state: string | undefined, progress?: number): StatusPresentation {
  switch ((state || '').toLowerCase()) {
    case 'ready': return { label: 'Replay ready', tone: 'ok', glyph: '✓' }
    case 'generating': return {
      label: progress !== undefined && Number.isFinite(progress) ? `Generating ${Math.round(progress)}%` : 'Generating',
      tone: 'accent',
      glyph: '◐'
    }
    case 'error':
    case 'failed': return { label: 'Replay failed', tone: 'danger', glyph: '✕' }
    case 'disabled': return { label: 'Replay unavailable', tone: 'neutral', glyph: '■' }
    case 'missing':
    case '': return { label: 'No replay yet', tone: 'neutral', glyph: '○' }
    default: return { label: humanize(state || ''), tone: 'neutral', glyph: '■' }
  }
}

export function toneClass(tone: StatusTone): string {
  return `ui-tone-${tone}`
}
