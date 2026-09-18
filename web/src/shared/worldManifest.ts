import { RED_WORLD_MANIFEST } from './redWorldManifest.generated'

export type WorldDirection = 'north' | 'south' | 'west' | 'east'

export interface WorldConnection {
  direction: WorldDirection
  to: number
  offsetBlocks: number
}

export type WorldPoiKind = 'npc' | 'trainer' | 'item' | 'sign' | 'encounter' | 'object'

export interface WorldPoi {
  x: number
  y: number
  kind: WorldPoiKind
  label: string
  sprite?: string
  spriteAsset?: string | null
  movement?: string
  facing?: string
  textSymbol?: string
  trainerClass?: string
  trainerNumber?: number | null
  item?: string
  species?: string
  level?: number
}

export interface WorldMapMeta {
  id: number
  name: string
  width: number
  height: number
  sourceName: string | null
  tileset: string | null
  x?: number
  y?: number
  connections: readonly WorldConnection[]
  pois: readonly WorldPoi[]
}

export const WORLD_MANIFEST = RED_WORLD_MANIFEST as readonly WorldMapMeta[]
const WORLD_BY_ID = new Map(WORLD_MANIFEST.map((map) => [map.id, map] as const))

export const WORLD_ATLAS_MAPS = WORLD_MANIFEST.filter((map) => {
  return Number.isFinite(map.x) && Number.isFinite(map.y) && map.width > 0 && map.height > 0
})

export function worldMapMeta(id: number): WorldMapMeta | undefined {
  return WORLD_BY_ID.get(Number(id))
}

export function worldConnections(id: number): readonly WorldConnection[] {
  return worldMapMeta(id)?.connections || []
}

export function worldPois(id: number): readonly WorldPoi[] {
  return worldMapMeta(id)?.pois || []
}
