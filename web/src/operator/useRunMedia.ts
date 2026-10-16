import { onBeforeUnmount, ref, watch, type Ref } from 'vue'
import {
  cancelMediaRenderJob,
  getHighlightStatus,
  getReplayStatus,
  getYouTubePublishStatus,
  publishToYouTube,
  renderHighlights,
  renderReplay
} from '../shared/api/client'
import type { HighlightStatus, ReplayStatus, YouTubePublishStatus } from '../shared/api/types'

// Media outputs of one run: replay, highlight reel and YouTube publication.
// Shared by Live's inspector (via useRunEvidence) and Media's Library/Studio.
// Generating outputs poll their own status until they settle.
export function useRunMedia(runID: Ref<string>) {
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
  let serial = 0
  const timers: Record<'replay' | 'highlight' | 'youtube', number> = { replay: 0, highlight: 0, youtube: 0 }

  function clear(name: keyof typeof timers): void {
    if (timers[name]) window.clearTimeout(timers[name])
    timers[name] = 0
  }

  function schedule(name: keyof typeof timers, ms: number, task: () => Promise<void>): void {
    clear(name)
    timers[name] = window.setTimeout(() => { void task() }, ms)
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
    replayError.value = ''
    const [replayResult, highlightResult] = await Promise.allSettled([getReplayStatus(id), getHighlightStatus(id)])
    if (current !== serial) return
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
    replay.value = null
    highlights.value = null
    youtube.value = null
    void load()
  }, { immediate: true })
  onBeforeUnmount(stopAll)

  return {
    replay, replayError, rendering, cancellingReplay, requestReplay, cancelReplay,
    highlights, highlightError, renderingHighlights, requestHighlights,
    youtube, youtubeError, publishing, publishYouTube,
    loadMedia: load
  }
}

export type RunMedia = ReturnType<typeof useRunMedia>
