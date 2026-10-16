import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { recordingMediaHref, recordingSummary } from '../src/operator/runRecording.ts'
import type { MediaRenderJob } from '../src/shared/api/types.ts'
import { operatorHref } from '../src/shared/urls.ts'

const history = readFileSync(new URL('../src/operator/RunArchiveView.vue', import.meta.url), 'utf8')
const pane = readFileSync(new URL('../src/operator/RunDetailPane.vue', import.meta.url), 'utf8')
const archiveQuery = readFileSync(new URL('../src/operator/runArchiveQuery.ts', import.meta.url), 'utf8')
const app = readFileSync(new URL('../src/operator/App.vue', import.meta.url), 'utf8')
const evidence = readFileSync(new URL('../src/operator/RunHistoricalEvidence.vue', import.meta.url), 'utf8')
const library = readFileSync(new URL('../src/operator/SpectatorView.vue', import.meta.url), 'utf8')
const jobs = readFileSync(new URL('../src/operator/RenderJobsPanel.vue', import.meta.url), 'utf8')

function job(state: string, id = 'job-1'): MediaRenderJob {
  return { id, state, run_id: 'run-1', version: 1, identity: '', artifact_key: '', mode: 'semantic', created_at_unix_ms: 1, updated_at_unix_ms: 2 }
}

test('recording readiness never equates a completed render job with playable video', () => {
  const ready = recordingSummary({ replay_available: true })
  assert.equal(ready.state, 'ready')
  assert.equal(ready.canWatch, true)
  assert.equal(ready.mediaSection, 'library')

  const syncing = recordingSummary({ replay_available: false }, job('ready'))
  assert.equal(syncing.state, 'syncing')
  assert.equal(syncing.canWatch, false)
  assert.equal(syncing.mediaSection, 'jobs')
  assert.equal(syncing.jobID, 'job-1')
  assert.match(syncing.detail, /not yet confirmed playable/)

  const existingWithFailure = recordingSummary({ replay_available: true }, job('failed'))
  assert.equal(existingWithFailure.canWatch, true)
  assert.match(existingWithFailure.detail, /latest render job failed/)
})

test('recording state separates queued, rendering, failed, cancelled, unavailable and loading', () => {
  assert.equal(recordingSummary({}, job('queued')).state, 'queued')
  assert.equal(recordingSummary({}, job('uploading')).state, 'rendering')
  assert.equal(recordingSummary({}, { ...job('rendering'), segments_total: 8, segments_done: 3 }).label, 'Rendering · 38%')
  assert.equal(recordingSummary({}, { ...job('failed'), last_error: 'render host offline' }).detail, 'render host offline')
  assert.equal(recordingSummary({}, job('cancelled')).state, 'cancelled')
  assert.equal(recordingSummary({}).state, 'unavailable')
  assert.equal(recordingSummary({}, undefined, 'stale').state, 'unknown')
  assert.equal(recordingSummary({}, undefined, 'loading').canWatch, false)
})

test('the archive searches server-side and keeps cleanup behind the history', () => {
  assert.match(archiveQuery, /search: state.q/)
  assert.match(history, /Search run ID/)
  assert.match(history, /getDashboard\(archiveDashboardQuery\(state.value, PAGE_SIZE\), signal\)/)
  // Bulk cleanup is not rendered on load: it lives in a drawer opened from the page header.
  assert.match(history, /<Dialog :open="cleanupOpen"[\s\S]*<RunCleanupPanel/)
  assert.match(history, /@click="cleanupOpen = true">Bulk cleanup…/)
  assert.match(history, /toggleVisibleSelection/)
  assert.match(history, /confirmDeleteSelected/)
  assert.match(history, /selectedRunIDs/)
  assert.match(history, /ResourceState/)
  assert.match(history, /role="region" aria-label="Finished run history table"/)
})

test('focused history context loads persisted evidence, survives navigation and avoids fake frame telemetry', () => {
  assert.match(app, /<RunArchiveView v-else-if="activeView === 'runs'" :run-id="linkedRunID"/)
  assert.match(pane, /getRun\(props.runId, signal\)/)
  assert.match(pane, /<RunHistoricalEvidence v-if="run.status === 'done'"/)
  assert.match(history, /window.history.pushState/)
  assert.match(history, /PopStateEvent\('popstate'\)/)
  assert.match(evidence, /getRunDebug\(props.runId/)
  assert.match(evidence, /getRunCheckpoints\(props.runId/)
  assert.match(evidence, /getRunArtifacts\(props.runId/)
  assert.match(evidence, /not live game telemetry or semantic annotations for every video frame/)
  // Inspect in Live navigates in-app (pushState + popstate), never a document reload.
  assert.match(pane, /operatorHref\('live', \{ run: id/)
  assert.match(history, /goTo\(operatorHref\('live', \{ run: runID/)
})

test('recording/source links preserve exact run and job IDs across views', () => {
  const runID = 'run/#with spaces'
  const jobID = 'job&x'
  const media = operatorHref('media', { section: 'jobs', run: runID, job: jobID, output: '' }, '?experiment=exp-1')
  assert.equal(media, '?experiment=exp-1&run=run%2F%23with+spaces&job=job%26x#media/jobs')
  const syncing = recordingSummary({ replay_available: false }, job('ready', jobID))
  assert.equal(recordingMediaHref(runID, syncing, '?experiment=exp-1'), media)
  assert.equal(recordingMediaHref(runID, recordingSummary({ replay_available: true })), '?run=run%2F%23with+spaces#media/library')
  assert.match(history, /recordingMediaHref\(run.run_id, recording\(run\)/)
  assert.match(history, /v-if="recording\(run\).canWatch"/)
  assert.match(library, /operatorHref\(run.status === 'done' \? 'runs' : 'live', \{ run: run.run_id, job: '' \}/)
  assert.match(jobs, /operatorHref\('runs', \{ run: job.run_id, job: '' \}/)
  assert.match(history, /safeIssueURL\(run\)/)
  assert.match(pane, /section: 'experiments', experiment: run.experiment_id/)
})

test('the run pane names the Pokémon starter and can be resized', () => {
  assert.match(pane, /starterPresentation/)
  assert.match(pane, /<dt>Starter<\/dt>/)
  assert.match(pane, /partyMonIsStarter/)
  assert.match(history, /aria-label="Resize run details"/)
  assert.match(history, /--run-pane/)
  assert.match(history, /onPaneResizeDown/)
})
