import type { DashboardQuery } from '../shared/api/types'

// Runs page state lives in the operator search string next to `run` (the
// inspected run), so reload and back/forward restore it. Facet keys are the
// archive API's own parameter names, which keeps the mapping a straight copy.
export const ARCHIVE_FACETS = ['game', 'outcome', 'model', 'goal', 'deployment', 'play_style', 'how', 'starter', 'compute', 'experiment'] as const
export type ArchiveFacet = typeof ARCHIVE_FACETS[number]
export const ARCHIVE_SORTS = ['finished', 'progress', 'runtime', 'frames', 'rounds', 'planner_time', 'strategic_calls'] as const
export type ArchiveSort = typeof ARCHIVE_SORTS[number]
export type SortDirection = 'asc' | 'desc'

export type ArchiveState = {
  q: string
  facets: Record<ArchiveFacet, string>
  success: boolean
  sort: ArchiveSort
  dir: SortDirection
  page: number // 1-based
}

export interface ArchiveDashboardQuery extends DashboardQuery {
  model?: string
  deployment?: string
  goal?: string
  play_style?: string
  compute?: string
  experiment?: string
  success?: boolean
  sort?: string
  direction?: string
}

const DEFAULT_SORT: ArchiveSort = 'finished'
const DEFAULT_DIR: SortDirection = 'desc'

export function emptyFacets(): Record<ArchiveFacet, string> {
  return Object.fromEntries(ARCHIVE_FACETS.map((key) => [key, ''])) as Record<ArchiveFacet, string>
}

export function parseArchiveState(search: string): ArchiveState {
  const params = new URLSearchParams(search)
  const facets = emptyFacets()
  for (const key of ARCHIVE_FACETS) facets[key] = params.get(key)?.trim() || ''
  const sort = params.get('sort') as ArchiveSort
  const page = Math.floor(Number(params.get('page')))
  return {
    q: params.get('q')?.trim() || '',
    facets,
    success: params.get('success') === '1',
    sort: ARCHIVE_SORTS.includes(sort) ? sort : DEFAULT_SORT,
    dir: params.get('dir') === 'asc' ? 'asc' : (params.get('dir') === 'desc' ? 'desc' : DEFAULT_DIR),
    page: page > 1 ? page : 1
  }
}

// Writes the archive keys into `search`, leaving every other key (run, job,
// experiment links…) intact. Defaults are omitted so plain #runs stays clean.
export function formatArchiveSearch(state: ArchiveState, search = ''): string {
  const params = new URLSearchParams(search)
  const set = (key: string, value: string) => value ? params.set(key, value) : params.delete(key)
  set('q', state.q)
  for (const key of ARCHIVE_FACETS) set(key, state.facets[key])
  set('success', state.success ? '1' : '')
  set('sort', state.sort === DEFAULT_SORT ? '' : state.sort)
  set('dir', state.dir === DEFAULT_DIR ? '' : state.dir)
  set('page', state.page > 1 ? String(state.page) : '')
  const query = params.toString()
  return query ? '?' + query : ''
}

export function archiveDashboardQuery(state: ArchiveState, pageSize: number): ArchiveDashboardQuery {
  return {
    status: 'done',
    search: state.q,
    limit: pageSize,
    offset: (state.page - 1) * pageSize,
    facets: true,
    ...state.facets,
    success: state.success || undefined,
    sort: state.sort,
    direction: state.dir
  }
}

// Views are presets over the same state; one is active only on an exact match.
export const ARCHIVE_VIEWS = [
  { id: 'all', label: 'All runs', success: false, sort: DEFAULT_SORT, dir: DEFAULT_DIR },
  { id: 'successful', label: 'Successful', success: true, sort: DEFAULT_SORT, dir: DEFAULT_DIR },
  { id: 'fastest', label: 'Fastest successful', success: true, sort: 'frames', dir: 'asc' }
] as const satisfies readonly { id: string; label: string; success: boolean; sort: ArchiveSort; dir: SortDirection }[]

export function viewState(view: typeof ARCHIVE_VIEWS[number]): ArchiveState {
  return { q: '', facets: emptyFacets(), success: view.success, sort: view.sort, dir: view.dir, page: 1 }
}

export function activeView(state: ArchiveState): string {
  const filtered = state.q || ARCHIVE_FACETS.some((key) => state.facets[key])
  if (filtered) return ''
  return ARCHIVE_VIEWS.find((view) => view.success === state.success && view.sort === state.sort && view.dir === state.dir)?.id || ''
}
