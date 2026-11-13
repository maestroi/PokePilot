import type { RunAttempt, RunAttemptProblem } from '../shared/api/types'

export type FixTone = 'ok' | 'accent' | 'warn' | 'muted'

// What the AI fixer will do about one run-story failure row. `key` is the
// triage group (null when nothing was filed); links point at the evidence.
export type FixState = {
  label: string
  detail: string
  tone: FixTone
  key: string | null
  issueURL?: string
  prURL?: string
}

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

// Objective failures are reported by the skill runner as source=skill,
// kind=failed with the objective as the message; only those are filed.
export function isObjectiveFailure(event: Record<string, unknown>): boolean {
  return text(event.source) === 'skill' && text(event.kind) === 'failed'
}

// Issue/PR URLs are written by fixer agents; never bind a javascript: or
// other non-web URL into an href.
export function safeWebURL(value: unknown): string | undefined {
  try {
    const url = new URL(text(value))
    return url.protocol === 'https:' || url.protocol === 'http:' ? url.href : undefined
  } catch {
    return undefined
  }
}

function matchProblem(event: Record<string, unknown>, problems: RunAttemptProblem[]): RunAttemptProblem | undefined {
  const objective = text(event.message)
  const candidates = problems.filter((p) => text(p.objective) === objective)
  const round = Number(event.round) || 0
  return candidates.find((p) => round >= (p.first_round || 0) && round <= (p.last_round || Infinity)) || candidates[0]
}

function fromProblem(p: RunAttemptProblem): FixState {
  const base = { key: p.triage_key, issueURL: safeWebURL(p.issue_url) }
  if (p.dismissed) return { ...base, tone: 'muted', label: 'Dismissed', detail: 'An operator dismissed this triage group; the fixer skips it.' }
  if (p.fixed) {
    return { ...base, tone: 'ok', label: 'Fixed', detail: `Fixed${p.fixed_revision ? ` in ${p.fixed_revision}` : ''}; later runs verify it stays fixed.` }
  }
  const solver = p.solver
  if (solver?.pr_number) {
    return {
      ...base, tone: 'accent', label: `Fixer PR #${solver.pr_number}`, prURL: safeWebURL(solver.pr_url),
      detail: `${solver.backend || 'Fixer'} opened PR #${solver.pr_number}${solver.state ? ` (${solver.state})` : ''}.`
    }
  }
  if (solver) {
    return { ...base, tone: 'accent', label: 'Fixer working', detail: `${solver.backend || 'Fixer'} attempt ${solver.state || 'in progress'}, no PR yet.` }
  }
  if (p.queued) return { ...base, tone: 'warn', label: 'Queued for fixer', detail: 'In the triage queue; the next free fixer replica will claim it.' }
  const why = p.recovered && !p.terminal
    ? 'the run recovered, and it did not repeat often enough in one run to count as a blocker'
    : 'it does not meet the triage queue rule yet'
  return { ...base, tone: 'muted', label: 'Not queued', detail: `Recorded, but not sent to the fixer: ${why}.` }
}

// attempts === null means the attempts endpoint is unavailable: show nothing
// rather than claim a state we cannot see.
export function failureFixState(event: Record<string, unknown>, attempts: RunAttempt[] | null): FixState | null {
  if (!attempts || !isObjectiveFailure(event)) return null
  const attemptNo = Number(event.attempt) || 0
  const attempt = attempts.find((a) => a.attempt === attemptNo)
  if (!attempt) {
    return { tone: 'muted', key: null, label: 'Pending', detail: `Failures are filed when attempt ${attemptNo || 'this'} ends.` }
  }
  const problem = matchProblem(event, attempt.problems || [])
  if (problem) return fromProblem(problem)
  return {
    tone: 'muted', key: null, label: 'Not filed',
    detail: 'No triage record, so the fixer never sees it. Runs from before blackouts and training retreats were filed, or the upload was lost.'
  }
}
