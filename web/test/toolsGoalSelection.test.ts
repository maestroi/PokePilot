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
