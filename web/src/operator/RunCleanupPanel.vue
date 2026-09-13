<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { deleteRun, getDashboard, getTriage } from '../shared/api/client'
import type { DashboardSnapshot, TriageGroup } from '../shared/api/types'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
import Panel from '../shared/components/Panel.vue'
import {
  AGE_OPTIONS,
  DEFAULT_AGE_SECONDS,
  DELETE_CONCURRENCY,
  ageDescription,
  bugGroupRuns,
  cleanupProgressText,
  cleanupResultText,
  deleteRuns,
  eligibleRuns,
  groupPattern,
  isResolvedGroup
} from './runCleanup'

const emit = defineEmits<{
  changed: []
}>()

const selectedAge = ref(DEFAULT_AGE_SECONDS)
const selectedBugKey = ref('')
const catalog = ref<DashboardSnapshot | null>(null)
const groups = ref<TriageGroup[]>([])
const busyKind = ref<'age' | 'bug' | ''>('')
const ageStatus = ref('')
const bugStatus = ref('')
const loadError = ref('')
const pendingKind = ref<'age' | 'bug' | ''>('')

const selectedBug = computed(() => groups.value.find((group) => group.key === selectedBugKey.value) || null)
const ageCandidates = computed(() => eligibleRuns(catalog.value?.runs, Number(catalog.value?.now || 0), selectedAge.value))
const bugCandidates = computed(() => selectedBug.value ? bugGroupRuns(catalog.value?.runs, groupPattern(selectedBug.value)) : [])
const confirmCount = computed(() => pendingKind.value === 'bug' ? bugCandidates.value.length : ageCandidates.value.length)
const confirmTitle = computed(() => pendingKind.value === 'bug' ? 'Delete matching failure runs?' : 'Delete older runs?')
const confirmMessage = computed(() => {
  if (busyKind.value) return pendingKind.value === 'bug' ? bugStatus.value : ageStatus.value
  if (pendingKind.value === 'bug' && selectedBug.value) {
    return `Delete all ${bugCandidates.value.length} finished run${bugCandidates.value.length === 1 ? '' : 's'} for ${selectedBug.value.key}? Pattern: ${groupPattern(selectedBug.value)}. This also permanently deletes their S3 artifacts and replay cache. This cannot be undone.`
  }
  if (pendingKind.value === 'age') {
    return `Delete ${ageCandidates.value.length} finished run${ageCandidates.value.length === 1 ? '' : 's'} older than ${ageDescription(selectedAge.value)}? This also permanently deletes their S3 artifacts and replay cache. This cannot be undone.`
  }
  return ''
})

function groupOptionLabel(group: TriageGroup): string {
  const state = isResolvedGroup(group) ? 'resolved' : String(group.issue?.status || group.issue?.resolution || '').trim()
  const prefix = state ? `[${state}] ` : ''
  const issue = group.issue?.issue_number ? ` #${group.issue.issue_number}` : ''
  return `${prefix}${group.pattern || group.key}${issue} (${Number(group.count) || 0})`
}

async function refreshCatalog(): Promise<void> {
  const [snapshot, triage] = await Promise.all([
    getDashboard({ status: 'done' }),
    getTriage()
  ])
  catalog.value = snapshot
  groups.value = triage.filter((group) => group.key && groupPattern(group))
  if (selectedBugKey.value && !groups.value.some((group) => group.key === selectedBugKey.value)) {
    selectedBugKey.value = ''
  }
}

onMounted(() => {
  refreshCatalog().catch((cause) => {
    loadError.value = cause instanceof Error ? cause.message : 'Could not load cleanup candidates'
  })
})

function requestAgeDelete(): void {
  if (busyKind.value || !ageCandidates.value.length) return
  pendingKind.value = 'age'
}

function requestBugDelete(): void {
  if (busyKind.value || !selectedBug.value || !bugCandidates.value.length) return
  pendingKind.value = 'bug'
}

function closePending(): void {
  if (!busyKind.value) pendingKind.value = ''
}

async function confirmPending(): Promise<void> {
  const kind = pendingKind.value
  if (!kind || busyKind.value) return
  busyKind.value = kind
  loadError.value = ''
  const status = kind === 'bug' ? bugStatus : ageStatus
  status.value = 'Checking finished runs…'
  try {
    await refreshCatalog()
    const runs = kind === 'bug' ? bugCandidates.value : ageCandidates.value
    if (!runs.length) {
      status.value = kind === 'bug'
        ? `No finished runs still match ${selectedBugKey.value}.`
        : `No finished runs are older than ${ageDescription(selectedAge.value)}.`
      pendingKind.value = ''
      return
    }
    const result = await deleteRuns(
      runs.map((run) => run.run_id),
      (id) => deleteRun(id),
      DELETE_CONCURRENCY,
      (progress) => {
        status.value = cleanupProgressText(progress)
      }
    )
    await refreshCatalog()
    status.value = cleanupResultText(result)
    pendingKind.value = ''
    emit('changed')
  } catch (cause) {
    status.value = `Cleanup failed: ${cause instanceof Error ? cause.message : String(cause)}`
  } finally {
    busyKind.value = ''
  }
}
</script>

<template>
  <Panel title="Bulk cleanup" description="Delete every finished run for one failure group, or every run older than a cutoff. Uses the same safe per-run delete path as a single-row delete." compact>
    <p v-if="loadError" class="mb-3 text-sm text-rose-300" role="alert">{{ loadError }}</p>

    <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
      <div class="space-y-2">
        <p class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Older than</p>
        <div class="flex flex-wrap gap-1.5">
          <button
            v-for="option in AGE_OPTIONS"
            :key="option.seconds"
            type="button"
            :aria-pressed="selectedAge === option.seconds"
            :class="[
              selectedAge === option.seconds ? 'bg-white/12 text-white' : 'bg-white/6 text-slate-300',
              'rounded-md px-2.5 py-1.5 text-[11px] font-semibold ring-1 ring-white/8 hover:bg-white/10'
            ]"
            @click="selectedAge = option.seconds"
          >
            {{ option.label }}
          </button>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            :disabled="Boolean(busyKind) || !catalog || ageCandidates.length === 0"
            class="inline-flex items-center rounded-md bg-rose-500 px-2.5 py-1.5 text-xs font-semibold text-white hover:bg-rose-400 disabled:cursor-not-allowed disabled:opacity-35"
            @click="requestAgeDelete"
          >
            {{ busyKind === 'age' ? 'Deleting…' : (ageCandidates.length ? `Delete ${ageCandidates.length} older run${ageCandidates.length === 1 ? '' : 's'}` : 'Delete older runs') }}
          </button>
          <p class="text-xs text-slate-500" aria-live="polite">
            {{ ageStatus || (catalog ? `${ageCandidates.length} finished run${ageCandidates.length === 1 ? '' : 's'} older than ${ageDescription(selectedAge)}.` : 'Loading finished history…') }}
          </p>
        </div>
      </div>

      <div class="space-y-2">
        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Failure group</span>
          <select
            v-model="selectedBugKey"
            :disabled="Boolean(busyKind) || !catalog || groups.length === 0"
            class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400"
          >
            <option value="">{{ groups.length ? 'Choose failure group…' : 'No failure groups' }}</option>
            <option v-for="group in groups" :key="group.key" :value="group.key">{{ groupOptionLabel(group) }}</option>
          </select>
        </label>
        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            :disabled="Boolean(busyKind) || !selectedBug || bugCandidates.length === 0"
            class="inline-flex items-center rounded-md bg-rose-500 px-2.5 py-1.5 text-xs font-semibold text-white hover:bg-rose-400 disabled:cursor-not-allowed disabled:opacity-35"
            @click="requestBugDelete"
          >
            {{ busyKind === 'bug' ? 'Deleting…' : (bugCandidates.length ? `Delete ${bugCandidates.length} matching run${bugCandidates.length === 1 ? '' : 's'}` : 'Delete matching runs') }}
          </button>
          <p class="text-xs text-slate-500" aria-live="polite">
            {{ bugStatus || (!catalog
              ? 'Loading failure groups…'
              : !selectedBug
                ? `${groups.length} failure group${groups.length === 1 ? '' : 's'} available.`
                : `${bugCandidates.length} finished run${bugCandidates.length === 1 ? '' : 's'} match ${selectedBug.key}.`) }}
          </p>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :open="Boolean(pendingKind)"
      :title="confirmTitle"
      :message="confirmMessage"
      :confirm-label="confirmCount ? `Delete ${confirmCount} run${confirmCount === 1 ? '' : 's'}` : 'Delete runs'"
      :busy="Boolean(busyKind)"
      danger
      @close="closePending"
      @confirm="confirmPending"
    />
  </Panel>
</template>
