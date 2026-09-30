import assert from 'node:assert/strict'
import test from 'node:test'
import { kantoRouteLine } from '../src/spectator/routeLine.ts'

test('route line marks earned badges and the first missing gym as next', () => {
  const line = kantoRouteLine({ player: { badges: ['Boulder', 'Cascade Badge', 'thunder'] } } as never)
  assert.deepEqual(line.stops.map((stop) => stop.earned), [true, true, true, false, false, false, false, false])
  assert.equal(line.stops.findIndex((stop) => stop.next), 3)
  assert.equal(line.earnedCount, 3)
  assert.equal(line.fill, 3 / 9)
})

test('route line fills to the furthest earned badge and has no next stop when complete', () => {
  const skipped = kantoRouteLine({ player: { badges: ['Soul'] } } as never)
  assert.equal(skipped.earnedCount, 1)
  assert.equal(skipped.fill, 5 / 9)
  const all = kantoRouteLine({ player: { badges: ['Boulder', 'Cascade', 'Thunder', 'Rainbow', 'Soul', 'Marsh', 'Volcano', 'Earth'] } } as never)
  assert.equal(all.stops.some((stop) => stop.next), false)
  assert.equal(kantoRouteLine(null).fill, 0)
})
