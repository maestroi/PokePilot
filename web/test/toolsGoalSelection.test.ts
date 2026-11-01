import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('../src/operator/ToolsView.vue', import.meta.url), 'utf8')

test('tools run goal stays independent from play style and purpose', () => {
  assert.doesNotMatch(source, /goalExplicitlySelected/)
  assert.doesNotMatch(source, /nextGoalForPlayStyle/)
  assert.doesNotMatch(source, /markGoalExplicitlySelected/)
  assert.match(source, /v-model="form\.goal"/)
  assert.match(source, /v-model="form\.play_style"/)
  assert.match(source, /v-model="form\.purpose"/)
  assert.match(source, /Debug coverage · exercise new interactions/)
})

test('Gen2 launcher allows unqualified 7/8-badge runs with visible warning', () => {
  assert.match(source, /\['Earn 7 badges\.', '7 badges · Glacier · Pryce · experimental'\]/)
  assert.match(source, /\['Earn 8 badges\.', '8 badges · Rising · Clair · experimental'\]/)
  assert.match(source, /\['progress:gs_indigo_plateau_reached', 'Indigo Plateau · Victory Road · experimental'\]/)
  assert.match(source, /\['progress:gs_league_will_defeated', 'Elite Four · Will · experimental'\]/)
  assert.doesNotMatch(source, /<option value="Earn (7|8) badges\." disabled/)
  assert.match(source, /Only cartridge-observed badge flags count as success/)
})
