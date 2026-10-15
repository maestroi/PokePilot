<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { deleteRun, getDashboard, getTriage } from '../shared/api/client'
import type { DashboardSnapshot, TriageGroup } from '../shared/api/types'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
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
  isResolvedGroup,
  lineageBlockedRuns
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
const ageBlocked = computed(() => lineageBlockedRuns(catalog.value?.runs, Number(catalog.value?.now || 0), selectedAge.value))
const bugCandidates = computed(() => selectedBug.value ? bugGroupRuns(catalog.value?.runs, groupPattern(selectedBug.value)) : [])
const confirmCount = computed(() => pendingKind.value === 'bug' ? bugCandidates.value.length : ageCandidates.value.length)
const confirmTitle = computed(() => {
  if (busyKind.value) return 'Deleting runs'
  return pendingKind.value === 'bug' ? 'Delete matching failure runs?' : 'Delete older runs?'
})
const confirmMessage = computed(() => {
  if (busyKind.value) return pendingKind.value === 'bug' ? bugStatus.value : ageStatus.value
  const kept = ageBlocked.value.length
    ? ` ${ageBlocked.value.length} older run${ageBlocked.value.length === 1 ? '' : 's'} stay because a live endless successor can still resume from them.`
    : ''
  if (pendingKind.value === 'bug' && selectedBug.value) {
    return `Delete ${bugCandidates.value.length} finished run${bugCandidates.value.length === 1 ? '' : 's'} for ${selectedBug.value.key} that are not on a live resume chain? Pattern: ${groupPattern(selectedBug.value)}. This also permanently deletes their S3 artifacts and replay cache.`
  }
  if (pendingKind.value === 'age') {
    return `Delete ${ageCandidates.value.length} finished run${ageCandidates.value.length === 1 ? '' : 's'} older than ${ageDescription(selectedAge.value)} that are not on a live resume chain?${kept} This also permanently deletes their S3 artifacts and replay cache.`
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
  <div class="space-y-6">
    <p class="max-w-[72ch] text-fg-2">Each run is deleted through the same safe per-run path as a single delete. Runs a live endless successor can still resume from are kept.</p>
    <p v-if="loadError" class="border border-danger/40 bg-danger/10 px-3 py-2 text-danger" role="alert">✕ {{ loadError }}</p>

    <section class="space-y-2" aria-labelledby="cleanup-age">
      <h3 id="cleanup-age" class="text-title font-semibold">Older than</h3>
      <div class="inline-flex flex-wrap border border-rule-strong" role="group" aria-label="Age cutoff">
        <button
          v-for="option in AGE_OPTIONS"
          :key="option.seconds"
          type="button"
          :aria-pressed="selectedAge === option.seconds"
          :class="selectedAge === option.seconds ? 'bg-selected text-fg' : 'text-fg-2 hover:bg-raised hover:text-fg'"
          class="h-7 border-r border-rule-strong px-3 last:border-r-0"
          @click="selectedAge = option.seconds"
        >
          {{ option.label }}
        </button>
      </div>
      <p class="text-fg-2" aria-live="polite">
        {{ ageStatus || (catalog
          ? (ageCandidates.length
            ? `${ageCandidates.length} finished run${ageCandidates.length === 1 ? '' : 's'} older than ${ageDescription(selectedAge)} can be deleted${ageBlocked.length ? `; ${ageBlocked.length} stay for live resume` : ''}.`
            : (ageBlocked.length
              ? `All ${ageBlocked.length} runs older than ${ageDescription(selectedAge)} are still required by a live endless successor.`
              : `No finished runs are older than ${ageDescription(selectedAge)}.`))
          : 'Loading finished history…') }}
      </p>
      <button
        type="button"
        :disabled="Boolean(busyKind) || !catalog || ageCandidates.length === 0"
        class="ui-btn ui-btn-danger"
        @click="requestAgeDelete"
      >
        {{ busyKind === 'age' ? 'Deleting…' : (ageCandidates.length ? `Delete ${ageCandidates.length} older run${ageCandidates.length === 1 ? '' : 's'}…` : 'Delete older runs…') }}
      </button>
    </section>

    <section class="space-y-2 border-t border-rule pt-5" aria-labelledby="cleanup-group">
      <h3 id="cleanup-group" class="text-title font-semibold">Failure group</h3>
      <label class="block">
        <span class="sr-only">Failure group</span>
        <select
          v-model="selectedBugKey"
          :disabled="Boolean(busyKind) || !catalog || groups.length === 0"
          class="block h-8 w-full rounded-[3px] border border-control bg-canvas px-2 text-fg disabled:text-fg-3"
        >
          <option value="">{{ groups.length ? 'Choose failure group…' : 'No failure groups' }}</option>
          <option v-for="group in groups" :key="group.key" :value="group.key">{{ groupOptionLabel(group) }}</option>
        </select>
      </label>
      <p class="text-fg-2" aria-live="polite">
        {{ bugStatus || (!catalog
          ? 'Loading failure groups…'
          : !selectedBug
            ? `${groups.length} failure group${groups.length === 1 ? '' : 's'} available.`
            : `${bugCandidates.length} finished run${bugCandidates.length === 1 ? '' : 's'} match ${selectedBug.key}.`) }}
      </p>
      <button
        type="button"
        :disabled="Boolean(busyKind) || !selectedBug || bugCandidates.length === 0"
        class="ui-btn ui-btn-danger"
        @click="requestBugDelete"
      >
        {{ busyKind === 'bug' ? 'Deleting…' : (bugCandidates.length ? `Delete ${bugCandidates.length} matching run${bugCandidates.length === 1 ? '' : 's'}…` : 'Delete matching runs…') }}
      </button>
    </section>

    <ConfirmDialog
      :open="Boolean(pendingKind)"
      :title="confirmTitle"
      :message="confirmMessage"
      :confirm-label="confirmCount ? `Delete ${confirmCount} run${confirmCount === 1 ? '' : 's'}` : 'Delete runs'"
      :require-text="confirmCount > 10 && !busyKind ? String(confirmCount) : ''"
      :busy="Boolean(busyKind)"
      danger
      @close="closePending"
      @confirm="confirmPending"
    />
  </div>
</template>
