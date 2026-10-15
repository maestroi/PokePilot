import assert from 'node:assert/strict'
import test from 'node:test'
import { presentOutcome, presentReplay, presentRunStatus } from '../src/shared/status.ts'

test('run statuses always carry a glyph and a word', () => {
  const cases: [string, string, string][] = [
    ['running', '●', 'Running'],
    ['paused', '‖', 'Paused'],
    ['queued', '○', 'Queued'],
    ['leased', '◐', 'Starting'],
    ['failed', '✕', 'Failed']
  ]
  for (const [status, glyph, label] of cases) {
    const shown = presentRunStatus({ status })
    assert.equal(shown.glyph, glyph, status)
    assert.equal(shown.label, label, status)
  }
  assert.equal(presentRunStatus({ status: 'running' }).live, true)
})

test('finished runs show their outcome, not just "done"', () => {
  assert.deepEqual(presentRunStatus({ status: 'done', reason: 'done' }), { label: 'Goal complete', tone: 'ok', glyph: '✓' })
  assert.equal(presentRunStatus({ status: 'done', reason: 'budget', stats: { goal_complete: true } }).label, 'Goal complete')
  assert.deepEqual(presentOutcome({ reason: 'objective_failed' }), { label: 'Objective failed', tone: 'danger', glyph: '✕' })
  assert.deepEqual(presentOutcome({ reason: 'cancelled' }), { label: 'Cancelled', tone: 'neutral', glyph: '■' })
  assert.deepEqual(presentOutcome({}), { label: 'Ended', tone: 'neutral', glyph: '■' })
})

test('unknown values stay visible with their raw label', () => {
  assert.deepEqual(presentRunStatus({ status: 'hibernating' }), { label: 'Hibernating', tone: 'neutral', glyph: '■' })
  assert.deepEqual(presentOutcome({ reason: 'force_ended' }), { label: 'Force ended', tone: 'neutral', glyph: '■' })
})

test('replay states', () => {
  assert.equal(presentReplay('generating', 41.6).label, 'Generating 42%')
  assert.equal(presentReplay('generating').label, 'Generating')
  assert.equal(presentReplay('ready').glyph, '✓')
  assert.equal(presentReplay('missing').label, 'No replay yet')
  assert.equal(presentReplay('error').tone, 'danger')
})
