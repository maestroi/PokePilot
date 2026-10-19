import assert from 'node:assert/strict'
import test from 'node:test'
import { E2E_HISTORY_KEY, isFullCampaignSource, parseE2EHistory, storeE2EEntry, type E2EHistoryEntry } from '../src/operator/e2eHistory.ts'
import { historyLabel, observeE2E } from '../src/operator/e2eRunStatus.ts'
import type { DashboardRun, StudioPublicationCandidate } from '../src/shared/api/types'

const now = 123_456_000
const receipt: E2EHistoryEntry = {
  runID: 'run-red-123',
  state: 'working',
  startedAt: now - 9000,
  updatedAt: now - 1000,
  uploadRequested: true,
  stages: [{ id: 'game', label: 'Play game', state: 'ready' }],
  outputs: [],
  failure: ''
}
const champion = (extra: Partial<DashboardRun> = {}): DashboardRun => ({
  run_id: 'run-red-123', game: 'pokemon-red', goal: 'Beat the Elite Four and Champion.',
  status: 'done', reason: 'done', stats: { goal_complete: true }, ...extra
})
const publication = (format: 'episode' | 'short', state: StudioPublicationCandidate['state'], privateIntent = true): StudioPublicationCandidate => ({
  version: 1, id: 'a'.repeat(64), approved: true, attempts: 1, state,
  provider_id: state === 'published' ? 'youtube-video' : undefined,
  proposal: { run_id: 'run-red-123', assessment_hash: 'b'.repeat(64), format,
    privacy: privateIntent ? 'private' : 'public' }
})

test('saved browser E2E receipts survive reload but cannot impersonate a live coordinator', () => {
  assert.ok(E2E_HISTORY_KEY.includes('full-e2e'))
  const restored = parseE2EHistory(JSON.stringify([receipt]), now)
  assert.equal(restored.length, 1)
  assert.equal(restored[0].runID, receipt.runID)
  assert.equal(restored[0].state, 'interrupted')
  assert.equal(restored[0].uploadRequested, true, 'requested privacy is receipt-only, not authorization')
  assert.match(historyLabel(restored[0]), /interrupted/i)
})

test('invalid and duplicate history records are ignored, newest wins and storage is bounded', () => {
  const corrupted = parseE2EHistory('}{', now)
  assert.deepEqual(corrupted, [])
  const incoming = [
    { ...receipt, runID: '../path' },
    { ...receipt, state: 'not_a_status' },
    receipt,
    { ...receipt, updatedAt: now - 4000 },
    { ...receipt, runID: 'run-red-124', state: 'passed' }
  ]
  const parsed = parseE2EHistory(JSON.stringify(incoming), now)
  assert.deepEqual(parsed.map(i => i.runID), ['run-red-123', 'run-red-124'])
  assert.equal(parsed[1].state, 'passed')
  const updated = storeE2EEntry(parsed, { ...receipt, state: 'failed', updatedAt: now })
  assert.equal(updated.length, 2)
  assert.equal(updated[0].state, 'failed')
})

test('cross-browser source discovery does not falsely classify regular games as E2E', () => {
  assert.equal(isFullCampaignSource(champion()), true)
  assert.equal(isFullCampaignSource(champion({ goal: 'Earn all 8 badges.' })), false)
  assert.equal(isFullCampaignSource(champion({ game: 'pokemon-gold' })), false)
  assert.equal(isFullCampaignSource(champion({ run_id: 'e2e-old-123', goal: '' })), true)
})

test('server status is not overwritten by a stale browser receipt', () => {
  assert.equal(observeE2E(champion({ status: 'running' }), null, null).tone, 'working')
  assert.equal(observeE2E(champion({ reason: 'timeout', stats: {} }), null, null).tone, 'failed')
  assert.match(observeE2E(champion(), { state: 'ready', size: 123 }, []).label, /production incomplete/)
  assert.equal(observeE2E(champion(), null, [publication('episode', 'queued'), publication('short', 'queued')]).tone, 'ready')
  const twoUploads = observeE2E(champion(), null, [publication('episode', 'published'), publication('short', 'published')])
  assert.match(twoUploads.label, /upload receipts/)
  assert.match(twoUploads.detail, /not independently verified/)
  assert.equal(observeE2E(champion(), null, [publication('episode', 'published'), publication('short', 'published', false)]).label.includes('upload receipts'), false)
  assert.equal(observeE2E(champion(), null, [publication('episode', 'blocked')]).tone, 'failed')
})
