import type { RenderState, RenderTileLayer } from './api/renderstate'

export interface SemanticViewport {
  startX: number
  startY: number
  endX: number
  endY: number
  offsetX: number
  offsetY: number
  tileSize: number
}

export function terrainLayer(state: RenderState | null | undefined): RenderTileLayer | undefined {
  return state?.layers?.find((layer) => layer.kind === 'terrain')
}

export function canRenderOverworld(state: RenderState | null | undefined): state is RenderState {
  if (!state || state.schema_version !== 1 || state.scene !== 'overworld') return false
  const capabilities = new Set(state.capabilities || [])
  if (!capabilities.has('map') || !capabilities.has('player') || !capabilities.has('layers')) return false
  if (!state.map || !state.player) return false
  const width = Number(state.map.width || 0)
  const height = Number(state.map.height || 0)
  if (width <= 0 || height <= 0) return false
  const terrain = terrainLayer(state)
  return Boolean(terrain && terrain.width === width && terrain.height === height && terrain.cells.length === width * height)
}

export function semanticViewport(
  state: RenderState,
  pixelWidth: number,
  pixelHeight: number,
  preferredTileSize = 32
): SemanticViewport {
  const mapWidth = Math.max(1, Number(state.map?.width || 1))
  const mapHeight = Math.max(1, Number(state.map?.height || 1))
  const tileSize = Math.max(16, Math.min(48, preferredTileSize))
  const visibleX = Math.max(1, Math.ceil(pixelWidth / tileSize) + 2)
  const visibleY = Math.max(1, Math.ceil(pixelHeight / tileSize) + 2)
  const playerX = Number(state.player?.position.x || 0)
  const playerY = Number(state.player?.position.y || 0)

  const maxStartX = Math.max(0, mapWidth - visibleX)
  const maxStartY = Math.max(0, mapHeight - visibleY)
  const startX = Math.max(0, Math.min(maxStartX, Math.floor(playerX - visibleX / 2)))
  const startY = Math.max(0, Math.min(maxStartY, Math.floor(playerY - visibleY / 2)))
  const endX = Math.min(mapWidth, startX + visibleX)
  const endY = Math.min(mapHeight, startY + visibleY)
  const contentWidth = (endX - startX) * tileSize
  const contentHeight = (endY - startY) * tileSize

  return {
    startX,
    startY,
    endX,
    endY,
    offsetX: Math.max(0, (pixelWidth - contentWidth) / 2),
    offsetY: Math.max(0, (pixelHeight - contentHeight) / 2),
    tileSize
  }
}
