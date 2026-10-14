<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowPathIcon, ArrowTopRightOnSquareIcon } from '@heroicons/vue/20/solid'
import { forceEndWorker, getDashboard } from '../shared/api/client'
import type { DashboardWorker } from '../shared/api/types'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { operatorHref } from '../shared/urls'
import {
  ageLabel,
  gameTitle,
  partitionOperationRuns,
  shortID,
  shortRevision,
  statusTone
} from './operations'

const {
  data: activeData,
  error: activeError,
  lastUpdatedAt: activeUpdatedAt,
  state: activeState,
  retry: retryActive
} = usePollingResource(
  (signal) => getDashboard({ active: true }, signal),
  { intervalMs: 2000 }
)

const workers = computed(() => [...(activeData.value?.workers ?? [])]
  .sort((a, b) => a.addr.localeCompare(b.addr)))
const runs = computed(() => partitionOperationRuns(activeData.value?.runs ?? []))
const idleWorkers = computed(() => workers.value.filter((worker) => !worker.run_id).length)
const connectionLabel = computed(() => activeState.value === 'stale' ? 'Stale' : activeState.value === 'error' ? 'Offline' : activeState.value === 'loading' ? 'Connecting' : 'Connected')
const activeUpdatedLabel = computed(() => activeUpdatedAt.value ? new Date(activeUpdatedAt.value).toLocaleTimeString() : '—')
const nowSeconds = computed(() => activeData.value?.now || Date.now() / 1000)

const forceEndTarget = ref<DashboardWorker | null>(null)
const forceEndBusy = ref(false)
const workerActionError = ref('')
const forceEndTitle = computed(() => forceEndTarget.value ? `Force end ${forceEndTarget.value.addr}?` : 'Force end worker?')
const forceEndMessage = computed(() => {
  const worker = forceEndTarget.value
  if (!worker) return ''
  if (worker.run_id) {
    return `This immediately terminates the worker and cancels active run ${shortID(worker.run_id)}. The run becomes terminal: it will not be retried and an endless successor will not be created.\n\nAny in-process work on this worker is abandoned. Runs still waiting in the shared farm queue stay queued for healthy workers.`
  }
  return 'This immediately terminates the idle worker. Any in-process background work on that worker is abandoned. Runs waiting in the shared farm queue stay queued for healthy workers.'
})

const metrics = computed(() => [
  { label: 'Wall', value: connectionLabel.value, note: '2s refresh', mono: false },
  { label: 'Workers', value: workers.value.length, note: `${idleWorkers.value} available`, mono: true },
  { label: 'Running', value: runs.value.active.length, note: 'active attempts', mono: true },
  { label: 'Waiting', value: runs.value.waiting.length, note: `${runs.value.waiting.filter((run) => run.status === 'leased').length} leased · ${runs.value.waiting.filter((run) => run.status === 'queued').length} queued`, mono: true }
])
const signals = computed(() => {
  const items: { title: string; detail: string; tone: 'warning' | 'info' | 'neutral' }[] = []
  if (activeState.value === 'stale') items.push({ title: 'Snapshot stale', detail: 'Worker and lease information reflects the last successful refresh.', tone: 'warning' })
  const waiting = runs.value.waiting.filter((run) => run.status === 'queued').length
  const leased = runs.value.waiting.filter((run) => run.status === 'leased').length
  if (waiting) items.push({ title: 'Queue backlog', detail: `${waiting} run(s) awaiting a worker.`, tone: 'warning' })
  if (leased) items.push({ title: 'Leases starting', detail: `${leased} run(s) leased but not yet running.`, tone: 'info' })
  const busy = workers.value.filter((worker) => worker.run_id).length
  if (busy) items.push({ title: 'Workers occupied', detail: `${busy} of ${workers.value.length} workers have a run assigned.`, tone: 'neutral' })
  return items
})
function operatorSearch(): string { return window.location.search }
function refreshActive(): void { void retryActive() }

function requestForceEnd(worker: DashboardWorker): void {
  workerActionError.value = ''
  forceEndTarget.value = worker
}

function closeForceEnd(): void {
  if (!forceEndBusy.value) forceEndTarget.value = null
}

async function confirmForceEnd(): Promise<void> {
  const worker = forceEndTarget.value
  if (!worker || forceEndBusy.value) return
  forceEndBusy.value = true
  workerActionError.value = ''
  try {
    await forceEndWorker(worker.addr)
    forceEndTarget.value = null
    void retryActive()
  } catch (error) {
    workerActionError.value = error instanceof Error ? error.message : String(error)
  } finally {
    forceEndBusy.value = false
  }
}
</script>

<template>
  <div class="space-y-3">
    <div class="flex flex-wrap justify-end gap-2">
      <span class="self-center font-mono text-[11px] text-slate-500" :title="activeData?.wall_version || ''">Snapshot {{ activeUpdatedLabel }} · wall {{ shortRevision(activeData?.wall_version) }}</span>
      <a href="#settings/models" class="rounded-sm px-2.5 py-1.5 text-xs font-semibold text-cyan-200 ring-1 ring-cyan-300/20 hover:bg-cyan-300/10 focus-visible:outline-2 focus-visible:outline-cyan-300">Configure models</a>
      <button type="button" class="inline-flex items-center gap-1.5 rounded-sm px-2.5 py-1.5 text-xs font-semibold text-slate-200 ring-1 ring-white/10 hover:bg-white/10 focus-visible:outline-2 focus-visible:outline-cyan-300" @click="refreshActive"><ArrowPathIcon class="size-3.5" aria-hidden="true" />Refresh</button>
    </div>
    <ResourceState
      :state="activeState"
      title="Operator data is unavailable"
      :message="activeError || 'Workers and active runs will recover automatically when the wall is reachable again.'"
      :rows="5"
    >
      <template #actions>
        <button type="button" class="inline-flex items-center gap-1.5 rounded-md bg-white/10 px-2.5 py-1.5 text-xs font-semibold text-white ring-1 ring-white/10 hover:bg-white/15" @click="refreshActive">
          <ArrowPathIcon class="size-3.5" aria-hidden="true" />
          Retry now
        </button>
      </template>

      <div class="grid grid-cols-2 gap-px overflow-hidden rounded-lg bg-white/10 ring-1 ring-white/10 lg:grid-cols-4">
        <div v-for="metric in metrics" :key="metric.label" class="bg-[#0b111a] px-4 py-4 sm:px-5">
          <dt class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">{{ metric.label }}</dt>
          <dd :class="['mt-1 text-xl font-semibold text-white', metric.mono ? 'font-mono' : 'font-mono tabular-nums']">{{ metric.value }}</dd>
          <p class="mt-0.5 text-xs text-slate-500">{{ metric.note }}</p>
        </div>
      </div>

      <div class="mt-3 grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,2fr)_minmax(18rem,1fr)]">
        <Panel title="Workers" description="Capacity, heartbeat and worker leases. Select a run for Live inspection." compact>
          <div v-if="workers.length" class="-mx-3 overflow-x-auto sm:-mx-4">
            <table class="min-w-full divide-y divide-white/10 text-left">
              <thead>
                <tr>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:px-4">Worker</th>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">State</th>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Run</th>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Seen</th>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Build</th>
                  <th class="px-3 py-2 text-right text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:pr-4">Action</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-white/8">
                <tr v-for="worker in workers" :key="worker.addr">
                  <td class="px-3 py-2.5 font-mono text-xs whitespace-nowrap text-slate-300 sm:px-4">{{ worker.addr }}</td>
                  <td class="px-3 py-2.5"><StatusBadge :tone="worker.run_id ? 'info' : 'success'">{{ worker.run_id ? 'Busy' : 'Available' }}</StatusBadge></td>
                  <td class="px-3 py-2.5 font-mono text-xs whitespace-nowrap text-slate-400" :title="worker.run_id || ''"><a v-if="worker.run_id" :href="operatorHref('live', { run: worker.run_id }, operatorSearch())" class="inline-flex items-center gap-1 text-cyan-200 hover:underline focus-visible:outline-2 focus-visible:outline-cyan-300"><span>{{ shortID(worker.run_id) }}</span><ArrowTopRightOnSquareIcon class="size-3" aria-hidden="true" /></a><span v-else>—</span></td>
                  <td class="px-3 py-2.5 text-xs whitespace-nowrap text-slate-500">{{ worker.seen_ago || 'Not reported' }}</td>
                  <td class="px-3 py-2.5 font-mono text-xs whitespace-nowrap text-slate-500">{{ shortRevision(worker.version) }}</td>
                  <td class="px-3 py-2.5 text-right sm:pr-4">
                    <button
                      type="button"
                      class="rounded-md bg-rose-400/8 px-2.5 py-1.5 text-[11px] font-semibold whitespace-nowrap text-rose-300 ring-1 ring-rose-400/20 hover:bg-rose-400/15 focus-visible:outline-2 focus-visible:outline-rose-300"
                      @click="requestForceEnd(worker)"
                    >
                      Force end
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="py-6 text-center text-sm text-slate-500">No workers are reporting.</p>
          <p v-if="workerActionError" class="mt-3 text-xs text-rose-300">Force end failed: {{ workerActionError }}</p>
        </Panel>

        <Panel title="Queue and leases" description="Waiting work in oldest-first order; select a run to inspect." compact>
          <ul v-if="runs.waiting.length" class="divide-y divide-white/8">
            <li v-for="run in runs.waiting" :key="run.run_id" class="py-3 first:pt-0 last:pb-0">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <a :href="operatorHref('live', { run: run.run_id }, operatorSearch())" class="font-mono text-xs text-cyan-200 hover:underline focus-visible:outline-2 focus-visible:outline-cyan-300" :title="run.run_id">{{ shortID(run.run_id) }}</a>
                    <StatusBadge :tone="statusTone(run.status)">{{ run.status }}</StatusBadge>
                    <StatusBadge tone="neutral">{{ gameTitle(run.game) }}</StatusBadge>
                  </div>
                  <p class="mt-1 text-xs text-slate-500">{{ run.status === 'leased' ? 'Lease assigned' : 'Awaiting capacity' }}</p>
                </div>
                <span class="shrink-0 font-mono text-[11px] text-slate-600">{{ ageLabel(run.queued_at, nowSeconds) }}</span>
              </div>
            </li>
          </ul>
          <p v-else class="py-6 text-center text-sm text-slate-500">Queue is empty.</p>
        </Panel>
      </div>

      <Panel title="Current operational signals" description="Derived from the live wall snapshot; completed outcomes belong to Runs." compact>
      <ul v-if="signals.length" class="grid gap-2 lg:grid-cols-2">
        <li v-for="signal in signals" :key="signal.title" class="rounded-sm border border-white/8 bg-black/10 px-3 py-2">
          <StatusBadge :tone="signal.tone">{{ signal.title }}</StatusBadge>
          <p class="mt-1 text-xs text-slate-400">{{ signal.detail }}</p>
        </li>
      </ul>
      <p v-else class="py-2 text-xs text-slate-500">No queue or lease signals. See <a href="#runs" class="text-cyan-200 underline">Runs</a> for finished outcomes.</p>
      </Panel>
    </ResourceState>

    <ConfirmDialog
      :open="forceEndTarget !== null"
      :title="forceEndTitle"
      :message="forceEndMessage"
      confirm-label="Force end worker"
      :busy="forceEndBusy"
      danger
      @close="closeForceEnd"
      @confirm="confirmForceEnd"
    />
  </div>
</template>