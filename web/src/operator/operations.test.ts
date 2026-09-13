import assert from 'node:assert/strict'
import { test } from 'node:test'
import { gameMediaLabel, isLiveStatus, railFacts, railStatusLabel } from './operations.ts'

test('live statuses keep the game pump on', () => {
  assert.equal(isLiveStatus('running'), true)
  assert.equal(isLiveStatus('leased'), true)
  assert.equal(isLiveStatus('queued'), false)
  assert.equal(isLiveStatus('done'), false)
})

test('game chrome names live and ended media differently', () => {
  assert.equal(gameMediaLabel('running'), 'Live')
  assert.equal(gameMediaLabel('done'), 'Ended · last frame')
  assert.equal(gameMediaLabel('queued'), 'Waiting for a worker')
})

test('rail facts distinguish a live attempt from an ended run', () => {
  assert.equal(railStatusLabel({ status: 'running' }), 'running')
  assert.equal(railStatusLabel({ status: 'done', reason: 'blackout' }), 'ended')
  assert.match(railFacts({
    run_id: 'live-1',
    status: 'running',
    frame: 1200,
    attempts: 1
  }), /^live · frame 1200/)
  assert.match(railFacts({
    run_id: 'dead-1',
    status: 'done',
    reason: 'blackout',
    ended_at: 1
  }), /^ended · blackout/)
})
