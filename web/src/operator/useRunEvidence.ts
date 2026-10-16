import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { getRunArtifacts, getRunCheckpoints, getRunDebug } from '../shared/api/client'
import type { RunArtifact } from '../shared/api/types'
import { useRunMedia } from './useRunMedia'

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
// artifacts, plus its media outputs (useRunMedia). Live runs poll the debug
// bundle. Selection follows the newest event while followLive is on.
export function useRunEvidence(runID: Ref<string>) {
  const media = useRunMedia(runID)
  const loading = ref(false)
  const error = ref('')
  const debug = ref<EvidenceRow | null>(null)
  const artifacts = ref<RunArtifact[]>([])
  const checkpoints = ref<unknown[]>([])
  const selectedEvent = ref(-1)
  const followLive = ref(true)
  let serial = 0
  let liveTimer = 0

  function stopLive(): void {
    if (liveTimer) window.clearTimeout(liveTimer)
    liveTimer = 0
  }

  function scheduleLive(): void {
    stopLive()
    liveTimer = window.setTimeout(() => { void refreshLive() }, 2000)
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
      if (isLive.value) scheduleLive()
    }
  }

  async function load(): Promise<void> {
    const id = runID.value
    if (!id) return
    const current = ++serial
    loading.value = true
    error.value = ''
    const [debugResult, artifactsResult, checkpointsResult] = await Promise.allSettled([
      getRunDebug(id),
      getRunArtifacts(id),
      getRunCheckpoints(id)
    ])
    if (current !== serial) return
    if (debugResult.status === 'fulfilled') debug.value = debugResult.value
    else {
      debug.value = null
      error.value = debugResult.reason instanceof Error ? debugResult.reason.message : 'Debug bundle unavailable'
    }
    artifacts.value = artifactsResult.status === 'fulfilled' ? artifactsResult.value : []
    checkpoints.value = checkpointsResult.status === 'fulfilled' ? checkpointsResult.value : []
    followLive.value = true
    selectLatest()
    loading.value = false
    if (isLive.value) scheduleLive()
  }

  watch(runID, () => {
    stopLive()
    debug.value = null
    selectedEvent.value = -1
    void load()
  }, { immediate: true })
  onBeforeUnmount(stopLive)

  return {
    loading, error, debug, run, isLive, artifacts, checkpoints, timeline, totalFrames,
    selectedEvent, followLive, select, setFollowLive,
    ...media,
    load
  }
}

export type RunEvidence = ReturnType<typeof useRunEvidence>
