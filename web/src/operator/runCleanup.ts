import type { DashboardRun, TriageGroup } from '../shared/api/types'

export const FAILURE_PATTERN_CAP = 128
export const DELETE_CONCURRENCY = 3

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
  failed: number
}

export interface DeleteFailure {
  id: string
  error: string
}

export interface DeleteRunsResult {
  deletedIds: string[]
  failures: DeleteFailure[]
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
  return (Array.isArray(runs) ? runs : [])
    .filter((run) =>
      run
      && run.status === 'done'
      && (run.reason === 'error' || run.reason === 'lost')
      && run.detail
      && normalizeFailureDetail(run.detail) === wanted)
    .sort((a, b) => Number(a.ended_at || 0) - Number(b.ended_at || 0))
}

export function matchingRunsForGroups(runs: DashboardRun[] | null | undefined, groups: TriageGroup[]): DashboardRun[] {
  const seen = new Set<string>()
  const matched: DashboardRun[] = []
  for (const group of groups) {
    for (const run of bugGroupRuns(runs, groupPattern(group))) {
      if (seen.has(run.run_id)) continue
      seen.add(run.run_id)
      matched.push(run)
    }
  }
  return matched.sort((a, b) => Number(a.ended_at || 0) - Number(b.ended_at || 0))
}

export function eligibleRuns(runs: DashboardRun[] | null | undefined, nowSeconds: number, ageSeconds: number): DashboardRun[] {
  const now = Number(nowSeconds) || 0
  const age = Number(ageSeconds) || 0
  if (now <= 0 || age <= 0) return []
  const cutoff = now - age
  return (Array.isArray(runs) ? runs : [])
    .filter((run) => run && run.status === 'done' && Number(run.ended_at || 0) > 0 && Number(run.ended_at) <= cutoff)
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
        failures.push({ id, error: error instanceof Error ? error.message : String(error) })
      } finally {
        completed++
        onProgress?.({ completed, total: queue.length, deleted: deletedIds.length, failed: failures.length })
      }
    }
  }

  await Promise.all(Array.from({ length: workerCount }, () => worker()))
  return { deletedIds, failures }
}

export function cleanupProgressText(progress: DeleteProgress): string {
  const failed = progress.failed ? ` · ${progress.failed} failed` : ''
  return `Deleting ${progress.completed}/${progress.total} · ${progress.deleted} deleted${failed}`
}

export function cleanupResultText(result: DeleteRunsResult): string {
  if (!result.failures.length) {
    return `${result.deletedIds.length} run${result.deletedIds.length === 1 ? '' : 's'} deleted with artifacts.`
  }
  const examples = result.failures.slice(0, 3).map((failure) => failure.id).join(', ')
  const extra = result.failures.length > 3 ? ', …' : ''
  return `${result.deletedIds.length} deleted · ${result.failures.length} failed${examples ? ` (${examples}${extra})` : ''}. Failed runs were kept so cleanup can be retried.`
}

export function groupCleanupLabel(group: TriageGroup): string {
  const issue = group.issue?.issue_number ? `issue #${group.issue.issue_number}` : `triage ${group.key}`
  return issue
}
