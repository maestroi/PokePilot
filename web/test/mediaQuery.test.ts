import assert from 'node:assert/strict'
import test from 'node:test'
import type { DashboardRun, MediaRenderJob } from '../src/shared/api/types'
import { presentBroadcast, presentJob, isBroadcastOnAir } from '../src/shared/status.ts'
import {
  cancellable,
  filterJobs,
  filterLibrary,
  formatMediaHref,
  jobOutputHref,
  jobTiming,
  latestJobByRun,
  parseMediaState,
  runOutputs,
  studioSteps,
  latestJobsByKind,
  presentRenderMark,
  sortJobs,
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
  assert.deepEqual(presentJob('ready'), { label: 'Done', tone: 'ok', glyph: '✓' })
})

test('job order is stable across progress polls (created time, then ID)', () => {
  const poll1 = [
    { id: 'b', run_id: 'r2', state: 'rendering', created_at_unix_ms: 200, updated_at_unix_ms: 900 },
    { id: 'a', run_id: 'r1', state: 'rendering', created_at_unix_ms: 100, updated_at_unix_ms: 950 },
    { id: 'c', run_id: 'r3', state: 'queued', created_at_unix_ms: 200, updated_at_unix_ms: 200 }
  ] as MediaRenderJob[]
  // Next poll: the older job heartbeated last, and the server returned it first.
  const poll2 = [{ ...poll1[1], updated_at_unix_ms: 990 }, poll1[0], poll1[2]]
  assert.deepEqual(sortJobs(poll1).map((job) => job.id), ['b', 'c', 'a'])
  assert.deepEqual(sortJobs(poll2).map((job) => job.id), ['b', 'c', 'a'])
})

test('job timing shows an estimate only when segment progress allows', () => {
  const start = 1_000_000
  const job = { state: 'rendering', started_at_unix_ms: start, segments_done: 1, segments_total: 4 } as MediaRenderJob
  assert.deepEqual(jobTiming(job, start + 60_000), { elapsed: '1m 00s', eta: '~3m 00s left' })
  assert.deepEqual(jobTiming({ ...job, segments_done: 0 }, start + 60_000), { elapsed: '1m 00s', eta: '' })
  assert.deepEqual(jobTiming({ ...job, segments_total: 0 }, start + 5_000), { elapsed: '5s', eta: '' })
  assert.deepEqual(jobTiming({ ...job, state: 'ready', finished_at_unix_ms: start + 90_000 }, start + 999_999), { elapsed: '1m 30s', eta: '' })
  assert.deepEqual(jobTiming({ state: 'queued' } as MediaRenderJob), { elapsed: '', eta: '' })
})

test('a done job links to its output; other outputs never stand in for the replay video', () => {
  const state = parseMediaState('#media/jobs', '?job=j1')
  assert.equal(jobOutputHref({ mode: 'semantic', run_id: 'r1', state: 'ready' }, state), '?run=r1#media/library')
  assert.equal(jobOutputHref({ mode: 'highlight', run_id: 'r1', state: 'ready' }, state), '?run=r1#media/studio')
  assert.equal(jobOutputHref({ mode: 'semantic', run_id: 'r1', state: 'failed' }, state), '')
  const latest = latestJobByRun([
    { id: 'replay', run_id: 'r1', mode: 'semantic', state: 'failed', updated_at_unix_ms: 1 },
    { id: 'reel', run_id: 'r1', mode: 'highlight', state: 'ready', updated_at_unix_ms: 9 }
  ] as MediaRenderJob[])
  assert.equal(latest.get('r1')?.id, 'replay')
})

test('run outputs say what exists and collapse what cannot be made into reasons', () => {
  const labels = (out: ReturnType<typeof runOutputs>) => out.rows.map((row) => `${row.label}: ${row.status.label}`)
  const done = runOutputs({
    replayData: true, live: false,
    replay: { state: 'ready' },
    highlights: { state: 'ready', windows: 3, duration_ms: 75_000 },
    packages: [{ hash: 'h', thumbnails: true, thumbnail_selected: true }]
  })
  assert.deepEqual(labels(done), ['Replay data: Stored', 'Replay video: Rendered', 'Highlight reel: Rendered', 'Episode package: 1 package', 'Thumbnails: Rendered · one selected'])
  assert.equal(done.rows[2].detail, '3 moments · 1m 15s')
  assert.deepEqual(done.unavailable, [])

  const dataOnly = runOutputs({ replayData: true, live: false, replay: { state: 'missing' }, highlights: { state: 'empty' }, packages: null, packagesError: 'studio artifact storage disabled' })
  assert.deepEqual(labels(dataOnly), ['Replay data: Stored', 'Replay video: Not rendered'])
  assert.equal(dataOnly.unavailable.length, 2)
  assert.match(dataOnly.unavailable[0], /No highlight-worthy moments/)

  const disabled = runOutputs({ replayData: true, live: false, replay: { state: 'disabled' }, highlights: { state: 'disabled' }, packages: [] })
  assert.deepEqual(labels(disabled), ['Replay data: Stored', 'Episode package: None'])
  assert.equal(disabled.unavailable.length, 2)

  const noData = runOutputs({ replayData: false, live: false, replay: { state: 'missing' } })
  assert.deepEqual(labels(noData), ['Replay data: None'])
  assert.match(noData.unavailable[0], /without replay data/)

  assert.equal(runOutputs({ replayData: false, live: false, replay: { state: 'ready' } }).rows[0].status.label, 'Stored', 'a video implies its data')

  const live = runOutputs({ replayData: false, live: true, replay: { state: 'missing' } })
  assert.deepEqual(labels(live), ['Replay data: Recording'])
  assert.match(live.unavailable[0], /after the run ends/)
})

test('studio pipeline orders the publishing steps and points at the first one not done', () => {
  const ids = (p: ReturnType<typeof studioSteps>) => p.steps.map((step) => `${step.id}:${step.status.label}`)
  const base = { replayData: true, replay: { state: 'ready' } } as const

  const fresh = studioSteps(base)
  assert.deepEqual(ids(fresh), ['video:Rendered', 'verdict:Not assessed', 'package:None', 'thumbnail:Not rendered', 'publish:Not queued'])
  assert.equal(fresh.current, 'verdict')

  assert.equal(studioSteps({ ...base, replay: { state: 'missing' } }).current, 'video')

  const archived = studioSteps({ ...base, assessment: { publication_class: 'archive_only', publication_reason: 'Too similar to a published run.' } })
  assert.deepEqual(ids(archived).slice(-1), ['verdict:Archive only'])
  assert.equal(archived.steps.at(-1)?.detail, 'Too similar to a published run.')
  assert.equal(archived.current, '')
  assert.match(archived.unavailable[0], /off YouTube/)

  const pkg = { plan_hash: 'p', titles: [{ id: 't', title: 'Episode title', angle: '' }], shorts: [{ id: 's', title: 'Short title', hook: '' }] }
  const short = studioSteps({ ...base, assessment: { publication_class: 'short_only', publication_reason: '' }, pkg, profile: 'short', edit: { state: 'ready' } })
  assert.equal(short.steps.find((s) => s.id === 'package')?.status.label, 'Short · edit ready')
  assert.equal(short.steps.find((s) => s.id === 'package')?.detail, 'Short title')
  assert.equal(short.current, 'publish', 'a Short needs no thumbnail')

  const episode = studioSteps({ ...base, assessment: { publication_class: 'full_episode', publication_reason: '' }, pkg, profile: 'episode', edit: { state: 'ready' }, thumbnails: { rendered: 4, selected: false } })
  assert.equal(episode.current, 'thumbnail')
  const done = studioSteps({ ...base, assessment: { publication_class: 'full_episode', publication_reason: '' }, pkg, profile: 'episode', edit: { state: 'ready' }, thumbnails: { rendered: 4, selected: true }, publication: { state: 'published' } })
  assert.equal(done.current, '')

  const unplanned = studioSteps({ ...base, assessment: { publication_class: 'full_episode', publication_reason: '' }, pkg: { ...pkg, plan_hash: undefined } })
  assert.equal(unplanned.current, 'package')

  const noData = studioSteps({ replayData: false, replay: { state: 'missing' } })
  assert.deepEqual(noData.steps, [])
  assert.match(noData.unavailable[0], /No recording/)

  const noStorage = studioSteps({ ...base, packagesError: 'studio artifact storage disabled' })
  assert.deepEqual(noStorage.steps.map((s) => s.id), ['video'])
  assert.equal(noStorage.current, '')
})


test('render marks keep the newest job of each kind per run', () => {
  const job = (id: string, mode: string, state: string, at: number) => ({ id, mode, state, run_id: 'r1', updated_at_unix_ms: at } as MediaRenderJob)
  const kinds = latestJobsByKind([job('v1', 'broadcast', 'failed', 1), job('v2', 'broadcast', 'ready', 2), job('h1', 'highlight', 'rendering', 3)]).get('r1')!
  assert.equal(kinds.video?.id, 'v2')
  assert.equal(kinds.highlight?.id, 'h1')
  assert.equal(kinds.edit, undefined)
  assert.equal(presentRenderMark(kinds.video).label, 'Done')
  assert.equal(presentRenderMark({ state: 'rendering', segments_done: 34, segments_total: 51 }).label, '34/51')
  assert.equal(presentRenderMark(undefined).glyph, '—')
})

test('Full E2E is a first-class Media route and keeps a selected source', () => {
  const state = parseMediaState('#media/e2e', '?run=e2e-test&experiment=keep')
  assert.equal(state.tab, 'e2e')
  assert.equal(state.run, 'e2e-test')
  assert.equal(tabHref(state, 'studio'), '?run=e2e-test#media/studio')
  assert.equal(tabHref(state, 'e2e'), '?run=e2e-test#media/e2e')
})
