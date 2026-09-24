import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { decisionCalibration, decisionFeed, decisionKindRows, hasDecisionTelemetry, latency, percent } from '../src/operator/decisionTelemetry.ts'
import type { DashboardStats, DecisionKindSummary } from '../src/shared/api/types.ts'

function kind(partial: Partial<DecisionKindSummary>): DecisionKindSummary {
  const zeros = Array(10).fill(0)
  return { calls: 0, confidence: zeros, confidence_judged: zeros, confidence_agreed: zeros, latency: Array(9).fill(0), ...partial }
}

const stats: DashboardStats = {
  decision_mode: 'shadow',
  decision_records_dropped: 40,
  decision_summary: {
    kinds: {
      failure_recovery: kind({ calls: 3, agreements: 3, shadow: 3, p50_seconds: 0.1 }),
      objective_selection: kind({
        calls: 12, shadow: 12, agreements: 6, disagreements: 2, fallbacks: 4, p50_seconds: 0.2, p95_seconds: 0.8,
        confidence_judged: [0, 0, 0, 0, 2, 0, 0, 0, 0, 6], confidence_agreed: [0, 0, 0, 0, 0, 0, 0, 0, 0, 6],
        engine_choices: { heal: 5, 'go to pewter city': 7 }
      })
    }
  },
  decision_records: [
    { kind: 'objective_selection', choice: '1', choice_label: 'heal', confidence: 0.95, shadow: true, executed: 'go to pewter city', agreed: false, duration_seconds: 0.2 },
    { kind: 'failure_recovery', choice: 'retry', choice_label: 'retry', confidence: 0.7, duration_seconds: 0.05 },
    { kind: 'objective_selection', error: 'timeout', shadow: true, executed: 'heal', duration_seconds: 9 }
  ]
}

test('summary rows rank kinds by calls and compute agreement over verdicts only', () => {
  const rows = decisionKindRows(stats)
  assert.deepEqual(rows.map((row) => row.kind), ['objective_selection', 'failure_recovery'])
  assert.equal(rows[0].label, 'Objectives')
  assert.equal(rows[0].agreement, 0.75)
  assert.equal(rows[0].judged, 8)
  assert.deepEqual(rows[0].topEngine[0], ['go to pewter city', 7])
  assert.equal(decisionKindRows({}).length, 0)
})

test('calibration reports agreement per confidence bucket and null where no verdicts', () => {
  const bins = decisionCalibration(stats.decision_summary?.kinds?.objective_selection)
  assert.equal(bins.length, 10)
  assert.equal(bins[9].rate, 1)
  assert.equal(bins[4].rate, 0)
  assert.equal(bins[0].rate, null)
})

test('live feed is newest first with a verdict per call', () => {
  const feed = decisionFeed(stats)
  assert.deepEqual(feed.map((row) => row.verdict), ['unusable', 'active', 'disagreed'])
  assert.equal(feed[2].choice, 'heal')
  assert.equal(feed[0].error, 'timeout')
  assert.ok(hasDecisionTelemetry(stats))
  assert.ok(!hasDecisionTelemetry({}))
  assert.equal(percent(null), '—')
  assert.equal(latency(0.2), '200ms')
})

test('live view streams the feed and the archive shows only the stored summary', () => {
  const live = readFileSync(new URL('../src/operator/LiveView.vue', import.meta.url), 'utf8')
  const archive = readFileSync(new URL('../src/operator/RunArchiveView.vue', import.meta.url), 'utf8')
  assert.match(live, /<DecisionTelemetry :stats="selectedRun\.stats" \/>/)
  assert.match(archive, /<DecisionTelemetry :stats="run\.stats" :show-feed="false" \/>/)
})
