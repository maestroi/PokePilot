import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const toolsSource = readFileSync(new URL('../src/operator/ToolsView.vue', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/shared/api/types.ts', import.meta.url), 'utf8')
const liveSource = readFileSync(new URL('../src/operator/LiveView.vue', import.meta.url), 'utf8')
const archiveSource = readFileSync(new URL('../src/operator/RunArchiveView.vue', import.meta.url), 'utf8')

test('run spec carries an optional fast decision engine selection without secrets', () => {
  assert.match(typesSource, /export interface DecisionEngineSpec[\s\S]*backend: 'off' \| 'jev' \| 'system-one'/)
  assert.match(typesSource, /export interface RunSpec[\s\S]*decision_engine\?: DecisionEngineSpec/)
  assert.doesNotMatch(typesSource, /export interface DecisionEngineSpec \{[^}]*(key|token)/i)
})

test('tools run form selects TypeSafe Jev independently of the strategist', () => {
  assert.match(toolsSource, /v-model="decision\.backend"/)
  assert.match(toolsSource, /<option value="jev">TypeSafe Jev<\/option>/)
  assert.match(toolsSource, /v-model="decision\.objectives"/)
  assert.match(toolsSource, /v-model="decision\.failures"/)
  assert.match(toolsSource, /v-model\.number="decision\.min_confidence"/)
  // Off (and scripted runs) send no selection, keeping the runner default.
  assert.match(toolsSource, /if \(!isLLM\.value \|\| !decisionEnabled\.value\) return undefined/)
  assert.match(toolsSource, /decision_engine: decisionRequest\(\)/)
})

test('live and archive views show the run decision engine', () => {
  assert.match(liveSource, /\['decision engine', decisionEngineLabel\(run\)\]/)
  assert.match(archiveSource, /decisionEngineLabel\(run\)/)
})
