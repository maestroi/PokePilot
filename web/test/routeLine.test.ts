import assert from 'node:assert/strict'
import test from 'node:test'
import { kantoRouteLine } from '../src/spectator/routeLine.ts'

test('route line marks earned badges and the first missing gym as next', () => {
  const line = kantoRouteLine({ player: { badges: ['Boulder', 'Cascade Badge', 'thunder'] } } as never)
  assert.deepEqual(line.stops.map((stop) => stop.earned), [true, true, true, false, false, false, false, false])
  assert.equal(line.stops.findIndex((stop) => stop.next), 3)
  assert.equal(line.earnedCount, 3)
  assert.equal(line.completedGoals, 3)
  assert.equal(line.goalCount, 9)
  assert.equal(line.leagueNext, false)
  assert.equal(line.nextGoal, 'Rainbow Badge · Celadon')
  assert.equal(line.fill, 3 / 9)
})

test('route line marks Indigo League as the ninth goal after all badges', () => {
  const all = kantoRouteLine({
    player: { badges: ['Boulder', 'Cascade', 'Thunder', 'Rainbow', 'Soul', 'Marsh', 'Volcano', 'Earth'] }
  } as never)
  assert.equal(all.stops.some((stop) => stop.next), false)
  assert.equal(all.earnedCount, 8)
  assert.equal(all.completedGoals, 8)
  assert.equal(all.goalCount, 9)
  assert.equal(all.leagueNext, true)
  assert.equal(all.leagueEarned, false)
  assert.equal(all.nextGoal, 'Indigo League · Hall of Fame')
  assert.equal(all.fill, 8 / 9)
})

test('route line completes the ninth goal at Hall of Fame', () => {
  const complete = kantoRouteLine({
    player: {
      badges: ['Boulder', 'Cascade', 'Thunder', 'Rainbow', 'Soul', 'Marsh', 'Volcano', 'Earth'],
      milestones: ['Hall of Fame']
    }
  } as never)
  assert.equal(complete.completedGoals, 9)
  assert.equal(complete.leagueEarned, true)
  assert.equal(complete.leagueNext, false)
  assert.equal(complete.fill, 1)
})

test('route line fill follows the furthest earned badge', () => {
  const skipped = kantoRouteLine({ player: { badges: ['Soul'] } } as never)
  assert.equal(skipped.earnedCount, 1)
  assert.equal(skipped.fill, 5 / 9)
  assert.equal(kantoRouteLine(null).fill, 0)
})
