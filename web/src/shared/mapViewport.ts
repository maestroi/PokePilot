/** Largest bitmap edge the map canvas may allocate. Larger sizes crash the tab. */
export const MAP_CANVAS_MAX_EDGE = 4096

export const MAP_ZOOM_MIN = 0.5
export const MAP_ZOOM_MAX = 4
export const MAP_ZOOM_STEP = 0.25

export interface MapFitInput {
  viewportWidth: number
  viewportHeight: number
  mapWidth: number
  mapHeight: number
  zoom: number
  /** Fit the whole map inside the box. Zoom is what scales past that. */
  fillBox: boolean
  minTile?: number
  evenTiles?: boolean
}

export interface MapFit {
  tile: number
  zoom: number
  /** Highest zoom step whose bitmap still fits inside the canvas edge cap. */
  zoomCeiling: number
}

function stepZoom(zoom: number, mode: 'round' | 'floor'): number {
  if (!Number.isFinite(zoom)) return 1
  const steps = mode === 'floor'
    ? Math.floor((zoom + 1e-9) / MAP_ZOOM_STEP)
    : Math.round(zoom / MAP_ZOOM_STEP)
  const stepped = Math.min(MAP_ZOOM_MAX, Math.max(MAP_ZOOM_MIN, steps * MAP_ZOOM_STEP))
  return Number(stepped.toFixed(2))
}

export function clampMapZoom(zoom: number): number {
  return stepZoom(zoom, 'round')
}

export function fitMapTiles(input: MapFitInput): MapFit {
  const tilesW = Math.max(1, Math.floor(input.mapWidth))
  const tilesH = Math.max(1, Math.floor(input.mapHeight))
  const viewW = Math.max(1, input.viewportWidth)
  const viewH = Math.max(1, input.viewportHeight)
  const widthFit = Math.floor(viewW / tilesW)
  const heightFit = Math.floor(viewH / tilesH)
  const contain = Math.min(widthFit, heightFit)
  const minTile = Math.max(2, Math.floor(input.minTile ?? 2))
  // 100% shows the entire map. A readability floor must not magnify past the box.
  let fit = Math.max(2, contain)
  if (minTile > fit && minTile * tilesW <= viewW && minTile * tilesH <= viewH) fit = minTile
  const edgeCap = Math.max(1, Math.min(
    Math.floor(MAP_CANVAS_MAX_EDGE / tilesW),
    Math.floor(MAP_CANVAS_MAX_EDGE / tilesH)
  ))
  const zoomCeiling = stepZoom(Math.min(MAP_ZOOM_MAX, edgeCap / fit), 'floor')
  let zoom = clampMapZoom(input.zoom)
  if (zoom > zoomCeiling) zoom = zoomCeiling
  let tile = Math.floor(fit * zoom)
  if (input.evenTiles) tile = Math.floor(tile / 2) * 2
  if (tile > edgeCap) tile = input.evenTiles ? Math.floor(edgeCap / 2) * 2 : edgeCap
  tile = Math.max(input.evenTiles ? 2 : 1, tile)
  return { tile, zoom, zoomCeiling }
}
