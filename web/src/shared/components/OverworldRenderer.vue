<script setup lang="ts">
import { onMounted, onScopeDispose, ref, watch } from 'vue'
import type { RenderActor, RenderPosition, RenderState, RenderTileCell, RenderTileLayer } from '../api/renderstate'
import { PresentationClock, presentationEntityKey, type PresentationSample } from '../animationClock'
import { loadGen1Sprite } from '../gen1Sprite'
import {
  actorStyle,
  characterAsset,
  tileStyle,
  type ResolvedRenderTheme,
  type ThemeTileStyle
} from '../renderTheme'
import { semanticViewport } from '../semanticRenderer'
import { parseTileImageReference, terrainAssetKey } from '../tileAssets'

const props = defineProps<{ state: RenderState; theme: ResolvedRenderTheme }>()

const shell = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)
const spriteImages = new Map<string, HTMLImageElement>()
const failedSprites = new Set<string>()
let observer: ResizeObserver | null = null
let raf = 0
let lastAnimatedDraw = 0
const animationClock = new PresentationClock()

const INDEXED_PALETTES: Record<string, readonly string[]> = {
  'ow-red': ['#deffde', '#ff9c52', '#ff3a08', '#000000'],
  'ow-blue': ['#deffde', '#ff9c52', '#524aff', '#000000'],
  'ow-green': ['#deffde', '#ff9c52', '#3abd19', '#000000'],
  'ow-brown': ['#deffde', '#ff9c52', '#7b5219', '#000000'],
  'ow-rock': ['#deffde', '#c5943a', '#a57b19', '#3a3a3a'],
  'bg-gray': ['#deffde', '#adadad', '#6b6b6b', '#3a3a3a'],
  'bg-green': ['#b5ff52', '#63ce08', '#297300', '#3a3a3a'],
  'bg-water': ['#ffffff', '#4263ff', '#0821ff', '#3a3a3a'],
  'bg-yellow': ['#deffde', '#ffff3a', '#ff8408', '#3a3a3a'],
  'bg-brown': ['#deffde', '#c5943a', '#a57b19', '#3a3a3a']
}


function defaultSpriteAsset(appearance: string | undefined, player = false): string {
  if (player) return 'red'
  switch ((appearance || '').toLowerCase()) {
    case 'player': return 'red'
    case 'rival': return 'blue'
    case 'professor_oak': return 'oak'
    case 'sleeping_gambler': return 'gambler_asleep'
    default: return (appearance || '').toLowerCase()
  }
}

function actorAssetReference(actor: RenderActor, player = false): string {
  const themed = characterAsset(props.theme, actor.appearance, player)
  if (themed) return themed
  if (!player) {
    const generic = props.theme.assets.characters[actor.kind] || props.theme.assets.characters.npc
    if (generic) return generic
  }
  const fallback = defaultSpriteAsset(actor.appearance, player)
  return fallback && fallback !== 'unknown' ? `gen1:${fallback}` : ''
}

function loadBrowserImage(source: string): Promise<HTMLImageElement> {
  return new Promise<HTMLImageElement>((resolve, reject) => {
    const image = new Image()
    image.decoding = 'async'
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error(`failed to load theme asset ${source}`))
    image.src = source
  })
}

function channel(hex: string, offset: number): number {
  return Number.parseInt(hex.slice(offset, offset + 2), 16)
}

async function recolorIndexedImage(image: HTMLImageElement, paletteName: string, transparent: boolean): Promise<HTMLImageElement> {
  const palette = INDEXED_PALETTES[paletteName]
  if (!palette) return image
  const canvas = document.createElement('canvas')
  canvas.width = image.naturalWidth
  canvas.height = image.naturalHeight
  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  if (!ctx) return image
  ctx.drawImage(image, 0, 0)
  const pixels = ctx.getImageData(0, 0, canvas.width, canvas.height)
  for (let i = 0; i < pixels.data.length; i += 4) {
    const index = Math.max(0, Math.min(3, Math.round((255 - pixels.data[i]) / 85)))
    if (transparent && index === 0) {
      pixels.data[i + 3] = 0
      continue
    }
    const color = palette[index]
    pixels.data[i] = channel(color, 1)
    pixels.data[i + 1] = channel(color, 3)
    pixels.data[i + 2] = channel(color, 5)
    pixels.data[i + 3] = 255
  }
  ctx.putImageData(pixels, 0, 0)
  return loadBrowserImage(canvas.toDataURL('image/png'))
}

async function loadImageReference(reference: string): Promise<HTMLImageElement> {
  if (reference.startsWith('gen1:')) return loadGen1Sprite(reference.slice('gen1:'.length))
  const parsed = new URL(reference, window.location.origin)
  const palette = parsed.searchParams.get('palette') || ''
  const transparent = parsed.searchParams.get('transparent') === '1'
  parsed.searchParams.delete('palette')
  parsed.searchParams.delete('transparent')
  parsed.searchParams.delete('repeat')
  const source = parsed.origin === window.location.origin ? parsed.pathname + parsed.search : parsed.toString()
  const image = await loadBrowserImage(source)
  return palette ? recolorIndexedImage(image, palette, transparent) : image
}

async function syncSpriteImages(): Promise<void> {
  const references = new Set<string>()
  if (props.state.player) {
    const player = actorAssetReference(props.state.player, true)
    if (player) references.add(player)
  }
  for (const actor of props.state.entities || []) {
    const reference = actorAssetReference(actor)
    if (reference) references.add(reference)
  }
  for (const reference of Object.values(props.theme.assets.tiles).concat(Object.values(props.theme.assets.objects))) {
    const tile = parseTileImageReference(reference)
    if (tile) references.add(tile.url)
  }

  const missing = [...references].filter((reference) => !spriteImages.has(reference) && !failedSprites.has(reference))
  await Promise.all(missing.map(async (reference) => {
    try {
      spriteImages.set(reference, await loadImageReference(reference))
    } catch {
      failedSprites.add(reference)
    }
  }))
  draw(performance.now())
}

function patternFor(style: ThemeTileStyle, kind: string): string {
  return style.pattern || kind || 'unknown'
}

function drawTile(
  ctx: CanvasRenderingContext2D,
  cell: RenderTileCell,
  x: number,
  y: number,
  size: number,
  now: number,
  objectLayer = false,
  assetReference = ''
): void {
  const kind = cell?.kind || 'unknown'
  const style = tileStyle(props.theme, kind, objectLayer)
  const pattern = patternFor(style, kind)
  if (!objectLayer) {
    ctx.fillStyle = assetReference && (kind === 'tree' || kind === 'path') ? props.theme.tiles.grass.fill : style.fill
    ctx.fillRect(x, y, size + 0.5, size + 0.5)
    if (assetReference && (kind === 'tree' || kind === 'path')) {
      const ground = parseTileImageReference(props.theme.assets.tiles.grass || '')
      const groundImage = ground && spriteImages.get(ground.url)
      if (ground?.source && groundImage) {
        ctx.drawImage(groundImage, ground.source.x, ground.source.y, ground.source.size, ground.source.size, x, y, size, size)
      }
    }
  }

  const tile = parseTileImageReference(assetReference)
  const image = tile && spriteImages.get(tile.url)
  if (image && (!tile.source || (tile.source.x + tile.source.size <= image.naturalWidth && tile.source.y + tile.source.size <= image.naturalHeight))) {
    ctx.save()
    ctx.imageSmoothingEnabled = false
    if (tile.source) {
      const repeat = tile.repeat || 1
      const drawSize = size / repeat
      for (let row = 0; row < repeat; row++) {
        for (let column = 0; column < repeat; column++) {
          ctx.drawImage(image, tile.source.x, tile.source.y, tile.source.size, tile.source.size,
            x + column * drawSize, y + row * drawSize, drawSize, drawSize)
        }
      }
    } else {
      ctx.drawImage(image, x, y, size, size)
    }
    ctx.restore()
    return
  }

  ctx.save()
  switch (pattern) {
    case 'path':
      ctx.fillStyle = style.detail || style.fill
      for (let i = 0; i < 4; i++) {
        const px = x + size * (0.18 + ((i * 0.31) % 0.7))
        const py = y + size * (0.2 + ((i * 0.43) % 0.64))
        ctx.fillRect(px, py, Math.max(1, size * 0.045), Math.max(1, size * 0.035))
      }
      break
    case 'floor':
      ctx.strokeStyle = style.detail || style.fill
      ctx.lineWidth = Math.max(1, size / 28)
      ctx.strokeRect(x + size * 0.08, y + size * 0.08, size * 0.84, size * 0.84)
      break
    case 'grass':
      ctx.strokeStyle = style.detail || style.fill
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
      const phase = (now / props.theme.animation.waterPeriodMs) % 1
      ctx.strokeStyle = style.detail || style.fill
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
      ctx.fillStyle = style.accent || style.fill
      ctx.fillRect(x + size * 0.38, y + size * 0.55, size * 0.24, size * 0.38)
      ctx.fillStyle = style.detail || style.fill
      ctx.beginPath()
      ctx.arc(x + size * 0.5, y + size * 0.38, size * 0.34, 0, Math.PI * 2)
      ctx.fill()
      break
    case 'ledge':
      ctx.fillStyle = style.detail || style.fill
      ctx.fillRect(x, y + size * 0.72, size, Math.max(2, size * 0.12))
      break
    case 'wall':
      ctx.strokeStyle = style.detail || style.fill
      ctx.lineWidth = Math.max(1, size / 22)
      ctx.beginPath()
      ctx.moveTo(x + size * 0.12, y + size * 0.34)
      ctx.lineTo(x + size * 0.88, y + size * 0.34)
      ctx.moveTo(x + size * 0.12, y + size * 0.68)
      ctx.lineTo(x + size * 0.88, y + size * 0.68)
      ctx.stroke()
      break
    case 'door':
      ctx.fillStyle = style.detail || style.fill
      ctx.fillRect(x + size * 0.22, y + size * 0.12, size * 0.56, size * 0.82)
      ctx.fillStyle = style.accent || style.fill
      ctx.beginPath()
      ctx.arc(x + size * 0.66, y + size * 0.55, Math.max(1.5, size * 0.045), 0, Math.PI * 2)
      ctx.fill()
      break
    case 'warp':
      ctx.fillStyle = style.detail || style.fill
      ctx.fillRect(x + size * 0.25, y + size * 0.25, size * 0.5, size * 0.5)
      break
    case 'sign':
      ctx.fillStyle = style.detail || style.fill
      ctx.fillRect(x + size * 0.18, y + size * 0.2, size * 0.64, size * 0.4)
      ctx.fillStyle = style.accent || style.fill
      ctx.fillRect(x + size * 0.45, y + size * 0.58, size * 0.1, size * 0.34)
      break
    case 'unknown':
      ctx.strokeStyle = style.detail || style.fill
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
  const style = actorStyle(props.theme, actor.kind, player)
  ctx.save()
  ctx.fillStyle = style.fill
  ctx.strokeStyle = style.stroke
  ctx.lineWidth = Math.max(2, size * 0.08)
  ctx.beginPath()
  ctx.arc(left + size / 2, top + size / 2, size * 0.32, 0, Math.PI * 2)
  ctx.fill()
  ctx.stroke()
  ctx.fillStyle = style.stroke
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
  player = false,
  position: RenderPosition = actor.position
): void {
  const x = viewport.offsetX + (position.x - viewport.startX) * viewport.tileSize
  const y = viewport.offsetY + (position.y - viewport.startY) * viewport.tileSize
  const size = viewport.tileSize
  if (x + size < 0 || y + size < 0) return

  ctx.save()
  ctx.fillStyle = props.theme.effects.shadow
  ctx.beginPath()
  ctx.ellipse(x + size / 2, y + size * 0.82, size * 0.3, size * 0.11, 0, 0, Math.PI * 2)
  ctx.fill()
  ctx.restore()

  const reference = actorAssetReference(actor, player)
  const image = spriteImages.get(reference)
  if (!image) {
    drawFallbackActor(ctx, actor, x, y, size, player)
    return
  }

  const frameSize = Math.min(16, image.naturalWidth, image.naturalHeight)
  const facing = (actor.facing || '').toLowerCase()
  let row = 0
  const frameRows = Math.floor(image.naturalHeight / frameSize)
  if (frameRows >= 6) {
    if (facing === 'up') row = 2
    else if (facing === 'left' || facing === 'right') row = 4
  } else if (frameRows >= 3) {
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

function draw(now = performance.now(), presentation: PresentationSample = animationClock.sample(now)): void {
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
  ctx.fillStyle = props.theme.effects.background
  ctx.fillRect(0, 0, width, height)

  const preferredTileSize = width < 700 ? Math.min(props.theme.tileSize, 28) : props.theme.tileSize
  const cameraFocus = presentation.camera || props.state.player.position
  const viewport = semanticViewport(props.state, width, height, preferredTileSize, cameraFocus)
  const layers = props.state.layers || []

  for (const layer of layers) {
    if (layer.kind !== 'terrain') continue
    for (let worldY = viewport.startY; worldY < viewport.endY; worldY++) {
      for (let worldX = viewport.startX; worldX < viewport.endX; worldX++) {
        const cell = layerCell(layer, worldX, worldY)
        if (!cell) continue
        const left = viewport.offsetX + (worldX - viewport.startX) * viewport.tileSize
        const top = viewport.offsetY + (worldY - viewport.startY) * viewport.tileSize
        const assetKey = terrainAssetKey(props.theme.assets.tiles, layer, worldX, worldY)
        drawTile(ctx, cell, left, top, viewport.tileSize, now, false, assetKey ? props.theme.assets.tiles[assetKey] : '')
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
        drawTile(ctx, cell, left, top, viewport.tileSize, now, true, props.theme.assets.objects[cell.kind] || '')
      }
    }
  }

  for (const [index, actor] of (props.state.entities || []).entries()) {
    const position = presentation.entityPositions.get(presentationEntityKey(actor, index)) || actor.position
    drawActor(ctx, actor, viewport, false, position)
  }
  drawActor(ctx, props.state.player, viewport, true, presentation.player || props.state.player.position)

  ctx.save()
  ctx.strokeStyle = props.theme.effects.grid
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
  const presentation = animationClock.sample(now)
  if (presentation.animating || now - lastAnimatedDraw >= props.theme.animation.redrawIntervalMs) {
    lastAnimatedDraw = now
    draw(now, presentation)
  }
  raf = requestAnimationFrame(animate)
}

watch(
  () => props.state,
  (state) => {
    const now = performance.now()
    animationClock.ingest(state, now)
    void syncSpriteImages()
    draw(now)
  },
  { deep: false }
)

watch(
  [() => props.theme.id, () => props.theme.version],
  () => {
    void syncSpriteImages()
    draw()
  },
  { deep: false }
)

onMounted(() => {
  const now = performance.now()
  animationClock.ingest(props.state, now)
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
    class="absolute inset-0 overflow-hidden"
    :style="{ background: theme.effects.background }"
    :aria-label="theme.name + ' semantic view of ' + (state.map?.name || state.map?.id || 'current map')"
  >
    <canvas ref="canvas" class="absolute inset-0 h-full w-full [image-rendering:pixelated]" />
    <div
      class="pointer-events-none absolute inset-0"
      :style="{ background: 'radial-gradient(circle at center, transparent 50%, ' + theme.effects.vignette + ' 100%)' }"
    />
  </div>
</template>
