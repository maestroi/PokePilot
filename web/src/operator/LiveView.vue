<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ArrowPathIcon, NoSymbolIcon } from '@heroicons/vue/20/solid'
import { cancelRun, getDashboard, getRun } from '../shared/api/client'
import type { DashboardRun, PartyMon } from '../shared/api/types'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { useFramePump } from '../shared/composables/useFramePump'
import { usePollingResource } from '../shared/composables/usePollingResource'
import InspectorPanel from './InspectorPanel.vue'
import SemanticMap from './SemanticMap.vue'
import { goalLabel, llmProfileLabel, shortID, statusTone } from './operations'

const params = new URLSearchParams(window.location.search)
const selectedRunID = ref(params.get('run') || '')
const selectionPinned = ref(Boolean(selectedRunID.value))
const historicalRun = ref<DashboardRun | null>(null)
const historicalError = ref('')
const canceling = ref(false)
const actionError = ref('')
let detailSerial = 0

const activeResource = usePollingResource(
  (signal) => getDashboard({ active: true }, signal),
  { intervalMs: 2000 }
)

const recentResource = usePollingResource(
  (signal) => getDashboard({ status: 'done', limit: 8 }, signal),
  { intervalMs: 10000 }
)

const activeRuns = computed(() => [...(activeResource.data.value?.runs ?? [])]
  .filter((run) => run.status !== 'done')
  .sort((a, b) => Number(b.queued_at || 0) - Number(a.queued_at || 0)))
const recentRuns = computed(() => [...(recentResource.data.value?.runs ?? [])]
  .filter((run) => run.status === 'done')
  .sort((a, b) => Number(b.ended_at || 0) - Number(a.ended_at || 0)))
const knownRuns = computed(() => [...activeRuns.value, ...recentRuns.value])

const selectedRun = computed<DashboardRun | null>(() => {
  const id = selectedRunID.value
  if (id) {
    const known = knownRuns.value.find((run) => run.run_id === id)
    if (known) return known
    if (historicalRun.value?.run_id === id) return historicalRun.value
  }
  return activeRuns.value[0] || recentRuns.value[0] || null
})

watch(selectedRun, (run) => {
  if (run && !selectionPinned.value) selectedRunID.value = run.run_id
}, { immediate: true })

watch(selectedRunID, async (runID) => {
  const id = ++detailSerial
  historicalRun.value = null
  historicalError.value = ''
  if (!runID || knownRuns.value.some((run) => run.run_id === runID)) return
  try {
    const run = await getRun(runID)
    if (id === detailSerial) historicalRun.value = run
  } catch (cause) {
    if (id === detailSerial) historicalError.value = cause instanceof Error ? cause.message : 'Run not found'
  }
}, { immediate: true })

const liveRunID = computed(() => {
  const run = selectedRun.value
  return run && (run.status === 'running' || run.status === 'leased') ? run.run_id : ''
})
const frameEnabled = computed(() => Boolean(liveRunID.value))
const { frameURL, state: frameState, error: frameError } = useFramePump(liveRunID, frameEnabled)

const party = computed<PartyMon[]>(() => selectedRun.value?.player?.party ?? [])
const badges = computed(() => selectedRun.value?.player?.badges ?? [])
const goalProgress = computed(() => {
  const stats = selectedRun.value?.stats
  if (stats?.goal_complete) return 100
  const current = Number(stats?.goal_current || 0)
  const target = Number(stats?.goal_target || 0)
  return target > 0 ? Math.max(0, Math.min(100, 100 * current / target)) : 0
})
const isCancelable = computed(() => {
  const status = selectedRun.value?.status
  return status === 'queued' || status === 'leased' || status === 'running'
})
const resourceState = computed(() => {
  if (activeResource.state.value === 'error' && recentResource.state.value === 'error') return 'error'
  if (activeResource.state.value === 'loading' && recentResource.state.value === 'loading') return 'loading'
  if (activeResource.state.value === 'stale' || recentResource.state.value === 'stale') return 'stale'
  if (!selectedRun.value && (activeResource.state.value === 'empty' || recentResource.state.value === 'empty')) return 'empty'
  return 'ready'
})

function selectRun(run: DashboardRun): void {
  selectedRunID.value = run.run_id
  selectionPinned.value = true
  const url = new URL(window.location.href)
  url.searchParams.set('run', run.run_id)
  history.replaceState(null, '', url)
}

function hpPercent(mon: PartyMon): number {
  if (!mon.max_hp) return 0
  return Math.max(0, Math.min(100, 100 * mon.hp / mon.max_hp))
}

function location(run: DashboardRun): string {
  return `map 0x${Number(run.map || 0).toString(16).padStart(2, '0')} · ${Number(run.x || 0)},${Number(run.y || 0)}`
}

function refresh(): void {
  void activeResource.retry()
  void recentResource.retry()
}

async function cancelSelected(): Promise<void> {
  const run = selectedRun.value
  if (!run || !isCancelable.value || canceling.value) return
  canceling.value = true
  actionError.value = ''
  try {
    await cancelRun(run.run_id)
    await activeResource.retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Cancel failed'
  } finally {
    canceling.value = false
  }
}
</script>

<template>
  <div class="space-y-3">
    <ResourceState
      :state="resourceState"
      title="Live run data unavailable"
      :message="historicalError || activeResource.error.value || recentResource.error.value || 'The console will recover automatically when the wall is reachable.'"
      :rows="8"
    >
      <template #actions>
        <button type="button" class="inline-flex items-center gap-1.5 rounded-md bg-white/10 px-2.5 py-1.5 text-xs font-semibold text-white ring-1 ring-white/10 hover:bg-white/15" @click="refresh">
          <ArrowPathIcon class="size-3.5" aria-hidden="true" /> Retry now
        </button>
      </template>

      <div v-if="selectedRun" class="grid grid-cols-1 gap-3 2xl:grid-cols-[12rem_minmax(0,1fr)]">
        <aside class="space-y-3">
          <Panel title="Runs" description="Active first, then recent." compact>
            <div class="max-h-[44rem] overflow-y-auto pr-0.5">
              <div v-if="activeRuns.length" class="mb-3">
                <span class="mb-1.5 block text-[10px] font-semibold tracking-[0.08em] text-slate-600 uppercase">Active</span>
                <div class="space-y-1.5">
                  <button
                    v-for="run in activeRuns"
                    :key="run.run_id"
                    type="button"
                    :class="[
                      selectedRun.run_id === run.run_id ? 'bg-cyan-300/10 ring-cyan-300/20' : 'bg-black/10 ring-white/8 hover:bg-white/5',
                      'w-full rounded-md px-2.5 py-2 text-left ring-1 transition-colors'
                    ]"
                    @click="selectRun(run)"
                  >
                    <div class="flex items-center justify-between gap-2">
                      <span class="truncate font-mono text-[11px] text-slate-300" :title="run.run_id">{{ shortID(run.run_id, 14) }}</span>
                      <span :class="['size-1.5 shrink-0 rounded-full', run.status === 'running' ? 'bg-emerald-300' : 'bg-amber-300']" />
                    </div>
                    <p class="mt-1 truncate text-[10px] text-slate-600" :title="goalLabel(run)">{{ goalLabel(run) }}</p>
                  </button>
                </div>
              </div>

              <div v-if="recentRuns.length">
                <span class="mb-1.5 block text-[10px] font-semibold tracking-[0.08em] text-slate-600 uppercase">Recent</span>
                <div class="space-y-1.5">
                  <button
                    v-for="run in recentRuns"
                    :key="run.run_id"
                    type="button"
                    :class="[
                      selectedRun.run_id === run.run_id ? 'bg-white/8 ring-cyan-300/20' : 'bg-black/10 ring-white/8 hover:bg-white/5',
                      'w-full rounded-md px-2.5 py-2 text-left ring-1 transition-colors'
                    ]"
                    @click="selectRun(run)"
                  >
                    <span class="block truncate font-mono text-[11px] text-slate-400" :title="run.run_id">{{ shortID(run.run_id, 14) }}</span>
                    <p class="mt-1 truncate text-[10px] text-slate-600">{{ run.reason || 'done' }}</p>
                  </button>
                </div>
              </div>
            </div>
          </Panel>
        </aside>

        <div class="min-w-0 space-y-3">
          <div class="grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,1.35fr)_minmax(18rem,0.65fr)] 2xl:grid-cols-[minmax(0,1.35fr)_minmax(17rem,0.62fr)_minmax(18rem,0.7fr)]">
            <Panel title="Game monitor" :description="goalLabel(selectedRun)" compact>
              <template #actions>
                <div class="flex items-center gap-2">
                  <StatusBadge :tone="statusTone(selectedRun.status)">{{ selectedRun.status }}</StatusBadge>
                  <button v-if="isCancelable" type="button" :disabled="canceling" class="inline-flex items-center gap-1 rounded-md bg-rose-400/8 px-2 py-1 text-[11px] font-semibold text-rose-200 ring-1 ring-rose-300/15 hover:bg-rose-400/15 disabled:opacity-50" @click="cancelSelected">
                    <NoSymbolIcon class="size-3.5" aria-hidden="true" /> {{ canceling ? 'Canceling…' : 'Cancel' }}
                  </button>
                </div>
              </template>

              <div class="relative grid h-[clamp(18rem,40vh,26rem)] place-items-center overflow-hidden rounded-md border border-white/10 bg-black/45">
                <img v-if="frameURL" :src="frameURL" :alt="`Live frame for ${selectedRun.run_id}`" class="h-full w-full object-contain [image-rendering:pixelated]" />
                <div v-else class="px-6 py-12 text-center">
                  <span :class="['mx-auto block size-2 rounded-full', frameEnabled ? 'animate-pulse bg-emerald-300' : 'bg-slate-600']" />
                  <p class="mt-3 text-sm font-medium text-slate-300">{{ frameEnabled ? 'Waiting for a live frame' : 'Run is not currently streaming' }}</p>
                  <p v-if="frameError" class="mt-1 text-xs text-amber-300/80">{{ frameError }}</p>
                </div>
                <div v-if="frameURL && frameState === 'error'" class="absolute right-3 bottom-3 rounded-md bg-black/70 px-2 py-1 text-[10px] text-amber-200 ring-1 ring-amber-300/20">Last frame · reconnecting</div>
              </div>
            </Panel>

            <Panel title="Semantic map" description="Collision grid, trail, objects, and player." compact>
              <SemanticMap :map="selectedRun.map" :x="selectedRun.x" :y="selectedRun.y" :trail="selectedRun.trail" :sprites="selectedRun.sprites" />
            </Panel>

            <Panel title="Party" :description="`${party.length} Pokémon · ₽${Number(selectedRun.player?.money || 0).toLocaleString()}`" compact class="xl:col-span-2 2xl:col-span-1">
              <div v-if="party.length" class="max-h-[22rem] divide-y divide-white/8 overflow-y-auto">
                <div v-for="(mon, index) in party" :key="`${mon.name}-${index}`" class="py-2 first:pt-0 last:pb-0">
                  <div class="flex items-baseline justify-between gap-3">
                    <strong class="truncate text-xs text-slate-200">{{ mon.name || 'Unknown' }}</strong>
                    <span class="font-mono text-[10px] text-slate-600">Lv {{ mon.level }}</span>
                  </div>
                  <div class="mt-1.5 h-1.5 overflow-hidden rounded-full bg-white/8"><div class="h-full rounded-full bg-emerald-400" :style="{ width: `${hpPercent(mon)}%` }" /></div>
                  <div class="mt-1 flex justify-between font-mono text-[10px] text-slate-600"><span>{{ mon.hp }}/{{ mon.max_hp }} HP</span><span>{{ mon.status || 'healthy' }}</span></div>
                </div>
              </div>
              <p v-else class="py-5 text-center text-xs text-slate-600">Party data not available yet.</p>
              <div v-if="badges.length" class="mt-3 flex flex-wrap gap-1.5 border-t border-white/8 pt-3"><StatusBadge v-for="badge in badges" :key="badge" tone="warning">{{ badge }}</StatusBadge></div>
            </Panel>
          </div>

          <div class="grid grid-cols-1 gap-3 xl:grid-cols-[minmax(18rem,0.75fr)_minmax(0,1.25fr)]">
            <Panel title="Objective" :description="selectedRun.stats?.goal_summary || selectedRun.stop_so_far || goalLabel(selectedRun)" compact>
              <div class="h-2 overflow-hidden rounded-full bg-white/8"><div class="h-full rounded-full bg-cyan-300 transition-[width]" :style="{ width: `${goalProgress}%` }" /></div>
              <div class="mt-2 flex justify-between font-mono text-[10px] text-slate-600">
                <span v-if="selectedRun.stats?.goal_target">{{ selectedRun.stats.goal_current || 0 }} / {{ selectedRun.stats.goal_target }}</span><span v-else>goal-driven</span><span>{{ goalProgress.toFixed(0) }}%</span>
              </div>
              <dl class="mt-2 grid grid-cols-2 gap-x-4 text-xs">
                <div class="flex items-center justify-between gap-3 border-t border-white/8 py-2"><dt class="text-slate-600">Round</dt><dd class="font-mono text-slate-400">{{ selectedRun.stats?.round ?? '—' }}</dd></div>
                <div class="flex items-center justify-between gap-3 border-t border-white/8 py-2"><dt class="text-slate-600">Frame</dt><dd class="font-mono text-slate-400">{{ Number(selectedRun.frame || 0).toLocaleString() }}</dd></div>
                <div class="col-span-2 flex items-center justify-between gap-3 border-t border-white/8 py-2"><dt class="text-slate-600">Route</dt><dd class="text-right text-slate-400">{{ llmProfileLabel(selectedRun) }}</dd></div>
                <div class="col-span-2 flex items-center justify-between gap-3 border-t border-white/8 py-2 last:pb-0"><dt class="text-slate-600">Location</dt><dd class="font-mono text-right text-slate-400">{{ location(selectedRun) }}</dd></div>
              </dl>
            </Panel>

            <Panel title="Planner" description="Latest model-facing choice and resolved objective." compact>
              <div class="grid gap-3 xl:grid-cols-2">
                <div><span class="text-[10px] font-semibold tracking-[0.08em] text-slate-600 uppercase">Question</span><p class="mt-1 max-h-32 overflow-auto whitespace-pre-wrap text-xs leading-5 text-slate-400">{{ selectedRun.question || 'No planner question yet.' }}</p></div>
                <div class="xl:border-l xl:border-white/8 xl:pl-3"><span class="text-[10px] font-semibold tracking-[0.08em] text-cyan-300/70 uppercase">Decision</span><p class="mt-1 max-h-32 overflow-auto whitespace-pre-wrap text-xs leading-5 text-slate-300">{{ selectedRun.decision || 'Waiting for a decision.' }}</p></div>
              </div>
              <details v-if="selectedRun.trace || selectedRun.raw" class="mt-3 border-t border-white/8 pt-3"><summary class="cursor-pointer text-[10px] font-semibold tracking-[0.08em] text-slate-600 uppercase">Raw / trace</summary><pre class="mt-2 max-h-48 overflow-auto whitespace-pre-wrap rounded-md bg-black/20 p-2 font-mono text-[9px] leading-4 text-slate-600">{{ selectedRun.raw || selectedRun.trace }}</pre></details>
            </Panel>
          </div>

          <div v-if="actionError" class="border-l-4 border-rose-400 bg-rose-400/10 p-3 text-sm text-rose-100" role="alert">{{ actionError }}</div>
        </div>
      </div>

      <div v-else class="py-16 text-center">
        <p class="text-sm font-medium text-slate-300">No run selected</p>
        <p class="mt-1 text-xs text-slate-600">Queue a run from Tools or choose one from the Runs archive.</p>
        <a href="#tools" class="mt-4 inline-flex rounded-md bg-cyan-500 px-3 py-2 text-xs font-semibold text-white hover:bg-cyan-400">Start a run</a>
      </div>
    </ResourceState>

    <InspectorPanel v-if="selectedRun" :run-id="selectedRun.run_id" />
  </div>
</template>
