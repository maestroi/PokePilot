import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  GAME_BOY_FPS,
  averageRunSpeed,
  elapsedRunSeconds,
  formatDuration,
  gameTimeSeconds,
  runTimingSummary,
  thinkingTimeSeconds
} from '../src/shared/runTiming.ts'

test('game time uses the native Game Boy cadence rather than configured runner FPS', () => {
  const frame = Math.round(GAME_BOY_FPS * 2 * 60 * 60)
  assert.ok(Math.abs(gameTimeSeconds({ frame }) - 7200) < 0.02)
})

test('elapsed time prefers started_at and freezes completed runs at ended_at', () => {
  assert.equal(elapsedRunSeconds({ status: 'running', started_at: 100, queued_at: 50 }, 220), 120)
  assert.equal(elapsedRunSeconds({ status: 'done', queued_at: 100, ended_at: 175 }, 999), 75)
})

test('thinking time is recovered from the cumulative call average', () => {
  assert.equal(thinkingTimeSeconds({ stats: { calls: 12, avg_seconds: 2.5 } }), 30)
})

test('average speed compares 1x game time with elapsed wall time', () => {
  const frame = Math.round(GAME_BOY_FPS * 3600)
  const speed = averageRunSpeed({ frame, status: 'running', queued_at: 100 }, 1000)
  assert.ok(speed != null)
  assert.ok(Math.abs(speed - 4) < 0.001)
})

test('summary is compact and includes game, elapsed, speed, and thinking time', () => {
  const frame = Math.round(GAME_BOY_FPS * 3600)
  const summary = runTimingSummary({
    frame,
    status: 'done',
    queued_at: 100,
    ended_at: 1000,
    stats: { calls: 10, avg_seconds: 3 }
  })
  assert.equal(summary, '1h @ 1× · 15m elapsed · 4.0× avg · 30s thinking')
  assert.equal(formatDuration(93784), '1d 2h')
})
