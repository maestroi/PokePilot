<script setup lang="ts">
import { computed } from 'vue'
import type { SpectatorRun } from '../shared/api/spectator'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { bagMeter, dexMeter } from '../shared/playerProgress'
import {
  averageRunSpeed,
  elapsedRunSeconds,
  formatDuration,
  formatRunSpeed,
  gameTimeSeconds,
  thinkingTimeSeconds
} from '../shared/runTiming'

const props = defineProps<{
  run: SpectatorRun
}>()

const timingMetrics = computed(() => {
  const run = props.run
  const gameTime = gameTimeSeconds(run)
  const elapsed = elapsedRunSeconds(run)
  const thinking = thinkingTimeSeconds(run)
  return [
    {
      label: 'Game time',
      value: gameTime > 0 ? formatDuration(gameTime) : '—',
      note: 'at native 1× speed'
    },
    {
      label: 'Elapsed',
      value: elapsed > 0 ? formatDuration(elapsed) : '—',
      note: run.status === 'done' ? 'wall-clock total' : 'wall clock so far'
    },
    {
      label: 'Avg speed',
      value: formatRunSpeed(averageRunSpeed(run)),
      note: 'game time ÷ elapsed'
    },
    {
      label: 'Thinking',
      value: thinking > 0 ? formatDuration(thinking) : '—',
      note: `${Number(run.stats?.calls || 0).toLocaleString()} model calls`
    }
  ]
})

const dexPercent = computed(() => {
  const owned = Number(props.run.player?.dex_owned || 0)
  const total = Number(props.run.player?.dex_total || 0)
  return total > 0 ? Math.max(0, Math.min(100, 100 * owned / total)) : 0
})

const bag = computed(() => props.run.player?.bag || [])
const milestones = computed(() => props.run.player?.milestones || [])
</script>

<template>
  <div v-if="run.player?.badges?.length" class="mt-3 flex flex-wrap gap-1.5 border-t border-white/8 pt-3">
    <StatusBadge v-for="badge in run.player.badges" :key="badge" tone="warning">{{ badge }}</StatusBadge>
  </div>

  <section v-if="Number(run.frame || 0) > 0" class="mt-3 border-t border-white/8 pt-3">
    <div class="mb-2 flex flex-wrap items-baseline justify-between gap-2">
      <div>
        <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-500 uppercase">Run pace</div>
        <p class="mt-0.5 text-[10px] text-slate-600">Measured from emulated Game Boy frames, not the configured FPS target.</p>
      </div>
      <span class="font-mono text-[10px] text-slate-600">frame {{ Number(run.frame || 0).toLocaleString() }}</span>
    </div>
    <div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
      <div v-for="metric in timingMetrics" :key="metric.label" class="rounded-lg border border-white/8 bg-black/15 px-3 py-2.5">
        <div class="text-[9px] font-semibold tracking-[0.09em] text-slate-500 uppercase">{{ metric.label }}</div>
        <div class="mt-1 font-mono text-lg font-semibold text-white">{{ metric.value }}</div>
        <div class="mt-0.5 text-[10px] text-slate-600">{{ metric.note }}</div>
      </div>
    </div>
  </section>

  <div
    v-if="bagMeter(run.player) || dexMeter(run.player) || milestones.length"
    class="mt-3 grid gap-2 border-t border-white/8 pt-3 lg:grid-cols-3"
  >
    <section v-if="bagMeter(run.player)" class="min-w-0 rounded-lg border border-white/8 bg-black/15 p-3">
      <div class="flex items-baseline justify-between gap-3">
        <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-500 uppercase">Bag</div>
        <strong class="font-mono text-xs text-slate-300">{{ bagMeter(run.player) }}</strong>
      </div>
      <div v-if="bag.length" class="mt-2 flex flex-wrap gap-1.5">
        <span
          v-for="item in bag"
          :key="item.name"
          class="rounded-md bg-white/5 px-2 py-1 text-[10px] text-slate-300 ring-1 ring-white/8"
        >
          <span class="capitalize">{{ item.name }}</span>
          <strong class="ml-1 font-mono text-slate-500">×{{ item.quantity }}</strong>
        </span>
      </div>
      <p v-else class="mt-2 text-xs text-slate-600">Empty</p>
    </section>

    <section v-if="dexMeter(run.player)" class="min-w-0 rounded-lg border border-white/8 bg-black/15 p-3">
      <div class="flex items-baseline justify-between gap-3">
        <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-500 uppercase">Pokédex</div>
        <span class="font-mono text-[10px] text-slate-500">{{ Number(run.player?.dex_seen || 0) }} seen</span>
      </div>
      <div class="mt-2 flex items-end gap-1.5">
        <strong class="font-mono text-xl text-white">{{ Number(run.player?.dex_owned || 0) }}</strong>
        <span class="pb-0.5 font-mono text-xs text-slate-500">/ {{ Number(run.player?.dex_total || 0) }} owned</span>
      </div>
      <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-white/8">
        <div class="mode-progress h-full rounded-full transition-[width]" :style="{ width: `${dexPercent}%` }" />
      </div>
      <div class="mt-1.5 flex justify-between font-mono text-[10px] text-slate-600">
        <span>{{ dexPercent.toFixed(1) }}% complete</span>
        <span>{{ Math.max(0, Number(run.player?.dex_total || 0) - Number(run.player?.dex_owned || 0)) }} left</span>
      </div>
    </section>

    <section class="min-w-0 rounded-lg border border-white/8 bg-black/15 p-3">
      <div class="flex items-baseline justify-between gap-3">
        <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-500 uppercase">Milestones</div>
        <span class="font-mono text-[10px] text-slate-500">{{ milestones.length }}</span>
      </div>
      <div v-if="milestones.length" class="mt-2 flex flex-wrap gap-1.5">
        <span
          v-for="milestone in milestones"
          :key="milestone"
          class="rounded-md bg-white/5 px-2 py-1 text-[10px] text-slate-300 ring-1 ring-white/8"
        >{{ milestone }}</span>
      </div>
      <p v-else class="mt-2 text-xs text-slate-600">No major milestones yet.</p>
    </section>
  </div>
</template>

<style scoped>
.mode-progress {
  background: var(--mode-accent);
}
</style>
