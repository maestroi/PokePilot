<script setup lang="ts">
import { computed } from 'vue'
import { ArrowPathIcon } from '@heroicons/vue/20/solid'
import { getDashboard } from '../shared/api/client'
import type { DashboardRun } from '../shared/api/types'
import BadgeIcon from '../shared/components/BadgeIcon.vue'
import Panel from '../shared/components/Panel.vue'
import PokemonSprite from '../shared/components/PokemonSprite.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import {
  ageLabel,
  formatFrame,
  goalLabel,
  llmProfileLabel,
  outcomeLabel,
  outcomeTone,
  partitionOperationRuns,
  shortID,
  shortRevision,
  statusTone
} from './operations'

const {
  data: activeData,
  error: activeError,
  lastUpdatedAt: activeUpdatedAt,
  state: activeState,
  retry: retryActive
} = usePollingResource(
  (signal) => getDashboard({ active: true }, signal),
  { intervalMs: 2000 }
)

const {
  data: recentData,
  error: recentError,
  state: recentState,
  retry: retryRecent
} = usePollingResource(
  (signal) => getDashboard({ status: 'done', limit: 12 }, signal),
  {
    intervalMs: 10000,
    isEmpty: (snapshot) => snapshot.runs.length === 0
  }
)

const workers = computed(() => [...(activeData.value?.workers ?? [])]
  .sort((a, b) => a.addr.localeCompare(b.addr)))
const runs = computed(() => partitionOperationRuns(activeData.value?.runs ?? []))
const recentRuns = computed(() => partitionOperationRuns(recentData.value?.runs ?? [], 12).recent)
const idleWorkers = computed(() => workers.value.filter((worker) => !worker.run_id).length)
const connectionTone = computed(() => activeState.value === 'stale' ? 'warning' : activeState.value === 'error' ? 'danger' : 'success')
const connectionLabel = computed(() => activeState.value === 'stale' ? 'Stale' : activeState.value === 'error' ? 'Offline' : 'Connected')
const activeUpdatedLabel = computed(() => activeUpdatedAt.value ? new Date(activeUpdatedAt.value).toLocaleTimeString() : '—')
const nowSeconds = computed(() => activeData.value?.now || Date.now() / 1000)

const metrics = computed(() => [
  { label: 'Workers', value: workers.value.length, note: `${idleWorkers.value} available` },
  { label: 'Active', value: runs.value.active.length, note: 'running attempts' },
  { label: 'Waiting', value: runs.value.waiting.length, note: 'queued + leased' },
  { label: 'Wall build', value: shortRevision(activeData.value?.wall_version), note: 'current revision', mono: true }
])

function leadMon(run: DashboardRun) {
  return run.player?.party?.[0]
}

function runBadges(run: DashboardRun): string[] {
  return run.player?.badges ?? []
}

function dexProgressLabel(run: DashboardRun): string {
  const owned = Number(run.player?.dex_owned || 0)
  const total = Number(run.player?.dex_total || 0)
  if (total > 0) return `${owned}/${total}`
  if (owned > 0) return String(owned)
  return '—'
}

function refreshActive(): void {
  void retryActive()
}

function refreshRecent(): void {
  void retryRecent()
}
</script>

<template>
  <div class="space-y-3">
    <ResourceState
      :state="activeState"
      title="Operator data is unavailable"
      :message="activeError || 'Workers and active runs will recover automatically when the wall is reachable again.'"
      :rows="5"
    >
      <template #actions>
        <button type="button" class="inline-flex items-center gap-1.5 rounded-md bg-white/10 px-2.5 py-1.5 text-xs font-semibold text-white ring-1 ring-white/10 hover:bg-white/15" @click="refreshActive">
          <ArrowPathIcon class="size-3.5" aria-hidden="true" />
          Retry now
        </button>
      </template>

      <div class="grid grid-cols-2 gap-px overflow-hidden rounded-lg bg-white/10 ring-1 ring-white/10 lg:grid-cols-4">
        <div v-for="metric in metrics" :key="metric.label" class="bg-[#0b111a] px-4 py-4 sm:px-5">
          <dt class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">{{ metric.label }}</dt>
          <dd :class="['mt-1 text-xl font-semibold text-white', metric.mono ? 'font-mono' : 'font-mono tabular-nums']">{{ metric.value }}</dd>
          <p class="mt-0.5 text-xs text-slate-500">{{ metric.note }}</p>
        </div>
      </div>

      <div class="mt-3 grid grid-cols-1 gap-3 xl:grid-cols-3">
        <Panel title="System health" description="Wall connection and dashboard refresh state." compact>
          <dl class="divide-y divide-white/8">
            <div class="flex items-center justify-between gap-4 py-2 first:pt-0">
              <dt class="text-xs text-slate-500">Wall</dt>
              <dd><StatusBadge :tone="connectionTone">{{ connectionLabel }}</StatusBadge></dd>
            </div>
            <div class="flex items-center justify-between gap-4 py-2">
              <dt class="text-xs text-slate-500">Active refresh</dt>
              <dd class="font-mono text-xs text-slate-300">2s</dd>
            </div>
            <div class="flex items-center justify-between gap-4 py-2">
              <dt class="text-xs text-slate-500">Last fresh snapshot</dt>
              <dd class="font-mono text-xs text-slate-300">{{ activeUpdatedLabel }}</dd>
            </div>
            <div class="flex items-center justify-between gap-4 py-2 last:pb-0">
              <dt class="text-xs text-slate-500">Reporting workers</dt>
              <dd class="font-mono text-xs text-slate-300">{{ workers.length }}</dd>
            </div>
          </dl>
        </Panel>

        <Panel title="Workers" description="Current farm capacity and worker leases." compact class="xl:col-span-2">
          <div v-if="workers.length" class="-mx-3 overflow-x-auto sm:-mx-4">
            <table class="min-w-full divide-y divide-white/10 text-left">
              <thead>
                <tr>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:px-4">Worker</th>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">State</th>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Run</th>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Seen</th>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:pr-4">Build</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-white/8">
                <tr v-for="worker in workers" :key="worker.addr">
                  <td class="px-3 py-2.5 font-mono text-xs whitespace-nowrap text-slate-300 sm:px-4">{{ worker.addr }}</td>
                  <td class="px-3 py-2.5"><StatusBadge :tone="worker.run_id ? 'info' : 'success'">{{ worker.run_id ? 'Busy' : 'Available' }}</StatusBadge></td>
                  <td class="px-3 py-2.5 font-mono text-xs whitespace-nowrap text-slate-400" :title="worker.run_id || ''">{{ shortID(worker.run_id) }}</td>
                  <td class="px-3 py-2.5 text-xs whitespace-nowrap text-slate-500">{{ worker.seen_ago || '—' }}</td>
                  <td class="px-3 py-2.5 font-mono text-xs whitespace-nowrap text-slate-500 sm:pr-4">{{ shortRevision(worker.version) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="py-6 text-center text-sm text-slate-500">No workers are reporting.</p>
        </Panel>

        <Panel title="Active attempts" description="Runs currently executing on a worker." compact class="xl:col-span-2">
          <div v-if="runs.active.length" class="-mx-3 overflow-x-auto sm:-mx-4">
            <table class="min-w-full divide-y divide-white/10 text-left">
              <thead>
                <tr>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:px-4">Run</th>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Pokémon</th>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Progress</th>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Goal</th>
                  <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Route</th>
                  <th class="px-3 py-2 text-right text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Round</th>
                  <th class="px-3 py-2 text-right text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:pr-4">Frame</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-white/8">
                <tr v-for="run in runs.active" :key="run.run_id">
                  <td class="px-3 py-2.5 sm:px-4">
                    <div class="flex items-center gap-2">
                      <span class="size-1.5 rounded-full bg-emerald-300 shadow-[0_0_10px_rgba(110,231,183,0.45)]" />
                      <span class="font-mono text-xs text-slate-300" :title="run.run_id">{{ shortID(run.run_id) }}</span>
                    </div>
                  </td>
                  <td class="px-3 py-2">
                    <div v-if="leadMon(run)" class="flex min-w-[9rem] items-center gap-2">
                      <PokemonSprite :name="leadMon(run)?.name || ''" :size="34" />
                      <div class="min-w-0">
                        <div class="truncate text-[11px] font-semibold text-slate-300">{{ leadMon(run)?.name }}</div>
                        <div class="font-mono text-[9px] text-slate-600">Lv {{ leadMon(run)?.level }} · {{ run.player?.party?.length || 0 }}/6</div>
                      </div>
                    </div>
                    <span v-else class="text-xs text-slate-600">—</span>
                  </td>
                  <td class="px-3 py-2">
                    <div class="min-w-[8rem]">
                      <div class="flex h-5 items-center gap-0.5">
                        <BadgeIcon v-for="badge in runBadges(run).slice(0, 4)" :key="badge" :name="badge" :size="18" />
                        <span v-if="runBadges(run).length > 4" class="ml-0.5 font-mono text-[9px] text-slate-600">+{{ runBadges(run).length - 4 }}</span>
                        <span v-if="!runBadges(run).length" class="text-[10px] text-slate-700">no badges</span>
                      </div>
                      <div class="mt-0.5 font-mono text-[9px] text-slate-500">Dex {{ dexProgressLabel(run) }}</div>
                    </div>
                  </td>
                  <td class="max-w-md truncate px-3 py-2.5 text-xs text-slate-300" :title="goalLabel(run)">{{ goalLabel(run) }}</td>
                  <td class="px-3 py-2.5 text-xs whitespace-nowrap text-slate-500">{{ llmProfileLabel(run) }}</td>
                  <td class="px-3 py-2.5 text-right font-mono text-xs tabular-nums text-slate-400">{{ run.stats?.round ?? '—' }}</td>
                  <td class="px-3 py-2.5 text-right font-mono text-xs tabular-nums text-slate-400 sm:pr-4">{{ formatFrame(run.frame) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <p v-else class="py-6 text-center text-sm text-slate-500">No runs are executing right now.</p>
        </Panel>

        <Panel title="Queue and leases" description="Waiting work in oldest-first order." compact>
          <ul v-if="runs.waiting.length" class="divide-y divide-white/8">
            <li v-for="run in runs.waiting" :key="run.run_id" class="py-3 first:pt-0 last:pb-0">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="font-mono text-xs text-slate-300" :title="run.run_id">{{ shortID(run.run_id) }}</span>
                    <StatusBadge :tone="statusTone(run.status)">{{ run.status }}</StatusBadge>
                  </div>
                  <p class="mt-1 truncate text-xs text-slate-500" :title="goalLabel(run)">{{ goalLabel(run) }}</p>
                </div>
                <span class="shrink-0 font-mono text-[11px] text-slate-600">{{ ageLabel(run.queued_at, nowSeconds) }}</span>
              </div>
            </li>
          </ul>
          <p v-else class="py-6 text-center text-sm text-slate-500">Queue is empty.</p>
        </Panel>
      </div>
    </ResourceState>

    <Panel title="Recent outcomes" description="The latest terminal runs refresh independently every 10 seconds." compact>
      <ResourceState
        :state="recentState"
        title="No terminal runs yet"
        :message="recentError || 'Recent outcomes will appear here without affecting the live Operations refresh.'"
        :rows="4"
      >
        <template #actions>
          <button type="button" class="inline-flex items-center gap-1.5 rounded-md bg-white/10 px-2.5 py-1.5 text-xs font-semibold text-white ring-1 ring-white/10 hover:bg-white/15" @click="refreshRecent">
            <ArrowPathIcon class="size-3.5" aria-hidden="true" />
            Retry now
          </button>
        </template>

        <div class="-mx-3 overflow-x-auto sm:-mx-4">
          <table class="min-w-full divide-y divide-white/10 text-left">
            <thead>
              <tr>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:px-4">Run</th>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Outcome</th>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Progress</th>
                <th class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Goal</th>
                <th class="px-3 py-2 text-right text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Attempts</th>
                <th class="px-3 py-2 text-right text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:pr-4">Ended</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-white/8">
              <tr v-for="run in recentRuns" :key="run.run_id">
                <td class="px-3 py-2 sm:px-4" :title="run.run_id">
                  <div class="flex min-w-[8rem] items-center gap-2">
                    <PokemonSprite v-if="leadMon(run)" :name="leadMon(run)?.name || ''" :size="30" :fainted="Number(leadMon(run)?.hp || 0) <= 0" />
                    <span class="font-mono text-xs whitespace-nowrap text-slate-300">{{ shortID(run.run_id) }}</span>
                  </div>
                </td>
                <td class="px-3 py-2.5"><StatusBadge :tone="outcomeTone(run)">{{ outcomeLabel(run) }}</StatusBadge></td>
                <td class="px-3 py-2">
                  <div class="flex items-center gap-2 whitespace-nowrap">
                    <div class="flex items-center gap-0.5">
                      <BadgeIcon v-for="badge in runBadges(run).slice(0, 3)" :key="badge" :name="badge" :size="17" />
                    </div>
                    <span class="font-mono text-[9px] text-slate-500">Dex {{ dexProgressLabel(run) }}</span>
                  </div>
                </td>
                <td class="max-w-lg truncate px-3 py-2.5 text-xs text-slate-400" :title="goalLabel(run)">{{ goalLabel(run) }}</td>
                <td class="px-3 py-2.5 text-right font-mono text-xs tabular-nums text-slate-500">{{ run.attempts || 1 }}</td>
                <td class="px-3 py-2.5 text-right font-mono text-xs whitespace-nowrap text-slate-500 sm:pr-4">{{ ageLabel(run.ended_at, recentData?.now || nowSeconds) }} ago</td>
              </tr>
            </tbody>
          </table>
        </div>
      </ResourceState>
    </Panel>
  </div>
</template>
