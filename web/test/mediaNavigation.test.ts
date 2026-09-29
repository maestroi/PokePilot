import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const operator = readFileSync(new URL('../src/operator/App.vue', import.meta.url), 'utf8')
const operations = readFileSync(new URL('../src/operator/OperationsView.vue', import.meta.url), 'utf8')
const shell = readFileSync(new URL('../src/shared/components/AppShell.vue', import.meta.url), 'utf8')
const home = readFileSync(new URL('../src/spectator/PublicHome.vue', import.meta.url), 'utf8')
const spectatorHTML = readFileSync(new URL('../spectator.html', import.meta.url), 'utf8')
const liveView = readFileSync(new URL('../src/operator/LiveView.vue', import.meta.url), 'utf8')
const renderJobs = readFileSync(new URL('../src/operator/RenderJobsPanel.vue', import.meta.url), 'utf8')
const spectator = readFileSync(new URL('../src/spectator/App.vue', import.meta.url), 'utf8')
const publicControls = readFileSync(new URL('../src/operator/SpectatorView.vue', import.meta.url), 'utf8')
const runInspectorProxy = readFileSync(new URL('../../cmd/pokeui/run_inspector_proxy.go', import.meta.url), 'utf8')

test('media is a top-level admin workspace and spectator is not', () => {
  assert.match(operator, /media: 'Media'/)
  assert.match(operator, /<MediaView/)
  assert.doesNotMatch(operator, /spectator: 'Spectator'/)
  assert.doesNotMatch(operator, /<SpectatorView/)
})

test('render jobs no longer live in Operations', () => {
  assert.doesNotMatch(operations, /RenderJobsPanel/)
})

test('public world navigation is capability-gated', () => {
  assert.match(shell, /capabilities\.has\('worldMap'\)/)
  assert.match(home, /v-if="canExploreWorld"/)
})

test('legacy floating replay shortcut is removed', () => {
  assert.doesNotMatch(spectatorHTML, /position:fixed[^\n]+Replay library/)
})

test('run details expose a direct public replay action when media is ready', () => {
  assert.match(liveView, /publicReplayHref/)
  assert.match(liveView, /Watch replay/)
  assert.match(liveView, /replayURL/)
})

test('media render jobs expose real lifecycle controls and renderer labels', () => {
  assert.match(renderJobs, /retryMediaRenderJob/)
  assert.match(renderJobs, /cancelMediaRenderJob/)
  assert.match(renderJobs, /deleteMediaRenderJob/)
  assert.match(renderJobs, /cancellable\(job\)/)
  assert.match(renderJobs, /removable\(job\)/)
  assert.match(renderJobs, /Composited/)
  assert.match(renderJobs, /does not publish the replay/)
})

test('operator proxy allowlists media render mutations', () => {
  assert.match(runInspectorProxy, /POST \/v1\/media\/render-jobs\/\{id\}\/retry/)
  assert.match(runInspectorProxy, /POST \/v1\/media\/render-jobs\/\{id\}\/cancel/)
  assert.match(runInspectorProxy, /DELETE \/v1\/media\/render-jobs\/\{id\}/)
})

test('public publishing controls describe what is actually public', () => {
  assert.match(publicControls, /Publish on start/)
  assert.match(publicControls, /Public live/)
  assert.match(publicControls, /Replay public/)
  assert.match(publicControls, /Publish replay/)
  assert.match(publicControls, /Feature live/)
  assert.match(publicControls, /Open replay/)
  assert.match(publicControls, /canFeature\(run\)/)
})

test('public completed runs resolve from the archive and surface replay state', () => {
  assert.match(spectator, /runs\.value\.find\(\(run\) => run\.run_id === selectedRunID\.value\)/)
  assert.match(spectator, /Replay ready/)
  assert.match(spectator, /Replay rendering/)
  assert.match(spectator, /replayRenderProgress/)
})
