<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { ArrowPathIcon, ClipboardDocumentIcon, XMarkIcon } from '@heroicons/vue/20/solid'
import { deleteRun, dismissTriages, getDashboard, getTriage, investigateTriage } from '../shared/api/client'
import type { TriageGroup } from '../shared/api/types'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { presentFailureGroup } from '../shared/status'
import { operatorHref } from '../shared/urls'
import FailureActionMenu from './FailureActionMenu.vue'
import FailureDetailPane from './FailureDetailPane.vue'
import {
  FAILURES_PAGE_SIZE,
  FAILURE_VIEWS,
  canDismiss,
  exampleRuns,
  filterGroups,
  formatFailuresSearch,
  groupTitle,
  inView,
  issueURL,
  latestRun,
  parseFailuresState,
  type FailureSort,
  type FailuresState,
  type IssueFilter
} from './failuresQuery'
import {
  DELETE_CONCURRENCY,
  cleanupProgressText,
  cleanupResultText,
  deleteRuns,
  groupCleanupLabel,
  groupPattern,
  isResolvedGroup,
  matchingRunsForGroups
} from './runCleanup'
import { formatInteger, middleTruncate, timeAgo } from './runs'

// ---- URL state (view, filters, sort, page, inspected group) --------------------
const search = ref(window.location.search)
const state = computed(() => parseFailuresState(search.value))

function syncSearch(): void { search.value = window.location.search }

function navigate(next: FailuresState, replace = false): void {
  const url = (formatFailuresSearch(next, window.location.search) || window.location.pathname) + '#failures'
  if (replace) window.history.replaceState(null, '', url)
  else window.history.pushState(null, '', url)
  window.dispatchEvent(new PopStateEvent('popstate'))
}
function patch(change: Partial<FailuresState>, replace = false): void {
  navigate({ ...state.value, page: 1, ...change }, replace)
}
function clearFilters(): void {
  searchInput.value = ''
  patch({ issue: '', q: '' })
}

// ---- Data ------------------------------------------------------------------------
const resource = usePollingResource(
  (signal) => getTriage(signal),
  { intervalMs: 5000, isEmpty: (groups) => groups.length === 0 }
)
const groups = computed(() => resource.data.value ?? [])
const filtered = computed(() => filterGroups(groups.value, state.value))
const total = computed(() => filtered.value.length)
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / FAILURES_PAGE_SIZE)))
const rows = computed(() => filtered.value.slice((state.value.page - 1) * FAILURES_PAGE_SIZE, state.value.page * FAILURES_PAGE_SIZE))
const rangeStart = computed(() => total.value ? (state.value.page - 1) * FAILURES_PAGE_SIZE + 1 : 0)
const rangeEnd = computed(() => Math.min(total.value, state.value.page * FAILURES_PAGE_SIZE))
const viewCounts = computed(() => Object.fromEntries(FAILURE_VIEWS.map((view) => [view.id, groups.value.filter((group) => inView(group, view.id)).length])))
const dismissableGroups = computed(() => groups.value.filter((group) => !isResolvedGroup(group) && canDismiss(group)))
const hasFilters = computed(() => Boolean(state.value.q || state.value.issue))
const loaded = computed(() => resource.state.value !== 'loading' && resource.state.value !== 'error' && resource.state.value !== 'empty')

watch(pageCount, (last) => {
  if (resource.state.value !== 'loading' && state.value.page > last) navigate({ ...state.value, page: last }, true)
})

const ISSUE_LABELS: Record<Exclude<IssueFilter, ''>, string> = { linked: 'Has issue', none: 'No issue' }
const SORT_LABELS: Record<FailureSort, string> = { count: 'Occurrences', updated: 'Last updated' }
const chips = computed(() => [
  ...(state.value.q ? [{ key: 'q', label: `Search: ${state.value.q}` }] : []),
  ...(state.value.issue ? [{ key: 'issue', label: `Issue: ${ISSUE_LABELS[state.value.issue]}` }] : [])
])
function removeChip(key: string): void {
  if (key === 'q') { searchInput.value = ''; patch({ q: '' }) }
  else patch({ issue: '' })
}

function status(group: TriageGroup) {
  return presentFailureGroup(group, isResolvedGroup(group))
}
function updatedTitle(group: TriageGroup): string {
  const seconds = Number(group.issue?.updated_at || 0)
  return seconds ? new Date(seconds * 1000).toISOString() : ''
}

// ---- Search ----------------------------------------------------------------------
const searchInput = ref(state.value.q)
const searchEl = ref<HTMLInputElement | null>(null)
let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(() => state.value.q, (value) => { if (value !== searchInput.value.trim()) searchInput.value = value })
watch(searchInput, (value) => {
  if (searchTimer !== null) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    const q = value.trim()
    if (q !== state.value.q) patch({ q }, true)
  }, 300)
})

// ---- Inspected group (detail pane) ---------------------------------------------
const paneEl = ref<HTMLElement | null>(null)
const inspectedKey = computed(() => state.value.failure)
const inspected = computed(() => groups.value.find((group) => group.key === inspectedKey.value) || null)

function openGroup(key: string, focusPane = true): void {
  if (key !== inspectedKey.value) navigate({ ...state.value, failure: key }, Boolean(inspectedKey.value))
  if (focusPane) void nextTick(() => paneEl.value?.focus())
}
function closeGroup(): void {
  const key = inspectedKey.value
  navigate({ ...state.value, failure: '' })
  void nextTick(() => rowEl(key)?.focus())
}
function rowEl(key: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-failure-row="${CSS.escape(key)}"]`)
}
function moveSelection(step: number): void {
  const index = rows.value.findIndex((group) => group.key === inspectedKey.value)
  const next = rows.value[index + step]
  if (!next) return
  openGroup(next.key, false)
  void nextTick(() => rowEl(next.key)?.focus())
}

const dialogOpen = computed(() => Boolean(cleanupTarget.value) || Boolean(dismissTarget.value))
function onKeydown(event: KeyboardEvent): void {
  if (dialogOpen.value || event.metaKey || event.ctrlKey || event.altKey) return
  const target = event.target as HTMLElement | null
  const typing = Boolean(target?.closest('input, select, textarea, [role="menu"]'))
  if (event.key === '/' && !typing) {
    event.preventDefault()
    searchEl.value?.focus()
    return
  }
  if (!inspectedKey.value || typing) return
  if (event.key === 'Escape') {
    event.preventDefault()
    closeGroup()
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
const actionError = ref('')
function failed(cause: unknown, fallback: string): string {
  return cause instanceof Error ? cause.message : fallback
}

// Open in Live carries the group so Live can show "From failure group" with a back link.
function openInLive(runID: string, key: string): void {
  window.history.pushState(null, '', operatorHref('live', { run: runID, failure: key, job: '', output: '' }, window.location.search))
  window.dispatchEvent(new PopStateEvent('popstate'))
}

// ---- Investigate --------------------------------------------------------------------
const investigating = ref(new Set<string>())
const investigateError = ref<Record<string, string>>({})
async function investigate(group: TriageGroup): Promise<void> {
  if (investigating.value.has(group.key)) return
  investigating.value = new Set([...investigating.value, group.key])
  investigateError.value = { ...investigateError.value, [group.key]: '' }
  try {
    await investigateTriage(group.key)
    announce(`Investigation requested for ${groupCleanupLabel(group)}`)
    await resource.retry()
  } catch (cause) {
    const message = failed(cause, 'Investigation request failed')
    investigateError.value = { ...investigateError.value, [group.key]: message }
    announce(`Investigate failed: ${message}`)
  } finally {
    const next = new Set(investigating.value)
    next.delete(group.key)
    investigating.value = next
  }
}

// ---- Selection ------------------------------------------------------------------------
const selectedKeys = ref<Set<string>>(new Set())
const selectMode = ref(false)
const selectedGroups = computed(() => groups.value.filter((group) => selectedKeys.value.has(group.key)))
const selectedDismissable = computed(() => selectedGroups.value.filter(canDismiss))
const allVisibleSelected = computed(() => rows.value.length > 0 && rows.value.every((group) => selectedKeys.value.has(group.key)))
function toggleSelection(key: string): void {
  const next = new Set(selectedKeys.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  selectedKeys.value = next
}
function toggleVisibleSelection(): void {
  const next = new Set(selectedKeys.value)
  if (allVisibleSelected.value) for (const group of rows.value) next.delete(group.key)
  else for (const group of rows.value) next.add(group.key)
  selectedKeys.value = next
}
function clearSelection(): void {
  selectedKeys.value = new Set()
  selectMode.value = false
}
function dropSelected(keys: string[]): void {
  selectedKeys.value = new Set([...selectedKeys.value].filter((key) => !keys.includes(key)))
}

// ---- Delete matching runs -----------------------------------------------------------
const cleanupBusy = ref(false)
const cleanupTarget = ref<TriageGroup[] | null>(null)
const cleanupCount = ref(0)
const cleanupProgress = ref('')

function plural(count: number, noun: string): string { return `${count} ${noun}${count === 1 ? '' : 's'}` }
const cleanupTitle = computed(() => {
  const target = cleanupTarget.value || []
  const subject = target.length === 1 ? groupCleanupLabel(target[0]) : plural(target.length, 'failure group')
  return `Delete ${plural(cleanupCount.value, 'run')} for ${subject}?`
})
const cleanupMessage = computed(() => {
  const target = cleanupTarget.value || []
  const subject = target.length === 1 ? `Pattern: ${groupPattern(target[0])}` : target.map(groupCleanupLabel).join(', ')
  const body = `Every finished run that still matches is deleted, with its S3 artifacts and replay cache. This can't be undone.\n\n${subject}`
  return cleanupBusy.value && cleanupProgress.value ? `${cleanupProgress.value}\n\n${body}` : body
})

async function requestCleanup(target: TriageGroup[]): Promise<void> {
  if (cleanupBusy.value || !target.length) return
  actionError.value = ''
  announce('Checking matching runs…')
  try {
    const snapshot = await getDashboard({ status: 'done' })
    const runs = matchingRunsForGroups(snapshot.runs, target)
    if (!runs.length) {
      announce(`No finished runs still match ${target.length === 1 ? groupCleanupLabel(target[0]) : 'the selected groups'}.`)
      return
    }
    cleanupCount.value = runs.length
    cleanupTarget.value = target
  } catch (cause) {
    actionError.value = `Couldn’t check matching runs: ${failed(cause, 'unknown error')}`
  }
}
function closeCleanup(): void {
  if (!cleanupBusy.value) cleanupTarget.value = null
}
async function confirmCleanup(): Promise<void> {
  const target = cleanupTarget.value
  if (!target?.length || cleanupBusy.value) return
  cleanupBusy.value = true
  cleanupProgress.value = 'Checking matching runs…'
  try {
    // Re-check at confirm time: runs may have finished or been deleted since the count.
    const snapshot = await getDashboard({ status: 'done' })
    const runs = matchingRunsForGroups(snapshot.runs, target)
    const result = await deleteRuns(
      runs.map((run) => run.run_id),
      (id) => deleteRun(id),
      DELETE_CONCURRENCY,
      (progress) => { cleanupProgress.value = cleanupProgressText(progress) }
    )
    announce(runs.length ? cleanupResultText(result) : 'No finished runs still match.')
    dropSelected(target.map((group) => group.key))
    cleanupTarget.value = null
    await resource.retry()
  } catch (cause) {
    actionError.value = `Cleanup failed: ${failed(cause, 'unknown error')}`
    cleanupTarget.value = null
  } finally {
    cleanupBusy.value = false
    cleanupProgress.value = ''
  }
}

// ---- Dismiss ---------------------------------------------------------------------------
const dismissBusy = ref(false)
const dismissTarget = ref<TriageGroup[] | null>(null)
const dismissError = ref('')
function requestDismiss(target: TriageGroup[]): void {
  const eligible = target.filter(canDismiss)
  if (dismissBusy.value || !eligible.length) return
  dismissError.value = ''
  dismissTarget.value = eligible
}
function closeDismiss(): void {
  if (!dismissBusy.value) dismissTarget.value = null
}
async function confirmDismiss(): Promise<void> {
  const target = dismissTarget.value
  if (!target?.length || dismissBusy.value) return
  dismissBusy.value = true
  dismissError.value = ''
  try {
    const result = await dismissTriages(target.map((group) => group.key))
    const dismissed = Number(result.groups || 0)
    const skipped = result.skipped_linked?.length || 0
    announce(`Dismissed ${plural(dismissed, 'failure group')}${skipped ? `; skipped ${skipped} that gained an issue` : ''}. Future occurrences will reappear.`)
    dropSelected(target.map((group) => group.key))
    dismissTarget.value = null
    await resource.retry()
  } catch (cause) {
    // The dialog stays open with the reason so the operator can retry or cancel.
    dismissError.value = failed(cause, 'Could not dismiss failure group')
  } finally {
    dismissBusy.value = false
  }
}
</script>

<template>
  <div class="space-y-3">
    <!-- Page header -->
    <header class="flex min-h-12 flex-wrap items-center gap-x-3 gap-y-2">
      <h1 class="text-page font-semibold">Failures</h1>
      <span v-if="resource.data.value" class="text-fg-2">{{ formatInteger(viewCounts.open) }} open</span>
      <span v-if="resource.state.value === 'refreshing'" class="inline-flex items-center gap-1.5 text-meta text-fg-2" role="status">
        <span class="size-1.5 bg-accent motion-safe:animate-pulse" aria-hidden="true" /> Refreshing
      </span>
      <div class="ml-auto flex flex-wrap items-center gap-2">
        <button type="button" class="ui-btn md:hidden" :aria-pressed="selectMode" @click="selectMode ? clearSelection() : (selectMode = true)">{{ selectMode ? 'Done' : 'Select' }}</button>
        <button v-if="dismissableGroups.length" type="button" class="ui-btn max-md:h-10" :disabled="dismissBusy" @click="requestDismiss(dismissableGroups)">
          Dismiss unlinked ({{ dismissableGroups.length }})…
        </button>
        <button type="button" class="ui-btn ui-icon-btn max-md:size-10" aria-label="Refresh failures" title="Refresh" @click="resource.retry()">
          <ArrowPathIcon class="size-4" aria-hidden="true" />
        </button>
      </div>
    </header>

    <p v-if="actionError" class="border border-danger/40 bg-danger/10 px-3 py-2 text-danger" role="alert">✕ {{ actionError }}</p>

    <!-- Views -->
    <div class="inline-flex border border-rule-strong" role="group" aria-label="Failure group state">
      <button
        v-for="view in FAILURE_VIEWS"
        :key="view.id"
        type="button"
        :aria-pressed="state.view === view.id"
        :class="state.view === view.id ? 'bg-selected text-fg' : 'text-fg-2 hover:bg-raised hover:text-fg'"
        class="h-7 border-r border-rule-strong px-3 whitespace-nowrap last:border-r-0 max-md:h-10"
        @click="patch({ view: view.id })"
      >
        {{ view.label }} <span class="font-mono text-meta text-fg-2">{{ resource.data.value ? formatInteger(viewCounts[view.id]) : '' }}</span>
      </button>
    </div>

    <div :class="inspectedKey ? 'xl:grid xl:grid-cols-[minmax(0,1fr)_400px] xl:gap-3 min-[1440px]:grid-cols-[minmax(0,1fr)_440px]' : ''">
      <div class="min-w-0 border border-rule-strong bg-surface">
        <!-- FilterBar, or the bulk bar in its place while rows are selected -->
        <div v-if="selectedKeys.size" class="flex min-h-12 flex-wrap items-center gap-3 border-b border-rule bg-raised px-3 py-2" role="region" aria-label="Bulk actions">
          <span class="font-semibold">{{ selectedKeys.size }} selected</span>
          <button type="button" class="ui-btn ui-btn-sm ui-btn-danger max-md:h-10" :disabled="cleanupBusy" @click="requestCleanup(selectedGroups)">Delete matching runs…</button>
          <button type="button" class="ui-btn ui-btn-sm max-md:h-10" :disabled="dismissBusy || !selectedDismissable.length" :aria-describedby="selectedDismissable.length ? undefined : 'bulk-dismiss-reason'" @click="requestDismiss(selectedGroups)">
            Dismiss<template v-if="selectedDismissable.length && selectedDismissable.length < selectedKeys.size"> {{ selectedDismissable.length }}</template>…
          </button>
          <span v-if="!selectedDismissable.length" id="bulk-dismiss-reason" class="text-meta text-fg-2">Only unlinked groups can be dismissed</span>
          <button type="button" class="ui-btn ui-btn-sm ui-btn-quiet max-md:h-10" @click="clearSelection">Clear selection</button>
        </div>
        <div v-else class="border-b border-rule px-3 py-2">
          <div class="flex flex-wrap items-center gap-2">
            <div class="w-full md:w-60">
              <label for="failures-search" class="sr-only">Search failures</label>
              <input
                id="failures-search"
                ref="searchEl"
                v-model="searchInput"
                type="search"
                autocomplete="off"
                spellcheck="false"
                placeholder="Search pattern, key or run ID…  /"
                class="block h-7 w-full rounded-[3px] border border-control bg-canvas px-2.5 text-fg placeholder:text-fg-3 max-md:h-10"
              >
            </div>
            <select
              :value="state.issue"
              aria-label="Issue"
              class="h-7 rounded-[3px] border border-control bg-canvas px-2 text-fg max-md:h-10 max-md:flex-1"
              @change="patch({ issue: ($event.target as HTMLSelectElement).value as IssueFilter })"
            >
              <option value="">Issue: any</option>
              <option value="linked">Has issue</option>
              <option value="none">No issue</option>
            </select>
            <div class="ml-auto flex items-center gap-2 max-md:w-full">
              <label class="flex items-center gap-1.5 max-md:flex-1">
                <span class="ui-label">Sort by</span>
                <select
                  :value="state.sort"
                  class="h-7 rounded-[3px] border border-control bg-canvas px-2 text-fg max-md:h-10 max-md:flex-1"
                  @change="patch({ sort: ($event.target as HTMLSelectElement).value as FailureSort })"
                >
                  <option v-for="(label, key) in SORT_LABELS" :key="key" :value="key">{{ label }}</option>
                </select>
              </label>
              <span class="text-meta whitespace-nowrap text-fg-2 tabular-nums" aria-live="polite">{{ rangeStart }}–{{ rangeEnd }} of {{ formatInteger(total) }}</span>
            </div>
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

        <ResourceState
          v-if="resource.state.value === 'stale'"
          state="stale"
          class="p-3"
          title="Showing the last loaded failure groups"
          :message="`Refresh failed: ${resource.error.value}`"
        >
          <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="resource.retry()">Retry</button></template>
        </ResourceState>

        <!-- Desktop / tablet table -->
        <div class="hidden md:block" role="region" aria-label="Failure groups table">
          <table class="w-full table-fixed border-collapse text-left">
            <thead>
              <tr class="h-9 border-b border-rule-strong bg-raised text-meta font-medium text-fg-2">
                <th class="w-10 pl-3">
                  <input
                    type="checkbox"
                    :checked="allVisibleSelected"
                    :aria-label="allVisibleSelected ? 'Deselect all groups on this page' : 'Select all groups on this page'"
                    class="size-4 accent-[var(--color-accent)]"
                    @change="toggleVisibleSelection"
                  >
                </th>
                <th class="w-36 px-3 font-medium">Status</th>
                <th class="px-3 font-medium">Failure</th>
                <th class="w-20 px-3 text-right font-medium" :aria-sort="state.sort === 'count' ? 'descending' : 'none'">Count</th>
                <th class="w-52 px-3 font-medium max-lg:hidden" :class="inspectedKey ? 'hidden min-[1680px]:table-cell' : ''">Latest run</th>
                <th class="w-28 px-3 font-medium max-xl:hidden" :aria-sort="state.sort === 'updated' ? 'descending' : 'none'">Updated</th>
                <th class="w-11 pr-2"><span class="sr-only">Actions</span></th>
              </tr>
            </thead>
            <tbody v-if="resource.state.value === 'loading'" aria-busy="true">
              <tr v-for="n in 8" :key="n" class="h-14 border-b border-rule">
                <td colspan="7" class="px-3"><div class="h-4 bg-raised motion-safe:animate-pulse" :class="n % 2 ? 'w-3/4' : 'w-1/2'" /></td>
              </tr>
            </tbody>
            <tbody v-else-if="loaded">
              <tr
                v-for="group in rows"
                :key="group.key"
                :data-failure-row="group.key"
                tabindex="0"
                :aria-selected="group.key === inspectedKey"
                :class="group.key === inspectedKey ? 'bg-selected shadow-[inset_2px_0_0_var(--color-accent)]' : 'hover:bg-raised'"
                class="group h-14 cursor-pointer border-b border-rule align-middle focus-visible:outline-offset-[-2px]"
                @click="openGroup(group.key)"
                @keydown.enter.self="openGroup(group.key)"
              >
                <td class="pl-3" @click.stop>
                  <input
                    type="checkbox"
                    :checked="selectedKeys.has(group.key)"
                    :aria-label="`Select failure group ${group.key}`"
                    class="size-4 accent-[var(--color-accent)]"
                    @change="toggleSelection(group.key)"
                  >
                </td>
                <td class="px-3">
                  <StatusPill :status="status(group)" class="max-w-full overflow-hidden" />
                  <a v-if="group.issue?.issue_number && issueURL(group)" :href="issueURL(group)" target="_blank" rel="noopener" class="mt-0.5 block text-meta text-accent hover:underline" @click.stop>#{{ group.issue.issue_number }} ↗</a>
                </td>
                <td class="px-3">
                  <div class="truncate" :title="groupTitle(group)">{{ groupTitle(group) }}</div>
                  <div class="flex min-w-0 items-center gap-1">
                    <span class="truncate font-mono text-meta text-fg-2">{{ group.key }}</span>
                    <button
                      type="button"
                      class="ui-btn ui-btn-sm ui-icon-btn size-6 shrink-0 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 focus-visible:opacity-100"
                      :aria-label="`Copy group key ${group.key}`"
                      @click.stop="copy(group.key, 'Group key')"
                    >
                      <ClipboardDocumentIcon class="size-4" aria-hidden="true" />
                    </button>
                  </div>
                </td>
                <td class="px-3 text-right font-mono">{{ formatInteger(Number(group.count || 0)) }}</td>
                <td class="px-3 max-lg:hidden" :class="inspectedKey ? 'hidden min-[1680px]:table-cell' : ''">
                  <div v-if="latestRun(group)" class="flex min-w-0 items-center gap-1">
                    <span class="truncate font-mono" :title="latestRun(group)">{{ middleTruncate(latestRun(group)) }}</span>
                    <button
                      type="button"
                      class="ui-btn ui-btn-sm ui-icon-btn size-6 shrink-0 opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 focus-visible:opacity-100"
                      :aria-label="`Copy run ID ${latestRun(group)}`"
                      @click.stop="copy(latestRun(group), 'Run ID')"
                    >
                      <ClipboardDocumentIcon class="size-4" aria-hidden="true" />
                    </button>
                  </div>
                  <div v-else class="text-fg-3">—</div>
                  <div class="text-meta text-fg-2">{{ exampleRuns(group).length }} run{{ exampleRuns(group).length === 1 ? '' : 's' }}</div>
                </td>
                <td class="px-3 whitespace-nowrap max-xl:hidden" :title="updatedTitle(group)">{{ timeAgo(Number(group.issue?.updated_at || 0)) }}</td>
                <td class="pr-2 text-right" @click.stop @keydown.stop>
                  <FailureActionMenu
                    :group-key="group.key"
                    :latest-run="latestRun(group)"
                    :dismissable="canDismiss(group)"
                    @live="openInLive(latestRun(group), group.key)"
                    @copy="copy(group.key, 'Group key')"
                    @dismiss="requestDismiss([group])"
                    @cleanup="requestCleanup([group])"
                  />
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
          <ul v-else-if="loaded" role="list" aria-label="Failure groups">
            <li
              v-for="group in rows"
              :key="group.key"
              :class="group.key === inspectedKey ? 'bg-selected shadow-[inset_2px_0_0_var(--color-accent)]' : ''"
              class="flex gap-3 border-b border-rule px-3 py-2.5"
            >
              <input
                v-if="selectMode"
                type="checkbox"
                :checked="selectedKeys.has(group.key)"
                :aria-label="`Select failure group ${group.key}`"
                class="mt-1 size-5 shrink-0 accent-[var(--color-accent)]"
                @change="toggleSelection(group.key)"
              >
              <div class="min-w-0 flex-1">
                <div class="flex min-w-0 items-center gap-2">
                  <StatusPill :status="status(group)" class="shrink-0" />
                  <span class="font-mono text-fg-2">×{{ formatInteger(Number(group.count || 0)) }}</span>
                  <span class="ml-auto shrink-0 text-meta text-fg-2" :title="updatedTitle(group)">{{ timeAgo(Number(group.issue?.updated_at || 0)) }}</span>
                </div>
                <button type="button" class="mt-1 block w-full truncate text-left text-accent" :title="groupTitle(group)" @click="openGroup(group.key)">{{ groupTitle(group) }}</button>
                <div class="mt-1 flex min-w-0 items-center gap-2 text-meta text-fg-2">
                  <span class="truncate font-mono">{{ group.key }}</span>
                  <a v-if="group.issue?.issue_number && issueURL(group)" :href="issueURL(group)" target="_blank" rel="noopener" class="ml-auto shrink-0 text-accent">#{{ group.issue.issue_number }} ↗</a>
                </div>
              </div>
              <FailureActionMenu
                class="shrink-0"
                :group-key="group.key"
                :latest-run="latestRun(group)"
                :dismissable="canDismiss(group)"
                @live="openInLive(latestRun(group), group.key)"
                @copy="copy(group.key, 'Group key')"
                @dismiss="requestDismiss([group])"
                @cleanup="requestCleanup([group])"
              />
            </li>
          </ul>
        </div>

        <!-- Error / empty states sit inside the table frame, under the header -->
        <ResourceState
          v-if="resource.state.value === 'error'"
          state="error"
          class="m-3"
          title="Couldn’t load failure groups"
          :message="`${resource.error.value}. Retry when the wall is reachable.`"
        >
          <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="resource.retry()">Retry</button></template>
        </ResourceState>
        <div v-else-if="resource.state.value === 'empty' || (loaded && !rows.length)" class="flex flex-col items-center gap-2 px-4 py-12 text-center">
          <span class="text-[20px] leading-5 text-fg-3" aria-hidden="true">○</span>
          <template v-if="resource.state.value === 'empty'">
            <p class="text-title font-semibold">No failure groups</p>
            <p class="text-fg-2">New actionable failure groups appear here.</p>
          </template>
          <template v-else>
            <p class="text-title font-semibold">{{ hasFilters ? 'No failure groups match' : `No ${state.view} failure groups` }}</p>
            <p class="text-fg-2">{{ hasFilters ? 'Remove a filter or clear them all.' : 'Nothing needs attention in this view.' }}</p>
            <button v-if="hasFilters" type="button" class="ui-btn mt-1" @click="clearFilters">Clear filters</button>
          </template>
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
      <template v-if="inspectedKey">
        <div class="fixed inset-0 z-40 bg-black/50 xl:hidden" aria-hidden="true" @click="closeGroup" />
        <aside
          ref="paneEl"
          tabindex="-1"
          aria-label="Failure group details"
          class="fixed inset-0 z-50 border-rule-strong shadow-[0_8px_24px_rgb(0_0_0/45%)] outline-none md:left-auto md:w-[440px] md:border-l xl:sticky xl:top-14 xl:z-auto xl:h-[calc(100vh-168px)] xl:self-start xl:w-auto xl:border xl:shadow-none"
        >
          <FailureDetailPane
            :group-key="inspectedKey"
            :group="inspected"
            :loading="resource.state.value === 'loading'"
            :investigating="investigating.has(inspectedKey)"
            :investigate-error="investigateError[inspectedKey] || ''"
            :cleanup-busy="cleanupBusy"
            :dismiss-busy="dismissBusy"
            @close="closeGroup"
            @copy="copy"
            @live="(runID) => openInLive(runID, inspectedKey)"
            @investigate="investigate"
            @dismiss="(group) => requestDismiss([group])"
            @cleanup="(group) => requestCleanup([group])"
          />
        </aside>
      </template>
    </div>

    <ConfirmDialog
      :open="Boolean(cleanupTarget)"
      :title="cleanupTitle"
      :message="cleanupMessage"
      :confirm-label="`Delete ${plural(cleanupCount, 'run')}`"
      :require-text="cleanupCount > 10 ? String(cleanupCount) : ''"
      :busy="cleanupBusy"
      danger
      @close="closeCleanup"
      @confirm="confirmCleanup"
    />

    <ConfirmDialog
      :open="Boolean(dismissTarget)"
      :title="dismissTarget ? `Dismiss ${plural(dismissTarget.length, 'unlinked failure group')}?` : 'Dismiss failure group?'"
      :message="`Hidden from triage; run data is kept. If the same failure happens again, it appears again.${dismissError ? `\n\n✕ Dismiss failed: ${dismissError}` : ''}`"
      :confirm-label="dismissTarget && dismissTarget.length > 1 ? `Dismiss ${dismissTarget.length} groups` : 'Dismiss group'"
      :busy="dismissBusy"
      @close="closeDismiss"
      @confirm="confirmDismiss"
    />

    <div class="pointer-events-none fixed right-4 bottom-4 z-[80]" aria-live="polite" role="status">
      <div v-if="toast" class="pointer-events-auto rounded-md border border-rule-strong bg-raised px-4 py-2.5 shadow-[0_8px_24px_rgb(0_0_0/45%)]">{{ toast }}</div>
    </div>
  </div>
</template>
