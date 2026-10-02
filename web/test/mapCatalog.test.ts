import assert from 'node:assert/strict'
import { test } from 'node:test'
import { MAP_CATALOG, mapEntry, mapEntryForGame, mapCatalogForGame, resolveMapQuery, resolveMapQueryForGame } from '../src/shared/mapCatalog.ts'

test('map catalog exposes the same named Red maps used by failures', () => {
  assert.ok(MAP_CATALOG.length > 200)
  assert.equal(mapEntry(0x18)?.name, 'ROUTE_13')
  assert.equal(mapEntry(0x93)?.name, 'POKEMON_TOWER_6F')
})

test('map lookup accepts names, friendly names, decimal and explicit hex', () => {
  assert.equal(resolveMapQuery('ROUTE_13')?.id, 0x18)
  assert.equal(resolveMapQuery('Route 13')?.id, 0x18)
  assert.equal(resolveMapQuery('0x18')?.id, 0x18)
  assert.equal(resolveMapQuery('24')?.id, 24)
  assert.equal(resolveMapQuery('D4')?.name, 'SILPH_CO_7F')
})


test('map identity is qualified by game when numeric ids overlap', () => {
  assert.equal(mapEntryForGame('pokemon-red', 0x01)?.name, 'VIRIDIAN_CITY')
  assert.equal(mapEntryForGame('pokemon-gold', 0x0101)?.name, 'OLIVINE_POKECENTER_1F')
  assert.equal(mapEntryForGame('pokemon-silver', 0x0101)?.name, 'OLIVINE_POKECENTER_1F')
  assert.equal(mapEntryForGame('pokemon-gold', 0x01), undefined)
  assert.equal(resolveMapQueryForGame('pokemon-gold', '0x0101')?.name, 'OLIVINE_POKECENTER_1F')
  assert.ok(mapCatalogForGame('pokemon-gold').length > 300)
  assert.equal(mapCatalogForGame('tetris').length, 0)
})
