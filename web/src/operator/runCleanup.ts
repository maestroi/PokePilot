import type { DashboardRun, TriageGroup } from '../shared/api/types'

export const FAILURE_PATTERN_CAP = 128
export const DELETE_CONCURRENCY = 3

// pokewall composes an objective-failure pattern as
// normalizeDetail(objective + " | " + error)[:128] + " | map=xx". The result
// is longer than any normalizeFailureDetail(run.detail) can be, so an exact
// string comparison can never match one of those groups. The stable identity
// both sides carry is the canonical failure-id marker, so cleanup matches on
// that; groups without a marker keep the normalized-pattern fallback.
const FAILURE_MAP_SUFFIX_RE = /\s\|\smap=[0-9a-fA-F]{2}$/

// Reasons that mark a run as having stopped on a failure. A cleanly finished
// run that still carries an old failure detail must never be deleted.
const FAILURE_REASONS = new Set(['error', 'lost', 'failed', 'stuck'])

export interface FailureGroupIdentity {
  tokens: string[]
  patterns: string[]
}

export const AGE_OPTIONS = [
  { seconds: 60 * 60, label: '1h', description: '1 hour' },
  { seconds: 6 * 60 * 60, label: '6h', description: '6 hours' },
  { seconds: 24 * 60 * 60, label: '24h', description: '24 hours' },
  { seconds: 7 * 24 * 60 * 60, label: '7d', description: '7 days' },
  { seconds: 30 * 24 * 60 * 60, label: '30d', description: '30 days' }
] as const

export const DEFAULT_AGE_SECONDS = 7 * 24 * 60 * 60

export interface DeleteProgress {
  completed: number
  total: number
  deleted: number
  skipped: number
  failed: number
}

export interface DeleteFailure {
  id: string
  error: string
}

export interface DeleteRunsResult {
  deletedIds: string[]
  skippedIds: string[]
  failures: DeleteFailure[]
}

export function isLiveLineageError(error: string): boolean {
  return /resume lineage|still required by an active resume/i.test(error)
}

export function isDeletableFinishedRun(run: DashboardRun | null | undefined): boolean {
  return Boolean(run && run.status === 'done' && !run.resume_protected)
}

export function normalizeFailureDetail(detail: string): string {
  return String(detail || '')
    .replace(/0x[0-9a-fA-F]+/g, '<hex>')
    .replace(/\d+/g, '<n>')
    .slice(0, FAILURE_PATTERN_CAP)
}

export function groupPattern(group: Pick<TriageGroup, 'pattern' | 'detail'> & { example?: string }): string {
  if (group.pattern) return group.pattern
  return normalizeFailureDetail(group.detail || group.example || '')
}

export function isResolvedGroup(group: TriageGroup): boolean {
  const status = String(group.issue?.status || '').toLowerCase()
  const resolution = String(group.issue?.resolution || '').toLowerCase()
  return status === 'resolved' || status === 'fixed' || resolution === 'fixed'
}

export function bugGroupRuns(runs: DashboardRun[] | null | undefined, pattern: string): DashboardRun[] {
  const wanted = String(pattern || '')
  if (!wanted) return []
  return selectGroupRuns(runs, failureGroupIdentity([wanted]))
}

// groupFailureIdentity collects every identity a triage group advertises: the
// failure-id markers it carries and the normalized patterns it may equal. A
// composed objective-failure pattern truncates the marker it embeds, so the
// group's raw `example` is read too — it usually keeps the full marker.
export function groupFailureIdentity(group: Pick<TriageGroup, 'pattern' | 'detail' | 'example' | 'examples'>): FailureGroupIdentity {
  const examples = Array.isArray(group.examples) ? group.examples : []
  return failureGroupIdentity([group.pattern, group.detail, group.example, ...examples])
}

function failureGroupIdentity(texts: Array<string | undefined | null>): FailureGroupIdentity {
  const tokens = new Set<string>()
  const patterns = new Set<string>()
  for (const raw of texts) {
    const text = String(raw || '').trim()
    if (!text) continue
    patterns.add(text)
    const base = text.replace(FAILURE_MAP_SUFFIX_RE, '').trim()
    if (base) patterns.add(base)
    for (const match of text.matchAll(/failure-id:([a-p]{12,64})/g)) tokens.add(match[1])
  }
  return { tokens: [...tokens], patterns: [...patterns] }
}

export function isFailureFinishedRun(run: DashboardRun | null | undefined): boolean {
  return isDeletableFinishedRun(run) && FAILURE_REASONS.has(String(run?.reason || '').toLowerCase())
}

export function runMatchesFailureGroup(run: DashboardRun, identity: FailureGroupIdentity): boolean {
  const detail = String(run.detail || '')
  if (!detail) return false
  if (identity.tokens.some((token) => detail.includes(`failure-id:${token}`))) return true
  const normalized = normalizeFailureDetail(detail)
  return identity.patterns.includes(normalized)
}

function selectGroupRuns(runs: DashboardRun[] | null | undefined, identity: FailureGroupIdentity): DashboardRun[] {
  return (Array.isArray(runs) ? runs : [])
    .filter((run) => isFailureFinishedRun(run) && runMatchesFailureGroup(run, identity))
    .sort((a, b) => Number(a.ended_at || 0) - Number(b.ended_at || 0))
}

export function matchingRunsForGroups(runs: DashboardRun[] | null | undefined, groups: TriageGroup[]): DashboardRun[] {
  const seen = new Set<string>()
  const matched: DashboardRun[] = []
  for (const group of groups) {
    for (const run of selectGroupRuns(runs, groupFailureIdentity(group))) {
      if (seen.has(run.run_id)) continue
      seen.add(run.run_id)
      matched.push(run)
    }
  }
  return matched.sort((a, b) => Number(a.ended_at || 0) - Number(b.ended_at || 0))
}

function finishedBeforeCutoff(run: DashboardRun, cutoff: number): boolean {
  return run.status === 'done' && Number(run.ended_at || 0) > 0 && Number(run.ended_at) <= cutoff
}

export function eligibleRuns(runs: DashboardRun[] | null | undefined, nowSeconds: number, ageSeconds: number): DashboardRun[] {
  const now = Number(nowSeconds) || 0
  const age = Number(ageSeconds) || 0
  if (now <= 0 || age <= 0) return []
  const cutoff = now - age
  return (Array.isArray(runs) ? runs : [])
    .filter((run) => isDeletableFinishedRun(run) && finishedBeforeCutoff(run, cutoff))
    .sort((a, b) => Number(a.ended_at) - Number(b.ended_at))
}

export function lineageBlockedRuns(runs: DashboardRun[] | null | undefined, nowSeconds: number, ageSeconds: number): DashboardRun[] {
  const now = Number(nowSeconds) || 0
  const age = Number(ageSeconds) || 0
  if (now <= 0 || age <= 0) return []
  const cutoff = now - age
  return (Array.isArray(runs) ? runs : [])
    .filter((run) => run && run.resume_protected && finishedBeforeCutoff(run, cutoff))
    .sort((a, b) => Number(a.ended_at) - Number(b.ended_at))
}

export function ageDescription(ageSeconds: number): string {
  const match = AGE_OPTIONS.find((option) => option.seconds === Number(ageSeconds))
  return match ? match.description : `${Math.round(Number(ageSeconds) || 0)} seconds`
}

export async function deleteRuns(
  ids: string[],
  deleteOne: (id: string) => Promise<void>,
  concurrency: number,
  onProgress?: (progress: DeleteProgress) => void
): Promise<DeleteRunsResult> {
  const queue = ids.filter(Boolean)
  const workerCount = Math.max(1, Math.min(queue.length || 1, Number(concurrency) || 1))
  const deletedIds: string[] = []
  const skippedIds: string[] = []
  const failures: DeleteFailure[] = []
  let cursor = 0
  let completed = 0

  async function worker(): Promise<void> {
    while (true) {
      const index = cursor++
      if (index >= queue.length) return
      const id = queue[index]
      try {
        await deleteOne(id)
        deletedIds.push(id)
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error)
        if (isLiveLineageError(message)) skippedIds.push(id)
        else failures.push({ id, error: message })
      } finally {
        completed++
        onProgress?.({
          completed,
          total: queue.length,
          deleted: deletedIds.length,
          skipped: skippedIds.length,
          failed: failures.length
        })
      }
    }
  }

  await Promise.all(Array.from({ length: workerCount }, () => worker()))
  return { deletedIds, skippedIds, failures }
}

export function cleanupProgressText(progress: DeleteProgress): string {
  const skipped = progress.skipped ? ` · ${progress.skipped} kept for live resume` : ''
  const failed = progress.failed ? ` · ${progress.failed} failed` : ''
  return `Deleting ${progress.completed}/${progress.total} · ${progress.deleted} deleted${skipped}${failed}`
}

export function cleanupResultText(result: DeleteRunsResult): string {
  const skipped = result.skippedIds.length
    ? ` · ${result.skippedIds.length} kept because a live endless run can still resume from them`
    : ''
  if (!result.failures.length) {
    return `${result.deletedIds.length} run${result.deletedIds.length === 1 ? '' : 's'} deleted with artifacts${skipped}.`
  }
  const examples = result.failures.slice(0, 3).map((failure) => failure.id).join(', ')
  const extra = result.failures.length > 3 ? ', …' : ''
  return `${result.deletedIds.length} deleted${skipped} · ${result.failures.length} failed${examples ? ` (${examples}${extra})` : ''}. Failed runs were kept so cleanup can be retried.`
}

export function groupCleanupLabel(group: TriageGroup): string {
  const issue = group.issue?.issue_number ? `issue #${group.issue.issue_number}` : `triage ${group.key}`
  return issue
}
