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
