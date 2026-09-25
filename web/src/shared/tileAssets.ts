import type { RenderTileCell, RenderTileLayer } from './api/renderstate'

export interface TileImageReference {
  url: string
  source?: { x: number; y: number; size: number }
  repeat?: number
}

export function parseTileImageReference(reference: string): TileImageReference | null {
  const match = /^(\/[^#]+\.png(?:\?[^#]*)?)#tile=(\d+),(\d+),(\d+)$/.exec(reference)
  if (!match) return reference && !reference.includes('#') ? { url: reference } : null
  const column = Number(match[2])
  const row = Number(match[3])
  const size = Number(match[4])
  if (size < 1 || size > 256) return null
  const repeatMatch = /(?:\?|&)repeat=(\d+)/.exec(match[1])
  const repeat = repeatMatch ? Number(repeatMatch[1]) : 1
  if (!Number.isInteger(repeat) || repeat < 1 || repeat > 4) return null
  return { url: match[1], source: { x: column * size, y: row * size, size }, ...(repeat > 1 ? { repeat } : {}) }
}

function layerCell(layer: RenderTileLayer, x: number, y: number): RenderTileCell | undefined {
  const localX = x - (layer.origin?.x || 0)
  const localY = y - (layer.origin?.y || 0)
  if (localX < 0 || localY < 0 || localX >= layer.width || localY >= layer.height) return undefined
  return layer.cells[localY * layer.width + localX]
}

export function terrainAssetKey(assets: Record<string, string>, layer: RenderTileLayer, x: number, y: number): string | undefined {
  const cell = layerCell(layer, x, y)
  if (!cell) return undefined
  const kind = cell.kind || 'unknown'
  if (cell.variant && assets[`${kind}.${cell.variant}`]) return `${kind}.${cell.variant}`
  if (assets[`${kind}.center`]) {
    const north = layerCell(layer, x, y - 1)?.kind === kind
    const south = layerCell(layer, x, y + 1)?.kind === kind
    const west = layerCell(layer, x - 1, y)?.kind === kind
    const east = layerCell(layer, x + 1, y)?.kind === kind
    const vertical = !north ? 'top' : !south ? 'bottom' : 'middle'
    const horizontal = !west ? 'left' : !east ? 'right' : 'center'
    const key = `${kind}.${vertical}-${horizontal}`
    if (assets[key]) return key
    return `${kind}.center`
  }
  return assets[kind] ? kind : undefined
}
