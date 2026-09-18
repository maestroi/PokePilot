const TILES_PER_BLOCK = 4
const BLOCK_TILE_COUNT = TILES_PER_BLOCK * TILES_PER_BLOCK

const TILESET_ASSET_STEMS: Readonly<Record<string, string>> = {
  OVERWORLD: 'overworld',
  REDS_HOUSE_1: 'reds_house',
  MART: 'pokecenter',
  FOREST: 'forest',
  REDS_HOUSE_2: 'reds_house',
  DOJO: 'gym',
  POKECENTER: 'pokecenter',
  GYM: 'gym',
  HOUSE: 'house',
  FOREST_GATE: 'gate',
  MUSEUM: 'gate',
  UNDERGROUND: 'underground',
  GATE: 'gate',
  SHIP: 'ship',
  SHIP_PORT: 'ship_port',
  CEMETERY: 'cemetery',
  INTERIOR: 'interior',
  CAVERN: 'cavern',
  LOBBY: 'lobby',
  MANSION: 'mansion',
  LAB: 'lab',
  CLUB: 'club',
  FACILITY: 'facility',
  PLATEAU: 'plateau'
}

export function gen1TilesetAssetStem(tileset: string | null | undefined): string | null {
  if (!tileset) return null
  return TILESET_ASSET_STEMS[tileset] || null
}

export function expandGen1Blocks(
  mapBlocks: Uint8Array,
  blockWidth: number,
  blockHeight: number,
  blockset: Uint8Array
): Uint8Array {
  if (!Number.isInteger(blockWidth) || blockWidth <= 0 || !Number.isInteger(blockHeight) || blockHeight <= 0) {
    throw new Error(`invalid block dimensions ${blockWidth}x${blockHeight}`)
  }
  const expectedBlocks = blockWidth * blockHeight
  if (mapBlocks.length !== expectedBlocks) {
    throw new Error(`map block count ${mapBlocks.length} does not match ${blockWidth}x${blockHeight}`)
  }
  if (blockset.length % BLOCK_TILE_COUNT !== 0) {
    throw new Error(`blockset length ${blockset.length} is not divisible by ${BLOCK_TILE_COUNT}`)
  }

  const blockCount = blockset.length / BLOCK_TILE_COUNT
  const widthTiles = blockWidth * TILES_PER_BLOCK
  const heightTiles = blockHeight * TILES_PER_BLOCK
  const tiles = new Uint8Array(widthTiles * heightTiles)

  for (let blockY = 0; blockY < blockHeight; blockY++) {
    for (let blockX = 0; blockX < blockWidth; blockX++) {
      const blockID = mapBlocks[blockY * blockWidth + blockX]
      if (blockID >= blockCount) {
        throw new Error(`map references block 0x${blockID.toString(16).padStart(2, '0')} but blockset only has ${blockCount} blocks`)
      }
      const blockOffset = blockID * BLOCK_TILE_COUNT

      for (let tileY = 0; tileY < TILES_PER_BLOCK; tileY++) {
        for (let tileX = 0; tileX < TILES_PER_BLOCK; tileX++) {
          const destinationX = blockX * TILES_PER_BLOCK + tileX
          const destinationY = blockY * TILES_PER_BLOCK + tileY
          tiles[destinationY * widthTiles + destinationX] = blockset[blockOffset + tileY * TILES_PER_BLOCK + tileX]
        }
      }
    }
  }

  return tiles
}
