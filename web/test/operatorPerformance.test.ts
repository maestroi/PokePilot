import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

function source(path: string): string {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

const app = source('../src/operator/App.vue')
const media = source('../src/operator/MediaView.vue')
const analytics = source('../src/operator/AnalyticsView.vue')
const archive = source('../src/operator/RunArchiveView.vue')
const failures = source('../src/operator/FailuresView.vue')
const polling = source('../src/shared/composables/usePollingResource.ts')
const live = source('../src/operator/LiveView.vue')
const operations = source('../src/operator/OperationsView.vue')

test('operator only downloads the active workspace module', () => {
  for (const name of [
    'LiveView', 'RunArchiveView', 'FailuresView', 'AnalyticsView',
    'MediaView', 'OperationsView', 'SettingsView', 'ToolsView'
  ]) {
    assert.match(app, new RegExp(`defineAsyncComponent\\(\\(\\) => import\\('\\./${name}\\.vue'\\)\\)`))
    assert.doesNotMatch(app, new RegExp(`import ${name} from`))
  }
})

test('media and experiment subviews are split from their parent workspaces', () => {
  for (const name of ['MediaBroadcasts', 'MediaJobs', 'MediaLibrary', 'MediaStudio', 'MediaFullE2E']) {
    assert.match(media, new RegExp(`defineAsyncComponent\\(\\(\\) => import\\('\\./${name}\\.vue'\\)\\)`))
  }
  for (const name of ['PairedExperimentPanel', 'LeaderboardPanel']) {
    assert.match(analytics, new RegExp(`defineAsyncComponent\\(\\(\\) => import\\('\\./${name}\\.vue'\\)\\)`))
  }
  assert.match(archive, /defineAsyncComponent\(\(\) => import\('\.\/RunCleanupPanel\.vue'\)\)/)
})

test('pollers stop background requests and refresh when the admin tab is visible again', () => {
  assert.match(polling, /document\.addEventListener\('visibilitychange', onVisibilityChange\)/)
  assert.match(polling, /document\.removeEventListener\('visibilitychange', onVisibilityChange\)/)
  assert.match(polling, /if \(document\.hidden\) \{[\s\S]*?suspend\(\)/)
  assert.match(polling, /controller\?\.abort\(\)/)
  assert.match(polling, /if \(data\.value !== null \|\| !immediate\) void refresh\(\)/)
})

test('expensive summary polling follows its backend data freshness', () => {
  assert.match(analytics, /intervalMs: 30000/)
  assert.match(archive, /getMediaRenderJobs\(200, signal\)[\s\S]*?intervalMs: 30000/)
  assert.match(failures, /getTriage\(signal\)[\s\S]*?intervalMs: 15000/)
})

test('the shell, Live, and Operations share one active dashboard request', () => {
  assert.match(app, /intervalMs: \(\) => activeView\.value === 'live'/)
  assert.match(app, /<LiveView[^>]+:active-resource="fleet"/)
  assert.match(app, /<OperationsView[^>]+:active-resource="fleet"/)
  assert.match(live, /const activeResource = props\.activeResource/)
  assert.match(operations, /= props\.activeResource/)
  assert.doesNotMatch(live, /getDashboard\(\{ active: true \}/)
  assert.doesNotMatch(operations, /getDashboard\(\{ active: true \}/)
})

test('polls do not overlap when a wall endpoint responds slowly', () => {
  assert.match(polling, /setTimeout\(\(\) => \{/)
  assert.match(polling, /void refresh\(\)\.then\(schedule\)/)
  assert.match(polling, /if \(!enabled \|\| document\.hidden \|\| timer !== null\) return/)
})
