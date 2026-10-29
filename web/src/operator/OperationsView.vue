<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowPathIcon } from '@heroicons/vue/20/solid'
import { forceEndWorker } from '../shared/api/client'
import type { DashboardSnapshot, DashboardWorker } from '../shared/api/types'
import ActionMenu, { type ActionMenuItem } from '../shared/components/ActionMenu.vue'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import type { PollingResource } from '../shared/composables/usePollingResource'
import { presentRunStatus, presentWorker } from '../shared/status'
import { operatorHref } from '../shared/urls'
import { ageLabel, gameTitle, partitionOperationRuns, shortRevision } from './operations'
import { middleTruncate } from './runs'

// The shell's active dashboard supplies workers and runs at the same cadence
// as this page, avoiding a duplicate request.
const props = defineProps<{ activeResource: PollingResource<DashboardSnapshot> }>()
const {
  data: activeData,
  error: activeError,
  state: activeState,
  retry: retryActive
} = props.activeResource

const loaded = computed(() => activeState.value !== 'loading' && activeState.value !== 'error')
const workers = computed(() => [...(activeData.value?.workers ?? [])]
  .sort((a, b) => a.addr.localeCompare(b.addr)))
const runs = computed(() => partitionOperationRuns(activeData.value?.runs ?? []))
const busyWorkers = computed(() => workers.value.filter((worker) => worker.run_id).length)
const idleWorkers = computed(() => workers.value.length - busyWorkers.value)
const queued = computed(() => runs.value.waiting.filter((run) => run.status === 'queued'))
const leased = computed(() => runs.value.waiting.filter((run) => run.status === 'leased').length)
const nowSeconds = computed(() => activeData.value?.now || Date.now() / 1000)
// Queued work with no idle worker is the one state here that needs the operator.
const backlog = computed(() => queued.value.length > 0 && idleWorkers.value === 0)

const health = computed(() => [
  { label: 'Workers', value: workers.value.length, title: 'Workers reporting to the wall' },
  { label: 'Busy', value: busyWorkers.value, title: 'Workers holding a run' },
  { label: 'Idle', value: idleWorkers.value, title: 'Workers free for the next queued run' },
  { label: 'Running', value: runs.value.active.length, title: 'Attempts currently playing' },
  { label: 'Starting', value: leased.value, title: 'Runs leased to a worker but not yet running' },
  { label: 'Queued', value: queued.value.length, title: 'Runs waiting for a free worker' },
  { label: 'Oldest wait', value: queued.value.length ? ageLabel(queued.value[0].queued_at, nowSeconds.value) : '—', title: 'How long the oldest queued run has waited' }
])

function operatorSearch(): string { return window.location.search }
function liveHref(runID: string): string { return operatorHref('live', { run: runID }, operatorSearch()) }
function refresh(): void { void retryActive() }

// ---- Receipts (one polite live region, mirrored as a toast) ----------------------
const toast = ref('')
let toastTimer = 0
function announce(text: string): void {
  toast.value = text
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => { toast.value = '' }, 4000)
}
async function copy(value: string, noun: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(value)
    announce(`${noun} copied`)
  } catch {
    announce(`Couldn’t copy the ${noun.toLowerCase()}`)
  }
}

function workerMenu(worker: DashboardWorker): ActionMenuItem[] {
  const items: ActionMenuItem[] = [{ label: 'Copy worker address', select: () => void copy(worker.addr, 'Worker address') }]
  if (worker.run_id) {
    const runID = worker.run_id
    items.unshift({ label: 'Open run in Live', href: liveHref(runID) })
    items.push({ label: 'Copy run ID', select: () => void copy(runID, 'Run ID') })
  }
  items.push({ label: 'Force end worker…', danger: true, separator: true, select: () => requestForceEnd(worker) })
  return items
}

// ---- Force end ------------------------------------------------------------------
const forceEndTarget = ref<DashboardWorker | null>(null)
const forceEndBusy = ref(false)
const workerActionError = ref('')
const forceEndTitle = computed(() => forceEndTarget.value ? `Force end ${forceEndTarget.value.addr}?` : 'Force end worker?')
const forceEndMessage = computed(() => {
  const worker = forceEndTarget.value
  if (!worker) return ''
  if (worker.run_id) {
    return `This immediately terminates the worker and cancels active run ${worker.run_id}. The run becomes terminal: it will not be retried and an endless successor will not be created.\n\nAny in-process work on this worker is abandoned. Runs still waiting in the shared farm queue stay queued for healthy workers.`
  }
  return 'This immediately terminates the idle worker. Any in-process background work on that worker is abandoned. Runs waiting in the shared farm queue stay queued for healthy workers.'
})

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
    announce(`Force end sent to ${worker.addr}`)
    forceEndTarget.value = null
    void retryActive()
  } catch (error) {
    workerActionError.value = `Force end ${worker.addr} failed: ${error instanceof Error ? error.message : String(error)}. Other workers are unaffected.`
  } finally {
    forceEndBusy.value = false
  }
}
</script>

<template>
  <div class="space-y-3">
    <header class="flex min-h-12 flex-wrap items-center gap-x-3 gap-y-2">
      <h1 class="text-page font-semibold">Operations</h1>
      <span v-if="loaded" class="text-fg-2">{{ workers.length }} workers · {{ runs.waiting.length }} waiting</span>
      <div class="ml-auto flex items-center gap-2">
        <button type="button" class="ui-btn ui-icon-btn max-md:size-10" aria-label="Refresh operations" title="Refresh" @click="refresh">
          <ArrowPathIcon class="size-4" aria-hidden="true" />
        </button>
      </div>
    </header>

    <ResourceState v-if="activeState === 'stale'" state="stale" title="Showing the last loaded worker snapshot" :message="`Refresh failed: ${activeError}`">
      <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="refresh">Retry</button></template>
    </ResourceState>
    <ResourceState v-if="!loaded" :state="activeState" title="Couldn’t load workers and queue" :message="`${activeError || 'The wall is unreachable'}. Running workers keep playing; this page retries automatically.`" :rows="6">
      <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="refresh">Retry</button></template>
    </ResourceState>

    <template v-else>
      <!-- Health strip -->
      <dl class="flex flex-wrap gap-x-6 gap-y-2 border border-rule-strong bg-surface px-3 py-2" aria-label="Farm capacity">
        <div v-for="item in health" :key="item.label" class="flex items-baseline gap-2" :title="item.title">
          <dt class="ui-label">{{ item.label }}</dt>
          <dd class="m-0 font-mono font-medium">{{ item.value }}</dd>
        </div>
      </dl>

      <p v-if="workerActionError" class="border border-danger/40 bg-danger/10 px-3 py-2 text-danger" role="alert">✕ {{ workerActionError }}</p>

      <div class="grid grid-cols-1 items-start gap-3 xl:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
        <section class="ui-bay" aria-labelledby="workers-title">
          <div class="ui-bay-header">
            <h2 id="workers-title">Workers</h2>
            <span class="text-meta font-normal text-fg-2">{{ busyWorkers }} busy · {{ idleWorkers }} idle</span>
          </div>
          <div v-if="workers.length" class="overflow-x-auto">
            <table class="w-full border-collapse text-left max-md:block">
              <thead class="max-md:hidden">
                <tr class="h-9 border-b border-rule-strong bg-raised text-meta text-fg-2">
                  <th class="px-3 font-medium">Worker</th>
                  <th class="w-24 px-3 font-medium">State</th>
                  <th class="px-3 font-medium">Run</th>
                  <th class="w-24 px-3 text-right font-medium">Seen</th>
                  <th class="w-20 px-3 font-medium">Build</th>
                  <th class="w-12 px-3"><span class="sr-only">Actions</span></th>
                </tr>
              </thead>
              <tbody class="max-md:block">
                <tr v-for="worker in workers" :key="worker.addr" class="h-10 border-b border-rule last:border-b-0 hover:bg-raised max-md:flex max-md:h-auto max-md:flex-wrap max-md:items-center max-md:gap-x-3 max-md:gap-y-1 max-md:px-3 max-md:py-2">
                  <td class="px-3 font-mono whitespace-nowrap max-md:order-1 max-md:px-0">{{ worker.addr }}</td>
                  <td class="px-3 max-md:order-2 max-md:px-0"><StatusPill :status="presentWorker(worker)" /></td>
                  <td :class="worker.run_id ? '' : 'max-md:hidden'" class="px-3 font-mono whitespace-nowrap max-md:order-4 max-md:basis-full max-md:px-0">
                    <a v-if="worker.run_id" :href="liveHref(worker.run_id)" class="text-accent hover:underline" :title="`Open ${worker.run_id} in Live`">{{ middleTruncate(worker.run_id) }}</a>
                    <span v-else class="text-fg-3">—</span>
                  </td>
                  <td class="px-3 text-right whitespace-nowrap text-fg-2 max-md:order-5 max-md:px-0 max-md:text-meta">{{ worker.seen_ago || 'Not reported' }}</td>
                  <td class="px-3 font-mono text-fg-2 max-md:order-6 max-md:px-0 max-md:text-meta" :title="worker.version || ''">{{ shortRevision(worker.version) }}</td>
                  <td class="px-3 text-right max-md:order-3 max-md:ml-auto max-md:px-0">
                    <ActionMenu :label="`Actions for worker ${worker.addr}`" :items="workerMenu(worker)" />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="px-3 py-8 text-center text-fg-2">No workers are reporting. Queued runs wait until one connects.</p>
        </section>

        <section class="ui-bay" aria-labelledby="queue-title">
          <div class="ui-bay-header">
            <h2 id="queue-title">Queue and leases</h2>
            <span v-if="backlog" class="ui-pill ui-tone-warn font-normal"><span aria-hidden="true">▲</span> No idle worker</span>
            <span v-else class="text-meta font-normal text-fg-2">Oldest first</span>
          </div>
          <table v-if="runs.waiting.length" class="w-full border-collapse text-left">
            <thead>
              <tr class="h-9 border-b border-rule-strong bg-raised text-meta text-fg-2">
                <th class="px-3 font-medium">Run</th>
                <th class="w-28 px-3 font-medium">State</th>
                <th class="w-20 px-3 text-right font-medium">Waiting</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="run in runs.waiting" :key="run.run_id" class="h-14 border-b border-rule last:border-b-0 hover:bg-raised">
                <td class="max-w-0 px-3">
                  <a :href="liveHref(run.run_id)" class="block truncate font-mono text-accent hover:underline" :title="`Open ${run.run_id} in Live`">{{ middleTruncate(run.run_id) }}</a>
                  <div class="truncate text-meta text-fg-2">{{ gameTitle(run.game) }}</div>
                </td>
                <td class="px-3"><StatusPill :status="presentRunStatus(run)" /></td>
                <td class="px-3 text-right font-mono text-fg-2" :title="run.queued_at ? new Date(run.queued_at * 1000).toISOString() : ''">{{ ageLabel(run.queued_at, nowSeconds) }}</td>
              </tr>
            </tbody>
          </table>
          <p v-else class="px-3 py-8 text-center text-fg-2">Queue is empty. Finished outcomes are in <a href="#runs" class="text-accent underline">Runs</a>.</p>
        </section>
      </div>
    </template>

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

    <div class="pointer-events-none fixed right-4 bottom-4 z-[80]" aria-live="polite" role="status">
      <div v-if="toast" class="pointer-events-auto rounded-md border border-rule-strong bg-raised px-4 py-2.5 shadow-[0_8px_24px_rgb(0_0_0/45%)]">{{ toast }}</div>
    </div>
  </div>
</template>
