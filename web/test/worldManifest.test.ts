import assert from 'node:assert/strict'
import { test } from 'node:test'
import { RED_WORLD_MANIFEST } from '../src/shared/redWorldManifest.generated.ts'

const maps = RED_WORLD_MANIFEST
const byID = new Map(maps.map((map) => [map.id, map] as const))
const atlasMaps = maps.filter((map) => Number.isFinite(map.x) && Number.isFinite(map.y) && map.width > 0 && map.height > 0)

test('Route 3 exposes destinations from the decomp topology', () => {
  const route3 = byID.get(0x0e)
  assert.ok(route3)
  assert.deepEqual(
    route3.connections.map((connection) => [connection.direction, connection.to, connection.offsetBlocks]),
    [['north', 0x0f, 25], ['west', 0x02, -4]]
  )
})

test('outdoor atlas maps receive stable global placement', () => {
  assert.ok(atlasMaps.length >= 36)
  const pewter = byID.get(0x02)
  const route3 = byID.get(0x0e)
  assert.ok(pewter && route3)
  assert.equal(typeof pewter.x, 'number')
  assert.equal(typeof route3.x, 'number')
  assert.equal(Number(route3.x), Number(pewter.x) + pewter.width)
  assert.equal(Number(route3.y), Number(pewter.y) + 8)
})

test('every atlas connection points at a known map', () => {
  for (const map of atlasMaps) {
    for (const connection of map.connections) {
      assert.ok(byID.get(connection.to), `${map.name} points to missing map ${connection.to}`)
    }
  }
})

test('Pallet Town exposes runless people and signs', () => {
  const pallet = byID.get(0x00)
  assert.ok(pallet)
  assert.ok(pallet.pois.some((poi) => poi.kind === 'npc' && poi.label === 'Oak'))
  assert.ok(pallet.pois.some((poi) => poi.kind === 'sign'))
})


test('render metadata stays aligned with the vendored decomp headers', () => {
  const pallet = byID.get(0x00)
  assert.ok(pallet)
  assert.equal(pallet.sourceName, 'PalletTown')
  assert.equal(pallet.tileset, 'OVERWORLD')

  const route1 = [...byID.values()].find((map) => map.name === 'ROUTE_1')
  assert.ok(route1)
  assert.equal(route1.sourceName, 'Route1')
  assert.equal(route1.tileset, 'OVERWORLD')
})


test('every stitched outdoor connection agrees with global placement', () => {
  const atlasByID = new Map(atlasMaps.map((map) => [map.id, map] as const))

  for (const source of atlasMaps) {
    for (const connection of source.connections) {
      const destination = atlasByID.get(connection.to)
      if (!destination) continue

      const delta = connection.offsetBlocks * 2
      let expectedX = Number(source.x)
      let expectedY = Number(source.y)
      if (connection.direction === 'north') {
        expectedX += delta
        expectedY -= destination.height
      } else if (connection.direction === 'south') {
        expectedX += delta
        expectedY += source.height
      } else if (connection.direction === 'west') {
        expectedX -= destination.width
        expectedY += delta
      } else {
        expectedX += source.width
        expectedY += delta
      }

      assert.equal(
        Number(destination.x),
        expectedX,
        `${source.name} ${connection.direction} -> ${destination.name} has mismatched atlas x`
      )
      assert.equal(
        Number(destination.y),
        expectedY,
        `${source.name} ${connection.direction} -> ${destination.name} has mismatched atlas y`
      )
    }
  }
})

test('every stitched atlas map has decomp render metadata', () => {
  for (const map of atlasMaps) {
    assert.ok(map.sourceName, `${map.name} is missing sourceName`)
    assert.ok(map.tileset, `${map.name} is missing tileset`)
  }
})
