import assert from 'node:assert/strict'
import test from 'node:test'

import { PresentationClock, presentationEntityKey } from '../src/shared/animationClock.ts'
import type { RenderState } from '../src/shared/api/renderstate.ts'

function state(frame: number, x: number, y = 5, map = 'route 1'): RenderState {
  return {
    schema_version: 1,
    game: { id: 'pokemon-red', revision: 'en-us-rev0' },
    clock: { frame, captured_at_unix_ms: 1_800_000_000_000 + frame },
    scene: 'overworld',
    capabilities: ['map', 'player', 'layers'],
    map: { id: map, width: 40, height: 40 },
    player: { id: 'player', kind: 'player', position: { x, y }, facing: 'right' }
  }
}

test('presentation clock smooths high-speed authoritative movement and catches up within a bound', () => {
  const clock = new PresentationClock({ minTweenMs: 40, maxTweenMs: 120 })
  clock.ingest(state(1, 2), 0)

  const fast = state(120, 10)
  fast.player!.movement = {
    kind: 'walk',
    from: { x: 10, y: 5 },
    to: { x: 11, y: 5 }
  }
  assert.equal(clock.ingest(fast, 100), 'none')

  const middle = clock.sample(140)
  assert.ok(middle.player)
  assert.ok(middle.player!.x > 2 && middle.player!.x < 10)
  assert.equal(middle.animating, true)

  const caughtUp = clock.sample(250)
  assert.deepEqual(caughtUp.player, { x: 10, y: 5 })
  assert.deepEqual(caughtUp.camera, { x: 10, y: 5 })
  assert.equal(caughtUp.animating, false)
})

test('repeated frames behave like a pause heartbeat and resume without a burst', () => {
  const clock = new PresentationClock({ minTweenMs: 40, maxTweenMs: 100, reconnectGapMs: 500 })
  clock.ingest(state(1, 1), 0)
  clock.ingest(state(2, 2), 100)
  assert.deepEqual(clock.sample(250).player, { x: 2, y: 5 })

  assert.equal(clock.ingest(state(2, 2), 900), 'reconnect')
  // Once reconnected, repeated identical frames refresh arrival time.
  assert.equal(clock.ingest(state(2, 2), 1000), 'none')
  assert.equal(clock.ingest(state(3, 3), 1100), 'none')
  const resumed = clock.sample(1140)
  assert.ok(resumed.player!.x > 2 && resumed.player!.x < 3)
})

test('long reconnects snap instead of replaying stale movement', () => {
  const clock = new PresentationClock({ reconnectGapMs: 500 })
  clock.ingest(state(1, 1), 0)
  assert.equal(clock.ingest(state(200, 9), 2000), 'reconnect')
  assert.deepEqual(clock.sample(2000).player, { x: 9, y: 5 })
  assert.equal(clock.sample(2000).animating, false)
})

test('map changes and large idle teleports are explicit discontinuities', () => {
  const clock = new PresentationClock({ teleportDistanceTiles: 5 })
  clock.ingest(state(1, 1), 0)

  assert.equal(clock.ingest(state(2, 2, 5, 'viridian city'), 100), 'map-change')
  assert.deepEqual(clock.sample(100).player, { x: 2, y: 5 })

  assert.equal(clock.ingest(state(3, 20, 5, 'viridian city'), 200), 'teleport')
  assert.deepEqual(clock.sample(200).player, { x: 20, y: 5 })
})

test('moving snapshots may cover larger high-speed gaps without being mistaken for teleports', () => {
  const clock = new PresentationClock({
    teleportDistanceTiles: 5,
    movingTeleportDistanceTiles: 20,
    minTweenMs: 40,
    maxTweenMs: 100
  })
  clock.ingest(state(1, 1), 0)

  const fast = state(60, 15)
  fast.player!.movement = {
    kind: 'bike',
    from: { x: 15, y: 5 },
    to: { x: 16, y: 5 }
  }
  assert.equal(clock.ingest(fast, 100), 'none')
  assert.ok(clock.sample(140).player!.x > 1)
  assert.ok(clock.sample(140).player!.x < 15)
})

test('frame rewinds snap and preserve deterministic source timing metadata', () => {
  const clock = new PresentationClock()
  clock.ingest(state(10, 5), 100)
  const rewind = state(4, 3)
  rewind.clock.captured_at_unix_ms = 123456
  assert.equal(clock.ingest(rewind, 200), 'rewind')

  const sample = clock.sample(200)
  assert.deepEqual(sample.player, { x: 3, y: 5 })
  assert.equal(sample.sourceFrame, 4)
  assert.equal(sample.sourceCapturedAtUnixMS, 123456)
})

test('stable entity identities interpolate independently and large entity jumps snap', () => {
  const clock = new PresentationClock({ entityTeleportDistanceTiles: 3, minTweenMs: 40, maxTweenMs: 100 })
  const first = state(1, 5)
  first.entities = [{ id: 'npc:1', kind: 'npc', position: { x: 4, y: 4 } }]
  clock.ingest(first, 0)

  const second = state(2, 6)
  second.entities = [{ id: 'npc:1', kind: 'npc', position: { x: 6, y: 4 } }]
  clock.ingest(second, 100)
  const key = presentationEntityKey(second.entities[0], 0)
  const mid = clock.sample(140).entityPositions.get(key)
  assert.ok(mid && mid.x > 4 && mid.x < 6)

  const third = state(3, 7)
  third.entities = [{ id: 'npc:1', kind: 'npc', position: { x: 20, y: 4 } }]
  clock.ingest(third, 200)
  assert.deepEqual(clock.sample(200).entityPositions.get(key), { x: 20, y: 4 })
})


test('emulator speed changes do not change presentation tween timing', () => {
  const slowFrames = new PresentationClock({ minTweenMs: 40, maxTweenMs: 100 })
  const fastFrames = new PresentationClock({ minTweenMs: 40, maxTweenMs: 100 })

  slowFrames.ingest(state(1, 2), 0)
  fastFrames.ingest(state(1, 2), 0)
  slowFrames.ingest(state(2, 8), 100)
  fastFrames.ingest(state(200, 8), 100)

  const slowSample = slowFrames.sample(140)
  const fastSample = fastFrames.sample(140)
  assert.deepEqual(slowSample.player, fastSample.player)
  assert.deepEqual(slowSample.camera, fastSample.camera)
})

test('authoritative movement progress is respected when supplied', () => {
  const clock = new PresentationClock()
  const moving = state(1, 10)
  moving.player!.movement = {
    kind: 'walk',
    from: { x: 10, y: 5 },
    to: { x: 11, y: 5 },
    progress: 0.25
  }
  clock.ingest(moving, 0)

  assert.deepEqual(clock.sample(0).player, { x: 10.25, y: 5 })
})
