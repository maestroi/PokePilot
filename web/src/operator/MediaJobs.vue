<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { ArrowPathIcon, ClipboardDocumentIcon, XMarkIcon } from '@heroicons/vue/20/solid'
import { cancelMediaRenderJob, deleteMediaRenderJob, getMediaRenderJobs, retryMediaRenderJob } from '../shared/api/client'
import type { MediaRenderJob } from '../shared/api/types'
import ActionMenu, { type ActionMenuItem } from '../shared/components/ActionMenu.vue'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { presentJob } from '../shared/status'
import { operatorHref } from '../shared/urls'
import {
  JOB_VIEWS,
  cancellable,
  copyReceipt,
  elapsedLabel,
  filterJobs,
  formatBytes,
  jobInView,
  jobProgress,
  msISO,
  removable,
  rendererLabel,
  rendererTitle,
  retryable,
  workerLabel,
  type MediaState
} from './mediaQuery'
import { formatInteger, middleTruncate, timeAgo } from './runs'

const props = defineProps<{ state: MediaState }>()
const emit = defineEmits<{ navigate: [next: MediaState, replace?: boolean]; announce: [text: string] }>()

const resource = usePollingResource((signal) => getMediaRenderJobs(50, signal), {
  intervalMs: 2000,
  isEmpty: (snapshot) => snapshot.jobs.length === 0
})
const jobs = computed(() => resource.data.value?.jobs ?? [])
const rows = computed(() => filterJobs(jobs.value, props.state))
const viewCounts = computed(() => Object.fromEntries(JOB_VIEWS.map((view) => [view.id, jobs.value.filter((job) => jobInView(job, view.id)).length])))
const loaded = computed(() => resource.state.value !== 'loading' && resource.state.value !== 'error' && resource.state.value !== 'empty')
const hasFilters = computed(() => Boolean(props.state.run) || props.state.jobView !== 'all')

function patch(change: Partial<MediaState>): void {
  emit('navigate', { ...props.state, ...change })
}
function status(job: MediaRenderJob) {
  return presentJob(job.state, jobProgress(job))
}
function progressText(job: MediaRenderJob): string {
  const total = Number(job.segments_total || 0)
  if (total > 0) return `${Number(job.segments_done || 0)}/${total} · ${jobProgress(job)}%`
  return job.state === 'ready' ? 'Complete' : (job.stage || job.state)
}
function runHref(runID: string): string {
  return operatorHref('runs', { run: runID, job: '' }, window.location.search)
}
async function copy(value: string, noun: string): Promise<void> {
  emit('announce', await copyReceipt(value, noun))
}

// ---- Selected job (detail pane) ------------------------------------------------------
const paneEl = ref<HTMLElement | null>(null)
const selected = computed(() => jobs.value.find((job) => job.id === props.state.job) || null)
function openJob(id: string, focusPane = true): void {
  if (id !== props.state.job) emit('navigate', { ...props.state, job: id }, Boolean(props.state.job))
  if (focusPane) void nextTick(() => paneEl.value?.focus())
}
function closeJob(): void {
  const id = props.state.job
  emit('navigate', { ...props.state, job: '' })
  void nextTick(() => rowEl(id)?.focus())
}
function rowEl(id: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-job-row="${CSS.escape(id)}"]`)
}
function onKeydown(event: KeyboardEvent): void {
  if (!props.state.job || pending.value || event.metaKey || event.ctrlKey || event.altKey) return
  const target = event.target as HTMLElement | null
  if (target?.closest('input, select, textarea, [role="menu"]')) return
  if (event.key === 'Escape') {
    event.preventDefault()
    closeJob()
  } else if ((event.key === 'ArrowDown' || event.key === 'ArrowUp') && !paneEl.value?.contains(target)) {
    const index = rows.value.findIndex((job) => job.id === props.state.job)
    const next = rows.value[index + (event.key === 'ArrowDown' ? 1 : -1)]
    if (!next) return
    event.preventDefault()
    openJob(next.id, false)
    void nextTick(() => rowEl(next.id)?.focus())
  }
}
onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => window.removeEventListener('keydown', onKeydown))

// ---- Lifecycle actions (retry runs at once; cancel and remove confirm) -----------------
type JobAction = 'retry' | 'cancel' | 'remove'
const busy = ref<{ id: string; action: JobAction } | null>(null)
const pending = ref<{ job: MediaRenderJob; action: 'cancel' | 'remove' } | null>(null)
const actionError = ref('')

function request(job: MediaRenderJob, action: JobAction): void {
  if (busy.value) return
  if (action === 'retry') void run(job, action)
  else pending.value = { job, action }
}
async function run(job: MediaRenderJob, action: JobAction): Promise<void> {
  if (action === 'retry' && !retryable(job)) return
  if (action === 'cancel' && !cancellable(job)) return
  if (action === 'remove' && !removable(job)) return
  busy.value = { id: job.id, action }
  actionError.value = ''
  try {
    if (action === 'retry') await retryMediaRenderJob(job.id)
    else if (action === 'cancel') await cancelMediaRenderJob(job.id)
    else await deleteMediaRenderJob(job.id)
    emit('announce', action === 'retry' ? 'Render job queued again' : action === 'cancel' ? 'Render job cancelled' : 'Render job removed')
    if (action === 'remove' && props.state.job === job.id) emit('navigate', { ...props.state, job: '' }, true)
    await resource.retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : `Replay render ${action} failed`
  } finally {
    busy.value = null
    pending.value = null
  }
}
function busyLabel(job: MediaRenderJob, action: JobAction, idle: string): string {
  if (busy.value?.id !== job.id || busy.value.action !== action) return idle
  return action === 'retry' ? 'Retrying…' : action === 'cancel' ? 'Cancelling…' : 'Removing…'
}
const confirmTitle = computed(() => {
  const target = pending.value
  if (!target) return ''
  return target.action === 'cancel' ? `Cancel replay rendering for ${target.job.run_id}?` : `Remove render job for ${target.job.run_id}?`
})
const confirmMessage = computed(() => {
  const target = pending.value
  if (!target) return ''
  if (target.action === 'cancel') return 'The current render attempt will stop and can be retried later.'
  return target.job.state === 'ready'
    ? 'The rendered replay file and run stay intact; only this durable job record is removed.'
    : 'This removes the terminal job record. The run itself is not deleted.'
})

function menu(job: MediaRenderJob): ActionMenuItem[] {
  const items: ActionMenuItem[] = [
    { label: 'Open source run', href: runHref(job.run_id) },
    { label: 'Copy job ID', select: () => copy(job.id, 'Job ID') },
    { label: 'Copy run ID', select: () => copy(job.run_id, 'Run ID') }
  ]
  const changes: ActionMenuItem[] = []
  if (retryable(job)) changes.push({ label: 'Retry', select: () => request(job, 'retry') })
  if (cancellable(job)) changes.push({ label: 'Cancel render…', select: () => request(job, 'cancel'), danger: true })
  if (removable(job)) changes.push({ label: 'Remove job record…', select: () => request(job, 'remove'), danger: true })
  if (changes.length) changes[0].separator = true
  return [...items, ...changes]
}
</script>

<template>
  <div class="space-y-3">
    <div class="flex flex-wrap items-center gap-3">
      <div class="inline-flex max-w-full overflow-x-auto border border-rule-strong" role="group" aria-label="Render job state">
        <button
          v-for="view in JOB_VIEWS"
          :key="view.id"
          type="button"
          :aria-pressed="state.jobView === view.id"
          :class="state.jobView === view.id ? 'bg-selected text-fg' : 'text-fg-2 hover:bg-raised hover:text-fg'"
          class="h-7 shrink-0 border-r border-rule-strong px-3 whitespace-nowrap last:border-r-0 max-md:h-10"
          @click="patch({ jobView: view.id })"
        >
          {{ view.label }} <span class="font-mono text-meta text-fg-2">{{ resource.data.value ? formatInteger(viewCounts[view.id]) : '' }}</span>
        </button>
      </div>
      <span v-if="resource.state.value === 'refreshing'" class="inline-flex items-center gap-1.5 text-meta text-fg-2" role="status">
        <span class="size-1.5 bg-accent motion-safe:animate-pulse" aria-hidden="true" /> Refreshing
      </span>
      <button type="button" class="ui-btn ui-icon-btn ml-auto max-md:size-10" aria-label="Refresh render jobs" title="Refresh" @click="resource.retry()">
        <ArrowPathIcon class="size-4" aria-hidden="true" />
      </button>
    </div>

    <p v-if="actionError" class="border border-danger/40 bg-danger/10 px-3 py-2 text-danger" role="alert">✕ {{ actionError }}</p>

    <div :class="state.job ? 'xl:grid xl:grid-cols-[minmax(0,1fr)_400px] xl:gap-3 min-[1440px]:grid-cols-[minmax(0,1fr)_440px]' : ''">
      <div class="min-w-0 border border-rule-strong bg-surface">
        <div class="flex min-h-11 flex-wrap items-center gap-2 border-b border-rule px-3 py-2">
          <button
            v-if="state.run"
            type="button"
            class="inline-flex h-6 max-w-full items-center gap-1 rounded-[3px] border border-rule-strong px-2 text-meta text-fg hover:bg-raised max-md:h-8"
            :aria-label="`Remove filter run ${state.run}`"
            @click="patch({ run: '' })"
          >
            <span class="truncate">Run: <span class="font-mono">{{ middleTruncate(state.run) }}</span></span> <XMarkIcon class="size-3.5 shrink-0 text-fg-2" aria-hidden="true" />
          </button>
          <span v-else class="text-meta text-fg-2">Durable replay jobs tracked by the wall and executed on the render VM</span>
          <span class="ml-auto text-meta whitespace-nowrap text-fg-2 tabular-nums" aria-live="polite">{{ formatInteger(rows.length) }} shown · {{ formatInteger(Number(resource.data.value?.total || 0)) }} total</span>
        </div>

        <ResourceState v-if="resource.state.value === 'stale'" state="stale" class="p-3" title="Showing the last loaded render jobs" :message="`Refresh failed: ${resource.error.value}`">
          <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="resource.retry()">Retry</button></template>
        </ResourceState>

        <!-- Desktop / tablet table -->
        <div class="hidden md:block" role="region" aria-label="Render jobs table">
          <table class="w-full table-fixed border-collapse text-left">
            <thead>
              <tr class="h-9 border-b border-rule-strong bg-raised text-meta font-medium text-fg-2">
                <th class="w-40 px-3 font-medium">Status</th>
                <th class="px-3 font-medium">Source run</th>
                <th class="w-48 px-3 font-medium max-lg:hidden">Progress</th>
                <th class="w-28 px-3 font-medium" :class="state.job ? 'hidden min-[1680px]:table-cell' : 'max-xl:hidden'">Renderer</th>
                <th class="w-28 px-3 font-medium max-xl:hidden">Updated</th>
                <th class="w-11 pr-2"><span class="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody v-if="resource.state.value === 'loading'" aria-busy="true">
              <tr v-for="n in 6" :key="n" class="h-14 border-b border-rule">
                <td colspan="6" class="px-3"><div class="h-4 bg-raised motion-safe:animate-pulse" :class="n % 2 ? 'w-3/4' : 'w-1/2'" /></td>
              </tr>
            </tbody>
            <tbody v-else-if="loaded">
              <tr
                v-for="job in rows"
                :key="job.id"
                :data-job-row="job.id"
                tabindex="0"
                :aria-selected="job.id === state.job"
                :class="job.id === state.job ? 'bg-selected shadow-[inset_2px_0_0_var(--color-accent)]' : 'hover:bg-raised'"
                class="group h-14 cursor-pointer border-b border-rule align-middle focus-visible:outline-offset-[-2px]"
                @click="openJob(job.id)"
                @keydown.enter.self="openJob(job.id)"
              >
                <td class="px-3">
                  <StatusPill :status="status(job)" class="max-w-full overflow-hidden" />
                  <div v-if="job.retry_count || job.failure_class" class="mt-0.5 truncate text-meta text-warn">
                    {{ job.failure_class || '' }}{{ job.failure_class && job.retry_count ? ' · ' : '' }}{{ job.retry_count ? `${job.retry_count} ${job.retry_count === 1 ? 'retry' : 'retries'}` : '' }}
                  </div>
                </td>
                <td class="px-3">
                  <div class="flex min-w-0 items-center gap-1">
                    <span class="truncate font-mono" :title="job.run_id">{{ middleTruncate(job.run_id) }}</span>
                    <button
                      type="button"
                      class="ui-btn ui-btn-sm ui-icon-btn size-6 shrink-0 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 focus-visible:opacity-100"
                      :aria-label="`Copy run ID ${job.run_id}`"
                      @click.stop="copy(job.run_id, 'Run ID')"
                    >
                      <ClipboardDocumentIcon class="size-4" aria-hidden="true" />
                    </button>
                  </div>
                  <div class="truncate font-mono text-meta text-fg-2" :title="job.id">{{ job.id }}</div>
                </td>
                <td class="px-3 max-lg:hidden">
                  <div class="flex items-center justify-between gap-2 font-mono text-meta text-fg-2">
                    <span class="truncate">{{ progressText(job) }}</span>
                    <span v-if="job.result_size">{{ formatBytes(job.result_size) }}</span>
                  </div>
                  <div class="ui-meter mt-1.5"><span class="bg-accent" :style="{ width: `${jobProgress(job) || 0}%` }" /></div>
                </td>
                <td class="px-3" :class="state.job ? 'hidden min-[1680px]:table-cell' : 'max-xl:hidden'" :title="rendererTitle(job)">{{ rendererLabel(job) }}</td>
                <td class="px-3 whitespace-nowrap max-xl:hidden" :title="msISO(job.updated_at_unix_ms)">{{ timeAgo(Number(job.updated_at_unix_ms || 0) / 1000) }}</td>
                <td class="pr-2 text-right" @click.stop @keydown.stop>
                  <ActionMenu :label="`More actions for render job ${job.id}`" :items="menu(job)" />
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Narrow list (< 768px) -->
        <div class="md:hidden">
          <div v-if="resource.state.value === 'loading'" class="space-y-4 p-3" aria-busy="true">
            <div v-for="n in 5" :key="n" class="h-12 bg-raised motion-safe:animate-pulse" />
          </div>
          <ul v-else-if="loaded" role="list" aria-label="Render jobs">
            <li
              v-for="job in rows"
              :key="job.id"
              :class="job.id === state.job ? 'bg-selected shadow-[inset_2px_0_0_var(--color-accent)]' : ''"
              class="flex gap-3 border-b border-rule px-3 py-2.5"
            >
              <div class="min-w-0 flex-1">
                <div class="flex min-w-0 items-center gap-2">
                  <StatusPill :status="status(job)" class="shrink-0" />
                  <span class="ml-auto shrink-0 text-meta text-fg-2" :title="msISO(job.updated_at_unix_ms)">{{ timeAgo(Number(job.updated_at_unix_ms || 0) / 1000) }}</span>
                </div>
                <button type="button" class="mt-1 block w-full truncate text-left font-mono text-accent" :title="job.run_id" @click="openJob(job.id)">{{ middleTruncate(job.run_id) }}</button>
                <div class="mt-1 flex min-w-0 items-center gap-2 text-meta text-fg-2">
                  <span class="truncate">{{ progressText(job) }}</span>
                  <span class="ml-auto shrink-0">{{ rendererLabel(job) }}</span>
                </div>
              </div>
              <ActionMenu class="shrink-0" :label="`More actions for render job ${job.id}`" :items="menu(job)" />
            </li>
          </ul>
        </div>

        <ResourceState v-if="resource.state.value === 'error'" state="error" class="m-3" title="Couldn’t load render jobs" :message="`${resource.error.value}. Retry when the wall is reachable.`">
          <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="resource.retry()">Retry</button></template>
        </ResourceState>
        <div v-else-if="resource.state.value === 'empty' || (loaded && !rows.length)" class="flex flex-col items-center gap-2 px-4 py-12 text-center">
          <span class="text-[20px] leading-5 text-fg-3" aria-hidden="true">○</span>
          <template v-if="resource.state.value === 'empty'">
            <p class="text-title font-semibold">No replay render jobs yet</p>
            <p class="text-fg-2">Jobs appear here as soon as a replay render is queued.</p>
          </template>
          <template v-else>
            <p class="text-title font-semibold">No render jobs match</p>
            <p class="text-fg-2">{{ state.run ? 'No job for this run is in the recent job list.' : 'Nothing in this state right now.' }}</p>
            <button v-if="hasFilters" type="button" class="ui-btn mt-1" @click="patch({ run: '', jobView: 'all' })">Show all jobs</button>
          </template>
        </div>
      </div>

      <!-- Detail pane: docked ≥ 1280px, overlay sheet below, full screen < 768px -->
      <template v-if="state.job">
        <div class="fixed inset-0 z-40 bg-black/50 xl:hidden" aria-hidden="true" @click="closeJob" />
        <aside
          ref="paneEl"
          tabindex="-1"
          aria-label="Render job details"
          class="fixed inset-0 z-50 border-rule-strong shadow-[0_8px_24px_rgb(0_0_0/45%)] outline-none md:left-auto md:w-[440px] md:border-l xl:sticky xl:top-14 xl:z-auto xl:h-[calc(100vh-168px)] xl:self-start xl:w-auto xl:border xl:shadow-none"
        >
          <section class="flex h-full min-h-0 flex-col bg-surface" aria-labelledby="job-pane-title">
            <header class="flex min-h-12 shrink-0 items-center gap-2 border-b border-rule bg-raised px-3 py-2">
              <h2 id="job-pane-title" class="min-w-0 truncate font-mono text-title font-medium" :title="state.job">
                <span class="sr-only">Render job </span>{{ state.job }}
              </h2>
              <button type="button" class="ui-btn ui-btn-sm ui-icon-btn shrink-0" aria-label="Copy job ID" title="Copy job ID" @click="copy(state.job, 'Job ID')">
                <ClipboardDocumentIcon class="size-4" aria-hidden="true" />
              </button>
              <StatusPill v-if="selected" :status="status(selected)" class="ml-auto" />
              <button type="button" class="ui-btn ui-btn-sm ui-icon-btn shrink-0" :class="selected ? '' : 'ml-auto'" aria-label="Close render job details" title="Close (Esc)" @click="closeJob">
                <XMarkIcon class="size-4" aria-hidden="true" />
              </button>
            </header>

            <div v-if="!selected && resource.state.value === 'loading'" class="flex-1 space-y-3 p-4" aria-busy="true" aria-label="Loading render job">
              <div v-for="n in 6" :key="n" class="h-4 bg-raised motion-safe:animate-pulse" :class="n % 3 === 0 ? 'w-2/3' : 'w-full'" />
            </div>
            <div v-else-if="!selected" class="flex flex-1 flex-col items-start gap-3 p-4" role="status">
              <p class="text-title">No matching render job is in the recent job list for this selection.</p>
              <p class="text-fg-2">It may have been removed, or it is older than the 50 most recent jobs.</p>
              <div class="flex gap-2">
                <button type="button" class="ui-btn" @click="patch({ job: '', run: '', jobView: 'all' })">Show all jobs</button>
                <button type="button" class="ui-btn ui-btn-quiet" @click="closeJob">Close</button>
              </div>
            </div>

            <template v-else>
              <div class="min-h-0 flex-1 overflow-y-auto px-3 pb-4">
                <h3 class="mt-3 mb-1 text-title font-semibold">Progress</h3>
                <div class="flex items-center gap-3">
                  <div class="ui-meter"><span class="bg-accent" :style="{ width: `${jobProgress(selected) || 0}%` }" /></div>
                  <span class="font-mono text-meta text-fg-2">{{ progressText(selected) }}</span>
                </div>
                <p v-if="selected.last_error" class="mt-3 border border-danger/40 bg-danger/10 px-3 py-2 text-danger" role="status">
                  ✕ Render failed<template v-if="selected.failure_class"> ({{ selected.failure_class }})</template>. The run and its recording are unaffected; retry the job when the cause is fixed.
                </p>
                <details v-if="selected.last_error" class="mt-2">
                  <summary class="cursor-pointer text-meta text-accent">Details</summary>
                  <p class="mt-1 max-w-[72ch] font-mono text-meta break-words whitespace-pre-wrap text-fg-2">{{ selected.last_error }}</p>
                </details>

                <h3 class="mt-5 mb-1 text-title font-semibold">Job</h3>
                <dl class="ui-kv">
                  <dt>Source run</dt>
                  <dd class="flex min-w-0 items-start gap-1">
                    <a :href="runHref(selected.run_id)" class="min-w-0 font-mono break-all text-accent hover:underline">{{ selected.run_id }}</a>
                    <button type="button" class="ui-btn ui-btn-sm ui-icon-btn size-6 shrink-0" aria-label="Copy run ID" @click="copy(selected.run_id, 'Run ID')">
                      <ClipboardDocumentIcon class="size-4" aria-hidden="true" />
                    </button>
                  </dd>
                  <dt>Stage</dt><dd>{{ selected.stage || selected.state }}</dd>
                  <dt>Renderer</dt><dd :title="rendererTitle(selected)">{{ rendererLabel(selected) }} <span class="block text-meta text-fg-2">{{ rendererTitle(selected) }}</span></dd>
                  <dt>Worker</dt><dd class="font-mono text-meta">{{ workerLabel(selected) }}</dd>
                  <dt>Retries</dt><dd class="font-mono">{{ selected.retry_count || 0 }}</dd>
                  <dt>Output size</dt><dd class="font-mono">{{ formatBytes(selected.result_size) }}</dd>
                  <dt>Runtime</dt><dd class="font-mono">{{ elapsedLabel(selected.started_at_unix_ms, selected.finished_at_unix_ms || Date.now()) }}</dd>
                  <dt>Created</dt><dd :title="msISO(selected.created_at_unix_ms)">{{ timeAgo(Number(selected.created_at_unix_ms || 0) / 1000) }}</dd>
                  <dt>Updated</dt><dd :title="msISO(selected.updated_at_unix_ms)">{{ timeAgo(Number(selected.updated_at_unix_ms || 0) / 1000) }}</dd>
                  <template v-if="selected.artifact_key">
                    <dt>Artifact key</dt><dd class="font-mono text-meta break-all">{{ selected.artifact_key }}</dd>
                  </template>
                </dl>
              </div>
              <footer class="flex shrink-0 flex-wrap items-center gap-2 border-t border-rule px-3 py-2">
                <button v-if="retryable(selected)" type="button" class="ui-btn ui-btn-primary max-md:h-10" :disabled="Boolean(busy)" @click="request(selected, 'retry')">{{ busyLabel(selected, 'retry', 'Retry') }}</button>
                <a :href="runHref(selected.run_id)" class="ui-btn max-md:h-10">Open source run</a>
                <button v-if="cancellable(selected)" type="button" class="ui-btn ui-btn-danger max-md:h-10" :disabled="Boolean(busy)" @click="request(selected, 'cancel')">{{ busyLabel(selected, 'cancel', 'Cancel render…') }}</button>
                <button v-if="removable(selected)" type="button" class="ui-btn ui-btn-danger max-md:h-10" :disabled="Boolean(busy)" :title="selected.state === 'ready' ? 'Remove the job record only; keep the replay file and run.' : 'Remove this terminal job record.'" @click="request(selected, 'remove')">{{ busyLabel(selected, 'remove', 'Remove job record…') }}</button>
              </footer>
            </template>
          </section>
        </aside>
      </template>
    </div>

    <ConfirmDialog
      :open="Boolean(pending)"
      :title="confirmTitle"
      :message="confirmMessage"
      :confirm-label="pending?.action === 'cancel' ? 'Cancel render' : 'Remove job record'"
      :cancel-label="pending?.action === 'cancel' ? 'Keep rendering' : 'Cancel'"
      :busy="Boolean(busy)"
      danger
      @close="pending = busy ? pending : null"
      @confirm="pending && run(pending.job, pending.action)"
    />
  </div>
</template>
