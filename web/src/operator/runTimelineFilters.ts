export type TimelineRow = Record<string, unknown>

export type EventKind = 'decision' | 'checkpoint' | 'progress' | 'failure' | 'recovery' | 'skill' | 'system' | 'event'
export type TimelineFilter = 'all' | 'attention' | Exclude<EventKind, 'event'>

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

export function eventKind(event: TimelineRow): EventKind {
  const source = text(event.source).toLowerCase()
  const kind = text(event.kind).toLowerCase()
  const type = text(event.type).toLowerCase()
  const value = `${kind} ${type} ${text(event.message)} ${text(event.detail)}`.toLowerCase()

  // Source tags are deliberate semantic ownership. Keep recovery/milestone/LLM
  // events in their own lanes even when their text describes an earlier failure.
  if (source === 'recovery') return 'recovery'
  if (source === 'milestone') return 'progress'
  if (source === 'llm') return 'decision'

  if (kind.includes('checkpoint') || type.includes('checkpoint') || value.includes('checkpoint')) return 'checkpoint'
  if (value.includes('fail') || value.includes('lost') || value.includes('error')) return 'failure'
  if (value.includes('recover') || value.includes('rollback') || value.includes('resume')) return 'recovery'
  if (value.includes('progress') || value.includes('finish') || value.includes('badge')) return 'progress'
  if (value.includes('decision') || value.includes('planning')) return 'decision'
  if (source === 'skill') return 'skill'
  if (source === 'system') return 'system'
  return 'event'
}

// Needs attention means something actually went wrong: a failure, or a recovery
// event that records an attempt stopping (lost, budget) or a circuit opening.
// Routine drain/retry/
// resume events from deploys stay under the Recovery filter, otherwise every
// healthy run shows a non-zero badge.
function needsAttention(event: TimelineRow, kind: EventKind): boolean {
  if (kind === 'failure') return true
  if (kind !== 'recovery') return false
  const recoveryKind = text(event.kind).toLowerCase()
  return recoveryKind === 'failure' || recoveryKind === 'circuit'
}

export function eventMatchesFilter(event: TimelineRow, filter: TimelineFilter): boolean {
  if (filter === 'all') return true
  const kind = eventKind(event)
  if (filter === 'attention') return needsAttention(event, kind)
  return kind === filter
}

export const timelineFilterLabels: Record<TimelineFilter, string> = {
  all: 'All events',
  attention: 'Needs attention',
  recovery: 'Recovery',
  failure: 'Failures',
  decision: 'LLM decisions',
  skill: 'Skills',
  progress: 'Milestones',
  checkpoint: 'Checkpoints',
  system: 'System'
}

// Stable marker shapes shared by the transport, story and inspector.
export const eventGlyphs: Record<EventKind, string> = {
  decision: '●',
  checkpoint: '⚑',
  progress: '★',
  failure: '✕',
  recovery: '▲',
  skill: '◆',
  system: '·',
  event: '·'
}

export const eventKindLabels: Record<EventKind, string> = {
  decision: 'Decision',
  checkpoint: 'Checkpoint',
  progress: 'Milestone',
  failure: 'Failure',
  recovery: 'Recovery',
  skill: 'Skill',
  system: 'System',
  event: 'Event'
}

export function eventTitle(event: TimelineRow): string {
  const kind = eventKind(event)
  const message = text(event.message)
  const checkpoint = text(event.checkpoint) || text(event.name)
  if (kind === 'checkpoint' && checkpoint) return checkpoint
  if (message) return message
  if (kind === 'decision') return 'LLM decision'
  if (kind === 'failure') return 'Attempt failed'
  if (kind === 'recovery') return 'Recovery action'
  if (kind === 'skill') return 'Skill execution'
  if (kind === 'system') return 'System event'
  if (kind === 'progress') return text(event.type).toLowerCase().includes('finish') ? 'Run finished' : 'Progress recorded'
  const raw = text(event.type) || text(event.kind) || 'recorded event'
  return raw.replaceAll('_', ' ').replace(/^\w/, (letter) => letter.toUpperCase())
}

export function eventDetail(event: TimelineRow): string {
  const question = text(event.question)
  const decision = text(event.decision)
  const message = text(event.message)
  const detail = text(event.detail)
  const progress = text(event.progress)
  const kind = eventKind(event)
  if (kind === 'decision') return decision || detail || question || message || 'The planner state changed.'
  if (kind === 'failure') return detail || message || question || 'The current attempt stopped with a failure.'
  if (kind === 'recovery') return detail || message || 'The recovery supervisor changed how the campaign will continue.'
  if (kind === 'skill') return detail || message || 'Deterministic gameplay execution.'
  if (kind === 'system') return detail || message || 'Run lifecycle event.'
  if (kind === 'progress') return progress || detail || message || 'Run progress was persisted.'
  if (detail && detail !== eventTitle(event)) return detail
  if (message && message !== eventTitle(event)) return message
  if (question) return question
  if (kind === 'checkpoint') return event.replayable ? 'Checkpoint with paired agent state.' : 'Persisted checkpoint evidence.'
  return 'No additional semantic detail was persisted.'
}

export function eventTime(event: TimelineRow): string {
  const at = Number(event.at || 0)
  if (at > 0) return new Date(at * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
  const frame = Number(event.frame || 0)
  if (frame > 0) return `f ${frame.toLocaleString()}`
  return '—'
}

export type TimelineFilterCounts = Record<TimelineFilter, number>

export function timelineFilterCounts(events: TimelineRow[]): TimelineFilterCounts {
  const counts: TimelineFilterCounts = {
    all: events.length,
    attention: 0,
    decision: 0,
    checkpoint: 0,
    progress: 0,
    failure: 0,
    recovery: 0,
    skill: 0,
    system: 0
  }

  for (const event of events) {
    const kind = eventKind(event)
    if (kind !== 'event') counts[kind] += 1
    if (needsAttention(event, kind)) counts.attention += 1
  }
  return counts
}
