import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  expandGen1Blocks,
  gen1TilesetAssetStem
} from '../src/shared/gen1TextureCodec.ts'

test('Gen 1 block expansion preserves 4x4 tile ordering', () => {
  const mapBlocks = new Uint8Array([0, 1])
  const blockset = new Uint8Array(32)
  for (let i = 0; i < blockset.length; i++) blockset[i] = i

  const tiles = expandGen1Blocks(mapBlocks, 2, 1, blockset)
  assert.equal(tiles.length, 8 * 4)
  assert.deepEqual([...tiles.slice(0, 8)], [0, 1, 2, 3, 16, 17, 18, 19])
  assert.deepEqual([...tiles.slice(8, 16)], [4, 5, 6, 7, 20, 21, 22, 23])
  assert.deepEqual([...tiles.slice(24, 32)], [12, 13, 14, 15, 28, 29, 30, 31])
})

test('tileset aliases match pret/pokered shared graphics', () => {
  assert.equal(gen1TilesetAssetStem('OVERWORLD'), 'overworld')
  assert.equal(gen1TilesetAssetStem('MART'), 'pokecenter')
  assert.equal(gen1TilesetAssetStem('DOJO'), 'gym')
  assert.equal(gen1TilesetAssetStem('MUSEUM'), 'gate')
  assert.equal(gen1TilesetAssetStem('REDS_HOUSE_2'), 'reds_house')
})
