import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const toolsSource = readFileSync(new URL('../src/operator/ToolsView.vue', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/shared/api/types.ts', import.meta.url), 'utf8')
const liveSource = readFileSync(new URL('../src/operator/LiveView.vue', import.meta.url), 'utf8')
const archiveSource = readFileSync(new URL('../src/operator/RunArchiveView.vue', import.meta.url), 'utf8')
const panelSource = readFileSync(new URL('../src/operator/LLMDeploymentsPanel.vue', import.meta.url), 'utf8')
const experimentSource = readFileSync(new URL('../src/operator/PairedExperimentPanel.vue', import.meta.url), 'utf8')

test('run spec carries an optional fast decision engine selection without secrets', () => {
  assert.match(typesSource, /export interface DecisionEngineSpec[\s\S]*backend: 'off' \| 'jev' \| 'system-one'/)
  assert.match(typesSource, /export interface RunSpec[\s\S]*decision_engine\?: DecisionEngineSpec/)
  assert.doesNotMatch(typesSource, /export interface DecisionEngineSpec \{[^}]*(key|token)/i)
})

test('tools run form picks the decision engine from registered deployments', () => {
  assert.match(toolsSource, /v-model="decisionTarget"/)
  assert.match(toolsSource, /v-for="deployment in decisionDeployments"[\s\S]*:value="`deployment:\$\{deployment\.id\}`"/)
  // The strategist list never offers a choice-only API.
  assert.match(toolsSource, /v-for="deployment in strategists"/)
  assert.match(toolsSource, /deployments\.value\.filter\(servesStrategist\)/)
  // Runner-endpoint fallbacks exist only when no registry is configured.
  assert.match(toolsSource, /<template v-if="!deployments\.length">[\s\S]*value="env:jev"/)
  assert.match(toolsSource, /v-model="decision\.objectives"/)
  assert.match(toolsSource, /v-model="decision\.failures"/)
  assert.match(toolsSource, /v-model\.number="decision\.min_confidence"/)
  // Off (and scripted runs) send no selection, keeping the runner default.
  assert.match(toolsSource, /if \(!isLLM\.value \|\| !decisionSelected\.value\) return undefined/)
  assert.match(toolsSource, /decision_engine: decisionRequest\(\)/)
  // Clients name a deployment; only the wall writes its identity.
  assert.doesNotMatch(toolsSource, /inference:/)
})

test('deployment registry editor declares the wire protocol', () => {
  assert.match(typesSource, /export type DeploymentProtocol = '' \| 'openai' \| 'typesafe-choice'/)
  assert.match(panelSource, /v-model="draft\.protocol"/)
  assert.match(panelSource, /<option value="typesafe-choice">/)
  assert.match(panelSource, /value\.token_env \|\|= 'TYPESAFE_API_KEY'/)
  assert.match(experimentSource, /strategistDeployments\(modelsData\.value\?\.deployments/)
})

test('tools run form selects decision mode and shadow-only battles', () => {
  assert.match(typesSource, /export interface DecisionEngineSpec[\s\S]*mode\?: 'off' \| 'shadow' \| 'active'/)
  assert.match(typesSource, /export interface DecisionEngineSpec[\s\S]*battles\?: boolean/)
  assert.match(toolsSource, /v-model="decision\.mode"/)
  assert.match(toolsSource, /<option value="shadow">Shadow<\/option>/)
  assert.match(toolsSource, /v-model="decision\.battles" type="checkbox" :disabled="!decisionShadow"/)
  // Active runs never send battle decisions; the wall would reject them.
  assert.match(toolsSource, /battles: decisionShadow\.value && decision\.battles/)
})

test('live and archive views show the run decision engine', () => {
  assert.match(liveSource, /\['decision engine', decisionEngineLabel\(run\)\]/)
  assert.match(archiveSource, /decisionEngineLabel\(run\)/)
})
