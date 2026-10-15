import assert from 'node:assert/strict'
import test from 'node:test'
import {
  ARCHIVE_VIEWS,
  activeView,
  archiveDashboardQuery,
  formatArchiveSearch,
  parseArchiveState,
  viewState
} from '../src/operator/runArchiveQuery.ts'

test('route → archive API params → route round-trips and keeps foreign keys', () => {
  const search = '?run=run-1&q=abc&game=pokemon-red&model=qwen+3&play_style=speedrun&success=1&sort=frames&dir=asc&page=3'
  const state = parseArchiveState(search)
  const query = archiveDashboardQuery(state, 25)
  assert.equal(query.status, 'done')
  assert.equal(query.search, 'abc')
  assert.equal(query.game, 'pokemon-red')
  assert.equal(query.model, 'qwen 3')
  assert.equal(query.play_style, 'speedrun')
  assert.equal(query.success, true)
  assert.equal(query.sort, 'frames')
  assert.equal(query.direction, 'asc')
  assert.equal(query.offset, 50)
  assert.equal(query.limit, 25)
  assert.deepEqual(parseArchiveState(formatArchiveSearch(state, search)), state)
  assert.equal(new URLSearchParams(formatArchiveSearch(state, search)).get('run'), 'run-1')
})

test('defaults stay out of the URL and junk values fall back', () => {
  assert.equal(formatArchiveSearch(parseArchiveState(''), '?run=x'), '?run=x')
  const junk = parseArchiveState('?sort=drop&dir=sideways&page=-4')
  assert.equal(junk.sort, 'finished')
  assert.equal(junk.dir, 'desc')
  assert.equal(junk.page, 1)
  assert.equal(archiveDashboardQuery(junk, 25).success, undefined)
})

test('views are active only on an exact preset match', () => {
  for (const view of ARCHIVE_VIEWS) assert.equal(activeView(viewState(view)), view.id)
  const filtered = { ...viewState(ARCHIVE_VIEWS[1]), facets: { ...viewState(ARCHIVE_VIEWS[1]).facets, game: 'pokemon-red' } }
  assert.equal(activeView(filtered), '')
  assert.equal(activeView({ ...viewState(ARCHIVE_VIEWS[0]), sort: 'rounds' }), '')
})
