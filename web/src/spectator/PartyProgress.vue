<script setup lang="ts">
import { computed, ref } from 'vue'
import type { SpectatorRun } from '../shared/api/spectator'
import BadgeIcon from '../shared/components/BadgeIcon.vue'
import ItemIcon from '../shared/components/ItemIcon.vue'
import MilestoneIcon from '../shared/components/MilestoneIcon.vue'
import { itemDisplayName } from '../shared/pokemonAssets'
import { bagMeter, dexMeter } from '../shared/playerProgress'
import { playSpeedLabel } from '../shared/playstyle'
import {
  elapsedRunSeconds,
  formatDuration,
  thinkingTimeSeconds
} from '../shared/runTiming'
import WorldExplorer from './WorldExplorer.vue'

const props = defineProps<{
  run: SpectatorRun
}>()

const worldOpen = ref(false)

const timingMetrics = computed(() => {
  const run = props.run
  const elapsed = elapsedRunSeconds(run)
  const thinking = thinkingTimeSeconds(run)
  const fps = Number(run.fps || 0)
  return [
    {
      label: 'Configured speed',
      value: playSpeedLabel(run),
      note: fps > 0 ? `${fps} FPS emulator target` : 'uncapped emulator target'
    },
    {
      label: 'Real time',
      value: elapsed > 0 ? formatDuration(elapsed) : '—',
      note: run.status === 'done' ? 'wall-clock total' : 'wall clock so far'
    },
    {
      label: 'Planner',
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
  <div v-if="run.player?.badges?.length" class="mt-3 border-t border-white/8 pt-3">
    <div class="mb-2 flex items-center justify-between gap-2">
      <div class="flex items-center gap-2">
        <span class="pokeball-mark"><span /></span>
        <span class="text-[9px] font-black tracking-[0.11em] text-slate-500 uppercase">Gym badges</span>
      </div>
      <span class="font-mono text-[9px] text-slate-600">{{ run.player.badges.length }}/8</span>
    </div>
    <div class="grid grid-cols-2 gap-1.5 sm:grid-cols-4 xl:grid-cols-8">
      <div
        v-for="badge in run.player.badges"
        :key="badge"
        class="badge-tile flex min-w-0 items-center gap-2 rounded-md px-2 py-1.5"
      >
        <BadgeIcon :name="badge" :size="30" />
        <span class="min-w-0 truncate text-[9px] font-bold tracking-[0.04em] text-amber-100/80 uppercase">{{ badge }}</span>
      </div>
    </div>
  </div>

  <section v-if="Number(run.frame || 0) > 0" class="mt-3 border-t border-white/8 pt-3">
    <div class="mb-2 flex flex-wrap items-baseline justify-between gap-2">
      <div class="flex items-center gap-2">
        <span class="pokeball-mark"><span /></span>
        <div>
          <div class="text-[9px] font-black tracking-[0.1em] text-slate-500 uppercase">Run timing</div>
          <p class="mt-0.5 text-[10px] text-slate-600">Configured emulator speed and real wall-clock runtime.</p>
        </div>
      </div>
      <span class="font-mono text-[10px] text-slate-600">frame {{ Number(run.frame || 0).toLocaleString() }}</span>
    </div>
    <div class="grid gap-2 sm:grid-cols-3">
      <div v-for="metric in timingMetrics" :key="metric.label" class="poke-stat rounded-lg border border-white/8 px-3 py-2.5">
        <div class="text-[9px] font-bold tracking-[0.09em] text-slate-500 uppercase">{{ metric.label }}</div>
        <div class="mt-1 font-mono text-lg font-semibold text-white">{{ metric.value }}</div>
        <div class="mt-0.5 text-[10px] text-slate-600">{{ metric.note }}</div>
      </div>
    </div>
  </section>

  <div
    v-if="bagMeter(run.player) || dexMeter(run.player) || milestones.length"
    class="mt-3 grid gap-2 border-t border-white/8 pt-3 lg:grid-cols-3"
  >
    <section v-if="bagMeter(run.player)" class="poke-panel bag-panel min-w-0 overflow-hidden rounded-lg border p-3">
      <div class="flex items-center justify-between gap-3">
        <div class="flex items-center gap-2">
          <span class="pokeball-mark"><span /></span>
          <div>
            <div class="text-[9px] font-black tracking-[0.11em] text-slate-400 uppercase">Bag</div>
            <div class="text-[9px] text-slate-600">Trainer inventory</div>
          </div>
        </div>
        <strong class="rounded bg-black/20 px-2 py-1 font-mono text-[10px] text-slate-300 ring-1 ring-white/8">{{ bagMeter(run.player) }}</strong>
      </div>
      <div v-if="bag.length" class="mt-3 grid gap-1.5 sm:grid-cols-2 lg:grid-cols-1 xl:grid-cols-2">
        <div
          v-for="item in bag"
          :key="item.name"
          class="flex min-w-0 items-center gap-2 rounded-md bg-black/20 p-1.5 ring-1 ring-white/7"
          :title="itemDisplayName(item.name)"
        >
          <ItemIcon :name="item.name" :size="28" />
          <span class="min-w-0 flex-1 truncate text-[10px] font-medium text-slate-300">{{ itemDisplayName(item.name) }}</span>
          <strong class="shrink-0 rounded bg-black/25 px-1.5 py-0.5 font-mono text-[9px] text-slate-500">×{{ item.quantity }}</strong>
        </div>
      </div>
      <p v-else class="mt-3 text-xs text-slate-600">The bag is empty.</p>
    </section>

    <section v-if="dexMeter(run.player)" class="poke-panel dex-panel relative min-w-0 overflow-hidden rounded-lg border p-3">
      <div class="dex-glow absolute -right-8 -top-8 size-28 rounded-full" />
      <div class="relative">
        <div class="flex items-center justify-between gap-3">
          <div class="flex items-center gap-2">
            <span class="dex-lens grid size-6 place-items-center rounded-full"><span class="size-2 rounded-full bg-cyan-200" /></span>
            <div>
              <div class="text-[9px] font-black tracking-[0.11em] text-red-100/85 uppercase">Pokédex</div>
              <div class="text-[9px] text-red-100/45">Kanto collection</div>
            </div>
          </div>
          <span class="font-mono text-[10px] text-red-100/55">{{ Number(run.player?.dex_seen || 0) }} seen</span>
        </div>
        <div class="mt-3 flex items-end gap-1.5">
          <strong class="font-mono text-2xl text-white">{{ Number(run.player?.dex_owned || 0) }}</strong>
          <span class="pb-1 font-mono text-xs text-red-100/55">/ {{ Number(run.player?.dex_total || 0) }} owned</span>
        </div>
        <div class="mt-2 h-2 overflow-hidden rounded-full bg-black/25 ring-1 ring-white/10">
          <div class="dex-progress h-full rounded-full transition-[width]" :style="{ width: `${dexPercent}%` }" />
        </div>
        <div class="mt-1.5 flex justify-between font-mono text-[10px] text-red-100/50">
          <span>{{ dexPercent.toFixed(1) }}% complete</span>
          <span>{{ Math.max(0, Number(run.player?.dex_total || 0) - Number(run.player?.dex_owned || 0)) }} left</span>
        </div>
      </div>
    </section>

    <section class="poke-panel milestone-panel min-w-0 overflow-hidden rounded-lg border p-3">
      <div class="flex items-center justify-between gap-3">
        <div class="flex items-center gap-2">
          <span class="pokeball-mark"><span /></span>
          <div>
            <div class="text-[9px] font-black tracking-[0.11em] text-slate-400 uppercase">Milestones</div>
            <div class="text-[9px] text-slate-600">Journey collection</div>
          </div>
        </div>
        <span class="font-mono text-[10px] text-slate-500">{{ milestones.length }}</span>
      </div>
      <div v-if="milestones.length" class="mt-3 grid gap-1.5 sm:grid-cols-2 lg:grid-cols-1 xl:grid-cols-2">
        <div
          v-for="milestone in milestones"
          :key="milestone"
          class="milestone-chip flex min-w-0 items-center gap-2 rounded-md px-2 py-1.5"
          :title="milestone"
        >
          <MilestoneIcon :name="milestone" :size="24" />
          <span class="min-w-0 flex-1 truncate text-[10px] text-slate-300">{{ milestone }}</span>
        </div>
      </div>
      <p v-else class="mt-3 text-xs text-slate-600">No major milestones yet.</p>
    </section>
  </div>

  <Teleport to="body">
    <button
      v-if="!worldOpen && Number(run.map) >= 0"
      type="button"
      class="fixed right-4 bottom-4 z-[80] inline-flex items-center gap-2 rounded-full bg-[#111a26] px-4 py-2.5 text-xs font-bold text-white shadow-2xl shadow-black/40 ring-1 ring-cyan-300/25 transition hover:bg-[#172334] hover:ring-cyan-300/40"
      @click="worldOpen = true"
    >
      <span class="size-2 rounded-full bg-cyan-300 shadow-[0_0_12px_rgba(103,232,249,0.65)]" />
      Explore world
    </button>

    <div
      v-if="worldOpen"
      class="fixed inset-0 z-[90] flex items-center justify-center bg-black/80 p-2 backdrop-blur-sm sm:p-5"
      role="dialog"
      aria-modal="true"
      aria-label="World explorer"
      @click.self="worldOpen = false"
    >
      <section class="flex max-h-[94vh] w-full max-w-[96rem] flex-col overflow-hidden rounded-xl border border-white/12 bg-[#0a1018] shadow-2xl shadow-black/60">
        <header class="flex flex-wrap items-center justify-between gap-3 border-b border-white/10 bg-[#0d141e] px-3 py-2.5 sm:px-4">
          <div>
            <div class="text-xs font-bold text-white">PokéPilot World Explorer</div>
            <div class="mt-0.5 text-[10px] text-slate-500">Explore the same ROM-derived semantic maps used by routing and the operator minimap.</div>
          </div>
          <button
            type="button"
            class="rounded-md bg-white/7 px-3 py-1.5 text-xs font-semibold text-slate-300 ring-1 ring-white/10 hover:bg-white/12 hover:text-white"
            @click="worldOpen = false"
          >
            Close
          </button>
        </header>
        <div class="min-h-0 flex-1 overflow-auto p-2 sm:p-3">
          <WorldExplorer :run="run" />
        </div>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.poke-stat,
.poke-panel {
  border-color: rgba(148, 163, 184, 0.11);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.025), transparent 48%),
    rgba(0, 0, 0, 0.15);
  box-shadow: inset 0 1px rgba(255, 255, 255, 0.025);
}

.bag-panel {
  border-top-color: rgba(96, 165, 250, 0.28);
}

.milestone-panel {
  border-top-color: rgba(250, 204, 21, 0.24);
}

.dex-panel {
  border-color: rgba(248, 113, 113, 0.32);
  background:
    linear-gradient(145deg, rgba(127, 29, 29, 0.76), rgba(69, 10, 10, 0.5)),
    #22090c;
}

.dex-glow {
  background: rgba(248, 113, 113, 0.13);
  filter: blur(4px);
}

.dex-lens {
  border: 2px solid rgba(224, 242, 254, 0.35);
  background: rgb(14 116 144 / 0.75);
  box-shadow: 0 0 10px rgba(34, 211, 238, 0.25);
}

.dex-progress {
  background: linear-gradient(90deg, rgb(254 240 138), rgb(74 222 128));
}

.badge-tile {
  border: 1px solid rgba(250, 204, 21, 0.14);
  background:
    radial-gradient(circle at 15% 50%, rgba(250, 204, 21, 0.09), transparent 3rem),
    rgba(113, 63, 18, 0.1);
  box-shadow: inset 0 1px rgba(255, 255, 255, 0.025);
}

.milestone-chip {
  border: 1px solid rgba(148, 163, 184, 0.11);
  background: rgba(255, 255, 255, 0.035);
}

.pokeball-mark {
  position: relative;
  display: inline-block;
  flex: none;
  width: 1.3rem;
  height: 1.3rem;
  border: 1px solid rgba(148, 163, 184, 0.42);
  border-radius: 9999px;
  background: linear-gradient(to bottom, rgb(185 28 28) 0 44%, rgb(30 41 59) 44% 56%, rgb(226 232 240) 56% 100%);
}

.pokeball-mark > span {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 31%;
  aspect-ratio: 1;
  transform: translate(-50%, -50%);
  border: 1px solid rgb(71 85 105);
  border-radius: 9999px;
  background: rgb(226 232 240);
}
</style>
