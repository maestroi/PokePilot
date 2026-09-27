import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const app = readFileSync(new URL('../src/spectator/App.vue', import.meta.url), 'utf8')
const types = readFileSync(new URL('../src/shared/api/spectator.ts', import.meta.url), 'utf8')

test('public Tetris spectator exposes a safe Jev placement summary', () => {
  assert.match(types, /export interface SpectatorDecisionEngine/)
  assert.match(types, /decision_reference_agreements\?: number/)
  assert.match(types, /decision_records\?: SpectatorDecisionRecord\[\]/)
  assert.match(app, /Placement model/)
  assert.match(app, /Latest confidence/)
  assert.match(app, /Policy agreement/)
  assert.match(app, /Fallback rate/)
  assert.match(app, /Avg latency/)
  assert.match(app, /Recent Jev choices/)
  assert.match(app, /not ground-truth accuracy/)
})

test('Jev choices also appear in public live activity', () => {
  assert.match(app, /pushActivity\(run\.run_id, 'decision', 'Jev decision'/)
  assert.match(app, /typedDecisionActivityDetail/)
  assert.match(app, /decision_calls/)
})
