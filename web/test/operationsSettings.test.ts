import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { DEFAULT_RUN_LAUNCH, parseRunLaunchDefaults, readRunLaunchDefaults, RUN_LAUNCH_DEFAULTS_KEY, saveRunLaunchDefaults } from '../src/operator/runLaunchDefaults.ts'
import { operatorHref, operatorRoute } from '../src/shared/urls.ts'

const operations = readFileSync(new URL('../src/operator/OperationsView.vue', import.meta.url), 'utf8')
const settings = readFileSync(new URL('../src/operator/SettingsView.vue', import.meta.url), 'utf8')
const launch = readFileSync(new URL('../src/operator/ToolsView.vue', import.meta.url), 'utf8')
const media = readFileSync(new URL('../src/operator/MediaView.vue', import.meta.url), 'utf8')
const broadcast = readFileSync(new URL('../src/operator/BroadcastPanel.vue', import.meta.url), 'utf8')
const operator = readFileSync(new URL('../src/operator/App.vue', import.meta.url), 'utf8')

test('Operations is a worker/queue control plane, not a results browser', () => {
  assert.match(operations, /title="Workers"/)
  assert.match(operations, /Queue and leases/)
  assert.match(operations, /Current operational signals/)
  assert.match(operations, /operatorHref\('live', \{ run: worker\.run_id \}/)
  assert.match(operations, /operatorHref\('live', \{ run: run\.run_id \}/)
  assert.match(operations, /forceEndWorker/)
  assert.doesNotMatch(operations, /Recent outcomes|Active attempts|LLMDeploymentsPanel|PairedExperimentPanel/)
  assert.doesNotMatch(operations, /getDashboard\(\{ status: 'done'/)
})

test('Settings owns real configuration or honestly read-only state', () => {
  assert.match(settings, /<LLMDeploymentsPanel/)
  assert.match(settings, /getBroadcastControl/)
  assert.match(settings, /credentials and stream keys are configured on the server/)
  assert.match(settings, /saveRunLaunchDefaults/)
  assert.match(settings, /pokepilot\.operator\.renderer/)
  assert.match(settings, /publicRenderThemeOptions/)
  assert.match(broadcast, /#settings\/destinations/)
  assert.doesNotMatch(media, /Renderer assets & themes/)
  assert.match(operator, /:section="activeSection"/)
})

test('run launch defaults survive save/read and reject invalid values', () => {
  assert.deepEqual(parseRunLaunchDefaults(null), DEFAULT_RUN_LAUNCH)
  assert.deepEqual(parseRunLaunchDefaults({ playStyle: 'speedrun', fps: 240, recoveryProfile: 'strict' }), {
    playStyle: 'speedrun', fps: 240, recoveryProfile: 'strict'
  })
  assert.deepEqual(parseRunLaunchDefaults({ playStyle: 'nonsense', fps: -1, recoveryProfile: 'bogus' }), DEFAULT_RUN_LAUNCH)
  const saved = new Map<string, string>()
  const storage = {
    getItem: (key: string) => saved.get(key) ?? null,
    setItem: (key: string, value: string) => { saved.set(key, value) }
  }
  saveRunLaunchDefaults({ playStyle: 'team_builder', fps: 0, recoveryProfile: 'resilient' }, storage)
  assert.ok(saved.has(RUN_LAUNCH_DEFAULTS_KEY))
  assert.equal(readRunLaunchDefaults(storage).playStyle, 'team_builder')
  assert.equal(readRunLaunchDefaults(storage).fps, 0)
  assert.match(launch, /const launchDefaults = readRunLaunchDefaults\(\)/)
  assert.match(launch, /recovery_profile: launchDefaults\.recoveryProfile/)
})

test('settings and legacy media theme routes are addressable', () => {
  assert.deepEqual(operatorRoute('#settings/renderer'), { view: 'settings', section: 'renderer' })
  assert.deepEqual(operatorRoute('#settings/defaults'), { view: 'settings', section: 'defaults' })
  assert.deepEqual(operatorRoute('#settings/destinations'), { view: 'settings', section: 'destinations' })
  assert.deepEqual(operatorRoute('#media/themes'), { view: 'settings', section: 'renderer' })
  assert.equal(operatorHref('settings', { section: 'destinations' }, '?run=example'), '?run=example#settings/destinations')
})
