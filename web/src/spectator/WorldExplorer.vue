<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { SpectatorRun } from '../shared/api/spectator'
import SemanticMap from '../shared/components/SemanticMap.vue'
import { locationLabel } from './model'

const props = defineProps<{
  run: SpectatorRun
}>()

const selectedMap = ref(Number(props.run.map || 0))
const history = ref<number[]>([])
const followLive = ref(true)
const showTrail = ref(true)
const showSprites = ref(true)
const showWarps = ref(true)
const developerOverlay = ref(false)

const liveMap = computed(() => Number(props.run.map || 0))
const isLiveMap = computed(() => selectedMap.value === liveMap.value)
const mapID = computed(() => Math.max(0, selectedMap.value).toString(16).padStart(2, '0').toUpperCase())
const mapTrail = computed<[number, number][]>(() => isLiveMap.value ? props.run.trail || [] : [])
const mapSprites = computed(() => isLiveMap.value ? props.run.sprites || [] : [])

watch(() => props.run.run_id, () => {
  selectedMap.value = Number(props.run.map || 0)
  history.value = []
  followLive.value = true
})

watch(liveMap, (next) => {
  if (followLive.value) selectedMap.value = next
})

function exploreMap(destination: number): void {
  const next = Math.max(0, Math.min(255, Math.trunc(Number(destination))))
  if (!Number.isFinite(next) || next === selectedMap.value) return
  history.value.push(selectedMap.value)
  selectedMap.value = next
  followLive.value = next === liveMap.value
}

function goBack(): void {
  const previous = history.value.pop()
  if (previous === undefined) return
  selectedMap.value = previous
  followLive.value = previous === liveMap.value
}

function returnToLive(): void {
  selectedMap.value = liveMap.value
  history.value = []
  followLive.value = true
}
</script>

<template>
  <div class="overflow-hidden rounded-lg border border-white/10 bg-[#080d14]">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-white/10 bg-[#0d141e] px-3 py-2.5">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <strong class="text-xs text-white">World explorer</strong>
          <span class="rounded bg-black/30 px-1.5 py-0.5 font-mono text-[9px] text-slate-400 ring-1 ring-white/10">MAP {{ mapID }}</span>
          <span v-if="isLiveMap" class="rounded bg-emerald-300/10 px-1.5 py-0.5 text-[9px] font-bold text-emerald-200 ring-1 ring-emerald-300/20">LIVE</span>
          <span v-else class="rounded bg-purple-300/10 px-1.5 py-0.5 text-[9px] font-bold text-purple-200 ring-1 ring-purple-300/20">EXPLORING</span>
        </div>
        <p class="mt-1 truncate text-[10px] text-slate-500">
          {{ isLiveMap ? locationLabel(run) : 'Follow a highlighted warp to move between ROM maps.' }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-1.5">
        <button
          v-if="history.length"
          type="button"
          class="rounded bg-white/7 px-2 py-1 text-[10px] font-semibold text-slate-300 ring-1 ring-white/10 hover:bg-white/12 hover:text-white"
          @click="goBack"
        >
          Back
        </button>
        <button
          v-if="!isLiveMap"
          type="button"
          class="rounded bg-emerald-300/10 px-2 py-1 text-[10px] font-semibold text-emerald-100 ring-1 ring-emerald-300/20 hover:bg-emerald-300/15"
          @click="returnToLive"
        >
          Return live
        </button>
      </div>
    </div>

    <div class="grid min-h-[30rem] grid-rows-[minmax(0,1fr)_auto] lg:min-h-[36rem]">
      <SemanticMap
        :map="selectedMap"
        :x="run.x"
        :y="run.y"
        :trail="mapTrail"
        :sprites="mapSprites"
        :show-player="isLiveMap"
        :show-trail="showTrail && isLiveMap"
        :show-sprites="showSprites && isLiveMap"
        :show-warps="showWarps"
        :debug="developerOverlay"
        interactive
        @warp-select="exploreMap"
      />

      <div class="flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-white/10 bg-[#0d141e] px-3 py-2 text-[10px] text-slate-400">
        <label class="inline-flex cursor-pointer items-center gap-1.5">
          <input v-model="showTrail" type="checkbox" class="size-3 accent-cyan-300" />
          Trail
        </label>
        <label class="inline-flex cursor-pointer items-center gap-1.5">
          <input v-model="showSprites" type="checkbox" class="size-3 accent-amber-300" />
          NPCs / objects
        </label>
        <label class="inline-flex cursor-pointer items-center gap-1.5">
          <input v-model="showWarps" type="checkbox" class="size-3 accent-purple-300" />
          Warps
        </label>
        <label class="ml-auto inline-flex cursor-pointer items-center gap-1.5 text-slate-500">
          <input v-model="developerOverlay" type="checkbox" class="size-3 accent-cyan-300" />
          Developer overlay
        </label>
      </div>
    </div>
  </div>
</template>
