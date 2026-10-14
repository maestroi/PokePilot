import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { experimentProgress, experimentStage, parseExperimentSeeds } from '../src/operator/experimentPresentation.ts'
import type { ExperimentView } from '../src/shared/api/types.ts'
import { operatorHref, operatorRoute } from '../src/shared/urls.ts'

const paired = readFileSync(new URL('../src/operator/PairedExperimentPanel.vue', import.meta.url), 'utf8')
const analytics = readFileSync(new URL('../src/operator/AnalyticsView.vue', import.meta.url), 'utf8')
const leaderboard = readFileSync(new URL('../src/operator/LeaderboardPanel.vue', import.meta.url), 'utf8')
const archive = readFileSync(new URL('../src/operator/RunArchiveView.vue', import.meta.url), 'utf8')
const live = readFileSync(new URL('../src/operator/LiveView.vue', import.meta.url), 'utf8')

test('seed input is validated without silently dropping requested comparisons', () => {
  assert.deepEqual(parseExperimentSeeds('', 20), { seeds: [], error: '' })
  assert.deepEqual(parseExperimentSeeds('1, 7 42', 20), { seeds: [1, 7, 42], error: '' })
  assert.deepEqual(parseExperimentSeeds('-1,0,4', 3), { seeds: [-1, 0, 4], error: '' })
  assert.match(parseExperimentSeeds('1, garbage, 3', 20).error, /whole numbers/)
  assert.match(parseExperimentSeeds('1,1', 20).error, /unique/)
  assert.match(parseExperimentSeeds('9007199254740992', 20).error, /safe/)
  assert.match(parseExperimentSeeds('', 0).error, /1 to 1000/)
  assert.match(parseExperimentSeeds('', 1001).error, /1 to 1000/)
})

test('progress distinguishes active, complete and excluded matched pairs', () => {
  const active: ExperimentView = {
    id: 'pair-1', name: 'Comparison', total_pairs: 3,
    pairs: [
      { seed: 1, comparable: true, status_a: 'done', status_b: 'done', run_id_a: 'run-1-a', run_id_b: 'run-1-b' },
      { seed: 2, comparable: true, status_a: 'running', status_b: 'queued' },
      { seed: 3, comparable: false, status_a: 'done', status_b: 'done', non_comparable_reason: 'config mismatch' }
    ]
  }
  assert.deepEqual(experimentProgress(active), { total: 3, completed: 1, inProgress: 1, excluded: 1 })
  assert.equal(experimentStage(active), 'In progress')
  assert.equal(experimentStage({ ...active, pairs: [
    { seed: 1, comparable: true, status_a: 'done', status_b: 'done' },
    { seed: 2, comparable: true, status_a: 'done', status_b: 'done' },
    { seed: 3, comparable: false, status_a: 'done', status_b: 'done' }
  ] }), 'Completed · some pairs excluded')
  assert.equal(experimentStage({ id: 'all-excluded', name: 'Bad compare', total_pairs: 1, pairs: [
    { seed: 1, comparable: false }
  ] }), 'Excluded · no comparable pairs')
  assert.equal(experimentStage(null), 'Unavailable')
})

test('experiment history, exact member links and reverse links use durable IDs', () => {
  assert.match(paired, /getExperiment\(props.experimentID/)
  assert.match(paired, /operatorHref\('analytics', \{ section: 'experiments', experiment: experiment.id \}/)
  assert.match(paired, /operatorHref\('live', \{ run: pair.run_id_a, experiment: latest.id \}/)
  assert.match(paired, /operatorHref\('live', \{ run: pair.run_id_b, experiment: latest.id \}/)
  assert.match(paired, /window.history.pushState/)
  assert.match(paired, /PopStateEvent\('popstate'\)/)
  assert.match(archive, /operatorHref\('analytics', \{ section: 'experiments', experiment: run.experiment_id \}/)
  assert.match(live, /operatorHref\('analytics', \{ section: 'experiments', experiment: selectedRun.experiment_id \}/)
  const deepLink = operatorHref('analytics', { section: 'experiments', experiment: 'exp/seed #1' }, '?run=sample')
  assert.equal(deepLink, '?run=sample&experiment=exp%2Fseed+%231#analytics/experiments')
  assert.deepEqual(operatorRoute('#analytics/experiments'), { view: 'analytics', section: 'experiments' })
})

test('Analytics owns comparison and leaderboard; Operations/Runs do not duplicate it', () => {
  assert.match(analytics, /PairedExperimentPanel/)
  assert.match(analytics, /Endless-run aggregates/)
  assert.match(analytics, /LeaderboardPanel/)
  assert.match(leaderboard, /success: true, sort: 'frames', direction: 'asc'/)
  assert.doesNotMatch(archive, /fastestSuccessful\(/)
  assert.match(archive, /View leaderboards/)
})
