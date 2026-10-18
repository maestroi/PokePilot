import assert from 'node:assert/strict'
import test from 'node:test'
import { isVerifiedChampion, runFullE2E, type FullE2EAPI, type FullE2EOutput, type FullE2EStage } from '../src/operator/fullProduction.ts'
import type { DashboardRun, StudioPublicationCandidate } from '../src/shared/api/types'

const A = 'a'.repeat(64)
const B = 'b'.repeat(64)
const C = 'c'.repeat(64)
const D = 'd'.repeat(64)
const E = 'e'.repeat(64)
const F = 'f'.repeat(64)
const EP = '1'.repeat(64)
const SHORT = '2'.repeat(64)

function champ(extra: Partial<DashboardRun> = {}): DashboardRun {
  return { run_id: 'e2e-test', status: 'done', reason: 'done', game: 'pokemon-red',
    goal: 'elite-four', stats: { goal_complete: true }, ...extra }
}

function mock(verdict = 'full_episode') {
  const calls: string[] = []
  const candidates = new Map<string, StudioPublicationCandidate>()
  const api: FullE2EAPI = {
    async createRun(spec) { calls.push('launch:' + spec.goal); return { status: 'queued' } },
    async getRun() { calls.push('get-run'); return champ() },
    async renderBroadcast() { calls.push('broadcast-render'); return { state: 'generating' } },
    async broadcastStatus() { calls.push('broadcast-status'); return { state: 'ready', size: 123 } },
    async assess() { calls.push('assess'); return { assessment: {
      hash: A, run_id: 'e2e-test', beats: [], publication_class: verdict as 'full_episode',
      publication_reason: ''
    } } },
    async createPlan(_, __, fmt) { calls.push('edit-plan:' + fmt); return {
      plan: { hash: fmt === 'episode' ? B : C, profile: fmt }
    } },
    async renderEdit() { calls.push('edit-render'); return { state: 'ready', plan_hash: B } },
    async editStatus() { calls.push('edit-status'); return { state: 'ready', plan_hash: B } },
    async createPackage(_, __, planHash) { const isEpisode = planHash === B; calls.push('package:' + (isEpisode ? 'episode' : 'short')); return {
      package: { hash: isEpisode ? D : E, run_id: 'e2e-test', assessment_hash: A,
        titles: [{ id: 'title-1', title: 'Episode title', angle: '' }],
        shorts: [{ id: 'short-1', title: 'Short title', hook: '' }],
        thumbnail_concepts: [] }
    } },
    async renderThumbnails() { calls.push('thumbnail'); return { manifest: {
      version: 1, run_id: 'e2e-test', package_hash: D, hash: F, template: 'v1',
      candidates: [{ id: F, concept_id: 'title-1', variant: 1, bytes: 22, sha256: F,
        artwork_status: 'not_requested', spec: { hash: F, template: 'v1', renderer: 'v1', provider: 'none' } }]
    } } },
    async selectThumbnail() { calls.push('thumbnail-select'); return {} },
    async createCandidate(_, p) {
      calls.push('queue:' + p.format + ':' + (p.privacy || 'none'))
      const id = p.format === 'episode' ? EP : p.format === 'short' ? SHORT : F
      const c = { version: 1, id, state: p.format === 'archive' ? 'skipped' : 'queued',
        approved: false, attempts: 0, proposal: {
          run_id: 'e2e-test', assessment_hash: A, format: p.format,
          privacy: p.privacy || 'private', title_id: p.title_id, thumbnail_id: p.thumbnail_id,
          package_hash: p.package_hash
        }
      } as StudioPublicationCandidate
      candidates.set(id, c)
      return c
    },
    async publicationAction(_, id, action) {
      calls.push('action:' + action)
      const c = candidates.get(id)!
      if (action === 'approve') { c.state = 'approved'; c.approved = true }
      else {
        c.state = 'published'
        c.provider_id = id === EP ? 'ep-video' : 'short-video'
        c.provider_url = 'https://youtube.example/video'
      }
      return c
    },
    async publications() { calls.push('publication-status'); return { candidates: [...candidates.values()] } },
    async youtubeStatus() { calls.push('youtube-status'); return {
      configured: true, publication: { run_id: 'e2e-test', state: 'none' }
    } }
  }
  return { api, calls }
}

async function exercise(uploadPrivate: boolean, verdict = 'full_episode') {
  const { api, calls } = mock(verdict)
  const stages: FullE2EStage[] = []
  const outputs: FullE2EOutput[] = []
  const options = { runID: 'e2e-test', fresh: true, starter: 'squirtle',
    seed: 1, deployment: 'model-x', uploadPrivate, signal: new AbortController().signal,
    pollMS: 1, timeoutMS: 1000, onStage: (s: FullE2EStage) => stages.push(s),
    onOutput: (o: FullE2EOutput) => outputs.push(o) }
  return { api, calls, stages, outputs, options }
}

test('eight badges are not enough: Red Champion needs semantic completion', () => {
  assert.equal(isVerifiedChampion(champ()), true)
  assert.equal(isVerifiedChampion(champ({ stats: { goal_complete: false },
    player: { badges: Array(8).fill('Badge'), party: [] } })), false)
  assert.equal(isVerifiedChampion(champ({ reason: 'budget' })), false)
  assert.equal(isVerifiedChampion(champ({ game: 'pokemon-gold' })), false)
  assert.equal(isVerifiedChampion(champ({ stats: {}, player: {
    milestones: ['hall_of_fame'], party: []
  } })), true)
})

test('UI full E2E prepares two private candidates without uploading by default', async () => {
  const t = await exercise(false)
  const result = await runFullE2E(t.api, t.options)
  assert.deepEqual(result.map(o => o.format), ['episode', 'short'])
  assert.deepEqual(result.map(o => o.privacy), ['private', 'private'])
  assert.equal(t.calls.filter(c => c.startsWith('action:')).length, 0)
  assert.equal(t.calls.filter(c => c.startsWith('queue:')).length, 2)
  assert.ok(t.calls.includes('launch:elite-four'))
  assert.ok(t.stages.some(s => s.id === 'producer' && s.state === 'ready'))
})

test('explicit private upload waits for two separate recorded video IDs', async () => {
  const t = await exercise(true)
  const result = await runFullE2E(t.api, t.options)
  assert.deepEqual(result.map(o => o.videoID), ['ep-video', 'short-video'])
  assert.equal(t.calls.filter(c => c === 'action:approve').length, 2)
  assert.equal(t.calls.filter(c => c === 'action:publish').length, 2)
  assert.ok(t.calls.indexOf('queue:short:private') < t.calls.indexOf('action:publish'))
})

test('producer archive gates all video and upload work', async () => {
  const t = await exercise(true, 'archive_only')
  await assert.rejects(runFullE2E(t.api, t.options), /No upload is allowed/)
  assert.equal(t.calls.filter(c => c.startsWith('action:')).length, 0)
  assert.ok(t.calls.includes('queue:archive:none'))
  assert.ok(t.stages.some(s => s.state === 'failed'))
})

test('browser cancellation does not create uploads', async () => {
  const t = await exercise(true)
  const controller = new AbortController()
  controller.abort()
  await assert.rejects(runFullE2E(t.api, { ...t.options, signal: controller.signal }), /Stopped coordinating/)
  assert.equal(t.calls.length, 0)
})
