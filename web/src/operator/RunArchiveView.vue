<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ArrowLeftIcon, ArrowPathIcon, ArrowRightIcon, ArrowTopRightOnSquareIcon, TrashIcon } from '@heroicons/vue/20/solid'
import { deleteRun, getDashboard } from '../shared/api/client'
import type { DashboardQuery, DashboardRun } from '../shared/api/types'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import RunCleanupPanel from './RunCleanupPanel.vue'
import {
  archiveHow,
  archiveOutcome,
  archiveStarter,
  archiveWhen,
  archiveWhere,
  legacyRunURL,
  safeIssueURL
} from './runs'
import { isPlayStyleRun, playStyleLabel } from '../shared/playstyle'

const PAGE_SIZE = 25

type StringFilterKey = 'outcome' | 'how' | 'starter' | 'model' | 'deployment' | 'goal' | 'playStyle' | 'compute' | 'experiment'

interface ArchiveDashboardQuery extends DashboardQuery {
  model?: string
  deployment?: string
  goal?: string
  play_style?: string
  compute?: string
  experiment?: string
  success?: boolean
  sort?: string
  direction?: string
}

interface ArchiveFacets {
  outcomes: string[]
  hows: string[]
  starters: string[]
  models: string[]
  deployments: string[]
  goals: string[]
  play_styles: string[]
  computes: string[]
  experiments: string[]
}

const page = ref(0)
const filters = reactive({
  outcome: '',
  how: '',
  starter: '',
  model: '',
  deployment: '',
  goal: '',
  playStyle: '',
  compute: '',
  experiment: ''
})
const successOnly = ref(false)
const sortKey = ref('finished')
const sortDirection = ref('desc')
const expanded = ref<Set<string>>(new Set())
const deleteTarget = ref<DashboardRun | null>(null)
const deleting = ref(false)
const deleteError = ref('')

const resource = usePollingResource(
  (signal) => {
    const query: ArchiveDashboardQuery = {
      status: 'done',
      limit: PAGE_SIZE,
      offset: page.value * PAGE_SIZE,
      facets: true,
      outcome: filters.outcome,
      how: filters.how,
      starter: filters.starter,
      model: filters.model,
      deployment: filters.deployment,
      goal: filters.goal,
      play_style: filters.playStyle,
      compute: filters.compute,
      experiment: filters.experiment,
      success: successOnly.value || undefined,
      sort: sortKey.value,
      direction: sortDirection.value
    }
    return getDashboard(query, signal)
  },
  {
    intervalMs: 30000,
    isEmpty: (snapshot) => snapshot.runs.length === 0
  }
)

const rows = computed(() => resource.data.value?.runs ?? [])
const total = computed(() => Number(resource.data.value?.total || 0))
const facets = computed<ArchiveFacets>(() => {
  const raw = resource.data.value?.history_facets as unknown as Partial<ArchiveFacets> | undefined
  return {
    outcomes: raw?.outcomes ?? [],
    hows: raw?.hows ?? [],
    starters: raw?.starters ?? [],
    models: raw?.models ?? [],
    deployments: raw?.deployments ?? [],
    goals: raw?.goals ?? [],
    play_styles: raw?.play_styles ?? [],
    computes: raw?.computes ?? [],
    experiments: raw?.experiments ?? []
  }
})
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const rangeStart = computed(() => total.value ? page.value * PAGE_SIZE + 1 : 0)
const rangeEnd = computed(() => Math.min(total.value, (page.value + 1) * PAGE_SIZE))
const hasFilters = computed(() => Boolean(
  filters.outcome || filters.how || filters.starter || filters.model || filters.deployment ||
  filters.goal || filters.playStyle || filters.compute || filters.experiment || successOnly.value
))
const emptyTitle = computed(() => hasFilters.value ? 'No runs match these filters' : 'Nothing finished yet')

watch(
  [
    page,
    () => filters.outcome,
    () => filters.how,
    () => filters.starter,
    () => filters.model,
    () => filters.deployment,
    () => filters.goal,
    () => filters.playStyle,
    () => filters.compute,
    () => filters.experiment,
    successOnly,
    sortKey,
    sortDirection
  ],
  () => { void resource.retry() }
)

watch(total, (value) => {
  const lastPage = Math.max(0, Math.ceil(value / PAGE_SIZE) - 1)
  if (page.value > lastPage) page.value = lastPage
})

function setFilter(key: StringFilterKey, value: string): void {
  filters[key] = value
  page.value = 0
}

function clearFilters(): void {
  for (const key of Object.keys(filters) as StringFilterKey[]) filters[key] = ''
  successOnly.value = false
  page.value = 0
}

function fastestSuccessful(): void {
  successOnly.value = true
  sortKey.value = 'frames'
  sortDirection.value = 'asc'
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

function toggleExpanded(runID: string): void {
  const next = new Set(expanded.value)
  if (next.has(runID)) next.delete(runID)
  else next.add(runID)
  expanded.value = next
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

function modelName(run: DashboardRun): string {
  return run.inference?.label || run.inference?.model_id || String(run.stats?.model || run.llm_profile || '—')
}

function modelID(run: DashboardRun): string {
  return run.inference?.model_id || String(run.stats?.model || run.llm_profile || '')
}

function deploymentName(run: DashboardRun): string {
  return run.llm_deployment || run.inference?.deployment_id || ''
}

function computeName(run: DashboardRun): string {
  return run.inference?.compute || ''
}

function statNumber(run: DashboardRun, key: string): number {
  return Number(run.stats?.[key] || 0)
}

function plannerSeconds(run: DashboardRun): number {
  const strategic = statNumber(run, 'strategic_seconds')
  if (strategic > 0) return strategic
  const average = statNumber(run, 'avg_seconds')
  const calls = statNumber(run, 'calls')
  return average > 0 && calls > 0 ? average * calls : 0
}

function runtimeSeconds(run: DashboardRun): number {
  return Number(run.runtime_seconds || 0)
}

function rounds(run: DashboardRun): number {
  return statNumber(run, 'rounds') || statNumber(run, 'round')
}

function strategicCalls(run: DashboardRun): number {
  return statNumber(run, 'strategic_calls') || statNumber(run, 'calls')
}

function formatInteger(value: number): string {
  return Number.isFinite(value) ? Math.round(value).toLocaleString() : '—'
}

function formatSeconds(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '—'
  if (value < 60) return `${value < 10 ? value.toFixed(2) : value.toFixed(1)}s`
  const minutes = Math.floor(value / 60)
  const seconds = Math.round(value % 60)
  if (minutes < 60) return `${minutes}m ${seconds}s`
  const hours = Math.floor(minutes / 60)
  return `${hours}h ${minutes % 60}m`
}

function progressLabel(run: DashboardRun): string {
  const current = statNumber(run, 'goal_current')
  const target = statNumber(run, 'goal_target')
  if (target > 0) return `${formatInteger(current)}/${formatInteger(target)}`
  const badges = run.player?.badges?.length || 0
  if (badges > 0) return `${badges} badge${badges === 1 ? '' : 's'}`
  if (run.stats?.goal_complete) return 'Goal complete'
  return 'No structured progress'
}

function partySummary(run: DashboardRun): string {
  const party = run.player?.party || []
  if (!party.length) return '—'
  return party.map((mon) => `${mon.name} L${mon.level}`).join(' · ')
}

function experimentLabel(run: DashboardRun): string {
  if (!run.experiment_id) return ''
  return run.experiment_arm ? `${run.experiment_id} · ${run.experiment_arm}` : run.experiment_id
}
</script>

<template>
  <div class="space-y-3">
    <RunCleanupPanel @changed="retry" />

    <Panel title="Runs" description="Finished-run database and lightweight model leaderboard. Filters and sorting apply across the full server-side archive." compact>
      <template #actions>
        <div class="flex flex-wrap items-center justify-end gap-2">
          <button
            type="button"
            class="rounded-md bg-cyan-400/10 px-2.5 py-1.5 text-xs font-semibold text-cyan-100 ring-1 ring-cyan-300/20 hover:bg-cyan-400/15"
            @click="fastestSuccessful"
          >
            Fastest successful
          </button>
          <button
            v-if="hasFilters"
            type="button"
            class="rounded-md bg-white/6 px-2.5 py-1.5 text-xs font-semibold text-slate-300 ring-1 ring-white/8 hover:bg-white/10"
            @click="clearFilters"
          >
            Clear filters
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md bg-white/8 px-2.5 py-1.5 text-xs font-semibold text-slate-200 ring-1 ring-white/10 hover:bg-white/12"
            @click="retry"
          >
            <ArrowPathIcon class="size-3.5" aria-hidden="true" />
            Refresh
          </button>
        </div>
      </template>

      <div class="grid grid-cols-1 gap-2 border-b border-white/8 pb-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Outcome</span>
          <select :value="filters.outcome" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" @change="setFilter('outcome', ($event.target as HTMLSelectElement).value)">
            <option value="">All outcomes</option>
            <option v-for="value in facets.outcomes" :key="value" :value="value">{{ value }}</option>
          </select>
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Model</span>
          <select :value="filters.model" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" @change="setFilter('model', ($event.target as HTMLSelectElement).value)">
            <option value="">All models</option>
            <option v-for="value in facets.models" :key="value" :value="value">{{ value }}</option>
          </select>
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Deployment</span>
          <select :value="filters.deployment" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" @change="setFilter('deployment', ($event.target as HTMLSelectElement).value)">
            <option value="">All deployments</option>
            <option v-for="value in facets.deployments" :key="value" :value="value">{{ value }}</option>
          </select>
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Goal</span>
          <select :value="filters.goal" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" @change="setFilter('goal', ($event.target as HTMLSelectElement).value)">
            <option value="">All goals</option>
            <option v-for="value in facets.goals" :key="value" :value="value">{{ value }}</option>
          </select>
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Play style</span>
          <select :value="filters.playStyle" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" @change="setFilter('playStyle', ($event.target as HTMLSelectElement).value)">
            <option value="">All styles</option>
            <option v-for="value in facets.play_styles" :key="value" :value="value">{{ value }}</option>
          </select>
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Mode</span>
          <select :value="filters.how" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" @change="setFilter('how', ($event.target as HTMLSelectElement).value)">
            <option value="">All modes</option>
            <option v-for="value in facets.hows" :key="value" :value="value">{{ value }}</option>
          </select>
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Starter</span>
          <select :value="filters.starter" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" @change="setFilter('starter', ($event.target as HTMLSelectElement).value)">
            <option value="">All starters</option>
            <option v-for="value in facets.starters" :key="value" :value="value">{{ value }}</option>
          </select>
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Compute</span>
          <select :value="filters.compute" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" @change="setFilter('compute', ($event.target as HTMLSelectElement).value)">
            <option value="">All compute</option>
            <option v-for="value in facets.computes" :key="value" :value="value">{{ value }}</option>
          </select>
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Experiment</span>
          <select :value="filters.experiment" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" @change="setFilter('experiment', ($event.target as HTMLSelectElement).value)">
            <option value="">All experiments</option>
            <option v-for="value in facets.experiments" :key="value" :value="value">{{ value }}</option>
          </select>
        </label>

        <label class="flex items-end gap-2 rounded-md bg-white/[0.025] px-2.5 py-2 ring-1 ring-white/8">
          <input v-model="successOnly" type="checkbox" class="size-4 rounded border-white/15 bg-white/8 text-cyan-400 focus:ring-cyan-400" @change="page = 0">
          <span class="pb-0.5 text-xs font-semibold text-slate-300">Success only</span>
        </label>
      </div>

      <div class="grid grid-cols-1 gap-2 border-b border-white/8 py-3 sm:grid-cols-2 lg:max-w-2xl">
        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Sort</span>
          <select v-model="sortKey" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" @change="page = 0">
            <option value="finished">Finished time</option>
            <option value="frames">Emulated frames</option>
            <option value="rounds">Rounds</option>
            <option value="planner_time">Planner time</option>
            <option value="runtime">Real runtime</option>
            <option value="progress">Progress</option>
            <option value="strategic_calls">Strategic calls</option>
          </select>
        </label>
        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Direction</span>
          <select v-model="sortDirection" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-2.5 py-2 text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" @change="page = 0">
            <option value="asc">Ascending ↑</option>
            <option value="desc">Descending ↓</option>
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
          <table class="min-w-[1180px] divide-y divide-white/10 text-left">
            <thead>
              <tr>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:px-4">Finished</th>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Run / setup</th>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Progress</th>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Model</th>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Performance</th>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Outcome</th>
                <th class="px-3 py-2 text-right text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:pr-4">Actions</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-white/8">
              <template v-for="run in rows" :key="run.run_id">
                <tr class="align-top hover:bg-white/[0.025]">
                  <td class="px-3 py-3 text-xs whitespace-nowrap text-slate-500 sm:px-4">
                    <div>{{ archiveWhen(run) }}</div>
                    <div class="mt-1 font-mono text-[10px] text-slate-600">{{ formatSeconds(runtimeSeconds(run)) }} runtime</div>
                  </td>
                  <td class="max-w-xs px-3 py-3">
                    <a :href="legacyRunURL(run.run_id)" class="font-mono text-xs text-cyan-200 hover:text-cyan-100" :title="run.run_id">{{ run.run_id }}</a>
                    <div class="mt-1.5 flex flex-wrap gap-1.5">
                      <StatusBadge tone="neutral">{{ archiveHow(run) }}</StatusBadge>
                      <button v-if="isPlayStyleRun(run)" type="button" @click="setFilter('playStyle', run.play_style || '')">
                        <StatusBadge tone="info">{{ playStyleLabel(run) }}</StatusBadge>
                      </button>
                      <button type="button" @click="setFilter('starter', archiveStarter(run))">
                        <StatusBadge tone="info">{{ archiveStarter(run) }}</StatusBadge>
                      </button>
                    </div>
                    <button v-if="run.goal" type="button" class="mt-1.5 block max-w-full truncate text-left text-[11px] text-slate-500 hover:text-slate-300" :title="run.goal" @click="setFilter('goal', run.goal || '')">
                      {{ run.goal }}
                    </button>
                    <div class="mt-1 font-mono text-[10px] text-slate-600">seed {{ run.seed ?? '—' }}</div>
                  </td>
                  <td class="max-w-xs px-3 py-3">
                    <div class="text-xs font-semibold text-slate-200">{{ progressLabel(run) }}</div>
                    <div v-if="run.player?.badges?.length" class="mt-1 text-[11px] text-emerald-200">{{ run.player.badges.join(' · ') }}</div>
                    <div v-if="run.stats?.goal_summary" class="mt-1 line-clamp-2 text-[11px] text-slate-500" :title="run.stats.goal_summary">{{ run.stats.goal_summary }}</div>
                    <div class="mt-1 text-[11px] text-slate-500">{{ archiveWhere(run) }}</div>
                  </td>
                  <td class="max-w-xs px-3 py-3">
                    <button v-if="modelID(run)" type="button" class="block max-w-full truncate text-left text-xs font-semibold text-slate-200 hover:text-cyan-100" :title="modelID(run)" @click="setFilter('model', modelID(run))">
                      {{ modelName(run) }}
                    </button>
                    <span v-else class="text-xs text-slate-500">—</span>
                    <button v-if="deploymentName(run)" type="button" class="mt-1 block max-w-full truncate font-mono text-[10px] text-cyan-300/80 hover:text-cyan-200" :title="deploymentName(run)" @click="setFilter('deployment', deploymentName(run))">
                      {{ deploymentName(run) }}
                    </button>
                    <button v-if="computeName(run)" type="button" class="mt-1 text-[10px] text-slate-500 hover:text-slate-300" @click="setFilter('compute', computeName(run))">
                      {{ computeName(run) }}
                    </button>
                    <button v-if="experimentLabel(run)" type="button" class="mt-1 block max-w-full truncate text-[10px] text-violet-300/80 hover:text-violet-200" :title="experimentLabel(run)" @click="setFilter('experiment', run.experiment_id || '')">
                      {{ experimentLabel(run) }}
                    </button>
                  </td>
                  <td class="px-3 py-3 text-[11px] text-slate-400">
                    <div><span class="text-slate-600">Rounds</span> {{ formatInteger(rounds(run)) }}</div>
                    <div><span class="text-slate-600">Frames</span> {{ formatInteger(Number(run.frame || 0)) }}</div>
                    <div><span class="text-slate-600">Strategic</span> {{ formatInteger(strategicCalls(run)) }} calls</div>
                    <div><span class="text-slate-600">Planner</span> {{ formatSeconds(plannerSeconds(run)) }}</div>
                  </td>
                  <td class="max-w-sm px-3 py-3">
                    <div class="flex items-center gap-2">
                      <a v-if="run.issue?.issue_number && safeIssueURL(run)" :href="safeIssueURL(run)" target="_blank" rel="noopener" class="shrink-0 text-[11px] font-semibold text-amber-200 hover:text-amber-100">#{{ run.issue.issue_number }}</a>
                      <StatusBadge :tone="run.reason === 'done' || run.stats?.goal_complete ? 'success' : 'neutral'">{{ run.reason || 'done' }}</StatusBadge>
                    </div>
                    <p v-if="run.detail" class="mt-1.5 line-clamp-2 text-xs text-slate-400" :title="archiveOutcome(run)">{{ run.detail }}</p>
                  </td>
                  <td class="px-3 py-3 sm:pr-4">
                    <div class="flex justify-end gap-1.5">
                      <button type="button" class="rounded-md bg-white/6 px-2 py-1.5 text-[11px] font-semibold text-slate-300 ring-1 ring-white/8 hover:bg-white/10 hover:text-white" @click="toggleExpanded(run.run_id)">
                        {{ expanded.has(run.run_id) ? 'Less' : 'Details' }}
                      </button>
                      <a :href="legacyRunURL(run.run_id)" class="inline-flex items-center gap-1 rounded-md bg-white/6 px-2 py-1.5 text-[11px] font-semibold text-slate-300 ring-1 ring-white/8 hover:bg-white/10 hover:text-white" title="Open in the existing Live inspector">
                        <ArrowTopRightOnSquareIcon class="size-3.5" aria-hidden="true" />
                        Inspect
                      </a>
                      <button type="button" class="inline-flex items-center gap-1 rounded-md bg-rose-400/8 px-2 py-1.5 text-[11px] font-semibold text-rose-200 ring-1 ring-rose-300/15 hover:bg-rose-400/15" @click="requestDelete(run)">
                        <TrashIcon class="size-3.5" aria-hidden="true" />
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
                <tr v-if="expanded.has(run.run_id)" class="bg-white/[0.018]">
                  <td colspan="7" class="px-3 py-4 sm:px-4">
                    <div class="grid gap-x-8 gap-y-4 text-xs sm:grid-cols-2 xl:grid-cols-4">
                      <dl class="space-y-1.5">
                        <div class="flex gap-3"><dt class="w-24 shrink-0 text-slate-600">Goal</dt><dd class="text-slate-300">{{ run.goal || '—' }}</dd></div>
                        <div class="flex gap-3"><dt class="w-24 shrink-0 text-slate-600">Result</dt><dd class="text-slate-300">{{ run.reason || 'done' }}</dd></div>
                        <div class="flex gap-3"><dt class="w-24 shrink-0 text-slate-600">Progress</dt><dd class="text-slate-300">{{ progressLabel(run) }}</dd></div>
                        <div class="flex gap-3"><dt class="w-24 shrink-0 text-slate-600">Location</dt><dd class="text-slate-300">{{ archiveWhere(run) }}</dd></div>
                        <div class="flex gap-3"><dt class="w-24 shrink-0 text-slate-600">Frames</dt><dd class="font-mono text-slate-300">{{ formatInteger(Number(run.frame || 0)) }}</dd></div>
                        <div class="flex gap-3"><dt class="w-24 shrink-0 text-slate-600">Rounds</dt><dd class="font-mono text-slate-300">{{ formatInteger(rounds(run)) }}</dd></div>
                        <div class="flex gap-3"><dt class="w-24 shrink-0 text-slate-600">Runtime</dt><dd class="font-mono text-slate-300">{{ formatSeconds(runtimeSeconds(run)) }}</dd></div>
                      </dl>

                      <dl class="space-y-1.5">
                        <div class="flex gap-3"><dt class="w-24 shrink-0 text-slate-600">Model</dt><dd class="text-slate-300">{{ modelName(run) }}</dd></div>
                        <div class="flex gap-3"><dt class="w-24 shrink-0 text-slate-600">Deployment</dt><dd class="font-mono text-slate-300">{{ deploymentName(run) || '—' }}</dd></div>
                        <div class="flex gap-3"><dt class="w-24 shrink-0 text-slate-600">Compute</dt><dd class="text-slate-300">{{ computeName(run) || '—' }}</dd></div>
                        <div class="flex gap-3"><dt class="w-24 shrink-0 text-slate-600">Quantization</dt><dd class="text-slate-300">{{ run.inference?.quantization || '—' }}</dd></div>
                        <div class="flex gap-3"><dt class="w-24 shrink-0 text-slate-600">Reasoning</dt><dd class="text-slate-300">{{ run.reasoning_effort || 'default' }}</dd></div>
                        <div class="flex gap-3"><dt class="w-24 shrink-0 text-slate-600">Seed</dt><dd class="font-mono text-slate-300">{{ run.seed ?? '—' }}</dd></div>
                        <div class="flex gap-3"><dt class="w-24 shrink-0 text-slate-600">Experiment</dt><dd class="text-slate-300">{{ experimentLabel(run) || '—' }}</dd></div>
                      </dl>

                      <dl class="space-y-1.5">
                        <div class="flex gap-3"><dt class="w-28 shrink-0 text-slate-600">Strategic calls</dt><dd class="font-mono text-slate-300">{{ formatInteger(strategicCalls(run)) }}</dd></div>
                        <div class="flex gap-3"><dt class="w-28 shrink-0 text-slate-600">Plans executed</dt><dd class="font-mono text-slate-300">{{ formatInteger(statNumber(run, 'plan_executions')) }}</dd></div>
                        <div class="flex gap-3"><dt class="w-28 shrink-0 text-slate-600">Steps skipped</dt><dd class="font-mono text-slate-300">{{ formatInteger(statNumber(run, 'steps_skipped')) }}</dd></div>
                        <div class="flex gap-3"><dt class="w-28 shrink-0 text-slate-600">Rejected</dt><dd class="font-mono text-slate-300">{{ formatInteger(statNumber(run, 'rejected')) }}</dd></div>
                        <div class="flex gap-3"><dt class="w-28 shrink-0 text-slate-600">Planner time</dt><dd class="font-mono text-slate-300">{{ formatSeconds(plannerSeconds(run)) }}</dd></div>
                        <div class="flex gap-3"><dt class="w-28 shrink-0 text-slate-600">Prompt tokens</dt><dd class="font-mono text-slate-300">{{ formatInteger(statNumber(run, 'prompt_tokens')) }}</dd></div>
                        <div class="flex gap-3"><dt class="w-28 shrink-0 text-slate-600">Completion tokens</dt><dd class="font-mono text-slate-300">{{ formatInteger(statNumber(run, 'completion_tokens')) }}</dd></div>
                      </dl>

                      <dl class="space-y-1.5">
                        <div><dt class="text-slate-600">Party</dt><dd class="mt-1 text-slate-300">{{ partySummary(run) }}</dd></div>
                        <div><dt class="text-slate-600">Milestones</dt><dd class="mt-1 text-slate-300">{{ run.player?.milestones?.join(' · ') || '—' }}</dd></div>
                        <div><dt class="text-slate-600">Failure / stop detail</dt><dd class="mt-1 whitespace-pre-wrap text-slate-300">{{ run.detail || '—' }}</dd></div>
                      </dl>
                    </div>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </ResourceState>

      <div class="mt-4 flex flex-col gap-3 border-t border-white/8 pt-4 sm:flex-row sm:items-center sm:justify-between">
        <p class="text-xs text-slate-500">{{ rangeStart }}–{{ rangeEnd }} of {{ total }}</p>
        <div class="flex items-center gap-2">
          <button type="button" class="inline-flex items-center gap-1 rounded-md bg-white/6 px-2.5 py-1.5 text-xs font-semibold text-slate-300 ring-1 ring-white/8 hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-35" :disabled="page === 0" @click="previousPage">
            <ArrowLeftIcon class="size-3.5" aria-hidden="true" />
            Previous
          </button>
          <span class="min-w-16 text-center font-mono text-[11px] text-slate-500">{{ page + 1 }} / {{ pageCount }}</span>
          <button type="button" class="inline-flex items-center gap-1 rounded-md bg-white/6 px-2.5 py-1.5 text-xs font-semibold text-slate-300 ring-1 ring-white/8 hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-35" :disabled="page + 1 >= pageCount" @click="nextPage">
            Next
            <ArrowRightIcon class="size-3.5" aria-hidden="true" />
          </button>
        </div>
      </div>
    </Panel>

    <p v-if="deleteError" class="text-sm text-rose-300" role="alert">{{ deleteError }}</p>

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
