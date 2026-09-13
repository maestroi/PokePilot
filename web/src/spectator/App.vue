<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ArrowPathIcon, LinkIcon } from '@heroicons/vue/20/solid'
import { getSpectatorSnapshot, spectatorReplayVideoURL } from '../shared/api/spectator-client'
import type { SpectatorRun } from '../shared/api/spectator'
import AppShell from '../shared/components/AppShell.vue'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { useFramePump } from '../shared/composables/useFramePump'
import { usePollingResource } from '../shared/composables/usePollingResource'
import {
  goalProgress,
  isLiveRun,
  locationLabel,
  objectiveLabel,
  preferredRun,
  routeLabel,
  runStatusLabel,
  runTone,
  shortRunID,
  splitSpectatorRuns
} from './model'

const initialParams = new URLSearchParams(window.location.search)
const selectedRunID = ref(initialParams.get('run') || '')
const selectionPinned = ref(initialParams.has('run'))
const copyState = ref('')

const {
  data: snapshot,
  error,
  state,
  retry
} = usePollingResource(
  (signal) => getSpectatorSnapshot(signal),
  {
    intervalMs: 2000,
    isEmpty: (value) => value.runs.length === 0
  }
)

const runs = computed(() => snapshot.value?.runs ?? [])
const groupedRuns = computed(() => splitSpectatorRuns(runs.value))
const selectedRun = computed(() => preferredRun(
  runs.value,
  selectionPinned.value ? selectedRunID.value : ''
))
const liveRunID = computed(() => isLiveRun(selectedRun.value) ? selectedRun.value?.run_id || '' : '')
const frameEnabled = computed(() => Boolean(liveRunID.value))
const { frameURL, state: frameState, error: frameError } = useFramePump(liveRunID, frameEnabled)
const replayURL = computed(() => {
  const run = selectedRun.value
  return run?.status === 'done' && run.replay_ready ? spectatorReplayVideoURL(run.run_id) : ''
})

const summaryMetrics = computed(() => {
  const summary = snapshot.value?.summary || { live: 0, queued: 0, completed: 0 }
  return [
    { label: 'Live', value: summary.live, note: 'running + leased' },
    { label: 'Queued', value: summary.queued, note: 'waiting for a worker' },
    { label: 'Completed', value: summary.completed, note: 'farm total' }
  ]
})

watch(selectedRun, (run) => {
  if (run && !selectionPinned.value) selectedRunID.value = run.run_id
}, { immediate: true })

function selectRun(run: SpectatorRun): void {
  selectedRunID.value = run.run_id
  selectionPinned.value = true
  const url = new URL(window.location.href)
  url.searchParams.set('run', run.run_id)
  history.replaceState(null, '', url)
}

function refresh(): void {
  void retry()
}

async function copyLink(): Promise<void> {
  const run = selectedRun.value
  if (!run) return
  const url = new URL(window.location.href)
  url.searchParams.set('run', run.run_id)
  try {
    await navigator.clipboard.writeText(url.toString())
    copyState.value = 'Copied'
  } catch {
    copyState.value = 'Copy failed'
  }
  window.setTimeout(() => { copyState.value = '' }, 1500)
}

function hpPercent(run: SpectatorRun, index: number): number {
  const mon = run.player?.party?.[index]
  if (!mon || mon.max_hp <= 0) return 0
  return Math.max(0, Math.min(100, 100 * mon.hp / mon.max_hp))
}
</script>

<template>
  <AppShell
    eyebrow="PokéPilot"
    title="Spectator"
    subtitle="Watch public-safe live runs and curated replay highlights. This surface stays read-only at both the UI and backend route layers."
    mode="public"
  >
    <template #actions>
      <button
        v-if="selectedRun"
        type="button"
        class="inline-flex items-center gap-1.5 rounded-md bg-white/8 px-2.5 py-1.5 text-xs font-semibold text-slate-200 ring-1 ring-white/10 hover:bg-white/12"
        @click="copyLink"
      >
        <LinkIcon class="size-3.5" aria-hidden="true" />
        {{ copyState || 'Copy run link' }}
      </button>
    </template>

    <ResourceState
      :state="state"
      title="Spectator feed unavailable"
      :message="error || 'The public endpoint will reconnect automatically when the wall is reachable again.'"
      :rows="6"
    >
      <template #actions>
        <button type="button" class="inline-flex items-center gap-1.5 rounded-md bg-white/10 px-2.5 py-1.5 text-xs font-semibold text-white ring-1 ring-white/10 hover:bg-white/15" @click="refresh">
          <ArrowPathIcon class="size-3.5" aria-hidden="true" />
          Retry now
        </button>
      </template>

      <div class="mb-3 grid grid-cols-3 gap-px overflow-hidden rounded-lg bg-white/10 ring-1 ring-white/10">
        <div v-for="metric in summaryMetrics" :key="metric.label" class="bg-[#0b111a] px-3 py-3 sm:px-5 sm:py-4">
          <dt class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">{{ metric.label }}</dt>
          <dd class="mt-1 font-mono text-xl font-semibold tabular-nums text-white">{{ metric.value }}</dd>
          <p class="mt-0.5 hidden text-xs text-slate-500 sm:block">{{ metric.note }}</p>
        </div>
      </div>

      <div v-if="selectedRun" class="grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,2fr)_minmax(20rem,1fr)]">
        <div class="space-y-3">
          <Panel title="Game" :description="routeLabel(selectedRun)" compact>
            <template #actions><StatusBadge :tone="runTone(selectedRun)">{{ runStatusLabel(selectedRun) }}</StatusBadge></template>

            <div class="relative min-h-80 overflow-hidden rounded-md border border-white/10 bg-[#0c1118] sm:min-h-[28rem] lg:min-h-[32rem]">
              <video
                v-if="replayURL"
                :key="selectedRun.run_id"
                class="absolute inset-0 h-full w-full object-contain object-center [image-rendering:pixelated]"
                :src="replayURL"
                controls
                preload="metadata"
                playsinline
              />

              <img
                v-else-if="frameURL"
                :src="frameURL"
                :alt="`Live frame for ${selectedRun.run_id}`"
                class="absolute inset-0 h-full w-full object-contain object-center [image-rendering:pixelated]"
              />

              <div v-else class="absolute inset-0 grid place-items-center px-6 py-12 text-center">
                <div class="mx-auto flex size-11 items-center justify-center rounded-lg border border-white/10 bg-white/5">
                  <span :class="['size-2 rounded-full', isLiveRun(selectedRun) ? 'animate-pulse bg-emerald-300' : 'bg-slate-600']" />
                </div>
                <p class="mt-3 text-sm font-medium text-slate-300">
                  {{ selectedRun.status === 'queued' ? 'Waiting for a worker' : selectedRun.status === 'done' ? 'Replay is not public for this run' : 'Waiting for a live frame' }}
                </p>
                <p v-if="frameState === 'error' && frameError" class="mt-1 text-xs text-amber-300/80">{{ frameError }}</p>
              </div>

              <div v-if="frameURL && frameState === 'error'" class="absolute right-3 bottom-3 rounded-md bg-black/70 px-2 py-1 text-[10px] font-semibold text-amber-200 ring-1 ring-amber-300/20">
                Last frame · reconnecting
              </div>
            </div>
          </Panel>

          <Panel title="Party" description="Live trainer snapshot supplied by the restricted public feed." compact>
            <div v-if="selectedRun.player?.party?.length" class="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
              <div v-for="(mon, index) in selectedRun.player.party" :key="`${mon.name}-${index}`" class="rounded-md border border-white/10 bg-black/15 p-3">
                <div class="flex items-baseline justify-between gap-3">
                  <strong class="truncate text-sm text-white">{{ mon.name || 'Unknown' }}</strong>
                  <span class="font-mono text-[11px] text-slate-500">Lv {{ mon.level }}</span>
                </div>
                <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-white/8">
                  <div class="h-full rounded-full bg-emerald-400 transition-[width]" :style="{ width: `${hpPercent(selectedRun, index)}%` }" />
                </div>
                <div class="mt-1.5 flex justify-between font-mono text-[10px] text-slate-500">
                  <span>{{ mon.hp }}/{{ mon.max_hp }} HP</span>
                  <span>{{ mon.status || 'healthy' }}</span>
                </div>
              </div>
            </div>
            <p v-else class="py-5 text-center text-sm text-slate-500">Party data is not available yet.</p>

            <div v-if="selectedRun.player?.badges?.length" class="mt-3 flex flex-wrap gap-1.5 border-t border-white/8 pt-3">
              <StatusBadge v-for="badge in selectedRun.player.badges" :key="badge" tone="warning">{{ badge }}</StatusBadge>
            </div>
          </Panel>
        </div>

        <div class="space-y-3">
          <Panel title="Run state" description="Public-safe progress and decision context." compact>
            <dl class="divide-y divide-white/8">
              <div class="py-2 first:pt-0">
                <dt class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Run</dt>
                <dd class="mt-1 break-all font-mono text-xs text-slate-300">{{ selectedRun.run_id }}</dd>
              </div>
              <div class="py-2">
                <dt class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Location</dt>
                <dd class="mt-1 font-mono text-xs text-slate-300">{{ locationLabel(selectedRun) }}</dd>
              </div>
              <div class="py-2">
                <dt class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Round / frame</dt>
                <dd class="mt-1 font-mono text-xs text-slate-300">{{ selectedRun.stats?.round ?? '—' }} / {{ Number(selectedRun.frame || 0).toLocaleString() }}</dd>
              </div>
              <div class="py-2 last:pb-0">
                <dt class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Latest decision</dt>
                <dd class="mt-1 text-xs leading-5 text-slate-300">{{ selectedRun.decision || 'Waiting for a decision' }}</dd>
              </div>
            </dl>
          </Panel>

          <Panel title="Objective" :description="objectiveLabel(selectedRun)" compact>
            <div class="h-2 overflow-hidden rounded-full bg-white/8">
              <div class="h-full rounded-full bg-cyan-300 transition-[width]" :style="{ width: `${goalProgress(selectedRun)}%` }" />
            </div>
            <div class="mt-2 flex items-center justify-between gap-3 font-mono text-[10px] text-slate-500">
              <span v-if="selectedRun.stats?.goal_target">{{ selectedRun.stats.goal_current || 0 }} / {{ selectedRun.stats.goal_target }}</span>
              <span v-else>goal-driven</span>
              <span>{{ goalProgress(selectedRun).toFixed(0) }}%</span>
            </div>
          </Panel>

          <Panel title="Live runs" description="Select a public in-flight run." compact>
            <div v-if="groupedRuns.live.length" class="space-y-1.5">
              <button
                v-for="run in groupedRuns.live"
                :key="run.run_id"
                type="button"
                :class="[
                  run.run_id === selectedRun.run_id ? 'bg-white/8 ring-cyan-300/20' : 'bg-black/10 ring-white/8 hover:bg-white/5',
                  'w-full rounded-md px-3 py-2 text-left ring-1 transition-colors'
                ]"
                @click="selectRun(run)"
              >
                <div class="flex items-center justify-between gap-3">
                  <strong class="truncate font-mono text-xs text-slate-300">{{ shortRunID(run.run_id) }}</strong>
                  <StatusBadge :tone="runTone(run)">{{ runStatusLabel(run) }}</StatusBadge>
                </div>
                <p class="mt-1 truncate text-[11px] text-slate-500">{{ routeLabel(run) }}</p>
              </button>
            </div>
            <p v-else class="py-4 text-center text-xs text-slate-500">Nothing is running right now.</p>
          </Panel>

          <Panel title="Replay highlights" description="Curated completed runs whose public video is ready." compact>
            <div v-if="groupedRuns.recent.length" class="space-y-1.5">
              <button
                v-for="run in groupedRuns.recent"
                :key="run.run_id"
                type="button"
                :class="[
                  run.run_id === selectedRun.run_id ? 'bg-white/8 ring-cyan-300/20' : 'bg-black/10 ring-white/8 hover:bg-white/5',
                  'w-full rounded-md px-3 py-2 text-left ring-1 transition-colors'
                ]"
                @click="selectRun(run)"
              >
                <div class="flex items-center justify-between gap-3">
                  <strong class="truncate font-mono text-xs text-slate-300">{{ shortRunID(run.run_id) }}</strong>
                  <StatusBadge :tone="runTone(run)">{{ runStatusLabel(run) }}</StatusBadge>
                </div>
                <p class="mt-1 truncate text-[11px] text-slate-500">{{ routeLabel(run) }}</p>
              </button>
            </div>
            <p v-else class="py-4 text-center text-xs text-slate-500">No public replay highlights are available yet.</p>
          </Panel>
        </div>
      </div>
    </ResourceState>
  </AppShell>
</template>
