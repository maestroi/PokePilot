<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowPathIcon, ArrowTopRightOnSquareIcon, MagnifyingGlassIcon, TrashIcon } from '@heroicons/vue/20/solid'
import { deleteRun, getDashboard, getTriage, investigateTriage } from '../shared/api/client'
import type { TriageGroup } from '../shared/api/types'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
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

const investigating = ref(new Set<string>())
const selectedKeys = ref(new Set<string>())
const actionError = ref('')
const cleanupStatus = ref('')
const cleanupBusy = ref(false)
const cleanupTarget = ref<TriageGroup[] | null>(null)
const cleanupCount = ref(0)

const resource = usePollingResource(
  (signal) => getTriage(signal),
  { intervalMs: 5000, isEmpty: (groups) => groups.length === 0 }
)

const openGroups = computed(() => resource.data.value?.filter((group) => !isResolvedGroup(group)) ?? [])
const resolvedGroups = computed(() => resource.data.value?.filter(isResolvedGroup) ?? [])
const selectedResolved = computed(() => resolvedGroups.value.filter((group) => selectedKeys.value.has(group.key)))
const allResolvedSelected = computed(() =>
  resolvedGroups.value.length > 0 && resolvedGroups.value.every((group) => selectedKeys.value.has(group.key))
)

function issueURL(group: TriageGroup): string {
  const raw = group.issue?.issue_url || ''
  if (!raw) return ''
  try {
    const url = new URL(raw)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.toString() : ''
  } catch {
    return ''
  }
}

function groupTitle(group: TriageGroup): string {
  return group.pattern || group.detail || group.example || group.fingerprint || group.key
}

function exampleRuns(group: TriageGroup): string[] {
  const candidates = group.run_ids || group.runs || group.examples || []
  return candidates.slice(0, 5).map(String)
}

function inspectURL(runID: string): string {
  const url = new URL('/', window.location.origin)
  url.searchParams.set('run', runID)
  url.hash = 'live'
  return url.toString()
}

function toggleResolved(key: string, checked: boolean): void {
  const next = new Set(selectedKeys.value)
  if (checked) next.add(key)
  else next.delete(key)
  selectedKeys.value = next
}

function toggleAllResolved(checked: boolean): void {
  selectedKeys.value = checked ? new Set(resolvedGroups.value.map((group) => group.key)) : new Set()
}

function cleanupTitle(groups: TriageGroup[]): string {
  if (groups.length === 1) return `Delete ${groupCleanupLabel(groups[0])} runs?`
  return `Delete ${groups.length} resolved issues?`
}

function cleanupMessage(groups: TriageGroup[], count: number): string {
  if (!groups.length) return ''
  if (count === 0) return 'No finished runs still match the selected failure group.'
  const subject = groups.length === 1
    ? `${groupCleanupLabel(groups[0])}. Pattern: ${groupPattern(groups[0])}`
    : groups.map(groupCleanupLabel).join(', ')
  return `Delete all ${count} finished run${count === 1 ? '' : 's'} for ${subject}? This also permanently deletes their S3 artifacts and replay cache. This cannot be undone.`
}

async function requestCleanup(groups: TriageGroup[]): Promise<void> {
  if (cleanupBusy.value || !groups.length) return
  actionError.value = ''
  cleanupStatus.value = 'Checking matching runs…'
  try {
    const snapshot = await getDashboard({ status: 'done' })
    const runs = matchingRunsForGroups(snapshot.runs, groups)
    cleanupCount.value = runs.length
    if (!runs.length) {
      cleanupStatus.value = `No finished runs still match ${groups.length === 1 ? groups[0].key : 'the selected groups'}.`
      return
    }
    cleanupTarget.value = groups
    cleanupStatus.value = `${runs.length} matching finished run${runs.length === 1 ? '' : 's'} ready to delete.`
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Could not check matching runs'
    cleanupStatus.value = ''
  }
}

function closeCleanup(): void {
  if (!cleanupBusy.value) cleanupTarget.value = null
}

async function confirmCleanup(): Promise<void> {
  const groups = cleanupTarget.value
  if (!groups?.length || cleanupBusy.value) return
  cleanupBusy.value = true
  actionError.value = ''
  cleanupStatus.value = 'Checking matching runs…'
  try {
    const snapshot = await getDashboard({ status: 'done' })
    const runs = matchingRunsForGroups(snapshot.runs, groups)
    cleanupCount.value = runs.length
    if (!runs.length) {
      cleanupStatus.value = 'No finished runs still match.'
      cleanupTarget.value = null
      return
    }
    const result = await deleteRuns(
      runs.map((item) => item.run_id),
      (id) => deleteRun(id),
      DELETE_CONCURRENCY,
      (progress) => {
        cleanupStatus.value = cleanupProgressText(progress)
      }
    )
    cleanupStatus.value = cleanupResultText(result)
    selectedKeys.value = new Set([...selectedKeys.value].filter((key) => !groups.some((group) => group.key === key)))
    cleanupTarget.value = null
    await resource.retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Cleanup failed'
  } finally {
    cleanupBusy.value = false
  }
}

async function investigate(group: TriageGroup): Promise<void> {
  if (investigating.value.has(group.key)) return
  actionError.value = ''
  investigating.value = new Set([...investigating.value, group.key])
  try {
    await investigateTriage(group.key)
    await resource.retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Investigation request failed'
  } finally {
    const next = new Set(investigating.value)
    next.delete(group.key)
    investigating.value = next
  }
}

function retry(): void {
  void resource.retry()
}
</script>

<template>
  <div class="space-y-3">
    <Panel title="Failure triage" description="Actionable failures grouped by their normalized fingerprint, not one row per failed attempt." compact>
      <template #actions>
        <button type="button" class="inline-flex items-center gap-1.5 rounded-md bg-white/8 px-2.5 py-1.5 text-xs font-semibold text-slate-200 ring-1 ring-white/10 hover:bg-white/12" @click="retry">
          <ArrowPathIcon class="size-3.5" aria-hidden="true" />
          Refresh
        </button>
      </template>

      <ResourceState
        :state="resource.state.value"
        :title="resource.state.value === 'empty' ? 'No failure groups' : 'Failure triage unavailable'"
        :message="resource.error.value || 'New actionable failure groups will appear here.'"
        :rows="6"
      >
        <template #actions>
          <button type="button" class="rounded-md bg-white/10 px-2.5 py-1.5 text-xs font-semibold text-white ring-1 ring-white/10 hover:bg-white/15" @click="retry">Retry now</button>
        </template>

        <p v-if="actionError" class="mb-3 text-sm text-rose-300" role="alert">{{ actionError }}</p>
        <p v-if="cleanupStatus" class="mb-3 text-xs text-slate-400" aria-live="polite">{{ cleanupStatus }}</p>

        <div v-if="openGroups.length" class="divide-y divide-white/8">
          <article v-for="group in openGroups" :key="group.key" class="py-4 first:pt-0 last:pb-0">
            <div class="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2">
                  <StatusBadge tone="danger">{{ Number(group.count || 0) }} occurrence{{ Number(group.count || 0) === 1 ? '' : 's' }}</StatusBadge>
                  <StatusBadge v-if="group.issue?.issue_number" :tone="group.issue?.stale ? 'warning' : 'info'">Issue #{{ group.issue.issue_number }}</StatusBadge>
                  <span class="font-mono text-[10px] text-slate-600">{{ group.key }}</span>
                </div>
                <h3 class="mt-2 break-words text-sm font-semibold leading-6 text-slate-100">{{ groupTitle(group) }}</h3>

                <div v-if="exampleRuns(group).length" class="mt-3 flex flex-wrap gap-1.5">
                  <a
                    v-for="runID in exampleRuns(group)"
                    :key="runID"
                    :href="inspectURL(runID)"
                    class="rounded-md bg-black/20 px-2 py-1 font-mono text-[11px] text-cyan-200 ring-1 ring-white/8 hover:bg-white/8"
                    :title="runID"
                  >
                    {{ runID.length > 18 ? `${runID.slice(0, 18)}…` : runID }}
                  </a>
                </div>
              </div>

              <div class="flex shrink-0 flex-wrap gap-2">
                <a
                  v-if="issueURL(group)"
                  :href="issueURL(group)"
                  target="_blank"
                  rel="noopener"
                  class="inline-flex items-center gap-1.5 rounded-md bg-white/6 px-2.5 py-1.5 text-xs font-semibold text-slate-300 ring-1 ring-white/8 hover:bg-white/10 hover:text-white"
                >
                  Open issue
                  <ArrowTopRightOnSquareIcon class="size-3.5" aria-hidden="true" />
                </a>
                <button
                  type="button"
                  :disabled="cleanupBusy"
                  class="inline-flex items-center gap-1.5 rounded-md bg-rose-400/8 px-2.5 py-1.5 text-xs font-semibold text-rose-200 ring-1 ring-rose-300/15 hover:bg-rose-400/15 disabled:cursor-wait disabled:opacity-60"
                  @click="requestCleanup([group])"
                >
                  <TrashIcon class="size-3.5" aria-hidden="true" />
                  Delete matching
                </button>
                <button
                  type="button"
                  :disabled="investigating.has(group.key)"
                  class="inline-flex items-center gap-1.5 rounded-md bg-cyan-500 px-2.5 py-1.5 text-xs font-semibold text-white hover:bg-cyan-400 disabled:cursor-wait disabled:opacity-60"
                  @click="investigate(group)"
                >
                  <MagnifyingGlassIcon class="size-3.5" aria-hidden="true" />
                  {{ investigating.has(group.key) ? 'Starting…' : 'Investigate' }}
                </button>
              </div>
            </div>
          </article>
        </div>

        <p v-else class="py-8 text-center text-sm text-slate-500">No open failure groups.</p>
      </ResourceState>
    </Panel>

    <Panel v-if="resolvedGroups.length" title="Resolved history" description="Already-solved issues. Select one or many, then delete every finished run that still matches that failure." compact>
      <template #actions>
        <button
          type="button"
          :disabled="cleanupBusy || selectedResolved.length === 0"
          class="inline-flex items-center gap-1.5 rounded-md bg-rose-500 px-2.5 py-1.5 text-xs font-semibold text-white hover:bg-rose-400 disabled:cursor-not-allowed disabled:opacity-35"
          @click="requestCleanup(selectedResolved)"
        >
          <TrashIcon class="size-3.5" aria-hidden="true" />
          Delete selected{{ selectedResolved.length ? ` (${selectedResolved.length})` : '' }}
        </button>
      </template>

      <div class="mb-2 flex items-center justify-between gap-3 border-b border-white/8 pb-2">
        <label class="inline-flex items-center gap-2 text-xs text-slate-400">
          <input
            type="checkbox"
            class="size-3.5 rounded border-white/20 bg-white/5 text-cyan-400 focus:ring-cyan-400"
            :checked="allResolvedSelected"
            :disabled="cleanupBusy"
            @change="toggleAllResolved(($event.target as HTMLInputElement).checked)"
          />
          Select all {{ resolvedGroups.length }} resolved group{{ resolvedGroups.length === 1 ? '' : 's' }}
        </label>
      </div>

      <div class="max-h-[32rem] divide-y divide-white/8 overflow-y-auto">
        <div v-for="group in resolvedGroups" :key="group.key" class="flex items-start gap-3 py-2.5 first:pt-0 last:pb-0">
          <input
            type="checkbox"
            class="mt-1 size-3.5 rounded border-white/20 bg-white/5 text-cyan-400 focus:ring-cyan-400"
            :checked="selectedKeys.has(group.key)"
            :disabled="cleanupBusy"
            :aria-label="`Select ${groupCleanupLabel(group)}`"
            @change="toggleResolved(group.key, ($event.target as HTMLInputElement).checked)"
          />
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2">
              <StatusBadge v-if="group.issue?.issue_number" tone="info">Issue #{{ group.issue.issue_number }}</StatusBadge>
              <StatusBadge tone="success">resolved</StatusBadge>
              <StatusBadge tone="neutral">{{ Number(group.count || 0) }} recorded</StatusBadge>
            </div>
            <p class="mt-1 truncate text-xs text-slate-300" :title="groupTitle(group)">{{ groupTitle(group) }}</p>
            <p class="mt-0.5 font-mono text-[10px] text-slate-600">{{ group.key }}</p>
          </div>
          <button
            type="button"
            :disabled="cleanupBusy"
            class="inline-flex shrink-0 items-center gap-1 rounded-md bg-rose-400/8 px-2 py-1.5 text-[11px] font-semibold text-rose-200 ring-1 ring-rose-300/15 hover:bg-rose-400/15 disabled:cursor-wait disabled:opacity-60"
            @click="requestCleanup([group])"
          >
            <TrashIcon class="size-3.5" aria-hidden="true" />
            Delete matching
          </button>
        </div>
      </div>
    </Panel>

    <ConfirmDialog
      :open="Boolean(cleanupTarget)"
      :title="cleanupTarget ? cleanupTitle(cleanupTarget) : 'Delete matching runs?'"
      :message="cleanupBusy ? cleanupStatus : (cleanupTarget ? cleanupMessage(cleanupTarget, cleanupCount) : '')"
      :confirm-label="cleanupCount ? `Delete ${cleanupCount} run${cleanupCount === 1 ? '' : 's'}` : 'Delete matching runs'"
      :busy="cleanupBusy"
      danger
      @close="closeCleanup"
      @confirm="confirmCleanup"
    />
  </div>
</template>
