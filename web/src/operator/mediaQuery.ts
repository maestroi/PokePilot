import type { BroadcastStatus } from '../shared/api/broadcast'
import type { DashboardRun, MediaRenderJob } from '../shared/api/types'
import { operatorRoute } from '../shared/urls.ts' // .ts: node --test imports this file directly

// Media page state: the tab lives in the hash (#media/<tab>), everything else in
// the operator search string next to the keys other pages own, so reload and
// back/forward restore it. `run` and `job` are the shared cross-page keys that
// Runs and Live already link with (`#media/library?run=…`, `#media/jobs?job=…`).
// Media-only filters get prefixed names so Failures' `q`/`state` never leak in.
export const MEDIA_TABS = [
  { id: 'library', label: 'Library' },
  { id: 'studio', label: 'Studio' },
  { id: 'broadcasts', label: 'Broadcasts' },
  { id: 'jobs', label: 'Jobs' }
] as const
export type MediaTab = typeof MEDIA_TABS[number]['id']

export const LIBRARY_VIEWS = [
  { id: 'all', label: 'All' },
  { id: 'finished', label: 'Finished' },
  { id: 'live', label: 'Live' }
] as const
export type LibraryView = typeof LIBRARY_VIEWS[number]['id']
export type RecordingFilter = '' | 'ready' | 'rendering' | 'failed' | 'none'
export type VisibilityFilter = '' | 'public' | 'private'

export const JOB_VIEWS = [
  { id: 'all', label: 'All' },
  { id: 'active', label: 'Active' },
  { id: 'queued', label: 'Queued' },
  { id: 'failed', label: 'Failed' },
  { id: 'ready', label: 'Ready' },
  { id: 'cancelled', label: 'Cancelled' }
] as const
export type JobView = typeof JOB_VIEWS[number]['id']

export type MediaState = {
  tab: MediaTab
  run: string // Library/Studio/Broadcasts: selected source run. Jobs: run filter.
  job: string // Jobs: selected render job
  q: string
  view: LibraryView
  recording: RecordingFilter
  visibility: VisibilityFilter
  jobView: JobView
}

const RECORDING_FILTERS: RecordingFilter[] = ['ready', 'rendering', 'failed', 'none']

export function parseMediaState(hash: string, search: string): MediaState {
  const params = new URLSearchParams(search)
  const job = params.get('job')?.trim() || ''
  const route = operatorRoute(hash)
  let tab = (route.view === 'media' ? route.section : 'library') as MediaTab
  // Legacy links (#media?job=…, #media/replays?job=…) predate the Jobs tab as the
  // job owner. Library never selects a job, so a job always means Jobs.
  if (job && tab === 'library') tab = 'jobs'
  const view = params.get('media_view') as LibraryView
  const recording = params.get('recording') as RecordingFilter
  const visibility = params.get('visibility') as VisibilityFilter
  const jobView = params.get('job_state') as JobView
  return {
    tab,
    run: params.get('run')?.trim() || '',
    job,
    q: params.get('media_q')?.trim() || '',
    view: LIBRARY_VIEWS.some((item) => item.id === view) ? view : 'all',
    recording: RECORDING_FILTERS.includes(recording) ? recording : '',
    visibility: visibility === 'public' || visibility === 'private' ? visibility : '',
    jobView: JOB_VIEWS.some((item) => item.id === jobView) ? jobView : 'all'
  }
}

// Writes the Media keys into `search`, leaving every other key intact, and
// returns a full `?…#media/<tab>` href. Defaults are omitted.
export function formatMediaHref(state: MediaState, search = ''): string {
  const params = new URLSearchParams(search)
  const set = (key: string, value: string) => value ? params.set(key, value) : params.delete(key)
  set('run', state.run)
  set('job', state.job)
  set('media_q', state.q)
  set('media_view', state.view === 'all' ? '' : state.view)
  set('recording', state.recording)
  set('visibility', state.visibility)
  set('job_state', state.jobView === 'all' ? '' : state.jobView)
  const query = params.toString()
  return (query ? '?' + query : '') + '#media/' + state.tab
}

// Switching tabs drops the selection that belongs to the tab being left: a job
// only means something on Jobs. The source run carries over.
export function tabHref(state: MediaState, tab: MediaTab, search = ''): string {
  return formatMediaHref({ ...state, tab, job: tab === 'jobs' ? state.job : '' }, search)
}

// One running run and its actual outbound broadcast state (null when the replay
// host didn't answer for it).
export type BroadcastRow = { run: DashboardRun; status: BroadcastStatus | null; error: string }

// ---- Library -----------------------------------------------------------------

export type LibraryContext = {
  isPublic: (run: DashboardRun) => boolean
  recordingState: (run: DashboardRun) => string // runRecording state
}

export function isLiveRun(run: Pick<DashboardRun, 'status'>): boolean {
  return run.status !== 'done'
}

export function recordingMatches(state: string, filter: RecordingFilter): boolean {
  if (!filter) return true
  if (filter === 'ready') return state === 'ready'
  if (filter === 'rendering') return state === 'queued' || state === 'rendering' || state === 'syncing'
  if (filter === 'failed') return state === 'failed' || state === 'cancelled'
  return state === 'unavailable' || state === 'unknown'
}

export function filterLibrary(runs: DashboardRun[], state: MediaState, ctx: LibraryContext): DashboardRun[] {
  const q = state.q.toLowerCase()
  return runs.filter((run) => {
    if (state.view === 'live' && !isLiveRun(run)) return false
    if (state.view === 'finished' && isLiveRun(run)) return false
    if (state.recording && (isLiveRun(run) || !recordingMatches(ctx.recordingState(run), state.recording))) return false
    if (state.visibility && ctx.isPublic(run) !== (state.visibility === 'public')) return false
    if (!q) return true
    return [run.run_id, run.game, run.goal].some((value) => String(value || '').toLowerCase().includes(q))
  })
}

// Live runs first (newest queued), then finished runs (newest ended).
export function sortLibrary(runs: DashboardRun[]): DashboardRun[] {
  return [...runs].sort((a, b) => {
    if (isLiveRun(a) !== isLiveRun(b)) return isLiveRun(a) ? -1 : 1
    const at = Number(isLiveRun(a) ? a.queued_at : a.ended_at) || 0
    const bt = Number(isLiveRun(b) ? b.queued_at : b.ended_at) || 0
    return bt - at
  })
}

// ---- Public site (spectator visibility, distinct from outbound broadcasts) --------

type PublicRun = Pick<DashboardRun, 'status' | 'replay_available'>

export function canFeature(run: Pick<DashboardRun, 'status'>): boolean {
  return run.status === 'running' || run.status === 'leased'
}

export function visibilityLabel(run: PublicRun, visible: boolean): string {
  if (run.status === 'queued') return visible ? 'Publish on start' : 'Keep private'
  if (run.status === 'done') {
    if (run.replay_available) return visible ? 'Replay public' : 'Replay private'
    return visible ? 'Publish replay' : 'Keep private'
  }
  if (run.status === 'paused') return visible ? 'Public on resume' : 'Keep private'
  return visible ? 'Public live' : 'Private live'
}

export function visibilityTitle(run: PublicRun, visible: boolean): string {
  if (run.status === 'queued') {
    return visible ? 'This run will become public when it starts. Click to keep it private.' : 'Keep this run private when it starts. Click to publish on start.'
  }
  if (run.status === 'done') {
    if (run.replay_available) return visible ? 'This replay is published on the public site. Click to make it private.' : 'This replay is private. Click to publish it.'
    return visible ? 'Publish this replay automatically when media becomes available.' : 'Keep this completed run and any future replay private.'
  }
  return visible ? 'This live run is exposed on the public site. Click to hide it.' : 'This live run is private. Click to publish it.'
}

export function canOpenPublic(run: PublicRun, visible: boolean): boolean {
  if (!visible) return false
  if (run.status === 'done') return Boolean(run.replay_available)
  return run.status !== 'queued'
}

export function publicActionLabel(run: PublicRun, visible: boolean): string {
  if (!visible) return run.status === 'done' ? 'Private replay' : 'Private'
  if (run.status === 'queued') return 'Not live'
  if (run.status === 'done') return run.replay_available ? 'Open replay' : 'Replay not ready'
  return 'Open live'
}

// ---- Jobs --------------------------------------------------------------------

export const ACTIVE_JOB_STATES = new Set(['preparing', 'rendering', 'assembling', 'uploading'])

export function jobInView(job: Pick<MediaRenderJob, 'state'>, view: JobView): boolean {
  if (view === 'all') return true
  if (view === 'active') return ACTIVE_JOB_STATES.has(job.state)
  return job.state === view
}

export function filterJobs(jobs: MediaRenderJob[], state: MediaState): MediaRenderJob[] {
  return jobs.filter((job) => jobInView(job, state.jobView) && (!state.run || job.run_id === state.run))
}

export function retryable(job: Pick<MediaRenderJob, 'state' | 'failure_class'>): boolean {
  if (job.state === 'cancelled') return true
  return job.state === 'failed' && job.failure_class !== 'invalid_request'
}

export function cancellable(job: Pick<MediaRenderJob, 'state'>): boolean {
  return job.state === 'queued' || ACTIVE_JOB_STATES.has(job.state)
}

export function removable(job: Pick<MediaRenderJob, 'state'>): boolean {
  return job.state === 'ready' || job.state === 'failed' || job.state === 'cancelled'
}

export function jobProgress(job: Pick<MediaRenderJob, 'state' | 'segments_done' | 'segments_total'>): number | undefined {
  if (job.state === 'ready') return 100
  const total = Number(job.segments_total || 0)
  if (total <= 0) return undefined
  return Math.max(0, Math.min(100, Math.round(Number(job.segments_done || 0) * 100 / total)))
}

export function rendererLabel(job: Pick<MediaRenderJob, 'mode'>): string {
  if (job.mode === 'broadcast') return 'Composited'
  if (job.mode === 'semantic') return 'Semantic'
  if (job.mode === 'raw') return 'Raw'
  return job.mode || 'Default'
}

export function rendererTitle(job: Pick<MediaRenderJob, 'mode'>): string {
  if (job.mode === 'broadcast') return 'Composited replay renderer. “Broadcast” is the internal compatibility mode name; it does not publish the replay.'
  if (job.mode === 'semantic') return 'Headless semantic replay renderer.'
  return `Replay renderer mode: ${job.mode || 'default'}`
}

export function formatBytes(value?: number): string {
  const bytes = Number(value || 0)
  if (!bytes) return '—'
  const units = ['B', 'KB', 'MB', 'GB']
  let n = bytes
  let unit = 0
  while (n >= 1024 && unit < units.length - 1) {
    n /= 1024
    unit++
  }
  return `${n >= 10 || unit === 0 ? n.toFixed(0) : n.toFixed(1)} ${units[unit]}`
}

export function elapsedLabel(startMs?: number, endMs = Date.now()): string {
  const start = Number(startMs || 0)
  if (!start) return '—'
  const seconds = Math.max(0, Math.floor((endMs - start) / 1000))
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (h) return `${h}h ${String(m).padStart(2, '0')}m`
  return m ? `${m}m ${String(seconds % 60).padStart(2, '0')}s` : `${seconds}s`
}

export function workerLabel(job: Pick<MediaRenderJob, 'worker_id' | 'lease_expires_at_unix_ms'>, nowMs = Date.now()): string {
  if (!job.worker_id) return '—'
  const expires = Number(job.lease_expires_at_unix_ms || 0)
  return expires > 0 && expires <= nowMs ? `${job.worker_id} · lease expired` : job.worker_id
}

export function msISO(ms?: number): string {
  return ms ? new Date(ms).toISOString() : ''
}

// Copies a value and returns the receipt for the page's live region.
export async function copyReceipt(value: string, noun: string): Promise<string> {
  try {
    await navigator.clipboard.writeText(value)
    return `${noun} copied`
  } catch {
    return `Couldn’t copy the ${noun.toLowerCase()}`
  }
}

// Newest job per run, for recording summaries.
export function latestJobByRun(jobs: MediaRenderJob[]): Map<string, MediaRenderJob> {
  const out = new Map<string, MediaRenderJob>()
  for (const job of jobs) {
    const current = out.get(job.run_id)
    if (!current || Number(job.updated_at_unix_ms || 0) > Number(current.updated_at_unix_ms || 0)) out.set(job.run_id, job)
  }
  return out
}
