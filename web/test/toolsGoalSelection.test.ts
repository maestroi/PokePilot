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

test('Gen2 launcher keeps badge goals and one Hall of Fame target', () => {
  assert.match(source, /\['Earn 7 badges\.', '7 badges · Glacier · Pryce'\]/)
  assert.match(source, /\['Earn 8 badges\.', '8 badges · Rising · Clair'\]/)
  assert.match(source, /\['progress:gs_indigo_plateau_reached', 'Indigo Plateau · Victory Road · experimental'\]/)
  // Internal Elite Four and Champion milestones must not appear as separate
  // stop targets; the single Hall of Fame run progresses through them all.
  assert.doesNotMatch(source, /\['progress:gs_league_(?:will|koga|bruno|karen|lance)_defeated',/)
  assert.match(source, /Will, Koga, Bruno, Karen, and Lance are automatic internal stages/)
  assert.doesNotMatch(source, /Ice Path, Clair and Dragon’s Den are unfinished/)
  assert.match(source, /\['progress:gs_league_hall_of_fame', 'Hall of Fame · Johto League · experimental'\]/)
  assert.doesNotMatch(source, /<option value="Earn (7|8) badges\." disabled/)
  assert.match(source, /Only cartridge-observed badge and Hall of Fame events count as success/)
})
