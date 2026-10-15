import type { TriageGroup } from '../shared/api/types'
import { isResolvedGroup } from './runCleanup.ts'

// Failures page state lives in the operator search string next to the keys other
// pages own, so reload and back/forward restore it. /v1/triage returns every
// group at once with no server filtering, so filtering, sort and paging happen
// here over the loaded list.
export const FAILURE_VIEWS = [
  { id: 'open', label: 'Open' },
  { id: 'resolved', label: 'Resolved' },
  { id: 'all', label: 'All' }
] as const
export type FailureView = typeof FAILURE_VIEWS[number]['id']
export type IssueFilter = '' | 'linked' | 'none'
export const FAILURE_SORTS = ['count', 'updated'] as const
export type FailureSort = typeof FAILURE_SORTS[number]
export const FAILURES_PAGE_SIZE = 50

export type FailuresState = {
  view: FailureView
  issue: IssueFilter
  q: string
  sort: FailureSort
  page: number // 1-based
  failure: string // inspected group key
}

export function parseFailuresState(search: string): FailuresState {
  const params = new URLSearchParams(search)
  const view = params.get('state') as FailureView
  const issue = params.get('issue') as IssueFilter
  const sort = params.get('sort') as FailureSort
  const page = Math.floor(Number(params.get('page')))
  return {
    view: FAILURE_VIEWS.some((item) => item.id === view) ? view : 'open',
    issue: issue === 'linked' || issue === 'none' ? issue : '',
    q: params.get('q')?.trim() || '',
    sort: FAILURE_SORTS.includes(sort) ? sort : 'count',
    page: page > 1 ? page : 1,
    failure: params.get('failure')?.trim() || ''
  }
}

// Writes the Failures keys into `search`, leaving every other key intact.
// Defaults are omitted so plain #failures stays clean.
export function formatFailuresSearch(state: FailuresState, search = ''): string {
  const params = new URLSearchParams(search)
  const set = (key: string, value: string) => value ? params.set(key, value) : params.delete(key)
  set('state', state.view === 'open' ? '' : state.view)
  set('issue', state.issue)
  set('q', state.q)
  set('sort', state.sort === 'count' ? '' : state.sort)
  set('page', state.page > 1 ? String(state.page) : '')
  set('failure', state.failure)
  const query = params.toString()
  return query ? '?' + query : ''
}

export function exampleRuns(group: TriageGroup): string[] {
  return (group.run_ids || group.runs || group.examples || []).map(String)
}

export function latestRun(group: TriageGroup): string {
  return group.latest_run_id || exampleRuns(group)[0] || ''
}

export function groupTitle(group: TriageGroup): string {
  return group.pattern || group.detail || group.example || group.fingerprint || group.key
}

export function inView(group: TriageGroup, view: FailureView): boolean {
  if (view === 'all') return true
  return isResolvedGroup(group) === (view === 'resolved')
}

export function filterGroups(groups: TriageGroup[], state: FailuresState): TriageGroup[] {
  const q = state.q.toLowerCase()
  const matches = groups.filter((group) => {
    if (!inView(group, state.view)) return false
    if (state.issue === 'linked' && !group.issue) return false
    if (state.issue === 'none' && group.issue) return false
    if (!q) return true
    return [group.key, group.pattern, group.detail, group.fingerprint, ...exampleRuns(group)]
      .some((value) => String(value || '').toLowerCase().includes(q))
  })
  const updated = (group: TriageGroup) => Number(group.issue?.updated_at || 0)
  return matches.sort((a, b) => state.sort === 'updated'
    ? updated(b) - updated(a) || Number(b.count || 0) - Number(a.count || 0)
    : Number(b.count || 0) - Number(a.count || 0) || updated(b) - updated(a))
}

export function canDismiss(group: TriageGroup): boolean {
  return Boolean(group.dismissable && !group.issue)
}

// Why Investigate with AI is unavailable, or '' when it is available.
export function investigateBlocker(group: TriageGroup): string {
  if (!group.issue) return 'Needs a linked issue first'
  if (isResolvedGroup(group)) return 'Already resolved'
  return ''
}

export function solverSummary(group: TriageGroup): string {
  const attempts = group.issue?.solver_attempts || []
  if (!attempts.length) return ''
  const latest = attempts[attempts.length - 1]
  const model = latest.model || latest.backend || 'unknown model'
  const state = String(latest.state || '').replace(/_/g, ' ')
  return `${attempts.length} attempt${attempts.length === 1 ? '' : 's'} · latest ${model}${state ? `, ${state}` : ''}`
}

export function issueURL(group: TriageGroup): string {
  const raw = group.issue?.issue_url || ''
  try {
    const url = new URL(raw)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.toString() : ''
  } catch {
    return ''
  }
}
