<script lang="ts">
export interface WorldAtlasMarker {
  map: number
  x: number
  y: number
  label?: string
}
</script>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { mapEntry } from '../mapCatalog'
import { WORLD_ATLAS_MAPS, worldMapMeta } from '../worldManifest'

const props = withDefaults(defineProps<{
  selectedMap?: number
  markers?: WorldAtlasMarker[]
}>(), {
  selectedMap: 0,
  markers: () => []
})

const emit = defineEmits<{
  selectMap: [map: number]
}>()

const zoom = ref(1)
const padding = 14
const atlasMaps = WORLD_ATLAS_MAPS
const byID = new Map(atlasMaps.map((map) => [map.id, map] as const))

const bounds = computed(() => {
  const minX = Math.min(...atlasMaps.map((map) => Number(map.x || 0)))
  const minY = Math.min(...atlasMaps.map((map) => Number(map.y || 0)))
  const maxX = Math.max(...atlasMaps.map((map) => Number(map.x || 0) + map.width))
  const maxY = Math.max(...atlasMaps.map((map) => Number(map.y || 0) + map.height))
  return {
    x: minX - padding,
    y: minY - padding,
    width: maxX - minX + padding * 2,
    height: maxY - minY + padding * 2
  }
})

const edges = computed(() => {
  const seen = new Set<string>()
  const result: { from: number; to: number; x1: number; y1: number; x2: number; y2: number }[] = []
  for (const map of atlasMaps) {
    for (const connection of map.connections) {
      const destination = byID.get(connection.to)
      if (!destination) continue
      const key = [map.id, destination.id].sort((a, b) => a - b).join(':')
      if (seen.has(key)) continue
      seen.add(key)
      result.push({
        from: map.id,
        to: destination.id,
        x1: Number(map.x || 0) + map.width / 2,
        y1: Number(map.y || 0) + map.height / 2,
        x2: Number(destination.x || 0) + destination.width / 2,
        y2: Number(destination.y || 0) + destination.height / 2
      })
    }
  }
  return result
})

const placedMarkers = computed(() => props.markers.flatMap((marker, index) => {
  const map = worldMapMeta(marker.map)
  if (!map || !Number.isFinite(map.x) || !Number.isFinite(map.y)) return []
  const jitter = (index % 4) * 1.6
  return [{
    ...marker,
    gx: Number(map.x) + Number(marker.x || 0) + jitter,
    gy: Number(map.y) + Number(marker.y || 0) + jitter
  }]
}))

function labelFor(id: number): string {
  return mapEntry(id)?.label || `Map ${id}`
}

function shortLabel(id: number): string {
  const label = labelFor(id)
  if (label.startsWith('Route ')) return label.replace('Route ', 'R')
  return label
}

function isCity(id: number): boolean {
  return id <= 0x0a
}

function zoomBy(delta: number): void {
  zoom.value = Math.max(0.7, Math.min(3, Math.round((zoom.value + delta) * 10) / 10))
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col bg-[#08110d]">
    <div class="flex flex-wrap items-center justify-between gap-2 border-b border-white/10 bg-black/20 px-3 py-2">
      <div>
        <div class="text-[10px] font-bold tracking-[0.1em] text-emerald-100 uppercase">Kanto atlas</div>
        <div class="mt-0.5 text-[9px] text-slate-500">Click any city or route to open its detailed map.</div>
      </div>
      <div class="flex items-center gap-1">
        <button type="button" class="rounded bg-white/7 px-2 py-1 text-xs text-slate-300 ring-1 ring-white/10 hover:bg-white/12" @click="zoomBy(-0.2)">−</button>
        <button type="button" class="min-w-14 rounded bg-white/7 px-2 py-1 font-mono text-[10px] text-slate-300 ring-1 ring-white/10 hover:bg-white/12" @click="zoom = 1">{{ Math.round(zoom * 100) }}%</button>
        <button type="button" class="rounded bg-white/7 px-2 py-1 text-xs text-slate-300 ring-1 ring-white/10 hover:bg-white/12" @click="zoomBy(0.2)">+</button>
      </div>
    </div>

    <div class="min-h-0 flex-1 overflow-auto p-3">
      <svg
        :viewBox="`${bounds.x} ${bounds.y} ${bounds.width} ${bounds.height}`"
        :style="{ width: `${zoom * 100}%`, minWidth: '100%', height: 'auto' }"
        class="mx-auto block rounded-xl border border-white/8 bg-[#0b1711] shadow-inner shadow-black/50"
        role="img"
        aria-label="Clickable Kanto world atlas"
      >
        <g opacity="0.24" stroke="#9bd9ae" stroke-width="1.2">
          <line v-for="edge in edges" :key="`${edge.from}-${edge.to}`" :x1="edge.x1" :y1="edge.y1" :x2="edge.x2" :y2="edge.y2" />
        </g>

        <g
          v-for="map in atlasMaps"
          :key="map.id"
          role="button"
          tabindex="0"
          class="cursor-pointer outline-none"
          @click="emit('selectMap', map.id)"
          @keydown.enter.prevent="emit('selectMap', map.id)"
          @keydown.space.prevent="emit('selectMap', map.id)"
        >
          <title>{{ labelFor(map.id) }}</title>
          <rect
            :x="map.x"
            :y="map.y"
            :width="map.width"
            :height="map.height"
            :rx="isCity(map.id) ? 3 : 1.5"
            :class="[
              map.id === selectedMap ? 'atlas-selected' : '',
              isCity(map.id) ? 'atlas-city' : 'atlas-route'
            ]"
          />
          <text
            :x="Number(map.x) + map.width / 2"
            :y="Number(map.y) + map.height / 2"
            dominant-baseline="middle"
            text-anchor="middle"
            :class="isCity(map.id) ? 'atlas-city-label' : 'atlas-route-label'"
          >{{ shortLabel(map.id) }}</text>
        </g>

        <g v-for="(marker, index) in placedMarkers" :key="`${marker.map}-${index}-${marker.label || ''}`">
          <title>{{ marker.label || `Agent on ${labelFor(marker.map)}` }}</title>
          <circle :cx="marker.gx" :cy="marker.gy" r="4.5" fill="none" stroke="#67e8f9" stroke-width="1.3" opacity="0.45" />
          <circle :cx="marker.gx" :cy="marker.gy" r="2.3" fill="#ecfeff" stroke="#082f49" stroke-width="1" />
        </g>
      </svg>
    </div>
  </div>
</template>

<style scoped>
.atlas-city,
.atlas-route {
  vector-effect: non-scaling-stroke;
  transition: filter 120ms ease, stroke 120ms ease, opacity 120ms ease;
}

.atlas-city {
  fill: #315b4b;
  stroke: rgba(167, 243, 208, 0.58);
  stroke-width: 1.1;
}

.atlas-route {
  fill: #667247;
  stroke: rgba(236, 252, 203, 0.36);
  stroke-width: 0.9;
}

g:hover > .atlas-city,
g:hover > .atlas-route,
g:focus > .atlas-city,
g:focus > .atlas-route {
  filter: brightness(1.28);
  stroke: #e2fbe9;
}

.atlas-selected {
  stroke: #67e8f9;
  stroke-width: 2.4;
  filter: drop-shadow(0 0 3px rgba(103, 232, 249, 0.7));
}

.atlas-city-label,
.atlas-route-label {
  pointer-events: none;
  fill: rgba(240, 253, 244, 0.92);
  font-family: ui-sans-serif, system-ui, sans-serif;
  font-weight: 800;
  letter-spacing: 0.02em;
  paint-order: stroke;
  stroke: rgba(5, 15, 10, 0.8);
  stroke-width: 1.5px;
}

.atlas-city-label {
  font-size: 5px;
}

.atlas-route-label {
  font-size: 4px;
}
</style>
