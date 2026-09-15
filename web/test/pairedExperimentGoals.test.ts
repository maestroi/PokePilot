import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('../src/operator/PairedExperimentPanel.vue', import.meta.url), 'utf8')

test('paired experiment goal is a preset select instead of free text', () => {
  assert.match(source, /<select v-model="form\.goal"/)
  assert.match(source, /v-for="goal in GOAL_OPTIONS"/)
  assert.doesNotMatch(source, /<input v-model="form\.goal"/)
})
