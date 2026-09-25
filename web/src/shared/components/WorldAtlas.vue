<script lang="ts">
export interface WorldAtlasMarker {
  map: number
  x: number
  y: number
  label?: string
}
</script>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, shallowRef } from 'vue'
import { mapEntry } from '../mapCatalog'
import { drawGen1TextureMap, loadGen1TextureMap, type Gen1TextureMap } from '../gen1Texture'
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

const RENDER_CELL_PX = 4
const zoom = ref(1)
const loading = ref(true)
const failedMaps = ref(0)
const canvas = ref<HTMLCanvasElement | null>(null)
const atlasTextures = shallowRef<Map<number, Gen1TextureMap>>(new Map())
const padding = 14
const atlasMaps = WORLD_ATLAS_MAPS

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

const canvasWidth = computed(() => Math.max(1, Math.ceil(bounds.value.width * RENDER_CELL_PX)))
const canvasHeight = computed(() => Math.max(1, Math.ceil(bounds.value.height * RENDER_CELL_PX)))
const loadedMaps = computed(() => atlasTextures.value.size)

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

function hasTexture(id: number): boolean {
  return atlasTextures.value.has(id)
}

function zoomBy(delta: number): void {
  zoom.value = Math.max(0.7, Math.min(3, Math.round((zoom.value + delta) * 10) / 10))
}

function drawAtlas(): void {
  const node = canvas.value
  if (!node) return
  const ctx = node.getContext('2d')
  if (!ctx) return

  node.width = canvasWidth.value
  node.height = canvasHeight.value
  ctx.imageSmoothingEnabled = false
  ctx.fillStyle = '#07100c'
  ctx.fillRect(0, 0, node.width, node.height)

  for (const map of atlasMaps) {
    const texture = atlasTextures.value.get(map.id)
    if (!texture) continue
    const x = (Number(map.x || 0) - bounds.value.x) * RENDER_CELL_PX
    const y = (Number(map.y || 0) - bounds.value.y) * RENDER_CELL_PX
    drawGen1TextureMap(ctx, texture, RENDER_CELL_PX, x, y)
  }
}

async function loadAtlas(): Promise<void> {
  loading.value = true
  failedMaps.value = 0

  const settled = await Promise.allSettled(
    atlasMaps.map(async (map) => {
      const texture = await loadGen1TextureMap(map.id)
      return texture ? [map.id, texture] as const : null
    })
  )

  const next = new Map<number, Gen1TextureMap>()
  let failed = 0
  for (const result of settled) {
    if (result.status === 'fulfilled' && result.value) {
      next.set(result.value[0], result.value[1])
    } else {
      failed++
    }
  }

  atlasTextures.value = next
  failedMaps.value = failed
  loading.value = false
  await nextTick()
  drawAtlas()
}

onMounted(() => {
  void loadAtlas()
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col bg-[#08110d]">
    <div class="flex flex-wrap items-center justify-between gap-2 border-b border-white/10 bg-black/20 px-3 py-2">
      <div>
        <div class="flex items-center gap-2">
          <div class="text-[10px] font-bold tracking-[0.1em] text-emerald-100 uppercase">Kanto atlas</div>
          <span v-if="!loading && loadedMaps" class="rounded-full bg-emerald-300/8 px-2 py-0.5 text-[9px] font-semibold text-emerald-100 ring-1 ring-emerald-300/15">
            {{ loadedMaps }} decomp maps
          </span>
        </div>
        <div class="mt-0.5 text-[9px] text-slate-500">
          Stitched from the same decomp maps as the detailed view. Click a city or route to open it.
        </div>
      </div>
      <div class="flex items-center gap-1">
        <span v-if="loading" class="mr-2 text-[9px] text-slate-500">Loading Kanto art…</span>
        <span v-else-if="failedMaps" class="mr-2 text-[9px] text-amber-300">{{ failedMaps }} map{{ failedMaps === 1 ? '' : 's' }} unavailable</span>
        <button type="button" class="rounded bg-white/7 px-2 py-1 text-xs text-slate-300 ring-1 ring-white/10 hover:bg-white/12" @click="zoomBy(-0.2)">−</button>
        <button type="button" class="min-w-14 rounded bg-white/7 px-2 py-1 font-mono text-[10px] text-slate-300 ring-1 ring-white/10 hover:bg-white/12" @click="zoom = 1">{{ Math.round(zoom * 100) }}%</button>
        <button type="button" class="rounded bg-white/7 px-2 py-1 text-xs text-slate-300 ring-1 ring-white/10 hover:bg-white/12" @click="zoomBy(0.2)">+</button>
      </div>
    </div>

    <div class="min-h-0 flex-1 overflow-auto p-3">
      <div
        :style="{ width: `${zoom * 100}%`, minWidth: '100%' }"
        class="relative mx-auto overflow-hidden rounded-xl border border-white/8 bg-[#07100c] shadow-inner shadow-black/50"
      >
        <canvas
          ref="canvas"
          :width="canvasWidth"
          :height="canvasHeight"
          class="block h-auto w-full [image-rendering:pixelated]"
          aria-hidden="true"
        />

        <svg
          :viewBox="`${bounds.x} ${bounds.y} ${bounds.width} ${bounds.height}`"
          class="absolute inset-0 h-full w-full"
          role="img"
          aria-label="Clickable stitched Kanto world atlas"
        >
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
              :rx="isCity(map.id) ? 2.2 : 0.8"
              :class="[
                'atlas-hit',
                map.id === selectedMap ? 'atlas-selected' : '',
                !hasTexture(map.id) && !loading ? 'atlas-missing' : ''
              ]"
            />
            <text
              v-if="isCity(map.id) || zoom >= 1.1"
              :x="Number(map.x) + map.width / 2"
              :y="Number(map.y) + map.height / 2"
              dominant-baseline="middle"
              text-anchor="middle"
              :class="isCity(map.id) ? 'atlas-city-label' : 'atlas-route-label'"
            >{{ shortLabel(map.id) }}</text>
          </g>

          <g v-for="(marker, index) in placedMarkers" :key="`${marker.map}-${index}-${marker.label || ''}`">
            <title>{{ marker.label || `Agent on ${labelFor(marker.map)}` }}</title>
            <circle :cx="marker.gx" :cy="marker.gy" r="4.5" fill="none" stroke="#67e8f9" stroke-width="1.3" opacity="0.55" />
            <circle :cx="marker.gx" :cy="marker.gy" r="2.3" fill="#ecfeff" stroke="#082f49" stroke-width="1" />
          </g>
        </svg>
      </div>
    </div>
  </div>
</template>

<style scoped>
.atlas-hit {
  fill: transparent;
  stroke: rgba(255, 255, 255, 0.07);
  stroke-width: 0.65;
  vector-effect: non-scaling-stroke;
  transition: fill 120ms ease, stroke 120ms ease;
}

g:hover > .atlas-hit,
g:focus > .atlas-hit {
  fill: rgba(110, 231, 183, 0.14);
  stroke: rgba(209, 250, 229, 0.72);
}

.atlas-selected {
  fill: rgba(103, 232, 249, 0.12);
  stroke: #67e8f9;
  stroke-width: 2;
  filter: drop-shadow(0 0 3px rgba(103, 232, 249, 0.72));
}

.atlas-missing {
  fill: rgba(245, 158, 11, 0.12);
  stroke: rgba(251, 191, 36, 0.55);
}

.atlas-city-label,
.atlas-route-label {
  pointer-events: none;
  fill: rgba(248, 250, 252, 0.96);
  font-family: ui-sans-serif, system-ui, sans-serif;
  font-weight: 800;
  letter-spacing: 0.02em;
  paint-order: stroke;
  stroke: rgba(0, 0, 0, 0.9);
  stroke-width: 1.7px;
}

.atlas-city-label {
  font-size: 4.5px;
}

.atlas-route-label {
  font-size: 3.5px;
}
</style>
