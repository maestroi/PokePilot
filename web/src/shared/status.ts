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

// `noun` names the output ("Replay", "Highlights"); both use the same backend states.
export function presentReplay(state: string | undefined, progress?: number, noun = 'Replay'): StatusPresentation {
  switch ((state || '').toLowerCase()) {
    case 'ready': return { label: `${noun} ready`, tone: 'ok', glyph: '✓' }
    case 'generating': return {
      label: progress !== undefined && Number.isFinite(progress) ? `Generating ${Math.round(progress)}%` : 'Generating',
      tone: 'accent',
      glyph: '◐'
    }
    case 'error':
    case 'failed': return { label: `${noun} failed`, tone: 'danger', glyph: '✕' }
    case 'disabled': return { label: `${noun} unavailable`, tone: 'neutral', glyph: '■' }
    case 'empty': return { label: 'Nothing to show', tone: 'neutral', glyph: '■' }
    case 'missing':
    case '': return { label: `No ${noun.toLowerCase()} yet`, tone: 'neutral', glyph: '○' }
    default: return { label: humanize(state || ''), tone: 'neutral', glyph: '■' }
  }
}

export function toneClass(tone: StatusTone): string {
  return `ui-tone-${tone}`
}

type FailureGroupLike = {
  issue?: { verification_state?: string; circuit_open?: boolean } | null
}

// A triage group's state. `resolved` comes from the caller's actionable rule
// (isResolvedGroup) so this module stays free of triage policy.
export function presentFailureGroup(group: FailureGroupLike, resolved: boolean): StatusPresentation {
  if (resolved) return { label: 'Resolved', tone: 'ok', glyph: '✓' }
  if (!group.issue) return { label: 'Needs issue', tone: 'warn', glyph: '▲' }
  if ((group.issue.verification_state || '').toLowerCase() === 'regressed') return { label: 'Regressed', tone: 'danger', glyph: '✕' }
  if (group.issue.circuit_open) return { label: 'Quarantined', tone: 'warn', glyph: '▲' }
  return { label: 'Open', tone: 'danger', glyph: '✕' }
}

const JOB_ACTIVE = new Set(['preparing', 'rendering', 'assembling', 'uploading'])

// A durable media render job. Active stages share one glyph; the stage name
// stays visible as the label so the operator sees where the job is.
export function presentJob(state: string | undefined, progress?: number): StatusPresentation {
  const value = (state || '').toLowerCase()
  if (JOB_ACTIVE.has(value)) {
    const pct = progress !== undefined && Number.isFinite(progress) ? ` ${Math.round(progress)}%` : ''
    return { label: `${humanize(value)}${pct}`, tone: 'accent', glyph: '◐' }
  }
  switch (value) {
    case 'queued': return { label: 'Queued', tone: 'neutral', glyph: '○' }
    case 'ready': return { label: 'Ready', tone: 'ok', glyph: '✓' }
    case 'failed': return { label: 'Failed', tone: 'danger', glyph: '✕' }
    case 'cancelled': return { label: 'Cancelled', tone: 'neutral', glyph: '■' }
    default: return { label: humanize(value || 'unknown'), tone: 'neutral', glyph: '■' }
  }
}

// Outbound broadcast state exactly as the replay host reports it. A running
// game or an accepted start request is never "Live" (10-14 spec, Broadcasts).
export function presentBroadcast(state: string | undefined): StatusPresentation {
  switch ((state || '').toLowerCase()) {
    case 'live':
    case 'healthy': return { label: 'Live', tone: 'ok', glyph: '●', live: true }
    case 'starting': return { label: 'Starting', tone: 'accent', glyph: '◐' }
    case 'reconnecting': return { label: 'Reconnecting', tone: 'warn', glyph: '◐' }
    case 'stopping': return { label: 'Stopping', tone: 'accent', glyph: '◐' }
    case 'failed': return { label: 'Failed', tone: 'danger', glyph: '✕' }
    case 'stopped':
    case 'offline':
    case '': return { label: 'Offline', tone: 'neutral', glyph: '○' }
    default: return { label: humanize(state || ''), tone: 'neutral', glyph: '■' }
  }
}

export function isBroadcastOnAir(state: string | undefined): boolean {
  const value = (state || '').toLowerCase()
  return value === 'live' || value === 'healthy'
}

type RecordingLike = { state: string; label: string; tone: 'success' | 'info' | 'warning' | 'danger' | 'neutral' }
const RECORDING_PILL: Record<RecordingLike['tone'], Pick<StatusPresentation, 'tone' | 'glyph'>> = {
  success: { tone: 'ok', glyph: '✓' },
  info: { tone: 'accent', glyph: '◐' },
  warning: { tone: 'warn', glyph: '▲' },
  danger: { tone: 'danger', glyph: '✕' },
  neutral: { tone: 'neutral', glyph: '○' }
}

// A run's recording summary (runRecording.ts) as a pill.
export function presentRecording(summary: RecordingLike): StatusPresentation {
  return { label: summary.label, ...RECORDING_PILL[summary.tone] }
}
