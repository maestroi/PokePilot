<script setup lang="ts">
import { onMounted, onUnmounted, ref, watch } from 'vue'
import type { MapSprite } from '../shared/api/types'

interface MapWarp {
  x: number
  y: number
  dest: number
}

interface MapPayload {
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
}>(), {
  map: 0,
  x: 0,
  y: 0,
  trail: () => [],
  sprites: () => [],
  debug: false
})

const frame = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)
const loading = ref(false)
const error = ref('')
let payload: MapPayload | null = null
let serial = 0
let observer: ResizeObserver | null = null

function mapName(): string {
  return `${Number(props.map || 0).toString(16).padStart(2, '0')}.json`
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
  const px = Math.max(props.debug ? 18 : 6, fitPx)
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
      if (cell === 'W') {
        ctx.strokeStyle = colors.warp
        ctx.lineWidth = Math.max(1, Math.floor(px / 4))
        ctx.strokeRect(x * px + 1, y * px + 1, Math.max(1, px - 2), Math.max(1, px - 2))
      }
    }
  }

  if (props.debug) {
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

    for (const warp of data.warps || []) {
      const x = Number(warp.x)
      const y = Number(warp.y)
      if (x < 0 || y < 0 || x >= width || y >= height) continue
      drawDebugText(ctx, `→${hexByte(warp.dest)}`, (x + 0.5) * px, (y + 0.5) * px, px)
    }
  }

  const trail = props.trail || []
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

  for (const sprite of props.sprites || []) {
    const x = Number(sprite.x)
    const y = Number(sprite.y)
    if (x < 0 || y < 0 || x >= width || y >= height) continue
    const pad = Math.max(1, Math.floor(px / 4))
    ctx.fillStyle = colors.sprite
    ctx.fillRect(x * px + pad, y * px + pad, Math.max(2, px - pad * 2), Math.max(2, px - pad * 2))
    if (props.debug) {
      const slot = Number(sprite.slot)
      const slotLabel = Number.isFinite(slot) && slot > 0 ? String(slot) : '?'
      drawDebugText(ctx, `S${slotLabel}/${hexByte(sprite.picture_id)}`, (x + 0.5) * px, (y + 0.5) * px, px)
    }
  }

  const playerX = Number(props.x || 0)
  const playerY = Number(props.y || 0)
  if (playerX >= 0 && playerY >= 0 && playerX < width && playerY < height) {
    ctx.fillStyle = colors.player
    ctx.beginPath()
    ctx.arc((playerX + 0.5) * px, (playerY + 0.5) * px, Math.max(2, px * 0.42), 0, Math.PI * 2)
    ctx.fill()
    if (props.debug) {
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
    requestAnimationFrame(draw)
  } catch (cause) {
    if (id !== serial) return
    payload = null
    error.value = cause instanceof Error ? cause.message : 'Map unavailable'
  } finally {
    if (id === serial) loading.value = false
  }
}

watch(() => props.map, () => { void loadMap() }, { immediate: true })
watch([() => props.x, () => props.y, () => props.trail, () => props.sprites, () => props.debug], () => requestAnimationFrame(draw), { deep: true })

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
  <div
    ref="frame"
    :class="[
      debug ? 'place-items-start overflow-auto' : 'place-items-center overflow-hidden',
      'relative grid h-full min-h-0 w-full bg-[#0c1118] p-1.5'
    ]"
  >
    <canvas ref="canvas" class="max-w-none shrink-0 [image-rendering:pixelated]" aria-label="Semantic map" />
    <div v-if="loading" class="pointer-events-none absolute right-1.5 bottom-1.5 bg-black/70 px-1.5 py-0.5 text-[10px] text-[var(--poke-muted)]">Loading map…</div>
    <div v-else-if="error" class="pointer-events-none absolute right-1.5 bottom-1.5 max-w-[80%] bg-[#352529] px-1.5 py-0.5 text-[10px] text-[#e4b5b7]">{{ error }}</div>
  </div>
</template>
