import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import {
  cancelMediaRenderJob,
  getHighlightStatus,
  getReplayStatus,
  getRunArtifacts,
  getRunCheckpoints,
  getRunDebug,
  getYouTubePublishStatus,
  publishToYouTube,
  renderHighlights,
  renderReplay
} from '../shared/api/client'
import type { HighlightStatus, ReplayStatus, RunArtifact, YouTubePublishStatus } from '../shared/api/types'

export type EvidenceRow = Record<string, any>

function object(value: unknown): EvidenceRow {
  return value && typeof value === 'object' ? value as EvidenceRow : {}
}

function finite(value: unknown): number {
  const parsed = Number(value || 0)
  return Number.isFinite(parsed) ? parsed : 0
}

export function eventFrame(value: unknown): number {
  return finite(object(value).frame)
}

// Persisted evidence for one run: debug bundle and timeline, checkpoints,
// artifacts, replay/highlight/YouTube state. Live runs poll the debug bundle;
// generating media polls its own status. Selection follows the newest event
// while followLive is on.
export function useRunEvidence(runID: Ref<string>) {
  const loading = ref(false)
  const error = ref('')
  const debug = ref<EvidenceRow | null>(null)
  const artifacts = ref<RunArtifact[]>([])
  const checkpoints = ref<unknown[]>([])
  const replay = ref<ReplayStatus | null>(null)
  const replayError = ref('')
  const highlights = ref<HighlightStatus | null>(null)
  const highlightError = ref('')
  const youtube = ref<YouTubePublishStatus | null>(null)
  const youtubeError = ref('')
  const rendering = ref(false)
  const renderingHighlights = ref(false)
  const cancellingReplay = ref(false)
  const publishing = ref(false)
  const selectedEvent = ref(-1)
  const followLive = ref(true)
  let serial = 0
  const timers: Record<'live' | 'replay' | 'highlight' | 'youtube', number> = { live: 0, replay: 0, highlight: 0, youtube: 0 }

  function clear(name: keyof typeof timers): void {
    if (timers[name]) window.clearTimeout(timers[name])
    timers[name] = 0
  }

  function schedule(name: keyof typeof timers, ms: number, task: () => Promise<void>): void {
    clear(name)
    timers[name] = window.setTimeout(() => { void task() }, ms)
  }

  const run = computed<EvidenceRow>(() => object(debug.value?.run))
  const isLive = computed(() => ['queued', 'leased', 'running', 'paused'].includes(String(run.value.status || '')))

  const timeline = computed<EvidenceRow[]>(() => {
    const source = Array.isArray(debug.value?.timeline) ? debug.value!.timeline.map(object) : []
    const rows: EvidenceRow[] = source.map((event: EvidenceRow) => ({ ...event }))
    checkpoints.value.forEach((value, index) => {
      const checkpoint = object(value)
      rows.push({
        ...checkpoint,
        type: 'checkpoint',
        kind: 'checkpoint',
        checkpoint: checkpoint.name || checkpoint.kind || checkpoint.label || checkpoint.key || `checkpoint ${index + 1}`
      })
    })
    return rows.sort((a, b) => {
      const aAt = finite(a.at)
      const bAt = finite(b.at)
      if (aAt && bAt && aAt !== bAt) return aAt - bAt
      return eventFrame(a) - eventFrame(b) || finite(a.round) - finite(b.round)
    })
  })

  const totalFrames = computed(() => Math.max(finite(run.value.frame), ...timeline.value.map(eventFrame), 1))

  function selectLatest(): void {
    selectedEvent.value = timeline.value.length ? timeline.value.length - 1 : -1
  }

  function select(index: number): void {
    if (index < 0 || index >= timeline.value.length) return
    selectedEvent.value = index
    followLive.value = index === timeline.value.length - 1 && isLive.value
  }

  function setFollowLive(value: boolean): void {
    followLive.value = value
    if (value) selectLatest()
  }

  async function refreshLive(): Promise<void> {
    const id = runID.value
    if (!id) return
    try {
      const value = await getRunDebug(id)
      if (id !== runID.value) return
      debug.value = value
      if (followLive.value) selectLatest()
    } catch {
      // A full load surfaces errors; background refresh stays quiet.
    } finally {
      if (isLive.value) schedule('live', 2000, refreshLive)
    }
  }

  async function refreshReplay(): Promise<void> {
    const id = runID.value
    const previous = replay.value?.state
    try {
      const next = await getReplayStatus(id)
      if (id !== runID.value) return
      replay.value = next
      replayError.value = ''
      // A replay sidecar restart drops its in-memory job. Finished attempt
      // segments stay cached in S3, so re-requesting resumes where it stopped.
      if (previous === 'generating' && next.state === 'missing') {
        void requestReplay()
        return
      }
    } catch (cause) {
      replayError.value = cause instanceof Error ? cause.message : 'Replay status unavailable'
    }
    if (replay.value?.state === 'generating') schedule('replay', 5000, refreshReplay)
  }

  async function refreshHighlights(): Promise<void> {
    const id = runID.value
    try {
      const next = await getHighlightStatus(id)
      if (id !== runID.value) return
      highlights.value = next
      highlightError.value = ''
    } catch (cause) {
      highlightError.value = cause instanceof Error ? cause.message : 'Highlight status unavailable'
    }
    if (highlights.value?.state === 'generating') schedule('highlight', 5000, refreshHighlights)
  }

  async function refreshYouTube(): Promise<void> {
    const id = runID.value
    clear('youtube')
    try {
      const next = await getYouTubePublishStatus(id)
      if (id !== runID.value) return
      youtube.value = next
      youtubeError.value = ''
    } catch {
      if (id === runID.value) youtube.value = null // replay host without publishing: hide the control
      return
    }
    if (youtube.value?.publication.state === 'uploading') schedule('youtube', 5000, refreshYouTube)
  }

  async function load(): Promise<void> {
    const id = runID.value
    if (!id) return
    const current = ++serial
    loading.value = true
    error.value = ''
    replayError.value = ''
    const [debugResult, artifactsResult, checkpointsResult, replayResult, highlightResult] = await Promise.allSettled([
      getRunDebug(id),
      getRunArtifacts(id),
      getRunCheckpoints(id),
      getReplayStatus(id),
      getHighlightStatus(id)
    ])
    if (current !== serial) return
    if (debugResult.status === 'fulfilled') debug.value = debugResult.value
    else {
      debug.value = null
      error.value = debugResult.reason instanceof Error ? debugResult.reason.message : 'Debug bundle unavailable'
    }
    artifacts.value = artifactsResult.status === 'fulfilled' ? artifactsResult.value : []
    checkpoints.value = checkpointsResult.status === 'fulfilled' ? checkpointsResult.value : []
    if (replayResult.status === 'fulfilled') replay.value = replayResult.value
    else {
      replay.value = null
      replayError.value = replayResult.reason instanceof Error ? replayResult.reason.message : 'Replay service unavailable'
    }
    if (highlightResult.status === 'fulfilled') highlights.value = highlightResult.value
    else {
      highlights.value = null
      highlightError.value = highlightResult.reason instanceof Error ? highlightResult.reason.message : 'Highlight service unavailable'
    }
    followLive.value = true
    selectLatest()
    loading.value = false
    if (isLive.value) schedule('live', 2000, refreshLive)
    if (replay.value?.state === 'generating') schedule('replay', 5000, refreshReplay)
    if (highlights.value?.state === 'generating') schedule('highlight', 5000, refreshHighlights)
    void refreshYouTube()
  }

  async function requestReplay(): Promise<void> {
    if (!runID.value || rendering.value) return
    rendering.value = true
    replayError.value = ''
    try {
      replay.value = await renderReplay(runID.value)
      if (replay.value?.state === 'generating') schedule('replay', 5000, refreshReplay)
    } catch (cause) {
      replayError.value = cause instanceof Error ? cause.message : 'Replay generation failed'
    } finally {
      rendering.value = false
    }
  }

  async function cancelReplay(): Promise<void> {
    const jobID = String(replay.value?.job_id || '')
    if (!jobID || cancellingReplay.value) return
    cancellingReplay.value = true
    replayError.value = ''
    try {
      await cancelMediaRenderJob(jobID)
      await refreshReplay()
    } catch (cause) {
      replayError.value = cause instanceof Error ? cause.message : 'Replay cancel failed'
    } finally {
      cancellingReplay.value = false
    }
  }

  async function requestHighlights(): Promise<void> {
    if (!runID.value || renderingHighlights.value) return
    renderingHighlights.value = true
    highlightError.value = ''
    try {
      highlights.value = await renderHighlights(runID.value)
      if (highlights.value?.state === 'generating') schedule('highlight', 5000, refreshHighlights)
    } catch (cause) {
      highlightError.value = cause instanceof Error ? cause.message : 'Highlight generation failed'
    } finally {
      renderingHighlights.value = false
    }
  }

  async function publishYouTube(privacy: 'private' | 'unlisted' | 'public'): Promise<void> {
    if (!runID.value || publishing.value) return
    publishing.value = true
    youtubeError.value = ''
    try {
      await publishToYouTube(runID.value, privacy)
      await refreshYouTube()
    } catch (cause) {
      youtubeError.value = cause instanceof Error ? cause.message : 'YouTube publish failed'
    } finally {
      publishing.value = false
    }
  }

  function stopAll(): void {
    for (const name of Object.keys(timers) as (keyof typeof timers)[]) clear(name)
  }

  watch(runID, () => {
    stopAll()
    youtube.value = null
    debug.value = null
    selectedEvent.value = -1
    void load()
  }, { immediate: true })
  onBeforeUnmount(stopAll)

  return {
    loading, error, debug, run, isLive, artifacts, checkpoints, timeline, totalFrames,
    selectedEvent, followLive, select, setFollowLive,
    replay, replayError, rendering, cancellingReplay, requestReplay, cancelReplay,
    highlights, highlightError, renderingHighlights, requestHighlights,
    youtube, youtubeError, publishing, publishYouTube,
    load
  }
}

export type RunEvidence = ReturnType<typeof useRunEvidence>
