import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('../src/operator/RunArchiveView.vue', import.meta.url), 'utf8')

test('run archive exposes per-row and visible-page selection controls', () => {
  assert.match(source, /selectedRunIDs/)
  assert.match(source, /toggleRunSelection\(run\.run_id\)/)
  assert.match(source, /toggleVisibleSelection/)
  assert.match(source, /Delete selected/)
})

test('bulk selection uses the safe per-run deletion helper', () => {
  assert.match(source, /deleteRuns\(/)
  assert.match(source, /\(id\) => deleteRun\(id\)/)
  assert.match(source, /DELETE_CONCURRENCY/)
})
