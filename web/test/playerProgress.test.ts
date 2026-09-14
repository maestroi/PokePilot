import assert from 'node:assert/strict'
import { test } from 'node:test'
import { bagItemsLabel, bagMeter, dexDetail, dexMeter, milestonesLabel } from '../src/shared/playerProgress.ts'

test('meters hide when adapter totals are missing', () => {
  assert.equal(bagMeter({}), null)
  assert.equal(dexMeter({ dex_owned: 4 }), null)
  assert.equal(dexDetail({ dex_seen: 8 }), null)
})

test('meters use adapter capacity and Dex total', () => {
  assert.equal(bagMeter({ bag_used: 14, bag_capacity: 20 }), '14/20')
  assert.equal(bagMeter({ bag_capacity: 20 }), '0/20')
  assert.equal(dexMeter({ dex_owned: 12, dex_total: 151 }), '12/151')
  assert.equal(dexDetail({ dex_owned: 12, dex_seen: 18, dex_total: 151 }), '12 owned / 18 seen / 151')
})

test('inline lists join bag items and milestones', () => {
  assert.equal(bagItemsLabel({}), 'Empty')
  assert.equal(bagItemsLabel({ bag: [{ name: 'pokeball', quantity: 5 }, { name: 'potion', quantity: 2 }] }), 'pokeball ×5, potion ×2')
  assert.equal(milestonesLabel({}), 'No major milestones yet')
  assert.equal(milestonesLabel({ milestones: ['Pokédex', 'S.S. Ticket'] }), 'Pokédex, S.S. Ticket')
})
