<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ArrowPathIcon, PlayIcon, StopIcon, SignalIcon } from '@heroicons/vue/20/solid'
import { getDashboard } from '../shared/api/client'
import {
  getBroadcastControl,
  getBroadcastStatus,
  startBroadcast,
  stopBroadcast,
  type BroadcastDestination,
  type BroadcastStatus
} from '../shared/api/broadcast'
import Panel from '../shared/components/Panel.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import type { DashboardRun } from '../shared/api/types'

const activeResource = usePollingResource(
  (signal) => getDashboard({ active: true }, signal),
  { intervalMs: 2000 }
)
const configResource = usePollingResource(
  (signal) => getBroadcastControl(signal),
  { intervalMs: 30000 }
)

const selectedRunID = ref('')
const selectedProvider = ref('')
const width = ref(1280)
const height = ref(720)
const fps = ref(30)
const videoBitrateKbps = ref(4500)
const preset = ref('veryfast')
const actionError = ref('')
const busy = ref(false)

const activeRuns = computed<DashboardRun[]>(() =>
  [...(activeResource.data.value?.runs || [])]
    .filter((run) => run.status === 'running')
    .sort((a, b) => Number(b.queued_at || 0) - Number(a.queued_at || 0))
)

const destinations = computed<BroadcastDestination[]>(() => configResource.data.value?.destinations || [])
const selectedDestination = computed(() =>
  destinations.value.find((destination) => destination.provider === selectedProvider.value)
)

watch(activeRuns, (runs) => {
  if (!runs.length) {
    selectedRunID.value = ''
    return
  }
  if (!runs.some((run) => run.run_id === selectedRunID.value)) {
    selectedRunID.value = runs[0].run_id
  }
}, { immediate: true })

watch(destinations, (items) => {
  if (items.some((item) => item.provider === selectedProvider.value)) return
  selectedProvider.value = items.find((item) => item.configured)?.provider || items[0]?.provider || ''
}, { immediate: true })

const statusResource = usePollingResource<BroadcastStatus>(
  (signal) => selectedRunID.value
    ? getBroadcastStatus(selectedRunID.value, signal)
    : Promise.resolve({ run_id: '', state: 'stopped' }),
  { intervalMs: 2000 }
)

watch(selectedRunID, () => {
  actionError.value = ''
  void statusResource.retry()
})

const status = computed<BroadcastStatus>(() =>
  statusResource.data.value || { run_id: selectedRunID.value, state: 'stopped' }
)
const isBroadcasting = computed(() =>
  ['starting', 'healthy', 'live', 'reconnecting'].includes(status.value.state)
)
const canStart = computed(() =>
  Boolean(selectedRunID.value && selectedDestination.value?.configured && !isBroadcasting.value && !busy.value)
)
const canStop = computed(() => Boolean(selectedRunID.value && isBroadcasting.value && !busy.value))

function stateLabel(value: string): string {
  if (value === 'healthy' || value === 'live') return 'Live'
  if (value === 'reconnecting') return 'Reconnecting'
  if (value === 'starting') return 'Starting'
  if (value === 'failed') return 'Failed'
  return 'Stopped'
}

function stateTone(value: string): 'neutral' | 'info' | 'success' | 'warning' | 'danger' {
  if (value === 'healthy' || value === 'live') return 'success'
  if (value === 'starting') return 'info'
  if (value === 'reconnecting') return 'warning'
  if (value === 'failed') return 'danger'
  return 'neutral'
}

async function refresh(): Promise<void> {
  actionError.value = ''
  await Promise.allSettled([
    activeResource.retry(),
    configResource.retry(),
    statusResource.retry()
  ])
}

async function start(): Promise<void> {
  if (!canStart.value || !selectedProvider.value || !selectedRunID.value) return
  busy.value = true
  actionError.value = ''
  try {
    await startBroadcast(selectedRunID.value, {
      provider: selectedProvider.value,
      width: width.value,
      height: height.value,
      fps: fps.value,
      video_bitrate_kbps: videoBitrateKbps.value,
      preset: preset.value
    })
    await statusResource.retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Broadcast start failed'
  } finally {
    busy.value = false
  }
}

async function stop(): Promise<void> {
  if (!canStop.value || !selectedRunID.value) return
  busy.value = true
  actionError.value = ''
  try {
    await stopBroadcast(selectedRunID.value)
    await statusResource.retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Broadcast stop failed'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="space-y-3">
    <Panel
      title="Live broadcast"
      description="Start or stop RTMP(S) publishing for a running game. Credentials stay on the replay host and are never sent to this browser."
      compact
    >
      <div class="flex flex-col gap-3">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="flex items-center gap-2">
            <SignalIcon class="size-4 text-fuchsia-300" aria-hidden="true" />
            <span class="text-xs font-bold text-white">Broadcast control</span>
            <StatusBadge :tone="stateTone(status.state)">{{ stateLabel(status.state) }}</StatusBadge>
          </div>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md border border-white/10 px-2.5 py-1.5 text-xs text-slate-300 hover:bg-white/5 disabled:opacity-50"
            :disabled="busy"
            @click="refresh"
          >
            <ArrowPathIcon class="size-3.5" aria-hidden="true" />
            Refresh
          </button>
        </div>

        <p v-if="actionError" class="rounded-md border border-rose-300/20 bg-rose-300/10 px-3 py-2 text-xs text-rose-200">
          {{ actionError }}
        </p>

        <div v-if="!activeRuns.length" class="rounded-md border border-white/8 bg-black/15 px-3 py-4 text-xs text-slate-400">
          No running run is available to broadcast.
        </div>

        <div v-else class="grid gap-3 lg:grid-cols-2">
          <section class="space-y-3 rounded-lg border border-white/8 bg-black/15 p-3">
            <label class="block">
              <span class="mb-1 block text-[10px] font-bold uppercase tracking-wide text-slate-500">Running run</span>
              <select
                v-model="selectedRunID"
                class="w-full rounded-md border border-white/10 bg-slate-950 px-2.5 py-2 text-xs text-slate-200"
                :disabled="isBroadcasting || busy"
              >
                <option v-for="run in activeRuns" :key="run.run_id" :value="run.run_id">
                  {{ run.run_id }} · {{ run.game || 'game' }}
                </option>
              </select>
            </label>

            <label class="block">
              <span class="mb-1 block text-[10px] font-bold uppercase tracking-wide text-slate-500">Destination</span>
              <select
                v-model="selectedProvider"
                class="w-full rounded-md border border-white/10 bg-slate-950 px-2.5 py-2 text-xs text-slate-200"
                :disabled="isBroadcasting || busy"
              >
                <option v-for="destination in destinations" :key="destination.provider" :value="destination.provider">
                  {{ destination.label }}{{ destination.configured ? '' : ' · not configured' }}
                </option>
              </select>
            </label>

            <div v-if="selectedDestination" class="rounded-md border border-white/8 bg-white/[0.03] p-2.5 text-[11px] leading-5 text-slate-400">
              <div class="flex items-center justify-between gap-3">
                <span>{{ selectedDestination.label }}</span>
                <StatusBadge :tone="selectedDestination.configured ? 'success' : 'warning'">
                  {{ selectedDestination.configured ? 'Configured' : 'Unavailable' }}
                </StatusBadge>
              </div>
              <p v-if="selectedDestination.host" class="mt-1 font-mono text-[10px] text-slate-500">
                {{ selectedDestination.host }}
              </p>
              <p v-if="selectedDestination.reason" class="mt-1 text-amber-200">
                {{ selectedDestination.reason }}
              </p>
            </div>
          </section>

          <section class="space-y-3 rounded-lg border border-white/8 bg-black/15 p-3">
            <div class="grid grid-cols-2 gap-2">
              <label class="block">
                <span class="mb-1 block text-[10px] font-bold uppercase tracking-wide text-slate-500">Width</span>
                <input v-model.number="width" type="number" min="160" max="3840" step="2" class="w-full rounded-md border border-white/10 bg-slate-950 px-2 py-1.5 text-xs text-slate-200" :disabled="isBroadcasting || busy" />
              </label>
              <label class="block">
                <span class="mb-1 block text-[10px] font-bold uppercase tracking-wide text-slate-500">Height</span>
                <input v-model.number="height" type="number" min="144" max="2160" step="2" class="w-full rounded-md border border-white/10 bg-slate-950 px-2 py-1.5 text-xs text-slate-200" :disabled="isBroadcasting || busy" />
              </label>
              <label class="block">
                <span class="mb-1 block text-[10px] font-bold uppercase tracking-wide text-slate-500">FPS</span>
                <input v-model.number="fps" type="number" min="1" max="60" class="w-full rounded-md border border-white/10 bg-slate-950 px-2 py-1.5 text-xs text-slate-200" :disabled="isBroadcasting || busy" />
              </label>
              <label class="block">
                <span class="mb-1 block text-[10px] font-bold uppercase tracking-wide text-slate-500">Bitrate kbps</span>
                <input v-model.number="videoBitrateKbps" type="number" min="250" max="20000" step="250" class="w-full rounded-md border border-white/10 bg-slate-950 px-2 py-1.5 text-xs text-slate-200" :disabled="isBroadcasting || busy" />
              </label>
            </div>
            <label class="block">
              <span class="mb-1 block text-[10px] font-bold uppercase tracking-wide text-slate-500">Encoder preset</span>
              <select v-model="preset" class="w-full rounded-md border border-white/10 bg-slate-950 px-2.5 py-2 text-xs text-slate-200" :disabled="isBroadcasting || busy">
                <option value="ultrafast">ultrafast</option>
                <option value="superfast">superfast</option>
                <option value="veryfast">veryfast</option>
                <option value="faster">faster</option>
                <option value="fast">fast</option>
                <option value="medium">medium</option>
              </select>
            </label>
          </section>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md bg-emerald-500/15 px-3 py-2 text-xs font-bold text-emerald-200 ring-1 ring-emerald-300/20 hover:bg-emerald-500/20 disabled:cursor-not-allowed disabled:opacity-40"
            :disabled="!canStart"
            @click="start"
          >
            <PlayIcon class="size-3.5" aria-hidden="true" />
            Start broadcast
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md bg-rose-500/15 px-3 py-2 text-xs font-bold text-rose-200 ring-1 ring-rose-300/20 hover:bg-rose-500/20 disabled:cursor-not-allowed disabled:opacity-40"
            :disabled="!canStop"
            @click="stop"
          >
            <StopIcon class="size-3.5" aria-hidden="true" />
            Stop broadcast
          </button>
          <span class="text-[10px] text-slate-500">
            Replay-sidecar restart stops active broadcasts; configured destinations remain available and must be started again.
          </span>
        </div>
      </div>
    </Panel>

    <Panel v-if="selectedRunID" title="Broadcast status" compact>
      <dl class="grid gap-2 text-xs sm:grid-cols-2 lg:grid-cols-4">
        <div>
          <dt class="text-[10px] uppercase tracking-wide text-slate-500">Provider</dt>
          <dd class="mt-0.5 text-slate-200">{{ status.provider || 'Off' }}</dd>
        </div>
        <div>
          <dt class="text-[10px] uppercase tracking-wide text-slate-500">Destination</dt>
          <dd class="mt-0.5 font-mono text-[11px] text-slate-300">{{ status.host || '—' }}</dd>
        </div>
        <div>
          <dt class="text-[10px] uppercase tracking-wide text-slate-500">Output</dt>
          <dd class="mt-0.5 text-slate-200">
            {{ status.width && status.height ? `${status.width}×${status.height}` : '—' }}
            <span v-if="status.fps"> · {{ status.fps }} fps</span>
          </dd>
        </div>
        <div>
          <dt class="text-[10px] uppercase tracking-wide text-slate-500">Reconnects</dt>
          <dd class="mt-0.5 text-slate-200">{{ status.reconnects || 0 }}</dd>
        </div>
      </dl>
      <p v-if="status.last_error" class="mt-3 rounded-md border border-amber-300/20 bg-amber-300/10 px-3 py-2 text-xs text-amber-100">
        {{ status.last_error }}
      </p>
    </Panel>
  </div>
</template>
