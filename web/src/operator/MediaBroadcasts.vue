<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ArrowPathIcon, PlayIcon, StopIcon } from '@heroicons/vue/20/solid'
import { startBroadcast, stopBroadcast, type BroadcastDestination } from '../shared/api/broadcast'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import type { ResourceState as ResourceStateValue } from '../shared/resource'
import { presentBroadcast } from '../shared/status'
import { operatorHref } from '../shared/urls'
import { elapsedLabel, msISO, type BroadcastRow, type MediaState } from './mediaQuery'
import { gameTitle } from './operations'
import { middleTruncate } from './runs'

const props = defineProps<{
  state: MediaState
  rows: BroadcastRow[]
  rowsState: ResourceStateValue
  rowsError: string
  destinations: BroadcastDestination[]
}>()
const emit = defineEmits<{ navigate: [next: MediaState, replace?: boolean]; refresh: [] }>()

// The selected source is ?run= when that run is running, otherwise the newest running run.
const selected = computed(() => props.rows.find((row) => row.run.run_id === props.state.run) || props.rows[0] || null)
const status = computed(() => selected.value?.status || { run_id: selected.value?.run.run_id || '', state: 'stopped' })
const active = computed(() => ['starting', 'healthy', 'live', 'reconnecting', 'stopping'].includes(status.value.state))

const provider = ref('')
const width = ref(1280)
const height = ref(720)
const fps = ref(30)
const videoBitrateKbps = ref(4500)
const preset = ref('veryfast')
const busy = ref(false)
const actionError = ref('')

const destination = computed(() => props.destinations.find((item) => item.provider === provider.value))
watch(() => props.destinations, (items) => {
  if (items.some((item) => item.provider === provider.value)) return
  provider.value = items.find((item) => item.configured)?.provider || items[0]?.provider || ''
}, { immediate: true })
watch(() => selected.value?.run.run_id, () => { actionError.value = '' })

const startBlocker = computed(() => {
  if (!selected.value) return 'No running run to broadcast'
  if (active.value) return ''
  if (!destination.value) return 'No destination is configured on the replay host'
  if (!destination.value.configured) return destination.value.reason || `${destination.value.label} isn’t configured`
  return ''
})
const canStart = computed(() => Boolean(selected.value && !startBlocker.value && !active.value && !busy.value))
const canStop = computed(() => Boolean(selected.value && active.value && !busy.value))

function select(runID: string): void {
  emit('navigate', { ...props.state, run: runID }, true)
}
function liveHref(runID: string): string {
  return operatorHref('live', { run: runID, job: '' }, window.location.search)
}
function destinationLabel(value?: string): string {
  return props.destinations.find((item) => item.provider === value)?.label || value || '—'
}

async function start(): Promise<void> {
  const runID = selected.value?.run.run_id
  if (!canStart.value || !runID || !provider.value) return
  busy.value = true
  actionError.value = ''
  try {
    await startBroadcast(runID, {
      provider: provider.value,
      width: width.value,
      height: height.value,
      fps: fps.value,
      video_bitrate_kbps: videoBitrateKbps.value,
      preset: preset.value
    })
    emit('refresh')
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Broadcast start failed'
  } finally {
    busy.value = false
  }
}

async function stop(): Promise<void> {
  const runID = selected.value?.run.run_id
  if (!canStop.value || !runID) return
  busy.value = true
  actionError.value = ''
  try {
    await stopBroadcast(runID)
    emit('refresh')
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Broadcast stop failed'
  } finally {
    busy.value = false
  }
}
const fieldClass = 'block h-8 w-full rounded-[3px] border border-control bg-canvas px-2 text-fg disabled:text-fg-3 max-md:h-10'
</script>

<template>
  <div class="grid gap-3 xl:grid-cols-[minmax(0,1fr)_440px]">
    <!-- Sources: running runs and their actual outbound state -->
    <section class="ui-bay min-w-0" aria-labelledby="broadcast-sources-title">
      <div class="ui-bay-header">
        <h2 id="broadcast-sources-title">Running runs</h2>
        <button type="button" class="ui-btn ui-btn-sm ui-icon-btn" aria-label="Refresh broadcast status" title="Refresh" @click="emit('refresh')">
          <ArrowPathIcon class="size-4" aria-hidden="true" />
        </button>
      </div>
      <ResourceState v-if="rowsState === 'error'" state="error" class="m-3" title="Couldn’t load running runs" :message="`${rowsError}. Retry when the wall is reachable.`">
        <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="emit('refresh')">Retry</button></template>
      </ResourceState>
      <div v-else-if="rowsState === 'loading'" class="space-y-3 p-3" aria-busy="true">
        <div v-for="n in 3" :key="n" class="h-10 bg-raised motion-safe:animate-pulse" />
      </div>
      <div v-else-if="!rows.length" class="flex flex-col items-center gap-2 px-4 py-12 text-center">
        <span class="text-[20px] leading-5 text-fg-3" aria-hidden="true">○</span>
        <p class="text-title font-semibold">No running run to broadcast</p>
        <p class="text-fg-2">Broadcasts start from a running run. Destinations stay available in <a href="#settings/destinations" class="text-accent hover:underline">Settings › Destinations</a>.</p>
      </div>
      <ul v-else role="listbox" aria-label="Broadcast source">
        <li
          v-for="row in rows"
          :key="row.run.run_id"
          role="option"
          tabindex="0"
          :aria-selected="row.run.run_id === selected?.run.run_id"
          :class="row.run.run_id === selected?.run.run_id ? 'bg-selected shadow-[inset_2px_0_0_var(--color-accent)]' : 'hover:bg-raised'"
          class="flex min-h-14 cursor-pointer flex-wrap items-center gap-x-3 gap-y-1 border-b border-rule px-3 py-2 last:border-b-0 focus-visible:outline-offset-[-2px]"
          @click="select(row.run.run_id)"
          @keydown.enter.prevent="select(row.run.run_id)"
          @keydown.space.prevent="select(row.run.run_id)"
        >
          <div class="min-w-0 flex-1">
            <div class="truncate font-mono" :title="row.run.run_id">{{ middleTruncate(row.run.run_id) }}</div>
            <div class="truncate text-meta text-fg-2">{{ row.run.game ? gameTitle(row.run.game) : 'Game' }}<template v-if="row.run.goal"> · {{ row.run.goal }}</template></div>
          </div>
          <div class="flex items-center gap-2">
            <StatusPill v-if="row.status" :status="presentBroadcast(row.status.state)" />
            <span v-else class="ui-pill ui-tone-neutral" :title="row.error"><span aria-hidden="true">■</span> Status unavailable</span>
            <span v-if="row.status?.provider" class="text-meta text-fg-2">{{ destinationLabel(row.status.provider) }}</span>
            <span v-if="row.status?.started_at_unix_ms" class="font-mono text-meta text-fg-2" :title="msISO(row.status.started_at_unix_ms)">{{ elapsedLabel(row.status.started_at_unix_ms) }}</span>
          </div>
        </li>
      </ul>
    </section>

    <!-- Output: destination, encoding and deliberate start/stop for the selected source -->
    <section v-if="selected" class="ui-bay min-w-0" aria-labelledby="broadcast-output-title">
      <div class="ui-bay-header">
        <h2 id="broadcast-output-title" class="min-w-0 truncate">Output · <span class="font-mono font-medium" :title="selected.run.run_id">{{ middleTruncate(selected.run.run_id) }}</span></h2>
        <StatusPill :status="presentBroadcast(status.state)" />
      </div>
      <div class="space-y-3 p-3">
        <p class="text-meta text-fg-2">Credentials stay on the replay host and are never sent to this browser.</p>
        <p v-if="actionError" class="border border-danger/40 bg-danger/10 px-3 py-2 text-danger" role="alert">✕ {{ actionError }}</p>
        <p v-if="selected.error" class="border border-warn/40 bg-warn/10 px-3 py-2 text-warn" role="status">▲ Broadcast status unavailable: {{ selected.error }}</p>

        <label class="block">
          <span class="ui-label mb-1 block">Destination</span>
          <select v-model="provider" :class="fieldClass" :disabled="active || busy">
            <option v-for="item in destinations" :key="item.provider" :value="item.provider">{{ item.label }}{{ item.configured ? '' : ' · not configured' }}</option>
          </select>
        </label>
        <div v-if="destination" class="flex flex-wrap items-center gap-2 text-meta text-fg-2">
          <span :class="destination.configured ? 'ui-pill ui-tone-ok' : 'ui-pill ui-tone-warn'"><span aria-hidden="true">{{ destination.configured ? '✓' : '▲' }}</span> {{ destination.configured ? 'Configured' : 'Unavailable' }}</span>
          <span v-if="destination.host" class="font-mono">{{ destination.host }}</span>
          <a href="#settings/destinations" class="ml-auto text-accent hover:underline">Destination settings →</a>
        </div>

        <details class="border-t border-rule pt-2">
          <summary class="cursor-pointer text-meta font-medium text-accent">Encoding · {{ width }}×{{ height }} · {{ fps }} fps · {{ videoBitrateKbps }} kbps · {{ preset }}</summary>
          <div class="mt-2 grid grid-cols-2 gap-2">
            <label><span class="ui-label mb-1 block">Width</span><input v-model.number="width" type="number" min="160" max="3840" step="2" :class="fieldClass" :disabled="active || busy"></label>
            <label><span class="ui-label mb-1 block">Height</span><input v-model.number="height" type="number" min="144" max="2160" step="2" :class="fieldClass" :disabled="active || busy"></label>
            <label><span class="ui-label mb-1 block">FPS</span><input v-model.number="fps" type="number" min="1" max="60" :class="fieldClass" :disabled="active || busy"></label>
            <label><span class="ui-label mb-1 block">Bitrate kbps</span><input v-model.number="videoBitrateKbps" type="number" min="250" max="20000" step="250" :class="fieldClass" :disabled="active || busy"></label>
            <label class="col-span-2">
              <span class="ui-label mb-1 block">Encoder preset</span>
              <select v-model="preset" :class="fieldClass" :disabled="active || busy">
                <option v-for="name in ['ultrafast', 'superfast', 'veryfast', 'faster', 'fast', 'medium']" :key="name" :value="name">{{ name }}</option>
              </select>
            </label>
          </div>
        </details>

        <div class="flex flex-wrap items-center gap-2 border-t border-rule pt-3">
          <button type="button" class="ui-btn ui-btn-primary max-md:h-10" :disabled="!canStart" :aria-describedby="startBlocker ? 'broadcast-start-reason' : undefined" @click="start">
            <PlayIcon class="size-4" aria-hidden="true" /> {{ busy && !active ? 'Starting…' : 'Start broadcast' }}
          </button>
          <button type="button" class="ui-btn ui-btn-danger max-md:h-10" :disabled="!canStop" @click="stop">
            <StopIcon class="size-4" aria-hidden="true" /> {{ busy && active ? 'Stopping…' : 'Stop broadcast' }}
          </button>
          <span v-if="startBlocker && !active" id="broadcast-start-reason" class="text-meta text-fg-2">{{ startBlocker }}</span>
        </div>
        <p class="text-meta text-fg-2">Replay-sidecar restart stops active broadcasts; configured destinations remain available and must be started again.</p>

        <dl class="ui-kv border-t border-rule">
          <dt>Destination</dt><dd>{{ status.provider ? destinationLabel(status.provider) : 'Off' }}<span v-if="status.host" class="block font-mono text-meta text-fg-2">{{ status.host }}</span></dd>
          <dt>Output</dt><dd class="font-mono">{{ status.width && status.height ? `${status.width}×${status.height}` : '—' }}<template v-if="status.fps"> · {{ status.fps }} fps</template></dd>
          <dt>Elapsed</dt><dd class="font-mono" :title="msISO(status.started_at_unix_ms)">{{ active ? elapsedLabel(status.started_at_unix_ms) : '—' }}</dd>
          <dt>Reconnects</dt><dd class="font-mono">{{ status.reconnects || 0 }}</dd>
          <dt>Source run</dt><dd><a :href="liveHref(selected.run.run_id)" class="font-mono break-all text-accent hover:underline">{{ selected.run.run_id }}</a></dd>
        </dl>
        <p v-if="status.last_error" class="border border-warn/40 bg-warn/10 px-3 py-2 text-warn" role="status">▲ {{ status.last_error }}</p>
      </div>
    </section>
  </div>
</template>
