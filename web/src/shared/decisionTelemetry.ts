import type { DecisionEngineSpec, DecisionKindSummary, DecisionTelemetryStats, TypedDecisionRecord } from './api/types'

// Typed-decision telemetry is two things: a fixed-size per-run summary that
// is stored with the run, and a short live feed of the most recent calls that
// is never stored. These helpers turn both into display rows.

const KIND_LABELS: Record<string, string> = {
  objective_selection: 'Objectives',
  failure_recovery: 'Recovery',
  battle_turn: 'Battles'
}

export function decisionKindLabel(kind: string | undefined): string {
  if (!kind) return '—'
  return KIND_LABELS[kind] || kind.replace(/_/g, ' ')
}

export interface DecisionKindRow {
  kind: string
  label: string
  calls: number
  shadow: number
  // Agreement over shadow answers that could be compared; null when none.
  agreement: number | null
  judged: number
  fallbacks: number
  errors: number
  p50: number
  p95: number
  avgSeconds: number
  topEngine: [string, number][]
  topExecuted: [string, number][]
}

export interface CalibrationBin {
  from: number
  to: number
  answers: number
  judged: number
  agreed: number
  // Agreement rate within the bin; null when no shadow verdicts landed here.
  rate: number | null
}

function top(counts: Record<string, number> | undefined, n = 3): [string, number][] {
  return Object.entries(counts || {}).sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).slice(0, n)
}

export function decisionKindRows(stats: DecisionTelemetryStats | undefined): DecisionKindRow[] {
  const kinds = stats?.decision_summary?.kinds || {}
  return Object.entries(kinds)
    .map(([kind, k]) => kindRow(kind, k))
    .sort((a, b) => b.calls - a.calls || a.kind.localeCompare(b.kind))
}

function kindRow(kind: string, k: DecisionKindSummary): DecisionKindRow {
  const agreements = Number(k.agreements || 0)
  const judged = agreements + Number(k.disagreements || 0)
  const calls = Number(k.calls || 0)
  return {
    kind,
    label: decisionKindLabel(kind),
    calls,
    shadow: Number(k.shadow || 0),
    agreement: judged > 0 ? agreements / judged : null,
    judged,
    fallbacks: Number(k.fallbacks || 0),
    errors: Number(k.errors || 0),
    p50: Number(k.p50_seconds || 0),
    p95: Number(k.p95_seconds || 0),
    avgSeconds: calls > 0 ? Number(k.latency_seconds || 0) / calls : 0,
    topEngine: top(k.engine_choices),
    topExecuted: top(k.executed_choices)
  }
}

// calibration answers "when the engine is this confident, how often does it
// agree with what actually ran?" — per confidence bucket.
export function decisionCalibration(k: DecisionKindSummary | undefined): CalibrationBin[] {
  const answers = k?.confidence || []
  const n = answers.length || 10
  return Array.from({ length: n }, (_, i) => {
    const judged = Number(k?.confidence_judged?.[i] || 0)
    const agreed = Number(k?.confidence_agreed?.[i] || 0)
    return {
      from: i / n,
      to: (i + 1) / n,
      answers: Number(answers[i] || 0),
      judged,
      agreed,
      rate: judged > 0 ? agreed / judged : null
    }
  })
}

export interface DecisionFeedRow {
  kind: string
  choice: string
  confidence: number
  executed: string
  verdict: 'agreed' | 'disagreed' | 'active' | 'unusable'
  seconds: number
  error: string
  // Why the call produced no usable answer, in plain words; '' on success.
  failure: string
}

const ERROR_KIND_LABELS: Record<string, string> = {
  timeout: 'timed out',
  invalid_answer: 'answer rejected',
  low_confidence: 'below confidence floor',
  credentials: 'no credentials',
  backend: 'backend error'
}

// decisionFailure names why a call failed. Records from runners that predate
// error_kind still read as failed, just without a reason.
export function decisionFailure(r: TypedDecisionRecord): string {
  if (!r.error && !r.error_kind) return ''
  return ERROR_KIND_LABELS[r.error_kind || ''] || 'failed'
}

// decisionFeed returns the live feed newest first.
export function decisionFeed(stats: DecisionTelemetryStats | undefined): DecisionFeedRow[] {
  const records: TypedDecisionRecord[] = stats?.decision_records || []
  return records.slice().reverse().map((r) => ({
    kind: decisionKindLabel(r.kind),
    choice: r.choice_label || r.choice || '—',
    confidence: Number(r.confidence || 0),
    executed: r.executed || '',
    verdict: r.error || r.error_kind
      ? 'unusable'
      : r.shadow
        ? (r.agreed === true ? 'agreed' : r.agreed === false ? 'disagreed' : 'unusable')
        : 'active',
    seconds: Number(r.duration_seconds || 0),
    error: r.error || '',
    failure: decisionFailure(r)
  }))
}

export function hasDecisionTelemetry(stats: DecisionTelemetryStats | undefined): boolean {
  return Boolean(stats?.decision_summary?.kinds && Object.keys(stats.decision_summary.kinds).length)
    || Boolean(stats?.decision_records?.length)
}

// decisionEngineSelected is true when the run's spec asks a fast decision
// engine anything. Such a run shows the panel before its first call, so an
// idle engine reads as idle instead of as a missing panel.
// decisionPausedNote explains a run whose battle shadow calls stopped.
export function decisionPausedNote(stats: DecisionTelemetryStats | undefined): string {
  return stats?.decision_battles_paused
    ? 'Battle calls paused for the rest of this run after 3 failures in a row.'
    : ''
}

export function decisionEngineSelected(engine: DecisionEngineSpec | undefined): boolean {
  return Boolean(engine && engine.backend !== 'off' && engine.mode !== 'off')
}

export function showDecisionTelemetry(stats: DecisionTelemetryStats | undefined, engine: DecisionEngineSpec | undefined): boolean {
  return hasDecisionTelemetry(stats) || decisionEngineSelected(engine)
}

// decisionIdleNote says when a selected engine will first be asked, for the
// panel of a run that has made no calls yet.
export function decisionIdleNote(engine: DecisionEngineSpec | undefined): string {
  if (!decisionEngineSelected(engine)) return 'No fast decision engine selected.'
  const points = [
    engine?.objectives ? 'every objective choice' : '',
    engine?.battles && engine?.mode === 'shadow' ? 'every battle move' : '',
    engine?.failures ? 'recoverable failures' : ''
  ].filter(Boolean)
  if (!points.length) return 'No decision points enabled, so the engine is never asked.'
  return `No calls yet. The engine is asked on ${points.join(', ')}.`
}

export function percent(value: number | null): string {
  return value === null ? '—' : `${Math.round(value * 100)}%`
}

export function latency(seconds: number): string {
  if (!seconds) return '—'
  return seconds < 1 ? `${Math.round(seconds * 1000)}ms` : `${seconds.toFixed(1)}s`
}
