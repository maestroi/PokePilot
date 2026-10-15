import type { DashboardRun, MediaRenderJob } from '../shared/api/types'
import { operatorHref } from '../shared/urls.ts' // .ts: node --test imports this file directly

export type RecordingState = 'ready' | 'queued' | 'rendering' | 'syncing' | 'failed' | 'cancelled' | 'unavailable' | 'unknown'
export type RecordingSummary = {
  state: RecordingState
  label: string
  detail: string
  tone: 'success' | 'info' | 'warning' | 'danger' | 'neutral'
  canWatch: boolean
  mediaSection: 'library' | 'jobs'
  jobID: string
}

const activeStates = new Set(['preparing', 'rendering', 'assembling', 'uploading'])

/** The source run's accessible replay is independent of its gameplay outcome
 * and any rendering job. A 'ready' job is not proof of a playable video.
 * Public visibility belongs to Media and is deliberately not inferred here. */
export function recordingSummary(
  run: Pick<DashboardRun, 'replay_available'>,
  job?: Pick<MediaRenderJob, 'id' | 'state' | 'segments_done' | 'segments_total' | 'stage' | 'last_error'>,
  status: 'ready' | 'loading' | 'stale' | 'error' = 'ready'
): RecordingSummary {
  const jobID = job?.id || ''
  const base = { jobID, canWatch: false, mediaSection: (job ? 'jobs' : 'library') as 'library' | 'jobs' }
  if (run.replay_available) {
    return {
      ...base, canWatch: true, mediaSection: 'library',
      state: 'ready', label: 'Replay available', tone: 'success',
      detail: job?.state === 'failed' ? 'A prior replay is available; the latest render job failed.' : 'Video is available. Public visibility is managed separately in Media.'
    }
  }
  if (job?.state === 'ready') {
    return { ...base, state: 'syncing', label: 'Render complete · syncing', tone: 'info', detail: 'Renderer finished, but the video is not yet confirmed playable.' }
  }
  if (job?.state === 'queued') {
    return { ...base, state: 'queued', label: 'Render queued', tone: 'info', detail: 'A replay job is awaiting capacity.' }
  }
  if (job && activeStates.has(job.state)) {
    const total = Number(job.segments_total || 0)
    const done = Number(job.segments_done || 0)
    const pct = total > 0 ? ` · ${Math.min(100, Math.max(0, Math.round(done * 100 / total)))}%` : ''
    return { ...base, state: 'rendering', label: `Rendering${pct}`, tone: 'info', detail: job.stage || 'Video generation in progress.' }
  }
  if (job?.state === 'failed') {
    return { ...base, state: 'failed', label: 'Render failed', tone: 'danger', detail: job.last_error || 'Open the render job in Media for details and retry.' }
  }
  if (job?.state === 'cancelled') {
    return { ...base, state: 'cancelled', label: 'Render cancelled', tone: 'warning', detail: 'Open the render job in Media to retry.' }
  }
  if (status !== 'ready') {
    return { ...base, state: 'unknown', label: status === 'loading' ? 'Checking recording…' : 'Recording status unknown', tone: 'warning', detail: 'Recent render job status is unavailable. Check Media for the current result.' }
  }
  return { ...base, state: 'unavailable', label: 'No replay reported', tone: 'neutral', detail: 'No playable video or recent render job was reported. See Media for more.' }
}

/** Media handoff for a run: the exact render job when one is in flight or
 * failed, otherwise the run's library entry. */
export function recordingMediaHref(runID: string, summary: RecordingSummary, search = ''): string {
  return operatorHref('media', {
    section: summary.mediaSection, run: runID,
    job: summary.mediaSection === 'jobs' ? summary.jobID : '',
    output: ''
  }, search)
}
