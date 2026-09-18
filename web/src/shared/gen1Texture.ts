import { worldMapMeta } from './worldManifest'
import { expandGen1Blocks, gen1TilesetAssetStem } from './gen1TextureCodec'

const TILE_SIZE = 8
const TILES_PER_BLOCK = 4
const GEN1_RENDER_REVISION = '0cd19d3'

export interface Gen1TextureAssetPaths {
  map: string
  blockset: string
  tileset: string
}

export interface Gen1TextureMap {
  map: number
  widthTiles: number
  heightTiles: number
  tileIDs: Uint8Array
  image: HTMLImageElement
  tileColumns: number
}

const bytesCache = new Map<string, Promise<Uint8Array>>()
const imageCache = new Map<string, Promise<HTMLImageElement>>()

function versionedAsset(path: string): string {
  return `${path}?rev=${GEN1_RENDER_REVISION}`
}

export function gen1TextureAssetPaths(mapID: number): Gen1TextureAssetPaths | null {
  const meta = worldMapMeta(Number(mapID))
  const stem = gen1TilesetAssetStem(meta?.tileset)
  if (!meta?.sourceName || !stem) return null
  return {
    map: versionedAsset(`/gen1/red/maps/${meta.sourceName}.blk`),
    blockset: versionedAsset(`/gen1/red/blocksets/${stem}.bst`),
    tileset: versionedAsset(`/gen1/red/tilesets/${stem}.png`)
  }
}

async function fetchBytes(url: string): Promise<Uint8Array> {
  let pending = bytesCache.get(url)
  if (!pending) {
    pending = fetch(url, { cache: 'force-cache' })
      .then(async (response) => {
        if (!response.ok) throw new Error(`${url} returned HTTP ${response.status}`)
        return new Uint8Array(await response.arrayBuffer())
      })
      .catch((cause) => {
        bytesCache.delete(url)
        throw cause
      })
    bytesCache.set(url, pending)
  }
  return pending
}

async function loadImage(url: string): Promise<HTMLImageElement> {
  let pending = imageCache.get(url)
  if (!pending) {
    pending = new Promise<HTMLImageElement>((resolve, reject) => {
      const image = new Image()
      image.decoding = 'async'
      image.onload = () => resolve(image)
      image.onerror = () => {
        imageCache.delete(url)
        reject(new Error(`failed to load ${url}`))
      }
      image.src = url
    })
    imageCache.set(url, pending)
  }
  return pending
}

export async function loadGen1TextureMap(mapID: number): Promise<Gen1TextureMap | null> {
  const meta = worldMapMeta(Number(mapID))
  const paths = gen1TextureAssetPaths(mapID)
  if (!meta || !paths) return null

  const blockWidth = meta.width / 2
  const blockHeight = meta.height / 2
  if (!Number.isInteger(blockWidth) || !Number.isInteger(blockHeight)) {
    throw new Error(`${meta.name} has odd field dimensions ${meta.width}x${meta.height}`)
  }

  const [mapBlocks, blockset, image] = await Promise.all([
    fetchBytes(paths.map),
    fetchBytes(paths.blockset),
    loadImage(paths.tileset)
  ])
  const tileIDs = expandGen1Blocks(mapBlocks, blockWidth, blockHeight, blockset)

  const tileColumns = Math.floor(image.naturalWidth / TILE_SIZE)
  const tileRows = Math.floor(image.naturalHeight / TILE_SIZE)
  if (tileColumns <= 0 || tileRows <= 0) {
    throw new Error(`${paths.tileset} is not an 8px-aligned tileset image`)
  }

  let maxTileID = 0
  for (const tileID of tileIDs) maxTileID = Math.max(maxTileID, tileID)
  if (maxTileID >= tileColumns * tileRows) {
    throw new Error(`${meta.name} needs tile 0x${maxTileID.toString(16)} but ${paths.tileset} only contains ${tileColumns * tileRows} tiles`)
  }

  return {
    map: Number(mapID),
    widthTiles: blockWidth * TILES_PER_BLOCK,
    heightTiles: blockHeight * TILES_PER_BLOCK,
    tileIDs,
    image,
    tileColumns
  }
}

export function drawGen1TextureMap(
  ctx: CanvasRenderingContext2D,
  texture: Gen1TextureMap,
  fieldCellPixels: number
): void {
  const destinationTileSize = fieldCellPixels / 2
  ctx.imageSmoothingEnabled = false

  for (let y = 0; y < texture.heightTiles; y++) {
    for (let x = 0; x < texture.widthTiles; x++) {
      const tileID = texture.tileIDs[y * texture.widthTiles + x]
      const sourceX = (tileID % texture.tileColumns) * TILE_SIZE
      const sourceY = Math.floor(tileID / texture.tileColumns) * TILE_SIZE
      ctx.drawImage(
        texture.image,
        sourceX,
        sourceY,
        TILE_SIZE,
        TILE_SIZE,
        x * destinationTileSize,
        y * destinationTileSize,
        destinationTileSize,
        destinationTileSize
      )
    }
  }
}
