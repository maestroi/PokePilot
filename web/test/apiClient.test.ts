import assert from 'node:assert/strict'
import test from 'node:test'
import { ApiError, createRun, getBuildProvenance, getDashboard } from '../src/shared/api/client.ts'

const originalFetch = globalThis.fetch

test.afterEach(() => {
  globalThis.fetch = originalFetch
})

test('build provenance uses the dedicated no-store endpoint', async () => {
  globalThis.fetch = async (input, init) => {
    assert.equal(String(input), '/v1/build')
    assert.equal(init?.cache, 'no-store')
    return new Response(JSON.stringify({
      version: '0123456789abcdef',
      pr_number: '270',
      pr_url: 'https://github.com/maestroi/PokePilot/pull/270',
      commit_url: 'https://github.com/maestroi/PokePilot/commit/0123456789abcdef'
    }), { status: 200, headers: { 'Content-Type': 'application/json' } })
  }

  const build = await getBuildProvenance()
  assert.equal(build.pr_number, '270')
  assert.match(build.commit_url || '', /0123456/)
})

test('dashboard query stays server-side and skips empty filters', async () => {
  globalThis.fetch = async (input) => {
    const url = String(input)
    assert.match(url, /^\/v1\/dashboard\?/)
    assert.match(url, /status=done/)
    assert.match(url, /limit=25/)
    assert.doesNotMatch(url, /starter=/)
    return new Response(JSON.stringify({ now: 1, runs: [], workers: [], total: 0 }), { status: 200 })
  }

  const snapshot = await getDashboard({ status: 'done', limit: 25, starter: '' })
  assert.equal(snapshot.total, 0)
})

test('createRun generates a run id and surfaces API errors', async () => {
  const spec = {
    run_id: '', seed: 0, planner: 'llm', starter: '', dest: '', goal: 'Earn the Boulder Badge.',
    llm_profile: 'auto', play_style: 'progression', risk_tolerance: 'normal', wild_encounters: 'normal',
    reasoning_effort: '', fps: 60, max_rounds: 0, max_frames: 0, endless: false, random_seed: false
  }
  globalThis.fetch = async (_input, init) => {
    const body = JSON.parse(String(init?.body || '{}'))
    assert.match(body.run_id, /^run-/)
    return new Response(JSON.stringify({ error: 'queue unavailable' }), {
      status: 503,
      statusText: 'Service Unavailable',
      headers: { 'Content-Type': 'application/json' }
    })
  }

  await assert.rejects(() => createRun(spec), (error) => {
    assert.ok(error instanceof ApiError)
    assert.equal(error.status, 503)
    assert.equal(error.message, 'queue unavailable')
    return true
  })
  assert.match(spec.run_id, /^run-/)
})
