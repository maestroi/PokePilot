<script setup lang="ts">
import { computed } from 'vue'
import { ArrowPathIcon } from '@heroicons/vue/20/solid'
import { getDashboard } from '../shared/api/client'
import type { DashboardQuery, DashboardRun } from '../shared/api/types'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { operatorHref } from '../shared/urls'

interface LeaderboardQuery extends DashboardQuery {
  success: boolean
  sort: string
  direction: string
}

const leaderboard = usePollingResource(
  (signal) => {
    const query: LeaderboardQuery = { status: 'done', success: true, sort: 'frames', direction: 'asc', limit: 10 }
    return getDashboard(query, signal)
  },
  { intervalMs: 30000, isEmpty: (snapshot) => snapshot.runs.length === 0 }
)
const rows = computed<DashboardRun[]>(() => leaderboard.data.value?.runs ?? [])
function nfmt(value: number | undefined): string {
  return Number(value || 0).toLocaleString()
}
function modelLabel(run: DashboardRun): string {
  return run.inference?.label || run.inference?.model_id || run.llm_deployment || run.llm_profile || 'Unspecified'
}
function operatorSearch(): string { return window.location.search }
function refresh(): void { void leaderboard.retry() }
</script>

<template>
  <Panel title="Fastest successful runs" description="Server-ranked completed goal wins, ordered by fewest frames. Inspect each exact run in Live." compact>
    <template #actions>
      <button type="button" class="inline-flex items-center gap-1 rounded-sm px-2 py-1.5 text-xs font-semibold text-slate-200 ring-1 ring-white/10 hover:bg-white/10 focus-visible:outline-2 focus-visible:outline-cyan-300" @click="refresh">
        <ArrowPathIcon class="size-3.5" aria-hidden="true" />Refresh
      </button>
    </template>
    <ResourceState :state="leaderboard.state.value" title="No successful runs yet" :message="leaderboard.error.value || 'The leaderboard fills in after a run reaches its goal and settles.'" :rows="4">
      <template #actions><button type="button" class="text-xs font-semibold text-cyan-200 hover:underline" @click="refresh">Retry leaderboard</button></template>
      <div class="-mx-2.5 overflow-x-auto">
        <table class="min-w-full divide-y divide-white/8 text-left text-xs">
          <thead><tr class="text-[10px] text-slate-500"><th class="px-3 py-2">Rank</th><th class="px-3 py-2">Run</th><th class="px-3 py-2">Goal</th><th class="px-3 py-2">Model</th><th class="px-3 py-2 text-right">Frames</th><th class="px-3 py-2 text-right">Rounds</th></tr></thead>
          <tbody class="divide-y divide-white/8">
            <tr v-for="(run, index) in rows" :key="run.run_id">
              <td class="px-3 py-2 font-mono text-slate-500">{{ index + 1 }}</td>
              <td class="px-3 py-2"><a :href="operatorHref('live', { run: run.run_id }, operatorSearch())" :title="run.run_id" class="inline-block max-w-40 truncate font-mono text-cyan-200 hover:underline focus-visible:outline-2 focus-visible:outline-cyan-300">{{ run.run_id }}</a></td>
              <td class="max-w-56 truncate px-3 py-2 text-slate-400" :title="run.goal || ''">{{ run.goal || 'Free play' }}</td>
              <td class="max-w-40 truncate px-3 py-2 text-slate-400" :title="modelLabel(run)">{{ modelLabel(run) }}</td>
              <td class="px-3 py-2 text-right font-mono text-slate-300">{{ nfmt(run.frame) }}</td>
              <td class="px-3 py-2 text-right font-mono text-slate-400">{{ nfmt(Number(run.stats?.rounds ?? run.stats?.round ?? 0)) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </ResourceState>
  </Panel>
</template>
