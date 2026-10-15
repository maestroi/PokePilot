import assert from 'node:assert/strict'
import test from 'node:test'
import type { TriageGroup } from '../src/shared/api/types'
import { presentFailureGroup } from '../src/shared/status.ts'
import {
  filterGroups,
  formatFailuresSearch,
  investigateBlocker,
  parseFailuresState
} from '../src/operator/failuresQuery.ts'

const groups: TriageGroup[] = [
  { key: 'aaa', count: 2, pattern: 'stuck at ledge', run_ids: ['run-1'] },
  { key: 'bbb', count: 9, pattern: 'terminal error', run_ids: ['run-2', 'run-3'], issue: { issue_number: 7, status: 'open', updated_at: 100 } },
  { key: 'ccc', count: 40, pattern: 'old crash', issue: { issue_number: 3, status: 'closed', resolution: 'fixed', updated_at: 200 } }
]

test('route → state → route round-trips and keeps foreign keys', () => {
  const search = '?run=run-9&state=all&issue=linked&q=term&sort=updated&page=2&failure=bbb&job=j1'
  const state = parseFailuresState(search)
  assert.deepEqual(state, { view: 'all', issue: 'linked', q: 'term', sort: 'updated', page: 2, failure: 'bbb' })
  const out = formatFailuresSearch(state, search)
  assert.deepEqual(parseFailuresState(out), state)
  assert.equal(new URLSearchParams(out).get('job'), 'j1')
  assert.equal(new URLSearchParams(out).get('run'), 'run-9', "Live's ?run= is not a Failures key")
})

test('defaults stay out of the URL and junk falls back', () => {
  assert.equal(formatFailuresSearch(parseFailuresState('?state=nope&sort=x&page=-1&issue=?'), ''), '')
  assert.equal(formatFailuresSearch(parseFailuresState(''), '?job=j'), '?job=j')
})

test('views, facets, search (incl. run IDs) and sort', () => {
  const keys = (search: string) => filterGroups(groups, parseFailuresState(search)).map((group) => group.key)
  assert.deepEqual(keys(''), ['bbb', 'aaa'])
  assert.deepEqual(keys('?state=resolved'), ['ccc'])
  assert.deepEqual(keys('?state=all'), ['ccc', 'bbb', 'aaa'])
  assert.deepEqual(keys('?state=all&sort=updated'), ['ccc', 'bbb', 'aaa'])
  assert.deepEqual(keys('?issue=none'), ['aaa'])
  assert.deepEqual(keys('?q=LEDGE'), ['aaa'])
  assert.deepEqual(keys('?q=run-2'), ['bbb'])
})

test('investigate is blocked with a reason unless an open issue exists', () => {
  assert.equal(investigateBlocker(groups[0]), 'Needs a linked issue first')
  assert.equal(investigateBlocker(groups[1]), '')
  assert.equal(investigateBlocker(groups[2]), 'Already resolved')
  assert.equal(presentFailureGroup(groups[0], false).label, 'Needs issue')
  assert.equal(presentFailureGroup(groups[2], true).glyph, '✓')
  assert.equal(presentFailureGroup({ issue: { verification_state: 'regressed', circuit_open: true } }, false).label, 'Regressed')
})
