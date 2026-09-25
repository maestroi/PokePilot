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

export function hasOverworldSurface(state: RenderState | null | undefined): state is RenderState {
  if (!state || state.schema_version !== 1) return false
  const capabilities = new Set(state.capabilities || [])
  if (!capabilities.has('map') || !capabilities.has('player') || !capabilities.has('layers')) return false
  if (!state.map || !state.player) return false
  const width = Number(state.map.width || 0)
  const height = Number(state.map.height || 0)
  if (width <= 0 || height <= 0) return false
  const terrain = terrainLayer(state)
  return Boolean(terrain && terrain.width === width && terrain.height === height && terrain.cells.length === width * height)
}

export function canRenderOverworld(state: RenderState | null | undefined): state is RenderState {
  return Boolean(state?.scene === 'overworld' && hasOverworldSurface(state))
}

export function canRenderDialogue(state: RenderState | null | undefined): state is RenderState {
  if (!state || state.schema_version !== 1 || state.scene !== 'dialogue') return false
  const capabilities = new Set(state.capabilities || [])
  return capabilities.has('dialogue') && Boolean(state.dialogue?.text?.trim())
}

export function canRenderMenu(state: RenderState | null | undefined): state is RenderState {
  if (!state || state.schema_version !== 1 || state.scene !== 'menu') return false
  const capabilities = new Set(state.capabilities || [])
  return capabilities.has('menu') && Boolean(state.menu && (state.menu.title?.trim() || state.menu.entries?.length))
}

export function canRenderBattle(state: RenderState | null | undefined): state is RenderState {
  if (!state || state.schema_version !== 1 || state.scene !== 'battle') return false
  const capabilities = new Set(state.capabilities || [])
  const actors = state.battle?.actors || []
  return capabilities.has('battle') && actors.length >= 2
}

export type ModernSceneKind = 'overworld' | 'dialogue' | 'menu' | 'battle' | ''

export function modernSceneKind(state: RenderState | null | undefined): ModernSceneKind {
  if (canRenderBattle(state)) return 'battle'
  if (canRenderDialogue(state)) return 'dialogue'
  if (canRenderMenu(state)) return 'menu'
  if (canRenderOverworld(state)) return 'overworld'
  return ''
}

export function canRenderModernScene(state: RenderState | null | undefined): state is RenderState {
  return modernSceneKind(state) !== ''
}

export function semanticViewport(
  state: RenderState,
  pixelWidth: number,
  pixelHeight: number,
  preferredTileSize = 32,
  focus = state.player?.position
): SemanticViewport {
  const mapWidth = Math.max(1, Number(state.map?.width || 1))
  const mapHeight = Math.max(1, Number(state.map?.height || 1))
  const tileSize = Math.max(16, Math.min(48, preferredTileSize))
  const viewTilesX = Math.max(1, pixelWidth / tileSize)
  const viewTilesY = Math.max(1, pixelHeight / tileSize)
  const focusX = Number(focus?.x || 0)
  const focusY = Number(focus?.y || 0)

  const maxCameraX = Math.max(0, mapWidth - viewTilesX)
  const maxCameraY = Math.max(0, mapHeight - viewTilesY)
  const cameraX = Math.max(0, Math.min(maxCameraX, focusX + 0.5 - viewTilesX / 2))
  const cameraY = Math.max(0, Math.min(maxCameraY, focusY + 0.5 - viewTilesY / 2))

  const startX = Math.max(0, Math.floor(cameraX) - 1)
  const startY = Math.max(0, Math.floor(cameraY) - 1)
  const endX = Math.min(mapWidth, Math.ceil(cameraX + viewTilesX) + 1)
  const endY = Math.min(mapHeight, Math.ceil(cameraY + viewTilesY) + 1)

  const smallMapOffsetX = mapWidth <= viewTilesX ? Math.max(0, (pixelWidth - mapWidth * tileSize) / 2) : 0
  const smallMapOffsetY = mapHeight <= viewTilesY ? Math.max(0, (pixelHeight - mapHeight * tileSize) / 2) : 0

  return {
    startX,
    startY,
    endX,
    endY,
    offsetX: smallMapOffsetX - (cameraX - startX) * tileSize,
    offsetY: smallMapOffsetY - (cameraY - startY) * tileSize,
    tileSize
  }
}
