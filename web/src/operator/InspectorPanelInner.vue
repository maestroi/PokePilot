<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { ArrowDownTrayIcon, ArrowPathIcon, FilmIcon } from '@heroicons/vue/20/solid'
import {
  artifactContentURL,
  getReplayStatus,
  getRunArtifacts,
  getRunCheckpoints,
  getRunDebug,
  renderReplay,
  replayVideoURL
} from '../shared/api/client'
import type { ReplayStatus, RunArtifact } from '../shared/api/types'
import Panel from '../shared/components/Panel.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import RunTimeline from './RunTimeline.vue'

const props = defineProps<{ runID: string }>()

type Row = Record<string, any>
type TimelineRow = Record<string, unknown>

const loading = ref(false)
const error = ref('')
const debug = ref<Row | null>(null)
const artifacts = ref<RunArtifact[]>([])
const checkpoints = ref<unknown[]>([])
const replay = ref<ReplayStatus | null>(null)
const replayError = ref('')
const rendering = ref(false)
const selectedEvent = ref(-1)
const video = ref<HTMLVideoElement | null>(null)
const playbackRate = ref(Number(localStorage.getItem('pokepilot.replayPlaybackRate') || 1))
let serial = 0
let liveTimer = 0
let replayTimer = 0

function object(value: unknown): Row {
  return value && typeof value === 'object' ? value as Row : {}
}

function eventFrame(value: unknown): number {
  const frame = Number(object(value).frame || 0)
  return Number.isFinite(frame) ? frame : 0
}

function eventRound(value: unknown): number {
  const round = Number(object(value).round || 0)
  return Number.isFinite(round) ? round : 0
}

function eventAt(value: unknown): number {
  const at = Number(object(value).at || 0)
  return Number.isFinite(at) ? at : 0
}

const run = computed<Row>(() => object(debug.value?.run))
const activity = computed<Row[]>(() => Array.isArray(run.value.activity) ? run.value.activity.map(object) : [])
const latestActivity = computed<Row>(() => activity.value.length ? activity.value[activity.value.length - 1] : {})
const recoveryCount = computed(() => Number(run.value.recovery_attempts || 0))
const runAttempt = computed(() => Number(run.value.attempts || 0) + (String(run.value.status || '') === 'done' ? 0 : 1))
const recoveryMode = computed(() => String(run.value.recovery_profile || 'strict'))
const currentActor = computed(() => {
  if (run.value.question && !run.value.decision) return 'LLM planning'
  const source = String(latestActivity.value.source || '')
  if (source === 'recovery') return 'Recovery'
  if (source === 'llm') return 'LLM'
  if (source === 'skill') return 'Skill'
  if (source === 'milestone') return 'Milestone'
  if (run.value.trace) return 'Skill'
  return 'System'
})
const currentSummary = computed(() => String(latestActivity.value.summary || run.value.stop_so_far || run.value.decision || run.value.trace || 'Waiting for run activity'))
const liveStatus = computed(() => ['queued', 'leased', 'running', 'paused'].includes(String(run.value.status || '')))

function clearLiveTimer(): void {
  if (liveTimer) window.clearTimeout(liveTimer)
  liveTimer = 0
}

function scheduleLiveRefresh(): void {
  clearLiveTimer()
  if (!liveStatus.value || !props.runID) return
  liveTimer = window.setTimeout(() => { void refreshLive() }, 2000)
}

async function refreshLive(): Promise<void> {
  const runID = props.runID
  if (!runID) return
  const followLatest = selectedEvent.value < 0 || selectedEvent.value === timeline.value.length - 1
  try {
    const value = await getRunDebug(runID)
    if (runID !== props.runID) return
    debug.value = value
    if (followLatest) selectedEvent.value = timeline.value.length ? timeline.value.length - 1 : -1
  } catch {
    // Full refresh exposes errors; background live refresh stays unobtrusive.
  } finally {
    scheduleLiveRefresh()
  }
}

const timeline = computed<TimelineRow[]>(() => {
  const value = debug.value || {}
  const source = Array.isArray(value.timeline) ? value.timeline.map(object) : []
  const rows: TimelineRow[] = source.map((event) => ({ ...event }))
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
    const aAt = eventAt(a)
    const bAt = eventAt(b)
    if (aAt && bAt && aAt !== bAt) return aAt - bAt
    return eventFrame(a) - eventFrame(b) || eventRound(a) - eventRound(b)
  })
})

const totalFrames = computed(() => {
  const run = object(debug.value?.run)
  return Math.max(Number(run.frame || 0), ...timeline.value.map(eventFrame), 1)
})

function artifactMeta(artifact: RunArtifact): string {
  const parts = [artifact.media_type, artifact.size ? `${Number(artifact.size).toLocaleString()} bytes` : '', artifact.storage || artifact.location]
  return parts.filter(Boolean).join(' · ') || 'artifact'
}

async function load(): Promise<void> {
  const runID = props.runID
  if (!runID) return
  const id = ++serial
  loading.value = true
  error.value = ''
  replayError.value = ''

  const [debugResult, artifactsResult, checkpointsResult, replayResult] = await Promise.allSettled([
    getRunDebug(runID),
    getRunArtifacts(runID),
    getRunCheckpoints(runID),
    getReplayStatus(runID)
  ])
  if (id !== serial) return

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
  selectedEvent.value = timeline.value.length ? timeline.value.length - 1 : -1
  loading.value = false
  scheduleLiveRefresh()
  scheduleReplayRefresh()
}

async function requestReplay(): Promise<void> {
  if (!props.runID || rendering.value) return
  rendering.value = true
  replayError.value = ''
  try {
    replay.value = await renderReplay(props.runID)
    scheduleReplayRefresh()
  } catch (cause) {
    replayError.value = cause instanceof Error ? cause.message : 'Replay render failed'
  } finally {
    rendering.value = false
  }
}

function clearReplayTimer(): void {
  if (replayTimer) window.clearTimeout(replayTimer)
  replayTimer = 0
}

function scheduleReplayRefresh(): void {
  clearReplayTimer()
  if (replay.value?.state === 'generating') replayTimer = window.setTimeout(() => { void refreshReplay() }, 5000)
}

async function refreshReplay(): Promise<void> {
  const runID = props.runID
  const previous = replay.value?.state
  try {
    const next = await getReplayStatus(runID)
    if (runID !== props.runID) return
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
  scheduleReplayRefresh()
}

function checkpointLabel(value: unknown, index: number): string {
  const row = object(value)
  return String(row.name || row.kind || row.label || row.key || `checkpoint ${index + 1}`)
}

function selectEvent(index: number): void {
  if (index < 0 || index >= timeline.value.length) return
  selectedEvent.value = index
  const node = video.value
  if (!node || !(node.duration > 0) || totalFrames.value <= 0) return
  node.currentTime = node.duration * eventFrame(timeline.value[index]) / totalFrames.value
}

function syncEventFromVideo(): void {
  const node = video.value
  if (!node || !(node.duration > 0) || !timeline.value.length) return
  const frame = totalFrames.value * node.currentTime / node.duration
  let nearest = 0
  for (let index = 1; index < timeline.value.length; index++) {
    if (Math.abs(eventFrame(timeline.value[index]) - frame) < Math.abs(eventFrame(timeline.value[nearest]) - frame)) nearest = index
  }
  selectedEvent.value = nearest
}

function applyPlaybackRate(): void {
  const allowed = [1, 2, 4, 8, 16]
  const value = allowed.includes(Number(playbackRate.value)) ? Number(playbackRate.value) : 1
  playbackRate.value = value
  localStorage.setItem('pokepilot.replayPlaybackRate', String(value))
  if (video.value) video.value.playbackRate = value
}

watch(() => props.runID, () => {
  clearLiveTimer()
  clearReplayTimer()
  void load()
}, { immediate: true })
watch(playbackRate, () => { void nextTick(applyPlaybackRate) })
onBeforeUnmount(() => { clearLiveTimer(); clearReplayTimer() })
</script>

<template>
  <div class="space-y-2">
    <Panel title="Run inspector" description="Live activity, recovery history, checkpoints, artifacts, and deterministic replay." compact>
      <template #actions>
        <button type="button" class="inline-flex items-center gap-1.5 rounded-sm bg-white/8 px-2 py-1 text-[11px] font-semibold text-slate-200 ring-1 ring-white/10 hover:bg-white/12" :disabled="loading" @click="load">
          <ArrowPathIcon class="size-3.5" aria-hidden="true" /> {{ loading ? 'Loading…' : 'Refresh' }}
        </button>
      </template>

      <div v-if="error" class="mb-2 border border-[var(--poke-amber)]/40 bg-[#332d20] p-2 text-[12px] text-[#ddc18c]">{{ error }}</div>

      <div v-if="debug?.run" class="mb-3 flex flex-wrap items-center gap-2 rounded-md border border-white/8 bg-black/15 px-3 py-2">
        <StatusBadge :tone="run.status === 'done' ? 'neutral' : run.status === 'paused' ? 'warning' : 'success'">{{ run.status || 'unknown' }}</StatusBadge>
        <StatusBadge :tone="recoveryMode === 'resilient' ? 'info' : 'neutral'">{{ recoveryMode }}</StatusBadge>
        <StatusBadge v-if="recoveryCount" tone="warning">recovery {{ recoveryCount }}</StatusBadge>
        <span class="h-4 w-px bg-white/10" aria-hidden="true" />
        <span class="text-[10px] font-semibold tracking-[0.06em] text-slate-500 uppercase">{{ currentActor }}</span>
        <strong class="min-w-0 flex-1 truncate text-[11px] font-medium text-slate-300" :title="currentSummary">{{ currentSummary }}</strong>
        <span class="font-mono text-[9px] text-slate-600">attempt {{ runAttempt }}</span>
      </div>

      <div class="grid grid-cols-1 gap-3 2xl:grid-cols-[minmax(0,1.45fr)_minmax(18rem,0.55fr)]">
        <RunTimeline :events="timeline" :total-frames="totalFrames" :selected-index="selectedEvent" @select="selectEvent" />

        <section class="rounded-md border border-white/8 bg-black/10 p-3">
          <div class="flex items-center justify-between gap-3">
            <div>
              <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Playback</span>
              <h3 class="mt-0.5 text-sm font-semibold text-slate-200">Replay</h3>
            </div>
            <StatusBadge v-if="replay" :tone="replay.state === 'ready' ? 'success' : replay.state === 'error' ? 'danger' : replay.state === 'generating' ? 'warning' : 'neutral'">{{ replay.state }}</StatusBadge>
          </div>

          <template v-if="replay?.state === 'ready'">
            <video
              ref="video"
              :key="runID"
              class="mt-3 max-h-64 w-full rounded-md bg-black object-contain [image-rendering:pixelated]"
              :src="replayVideoURL(runID)"
              controls
              preload="metadata"
              playsinline
              @loadedmetadata="applyPlaybackRate"
              @timeupdate="syncEventFromVideo"
            />
            <label class="mt-2 flex items-center justify-end gap-2 text-[10px] text-slate-500">
              Speed
              <select v-model.number="playbackRate" class="rounded-md border-0 bg-white/6 px-2 py-1 text-[11px] text-slate-200 outline-1 -outline-offset-1 outline-white/10">
                <option :value="1">1×</option><option :value="2">2×</option><option :value="4">4×</option><option :value="8">8×</option><option :value="16">16×</option>
              </select>
            </label>
          </template>
          <div v-else class="mt-3 rounded-md border border-dashed border-white/10 px-4 py-6 text-center">
            <FilmIcon class="mx-auto size-6 text-slate-600" aria-hidden="true" />
            <p class="mt-2 text-xs text-slate-500">{{ replay?.state === 'generating' ? `Replay is being rendered${replay.segments ? ` (${replay.segments_done || 0}/${replay.segments} attempts)` : ''}.` : 'Render the canonical .gbrun recording into a seekable MP4 when available.' }}</p>
            <button v-if="replay?.state !== 'disabled'" type="button" class="mt-3 rounded-md bg-cyan-500 px-2.5 py-1.5 text-xs font-semibold text-white hover:bg-cyan-400 disabled:opacity-50" :disabled="rendering || replay?.state === 'generating'" @click="requestReplay">
              {{ rendering ? 'Requesting…' : replay?.state === 'generating' ? 'Generating…' : 'Render replay' }}
            </button>
          </div>
          <p v-if="replayError" class="mt-2 text-xs text-amber-300/80">{{ replayError }}</p>
        </section>
      </div>

      <details class="mt-3 rounded-md border border-white/8 bg-black/10">
        <summary class="cursor-pointer px-3 py-2 text-[11px] font-semibold text-slate-400 hover:text-slate-200">Raw debug bundle <span class="font-normal text-slate-600">· hidden by default</span></summary>
        <pre class="max-h-80 overflow-auto border-t border-white/8 bg-black/20 p-3 font-mono text-[10px] leading-5 text-slate-400">{{ debug ? JSON.stringify(debug, null, 2) : 'No debug bundle loaded.' }}</pre>
      </details>
    </Panel>

    <div class="grid grid-cols-1 gap-3 xl:grid-cols-2">
      <Panel title="Artifacts" :description="`${artifacts.length} indexed artifact${artifacts.length === 1 ? '' : 's'}`" compact>
        <div v-if="artifacts.length" class="max-h-64 divide-y divide-white/8 overflow-y-auto">
          <div v-for="artifact in artifacts" :key="artifact.name" class="flex items-start justify-between gap-3 py-2.5 first:pt-0 last:pb-0">
            <div class="min-w-0">
              <strong class="block truncate font-mono text-xs text-slate-300" :title="artifact.name">{{ artifact.name }}</strong>
              <span class="mt-0.5 block text-[11px] text-slate-600">{{ artifactMeta(artifact) }}</span>
              <span v-if="artifact.sha256" class="mt-0.5 block truncate font-mono text-[9px] text-slate-700" :title="artifact.sha256">sha256 {{ artifact.sha256 }}</span>
            </div>
            <a :href="artifactContentURL(runID, artifact.name)" class="inline-flex shrink-0 items-center gap-1 rounded-md bg-white/6 px-2 py-1 text-[11px] font-semibold text-slate-300 ring-1 ring-white/8 hover:bg-white/10">
              <ArrowDownTrayIcon class="size-3.5" aria-hidden="true" /> Open
            </a>
          </div>
        </div>
        <p v-else class="py-6 text-center text-xs text-slate-600">No artifacts indexed.</p>
      </Panel>

      <Panel title="Checkpoints" :description="`${checkpoints.length} persisted checkpoint${checkpoints.length === 1 ? '' : 's'}`" compact>
        <div v-if="checkpoints.length" class="max-h-64 divide-y divide-white/8 overflow-y-auto">
          <div v-for="(checkpoint, index) in checkpoints.slice(0, 40)" :key="index" class="py-2 first:pt-0 last:pb-0">
            <strong class="block text-xs text-slate-300">{{ checkpointLabel(checkpoint, index) }}</strong>
            <p class="mt-0.5 truncate font-mono text-[10px] text-slate-600" :title="JSON.stringify(checkpoint)">{{ JSON.stringify(checkpoint) }}</p>
          </div>
        </div>
        <p v-else class="py-6 text-center text-xs text-slate-600">No persisted checkpoints exposed for this run.</p>
      </Panel>
    </div>
  </div>
</template>
