<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { ArrowPathIcon, ClipboardDocumentIcon, XMarkIcon } from '@heroicons/vue/20/solid'
import { getDashboard, getMediaRenderJobs, getRun } from '../shared/api/client'
import { getSpectatorControl, patchSpectatorRunControl } from '../shared/api/spectator-control'
import type { DashboardRun } from '../shared/api/types'
import ActionMenu, { type ActionMenuItem } from '../shared/components/ActionMenu.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { presentRecording, presentRunStatus } from '../shared/status'
import { operatorHref, replayURL, spectatorURL } from '../shared/urls'
import MediaOutputPane from './MediaOutputPane.vue'
import {
  LIBRARY_VIEWS,
  canFeature,
  canOpenPublic,
  copyReceipt,
  filterLibrary,
  formatMediaHref,
  isLiveRun,
  latestJobByRun,
  publicActionLabel,
  sortLibrary,
  type MediaState,
  type RecordingFilter,
  type VisibilityFilter
} from './mediaQuery'
import { gameTitle } from './operations'
import { recordingSummary } from './runRecording'
import { formatInteger, middleTruncate, timeAgo } from './runs'

const props = defineProps<{ state: MediaState; publicBase: string }>()
const emit = defineEmits<{ navigate: [next: MediaState, replace?: boolean]; announce: [text: string] }>()

const RECENT_LIMIT = 48
const search = () => window.location.search

// ---- Data ------------------------------------------------------------------------------
const active = usePollingResource((signal) => getDashboard({ active: true }, signal), { intervalMs: 5000 })
const recent = usePollingResource((signal) => getDashboard({ status: 'done', limit: RECENT_LIMIT }, signal), { intervalMs: 15000 })
const control = usePollingResource((signal) => getSpectatorControl(signal), { intervalMs: 5000 })
const jobs = usePollingResource((signal) => getMediaRenderJobs(100, signal), { intervalMs: 5000 })

// A linked run (from Runs or Live) may be older than the recent page; load it directly.
const focused = ref<DashboardRun | null>(null)
const focusedError = ref('')
watch(() => props.state.run, async (runID, _previous, onCleanup) => {
  let cancelled = false
  onCleanup(() => { cancelled = true })
  focused.value = null
  focusedError.value = ''
  if (!runID) return
  try {
    const run = await getRun(runID)
    if (!cancelled) focused.value = run
  } catch (cause) {
    if (!cancelled) focusedError.value = cause instanceof Error ? cause.message : 'Run unavailable'
  }
}, { immediate: true })

const runs = computed<DashboardRun[]>(() => {
  const seen = new Set<string>()
  return sortLibrary([...(active.data.value?.runs || []), ...(recent.data.value?.runs || []), ...(focused.value ? [focused.value] : [])]
    .filter((run) => {
      if (!run.run_id || seen.has(run.run_id)) return false
      seen.add(run.run_id)
      return true
    }))
})
const jobByRun = computed(() => latestJobByRun(jobs.data.value?.jobs || []))
const jobsState = computed(() => {
  const value = jobs.state.value
  return value === 'loading' || value === 'stale' || value === 'error' ? value : 'ready'
})
const featuredRunID = computed(() => control.data.value?.featured_run_id || '')

function isPublic(run: DashboardRun): boolean {
  return control.data.value?.runs?.[run.run_id]?.visible !== false
}
function recording(run: DashboardRun) {
  return recordingSummary(run, jobByRun.value.get(run.run_id), jobsState.value)
}
function status(run: DashboardRun) {
  return isLiveRun(run) ? presentRunStatus(run) : presentRecording(recording(run))
}

const rows = computed(() => filterLibrary(runs.value, props.state, { isPublic, recordingState: (run) => recording(run).state }))
const viewCounts = computed(() => ({
  all: runs.value.length,
  finished: runs.value.filter((run) => !isLiveRun(run)).length,
  live: runs.value.filter(isLiveRun).length
}))
const resourceState = computed(() => {
  if (active.state.value === 'error' && recent.state.value === 'error') return 'error'
  if (active.state.value === 'loading' && recent.state.value === 'loading') return 'loading'
  if (!runs.value.length) return 'empty'
  return 'ready'
})
const loaded = computed(() => resourceState.value === 'ready')
const stale = computed(() => [active, recent, control].find((resource) => resource.state.value === 'stale'))

// ---- Filters ------------------------------------------------------------------------------
function patch(change: Partial<MediaState>, replace = false): void {
  emit('navigate', { ...props.state, ...change }, replace)
}
const hasFilters = computed(() => Boolean(props.state.q || props.state.recording || props.state.visibility))
function clearFilters(): void {
  searchInput.value = ''
  patch({ q: '', recording: '', visibility: '' })
}
const RECORDING_LABELS: Record<Exclude<RecordingFilter, ''>, string> = { ready: 'Ready', rendering: 'Generating', failed: 'Failed', none: 'No replay' }
const VISIBILITY_LABELS: Record<Exclude<VisibilityFilter, ''>, string> = { public: 'Public', private: 'Private' }
const chips = computed(() => [
  ...(props.state.q ? [{ key: 'q', label: `Search: ${props.state.q}` }] : []),
  ...(props.state.recording ? [{ key: 'recording', label: `Replay: ${RECORDING_LABELS[props.state.recording]}` }] : []),
  ...(props.state.visibility ? [{ key: 'visibility', label: `Public site: ${VISIBILITY_LABELS[props.state.visibility]}` }] : [])
])
function removeChip(key: string): void {
  if (key === 'q') { searchInput.value = ''; patch({ q: '' }) }
  else patch({ [key]: '' })
}

const searchInput = ref(props.state.q)
const searchEl = ref<HTMLInputElement | null>(null)
let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(() => props.state.q, (value) => { if (value !== searchInput.value.trim()) searchInput.value = value })
watch(searchInput, (value) => {
  if (searchTimer !== null) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    const q = value.trim()
    if (q !== props.state.q) patch({ q }, true)
  }, 300)
})

// ---- Selected output (detail pane) ------------------------------------------------------
const paneEl = ref<HTMLElement | null>(null)
const selected = computed(() => runs.value.find((run) => run.run_id === props.state.run) || null)
function openRun(runID: string, focusPane = true): void {
  if (runID !== props.state.run) patch({ run: runID }, Boolean(props.state.run))
  if (focusPane) void nextTick(() => paneEl.value?.focus())
}
function closeRun(): void {
  const runID = props.state.run
  patch({ run: '' })
  void nextTick(() => rowEl(runID)?.focus())
}
function rowEl(runID: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-media-row="${CSS.escape(runID)}"]`)
}
function onKeydown(event: KeyboardEvent): void {
  if (event.metaKey || event.ctrlKey || event.altKey || document.querySelector('[role="dialog"]')) return
  const target = event.target as HTMLElement | null
  const typing = Boolean(target?.closest('input, select, textarea, video, [role="menu"]'))
  if (event.key === '/' && !typing) {
    event.preventDefault()
    searchEl.value?.focus()
    return
  }
  if (!props.state.run || typing) return
  if (event.key === 'Escape') {
    event.preventDefault()
    closeRun()
  } else if ((event.key === 'ArrowDown' || event.key === 'ArrowUp') && !paneEl.value?.contains(target)) {
    const index = rows.value.findIndex((run) => run.run_id === props.state.run)
    const next = rows.value[index + (event.key === 'ArrowDown' ? 1 : -1)]
    if (!next) return
    event.preventDefault()
    openRun(next.run_id, false)
    void nextTick(() => rowEl(next.run_id)?.focus())
  }
}
onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
  if (searchTimer !== null) clearTimeout(searchTimer)
})

// ---- Actions ---------------------------------------------------------------------------
const busyRunID = ref('')
const actionError = ref('')
async function copy(value: string, noun: string): Promise<void> {
  emit('announce', await copyReceipt(value, noun))
}
async function setControl(run: DashboardRun, change: { visible?: boolean; featured?: boolean }): Promise<void> {
  if (busyRunID.value) return
  busyRunID.value = run.run_id
  actionError.value = ''
  try {
    await patchSpectatorRunControl(run.run_id, change)
    await control.retry()
    if (change.visible !== undefined) emit('announce', change.visible ? 'Visible on the public site' : 'Hidden from the public site')
    else emit('announce', change.featured ? 'Featured on the public site' : 'No longer featured')
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Public visibility update failed'
  } finally {
    busyRunID.value = ''
  }
}
function openPublic(run: DashboardRun): void {
  if (!canOpenPublic(run, isPublic(run))) return
  if (!props.publicBase) {
    actionError.value = 'Set POKEPILOT_PUBLIC_BASE_URL on the operator UI to enable public links.'
    return
  }
  window.open(run.status === 'done' ? replayURL(props.publicBase, run.run_id) : spectatorURL(props.publicBase, run.run_id), '_blank', 'noopener,noreferrer')
}
function sourceHref(run: DashboardRun): string {
  return operatorHref(isLiveRun(run) ? 'live' : 'runs', { run: run.run_id, job: '' }, window.location.search)
}
function refresh(): void {
  actionError.value = ''
  void Promise.allSettled([active.retry(), recent.retry(), control.retry(), jobs.retry()])
}

function menu(run: DashboardRun): ActionMenuItem[] {
  const job = jobByRun.value.get(run.run_id)
  const items: ActionMenuItem[] = [
    { label: 'Open source run', href: sourceHref(run) },
    { label: 'Copy run ID', select: () => copy(run.run_id, 'Run ID') }
  ]
  if (job) items.push({ label: 'Open render job', href: formatMediaHref({ ...props.state, tab: 'jobs', run: run.run_id, job: job.id }, window.location.search) })
  if (canOpenPublic(run, isPublic(run))) items.push({ label: publicActionLabel(run, true), select: () => openPublic(run), separator: true })
  items.push({ label: 'Visible on public site', checked: isPublic(run), select: () => setControl(run, { visible: !isPublic(run) }), separator: !canOpenPublic(run, isPublic(run)) })
  if (canFeature(run) || featuredRunID.value === run.run_id) {
    items.push({ label: featuredRunID.value === run.run_id ? 'Clear feature' : 'Feature live', select: () => setControl(run, { featured: featuredRunID.value !== run.run_id }) })
  }
  return items
}
</script>

<template>
  <div class="space-y-3">
    <div class="flex flex-wrap items-center gap-3">
      <div class="inline-flex border border-rule-strong" role="group" aria-label="Library view">
        <button
          v-for="view in LIBRARY_VIEWS"
          :key="view.id"
          type="button"
          :aria-pressed="state.view === view.id"
          :class="state.view === view.id ? 'bg-selected text-fg' : 'text-fg-2 hover:bg-raised hover:text-fg'"
          class="h-7 border-r border-rule-strong px-3 whitespace-nowrap last:border-r-0 max-md:h-10"
          @click="patch({ view: view.id })"
        >
          {{ view.label }} <span class="font-mono text-meta text-fg-2">{{ loaded ? formatInteger(viewCounts[view.id]) : '' }}</span>
        </button>
      </div>
      <button type="button" class="ui-btn ui-icon-btn ml-auto max-md:size-10" aria-label="Refresh library" title="Refresh" @click="refresh">
        <ArrowPathIcon class="size-4" aria-hidden="true" />
      </button>
    </div>

    <p v-if="actionError" class="border border-danger/40 bg-danger/10 px-3 py-2 text-danger" role="alert">✕ {{ actionError }}</p>
    <p v-if="focusedError && state.run" class="border border-warn/40 bg-warn/10 px-3 py-2 text-warn" role="status">
      ▲ Run {{ state.run }} couldn’t be loaded: {{ focusedError }}.
      <a :href="operatorHref('live', { run: state.run, job: '' }, search())" class="underline">Inspect source run</a>
    </p>

    <div :class="state.run ? 'xl:grid xl:grid-cols-[minmax(0,1fr)_400px] xl:gap-3 min-[1440px]:grid-cols-[minmax(0,1fr)_440px]' : ''">
      <div class="min-w-0 border border-rule-strong bg-surface">
        <div class="border-b border-rule px-3 py-2">
          <div class="flex flex-wrap items-center gap-2">
            <div class="w-full md:w-60">
              <label for="media-search" class="sr-only">Search media</label>
              <input
                id="media-search"
                ref="searchEl"
                v-model="searchInput"
                type="search"
                autocomplete="off"
                spellcheck="false"
                placeholder="Search run ID, game or goal…  /"
                class="block h-7 w-full rounded-[3px] border border-control bg-canvas px-2.5 text-fg placeholder:text-fg-3 max-md:h-10"
              >
            </div>
            <select
              :value="state.recording"
              aria-label="Replay state"
              class="h-7 rounded-[3px] border border-control bg-canvas px-2 text-fg max-md:h-10 max-md:flex-1"
              @change="patch({ recording: ($event.target as HTMLSelectElement).value as RecordingFilter })"
            >
              <option value="">Replay: any</option>
              <option v-for="(label, key) in RECORDING_LABELS" :key="key" :value="key">{{ label }}</option>
            </select>
            <select
              :value="state.visibility"
              aria-label="Public site"
              class="h-7 rounded-[3px] border border-control bg-canvas px-2 text-fg max-md:h-10 max-md:flex-1"
              @change="patch({ visibility: ($event.target as HTMLSelectElement).value as VisibilityFilter })"
            >
              <option value="">Public site: any</option>
              <option v-for="(label, key) in VISIBILITY_LABELS" :key="key" :value="key">{{ label }}</option>
            </select>
            <span class="ml-auto text-meta whitespace-nowrap text-fg-2 tabular-nums" aria-live="polite">{{ formatInteger(rows.length) }} of {{ formatInteger(runs.length) }}</span>
          </div>
          <div v-if="chips.length" class="mt-2 flex flex-wrap items-center gap-1.5">
            <button
              v-for="chip in chips"
              :key="chip.key"
              type="button"
              class="inline-flex h-6 max-w-full items-center gap-1 rounded-[3px] border border-rule-strong px-2 text-meta text-fg hover:bg-raised max-md:h-8"
              :aria-label="`Remove filter ${chip.label}`"
              @click="removeChip(chip.key)"
            >
              <span class="truncate">{{ chip.label }}</span> <XMarkIcon class="size-3.5 shrink-0 text-fg-2" aria-hidden="true" />
            </button>
            <button type="button" class="ui-btn ui-btn-sm ui-btn-quiet" @click="clearFilters">Clear all</button>
          </div>
        </div>

        <ResourceState v-if="stale" state="stale" class="p-3" title="Showing the last loaded media" :message="`Refresh failed: ${stale.error.value}`">
          <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="refresh">Retry</button></template>
        </ResourceState>

        <!-- Desktop / tablet table -->
        <div class="hidden md:block" role="region" aria-label="Media library table">
          <table class="w-full table-fixed border-collapse text-left">
            <thead>
              <tr class="h-9 border-b border-rule-strong bg-raised text-meta font-medium text-fg-2">
                <th class="w-52 px-3 font-medium">Replay</th>
                <th class="px-3 font-medium">Source run</th>
                <th class="w-32 px-3 font-medium max-lg:hidden">Public site</th>
                <th class="w-28 px-3 font-medium" :class="state.run ? 'hidden min-[1680px]:table-cell' : 'max-xl:hidden'">When</th>
                <th class="w-11 pr-2"><span class="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody v-if="resourceState === 'loading'" aria-busy="true">
              <tr v-for="n in 8" :key="n" class="h-14 border-b border-rule">
                <td colspan="5" class="px-3"><div class="h-4 bg-raised motion-safe:animate-pulse" :class="n % 2 ? 'w-3/4' : 'w-1/2'" /></td>
              </tr>
            </tbody>
            <tbody v-else-if="loaded">
              <tr
                v-for="run in rows"
                :key="run.run_id"
                :data-media-row="run.run_id"
                tabindex="0"
                :aria-selected="run.run_id === state.run"
                :class="run.run_id === state.run ? 'bg-selected shadow-[inset_2px_0_0_var(--color-accent)]' : 'hover:bg-raised'"
                class="group h-14 cursor-pointer border-b border-rule align-middle focus-visible:outline-offset-[-2px]"
                @click="openRun(run.run_id)"
                @keydown.enter.self="openRun(run.run_id)"
              >
                <td class="px-3">
                  <StatusPill :status="status(run)" class="max-w-full overflow-hidden" />
                  <div class="mt-0.5 truncate text-meta text-fg-2">{{ isLiveRun(run) ? 'Live run · replay after it ends' : 'Full replay' }}</div>
                </td>
                <td class="px-3">
                  <div class="flex min-w-0 items-center gap-1">
                    <span class="truncate font-mono" :title="run.run_id">{{ middleTruncate(run.run_id) }}</span>
                    <button
                      type="button"
                      class="ui-btn ui-btn-sm ui-icon-btn size-6 shrink-0 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 focus-visible:opacity-100"
                      :aria-label="`Copy run ID ${run.run_id}`"
                      @click.stop="copy(run.run_id, 'Run ID')"
                    >
                      <ClipboardDocumentIcon class="size-4" aria-hidden="true" />
                    </button>
                  </div>
                  <div class="truncate text-meta text-fg-2" :title="run.goal || ''">{{ run.game ? gameTitle(run.game) : 'Game' }} · {{ run.goal || 'Free play' }}</div>
                </td>
                <td class="px-3 max-lg:hidden">
                  <span class="ui-tag">{{ isPublic(run) ? 'Public' : 'Private' }}</span>
                  <div v-if="featuredRunID === run.run_id" class="mt-0.5 text-meta text-ok"><span aria-hidden="true">★</span> Featured</div>
                </td>
                <td class="px-3 whitespace-nowrap" :class="state.run ? 'hidden min-[1680px]:table-cell' : 'max-xl:hidden'">{{ timeAgo(Number(isLiveRun(run) ? run.queued_at : run.ended_at) || 0) }}</td>
                <td class="pr-2 text-right" @click.stop @keydown.stop>
                  <ActionMenu :label="`More actions for ${run.run_id}`" :items="menu(run)" />
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Narrow list (< 768px) -->
        <div class="md:hidden">
          <div v-if="resourceState === 'loading'" class="space-y-4 p-3" aria-busy="true">
            <div v-for="n in 6" :key="n" class="h-12 bg-raised motion-safe:animate-pulse" />
          </div>
          <ul v-else-if="loaded" role="list" aria-label="Media library">
            <li
              v-for="run in rows"
              :key="run.run_id"
              :class="run.run_id === state.run ? 'bg-selected shadow-[inset_2px_0_0_var(--color-accent)]' : ''"
              class="flex gap-3 border-b border-rule px-3 py-2.5"
            >
              <div class="min-w-0 flex-1">
                <div class="flex min-w-0 items-center gap-2">
                  <StatusPill :status="status(run)" class="min-w-0 shrink overflow-hidden" />
                  <span class="ml-auto shrink-0 text-meta text-fg-2">{{ isPublic(run) ? 'Public' : 'Private' }}</span>
                </div>
                <button type="button" class="mt-1 block w-full truncate text-left font-mono text-accent" :title="run.run_id" @click="openRun(run.run_id)">{{ middleTruncate(run.run_id) }}</button>
                <div class="mt-1 truncate text-meta text-fg-2">{{ run.game ? gameTitle(run.game) : 'Game' }} · {{ run.goal || 'Free play' }}</div>
              </div>
              <ActionMenu class="shrink-0" :label="`More actions for ${run.run_id}`" :items="menu(run)" />
            </li>
          </ul>
        </div>

        <ResourceState v-if="resourceState === 'error'" state="error" class="m-3" title="Couldn’t load the media library" :message="`${active.error.value || recent.error.value}. Retry when the wall is reachable.`">
          <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="refresh">Retry</button></template>
        </ResourceState>
        <div v-else-if="resourceState === 'empty' || (loaded && !rows.length)" class="flex flex-col items-center gap-2 px-4 py-12 text-center">
          <span class="text-[20px] leading-5 text-fg-3" aria-hidden="true">○</span>
          <template v-if="resourceState === 'empty'">
            <p class="text-title font-semibold">No media yet</p>
            <p class="text-fg-2">Runs and their replays appear here as soon as a run is queued.</p>
          </template>
          <template v-else>
            <p class="text-title font-semibold">No media matches</p>
            <p class="text-fg-2">{{ hasFilters ? 'Remove a filter or clear them all.' : 'Nothing in this view right now.' }}</p>
            <button v-if="hasFilters" type="button" class="ui-btn mt-1" @click="clearFilters">Clear filters</button>
          </template>
        </div>

        <footer class="flex flex-wrap items-center justify-between gap-2 px-3 py-2 text-meta text-fg-2">
          <span>Live runs and the {{ RECENT_LIMIT }} most recent finished runs</span>
          <a :href="operatorHref('runs', { run: '', job: '' }, search())" class="text-accent hover:underline">Browse all source runs in Runs</a>
        </footer>
      </div>

      <!-- Detail pane: docked ≥ 1280px, overlay sheet below, full screen < 768px -->
      <template v-if="state.run">
        <div class="fixed inset-0 z-40 bg-black/50 xl:hidden" aria-hidden="true" @click="closeRun" />
        <aside
          ref="paneEl"
          tabindex="-1"
          aria-label="Media details"
          class="fixed inset-0 z-50 border-rule-strong shadow-[0_8px_24px_rgb(0_0_0/45%)] outline-none md:left-auto md:w-[440px] md:border-l xl:sticky xl:top-14 xl:z-auto xl:h-[calc(100vh-168px)] xl:self-start xl:w-auto xl:border xl:shadow-none"
        >
          <MediaOutputPane
            :run-id="state.run"
            :run="selected"
            :loading="resourceState === 'loading' || (!selected && !focusedError)"
            :recording="selected ? recording(selected) : null"
            :visible="selected ? isPublic(selected) : false"
            :featured="featuredRunID === state.run"
            :busy="Boolean(busyRunID)"
            :jobs-href="selected && jobByRun.get(state.run) ? formatMediaHref({ ...state, tab: 'jobs', job: jobByRun.get(state.run)!.id }, search()) : ''"
            :studio-href="formatMediaHref({ ...state, tab: 'studio' }, search())"
            :source-href="selected ? sourceHref(selected) : ''"
            @close="closeRun"
            @copy="copy"
            @visible="(value) => selected && setControl(selected, { visible: value })"
            @feature="(value) => selected && setControl(selected, { featured: value })"
            @open-public="selected && openPublic(selected)"
          />
        </aside>
      </template>
    </div>
  </div>
</template>
