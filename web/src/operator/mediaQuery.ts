import type { BroadcastStatus } from '../shared/api/broadcast'
import type {
  DashboardRun, HighlightStatus, MediaRenderJob, ReplayStatus, StudioAssessment, StudioEditStatus,
  StudioPackage, StudioPackageSummary, StudioPublicationCandidate, StudioPublicationState
} from '../shared/api/types'
import type { StatusPresentation } from '../shared/status.ts'
import { operatorRoute } from '../shared/urls.ts' // .ts: node --test imports this file directly

// Media page state: the tab lives in the hash (#media/<tab>), everything else in
// the operator search string next to the keys other pages own, so reload and
// back/forward restore it. `run` and `job` are the shared cross-page keys that
// Runs and Live already link with (`#media/library?run=…`, `#media/jobs?job=…`).
// Media-only filters get prefixed names so Failures' `q`/`state` never leak in.
export const MEDIA_TABS = [
  { id: 'library', label: 'Library' },
  { id: 'studio', label: 'Studio' },
  { id: 'e2e', label: 'Full E2E' },
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
  { id: 'ready', label: 'Done' },
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
  return state === 'recorded' || state === 'unavailable' || state === 'unknown'
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

// Newest created first, then ID. Never updated_at: progress heartbeats bump it,
// so concurrently rendering jobs would swap rows on every poll.
export function sortJobs(jobs: MediaRenderJob[]): MediaRenderJob[] {
  return [...jobs].sort((a, b) => (Number(b.created_at_unix_ms || 0) - Number(a.created_at_unix_ms || 0)) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
}

export function filterJobs(jobs: MediaRenderJob[], state: MediaState): MediaRenderJob[] {
  return sortJobs(jobs.filter((job) => jobInView(job, state.jobView) && (!state.run || job.run_id === state.run)))
}

// What a job produces. Replay modes (raw/semantic/broadcast) make the replay video.
export function jobOutputLabel(job: Pick<MediaRenderJob, 'mode'>): string {
  if (job.mode === 'highlight') return 'Highlight reel'
  if (job.mode === 'studio-edit') return 'Episode edit'
  return 'Replay video'
}

export function isReplayJob(job: Pick<MediaRenderJob, 'mode'>): boolean {
  return job.mode !== 'highlight' && job.mode !== 'studio-edit'
}

// The newest render job of each kind per run, for a row's at-a-glance marks.
export type RenderKind = 'video' | 'highlight' | 'edit'
export function renderKind(job: Pick<MediaRenderJob, 'mode'>): RenderKind {
  return job.mode === 'highlight' ? 'highlight' : job.mode === 'studio-edit' ? 'edit' : 'video'
}
export function latestJobsByKind(jobs: MediaRenderJob[]): Map<string, Partial<Record<RenderKind, MediaRenderJob>>> {
  const out = new Map<string, Partial<Record<RenderKind, MediaRenderJob>>>()
  for (const job of jobs) {
    const kinds = out.get(job.run_id) || {}
    const kind = renderKind(job)
    const current = kinds[kind]
    if (!current || Number(job.updated_at_unix_ms || 0) > Number(current.updated_at_unix_ms || 0)) kinds[kind] = job
    out.set(job.run_id, kinds)
  }
  return out
}
// No job on record is "—", not "not rendered": job records can be pruned.
export function presentRenderMark(job?: Pick<MediaRenderJob, 'state' | 'segments_done' | 'segments_total'>): StatusPresentation {
  if (!job) return { label: 'None on record', tone: 'neutral', glyph: '—' }
  if (job.state === 'ready') return OK('Done')
  if (job.state === 'failed') return FAILED('Failed')
  if (job.state === 'cancelled') return NONE('Cancelled')
  const total = Number(job.segments_total || 0)
  return WORKING(job.state === 'queued' ? 'Queued' : total ? `${Number(job.segments_done || 0)}/${total}` : 'Rendering')
}

// Where a finished job's output can be watched: replay video in Library,
// highlight reel in Studio. Episode edits have no UI yet.
export function jobOutputHref(job: Pick<MediaRenderJob, 'mode' | 'run_id' | 'state'>, state: MediaState, search = ''): string {
  if (job.state !== 'ready' || job.mode === 'studio-edit') return ''
  return formatMediaHref({ ...state, tab: job.mode === 'highlight' ? 'studio' : 'library', run: job.run_id, job: '' }, search)
}

// Elapsed since the job started and, when segment progress allows it, a
// linear time-left estimate. No estimate is invented without progress.
export function jobTiming(job: Pick<MediaRenderJob, 'state' | 'started_at_unix_ms' | 'finished_at_unix_ms' | 'segments_done' | 'segments_total'>, nowMs = Date.now()): { elapsed: string; eta: string } {
  const start = Number(job.started_at_unix_ms || 0)
  if (!start) return { elapsed: '', eta: '' }
  const end = Number(job.finished_at_unix_ms || 0) || nowMs
  const elapsed = elapsedLabel(start, end)
  const done = Number(job.segments_done || 0)
  const total = Number(job.segments_total || 0)
  if (!ACTIVE_JOB_STATES.has(job.state) || done <= 0 || total <= done) return { elapsed, eta: '' }
  const leftMs = (end - start) * (total - done) / done
  return { elapsed, eta: `~${elapsedLabel(end - leftMs, end)} left` }
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

// Newest replay-video job per run, for recording summaries. Highlight and
// episode-edit jobs are other outputs and never stand in for the replay video.
export function latestJobByRun(jobs: MediaRenderJob[]): Map<string, MediaRenderJob> {
  const out = new Map<string, MediaRenderJob>()
  for (const job of jobs) {
    if (!isReplayJob(job)) continue
    const current = out.get(job.run_id)
    if (!current || Number(job.updated_at_unix_ms || 0) > Number(current.updated_at_unix_ms || 0)) out.set(job.run_id, job)
  }
  return out
}

// ---- Run outputs: what exists for one run, said precisely ----------------------------

export type OutputRow = { id: 'data' | 'video' | 'highlights' | 'package' | 'thumbnails'; label: string; status: StatusPresentation; detail: string }
export type RunOutputs = { rows: OutputRow[]; unavailable: string[] }

type OutputInput = {
  replayData: boolean // run.replay_available: the recording is stored, not a video
  live: boolean
  replay?: Pick<ReplayStatus, 'state' | 'segments' | 'segments_done'> | null
  highlights?: Pick<HighlightStatus, 'state' | 'windows' | 'windows_done' | 'duration_ms'> | null
  packages?: StudioPackageSummary[] | null // null: not loaded or unavailable
  packagesError?: string
}

const OK = (label: string): StatusPresentation => ({ label, tone: 'ok', glyph: '✓' })
const NONE = (label: string): StatusPresentation => ({ label, tone: 'neutral', glyph: '○' })
const WORKING = (label: string): StatusPresentation => ({ label, tone: 'accent', glyph: '◐' })
const FAILED = (label: string): StatusPresentation => ({ label, tone: 'danger', glyph: '✕' })

function plural(n: number, noun: string): string {
  return `${n} ${noun}${n === 1 ? '' : 's'}`
}

function seconds(ms?: number): string {
  const total = Math.max(0, Math.round(Number(ms || 0) / 1000))
  return total >= 60 ? `${Math.floor(total / 60)}m ${String(total % 60).padStart(2, '0')}s` : `${total}s`
}

// Rows only for outputs this run can have. Anything the host can't render for
// it (feature off, nothing worth cutting, no source data, still live) is not a
// row: it becomes one honest reason in `unavailable`.
export function runOutputs(input: OutputInput): RunOutputs {
  const rows: OutputRow[] = []
  const unavailable: string[] = []
  // A rendered (or rendering) replay video proves its source data exists, even
  // when the run summary isn't loaded yet.
  input = { ...input, replayData: input.replayData || input.replay?.state === 'ready' || input.replay?.state === 'generating' }
  rows.push({
    id: 'data', label: 'Replay data',
    status: input.replayData ? OK('Stored') : NONE(input.live ? 'Recording' : 'None'),
    detail: input.replayData ? 'The run’s recording. Videos are rendered from it.' : input.live ? 'Stored when the run ends.' : 'No recording was stored for this run.'
  })
  if (input.live) {
    unavailable.push('Videos, highlights and packages can be made after the run ends.')
    return { rows, unavailable }
  }

  const replay = input.replay
  if (replay?.state === 'disabled') unavailable.push('Replay video rendering isn’t enabled on the replay host.')
  else if (!input.replayData && (!replay || replay.state === 'missing')) unavailable.push('No replay video can be rendered without replay data.')
  else if (replay) {
    const total = Number(replay.segments || 0)
    const done = Number(replay.segments_done || 0)
    const status = replay.state === 'ready' ? OK('Rendered')
      : replay.state === 'generating' ? WORKING(total ? `Rendering ${done}/${total}` : 'Rendering')
      : replay.state === 'error' || replay.state === 'failed' ? FAILED('Render failed')
      : replay.state === 'missing' ? NONE('Not rendered') : NONE(replay.state)
    rows.push({ id: 'video', label: 'Replay video', status, detail: replay.state === 'ready' ? 'Full-length video, playable here.' : '' })
  }

  const h = input.highlights
  if (h?.state === 'disabled') unavailable.push('Highlight rendering isn’t enabled on the replay host.')
  else if (h?.state === 'empty') unavailable.push('No highlight-worthy moments were found in this run.')
  else if (h) {
    const windows = Number(h.windows || 0)
    const status = h.state === 'ready' ? OK('Rendered')
      : h.state === 'generating' ? WORKING(windows ? `Rendering ${Number(h.windows_done || 0)}/${windows}` : 'Rendering')
      : h.state === 'error' || h.state === 'failed' ? FAILED('Render failed')
      : h.state === 'missing' ? NONE('Not rendered') : NONE(h.state)
    rows.push({ id: 'highlights', label: 'Highlight reel', status, detail: h.state === 'ready' ? `${plural(windows, 'moment')} · ${seconds(h.duration_ms)}` : '' })
  }

  const packages = input.packages
  if (!packages) {
    if (input.packagesError) unavailable.push(`Episode packages and thumbnails aren’t available: ${input.packagesError}.`)
  } else {
    rows.push({
      id: 'package', label: 'Episode package',
      status: packages.length ? OK(plural(packages.length, 'package')) : NONE('None'),
      detail: packages.length ? 'Titles, description and thumbnail concepts grounded in the run.' : ''
    })
    const withThumbs = packages.filter((item) => item.thumbnails)
    if (packages.length) {
      rows.push({
        id: 'thumbnails', label: 'Thumbnails',
        status: withThumbs.some((item) => item.thumbnail_selected) ? OK('Rendered · one selected') : withThumbs.length ? OK('Rendered') : NONE('Not rendered'),
        detail: ''
      })
    }
  }
  return { rows, unavailable }
}

// ---- Studio pipeline: the publishing steps of one finished run -----------------------

export type StudioStepID = 'video' | 'verdict' | 'package' | 'thumbnail' | 'publish'
export type StudioStep = { id: StudioStepID; label: string; status: StatusPresentation; detail: string; done: boolean; optional?: boolean }
export type StudioPipeline = { steps: StudioStep[]; current: StudioStepID | ''; unavailable: string[] }

type StudioInput = {
  replayData: boolean
  replay?: Pick<ReplayStatus, 'state' | 'segments' | 'segments_done'> | null
  packagesError?: string
  assessment?: Pick<StudioAssessment, 'publication_class' | 'publication_reason'> | null
  pkg?: Pick<StudioPackage, 'plan_hash' | 'titles' | 'shorts'> | null
  profile?: 'episode' | 'short' | ''
  edit?: Pick<StudioEditStatus, 'state' | 'error'> | null
  thumbnails?: { rendered: number; selected: boolean }
  publication?: Pick<StudioPublicationCandidate, 'state' | 'reason'> | null // newest candidate of the open package
}

const PUBLICATION: Record<StudioPublicationState, StatusPresentation> = {
  queued: NONE('Awaiting approval'),
  blocked: { label: 'Blocked by quality gate', tone: 'warn', glyph: '▲' },
  rendering: WORKING('Awaiting edited video'),
  approved: OK('Approved · not uploaded'),
  publishing: WORKING('Uploading'),
  published: OK('Published'),
  failed: FAILED('Upload failed'),
  skipped: NONE('Skipped')
}
export function presentPublication(state: StudioPublicationState): StatusPresentation {
  return PUBLICATION[state] || NONE(state)
}

// Studio turns a recording into a published video in a fixed order: replay
// video → producer verdict → edited package → thumbnail → publication. The
// highlight reel is a side output. `current` is the first required step not
// yet done; steps the run or host can't reach collapse into `unavailable`.
export function studioSteps(input: StudioInput): StudioPipeline {
  const steps: StudioStep[] = []
  const unavailable: string[] = []
  const pipeline = (): StudioPipeline => ({ steps, unavailable, current: steps.find((step) => !step.done && !step.optional)?.id || '' })
  const replay = input.replay
  if (!input.replayData && replay?.state !== 'ready' && replay?.state !== 'generating') {
    unavailable.push('No recording was stored for this run, so Studio has nothing to cut.')
    return pipeline()
  }

  if (replay?.state === 'disabled') unavailable.push('Replay video rendering isn’t enabled on the replay host, so edits can’t render.')
  else if (replay) {
    const total = Number(replay.segments || 0)
    const status = replay.state === 'ready' ? OK('Rendered')
      : replay.state === 'generating' ? WORKING(total ? `Rendering ${Number(replay.segments_done || 0)}/${total}` : 'Rendering')
      : replay.state === 'error' || replay.state === 'failed' ? FAILED('Render failed')
      : NONE('Not rendered')
    steps.push({ id: 'video', label: 'Replay video', status, done: replay.state === 'ready', detail: replay.state === 'ready' ? '' : 'Every edit is cut from the full replay video.' })
  }

  // The highlight reel is a side output: nothing below is cut from it, so it isn't a step.
  if (input.packagesError) {
    unavailable.push(`Packaging and publishing aren’t available: ${input.packagesError}.`)
    return pipeline()
  }

  const verdict = input.assessment?.publication_class
  steps.push({
    id: 'verdict', label: 'Producer verdict', done: Boolean(verdict),
    status: verdict === 'full_episode' ? OK('Full episode') : verdict === 'short_only' ? OK('Short only')
      : verdict === 'archive_only' ? NONE('Archive only') : verdict === 'do_not_publish' ? NONE('Do not publish') : NONE('Not assessed'),
    detail: input.assessment?.publication_reason || 'Decides whether the run is worth an episode, a Short, or only the archive.'
  })
  if (verdict === 'archive_only' || verdict === 'do_not_publish') {
    unavailable.push('The producer kept this run off YouTube, so there is nothing to package or publish.')
    return pipeline()
  }

  const pkg = input.pkg
  const edit = input.edit?.state
  const format = input.profile === 'short' ? 'Short' : 'Episode'
  const title = (input.profile === 'short' ? pkg?.shorts : pkg?.titles)?.[0]?.title || ''
  steps.push({
    id: 'package', label: 'Edited package', done: Boolean(pkg?.plan_hash) && edit === 'ready',
    status: !pkg ? NONE('None') : !pkg.plan_hash ? NONE('No edit plan')
      : edit === 'ready' ? OK(`${format} · edit ready`) : edit === 'generating' ? WORKING('Rendering edit')
      : edit === 'error' || edit === 'failed' ? FAILED('Edit failed') : NONE('Edit not rendered'),
    detail: pkg && !pkg.plan_hash ? 'Made without an edit plan. Prepare a new package to publish.' : input.edit?.error || title || 'Titles, description, thumbnail concepts and the edited video.'
  })

  const thumbs = input.thumbnails || { rendered: 0, selected: false }
  steps.push({
    id: 'thumbnail', label: 'Thumbnail', done: thumbs.selected, optional: input.profile === 'short',
    status: thumbs.selected ? OK('Selected') : thumbs.rendered ? NONE('Pick one') : NONE('Not rendered'),
    detail: input.profile === 'short' ? 'Optional for Shorts.' : ''
  })

  const publication = input.publication
  steps.push({
    id: 'publish', label: 'Publication', done: publication?.state === 'published',
    status: publication ? presentPublication(publication.state) : NONE('Not queued'),
    detail: publication?.reason || 'Nothing uploads until you approve and then publish.'
  })
  return pipeline()
}
