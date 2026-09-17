import assert from 'node:assert/strict'
import { test } from 'node:test'
import { WORLD_ATLAS_MAPS, worldConnections, worldMapMeta } from '../src/shared/worldManifest.ts'

test('Route 3 exposes named-world destinations from the decomp topology', () => {
  const route3 = worldMapMeta(0x0e)
  assert.ok(route3)
  assert.deepEqual(
    worldConnections(0x0e).map((connection) => [connection.direction, connection.to, connection.offsetBlocks]),
    [['north', 0x0f, 25], ['west', 0x02, -4]]
  )
})

test('outdoor atlas maps receive stable global placement', () => {
  assert.ok(WORLD_ATLAS_MAPS.length >= 36)
  const pewter = worldMapMeta(0x02)
  const route3 = worldMapMeta(0x0e)
  assert.ok(pewter && route3)
  assert.equal(typeof pewter.x, 'number')
  assert.equal(typeof route3.x, 'number')
  assert.equal(Number(route3.x), Number(pewter.x) + pewter.width)
  assert.equal(Number(route3.y), Number(pewter.y) + 8)
})

test('every atlas connection points at a known map', () => {
  for (const map of WORLD_ATLAS_MAPS) {
    for (const connection of map.connections) {
      assert.ok(worldMapMeta(connection.to), `${map.name} points to missing map ${connection.to}`)
    }
  }
})
