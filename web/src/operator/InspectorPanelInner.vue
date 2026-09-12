<script setup lang="ts">
import { computed, ref, watch } from 'vue'
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

const props = defineProps<{ runID: string }>()

type Row = Record<string, any>

const loading = ref(false)
const error = ref('')
const debug = ref<Row | null>(null)
const artifacts = ref<RunArtifact[]>([])
const checkpoints = ref<unknown[]>([])
const replay = ref<ReplayStatus | null>(null)
const replayError = ref('')
const rendering = ref(false)
let serial = 0

function object(value: unknown): Row {
  return value && typeof value === 'object' ? value as Row : {}
}

const timeline = computed<Row[]>(() => {
  const value = debug.value || {}
  if (Array.isArray(value.timeline)) return value.timeline.map(object)
  const run = object(value.run)
  const items: Row[] = []
  const question = value.question || run.question
  const decision = value.decision || run.decision
  const stop = value.stop_so_far || run.stop_so_far
  const detail = value.detail || run.detail
  if (question) items.push({ label: 'Planner question', value: question })
  if (decision) items.push({ label: 'Decision', value: decision })
  if (stop) items.push({ label: 'Progress', value: stop })
  if (detail) items.push({ label: 'Stop detail', value: detail })
  return items
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
  loading.value = false
}

async function requestReplay(): Promise<void> {
  if (!props.runID || rendering.value) return
  rendering.value = true
  replayError.value = ''
  try {
    replay.value = await renderReplay(props.runID)
    window.setTimeout(() => { void refreshReplay() }, 1500)
  } catch (cause) {
    replayError.value = cause instanceof Error ? cause.message : 'Replay render failed'
  } finally {
    rendering.value = false
  }
}

async function refreshReplay(): Promise<void> {
  try {
    replay.value = await getReplayStatus(props.runID)
    replayError.value = ''
  } catch (cause) {
    replayError.value = cause instanceof Error ? cause.message : 'Replay status unavailable'
  }
}

function checkpointLabel(value: unknown, index: number): string {
  const row = object(value)
  return String(row.name || row.kind || row.label || row.key || `checkpoint ${index + 1}`)
}

watch(() => props.runID, () => { void load() }, { immediate: true })
</script>

<template>
  <div class="space-y-3">
    <Panel title="Run Inspector" description="Debug bundle, timeline evidence, checkpoints, artifacts, and deterministic replay." compact>
      <template #actions>
        <button type="button" class="inline-flex items-center gap-1.5 rounded-md bg-white/8 px-2.5 py-1.5 text-xs font-semibold text-slate-200 ring-1 ring-white/10 hover:bg-white/12" :disabled="loading" @click="load">
          <ArrowPathIcon class="size-3.5" aria-hidden="true" /> {{ loading ? 'Loading…' : 'Refresh' }}
        </button>
      </template>

      <div v-if="error" class="mb-3 border-l-4 border-amber-400 bg-amber-400/10 p-3 text-sm text-amber-100">{{ error }}</div>

      <div class="grid grid-cols-1 gap-3 xl:grid-cols-2">
        <section class="rounded-md border border-white/8 bg-black/10 p-3">
          <h3 class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Timeline / evidence</h3>
          <div v-if="timeline.length" class="mt-3 divide-y divide-white/8">
            <div v-for="(item, index) in timeline" :key="index" class="py-2.5 first:pt-0 last:pb-0">
              <span class="block text-[10px] font-semibold text-cyan-300/80 uppercase">{{ item.label || item.kind || item.type || `event ${index + 1}` }}</span>
              <p class="mt-1 whitespace-pre-wrap text-xs leading-5 text-slate-300">{{ item.value || item.detail || item.text || item.summary || JSON.stringify(item) }}</p>
            </div>
          </div>
          <p v-else class="mt-3 py-6 text-center text-xs text-slate-600">No persisted timeline markers for this run.</p>
        </section>

        <section class="rounded-md border border-white/8 bg-black/10 p-3">
          <div class="flex items-center justify-between gap-3">
            <h3 class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Replay</h3>
            <StatusBadge v-if="replay" :tone="replay.state === 'ready' ? 'success' : replay.state === 'error' ? 'danger' : replay.state === 'generating' ? 'warning' : 'neutral'">{{ replay.state }}</StatusBadge>
          </div>

          <video v-if="replay?.state === 'ready'" :key="runID" class="mt-3 max-h-80 w-full rounded-md bg-black object-contain [image-rendering:pixelated]" :src="replayVideoURL(runID)" controls preload="metadata" playsinline />
          <div v-else class="mt-3 rounded-md border border-dashed border-white/10 px-4 py-7 text-center">
            <FilmIcon class="mx-auto size-6 text-slate-600" aria-hidden="true" />
            <p class="mt-2 text-xs text-slate-500">{{ replay?.state === 'generating' ? 'Replay is being rendered.' : 'Render the canonical .gbrun recording into a seekable MP4 when available.' }}</p>
            <button v-if="replay?.state !== 'disabled'" type="button" class="mt-3 rounded-md bg-cyan-500 px-2.5 py-1.5 text-xs font-semibold text-white hover:bg-cyan-400 disabled:opacity-50" :disabled="rendering || replay?.state === 'generating'" @click="requestReplay">
              {{ rendering ? 'Requesting…' : replay?.state === 'generating' ? 'Generating…' : 'Render replay' }}
            </button>
          </div>
          <p v-if="replayError" class="mt-2 text-xs text-amber-300/80">{{ replayError }}</p>
        </section>
      </div>
    </Panel>

    <div class="grid grid-cols-1 gap-3 xl:grid-cols-2">
      <Panel title="Artifacts" :description="`${artifacts.length} indexed artifact${artifacts.length === 1 ? '' : 's'}`" compact>
        <div v-if="artifacts.length" class="divide-y divide-white/8">
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
        <div v-if="checkpoints.length" class="divide-y divide-white/8">
          <div v-for="(checkpoint, index) in checkpoints.slice(0, 20)" :key="index" class="py-2 first:pt-0 last:pb-0">
            <strong class="block text-xs text-slate-300">{{ checkpointLabel(checkpoint, index) }}</strong>
            <p class="mt-0.5 truncate font-mono text-[10px] text-slate-600" :title="JSON.stringify(checkpoint)">{{ JSON.stringify(checkpoint) }}</p>
          </div>
        </div>
        <p v-else class="py-6 text-center text-xs text-slate-600">No persisted checkpoints exposed for this run.</p>
      </Panel>
    </div>

    <Panel title="Debug bundle" description="Compact structured evidence used by operator and agent tooling." compact>
      <pre class="max-h-96 overflow-auto rounded-md border border-white/8 bg-black/25 p-3 font-mono text-[10px] leading-5 text-slate-400">{{ debug ? JSON.stringify(debug, null, 2) : 'No debug bundle loaded.' }}</pre>
    </Panel>
  </div>
</template>
