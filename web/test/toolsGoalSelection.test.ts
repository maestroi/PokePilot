import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('../src/operator/ToolsView.vue', import.meta.url), 'utf8')

test('tools run goal marks explicit selection before play-style changes', () => {
  assert.match(source, /goalExplicitlySelected/)
  assert.match(source, /nextGoalForPlayStyle/)
  assert.match(source, /@change="markGoalExplicitlySelected"/)
})
