import assert from 'node:assert/strict'
import test from 'node:test'
import {
  eventKind,
  eventMatchesFilter,
  timelineFilterCounts
} from '../src/operator/runTimelineFilters.ts'

test('recovery source remains recovery when its copy mentions a failure', () => {
  const event = {
    source: 'recovery',
    kind: 'circuit',
    message: 'Repeated failure detected',
    detail: 'failure fingerprint repeated three times'
  }

  assert.equal(eventKind(event), 'recovery')
  assert.equal(eventMatchesFilter(event, 'recovery'), true)
  assert.equal(eventMatchesFilter(event, 'failure'), false)
  assert.equal(eventMatchesFilter(event, 'attention'), true)
})

test('failure-like events are included in the attention focus', () => {
  const event = {
    source: 'skill',
    kind: 'execution',
    message: 'go to route 4 failed',
    detail: 'navigation error'
  }

  assert.equal(eventKind(event), 'failure')
  assert.equal(eventMatchesFilter(event, 'failure'), true)
  assert.equal(eventMatchesFilter(event, 'attention'), true)
})

test('timeline filter counts keep semantic event categories separate', () => {
  const counts = timelineFilterCounts([
    { source: 'llm', kind: 'decision', message: 'go to route 4' },
    { source: 'recovery', kind: 'retry', message: 'Retrying after loss' },
    { source: 'skill', kind: 'execution', message: 'navigation error' },
    { source: 'milestone', kind: 'badge', message: 'Cascade Badge obtained' },
    { source: 'system', kind: 'checkpoint', message: 'checkpoint saved' }
  ])

  assert.deepEqual(counts, {
    all: 5,
    attention: 2,
    decision: 1,
    checkpoint: 1,
    progress: 1,
    failure: 1,
    recovery: 1,
    skill: 0,
    system: 0
  })
})
