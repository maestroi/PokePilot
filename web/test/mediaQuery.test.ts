import assert from 'node:assert/strict'
import test from 'node:test'
import type { DashboardRun, MediaRenderJob } from '../src/shared/api/types'
import { presentBroadcast, presentJob, isBroadcastOnAir } from '../src/shared/status.ts'
import {
  cancellable,
  filterJobs,
  filterLibrary,
  formatMediaHref,
  parseMediaState,
  removable,
  retryable,
  tabHref
} from '../src/operator/mediaQuery.ts'

test('route → state → route round-trips and keeps foreign keys', () => {
  const search = '?state=all&q=term&run=r1&media_q=red&media_view=finished&recording=failed&visibility=private&experiment=e1'
  const state = parseMediaState('#media/library', search)
  assert.deepEqual(state, { tab: 'library', run: 'r1', job: '', q: 'red', view: 'finished', recording: 'failed', visibility: 'private', jobView: 'all' })
  const href = formatMediaHref(state, search)
  const [query, hash] = href.split('#')
  assert.deepEqual(parseMediaState('#' + hash, query), state)
  const params = new URLSearchParams(query)
  assert.equal(params.get('q'), 'term', "Failures' q is not a Media key")
  assert.equal(params.get('state'), 'all')
  assert.equal(params.get('experiment'), 'e1')

  const jobs = parseMediaState('#media/jobs', '?job=j9&job_state=failed')
  assert.deepEqual(parseMediaState('#media/jobs', formatMediaHref(jobs).split('#')[0]), jobs)
})

test('defaults stay out of the URL and junk falls back', () => {
  assert.equal(formatMediaHref(parseMediaState('#media/nope', '?media_view=x&recording=?&job_state=z'), ''), '#media/library')
  assert.equal(formatMediaHref(parseMediaState('#media/studio', ''), '?experiment=keep'), '?experiment=keep#media/studio')
})

test('tab links keep the source run but drop a job outside Jobs', () => {
  const state = parseMediaState('#media/jobs', '?run=r1&job=j1')
  assert.equal(tabHref(state, 'library'), '?run=r1#media/library')
  assert.equal(tabHref(state, 'jobs'), '?run=r1&job=j1#media/jobs')
})

test('library filters by view, replay state, visibility and search', () => {
  const runs = [
    { run_id: 'live-1', status: 'running', game: 'tetris' },
    { run_id: 'done-ready', status: 'done', replay_available: true, goal: 'Beat the gym' },
    { run_id: 'done-none', status: 'done' }
  ] as DashboardRun[]
  const ctx = {
    isPublic: (run: DashboardRun) => run.run_id !== 'done-none',
    recordingState: (run: DashboardRun) => run.replay_available ? 'ready' : 'unavailable'
  }
  const ids = (search: string) => filterLibrary(runs, parseMediaState('#media/library', search), ctx).map((run) => run.run_id)
  assert.deepEqual(ids(''), ['live-1', 'done-ready', 'done-none'])
  assert.deepEqual(ids('?media_view=live'), ['live-1'])
  assert.deepEqual(ids('?recording=ready'), ['done-ready'])
  assert.deepEqual(ids('?recording=none'), ['done-none'])
  assert.deepEqual(ids('?visibility=private'), ['done-none'])
  assert.deepEqual(ids('?media_q=GYM'), ['done-ready'])
  assert.deepEqual(ids('?media_q=tetris'), ['live-1'])
})

test('jobs filter by state view and run; lifecycle guards', () => {
  const jobs = [
    { id: 'a', run_id: 'r1', state: 'rendering' },
    { id: 'b', run_id: 'r2', state: 'failed', failure_class: 'invalid_request' },
    { id: 'c', run_id: 'r1', state: 'ready' }
  ] as MediaRenderJob[]
  const ids = (search: string) => filterJobs(jobs, parseMediaState('#media/jobs', search)).map((job) => job.id)
  assert.deepEqual(ids('?job_state=active'), ['a'])
  assert.deepEqual(ids('?run=r1'), ['a', 'c'])
  assert.equal(cancellable(jobs[0]), true)
  assert.equal(retryable(jobs[1]), false, 'invalid requests are not retryable')
  assert.equal(removable(jobs[2]), true)
})

test('broadcast and job states carry glyph + word; only live counts as on air', () => {
  assert.deepEqual(presentBroadcast('healthy'), { label: 'Live', tone: 'ok', glyph: '●', live: true })
  assert.equal(presentBroadcast('starting').label, 'Starting')
  assert.equal(presentBroadcast('stopped').label, 'Offline')
  assert.equal(presentBroadcast('failed').glyph, '✕')
  assert.equal(presentBroadcast('warming').label, 'Warming')
  assert.equal(isBroadcastOnAir('starting'), false)
  assert.equal(isBroadcastOnAir('reconnecting'), false)
  assert.equal(isBroadcastOnAir('live'), true)
  assert.equal(presentJob('rendering', 42).label, 'Rendering 42%')
  assert.equal(presentJob('queued').glyph, '○')
  assert.equal(presentJob('ready').glyph, '✓')
})
