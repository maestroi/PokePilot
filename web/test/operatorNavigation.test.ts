import assert from 'node:assert/strict'
import test from 'node:test'
import { operatorHref, operatorRoute } from '../src/shared/urls.ts'

test('private navigation retains legacy workspace links', () => {
  assert.deepEqual(operatorRoute('#tools'), { view: 'new-run', section: '' })
  assert.deepEqual(operatorRoute('#new-run'), { view: 'new-run', section: '' })
  assert.deepEqual(operatorRoute('#media'), { view: 'media', section: 'library' })
  assert.deepEqual(operatorRoute('#operations'), { view: 'operations', section: '' })
  assert.deepEqual(operatorRoute('#media/render-jobs'), { view: 'media', section: 'jobs' })
  assert.deepEqual(operatorRoute('#unknown'), { view: 'live', section: '' })
})

test('private section links survive reload and browser back/forward', () => {
  assert.deepEqual(operatorRoute('#media/jobs'), { view: 'media', section: 'jobs' })
  assert.deepEqual(operatorRoute('#media/broadcasts'), { view: 'media', section: 'broadcasts' })
  assert.deepEqual(operatorRoute('#analytics/experiments'), { view: 'analytics', section: 'experiments' })
  assert.deepEqual(operatorRoute('#settings/models'), { view: 'settings', section: 'models' })
  assert.deepEqual(operatorRoute('#settings/destinations'), { view: 'settings', section: 'destinations' })
  assert.deepEqual(operatorRoute('#settings/defaults'), { view: 'settings', section: 'defaults' })
  assert.deepEqual(operatorRoute('#media/themes'), { view: 'settings', section: 'renderer' })
})

test('operator resource links preserve exact IDs and encode values', () => {
  const href = operatorHref('media', { section: 'jobs', job: 'job #1', run: 'run / 2' }, '?experiment=pair%2F3')
  assert.equal(href, '?experiment=pair%2F3&run=run+%2F+2&job=job+%231#media/jobs')
  assert.equal(operatorHref('media', { section: 'library', job: '' }, '?run=abc&job=older'), '?run=abc#media/library')
  assert.equal(operatorHref('settings', {}, '?run=a'), '?run=a#settings')
})
