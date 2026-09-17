<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import type { MapSprite } from '../api/types'
import { mapEntry } from '../mapCatalog'
import { worldConnections } from '../worldManifest'
import WorldAtlas, { type WorldAtlasMarker } from './WorldAtlas.vue'

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
  showPlayer?: boolean
  showTrail?: boolean
  showSprites?: boolean
  showWarps?: boolean
  appearance?: 'semantic' | 'explorer'
  showDebugToggle?: boolean
}>(), {
  map: 0,
  x: 0,
  y: 0,
  trail: () => [],
  sprites: () => [],
  debug: false,
  interactive: false,
  showPlayer: true,
  showTrail: true,
  showSprites: true,
  showWarps: true,
  appearance: 'semantic',
  showDebugToggle: true
})

const emit = defineEmits<{
  warpSelect: [destination: number]
}>()

const frame = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)
const loading = ref(false)
const error = ref('')
const zoomLevel = ref(1)
const payload = ref<MapPayload | null>(null)
let savedDebug = false
try {
  savedDebug = window.localStorage.getItem('pokepilot.map.debug') === '1'
} catch {
  // Storage may be unavailable in hardened/private browser contexts.
}
const params = new URLSearchParams(window.location.search)
const localDebug = ref(params.get('debug') === '1' || savedDebug)
const atlasMode = ref(params.get('atlas') === '1')
const debugEnabled = computed(() => props.debug || localDebug.value)
const explorerAppearance = computed(() => props.appearance === 'explorer')
const atlasAvailable = computed(() => props.interactive && window.location.pathname.startsWith('/world'))
const currentConnections = computed(() => worldConnections(Number(props.map || 0)))
const atlasMarkers = computed<WorldAtlasMarker[]>(() => {
  if (!props.showPlayer) return []
  return [{
    map: Number(props.map || 0),
    x: Number(props.x || 0),
    y: Number(props.y || 0),
    label: `Current agent · ${friendlyMapLabel(Number(props.map || 0))}`
  }]
})
const warpDestinations = computed(() => {
  const seen = new Set<number>()
  const result: number[] = []
  for (const warp of payload.value?.warps || []) {
    const destination = Number(warp.dest)
    if (!Number.isFinite(destination) || seen.has(destination)) continue
    seen.add(destination)
    result.push(destination)
  }
  return result
})
const connections = computed(() => payload.value?.connections || [])
let serial = 0
let observer: ResizeObserver | null = null
let tileSize = 0

function mapName(): string {
  return `${Number(props.map || 0).toString(16).padStart(2, '0')}.json`
}

function mapLabel(value: number): string {
  return `Map ${Math.max(0, Number(value || 0)).toString(16).padStart(2, '0').toUpperCase()}`
}

function friendlyMapLabel(value: number): string {
  return mapEntry(Number(value))?.label || mapLabel(Number(value))
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

function isOutdoorMap(): boolean {
  const id = Number(props.map || 0)
  return id <= 0x24
}

function isCityMap(): boolean {
  const id = Number(props.map || 0)
  return id <= 0x0a
}

function explorerFill(cell: string): string {
  if (cell === '~') return '#347d96'
  if (cell === 'g') return '#4f8a53'
  if (cell === '#') {
    if (!isOutdoorMap()) return '#34434b'
    return isCityMap() ? '#48645d' : '#315d3e'
  }
  return isCityMap() ? '#b4a877' : '#aa9b60'
}

function drawExplorerTexture(ctx: CanvasRenderingContext2D, cell: string, x: number, y: number, px: number): void {
  if (px < 5) return
  const left = x * px
  const top = y * px

  if (cell === '#') {
    if (isOutdoorMap() && !isCityMap()) {
      const cx = left + px * 0.5
      const cy = top + px * 0.48
      ctx.fillStyle = 'rgba(101, 151, 86, 0.50)'
      ctx.beginPath()
      ctx.arc(cx - px * 0.18, cy, px * 0.24, 0, Math.PI * 2)
      ctx.arc(cx + px * 0.18, cy, px * 0.24, 0, Math.PI * 2)
      ctx.arc(cx, cy - px * 0.16, px * 0.28, 0, Math.PI * 2)
      ctx.fill()
      if (px >= 9) {
        ctx.fillStyle = 'rgba(44, 71, 42, 0.55)'
        ctx.fillRect(left + px * 0.44, top + px * 0.58, Math.max(1, px * 0.12), Math.max(1, px * 0.3))
      }
      return
    }

    ctx.strokeStyle = 'rgba(220, 240, 214, 0.15)'
    ctx.lineWidth = Math.max(1, Math.floor(px * 0.08))
    ctx.strokeRect(left + 0.5, top + 0.5, Math.max(1, px - 1), Math.max(1, px - 1))
    if (px >= 9) {
      ctx.fillStyle = isCityMap() ? 'rgba(28, 45, 45, 0.23)' : 'rgba(14, 31, 28, 0.22)'
      const inset = Math.max(1, Math.floor(px * 0.2))
      ctx.fillRect(left + inset, top + inset, Math.max(1, px - inset * 2), Math.max(1, px - inset * 2))
    }
    return
  }

  if (cell === 'g') {
    ctx.strokeStyle = 'rgba(225, 244, 170, 0.36)'
    ctx.lineWidth = Math.max(1, Math.floor(px * 0.08))
    const cx = left + px * 0.5
    const base = top + px * 0.72
    ctx.beginPath()
    ctx.moveTo(cx, base)
    ctx.lineTo(left + px * 0.35, top + px * 0.35)
    ctx.moveTo(cx, base)
    ctx.lineTo(left + px * 0.62, top + px * 0.3)
    ctx.stroke()
    return
  }

  if (cell === '~') {
    ctx.strokeStyle = 'rgba(214, 247, 255, 0.32)'
    ctx.lineWidth = Math.max(1, Math.floor(px * 0.08))
    ctx.beginPath()
    ctx.moveTo(left + px * 0.14, top + px * 0.38)
    ctx.quadraticCurveTo(left + px * 0.36, top + px * 0.25, left + px * 0.55, top + px * 0.38)
    ctx.quadraticCurveTo(left + px * 0.74, top + px * 0.51, left + px * 0.9, top + px * 0.38)
    ctx.moveTo(left + px * 0.1, top + px * 0.68)
    ctx.quadraticCurveTo(left + px * 0.3, top + px * 0.55, left + px * 0.5, top + px * 0.68)
    ctx.quadraticCurveTo(left + px * 0.7, top + px * 0.81, left + px * 0.88, top + px * 0.68)
    ctx.stroke()
    return
  }

  if (cell === 'W') {
    const inset = Math.max(1, Math.floor(px * 0.18))
    ctx.fillStyle = 'rgba(81, 46, 112, 0.58)'
    ctx.fillRect(left + inset, top + inset, Math.max(1, px - inset * 2), Math.max(1, px - inset))
    ctx.fillStyle = 'rgba(239, 213, 255, 0.75)'
    ctx.beginPath()
    ctx.arc(left + px * 0.7, top + px * 0.52, Math.max(1, px * 0.07), 0, Math.PI * 2)
    ctx.fill()
    return
  }

  ctx.fillStyle = 'rgba(247, 237, 177, 0.17)'
  const dot = Math.max(1, Math.floor(px * 0.1))
  const ox = ((x * 7 + y * 3) % 5 + 1) / 6
  const oy = ((x * 5 + y * 11) % 5 + 1) / 6
  ctx.fillRect(left + Math.floor(px * ox), top + Math.floor(px * oy), dot, dot)
  if (px >= 10 && !isCityMap()) {
    ctx.strokeStyle = 'rgba(75, 68, 38, 0.12)'
    ctx.lineWidth = 1
    ctx.beginPath()
    ctx.moveTo(left, top + px - 0.5)
    ctx.lineTo(left + px, top + px - 0.5)
    ctx.stroke()
  }
}

function draw(): void {
  const node = canvas.value
  const box = frame.value
  const data = payload.value
  if (!node || !box || !data || atlasMode.value) return

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
  const semanticColors = {
    ground: token('--map-ground', '#102229'),
    wall: token('--map-wall', '#40515d'),
    grass: token('--map-grass', '#28543c'),
    water: token('--map-water', '#1e5f78'),
    warp: token('--map-warp', '#c999ef'),
    trail: token('--map-trail', '#61e2ee'),
    sprite: token('--map-sprite', '#efb24f'),
    player: token('--map-player', '#f2fbff')
  }
  const explorerColors = {
    ground: '#aa9b60',
    wall: '#315d3e',
    grass: '#4f8a53',
    water: '#347d96',
    warp: '#d6a7ff',
    trail: '#7ee8f2',
    sprite: '#f2bd59',
    player: '#ffffff'
  }
  const colors = explorerAppearance.value ? explorerColors : semanticColors

  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      const cell = cellAt(data, x, y)
      ctx.fillStyle = explorerAppearance.value
        ? explorerFill(cell)
        : cell === '#' ? colors.wall : cell === 'g' ? colors.grass : cell === '~' ? colors.water : colors.ground
      ctx.fillRect(x * px, y * px, px, px)
      if (explorerAppearance.value) drawExplorerTexture(ctx, cell, x, y, px)
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

  if (props.showPlayer) {
    const playerX = Number(props.x || 0)
    const playerY = Number(props.y || 0)
    if (playerX >= 0 && playerY >= 0 && playerX < width && playerY < height) {
      ctx.fillStyle = colors.player
      ctx.beginPath()
      ctx.arc((playerX + 0.5) * px, (playerY + 0.5) * px, Math.max(2, px * 0.42), 0, Math.PI * 2)
      ctx.fill()
      ctx.strokeStyle = explorerAppearance.value ? 'rgba(12, 22, 24, 0.8)' : 'transparent'
      ctx.lineWidth = Math.max(1, Math.floor(px * 0.1))
      if (explorerAppearance.value) ctx.stroke()
      if (debugEnabled.value) {
        drawDebugText(ctx, `@${playerX},${playerY}`, (playerX + 0.5) * px, (playerY + 0.5) * px, px)
      }
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
    payload.value = next
    zoomLevel.value = 1
    requestAnimationFrame(draw)
  } catch (cause) {
    if (id !== serial) return
    payload.value = null
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
  if (!box || !node || tileSize <= 0 || !props.showPlayer || atlasMode.value) return
  const playerX = Number(props.x || 0)
  const playerY = Number(props.y || 0)
  box.scrollTo({
    left: Math.max(0, node.offsetLeft + (playerX + 0.5) * tileSize - box.clientWidth / 2),
    top: Math.max(0, node.offsetTop + (playerY + 0.5) * tileSize - box.clientHeight / 2),
    behavior: 'smooth'
  })
}

function onCanvasClick(event: MouseEvent): void {
  if (!props.interactive || !props.showWarps || !payload.value || !canvas.value || tileSize <= 0) return
  const rect = canvas.value.getBoundingClientRect()
  if (rect.width <= 0 || rect.height <= 0) return
  const intrinsicX = (event.clientX - rect.left) * canvas.value.width / rect.width
  const intrinsicY = (event.clientY - rect.top) * canvas.value.height / rect.height
  const tileX = Math.floor(intrinsicX / tileSize)
  const tileY = Math.floor(intrinsicY / tileSize)
  const warp = (payload.value.warps || []).find((candidate) => Number(candidate.x) === tileX && Number(candidate.y) === tileY)
  if (warp) selectDestination(Number(warp.dest))
}

function connectionArrow(direction: string): string {
  if (direction === 'north') return '↑'
  if (direction === 'south') return '↓'
  if (direction === 'west') return '←'
  if (direction === 'east') return '→'
  return '→'
}

function connectionOverlayClass(direction: string): string {
  const base = 'absolute z-10 max-w-[42%] truncate rounded-full bg-black/78 px-2.5 py-1 text-[10px] font-semibold text-emerald-50 shadow-lg shadow-black/30 ring-1 ring-emerald-200/25 backdrop-blur-sm hover:bg-emerald-950/90 hover:ring-emerald-200/45'
  if (direction === 'north') return `${base} top-2 left-1/2 -translate-x-1/2`
  if (direction === 'south') return `${base} bottom-2 left-1/2 -translate-x-1/2`
  if (direction === 'west') return `${base} top-1/2 left-2 -translate-y-1/2`
  return `${base} top-1/2 right-2 -translate-y-1/2`
}

function selectDestination(destination: number): void {
  atlasMode.value = false
  syncAtlasURL()
  emit('warpSelect', Number(destination))
}

function setAtlasMode(enabled: boolean): void {
  atlasMode.value = enabled
  syncAtlasURL()
  if (!enabled) requestAnimationFrame(draw)
}

function syncAtlasURL(): void {
  const url = new URL(window.location.href)
  if (atlasMode.value) url.searchParams.set('atlas', '1')
  else url.searchParams.delete('atlas')
  history.replaceState(null, '', url)
}

watch(() => props.map, () => { void loadMap() }, { immediate: true })
watch([
  () => props.x,
  () => props.y,
  () => props.trail,
  () => props.sprites,
  () => props.showPlayer,
  () => props.showTrail,
  () => props.showSprites,
  () => props.showWarps,
  () => props.appearance,
  () => debugEnabled.value,
  () => atlasMode.value
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
  <div :class="[appearance === 'explorer' ? 'bg-[#09130f]' : 'bg-[#0c1118]', 'flex h-full min-h-0 w-full flex-col']">
    <div v-if="interactive" class="flex flex-wrap items-center justify-between gap-2 border-b border-white/10 bg-black/20 px-2.5 py-2">
      <div class="flex min-w-0 items-center gap-2">
        <strong class="truncate text-[11px] text-white">{{ friendlyMapLabel(map) }}</strong>
        <span v-if="debugEnabled" class="shrink-0 font-mono text-[9px] text-slate-600">0x{{ hexByte(map) }}</span>
        <span v-if="!atlasMode && currentConnections.length" class="hidden truncate text-[10px] text-slate-500 sm:inline">{{ currentConnections.length }} connected exit{{ currentConnections.length === 1 ? '' : 's' }}</span>
        <span v-else-if="!atlasMode && connections.length" class="hidden truncate text-[10px] text-slate-500 sm:inline">edges: {{ connections.join(' · ') }}</span>
      </div>
      <div class="flex items-center gap-1">
        <div v-if="atlasAvailable" class="mr-1 flex items-center gap-0.5 rounded bg-black/30 p-0.5 ring-1 ring-white/10">
          <button type="button" :class="[!atlasMode ? 'bg-emerald-300/15 text-emerald-100' : 'text-slate-500 hover:text-white', 'rounded px-2 py-1 text-[10px] font-semibold']" @click="setAtlasMode(false)">Map</button>
          <button type="button" :class="[atlasMode ? 'bg-emerald-300/15 text-emerald-100' : 'text-slate-500 hover:text-white', 'rounded px-2 py-1 text-[10px] font-semibold']" @click="setAtlasMode(true)">Kanto</button>
        </div>
        <template v-if="!atlasMode">
          <button type="button" class="rounded bg-white/7 px-2 py-1 text-xs text-slate-300 ring-1 ring-white/10 hover:bg-white/12" aria-label="Zoom out" @click="zoomBy(-0.25)">−</button>
          <button type="button" class="min-w-14 rounded bg-white/7 px-2 py-1 font-mono text-[10px] text-slate-300 ring-1 ring-white/10 hover:bg-white/12" title="Reset zoom" @click="resetZoom">{{ Math.round(zoomLevel * 100) }}%</button>
          <button type="button" class="rounded bg-white/7 px-2 py-1 text-xs text-slate-300 ring-1 ring-white/10 hover:bg-white/12" aria-label="Zoom in" @click="zoomBy(0.25)">+</button>
          <button v-if="showPlayer" type="button" class="rounded bg-white/7 px-2 py-1 text-[10px] font-semibold text-slate-300 ring-1 ring-white/10 hover:bg-white/12" @click="centerOnPlayer">Locate</button>
        </template>
      </div>
    </div>

    <WorldAtlas
      v-if="atlasMode && atlasAvailable"
      class="min-h-0 flex-1"
      :selected-map="map"
      :markers="atlasMarkers"
      @select-map="selectDestination"
    />

    <div
      v-else
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

      <template v-if="interactive && explorerAppearance && !debugEnabled">
        <button
          v-for="connection in currentConnections"
          :key="`${connection.direction}-${connection.to}`"
          type="button"
          :class="connectionOverlayClass(connection.direction)"
          :title="`Open ${friendlyMapLabel(connection.to)}`"
          @click="selectDestination(connection.to)"
        >
          {{ connectionArrow(connection.direction) }} {{ friendlyMapLabel(connection.to) }}
        </button>
      </template>

      <button
        v-if="showDebugToggle"
        type="button"
        :aria-pressed="debugEnabled"
        :title="debugEnabled ? 'Hide map debug labels' : 'Show map debug labels'"
        :class="[
          debugEnabled ? 'bg-[var(--poke-cyan)] text-[#101820]' : 'bg-black/75 text-[var(--poke-muted)] hover:text-white',
          'absolute top-2 right-2 z-20 rounded-sm px-1.5 py-0.5 font-mono text-[9px] font-bold ring-1 ring-white/10'
        ]"
        @click="localDebug = !localDebug"
      >
        {{ debugEnabled ? 'DEBUG ON' : 'DEBUG' }}
      </button>
      <div v-if="debugEnabled" class="pointer-events-none absolute bottom-2 left-2 z-10 bg-black/80 px-1.5 py-1 font-mono text-[9px] leading-3 text-[var(--poke-muted)] ring-1 ring-white/10">
        <div><span class="text-white">S#/PP</span> sprite slot / picture ID</div>
        <div><span class="text-white">→MM</span> warp destination map</div>
      </div>
      <div v-if="loading" class="pointer-events-none absolute right-1.5 bottom-1.5 bg-black/70 px-1.5 py-0.5 text-[10px] text-[var(--poke-muted)]">Loading map…</div>
      <div v-else-if="error" class="pointer-events-none absolute right-1.5 bottom-1.5 max-w-[80%] bg-[#352529] px-1.5 py-0.5 text-[10px] text-[#e4b5b7]">{{ error }}</div>
    </div>

    <div v-if="interactive && !atlasMode" class="flex flex-wrap items-center gap-1.5 border-t border-white/10 bg-black/20 px-2.5 py-2 text-[10px] text-slate-500">
      <span v-if="showPlayer"><b class="text-white">●</b> player</span>
      <span v-if="showTrail"><b class="text-cyan-300">—</b> trail</span>
      <span v-if="showSprites"><b class="text-amber-300">■</b> sprites</span>
      <span v-if="showWarps"><b class="text-purple-300">□</b> warp</span>

      <span v-if="currentConnections.length" class="ml-auto flex flex-wrap items-center justify-end gap-1">
        <span class="mr-1 text-slate-600">Connected:</span>
        <button
          v-for="connection in currentConnections"
          :key="`${connection.direction}-${connection.to}`"
          type="button"
          class="rounded bg-emerald-300/8 px-1.5 py-0.5 text-[9px] font-semibold text-emerald-100 ring-1 ring-emerald-300/15 hover:bg-emerald-300/14"
          @click="selectDestination(connection.to)"
        >
          {{ connectionArrow(connection.direction) }} {{ friendlyMapLabel(connection.to) }}
        </button>
      </span>

      <span v-if="warpDestinations.length" :class="[currentConnections.length ? 'w-full justify-end' : 'ml-auto justify-end', 'flex flex-wrap items-center gap-1']">
        <span class="mr-1 text-slate-600">Warps:</span>
        <button
          v-for="destination in warpDestinations"
          :key="destination"
          type="button"
          class="rounded bg-purple-300/8 px-1.5 py-0.5 text-[9px] font-semibold text-purple-100 ring-1 ring-purple-300/15 hover:bg-purple-300/14"
          @click="selectDestination(destination)"
        >
          {{ friendlyMapLabel(destination) }}
        </button>
      </span>
    </div>
  </div>
</template>