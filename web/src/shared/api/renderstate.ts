export type RenderCapability =
  | 'map'
  | 'camera'
  | 'player'
  | 'entities'
  | 'layers'
  | 'dialogue'
  | 'menu'
  | 'battle'
  | 'transition'
  | 'effects'
  | string

export interface RenderPosition {
  x: number
  y: number
}

export interface RenderMovement {
  kind: string
  from: RenderPosition
  to: RenderPosition
  progress?: number
}

export interface RenderActor {
  id?: string
  kind: string
  label?: string
  appearance?: string
  position: RenderPosition
  facing?: string
  movement?: RenderMovement
}

export interface RenderCamera {
  x: number
  y: number
  width?: number
  height?: number
}

export interface RenderMap {
  id: string
  name?: string
  width?: number
  height?: number
  camera?: RenderCamera
}

export interface RenderTileCell {
  kind: string
  variant?: string
  tags?: string[]
}

export interface RenderTileLayer {
  id: string
  kind: string
  origin?: RenderPosition
  width: number
  height: number
  cells: RenderTileCell[]
}

export interface RenderState {
  schema_version: number
  game: {
    id: string
    revision: string
  }
  clock: {
    frame: number
    cycle?: number
    captured_at_unix_ms?: number
  }
  scene: string
  capabilities?: RenderCapability[]
  map?: RenderMap
  player?: RenderActor
  entities?: RenderActor[]
  layers?: RenderTileLayer[]
}
