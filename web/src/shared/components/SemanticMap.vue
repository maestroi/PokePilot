<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import type { MapSprite } from '../api/types'

interface MapWarp {
  x: number
  y: number
  dest: number
}

interface MapPayload {
  id?: number
  width?: number
  height?: number
  cells?: string | string[]
  warps?: MapWarp[]
  connections?: string[]
  fallback?: boolean
}

const props = withDefaults(defineProps<{
  map?: number
  x?: number
  y?: number
  trail?: [number, number][]
  sprites?: MapSprite[]
  debug?: boolean
  interactive?: boolean
  showTrail?: boolean
  showSprites?: boolean
  showWarps?: boolean
}>(), {
  map: 0,
  x: 0,
  y: 0,
  trail: () => [],
  sprites: () => [],
  debug: false,
  interactive: false,
  showTrail: true,
  showSprites: true,
  showWarps: true
})

const emit = defineEmits<{
  warpSelect: [destination: number]
}>()

const frame = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)
const loading = ref(false)
const error = ref('')
const zoomLevel = ref(1)
let savedDebug = false
try {
  savedDebug = window.localStorage.getItem('pokepilot.map.debug') === '1'
} catch {
  // Storage may be unavailable in hardened/private browser contexts.
}
const localDebug = ref(new URLSearchParams(window.location.search).get('debug') === '1' || savedDebug)
const debugEnabled = computed(() => props.debug || localDebug.value)
const warpDestinations = computed(() => {
  const seen = new Set<number>()
  const result: number[] = []
  for (const warp of payload?.warps || []) {
    const destination = Number(warp.dest)
    if (!Number.isFinite(destination) || seen.has(destination)) continue
    seen.add(destination)
    result.push(destination)
  }
  return result
})
const connections = computed(() => payload?.connections || [])
let payload: MapPayload | null = null
let serial = 0
let observer: ResizeObserver | null = null
let tileSize = 0

function mapName(): string {
  return `${Number(props.map || 0).toString(16).padStart(2, '0')}.json`
}

function mapLabel(value: number): string {
  return `Map ${Math.max(0, Number(value || 0)).toString(16).padStart(2, '0').toUpperCase()}`
}

function cellAt(data: MapPayload, x: number, y: number): string {
  const width = Math.max(1, Number(data.width || 1))
  if (typeof data.cells === 'string') return data.cells[y * width + x] || '#'
  if (Array.isArray(data.cells)) {
    const row = data.cells[y] || ''
    return row[x] || '#'
  }
  return '#'
}

function token(name: string, fallback: string): string {
  const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return value || fallback
}

function hexByte(value: unknown): string {
  const n = Number(value)
  return Number.isFinite(n) ? Math.max(0, n).toString(16).padStart(2, '0').slice(-2).toUpperCase() : '--'
}

function drawDebugText(ctx: CanvasRenderingContext2D, text: string, x: number, y: number, px: number): void {
  const fontSize = Math.max(7, Math.floor(px * 0.38))
  ctx.font = `700 ${fontSize}px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace`
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  const metrics = ctx.measureText(text)
  const padX = Math.max(2, Math.floor(px * 0.12))
  const boxW = metrics.width + padX * 2
  const boxH = fontSize + 3
  ctx.fillStyle = 'rgba(0, 0, 0, 0.82)'
  ctx.fillRect(x - boxW / 2, y - boxH / 2, boxW, boxH)
  ctx.fillStyle = '#f8fbff'
  ctx.fillText(text, x, y + 0.5)
}

function draw(): void {
  const node = canvas.value
  const box = frame.value
  const data = payload
  if (!node || !box || !data) return

  const width = Math.max(1, Number(data.width || 1))
  const height = Math.max(1, Number(data.height || 1))
  const style = getComputedStyle(box)
  const availW = Math.max(1, box.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight))
  const availH = Math.max(1, box.clientHeight - parseFloat(style.paddingTop) - parseFloat(style.paddingBottom))
  if (availW < 8 || availH < 8) return

  const fitPx = Math.floor(Math.min(availW / width, availH / height))
  const basePx = Math.max(debugEnabled.value ? 18 : 6, fitPx)
  const px = Math.max(2, Math.floor(basePx * zoomLevel.value))
  tileSize = px
  node.width = width * px
  node.height = height * px
  const ctx = node.getContext('2d')
  if (!ctx) return

  ctx.imageSmoothingEnabled = false
  const colors = {
    ground: token('--map-ground', '#102229'),
    wall: token('--map-wall', '#40515d'),
    grass: token('--map-grass', '#28543c'),
    water: token('--map-water', '#1e5f78'),
    warp: token('--map-warp', '#c999ef'),
    trail: token('--map-trail', '#61e2ee'),
    sprite: token('--map-sprite', '#efb24f'),
    player: token('--map-player', '#f2fbff')
  }

  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      const cell = cellAt(data, x, y)
      ctx.fillStyle = cell === '#' ? colors.wall : cell === 'g' ? colors.grass : cell === '~' ? colors.water : colors.ground
      ctx.fillRect(x * px, y * px, px, px)
      if (props.showWarps && cell === 'W') {
        ctx.strokeStyle = colors.warp
        ctx.lineWidth = Math.max(1, Math.floor(px / 4))
        ctx.strokeRect(x * px + 1, y * px + 1, Math.max(1, px - 2), Math.max(1, px - 2))
      }
    }
  }

  if (debugEnabled.value) {
    ctx.strokeStyle = 'rgba(255, 255, 255, 0.10)'
    ctx.lineWidth = 1
    ctx.beginPath()
    for (let x = 1; x < width; x++) {
      ctx.moveTo(x * px + 0.5, 0)
      ctx.lineTo(x * px + 0.5, height * px)
    }
    for (let y = 1; y < height; y++) {
      ctx.moveTo(0, y * px + 0.5)
      ctx.lineTo(width * px, y * px + 0.5)
    }
    ctx.stroke()

    if (props.showWarps) {
      for (const warp of data.warps || []) {
        const x = Number(warp.x)
        const y = Number(warp.y)
        if (x < 0 || y < 0 || x >= width || y >= height) continue
        drawDebugText(ctx, `→${hexByte(warp.dest)}`, (x + 0.5) * px, (y + 0.5) * px, px)
      }
    }
  }

  const trail = props.showTrail ? props.trail || [] : []
  if (trail.length > 1) {
    ctx.strokeStyle = colors.trail
    ctx.lineWidth = Math.max(1, Math.floor(px / 3))
    ctx.globalAlpha = 0.85
    ctx.beginPath()
    trail.forEach((point, index) => {
      const x = (Number(point[0]) + 0.5) * px
      const y = (Number(point[1]) + 0.5) * px
      if (index === 0) ctx.moveTo(x, y)
      else ctx.lineTo(x, y)
    })
    ctx.stroke()
    ctx.globalAlpha = 1
  } else {
    ctx.fillStyle = 'rgba(85, 215, 255, 0.35)'
    for (const point of trail) {
      if (point.length >= 2) ctx.fillRect(Number(point[0]) * px, Number(point[1]) * px, px, px)
    }
  }

  if (props.showSprites) {
    for (const sprite of props.sprites || []) {
      const x = Number(sprite.x)
      const y = Number(sprite.y)
      if (x < 0 || y < 0 || x >= width || y >= height) continue
      const pad = Math.max(1, Math.floor(px / 4))
      ctx.fillStyle = colors.sprite
      ctx.fillRect(x * px + pad, y * px + pad, Math.max(2, px - pad * 2), Math.max(2, px - pad * 2))
      if (debugEnabled.value) {
        const slot = Number(sprite.slot)
        const slotLabel = Number.isFinite(slot) && slot > 0 ? String(slot) : '?'
        drawDebugText(ctx, `S${slotLabel}/${hexByte(sprite.picture_id)}`, (x + 0.5) * px, (y + 0.5) * px, px)
      }
    }
  }

  const playerX = Number(props.x)
  const playerY = Number(props.y)
  if (Number.isFinite(playerX) && Number.isFinite(playerY) && playerX >= 0 && playerY >= 0 && playerX < width && playerY < height) {
    ctx.fillStyle = colors.player
    ctx.beginPath()
    ctx.arc((playerX + 0.5) * px, (playerY + 0.5) * px, Math.max(2, px * 0.42), 0, Math.PI * 2)
    ctx.fill()
    if (debugEnabled.value) {
      drawDebugText(ctx, `@${playerX},${playerY}`, (playerX + 0.5) * px, (playerY + 0.5) * px, px)
    }
  }
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
    zoomLevel.value = 1
    requestAnimationFrame(draw)
  } catch (cause) {
    if (id !== serial) return
    payload = null
    error.value = cause instanceof Error ? cause.message : 'Map unavailable'
  } finally {
    if (id === serial) loading.value = false
  }
}

function zoomBy(delta: number): void {
  zoomLevel.value = Math.max(0.5, Math.min(4, Math.round((zoomLevel.value + delta) * 4) / 4))
  requestAnimationFrame(draw)
}

function resetZoom(): void {
  zoomLevel.value = 1
  requestAnimationFrame(draw)
}

function centerOnPlayer(): void {
  const box = frame.value
  const node = canvas.value
  if (!box || !node || tileSize <= 0) return
  const playerX = Number(props.x)
  const playerY = Number(props.y)
  if (!Number.isFinite(playerX) || !Number.isFinite(playerY)) return
  box.scrollTo({
    left: Math.max(0, node.offsetLeft + (playerX + 0.5) * tileSize - box.clientWidth / 2),
    top: Math.max(0, node.offsetTop + (playerY + 0.5) * tileSize - box.clientHeight / 2),
    behavior: 'smooth'
  })
}

function onCanvasClick(event: MouseEvent): void {
  if (!props.interactive || !props.showWarps || !payload || !canvas.value || tileSize <= 0) return
  const rect = canvas.value.getBoundingClientRect()
  if (rect.width <= 0 || rect.height <= 0) return
  const intrinsicX = (event.clientX - rect.left) * canvas.value.width / rect.width
  const intrinsicY = (event.clientY - rect.top) * canvas.value.height / rect.height
  const tileX = Math.floor(intrinsicX / tileSize)
  const tileY = Math.floor(intrinsicY / tileSize)
  const warp = (payload.warps || []).find((candidate) => Number(candidate.x) === tileX && Number(candidate.y) === tileY)
  if (warp) emit('warpSelect', Number(warp.dest))
}

watch(() => props.map, () => { void loadMap() }, { immediate: true })
watch([
  () => props.x,
  () => props.y,
  () => props.trail,
  () => props.sprites,
  () => props.showTrail,
  () => props.showSprites,
  () => props.showWarps,
  () => debugEnabled.value
], () => requestAnimationFrame(draw), { deep: true })
watch(localDebug, (enabled) => {
  try {
    window.localStorage.setItem('pokepilot.map.debug', enabled ? '1' : '0')
  } catch {
    // Debug mode still works for the current page when storage is unavailable.
  }
})

onMounted(() => {
  observer = new ResizeObserver(() => requestAnimationFrame(draw))
  if (frame.value) observer.observe(frame.value)
  requestAnimationFrame(draw)
})

onUnmounted(() => {
  observer?.disconnect()
  observer = null
})
</script>

<template>
  <div class="flex h-full min-h-0 w-full flex-col bg-[#0c1118]">
    <div v-if="interactive" class="flex flex-wrap items-center justify-between gap-2 border-b border-white/10 bg-black/20 px-2.5 py-2">
      <div class="flex min-w-0 items-center gap-2">
        <strong class="font-mono text-[11px] text-white">{{ mapLabel(map) }}</strong>
        <span v-if="connections.length" class="truncate text-[10px] text-slate-500">edges: {{ connections.join(' · ') }}</span>
      </div>
      <div class="flex items-center gap-1">
        <button type="button" class="rounded bg-white/7 px-2 py-1 text-xs text-slate-300 ring-1 ring-white/10 hover:bg-white/12" aria-label="Zoom out" @click="zoomBy(-0.25)">−</button>
        <button type="button" class="min-w-14 rounded bg-white/7 px-2 py-1 font-mono text-[10px] text-slate-300 ring-1 ring-white/10 hover:bg-white/12" title="Reset zoom" @click="resetZoom">{{ Math.round(zoomLevel * 100) }}%</button>
        <button type="button" class="rounded bg-white/7 px-2 py-1 text-xs text-slate-300 ring-1 ring-white/10 hover:bg-white/12" aria-label="Zoom in" @click="zoomBy(0.25)">+</button>
        <button type="button" class="rounded bg-white/7 px-2 py-1 text-[10px] font-semibold text-slate-300 ring-1 ring-white/10 hover:bg-white/12" @click="centerOnPlayer">Locate</button>
      </div>
    </div>

    <div
      ref="frame"
      :class="[
        interactive || debugEnabled ? 'place-items-start overflow-auto' : 'place-items-center overflow-hidden',
        'relative grid min-h-0 flex-1 p-1.5'
      ]"
    >
      <canvas
        ref="canvas"
        :class="[
          interactive || debugEnabled ? 'max-w-none' : 'max-h-full max-w-full',
          interactive && showWarps ? 'cursor-crosshair' : '',
          'shrink-0 [image-rendering:pixelated]'
        ]"
        aria-label="Semantic map"
        @click="onCanvasClick"
      />
      <button
        type="button"
        :aria-pressed="debugEnabled"
        :title="debugEnabled ? 'Hide map debug labels' : 'Show map debug labels'"
        :class="[
          debugEnabled ? 'bg-[var(--poke-cyan)] text-[#101820]' : 'bg-black/75 text-[var(--poke-muted)] hover:text-white',
          'sticky top-1.5 ml-auto z-10 rounded-sm px-1.5 py-0.5 font-mono text-[9px] font-bold ring-1 ring-white/10'
        ]"
        @click="localDebug = !localDebug"
      >
        {{ debugEnabled ? 'DEBUG ON' : 'DEBUG' }}
      </button>
      <div v-if="debugEnabled" class="pointer-events-none sticky bottom-1.5 left-1.5 z-10 mt-auto mr-auto bg-black/80 px-1.5 py-1 font-mono text-[9px] leading-3 text-[var(--poke-muted)] ring-1 ring-white/10">
        <div><span class="text-white">S#/PP</span> sprite slot / picture ID</div>
        <div><span class="text-white">→MM</span> warp destination map</div>
      </div>
      <div v-if="loading" class="pointer-events-none sticky right-1.5 bottom-1.5 ml-auto mt-auto bg-black/70 px-1.5 py-0.5 text-[10px] text-[var(--poke-muted)]">Loading map…</div>
      <div v-else-if="error" class="pointer-events-none sticky right-1.5 bottom-1.5 ml-auto mt-auto max-w-[80%] bg-[#352529] px-1.5 py-0.5 text-[10px] text-[#e4b5b7]">{{ error }}</div>
    </div>

    <div v-if="interactive" class="flex flex-wrap items-center gap-1.5 border-t border-white/10 bg-black/20 px-2.5 py-2 text-[10px] text-slate-500">
      <span><b class="text-white">●</b> player</span>
      <span v-if="showTrail"><b class="text-cyan-300">—</b> trail</span>
      <span v-if="showSprites"><b class="text-amber-300">■</b> sprites</span>
      <span v-if="showWarps"><b class="text-purple-300">□</b> warp</span>
      <span v-if="warpDestinations.length" class="ml-auto flex flex-wrap items-center justify-end gap-1">
        <span class="mr-1">Explore warp:</span>
        <button
          v-for="destination in warpDestinations"
          :key="destination"
          type="button"
          class="rounded bg-white/7 px-1.5 py-0.5 font-mono text-[9px] text-slate-300 ring-1 ring-white/10 hover:bg-white/12 hover:text-white"
          @click="emit('warpSelect', destination)"
        >
          {{ hexByte(destination) }}
        </button>
      </span>
    </div>
  </div>
</template>
