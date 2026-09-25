import assert from 'node:assert/strict'
import test from 'node:test'

import { parseTileImageReference, terrainAssetKey } from '../src/shared/tileAssets.ts'
import type { RenderTileLayer } from '../src/shared/api/renderstate.ts'

test('packed atlas reference resolves to its exact source pixels', () => {
  assert.deepEqual(parseTileImageReference('/theme-assets/tilemap.png#tile=11,6,16'), {
    url: '/theme-assets/tilemap.png', source: { x: 176, y: 96, size: 16 }
  })
  assert.equal(parseTileImageReference('/theme-assets/tilemap.png#tile=0,0,0'), null)
  assert.equal(parseTileImageReference('/theme-assets/tilemap.png#unsupported'), null)
})

test('terrain asset joins a path to neighboring semantic tiles', () => {
  const layer = {
    id: 'terrain', kind: 'terrain', width: 3, height: 3,
    origin: { x: 0, y: 0 },
    cells: Array.from({ length: 9 }, () => ({ kind: 'path' }))
  } as RenderTileLayer
  const assets = {
    'path.center': 'center',
    'path.top-left': 'top-left',
    'path.middle-center': 'middle-center',
    'path.bottom-right': 'bottom-right'
  }
  assert.equal(terrainAssetKey(assets, layer, 0, 0), 'path.top-left')
  assert.equal(terrainAssetKey(assets, layer, 1, 1), 'path.middle-center')
  assert.equal(terrainAssetKey(assets, layer, 2, 2), 'path.bottom-right')
  assert.equal(terrainAssetKey(assets, layer, 4, 4), undefined)
})
