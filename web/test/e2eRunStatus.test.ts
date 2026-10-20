import assert from 'node:assert/strict'
import test from 'node:test'
import { isFullCampaignSource, isVerifiedChampion, observeE2E } from '../src/operator/e2eRunStatus.ts'
import type { DashboardRun, StudioPublicationCandidate } from '../src/shared/api/types'

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



test('cross-browser source discovery does not falsely classify regular games as E2E', () => {
  assert.equal(isFullCampaignSource(champion()), true)
  assert.equal(isFullCampaignSource(champion({ goal: 'Earn all 8 badges.' })), false)
  assert.equal(isFullCampaignSource(champion({ game: 'pokemon-gold' })), false)
  assert.equal(isFullCampaignSource(champion({ run_id: 'e2e-old-123', goal: '' })), true)
})

test('observed status comes only from server records', () => {
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

test('explicit database purpose identifies E2E independent of the goal or ID', () => {
  assert.equal(isFullCampaignSource(champion({ purpose: 'full_e2e', goal: 'other' })), true)
  assert.equal(isFullCampaignSource(champion({ purpose: 'normal', goal: 'Earn 1 badge.' })), false)
})

test('eight badges are not enough: Red Champion needs semantic completion', () => {
  assert.equal(isVerifiedChampion(champion()), true)
  assert.equal(isVerifiedChampion(champion({ stats: { goal_complete: false },
    player: { badges: Array(8).fill('Badge'), party: [] } })), false)
  assert.equal(isVerifiedChampion(champion({ reason: 'budget' })), false)
  assert.equal(isVerifiedChampion(champion({ game: 'pokemon-gold' })), false)
  assert.equal(isVerifiedChampion(champion({ stats: {}, player: {
    milestones: ['hall_of_fame'], party: []
  } })), true)
})
