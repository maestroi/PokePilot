<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowPathIcon } from '@heroicons/vue/20/solid'
import { getMediaRenderJobs, retryMediaRenderJob } from '../shared/api/client'
import type { MediaRenderJob } from '../shared/api/types'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { shortID } from './operations'

const ACTIVE_STATES = new Set(['preparing', 'rendering', 'assembling', 'uploading'])

const {
  data,
  error,
  lastUpdatedAt,
  state,
  retry: refreshJobs
} = usePollingResource(
  (signal) => getMediaRenderJobs(50, signal),
  {
    intervalMs: 2000,
    isEmpty: (snapshot) => snapshot.jobs.length === 0
  }
)

const jobs = computed(() => data.value?.jobs ?? [])
const stateCounts = computed(() => data.value?.states ?? {})
const activeCount = computed(() => [...ACTIVE_STATES].reduce((sum, name) => sum + Number(stateCounts.value[name] || 0), 0))
const lastUpdatedLabel = computed(() => lastUpdatedAt.value ? new Date(lastUpdatedAt.value).toLocaleTimeString() : '—')
const retryingJobID = ref('')
const actionError = ref('')
const readyBytes = computed(() => jobs.value
  .filter((job) => job.state === 'ready')
  .reduce((sum, job) => sum + Number(job.result_size || 0), 0)
)

const summary = computed(() => [
  { label: 'Active', value: activeCount.value, note: 'claimed by render worker' },
  { label: 'Queued', value: Number(stateCounts.value.queued || 0), note: 'waiting for replay VM' },
  { label: 'Failed', value: Number(stateCounts.value.failed || 0), note: 'needs investigation' },
  { label: 'Ready', value: Number(stateCounts.value.ready || 0), note: `${formatBytes(readyBytes.value)} shown output · ${Number(data.value?.total || 0)} jobs total` }
])

function tone(job: MediaRenderJob): 'success' | 'warning' | 'danger' | 'info' | 'neutral' {
  if (job.state === 'ready') return 'success'
  if (job.state === 'failed' || job.state === 'cancelled') return 'danger'
  if (job.state === 'queued') return 'warning'
  if (ACTIVE_STATES.has(job.state)) return 'info'
  return 'neutral'
}

function progressPercent(job: MediaRenderJob): number {
  if (job.state === 'ready') return 100
  const total = Number(job.segments_total || 0)
  if (total <= 0) return 0
  return Math.max(0, Math.min(100, Math.round(Number(job.segments_done || 0) * 100 / total)))
}

function progressLabel(job: MediaRenderJob): string {
  const total = Number(job.segments_total || 0)
  const done = Number(job.segments_done || 0)
  if (total > 0) return `${done}/${total} · ${progressPercent(job)}%`
  if (job.state === 'ready') return 'complete'
  return job.stage || job.state
}

function formatBytes(value?: number): string {
  const bytes = Number(value || 0)
  if (!bytes) return '—'
  const units = ['B', 'KB', 'MB', 'GB']
  let n = bytes
  let unit = 0
  while (n >= 1024 && unit < units.length - 1) {
    n /= 1024
    unit++
  }
  return `${n >= 10 || unit === 0 ? n.toFixed(0) : n.toFixed(1)} ${units[unit]}`
}

function ageLabel(ms?: number): string {
  const value = Number(ms || 0)
  if (!value) return '—'
  const seconds = Math.max(0, Math.floor((Date.now() - value) / 1000))
  if (seconds < 5) return 'now'
  if (seconds < 60) return `${seconds}s ago`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
  return `${Math.floor(seconds / 86400)}d ago`
}

function runtimeLabel(job: MediaRenderJob): string {
  const start = Number(job.started_at_unix_ms || 0)
  if (!start) return '—'
  const end = Number(job.finished_at_unix_ms || Date.now())
  const seconds = Math.max(0, Math.floor((end - start) / 1000))
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
}

function workerLabel(job: MediaRenderJob): string {
  if (!job.worker_id) return '—'
  const expires = Number(job.lease_expires_at_unix_ms || 0)
  if (expires > 0 && expires <= Date.now()) return `${job.worker_id} · lease expired`
  return job.worker_id
}

function retryable(job: MediaRenderJob): boolean {
  return job.state === 'failed' || job.state === 'cancelled'
}

async function retryJob(job: MediaRenderJob): Promise<void> {
  if (!retryable(job) || retryingJobID.value) return
  retryingJobID.value = job.id
  actionError.value = ''
  try {
    await retryMediaRenderJob(job.id)
    await refreshJobs()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Replay render retry failed'
  } finally {
    retryingJobID.value = ''
  }
}

function refresh(): void {
  actionError.value = ''
  void refreshJobs()
}
</script>

<template>
  <Panel
    title="Replay rendering"
    description="Durable replay jobs tracked by pokewall and executed on the dedicated render VM."
    compact
  >
    <ResourceState
      :state="state"
      title="No replay render jobs yet"
      :message="error || 'Jobs appear here as soon as a replay render is queued.'"
      :rows="5"
    >
      <template #actions>
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-md bg-white/10 px-2.5 py-1.5 text-xs font-semibold text-white ring-1 ring-white/10 hover:bg-white/15"
          @click="refresh"
        >
          <ArrowPathIcon class="size-3.5" aria-hidden="true" />
          Retry now
        </button>
      </template>

      <div class="mb-3 grid grid-cols-2 gap-px overflow-hidden rounded-lg bg-white/10 ring-1 ring-white/10 lg:grid-cols-4">
        <div v-for="item in summary" :key="item.label" class="bg-[#0b111a] px-3 py-3">
          <dt class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">{{ item.label }}</dt>
          <dd class="mt-1 font-mono text-lg font-semibold tabular-nums text-white">{{ item.value }}</dd>
          <p class="mt-0.5 text-[10px] text-slate-600">{{ item.note }}</p>
        </div>
      </div>

      <div v-if="actionError" class="mb-3 rounded-md border border-rose-300/20 bg-rose-400/8 px-3 py-2 text-xs text-rose-200" role="alert">
        {{ actionError }}
      </div>

      <div class="space-y-2 md:hidden">
        <article
          v-for="job in jobs"
          :key="`mobile-${job.id}`"
          class="rounded-lg border border-white/8 bg-black/15 p-3"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <a
                :href="`/?run=${encodeURIComponent(job.run_id)}#live`"
                class="block truncate font-mono text-xs text-cyan-300 hover:underline"
                :title="job.run_id"
              >{{ shortID(job.run_id) }}</a>
              <div class="mt-1 font-mono text-[9px] text-slate-600" :title="job.id">{{ job.id.slice(0, 18) }}</div>
            </div>
            <StatusBadge :tone="tone(job)">{{ job.state }}</StatusBadge>
          </div>

          <div class="mt-3">
            <div class="flex items-center justify-between gap-2 font-mono text-[10px] text-slate-400">
              <span>{{ progressLabel(job) }}</span>
              <span v-if="job.result_size" class="text-slate-600">{{ formatBytes(job.result_size) }}</span>
            </div>
            <div class="mt-1.5 h-1.5 overflow-hidden rounded-full bg-white/8">
              <div
                class="h-full rounded-full bg-cyan-300/80 transition-[width] duration-300"
                :style="{ width: `${progressPercent(job)}%` }"
              />
            </div>
          </div>

          <p v-if="job.last_error" class="mt-2 line-clamp-2 text-[10px] text-rose-300" :title="job.last_error">{{ job.last_error }}</p>

          <dl class="mt-3 grid grid-cols-2 gap-x-3 gap-y-2 text-[10px]">
            <div>
              <dt class="text-slate-600">Worker</dt>
              <dd class="mt-0.5 truncate font-mono text-slate-400" :title="workerLabel(job)">{{ workerLabel(job) }}</dd>
            </div>
            <div>
              <dt class="text-slate-600">Mode</dt>
              <dd class="mt-0.5 text-slate-400">{{ job.mode }}</dd>
            </div>
            <div>
              <dt class="text-slate-600">Runtime</dt>
              <dd class="mt-0.5 font-mono text-slate-400">{{ runtimeLabel(job) }}</dd>
            </div>
            <div>
              <dt class="text-slate-600">Updated</dt>
              <dd class="mt-0.5 font-mono text-slate-400">{{ ageLabel(job.updated_at_unix_ms) }}</dd>
            </div>
          </dl>

          <div v-if="retryable(job)" class="mt-3 flex justify-end">
            <button
              type="button"
              :disabled="Boolean(retryingJobID)"
              class="inline-flex items-center gap-1.5 rounded-md bg-amber-300/10 px-2.5 py-1.5 text-[10px] font-bold text-amber-100 ring-1 ring-amber-300/20 hover:bg-amber-300/15 disabled:opacity-50"
              @click="retryJob(job)"
            >
              <ArrowPathIcon class="size-3.5" aria-hidden="true" />
              {{ retryingJobID === job.id ? 'Retrying…' : 'Retry render' }}
            </button>
          </div>
        </article>
      </div>

      <div class="-mx-3 hidden overflow-x-auto sm:-mx-4 md:block">
        <table class="min-w-full divide-y divide-white/10 text-left">
          <thead>
            <tr>
              <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:px-4">Run</th>
              <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">State</th>
              <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Progress</th>
              <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Worker</th>
              <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Mode</th>
              <th class="px-3 py-2 text-right text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Runtime</th>
              <th class="px-3 py-2 text-right text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Updated</th>
              <th class="px-3 py-2 text-right text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:pr-4">Action</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-white/8">
            <tr v-for="job in jobs" :key="job.id">
              <td class="px-3 py-2.5 sm:px-4">
                <a
                  :href="`/?run=${encodeURIComponent(job.run_id)}#live`"
                  class="font-mono text-xs text-cyan-300 hover:underline"
                  :title="job.run_id"
                >{{ shortID(job.run_id) }}</a>
                <div class="mt-0.5 font-mono text-[9px] text-slate-600" :title="job.id">{{ job.id.slice(0, 18) }}</div>
              </td>
              <td class="px-3 py-2.5">
                <div class="flex items-center gap-2">
                  <StatusBadge :tone="tone(job)">{{ job.state }}</StatusBadge>
                  <span v-if="job.retry_count" class="font-mono text-[9px] text-amber-300">{{ job.retry_count }} retry</span>
                </div>
                <p v-if="job.stage && job.stage !== job.state" class="mt-1 text-[10px] text-slate-500">{{ job.stage }}</p>
                <p v-if="job.last_error" class="mt-1 max-w-xs truncate text-[10px] text-rose-300" :title="job.last_error">{{ job.last_error }}</p>
              </td>
              <td class="min-w-[10rem] px-3 py-2.5">
                <div class="flex items-center justify-between gap-2 font-mono text-[10px] text-slate-400">
                  <span>{{ progressLabel(job) }}</span>
                  <span v-if="job.result_size" class="text-slate-600">{{ formatBytes(job.result_size) }}</span>
                </div>
                <div class="mt-1.5 h-1.5 overflow-hidden rounded-full bg-white/8">
                  <div
                    class="h-full rounded-full bg-cyan-300/80 transition-[width] duration-300"
                    :style="{ width: `${progressPercent(job)}%` }"
                  />
                </div>
              </td>
              <td class="max-w-[15rem] px-3 py-2.5 font-mono text-[10px] text-slate-500" :title="workerLabel(job)">{{ workerLabel(job) }}</td>
              <td class="px-3 py-2.5 text-xs text-slate-500">{{ job.mode }}</td>
              <td class="px-3 py-2.5 text-right font-mono text-[10px] whitespace-nowrap text-slate-500">{{ runtimeLabel(job) }}</td>
              <td class="px-3 py-2.5 text-right font-mono text-[10px] whitespace-nowrap text-slate-500">{{ ageLabel(job.updated_at_unix_ms) }}</td>
              <td class="px-3 py-2.5 text-right sm:pr-4">
                <button
                  v-if="retryable(job)"
                  type="button"
                  :disabled="Boolean(retryingJobID)"
                  class="inline-flex items-center gap-1 rounded-md bg-amber-300/10 px-2 py-1 text-[10px] font-bold text-amber-100 ring-1 ring-amber-300/20 hover:bg-amber-300/15 disabled:opacity-50"
                  @click="retryJob(job)"
                >
                  <ArrowPathIcon class="size-3" aria-hidden="true" />
                  {{ retryingJobID === job.id ? 'Retrying…' : 'Retry' }}
                </button>
                <span v-else class="text-[10px] text-slate-700">—</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="mt-2 text-right font-mono text-[9px] text-slate-700">Last refreshed {{ lastUpdatedLabel }} · showing {{ jobs.length }} of {{ data?.total || 0 }}</p>
    </ResourceState>
  </Panel>
</template>
