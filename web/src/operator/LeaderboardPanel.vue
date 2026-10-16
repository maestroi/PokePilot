<script setup lang="ts">
import { computed } from 'vue'
import { getDashboard } from '../shared/api/client'
import type { DashboardQuery, DashboardRun } from '../shared/api/types'
import ResourceState from '../shared/components/ResourceState.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { operatorHref } from '../shared/urls'
import { middleTruncate } from './runs'

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
  return Number(value || 0).toLocaleString('en-US')
}
function modelLabel(run: DashboardRun): string {
  return run.inference?.label || run.inference?.model_id || run.llm_deployment || run.llm_profile || 'Unspecified'
}
function operatorSearch(): string { return window.location.search }
</script>

<template>
  <section class="ui-bay" aria-labelledby="leaderboard-title">
    <div class="ui-bay-header">
      <h2 id="leaderboard-title">Fastest successful runs</h2>
      <span class="text-meta font-normal text-fg-2">Goal wins · fewest frames</span>
    </div>
    <ResourceState v-if="leaderboard.state.value === 'loading' || leaderboard.state.value === 'error'" class="p-3" :state="leaderboard.state.value" title="Couldn’t load the leaderboard" :message="leaderboard.error.value" :rows="4">
      <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="leaderboard.retry()">Retry</button></template>
    </ResourceState>
    <p v-else-if="!rows.length" class="px-3 py-8 text-center text-fg-2">No successful runs yet. The leaderboard fills in once a run reaches its goal and settles.</p>
    <div v-else class="overflow-x-auto">
      <table class="w-full min-w-[640px] table-fixed border-collapse text-left">
        <thead>
          <tr class="h-9 border-b border-rule-strong bg-raised text-meta text-fg-2">
            <th class="w-14 px-3 text-right font-medium">Rank</th>
            <th class="w-48 px-3 font-medium">Run</th>
            <th class="px-3 font-medium">Goal</th>
            <th class="w-56 px-3 font-medium">Model</th>
            <th class="w-28 px-3 text-right font-medium">Frames</th>
            <th class="w-24 px-3 text-right font-medium">Rounds</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(run, index) in rows" :key="run.run_id" class="h-10 border-b border-rule last:border-b-0">
            <td class="px-3 text-right font-mono text-fg-2">{{ index + 1 }}</td>
            <td class="px-3"><a :href="operatorHref('live', { run: run.run_id }, operatorSearch())" :title="run.run_id" class="font-mono text-accent hover:underline">{{ middleTruncate(run.run_id) }}</a></td>
            <td class="truncate px-3 text-fg-2" :title="run.goal || ''">{{ run.goal || 'Free play' }}</td>
            <td class="truncate px-3 text-fg-2" :title="modelLabel(run)">{{ modelLabel(run) }}</td>
            <td class="px-3 text-right font-mono">{{ nfmt(run.frame) }}</td>
            <td class="px-3 text-right font-mono text-fg-2">{{ nfmt(Number(run.stats?.rounds ?? run.stats?.round ?? 0)) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
