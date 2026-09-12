<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import type { MapSprite } from '../shared/api/types'

interface MapPayload {
  width?: number
  height?: number
  cells?: string | string[]
  fallback?: boolean
}

const props = withDefaults(defineProps<{
  map?: number
  x?: number
  y?: number
  trail?: [number, number][]
  sprites?: MapSprite[]
}>(), {
  map: 0,
  x: 0,
  y: 0,
  trail: () => [],
  sprites: () => []
})

const canvas = ref<HTMLCanvasElement | null>(null)
const loading = ref(false)
const error = ref('')
let payload: MapPayload | null = null
let serial = 0

function mapName(): string {
  return `${Number(props.map || 0).toString(16).padStart(2, '0')}.json`
}

function cellAt(data: MapPayload, x: number, y: number): string {
  const width = Math.max(1, Number(data.width || 1))
  if (typeof data.cells === 'string') return data.cells[y * width + x] || '.'
  if (Array.isArray(data.cells)) {
    const row = data.cells[y] || ''
    return row[x] || '.'
  }
  return '.'
}

function draw(): void {
  const node = canvas.value
  const data = payload
  if (!node || !data) return
  const width = Math.max(1, Number(data.width || 1))
  const height = Math.max(1, Number(data.height || 1))
  node.width = width
  node.height = height
  const ctx = node.getContext('2d')
  if (!ctx) return

  ctx.imageSmoothingEnabled = false
  ctx.fillStyle = '#080d14'
  ctx.fillRect(0, 0, width, height)

  ctx.fillStyle = '#1f3141'
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      const cell = cellAt(data, x, y)
      if (cell !== '.' && cell !== '0' && cell !== ' ') ctx.fillRect(x, y, 1, 1)
    }
  }

  ctx.fillStyle = 'rgba(85, 215, 255, 0.35)'
  for (const point of props.trail || []) {
    if (point.length >= 2) ctx.fillRect(Number(point[0]), Number(point[1]), 1, 1)
  }

  ctx.fillStyle = '#f1b85b'
  for (const sprite of props.sprites || []) ctx.fillRect(Number(sprite.x), Number(sprite.y), 1, 1)

  ctx.fillStyle = '#4dd99a'
  ctx.fillRect(Number(props.x || 0), Number(props.y || 0), 1, 1)
}

async function loadMap(): Promise<void> {
  const id = ++serial
  loading.value = true
  error.value = ''
  try {
    const response = await fetch(`/maps/${mapName()}`, { cache: 'no-store' })
    if (!response.ok) throw new Error(`Map ${mapName()} unavailable (${response.status})`)
    const next = await response.json() as MapPayload
    if (id !== serial) return
    payload = next
    draw()
  } catch (cause) {
    if (id !== serial) return
    payload = null
    error.value = cause instanceof Error ? cause.message : 'Map unavailable'
  } finally {
    if (id === serial) loading.value = false
  }
}

watch(() => props.map, () => { void loadMap() }, { immediate: true })
watch([() => props.x, () => props.y, () => props.trail, () => props.sprites], draw, { deep: true })
onMounted(draw)
</script>

<template>
  <div class="relative grid h-[clamp(15rem,30vh,22rem)] place-items-center overflow-hidden rounded-md border border-white/10 bg-black/30 p-2">
    <canvas ref="canvas" class="max-h-full max-w-full [image-rendering:pixelated]" aria-label="Semantic map" />
    <div class="pointer-events-none absolute top-2 left-2 rounded-md bg-black/70 px-2 py-1 font-mono text-[10px] text-slate-300 ring-1 ring-white/10">
      map 0x{{ Number(map || 0).toString(16).padStart(2, '0') }} · {{ x }},{{ y }}
    </div>
    <div v-if="loading" class="pointer-events-none absolute right-2 bottom-2 rounded-md bg-black/70 px-2 py-1 text-[10px] font-semibold text-slate-400 ring-1 ring-white/10">Loading map…</div>
    <div v-else-if="error" class="pointer-events-none absolute right-2 bottom-2 max-w-[80%] rounded-md bg-rose-950/80 px-2 py-1 text-[10px] text-rose-200 ring-1 ring-rose-300/20">{{ error }}</div>
  </div>
</template>
