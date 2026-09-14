import assert from 'node:assert/strict'
import { test } from 'node:test'
import { fpsLabel, railFacts } from '../src/operator/operations.ts'

test('configured run speed is presented as a multiplier with FPS context', () => {
  assert.equal(fpsLabel({ status: 'running', fps: 60 } as any), '1× target · 60 FPS')
  assert.equal(fpsLabel({ status: 'running', fps: 120 } as any), '2× target · 120 FPS')
  assert.equal(fpsLabel({ status: 'running', fps: 240 } as any), '4× target · 240 FPS')
  assert.equal(fpsLabel({ status: 'running', fps: 480 } as any), '8× target · 480 FPS')
  assert.equal(fpsLabel({ status: 'running', fps: 0 } as any), 'MAX · uncapped')
})

test('measured frame rate is converted to an average game-speed multiplier', () => {
  assert.equal(
    fpsLabel({ status: 'done', queued_at: 100, ended_at: 110, frame: 1200, fps: 120 } as any),
    '2.0× avg · 120.0 FPS'
  )
})

test('operation rail does not append an extra fps unit to speed labels', () => {
  assert.equal(
    railFacts({ status: 'running', frame: 0, fps: 120, attempts: 1 } as any),
    'live · frame 0 · 2× target · 120 FPS · attempt 1'
  )
})
