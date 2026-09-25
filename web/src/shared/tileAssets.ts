import type { RenderTileCell, RenderTileLayer } from './api/renderstate'

export interface TileImageReference {
  url: string
  source?: { x: number; y: number; size: number }
}

// A bundled theme may refer to one tile within a packed, square-cell atlas.
// The URL remains a normal image URL; the fragment is presentation metadata.
export function parseTileImageReference(reference: string): TileImageReference | null {
  const match = /^(\/[^#]+\.png)#tile=(\d+),(\d+),(\d+)$/.exec(reference)
  if (!match) return reference && !reference.includes('#') ? { url: reference } : null
  const column = Number(match[2])
  const row = Number(match[3])
  const size = Number(match[4])
  if (size < 1 || size > 256) return null
  return { url: match[1], source: { x: column * size, y: row * size, size } }
}

function layerCell(layer: RenderTileLayer, x: number, y: number): RenderTileCell | undefined {
  const localX = x - (layer.origin?.x || 0)
  const localY = y - (layer.origin?.y || 0)
  if (localX < 0 || localY < 0 || localX >= layer.width || localY >= layer.height) return undefined
  return layer.cells[localY * layer.width + localX]
}

// Nine-slice terrain uses adjacent semantic cells, never ROM-specific tile IDs.
// A missing adjacent cell is an edge. Other themes can supply just the base key.
export function terrainAssetKey(
  assets: Record<string, string>,
  layer: RenderTileLayer,
  x: number,
  y: number
): string | undefined {
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
