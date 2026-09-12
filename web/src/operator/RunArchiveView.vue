<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ArrowLeftIcon, ArrowPathIcon, ArrowRightIcon, ArrowTopRightOnSquareIcon, TrashIcon } from '@heroicons/vue/20/solid'
import { deleteRun, getDashboard } from '../shared/api/client'
import type { DashboardRun } from '../shared/api/types'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import {
  archiveHow,
  archiveOutcome,
  archiveStarter,
  archiveWhen,
  archiveWhere,
  legacyRunURL,
  safeIssueURL
} from './runs'

const PAGE_SIZE = 25
const page = ref(0)
const filters = reactive({ outcome: '', how: '', starter: '' })
const deleteTarget = ref<DashboardRun | null>(null)
const deleting = ref(false)
const deleteError = ref('')

const resource = usePollingResource(
  (signal) => getDashboard({
    status: 'done',
    limit: PAGE_SIZE,
    offset: page.value * PAGE_SIZE,
    facets: true,
    outcome: filters.outcome,
    how: filters.how,
    starter: filters.starter
  }, signal),
  {
    intervalMs: 30000,
    isEmpty: (snapshot) => snapshot.runs.length === 0
  }
)

const rows = computed(() => resource.data.value?.runs ?? [])
const total = computed(() => Number(resource.data.value?.total || 0))
const facets = computed(() => resource.data.value?.history_facets || { outcomes: [], hows: [], starters: [] })
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const rangeStart = computed(() => total.value ? page.value * PAGE_SIZE + 1 : 0)
const rangeEnd = computed(() => Math.min(total.value, (page.value + 1) * PAGE_SIZE))
const hasFilters = computed(() => Boolean(filters.outcome || filters.how || filters.starter))
const emptyTitle = computed(() => hasFilters.value ? 'No runs match these filters' : 'Nothing finished yet')

watch(
  [page, () => filters.outcome, () => filters.how, () => filters.starter],
  () => { void resource.retry() }
)

watch(total, (value) => {
  const lastPage = Math.max(0, Math.ceil(value / PAGE_SIZE) - 1)
  if (page.value > lastPage) page.value = lastPage
})

function setFilter(key: keyof typeof filters, value: string): void {
  filters[key] = value
  page.value = 0
}

function previousPage(): void {
  if (page.value > 0) page.value--
}

function nextPage(): void {
  if (page.value + 1 < pageCount.value) page.value++
}

function retry(): void {
  void resource.retry()
}

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
    deleteTarget.value = null
    if (rows.value.length === 1 && page.value > 0) page.value--
    else await resource.retry()
  } catch (cause) {
    deleteError.value = cause instanceof Error ? cause.message : 'Delete failed'
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <div class="space-y-3">
    <Panel title="Runs" description="Completed run archive with server-side filtering and paging." compact>
      <template #actions>
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-md bg-white/8 px-2.5 py-1.5 text-xs font-semibold text-slate-200 ring-1 ring-white/10 hover:bg-white/12"
          @click="retry"
        >
          <ArrowPathIcon class="size-3.5" aria-hidden="true" />
          Refresh
        </button>
      </template>

      <div class="grid grid-cols-1 gap-2 border-b border-white/8 pb-4 sm:grid-cols-3">
        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Outcome</span>
          <select
            :value="filters.outcome"
            class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400"
            @change="setFilter('outcome', ($event.target as HTMLSelectElement).value)"
          >
            <option value="">All outcomes</option>
            <option v-for="value in facets.outcomes" :key="value" :value="value">{{ value }}</option>
          </select>
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Mode</span>
          <select
            :value="filters.how"
            class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400"
            @change="setFilter('how', ($event.target as HTMLSelectElement).value)"
          >
            <option value="">All modes</option>
            <option v-for="value in facets.hows" :key="value" :value="value">{{ value }}</option>
          </select>
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Starter</span>
          <select
            :value="filters.starter"
            class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400"
            @change="setFilter('starter', ($event.target as HTMLSelectElement).value)"
          >
            <option value="">All starters</option>
            <option v-for="value in facets.starters" :key="value" :value="value">{{ value }}</option>
          </select>
        </label>
      </div>

      <ResourceState
        :state="resource.state.value"
        :title="resource.state.value === 'empty' ? emptyTitle : 'Run archive unavailable'"
        :message="resource.error.value || (hasFilters ? 'Try a different filter combination.' : 'Completed runs will appear here.')"
        :rows="7"
      >
        <template #actions>
          <button type="button" class="rounded-md bg-white/10 px-2.5 py-1.5 text-xs font-semibold text-white ring-1 ring-white/10 hover:bg-white/15" @click="retry">
            Retry now
          </button>
        </template>

        <div class="-mx-3 mt-4 overflow-x-auto sm:-mx-4">
          <table class="min-w-full divide-y divide-white/10 text-left">
            <thead>
              <tr>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:px-4">Finished</th>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Run</th>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Setup</th>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Where</th>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Outcome</th>
                <th class="px-3 py-2 text-right text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:pr-4">Actions</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-white/8">
              <tr v-for="run in rows" :key="run.run_id" class="hover:bg-white/[0.025]">
                <td class="px-3 py-3 text-xs whitespace-nowrap text-slate-500 sm:px-4">{{ archiveWhen(run) }}</td>
                <td class="px-3 py-3">
                  <a :href="legacyRunURL(run.run_id)" class="font-mono text-xs text-cyan-200 hover:text-cyan-100" :title="run.run_id">{{ run.run_id }}</a>
                </td>
                <td class="px-3 py-3">
                  <div class="flex flex-wrap gap-1.5">
                    <StatusBadge tone="neutral">{{ archiveHow(run) }}</StatusBadge>
                    <StatusBadge tone="info">{{ archiveStarter(run) }}</StatusBadge>
                  </div>
                </td>
                <td class="px-3 py-3 text-xs whitespace-nowrap text-slate-400">{{ archiveWhere(run) }}</td>
                <td class="max-w-md px-3 py-3">
                  <div class="flex items-center gap-2">
                    <a
                      v-if="run.issue?.issue_number && safeIssueURL(run)"
                      :href="safeIssueURL(run)"
                      target="_blank"
                      rel="noopener"
                      class="shrink-0 text-[11px] font-semibold text-amber-200 hover:text-amber-100"
                    >
                      #{{ run.issue.issue_number }}
                    </a>
                    <span class="truncate text-xs text-slate-300" :title="archiveOutcome(run)">{{ archiveOutcome(run) }}</span>
                  </div>
                </td>
                <td class="px-3 py-3 sm:pr-4">
                  <div class="flex justify-end gap-1.5">
                    <a
                      :href="legacyRunURL(run.run_id)"
                      class="inline-flex items-center gap-1 rounded-md bg-white/6 px-2 py-1.5 text-[11px] font-semibold text-slate-300 ring-1 ring-white/8 hover:bg-white/10 hover:text-white"
                      title="Open in the existing Live inspector"
                    >
                      <ArrowTopRightOnSquareIcon class="size-3.5" aria-hidden="true" />
                      Inspect
                    </a>
                    <button
                      type="button"
                      class="inline-flex items-center gap-1 rounded-md bg-rose-400/8 px-2 py-1.5 text-[11px] font-semibold text-rose-200 ring-1 ring-rose-300/15 hover:bg-rose-400/15"
                      @click="requestDelete(run)"
                    >
                      <TrashIcon class="size-3.5" aria-hidden="true" />
                      Delete
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </ResourceState>

      <div class="mt-4 flex flex-col gap-3 border-t border-white/8 pt-4 sm:flex-row sm:items-center sm:justify-between">
        <p class="text-xs text-slate-500">{{ rangeStart }}–{{ rangeEnd }} of {{ total }}</p>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="inline-flex items-center gap-1 rounded-md bg-white/6 px-2.5 py-1.5 text-xs font-semibold text-slate-300 ring-1 ring-white/8 hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-35"
            :disabled="page === 0"
            @click="previousPage"
          >
            <ArrowLeftIcon class="size-3.5" aria-hidden="true" />
            Previous
          </button>
          <span class="min-w-16 text-center font-mono text-[11px] text-slate-500">{{ page + 1 }} / {{ pageCount }}</span>
          <button
            type="button"
            class="inline-flex items-center gap-1 rounded-md bg-white/6 px-2.5 py-1.5 text-xs font-semibold text-slate-300 ring-1 ring-white/8 hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-35"
            :disabled="page + 1 >= pageCount"
            @click="nextPage"
          >
            Next
            <ArrowRightIcon class="size-3.5" aria-hidden="true" />
          </button>
        </div>
      </div>
    </Panel>

    <div v-if="deleteError" class="border-l-4 border-rose-400 bg-rose-400/10 p-3 text-sm text-rose-100" role="alert">
      {{ deleteError }}
    </div>

    <ConfirmDialog
      :open="Boolean(deleteTarget)"
      title="Delete this run?"
      :message="deleteTarget ? `Delete ${deleteTarget.run_id} and its associated stored run data? This cannot be undone.` : ''"
      confirm-label="Delete run"
      :busy="deleting"
      danger
      @close="closeDelete"
      @confirm="confirmDelete"
    />
  </div>
</template>
