<script setup lang="ts">
import { onMounted, onScopeDispose, ref, watch } from 'vue'
import type { RenderActor, RenderState, RenderTileCell, RenderTileLayer } from '../api/renderstate'
import { loadGen1Sprite } from '../gen1Sprite'
import { semanticViewport } from '../semanticRenderer'

const props = defineProps<{ state: RenderState }>()

const shell = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)
const spriteImages = new Map<string, HTMLImageElement>()
const failedSprites = new Set<string>()
let observer: ResizeObserver | null = null
let raf = 0
let lastAnimatedDraw = 0

function spriteAsset(appearance: string | undefined, player = false): string {
  if (player) return 'red'
  switch ((appearance || '').toLowerCase()) {
    case 'player': return 'red'
    case 'rival': return 'blue'
    case 'professor_oak': return 'oak'
    case 'sleeping_gambler': return 'gambler_asleep'
    default: return (appearance || '').toLowerCase()
  }
}

async function syncSpriteImages(): Promise<void> {
  const assets = new Set<string>()
  assets.add('red')
  for (const actor of props.state.entities || []) {
    const asset = spriteAsset(actor.appearance)
    if (asset && asset !== 'unknown') assets.add(asset)
  }

  const missing = [...assets].filter((asset) => !spriteImages.has(asset) && !failedSprites.has(asset))
  await Promise.all(missing.map(async (asset) => {
    try {
      spriteImages.set(asset, await loadGen1Sprite(asset))
    } catch {
      failedSprites.add(asset)
    }
  }))
  draw(performance.now())
}

function tileColor(kind: string): string {
  switch (kind) {
    case 'path': return '#c8bb84'
    case 'floor': return '#b6aa7a'
    case 'grass': return '#4d9153'
    case 'water': return '#3f8ab5'
    case 'tree': return '#2f6d3f'
    case 'ledge': return '#927b55'
    case 'wall': return '#526268'
    case 'door': return '#8b6848'
    case 'warp': return '#7760a8'
    case 'sign': return '#806943'
    default: return '#35434a'
  }
}

function drawTile(
  ctx: CanvasRenderingContext2D,
  cell: RenderTileCell,
  x: number,
  y: number,
  size: number,
  now: number
): void {
  const kind = cell?.kind || 'unknown'
  ctx.fillStyle = tileColor(kind)
  ctx.fillRect(x, y, size + 0.5, size + 0.5)

  ctx.save()
  switch (kind) {
    case 'grass':
      ctx.strokeStyle = 'rgba(220,255,207,.35)'
      ctx.lineWidth = Math.max(1, size / 18)
      for (let i = 0; i < 3; i++) {
        const px = x + size * (0.22 + i * 0.28)
        ctx.beginPath()
        ctx.moveTo(px, y + size * 0.72)
        ctx.lineTo(px - size * 0.08, y + size * 0.5)
        ctx.moveTo(px, y + size * 0.72)
        ctx.lineTo(px + size * 0.08, y + size * 0.47)
        ctx.stroke()
      }
      break
    case 'water': {
      const phase = (now / 700) % 1
      ctx.strokeStyle = 'rgba(215,245,255,.42)'
      ctx.lineWidth = Math.max(1, size / 20)
      for (let row = -1; row < 4; row++) {
        const yy = y + ((row + phase) * size) / 3
        ctx.beginPath()
        ctx.moveTo(x + size * 0.12, yy)
        ctx.lineTo(x + size * 0.46, yy)
        ctx.moveTo(x + size * 0.62, yy + size * 0.08)
        ctx.lineTo(x + size * 0.88, yy + size * 0.08)
        ctx.stroke()
      }
      break
    }
    case 'tree':
      ctx.fillStyle = '#244f31'
      ctx.fillRect(x + size * 0.38, y + size * 0.55, size * 0.24, size * 0.38)
      ctx.fillStyle = '#65a55e'
      ctx.beginPath()
      ctx.arc(x + size * 0.5, y + size * 0.38, size * 0.34, 0, Math.PI * 2)
      ctx.fill()
      break
    case 'ledge':
      ctx.fillStyle = 'rgba(245,224,160,.5)'
      ctx.fillRect(x, y + size * 0.72, size, Math.max(2, size * 0.12))
      break
    case 'warp':
      ctx.fillStyle = 'rgba(225,214,255,.55)'
      ctx.fillRect(x + size * 0.25, y + size * 0.25, size * 0.5, size * 0.5)
      break
    case 'sign':
      ctx.fillStyle = '#d3b46d'
      ctx.fillRect(x + size * 0.18, y + size * 0.2, size * 0.64, size * 0.4)
      ctx.fillStyle = '#70562e'
      ctx.fillRect(x + size * 0.45, y + size * 0.58, size * 0.1, size * 0.34)
      break
    case 'unknown':
      ctx.strokeStyle = 'rgba(255,255,255,.09)'
      ctx.beginPath()
      ctx.moveTo(x, y)
      ctx.lineTo(x + size, y + size)
      ctx.moveTo(x + size, y)
      ctx.lineTo(x, y + size)
      ctx.stroke()
      break
  }
  ctx.restore()
}

function layerCell(layer: RenderTileLayer, worldX: number, worldY: number): RenderTileCell | null {
  const originX = layer.origin?.x || 0
  const originY = layer.origin?.y || 0
  const x = worldX - originX
  const y = worldY - originY
  if (x < 0 || y < 0 || x >= layer.width || y >= layer.height) return null
  return layer.cells[y * layer.width + x] || null
}

function facingArrow(facing: string | undefined): string {
  switch ((facing || '').toLowerCase()) {
    case 'up': return '▲'
    case 'down': return '▼'
    case 'left': return '◀'
    case 'right': return '▶'
    default: return '•'
  }
}

function drawFallbackActor(
  ctx: CanvasRenderingContext2D,
  actor: RenderActor,
  left: number,
  top: number,
  size: number,
  player: boolean
): void {
  ctx.save()
  ctx.fillStyle = player ? '#f4f7ff' : actor.kind === 'trainer' ? '#f6a65d' : '#f2d071'
  ctx.strokeStyle = player ? '#e43c4f' : '#18252a'
  ctx.lineWidth = Math.max(2, size * 0.08)
  ctx.beginPath()
  ctx.arc(left + size / 2, top + size / 2, size * 0.32, 0, Math.PI * 2)
  ctx.fill()
  ctx.stroke()
  ctx.fillStyle = '#162229'
  ctx.font = 'bold ' + Math.max(9, size * 0.24) + 'px ui-monospace, monospace'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText(facingArrow(actor.facing), left + size / 2, top + size / 2)
  ctx.restore()
}

function drawActor(
  ctx: CanvasRenderingContext2D,
  actor: RenderActor,
  viewport: ReturnType<typeof semanticViewport>,
  player = false
): void {
  const x = viewport.offsetX + (actor.position.x - viewport.startX) * viewport.tileSize
  const y = viewport.offsetY + (actor.position.y - viewport.startY) * viewport.tileSize
  const size = viewport.tileSize
  if (x + size < 0 || y + size < 0) return

  ctx.save()
  ctx.fillStyle = 'rgba(0,0,0,.22)'
  ctx.beginPath()
  ctx.ellipse(x + size / 2, y + size * 0.82, size * 0.3, size * 0.11, 0, 0, Math.PI * 2)
  ctx.fill()
  ctx.restore()

  const asset = spriteAsset(actor.appearance, player)
  const image = spriteImages.get(asset)
  if (!image) {
    drawFallbackActor(ctx, actor, x, y, size, player)
    return
  }

  const frameSize = Math.min(16, image.naturalWidth, image.naturalHeight)
  const facing = (actor.facing || '').toLowerCase()
  let row = 0
  if (image.naturalHeight >= 48) {
    if (facing === 'up') row = 1
    else if (facing === 'left' || facing === 'right') row = 2
  }
  const sourceY = Math.min(row * 16, Math.max(0, image.naturalHeight - frameSize))

  ctx.save()
  ctx.imageSmoothingEnabled = false
  if (facing === 'right' && image.naturalHeight >= 48) {
    ctx.translate(x + size, y)
    ctx.scale(-1, 1)
    ctx.drawImage(image, 0, sourceY, frameSize, frameSize, 0, 0, size, size)
  } else {
    ctx.drawImage(image, 0, sourceY, frameSize, frameSize, x, y, size, size)
  }
  ctx.restore()
}

function draw(now = performance.now()): void {
  const el = canvas.value
  const host = shell.value
  if (!el || !host || !props.state.map || !props.state.player) return

  const width = Math.max(1, host.clientWidth)
  const height = Math.max(1, host.clientHeight)
  const dpr = Math.min(2, window.devicePixelRatio || 1)
  const pixelWidth = Math.round(width * dpr)
  const pixelHeight = Math.round(height * dpr)
  if (el.width !== pixelWidth || el.height !== pixelHeight) {
    el.width = pixelWidth
    el.height = pixelHeight
    el.style.width = width + 'px'
    el.style.height = height + 'px'
  }

  const ctx = el.getContext('2d')
  if (!ctx) return
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.imageSmoothingEnabled = false
  ctx.clearRect(0, 0, width, height)
  ctx.fillStyle = '#142228'
  ctx.fillRect(0, 0, width, height)

  const viewport = semanticViewport(props.state, width, height, width < 700 ? 28 : 36)
  const layers = props.state.layers || []

  for (const layer of layers) {
    if (layer.kind !== 'terrain') continue
    for (let worldY = viewport.startY; worldY < viewport.endY; worldY++) {
      for (let worldX = viewport.startX; worldX < viewport.endX; worldX++) {
        const cell = layerCell(layer, worldX, worldY)
        if (!cell) continue
        const left = viewport.offsetX + (worldX - viewport.startX) * viewport.tileSize
        const top = viewport.offsetY + (worldY - viewport.startY) * viewport.tileSize
        drawTile(ctx, cell, left, top, viewport.tileSize, now)
      }
    }
  }

  for (const layer of layers) {
    if (layer.kind === 'terrain') continue
    for (let worldY = viewport.startY; worldY < viewport.endY; worldY++) {
      for (let worldX = viewport.startX; worldX < viewport.endX; worldX++) {
        const cell = layerCell(layer, worldX, worldY)
        if (!cell || !cell.kind || cell.kind === 'unknown') continue
        const left = viewport.offsetX + (worldX - viewport.startX) * viewport.tileSize
        const top = viewport.offsetY + (worldY - viewport.startY) * viewport.tileSize
        drawTile(ctx, cell, left, top, viewport.tileSize, now)
      }
    }
  }

  for (const actor of props.state.entities || []) {
    drawActor(ctx, actor, viewport)
  }
  drawActor(ctx, props.state.player, viewport, true)

  ctx.save()
  ctx.strokeStyle = 'rgba(255,255,255,.045)'
  ctx.lineWidth = 1
  for (let worldX = viewport.startX; worldX <= viewport.endX; worldX++) {
    const x = viewport.offsetX + (worldX - viewport.startX) * viewport.tileSize
    ctx.beginPath()
    ctx.moveTo(x, viewport.offsetY)
    ctx.lineTo(x, viewport.offsetY + (viewport.endY - viewport.startY) * viewport.tileSize)
    ctx.stroke()
  }
  for (let worldY = viewport.startY; worldY <= viewport.endY; worldY++) {
    const y = viewport.offsetY + (worldY - viewport.startY) * viewport.tileSize
    ctx.beginPath()
    ctx.moveTo(viewport.offsetX, y)
    ctx.lineTo(viewport.offsetX + (viewport.endX - viewport.startX) * viewport.tileSize, y)
    ctx.stroke()
  }
  ctx.restore()
}

function animate(now: number): void {
  if (now - lastAnimatedDraw >= 100) {
    lastAnimatedDraw = now
    draw(now)
  }
  raf = requestAnimationFrame(animate)
}

watch(() => props.state, () => {
  void syncSpriteImages()
  draw()
}, { deep: false })

onMounted(() => {
  observer = new ResizeObserver(() => draw())
  if (shell.value) observer.observe(shell.value)
  void syncSpriteImages()
  raf = requestAnimationFrame(animate)
})

onScopeDispose(() => {
  observer?.disconnect()
  if (raf) cancelAnimationFrame(raf)
})
</script>

<template>
  <div
    ref="shell"
    class="absolute inset-0 overflow-hidden bg-[#142228]"
    :aria-label="'Modern semantic view of ' + (state.map?.name || state.map?.id || 'current map')"
  >
    <canvas ref="canvas" class="absolute inset-0 h-full w-full [image-rendering:pixelated]" />
    <div class="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_center,transparent_50%,rgba(0,0,0,.28)_100%)]" />
  </div>
</template>
