import { onBeforeUnmount, ref, watch, type Ref } from 'vue'
import {
  cancelMediaRenderJob,
  getHighlightStatus,
  getStudioPackages,
  getReplayStatus,
  renderHighlights,
  renderReplay
} from '../shared/api/client'
import type { HighlightStatus, ReplayStatus, StudioPackageSummary } from '../shared/api/types'

// Media outputs of one run: replay and highlight reel.
// Shared by Live's inspector (via useRunEvidence) and Media's Library/Studio.
// Generating outputs poll their own status until they settle.
export function useRunMedia(runID: Ref<string>) {
  const replay = ref<ReplayStatus | null>(null)
  const replayError = ref('')
  const highlights = ref<HighlightStatus | null>(null)
  const highlightError = ref('')
  const packages = ref<StudioPackageSummary[] | null>(null)
  const packagesError = ref('')
  const rendering = ref(false)
  const renderingHighlights = ref(false)
  const cancellingReplay = ref(false)
  let serial = 0
  const timers: Record<'replay' | 'highlight', number> = { replay: 0, highlight: 0 }

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


  // Stored episode packages (and their thumbnails). Null with an error when
  // the replay host has no studio storage, so callers can say why once.
  async function refreshPackages(): Promise<void> {
    const id = runID.value
    if (!id) return
    try {
      const next = await getStudioPackages(id)
      if (id !== runID.value) return
      packages.value = next.packages || []
      packagesError.value = ''
    } catch (cause) {
      if (id !== runID.value) return
      packages.value = null
      packagesError.value = cause instanceof Error ? cause.message : 'Studio storage unavailable'
    }
  }

  async function load(): Promise<void> {
    const id = runID.value
    if (!id) return
    const current = ++serial
    replayError.value = ''
    highlightError.value = ''
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
    void refreshPackages()
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


  function stopAll(): void {
    for (const name of Object.keys(timers) as (keyof typeof timers)[]) clear(name)
  }

  watch(runID, () => {
    stopAll()
    replay.value = null
    highlights.value = null
    packages.value = null
    packagesError.value = ''
    void load()
  }, { immediate: true })
  onBeforeUnmount(stopAll)

  return {
    replay, replayError, rendering, cancellingReplay, requestReplay, cancelReplay,
    highlights, highlightError, renderingHighlights, requestHighlights,
    packages, packagesError, refreshPackages,
    loadMedia: load
  }
}

export type RunMedia = ReturnType<typeof useRunMedia>
