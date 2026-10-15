<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { Dialog, DialogPanel, DialogTitle } from '@headlessui/vue'
import { ArrowPathIcon, ClipboardDocumentIcon, XMarkIcon } from '@heroicons/vue/20/solid'
import { deleteRun, getDashboard, getMediaRenderJobs, getRun, replayVideoURL } from '../shared/api/client'
import type { DashboardRun, MediaRenderJob } from '../shared/api/types'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { presentOutcome } from '../shared/status'
import { operatorHref } from '../shared/urls'
import { gameTitle } from './operations'
import RunActionMenu from './RunActionMenu.vue'
import RunCleanupPanel from './RunCleanupPanel.vue'
import RunDetailPane from './RunDetailPane.vue'
import {
  ARCHIVE_FACETS,
  ARCHIVE_SORTS,
  ARCHIVE_VIEWS,
  activeView,
  archiveDashboardQuery,
  emptyFacets,
  formatArchiveSearch,
  parseArchiveState,
  viewState,
  type ArchiveFacet,
  type ArchiveSort,
  type ArchiveState
} from './runArchiveQuery'
import { DELETE_CONCURRENCY, cleanupProgressText, cleanupResultText, deleteRuns } from './runCleanup'
import { recordingMediaHref, recordingSummary, type RecordingSummary } from './runRecording'
import {
  archiveAgo,
  archiveHow,
  archiveWhen,
  archiveWhere,
  deploymentName,
  formatDuration,
  formatInteger,
  middleTruncate,
  modelName,
  plannerLabel,
  progressLabel,
  safeIssueURL
} from './runs'

const PAGE_SIZE = 25
const props = defineProps<{ runId?: string }>()

// ---- URL state (filters, sort, page, inspected run) -------------------------
const search = ref(window.location.search)
const state = computed(() => parseArchiveState(search.value))
const archiveKey = computed(() => formatArchiveSearch(state.value))
const inspectedID = computed(() => props.runId || '')

function syncSearch(): void { search.value = window.location.search }

function navigate(next: ArchiveState, options: { run?: string; replace?: boolean } = {}): void {
  const params = new URLSearchParams(formatArchiveSearch(next, window.location.search))
  if (options.run !== undefined) {
    if (options.run) params.set('run', options.run)
    else params.delete('run')
    params.delete('job')
    params.delete('output')
  }
  const query = params.toString()
  const url = (query ? '?' + query : window.location.pathname) + '#runs'
  if (options.replace) window.history.replaceState(null, '', url)
  else window.history.pushState(null, '', url)
  // App reads ?run= on popstate; this view re-reads its own keys from the same event.
  window.dispatchEvent(new PopStateEvent('popstate'))
}

function goTo(href: string): void {
  window.history.pushState(null, '', href)
  window.dispatchEvent(new PopStateEvent('popstate'))
}

function patch(change: Partial<ArchiveState>, replace = false): void {
  navigate({ ...state.value, page: 1, ...change }, { replace })
}

function setFacet(key: ArchiveFacet, value: string): void {
  patch({ facets: { ...state.value.facets, [key]: value } })
}

function clearFilters(): void {
  searchInput.value = ''
  patch({ q: '', facets: emptyFacets(), success: false })
}

// ---- Data ---------------------------------------------------------------------
const resource = usePollingResource(
  (signal) => getDashboard(archiveDashboardQuery(state.value, PAGE_SIZE), signal),
  { intervalMs: 30000, isEmpty: (snapshot) => snapshot.runs.length === 0 }
)
watch(archiveKey, () => { void resource.retry() })

const mediaResource = usePollingResource(
  (signal) => getMediaRenderJobs(250, signal),
  { intervalMs: 5000 }
)

const rows = computed(() => resource.data.value?.runs ?? [])
const total = computed(() => Number(resource.data.value?.total || 0))
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const rangeStart = computed(() => total.value ? (state.value.page - 1) * PAGE_SIZE + 1 : 0)
const rangeEnd = computed(() => Math.min(total.value, state.value.page * PAGE_SIZE))
const currentView = computed(() => activeView(state.value))
const showStateBody = computed(() => resource.state.value === 'error' || resource.state.value === 'empty')
const hasFilters = computed(() => Boolean(state.value.q || state.value.success || ARCHIVE_FACETS.some((key) => state.value.facets[key])))

watch(total, (value) => {
  if (resource.state.value === 'loading') return
  const last = Math.max(1, Math.ceil(value / PAGE_SIZE))
  if (state.value.page > last) navigate({ ...state.value, page: last }, { replace: true })
})

const FACET_LABELS: Record<ArchiveFacet, string> = {
  game: 'Game', outcome: 'Outcome', model: 'Model', goal: 'Goal', deployment: 'Deployment',
  play_style: 'Play style', how: 'Mode', starter: 'Starter', compute: 'Compute', experiment: 'Experiment'
}
const FACET_SOURCES: Record<ArchiveFacet, string> = {
  game: 'games', outcome: 'outcomes', model: 'models', goal: 'goals', deployment: 'deployments',
  play_style: 'play_styles', how: 'hows', starter: 'starters', compute: 'computes', experiment: 'experiments'
}
const VISIBLE_FACETS: ArchiveFacet[] = ['game', 'outcome', 'model', 'goal']
const MORE_FACETS = ARCHIVE_FACETS.filter((key) => !VISIBLE_FACETS.includes(key))
const SORT_LABELS: Record<ArchiveSort, string> = {
  finished: 'Finished', progress: 'Progress', runtime: 'Runtime', frames: 'Frames',
  rounds: 'Rounds', planner_time: 'Planner time', strategic_calls: 'Strategic calls'
}

function facetOptions(key: ArchiveFacet): string[] {
  const raw = resource.data.value?.history_facets as Record<string, string[] | undefined> | undefined
  const values = raw?.[FACET_SOURCES[key]] ?? []
  const current = state.value.facets[key]
  return current && !values.includes(current) ? [current, ...values] : values
}
function facetValueLabel(key: ArchiveFacet, value: string): string {
  return key === 'game' ? gameTitle(value) : value
}
const hiddenActive = computed(() => MORE_FACETS.filter((key) => state.value.facets[key]).length)
const chips = computed(() => [
  ...(state.value.q ? [{ key: 'q', label: `Run ID contains: ${state.value.q}` }] : []),
  ...(state.value.success ? [{ key: 'success', label: 'Successful only' }] : []),
  ...ARCHIVE_FACETS.filter((key) => state.value.facets[key])
    .map((key) => ({ key, label: `${FACET_LABELS[key]}: ${facetValueLabel(key, state.value.facets[key])}` }))
])
function removeChip(key: string): void {
  if (key === 'q') { searchInput.value = ''; patch({ q: '' }) }
  else if (key === 'success') patch({ success: false })
  else setFacet(key as ArchiveFacet, '')
}

// ---- Sorting ------------------------------------------------------------------
function sortBy(key: ArchiveSort): void {
  if (state.value.sort === key) patch({ dir: state.value.dir === 'asc' ? 'desc' : 'asc' })
  else patch({ sort: key, dir: key === 'finished' || key === 'progress' ? 'desc' : 'asc' })
}
function ariaSort(key: ArchiveSort): 'ascending' | 'descending' | 'none' {
  if (state.value.sort !== key) return 'none'
  return state.value.dir === 'asc' ? 'ascending' : 'descending'
}
function sortGlyph(key: ArchiveSort): string {
  if (state.value.sort !== key) return ''
  return state.value.dir === 'asc' ? '▲' : '▼'
}

// ---- Search / go to run ID ----------------------------------------------------
const searchInput = ref(state.value.q)
const searchEl = ref<HTMLInputElement | null>(null)
const gotoError = ref('')
let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(() => state.value.q, (value) => { if (value !== searchInput.value.trim()) searchInput.value = value })
watch(searchInput, (value) => {
  gotoError.value = ''
  if (searchTimer !== null) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    const q = value.trim()
    if (q !== state.value.q) patch({ q }, true)
  }, 300)
})

async function goToRun(): Promise<void> {
  const id = searchInput.value.trim()
  if (!id) return
  gotoError.value = ''
  try {
    const run = await getRun(id)
    openRun(run.run_id)
  } catch {
    gotoError.value = 'No run with that ID'
  }
}

// ---- Inspected run (detail pane) ----------------------------------------------
const paneEl = ref<HTMLElement | null>(null)
const inspectedRow = computed(() => rows.value.find((run) => run.run_id === inspectedID.value) || null)

function openRun(runID: string, focusPane = true): void {
  if (runID !== inspectedID.value) navigate(state.value, { run: runID, replace: Boolean(inspectedID.value) })
  if (focusPane) void nextTick(() => paneEl.value?.focus())
}

function closeRun(): void {
  const id = inspectedID.value
  navigate(state.value, { run: '' })
  void nextTick(() => rowEl(id)?.focus())
}

function rowEl(runID: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-run-row="${CSS.escape(runID)}"]`)
}

function moveSelection(step: number): void {
  const index = rows.value.findIndex((run) => run.run_id === inspectedID.value)
  const next = rows.value[index + step]
  if (!next) return
  openRun(next.run_id, false)
  void nextTick(() => rowEl(next.run_id)?.focus())
}

const dialogOpen = computed(() => Boolean(deleteTarget.value) || bulkDeleteOpen.value || cleanupOpen.value)

function onKeydown(event: KeyboardEvent): void {
  if (dialogOpen.value || event.metaKey || event.ctrlKey || event.altKey) return
  const target = event.target as HTMLElement | null
  const typing = Boolean(target?.closest('input, select, textarea, [role="menu"]'))
  if (event.key === '/' && !typing) {
    event.preventDefault()
    searchEl.value?.focus()
    return
  }
  if (!inspectedID.value || typing) return
  if (event.key === 'Escape') {
    event.preventDefault()
    closeRun()
  } else if ((event.key === 'ArrowDown' || event.key === 'ArrowUp') && !paneEl.value?.contains(target)) {
    event.preventDefault()
    moveSelection(event.key === 'ArrowDown' ? 1 : -1)
  }
}

onMounted(() => {
  window.addEventListener('popstate', syncSearch)
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => {
  window.removeEventListener('popstate', syncSearch)
  window.removeEventListener('keydown', onKeydown)
  if (searchTimer !== null) clearTimeout(searchTimer)
})

// ---- Replay / media ---------------------------------------------------------------
const mediaJobByRun = computed(() => {
  const result = new Map<string, MediaRenderJob>()
  for (const job of mediaResource.data.value?.jobs || []) {
    const current = result.get(job.run_id)
    if (!current || Number(job.updated_at_unix_ms || 0) > Number(current.updated_at_unix_ms || 0)) {
      result.set(job.run_id, job)
    }
  }
  return result
})
const mediaState = computed(() => {
  const value = mediaResource.state.value
  return value === 'loading' || value === 'stale' || value === 'error' ? value : 'ready'
})
function recording(run: DashboardRun): RecordingSummary {
  return recordingSummary(run, mediaJobByRun.value.get(run.run_id), mediaState.value)
}
function mediaHref(run: DashboardRun): string {
  return recordingMediaHref(run.run_id, recording(run), window.location.search)
}
const RECORDING_TONE: Record<RecordingSummary['tone'], string> = {
  success: 'text-ok', info: 'text-accent', warning: 'text-warn', danger: 'text-danger', neutral: 'text-fg-3'
}
// Table cells use short labels; the full label and detail stay in the title and the pane.
const RECORDING_SHORT: Partial<Record<RecordingSummary['state'], string>> = {
  queued: 'Queued', syncing: 'Syncing', failed: 'Render failed', cancelled: 'Cancelled', unavailable: 'No replay', unknown: 'Unknown'
}
const RECORDING_GLYPH: Record<RecordingSummary['tone'], string> = {
  success: '✓', info: '◐', warning: '▲', danger: '✕', neutral: '○'
}

// ---- Receipts (one polite live region, mirrored as a toast) -----------------------
const toast = ref('')
let toastTimer = 0
function announce(text: string): void {
  toast.value = text
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => { toast.value = '' }, 4000)
}
async function copyRunID(runID: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(runID)
    announce('Run ID copied')
  } catch {
    announce('Couldn’t copy the run ID')
  }
}
function inspectInLive(runID: string): void {
  goTo(operatorHref('live', { run: runID, job: '', output: '' }))
}

// ---- Selection and deletes -----------------------------------------------------------
const selectedRunIDs = ref<Set<string>>(new Set())
const selectMode = ref(false)
const selectedCount = computed(() => selectedRunIDs.value.size)
const allVisibleSelected = computed(() => rows.value.length > 0 && rows.value.every((run) => selectedRunIDs.value.has(run.run_id)))

function toggleRunSelection(runID: string): void {
  const next = new Set(selectedRunIDs.value)
  if (next.has(runID)) next.delete(runID)
  else next.add(runID)
  selectedRunIDs.value = next
}
function toggleVisibleSelection(): void {
  const next = new Set(selectedRunIDs.value)
  if (allVisibleSelected.value) for (const run of rows.value) next.delete(run.run_id)
  else for (const run of rows.value) next.add(run.run_id)
  selectedRunIDs.value = next
}
function clearSelection(): void {
  selectedRunIDs.value = new Set()
  selectMode.value = false
}

const deleteTarget = ref<DashboardRun | null>(null)
const deleting = ref(false)
const deleteError = ref('')
function requestDelete(run: DashboardRun): void {
  deleteError.value = ''
  deleteTarget.value = run
}
function closeDelete(): void {
  if (!deleting.value) deleteTarget.value = null
}
async function confirmDelete(): Promise<void> {
  const run = deleteTarget.value
  if (!run || deleting.value) return
  deleting.value = true
  deleteError.value = ''
  try {
    await deleteRun(run.run_id)
    const next = new Set(selectedRunIDs.value)
    next.delete(run.run_id)
    selectedRunIDs.value = next
    deleteTarget.value = null
    if (inspectedID.value === run.run_id) navigate(state.value, { run: '', replace: true })
    announce(`Deleted ${run.run_id}`)
    await resource.retry()
  } catch (cause) {
    // The dialog stays open with the reason so the operator can retry or cancel.
    deleteError.value = cause instanceof Error ? cause.message : 'Delete failed'
  } finally {
    deleting.value = false
  }
}

const bulkDeleteOpen = ref(false)
const deletingSelected = ref(false)
const bulkProgress = ref('')
const bulkIDs = computed(() => [...selectedRunIDs.value])
const bulkMessage = computed(() => {
  const ids = bulkIDs.value
  const listed = ids.slice(0, 5).join('\n')
  const more = ids.length > 5 ? `\nand ${ids.length - 5} more` : ''
  const body = `Their stored run data, recordings and replay videos are removed. This can't be undone.\n\n${listed}${more}`
  return deletingSelected.value && bulkProgress.value ? `${bulkProgress.value}\n\n${body}` : body
})
function plural(count: number): string { return `${count} run${count === 1 ? '' : 's'}` }

async function confirmDeleteSelected(): Promise<void> {
  const ids = bulkIDs.value
  if (!ids.length || deletingSelected.value) return
  deletingSelected.value = true
  bulkProgress.value = 'Preparing selected runs…'
  try {
    const result = await deleteRuns(
      ids,
      (id) => deleteRun(id),
      DELETE_CONCURRENCY,
      (progress) => { bulkProgress.value = cleanupProgressText(progress) }
    )
    const next = new Set(selectedRunIDs.value)
    for (const id of result.deletedIds) next.delete(id)
    selectedRunIDs.value = next
    if (result.deletedIds.includes(inspectedID.value)) navigate(state.value, { run: '', replace: true })
    announce(cleanupResultText(result))
    bulkDeleteOpen.value = false
    await resource.retry()
  } catch (cause) {
    announce(`Delete failed: ${cause instanceof Error ? cause.message : String(cause)}`)
  } finally {
    deletingSelected.value = false
    bulkProgress.value = ''
  }
}

const cleanupOpen = ref(false)
</script>

<template>
  <div class="space-y-3">
    <!-- Page header -->
    <header class="flex min-h-12 flex-wrap items-center gap-x-3 gap-y-2">
      <h1 class="text-page font-semibold">Runs</h1>
      <span v-if="resource.data.value" class="text-fg-2">{{ formatInteger(total) }} {{ hasFilters ? 'matching' : 'finished' }}</span>
      <span v-if="resource.state.value === 'refreshing'" class="inline-flex items-center gap-1.5 text-meta text-fg-2" role="status">
        <span class="size-1.5 bg-accent motion-safe:animate-pulse" aria-hidden="true" /> Refreshing
      </span>
      <div class="ml-auto flex flex-wrap items-center gap-2">
        <a :href="operatorHref('analytics', { section: 'overview' }, '')" class="ui-btn ui-btn-quiet max-md:hidden">View leaderboards</a>
        <button type="button" class="ui-btn md:hidden" :aria-pressed="selectMode" @click="selectMode ? clearSelection() : (selectMode = true)">{{ selectMode ? 'Done' : 'Select' }}</button>
        <button type="button" class="ui-btn" @click="cleanupOpen = true">Bulk cleanup…</button>
        <button type="button" class="ui-btn ui-icon-btn" aria-label="Refresh runs" title="Refresh" @click="resource.retry()">
          <ArrowPathIcon class="size-4" aria-hidden="true" />
        </button>
      </div>
    </header>

    <!-- Views -->
    <div class="-mx-3 overflow-x-auto px-3 md:mx-0 md:px-0">
      <div class="inline-flex border border-rule-strong" role="group" aria-label="Saved views">
        <button
          v-for="view in ARCHIVE_VIEWS"
          :key="view.id"
          type="button"
          :aria-pressed="currentView === view.id"
          :class="currentView === view.id ? 'bg-selected text-fg' : 'text-fg-2 hover:bg-raised hover:text-fg'"
          class="h-7 border-r border-rule-strong px-3 whitespace-nowrap last:border-r-0 max-md:h-10"
          @click="searchInput = ''; navigate(viewState(view))"
        >
          {{ view.label }}
        </button>
      </div>
    </div>

    <div :class="inspectedID ? 'xl:grid xl:grid-cols-[minmax(0,1fr)_400px] xl:gap-3 min-[1440px]:grid-cols-[minmax(0,1fr)_440px]' : ''">
      <div class="min-w-0 border border-rule-strong bg-surface">
        <!-- FilterBar, or the bulk bar in its place while rows are selected -->
        <div v-if="selectedCount" class="flex min-h-12 flex-wrap items-center gap-3 border-b border-rule bg-raised px-3 py-2" role="region" aria-label="Bulk actions">
          <span class="font-semibold">{{ selectedCount }} selected</span>
          <button type="button" class="ui-btn ui-btn-sm ui-btn-danger max-md:h-10" aria-label="Delete selected runs" @click="bulkDeleteOpen = true">Delete…</button>
          <button type="button" class="ui-btn ui-btn-sm ui-btn-quiet max-md:h-10" @click="clearSelection">Clear selection</button>
        </div>
        <div v-else class="border-b border-rule px-3 py-2">
          <div class="flex flex-wrap items-center gap-2">
            <div class="relative w-full md:w-60">
              <label for="runs-search" class="sr-only">Search run ID</label>
              <input
                id="runs-search"
                ref="searchEl"
                v-model="searchInput"
                type="search"
                autocomplete="off"
                spellcheck="false"
                placeholder="Search run ID…  /"
                :aria-invalid="Boolean(gotoError)"
                aria-describedby="runs-search-hint"
                class="block h-7 w-full rounded-[3px] border border-control bg-canvas px-2.5 font-mono text-fg placeholder:font-sans placeholder:text-fg-3 max-md:h-10"
                @keydown.enter.prevent="goToRun"
              >
              <span id="runs-search-hint" class="sr-only">Filters as you type. Press Enter to open an exact run ID.</span>
            </div>
            <select
              v-for="key in VISIBLE_FACETS"
              :key="key"
              :value="state.facets[key]"
              :aria-label="FACET_LABELS[key]"
              class="h-7 max-w-44 min-w-0 rounded-[3px] border border-control bg-canvas px-2 text-fg max-md:h-10 max-md:max-w-none max-md:flex-1 max-md:basis-[calc(50%-4px)]"
              @change="setFacet(key, ($event.target as HTMLSelectElement).value)"
            >
              <option value="">{{ FACET_LABELS[key] }}: all</option>
              <option v-for="value in facetOptions(key)" :key="value" :value="value">{{ facetValueLabel(key, value) }}</option>
            </select>
            <details class="group relative">
              <summary class="ui-btn ui-btn-sm list-none max-md:h-10 [&::-webkit-details-marker]:hidden">
                More filters<template v-if="hiddenActive"> ({{ hiddenActive }})</template>
                <span class="text-fg-3 group-open:rotate-180" aria-hidden="true">▾</span>
              </summary>
              <div class="absolute left-0 z-30 mt-1 grid w-72 gap-2 rounded-md border border-rule-strong bg-raised p-3 shadow-[0_8px_24px_rgb(0_0_0/45%)]">
                <label v-for="key in MORE_FACETS" :key="key" class="grid gap-1">
                  <span class="ui-label">{{ FACET_LABELS[key] }}</span>
                  <select
                    :value="state.facets[key]"
                    class="h-8 rounded-[3px] border border-control bg-canvas px-2 text-fg"
                    @change="setFacet(key, ($event.target as HTMLSelectElement).value)"
                  >
                    <option value="">All</option>
                    <option v-for="value in facetOptions(key)" :key="value" :value="value">{{ value }}</option>
                  </select>
                </label>
              </div>
            </details>
            <div class="ml-auto flex items-center gap-2">
              <label class="flex items-center gap-1.5">
                <span class="ui-label">Sort by</span>
                <select
                  :value="state.sort"
                  class="h-7 rounded-[3px] border border-control bg-canvas px-2 text-fg max-md:h-10"
                  @change="sortBy(($event.target as HTMLSelectElement).value as ArchiveSort)"
                >
                  <option v-for="key in ARCHIVE_SORTS" :key="key" :value="key">{{ SORT_LABELS[key] }}</option>
                </select>
              </label>
              <button
                type="button"
                class="ui-btn ui-btn-sm ui-icon-btn max-md:size-10"
                :aria-label="state.dir === 'asc' ? 'Sorted ascending; switch to descending' : 'Sorted descending; switch to ascending'"
                @click="patch({ dir: state.dir === 'asc' ? 'desc' : 'asc' })"
              >{{ state.dir === 'asc' ? '▲' : '▼' }}</button>
              <span class="text-meta whitespace-nowrap text-fg-2 tabular-nums" aria-live="polite">{{ rangeStart }}–{{ rangeEnd }} of {{ formatInteger(total) }}</span>
            </div>
          </div>
          <p v-if="gotoError" class="mt-1 text-meta text-danger" role="alert">✕ {{ gotoError }}</p>
          <div v-if="chips.length" class="mt-2 flex flex-wrap items-center gap-1.5">
            <button
              v-for="chip in chips"
              :key="chip.key"
              type="button"
              class="inline-flex h-6 items-center gap-1 rounded-[3px] border border-rule-strong px-2 text-meta text-fg hover:bg-raised max-md:h-8"
              :aria-label="`Remove filter ${chip.label}`"
              @click="removeChip(chip.key)"
            >
              {{ chip.label }} <XMarkIcon class="size-3.5 text-fg-2" aria-hidden="true" />
            </button>
            <button type="button" class="ui-btn ui-btn-sm ui-btn-quiet" @click="clearFilters">Clear all</button>
          </div>
        </div>

        <ResourceState
          v-if="resource.state.value === 'stale'"
          state="stale"
          class="p-3"
          title="Showing the last loaded runs"
          :message="`Refresh failed: ${resource.error.value}`"
        >
          <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="resource.retry()">Retry</button></template>
        </ResourceState>

        <!-- Desktop / tablet table -->
        <div class="hidden md:block" role="region" aria-label="Finished run history table">
          <table class="w-full table-fixed border-collapse text-left">
            <thead>
              <tr class="h-9 border-b border-rule-strong bg-raised text-meta font-medium text-fg-2">
                <th class="w-10 pl-3">
                  <input
                    type="checkbox"
                    :checked="allVisibleSelected"
                    :aria-label="allVisibleSelected ? 'Deselect all runs on this page' : 'Select all runs on this page'"
                    class="size-4 accent-[var(--color-accent)]"
                    @change="toggleVisibleSelection"
                  >
                </th>
                <th class="w-36 px-3 font-medium">Outcome</th>
                <th class="w-52 px-3 font-medium">Run</th>
                <th class="px-3 font-medium" :aria-sort="ariaSort('progress')">
                  <button type="button" class="inline-flex items-center gap-1 hover:text-fg" @click="sortBy('progress')">Goal / progress <span aria-hidden="true">{{ sortGlyph('progress') }}</span></button>
                </th>
                <th class="w-40 px-3 font-medium max-xl:hidden" :class="inspectedID ? 'hidden min-[1680px]:table-cell' : ''">Model</th>
                <th class="w-28 px-3 font-medium" :aria-sort="ariaSort('finished')">
                  <button type="button" class="inline-flex items-center gap-1 hover:text-fg" @click="sortBy('finished')">Finished <span aria-hidden="true">{{ sortGlyph('finished') }}</span></button>
                </th>
                <th class="w-28 px-3 text-right font-medium max-lg:hidden" :aria-sort="ariaSort('runtime')">
                  <button type="button" class="inline-flex items-center gap-1 hover:text-fg" @click="sortBy('runtime')">Runtime <span aria-hidden="true">{{ sortGlyph('runtime') }}</span></button>
                </th>
                <th class="w-32 px-3 font-medium">Replay</th>
                <th class="w-11 pr-2"><span class="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody v-if="resource.state.value === 'loading'" aria-busy="true">
              <tr v-for="n in 8" :key="n" class="h-14 border-b border-rule">
                <td colspan="9" class="px-3"><div class="h-4 bg-raised motion-safe:animate-pulse" :class="n % 2 ? 'w-3/4' : 'w-1/2'" /></td>
              </tr>
            </tbody>
            <tbody v-else-if="!showStateBody">
              <tr
                v-for="run in rows"
                :key="run.run_id"
                :data-run-row="run.run_id"
                tabindex="0"
                :aria-selected="run.run_id === inspectedID"
                :class="run.run_id === inspectedID ? 'bg-selected shadow-[inset_2px_0_0_var(--color-accent)]' : 'hover:bg-raised'"
                class="group h-14 cursor-pointer border-b border-rule align-middle focus-visible:outline-offset-[-2px]"
                @click="openRun(run.run_id)"
                @keydown.enter.self="openRun(run.run_id)"
              >
                <td class="pl-3" @click.stop>
                  <input
                    type="checkbox"
                    :checked="selectedRunIDs.has(run.run_id)"
                    :aria-label="`Select ${run.run_id}`"
                    class="size-4 accent-[var(--color-accent)]"
                    @change="toggleRunSelection(run.run_id)"
                  >
                </td>
                <td class="px-3">
                  <StatusPill :status="presentOutcome(run)" class="max-w-full overflow-hidden" />
                  <a v-if="run.issue?.issue_number && safeIssueURL(run)" :href="safeIssueURL(run)" target="_blank" rel="noopener" class="mt-0.5 block text-meta text-accent hover:underline" @click.stop>#{{ run.issue.issue_number }} ↗</a>
                </td>
                <td class="px-3">
                  <div class="flex min-w-0 items-center gap-1">
                    <span class="truncate font-mono" :title="run.run_id">{{ middleTruncate(run.run_id) }}</span>
                    <button
                      type="button"
                      class="ui-btn ui-btn-sm ui-icon-btn size-6 shrink-0 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 focus-visible:opacity-100"
                      :aria-label="`Copy run ID ${run.run_id}`"
                      @click.stop="copyRunID(run.run_id)"
                    >
                      <ClipboardDocumentIcon class="size-4" aria-hidden="true" />
                    </button>
                  </div>
                  <div class="mt-0.5 flex min-w-0 gap-1" @click.stop>
                    <button type="button" class="ui-tag hover:border-control hover:text-fg" :aria-label="`Filter by ${gameTitle(run.game)}`" @click="setFacet('game', run.game || 'pokemon-red')">{{ gameTitle(run.game) }}</button>
                    <button type="button" class="ui-tag shrink-0 hover:border-control hover:text-fg" :aria-label="`Filter by ${plannerLabel(run)} runs`" @click="setFacet('how', archiveHow(run))">{{ plannerLabel(run) }}</button>
                  </div>
                </td>
                <td class="px-3">
                  <div class="truncate" :title="run.goal || ''">{{ run.goal || '—' }}</div>
                  <div class="truncate text-meta text-fg-2" :title="`${progressLabel(run)} · ${archiveWhere(run)}`">{{ progressLabel(run) }} · {{ archiveWhere(run) }}</div>
                </td>
                <td class="px-3 max-xl:hidden" :class="inspectedID ? 'hidden min-[1680px]:table-cell' : ''">
                  <div v-if="modelName(run)" class="truncate" :title="modelName(run)">{{ modelName(run) }}</div>
                  <div v-else class="text-fg-3">{{ run.planner === 'scripted' ? 'Scripted' : '—' }}</div>
                  <div v-if="deploymentName(run)" class="truncate font-mono text-meta text-fg-2" :title="deploymentName(run)">{{ deploymentName(run) }}</div>
                </td>
                <td class="px-3 whitespace-nowrap" :title="archiveWhen(run)">{{ archiveAgo(run) }}</td>
                <td class="px-3 text-right font-mono whitespace-nowrap max-lg:hidden">
                  <div>{{ formatDuration(Number(run.runtime_seconds || 0)) }}</div>
                  <div class="text-meta text-fg-2" title="Emulated frames">{{ formatInteger(Number(run.frame || 0)) }}</div>
                </td>
                <td class="px-3" @click.stop>
                  <a
                    v-if="recording(run).canWatch"
                    :href="replayVideoURL(run.run_id)"
                    target="_blank"
                    rel="noopener"
                    class="ui-btn ui-btn-sm ui-btn-quiet -ml-2.5 text-accent"
                    aria-label="Watch replay"
                    :title="recording(run).detail"
                  >▶ Watch</a>
                  <a
                    v-else
                    :href="mediaHref(run)"
                    :class="RECORDING_TONE[recording(run).tone]"
                    class="block truncate text-meta hover:underline"
                    :title="`${recording(run).label}. ${recording(run).detail}`"
                  ><span aria-hidden="true">{{ RECORDING_GLYPH[recording(run).tone] }}</span> {{ RECORDING_SHORT[recording(run).state] || recording(run).label }}</a>
                </td>
                <td class="pr-2 text-right" @click.stop @keydown.stop>
                  <RunActionMenu :run-id="run.run_id" :media-href="mediaHref(run)" @inspect="inspectInLive(run.run_id)" @copy="copyRunID(run.run_id)" @delete="requestDelete(run)" />
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Narrow list (< 768px): same facts, three lines, no cards -->
        <div class="md:hidden">
          <div v-if="resource.state.value === 'loading'" class="space-y-4 p-3" aria-busy="true">
            <div v-for="n in 6" :key="n" class="h-12 bg-raised motion-safe:animate-pulse" />
          </div>
          <ul v-else-if="!showStateBody" role="list" aria-label="Finished runs">
            <li
              v-for="run in rows"
              :key="run.run_id"
              :class="run.run_id === inspectedID ? 'bg-selected shadow-[inset_2px_0_0_var(--color-accent)]' : ''"
              class="flex gap-3 border-b border-rule px-3 py-2.5"
            >
              <input
                v-if="selectMode"
                type="checkbox"
                :checked="selectedRunIDs.has(run.run_id)"
                :aria-label="`Select ${run.run_id}`"
                class="mt-1 size-5 shrink-0 accent-[var(--color-accent)]"
                @change="toggleRunSelection(run.run_id)"
              >
              <div class="min-w-0 flex-1">
                <div class="flex min-w-0 items-center gap-2">
                  <StatusPill :status="presentOutcome(run)" class="shrink-0" />
                  <button type="button" class="min-w-0 truncate text-left font-mono text-accent" :title="run.run_id" @click="openRun(run.run_id)">
                    {{ middleTruncate(run.run_id, 10, 4) }}
                  </button>
                  <span class="ml-auto shrink-0 text-meta text-fg-2" :title="archiveWhen(run)">{{ archiveAgo(run) }}</span>
                </div>
                <div class="mt-1 truncate">{{ run.goal || '—' }} <span class="text-fg-2">· {{ progressLabel(run) }}</span></div>
                <div class="mt-1 flex min-w-0 items-center gap-2 text-meta text-fg-2">
                  <span class="truncate">{{ modelName(run) || plannerLabel(run) }}</span>
                  <span class="shrink-0 font-mono">· {{ formatDuration(Number(run.runtime_seconds || 0)) }}</span>
                  <a v-if="recording(run).canWatch" :href="replayVideoURL(run.run_id)" target="_blank" rel="noopener" class="ml-auto shrink-0 text-accent" aria-label="Watch replay">▶ Watch</a>
                  <a v-else :href="mediaHref(run)" :class="RECORDING_TONE[recording(run).tone]" class="ml-auto shrink-0 truncate" :title="recording(run).detail">{{ RECORDING_SHORT[recording(run).state] || recording(run).label }}</a>
                </div>
              </div>
              <RunActionMenu class="shrink-0" :run-id="run.run_id" :media-href="mediaHref(run)" @inspect="inspectInLive(run.run_id)" @copy="copyRunID(run.run_id)" @delete="requestDelete(run)" />
            </li>
          </ul>
        </div>

        <!-- Error / empty states sit inside the table frame, under the header -->
        <ResourceState
          v-if="resource.state.value === 'error'"
          state="error"
          class="m-3"
          title="Couldn’t load runs"
          :message="`${resource.error.value}. Filters still work; retry when the archive is reachable.`"
        >
          <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="resource.retry()">Retry</button></template>
        </ResourceState>
        <div v-else-if="resource.state.value === 'empty'" class="flex flex-col items-center gap-2 px-4 py-12 text-center">
          <span class="text-[20px] leading-5 text-fg-3" aria-hidden="true">○</span>
          <p class="text-title font-semibold">{{ hasFilters ? 'No runs match these filters' : 'No finished runs yet' }}</p>
          <p class="text-fg-2">{{ hasFilters ? 'Remove a filter or clear them all.' : 'Finished runs appear here with their outcome and evidence.' }}</p>
          <button v-if="hasFilters" type="button" class="ui-btn mt-1" @click="clearFilters">Clear filters</button>
          <a v-else href="#new-run" class="ui-btn mt-1">New run</a>
        </div>

        <footer class="flex flex-wrap items-center justify-between gap-2 px-3 py-2">
          <span class="text-meta text-fg-2">{{ rangeStart }}–{{ rangeEnd }} of {{ formatInteger(total) }}</span>
          <div class="flex items-center gap-2">
            <button type="button" class="ui-btn ui-btn-sm max-md:h-10" :disabled="state.page <= 1" @click="navigate({ ...state, page: state.page - 1 })">‹ Previous</button>
            <span class="min-w-14 text-center font-mono text-meta text-fg-2">{{ state.page }} / {{ pageCount }}</span>
            <button type="button" class="ui-btn ui-btn-sm max-md:h-10" :disabled="state.page >= pageCount" @click="navigate({ ...state, page: state.page + 1 })">Next ›</button>
          </div>
        </footer>
      </div>

      <!-- Detail pane: docked ≥ 1280px, overlay sheet below, full screen < 768px -->
      <template v-if="inspectedID">
        <div class="fixed inset-0 z-40 bg-black/50 xl:hidden" aria-hidden="true" @click="closeRun" />
        <aside
          ref="paneEl"
          tabindex="-1"
          aria-label="Run details"
          class="fixed inset-0 z-50 border-rule-strong shadow-[0_8px_24px_rgb(0_0_0/45%)] outline-none md:left-auto md:w-[440px] md:border-l xl:sticky xl:top-14 xl:z-auto xl:h-[calc(100vh-168px)] xl:self-start xl:w-auto xl:border xl:shadow-none"
        >
          <RunDetailPane
            :run-id="inspectedID"
            :row="inspectedRow"
            :media-job="mediaJobByRun.get(inspectedID)"
            :media-state="mediaState"
            @close="closeRun"
            @inspect="inspectInLive"
            @copy="copyRunID"
            @delete="requestDelete"
          />
        </aside>
      </template>
    </div>

    <!-- Bulk cleanup lives in a drawer so the irreversible tool isn't on the page by default -->
    <Dialog :open="cleanupOpen" class="operator-root relative z-[60] !min-h-0 !bg-transparent" @close="cleanupOpen = false">
      <div class="fixed inset-0 bg-black/60" aria-hidden="true" />
      <DialogPanel class="fixed inset-y-0 right-0 flex w-full max-w-[560px] flex-col border-l border-rule-strong bg-surface shadow-[0_8px_24px_rgb(0_0_0/45%)]">
        <div class="ui-bay-header min-h-12">
          <DialogTitle as="h2">Bulk cleanup</DialogTitle>
          <button type="button" class="ui-btn ui-btn-sm ui-icon-btn" aria-label="Close bulk cleanup" @click="cleanupOpen = false">
            <XMarkIcon class="size-4" aria-hidden="true" />
          </button>
        </div>
        <div class="min-h-0 flex-1 overflow-y-auto p-3">
          <RunCleanupPanel @changed="resource.retry()" />
        </div>
      </DialogPanel>
    </Dialog>

    <ConfirmDialog
      :open="Boolean(deleteTarget)"
      :title="deleteTarget ? `Delete run ${middleTruncate(deleteTarget.run_id)}?` : 'Delete run?'"
      :message="deleteTarget ? `${deleteTarget.run_id}\n\nIts stored run data, recordings and replay video are removed. This can't be undone.${deleteError ? `\n\n✕ Delete failed: ${deleteError}` : ''}` : ''"
      confirm-label="Delete run"
      :busy="deleting"
      danger
      @close="closeDelete"
      @confirm="confirmDelete"
    />

    <ConfirmDialog
      :open="bulkDeleteOpen"
      :title="`Delete ${plural(selectedCount)}?`"
      :message="bulkMessage"
      :confirm-label="`Delete ${plural(selectedCount)}`"
      :require-text="selectedCount > 10 ? String(selectedCount) : ''"
      :busy="deletingSelected"
      danger
      @close="deletingSelected ? undefined : (bulkDeleteOpen = false)"
      @confirm="confirmDeleteSelected"
    />

    <div class="pointer-events-none fixed right-4 bottom-4 z-[80]" aria-live="polite" role="status">
      <div v-if="toast" class="pointer-events-auto rounded-md border border-rule-strong bg-raised px-4 py-2.5 shadow-[0_8px_24px_rgb(0_0_0/45%)]">{{ toast }}</div>
    </div>
  </div>
</template>
