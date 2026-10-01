import assert from 'node:assert/strict'
import { test } from 'node:test'
import { pokemonRouteLine } from '../src/spectator/routeLine.ts'

test('Gold spectator route uses Johto gyms and New Bark Town', () => {
  const line = pokemonRouteLine({
    game: 'pokemon-gold',
    player: { money: 0, party: [], badges: ['zephyr', 'hive'] }
  })

  assert.equal(line.region, 'Johto')
  assert.equal(line.startTown, 'New Bark Town')
  assert.equal(line.stops[0]?.badge, 'Zephyr')
  assert.equal(line.stops[2]?.badge, 'Plain')
  assert.equal(line.earnedCount, 2)
  assert.equal(line.nextGoal, 'Plain Badge · Goldenrod')
})

test('Red spectator route remains Kanto', () => {
  const line = pokemonRouteLine({
    game: 'pokemon-red',
    player: { money: 0, party: [], badges: ['Boulder'] }
  })

  assert.equal(line.region, 'Kanto')
  assert.equal(line.startTown, 'Pallet Town')
  assert.equal(line.stops[0]?.badge, 'Boulder')
  assert.equal(line.earnedCount, 1)
})
