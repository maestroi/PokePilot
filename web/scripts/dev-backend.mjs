// Read-only development backend for the operator UI.
//
// Serves real farm data to `npm run dev` without a local wall: run lists,
// run detail and debug bundles come from the operator MCP endpoint (read-only
// tools only), frames and maps from the public API. Every mutating request is
// refused, so UI work can never pause, cancel or delete a real run.
//
//   POKEPILOT_MCP_TOKEN=… node scripts/dev-backend.mjs   # then: npm run dev
//
// ponytail: endpoints without a read-only source (checkpoints, replay, media,
// models, analytics) answer with empty data; add them when a page needs them.
//
// Media has no read-only source at all (no MCP tool, nothing on the public API
// for private runs). POKEPILOT_DEV_MEDIA_FIXTURES=1 fills render jobs, replay
// availability and broadcast state with local fixtures keyed to real run IDs so
// the Media page can be checked visually; mutations stay refused either way.
import { createServer } from 'node:http'

const MEDIA_FIXTURES = process.env.POKEPILOT_DEV_MEDIA_FIXTURES === '1'
// POKEPILOT_DEV_ANALYTICS_FIXTURES=1 serves /v1/stats, /v1/experiments and /v1/models
// from local fixtures (the MCP has no read tool for them) so Analytics can be checked visually.
const ANALYTICS_FIXTURES = process.env.POKEPILOT_DEV_ANALYTICS_FIXTURES === '1'
const PORT = Number(process.env.PORT || 18080)
const MCP_URL = process.env.POKEPILOT_MCP_URL || 'https://admin.rompilot.app/mcp'
const PUBLIC_API = process.env.POKEPILOT_PUBLIC_API || 'https://api.rompilot.app'
const TOKEN = process.env.POKEPILOT_MCP_TOKEN || ''
const READ_TOOLS = new Set(['pokepilot_list_runs', 'pokepilot_get_run', 'pokepilot_get_run_debug', 'pokepilot_get_run_artifacts', 'pokepilot_get_triage'])

if (!TOKEN) {
  console.error('POKEPILOT_MCP_TOKEN is required')
  process.exit(1)
}

let rpcID = 0
async function tool(name, args) {
  if (!READ_TOOLS.has(name)) throw new Error(`refusing non-read tool ${name}`)
  const response = await fetch(MCP_URL, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${TOKEN}`,
      'Content-Type': 'application/json',
      Accept: 'application/json, text/event-stream'
    },
    body: JSON.stringify({ jsonrpc: '2.0', id: ++rpcID, method: 'tools/call', params: { name, arguments: args } })
  })
  const body = await response.json()
  if (body.error) throw new Error(body.error.message || 'MCP error')
  const text = body.result?.content?.[0]?.text ?? 'null'
  if (body.result?.isError) throw Object.assign(new Error(text), { status: 404 })
  return JSON.parse(text)
}

async function publicJSON(path) {
  const response = await fetch(PUBLIC_API + path)
  return response.ok ? response.json() : null
}

// The MCP run list omits trail/sprites; the public watch feed has them for public runs.
async function dashboard(query) {
  if (query.get('status') === 'done') return archive(query)
  const [list, watch] = await Promise.all([
    tool('pokepilot_list_runs', { limit: Number(query.get('limit') || 40), view: 'full' }),
    publicJSON('/v1/watch')
  ])
  const publicRuns = new Map((watch?.runs ?? []).map((run) => [run.run_id, run]))
  const runs = (list.runs ?? [])
    .filter((run) => run.status !== 'done')
    .map((run) => {
      const extra = publicRuns.get(run.run_id)
      return extra ? { ...run, trail: extra.trail, sprites: extra.sprites, player: { ...extra.player, ...run.player } } : run
    })
  return { now: list.now, runs, workers: list.workers ?? [] }
}

// ponytail: the MCP tool has no archive filters, so the newest 40 finished runs (the MCP wall caps responses at 2 MB)
// are filtered, sorted and paged here; enough to exercise the Runs page.
const ARCHIVE_FACETS = {
  game: ['games', (r) => r.game],
  outcome: ['outcomes', (r) => r.reason],
  how: ['hows', (r) => (r.planner === 'scripted' ? 'walk' : 'play')],
  model: ['models', (r) => r.inference?.model_id || r.stats?.model || r.llm_profile],
  deployment: ['deployments', (r) => r.llm_deployment || r.inference?.deployment_id],
  goal: ['goals', (r) => r.goal],
  starter: ['starters', (r) => r.starter],
  play_style: ['play_styles', (r) => r.play_style],
  compute: ['computes', (r) => r.inference?.compute],
  experiment: ['experiments', (r) => r.experiment_id]
}
const ARCHIVE_SORT = {
  finished: (r) => r.ended_at || 0,
  frames: (r) => r.frame || 0,
  rounds: (r) => r.stats?.rounds || r.stats?.round || 0,
  runtime: (r) => r.runtime_seconds || 0,
  planner_time: (r) => r.stats?.strategic_seconds || 0,
  strategic_calls: (r) => r.stats?.strategic_calls || r.stats?.calls || 0,
  progress: (r) => r.stats?.goal_current || r.player?.badges?.length || 0
}
async function archive(query) {
  const list = await tool('pokepilot_list_runs', { limit: 40, status: 'done', view: 'full' })
  let runs = (list.runs ?? []).filter((run) => run.status === 'done')
  const history_facets = {}
  for (const [param, [name, pick]] of Object.entries(ARCHIVE_FACETS)) {
    history_facets[name] = [...new Set(runs.map(pick).filter(Boolean).map(String))].sort()
    const wanted = query.get(param)
    if (wanted) runs = runs.filter((run) => String(pick(run) ?? '') === wanted)
  }
  const search = (query.get('search') || '').toLowerCase()
  if (search) runs = runs.filter((run) => run.run_id.toLowerCase().includes(search))
  if (query.get('success') === 'true') runs = runs.filter((run) => run.stats?.goal_complete)
  const key = ARCHIVE_SORT[query.get('sort')] || ARCHIVE_SORT.finished
  const sign = query.get('direction') === 'asc' ? 1 : -1
  runs.sort((a, b) => sign * (key(a) - key(b)))
  const offset = Number(query.get('offset') || 0)
  const limit = Number(query.get('limit') || 25)
  if (MEDIA_FIXTURES) runs = runs.map((run) => (fixtureReplayData(run.run_id) ? { ...run, replay_available: true } : run))
  return { now: list.now, runs: runs.slice(offset, offset + limit), total: runs.length, history_facets, workers: [] }
}

// ---- Media fixtures (POKEPILOT_DEV_MEDIA_FIXTURES=1) -------------------------------
const fixtureRuns = { done: [], running: [] }
async function loadFixtureRuns() {
  const [done, active] = await Promise.all([
    tool('pokepilot_list_runs', { limit: 12, status: 'done' }),
    tool('pokepilot_list_runs', { limit: 40 })
  ])
  fixtureRuns.done = (done.runs ?? []).filter((run) => run.status === 'done').map((run) => run.run_id)
  fixtureRuns.running = (active.runs ?? []).filter((run) => run.status === 'running').map((run) => run.run_id)
}
const fixtureReplayReady = (runID) => fixtureRuns.done.slice(0, 3).includes(runID)
// Replay data (the recording) without a rendered video for most of the rest.
const fixtureReplayData = (runID) => fixtureRuns.done.slice(0, 10).includes(runID)
function fixtureJobs() {
  const now = Date.now()
  const states = [
    ['ready', { segments_total: 24, segments_done: 24, result_size: 182_000_000, finished_at_unix_ms: now - 3_600_000 }],
    ['ready', { segments_total: 18, segments_done: 18, result_size: 96_500_000, finished_at_unix_ms: now - 7_200_000 }],
    ['ready', { segments_total: 30, segments_done: 30, result_size: 240_000_000, finished_at_unix_ms: now - 9_000_000 }],
    ['rendering', { stage: 'rendering', segments_total: 40, segments_done: 17, worker_id: 'render-vm-1', lease_expires_at_unix_ms: now + 60_000 }],
    ['failed', { failure_class: 'renderer_crash', retry_count: 2, last_error: 'ffmpeg exited with status 1: segment 12 frame decode failed' }],
    ['queued', {}],
    ['cancelled', {}],
    ['rendering', { mode: 'highlight', stage: 'rendering', segments_total: 6, segments_done: 2, worker_id: 'render-vm-2', lease_expires_at_unix_ms: now + 60_000 }]
  ]
  const jobs = fixtureRuns.done.slice(0, states.length).map((runID, index) => {
    const [state, extra] = states[index]
    return {
      version: 1, id: `job-${runID.slice(4, 16)}`, identity: runID, run_id: runID, mode: index % 2 ? 'semantic' : 'broadcast',
      artifact_key: `replays/${runID}/replay.mp4`, state, created_at_unix_ms: now - (index + 2) * 1_800_000,
      updated_at_unix_ms: now - index * 240_000, started_at_unix_ms: state === 'queued' ? undefined : now - (index + 1) * 1_500_000, ...extra
    }
  })
  const states_ = {}
  for (const job of jobs) states_[job.state] = (states_[job.state] || 0) + 1
  return { jobs, total: jobs.length, states: states_ }
}
function fixtureBroadcastStatus(runID) {
  if (runID !== fixtureRuns.running[0]) return { run_id: runID, state: 'stopped' }
  return {
    run_id: runID, state: 'live', provider: 'twitch', host: 'live.twitch.tv', width: 1280, height: 720, fps: 30,
    video_bitrate_kbps: 4500, reconnects: 1, started_at_unix_ms: Date.now() - 1_938_000
  }
}
function replayStatus(runID) {
  const job = MEDIA_FIXTURES ? fixtureJobs().jobs.find((item) => item.run_id === runID) : null
  if (fixtureReplayReady(runID)) return { run_id: runID, state: 'ready', job_id: job?.id }
  if (job && ['rendering', 'queued'].includes(job.state)) return { run_id: runID, state: 'generating', job_id: job.id, segments: job.segments_total, segments_done: job.segments_done }
  return { run_id: runID, state: job?.state === 'failed' ? 'error' : 'missing' }
}

function send(res, status, value) {
  res.writeHead(status, { 'Content-Type': 'application/json' })
  res.end(JSON.stringify(value))
}

async function proxyPublic(req, res) {
  const upstream = await fetch(PUBLIC_API + req.url)
  res.writeHead(upstream.status, { 'Content-Type': upstream.headers.get('content-type') || 'application/octet-stream' })
  res.end(Buffer.from(await upstream.arrayBuffer()))
}

// ---- Analytics fixtures (POKEPILOT_DEV_ANALYTICS_FIXTURES=1) ---------------------------
const fixtureStats = {
  completed_attempts: 1284, settled_runs: 1210, usable_progress_runs: 1154, goal_tracked_runs: 980, goal_wins: 412,
  at_least_one_badge: 704, best_badges: 5, retryable_failure_attempts: 23,
  terminal_reasons: [{ name: 'goal_complete', count: 412 }, { name: 'max_rounds', count: 388 }, { name: 'stuck', count: 214 }, { name: 'blackout', count: 96 }, { name: 'cancelled', count: 62 }, { name: 'error', count: 38 }],
  badge_distribution: [0, 1, 2, 3, 4, 5].map((badges, i) => ({ badges, count: [450, 402, 188, 74, 31, 9][i] })),
  endless_experiments: [
    { key: 'llm:endless:gpu:r0:f0', planner: 'llm', goal: 'Endless high score', completed_attempts: 86, usable_progress_runs: 80, at_least_one_badge: 61, goal_tracked_runs: 0, goal_wins: 0, best_badges: 5, retryable_failure_attempts: 2 },
    { key: 'llm:endless:auto:r0:f0', planner: 'llm', goal: 'Endless high score', completed_attempts: 41, usable_progress_runs: 38, at_least_one_badge: 22, goal_tracked_runs: 0, goal_wins: 0, best_badges: 3, retryable_failure_attempts: 5 }
  ],
  llm: {
    tracked_runs: 1102, calls: 184_220, rounds: 92_110, rejected: 1_904, repeats: 3_120, avg_seconds: 2.4, successful_avg_seconds: 2.3,
    rejected_avg_seconds: 4.1, strategic_avg_seconds: 11.8, prompt_tokens: 412_000_000, completion_tokens: 38_500_000, transport: 41,
    fallbacks: 118, failovers: 7, strategic_calls: 21_400, fast_calls: 162_820,
    profiles: [{ profile: 'gpu', runs: 702, calls: 121_400, avg_seconds: 1.9, rejected: 980 }, { profile: 'auto', runs: 400, calls: 62_820, avg_seconds: 3.4, rejected: 924 }],
    latest_models: [{ name: 'qwen3-27b-instruct-q5_k_m', backend: 'llama.cpp', runs: 702 }, { name: 'qwen3-4b-instruct', backend: 'vllm', runs: 400 }]
  }
}
const fixtureArm = (done, wins) => ({ runs: 20, done, goal_successes: wins, boulder_successes: wins, success_rate: done ? wins / done : 0, median_rounds_to_goal: 412, median_frames_to_goal: 1_204_330, avg_run_seconds: 2410, strategic_calls: 640, strategic_rejected: 12, plan_steps_produced: 3100, plan_executions: 2800, steps_skipped: 140, plan_execution_fraction: 0.9, avg_strategic_call_seconds: 9.2, p50_strategic_call_seconds: 8.1, p95_strategic_call_seconds: 21.4, strategic_seconds: 5890, avg_prefill_tps: 2210, avg_decode_tps: 41.3, prompt_tokens: 9_100_000, completion_tokens: 810_000, rejected: 14, transport_errors: 1, fallbacks: 3, blackouts: 4, objective_failures: 2 })
const fixtureExperiments = [
  { id: 'exp-20261016-brock-27b-4b', name: 'Brock · 27B vs 4B', created_at: '2026-10-16T09:12:00Z', total_pairs: 20,
    request: { name: 'Brock · 27B vs 4B', game: 'pokemon-red', goal: 'Earn the Boulder Badge.', play_style: 'adventure', risk_tolerance: 'balanced', wild_encounters: 'planner', recovery_profile: 'resilient', reasoning_effort: 'medium', arm_a: { name: 'A', deployment: 'qwen3-27b', max_parallel_workers: 1 }, arm_b: { name: 'B', deployment: 'qwen3-4b', max_parallel_workers: 1 } },
    arm_a: fixtureArm(14, 11), arm_b: fixtureArm(13, 6), paired: { a_wins: 7, ties: 4, b_wins: 1, comparable_pairs: 12, completed_pairs: 12, excluded_pairs: 1 },
    identity: { game: 'pokemon-red', git_revision: '5cac25f9a1b2c3d4e5f6', rom_identity: 'sha1:ea9bcae617fdf159b045185467ae58b2e4a48b9a', prompt_identity: 'sha256:9f2c1a7d', arm_a: { model_id: 'qwen3-27b', revision: 'q5_k_m', compute: 'gpu' }, arm_b: { model_id: 'qwen3-4b', revision: 'bf16', compute: 'auto' } },
    pairs: Array.from({ length: 20 }, (_, i) => ({ seed: 100 + i, run_id_a: `run-exp-a-${i}`, run_id_b: `run-exp-b-${i}`, comparable: i !== 7, non_comparable_reason: i === 7 ? 'ROM identity differs' : undefined, status_a: i < 13 ? 'done' : 'running', status_b: i < 13 ? 'done' : 'queued', winner: i < 13 && i !== 7 ? ['a', 'a', 'tie', 'b'][i % 4] : undefined })) },
  { id: 'exp-20261012-route3-27b-12b', name: 'Route 3 · 27B vs 12B', created_at: '2026-10-12T18:40:00Z', total_pairs: 10,
    request: { name: 'Route 3 · 27B vs 12B', game: 'pokemon-red', goal: 'Earn 2 badges.', play_style: 'speedrun', recovery_profile: 'strict', arm_a: { name: 'A', deployment: 'qwen3-27b' }, arm_b: { name: 'B', deployment: 'gemma3-12b' } },
    arm_a: fixtureArm(10, 6), arm_b: fixtureArm(10, 5), paired: { a_wins: 4, ties: 3, b_wins: 3, comparable_pairs: 10, completed_pairs: 10, excluded_pairs: 0 },
    pairs: Array.from({ length: 10 }, (_, i) => ({ seed: i + 1, comparable: true, status_a: 'done', status_b: 'done', winner: ['a', 'tie', 'b'][i % 3] })) }
]
const fixtureModels = { deployments: [
  { id: 'qwen3-27b', label: 'Qwen3 27B', role: 'strategist', enabled: true, state: 'ready', max_parallel_workers: 2 },
  { id: 'qwen3-4b', label: 'Qwen3 4B', role: 'strategist', enabled: true, state: 'ready', max_parallel_workers: 4 }
] }

const routes = [
  ...(ANALYTICS_FIXTURES ? [
    [/^\/v1\/stats$/, () => fixtureStats],
    [/^\/v1\/experiments$/, () => ({ experiments: fixtureExperiments })],
    [/^\/v1\/experiments\/([^/]+)$/, (q, id) => fixtureExperiments.find((item) => item.id === id) ?? Promise.reject(Object.assign(new Error('experiment not found'), { status: 404 }))],
    [/^\/v1\/models$/, () => fixtureModels]
  ] : []),
  [/^\/v1\/dashboard$/, (q) => dashboard(q)],
  [/^\/v1\/runs\/([^/]+)$/, (q, id) => tool('pokepilot_get_run', { run_id: id, view: 'full' })],
  [/^\/v1\/runs\/([^/]+)\/debug$/, (q, id) => tool('pokepilot_get_run_debug', { run_id: id })],
  [/^\/v1\/runs\/([^/]+)\/artifacts$/, (q, id) => tool('pokepilot_get_run_artifacts', { run_id: id })],
  [/^\/v1\/runs\/[^/]+\/checkpoints$/, () => []],
  [/^\/v1\/media\/render-jobs$/, () => (MEDIA_FIXTURES ? fixtureJobs() : { jobs: [], total: 0, states: {} })],
  [/^\/v1\/live\/broadcast\/config$/, () => ({
    destinations: MEDIA_FIXTURES
      ? [{ provider: 'twitch', label: 'Twitch', configured: true, host: 'live.twitch.tv' }, { provider: 'youtube', label: 'YouTube Live', configured: false, reason: 'No stream key configured on the replay host' }]
      : [],
    restart_behavior: 'manual_restart_required'
  })],
  [/^\/v1\/runs\/([^/]+)\/live\/broadcast\/status$/, (q, id) => (MEDIA_FIXTURES ? fixtureBroadcastStatus(id) : { run_id: id, state: 'stopped' })],
  // The wall's /v1/triage includes resolved groups; the MCP hides them unless asked.
  // If the full history ever trips the MCP's 2 MB response cap, fall back to open groups.
  [/^\/v1\/triage$/, () => tool('pokepilot_get_triage', { include_resolved: true, view: 'full' })
    .catch(() => tool('pokepilot_get_triage', { view: 'full' }))
    .then((value) => value?.groups ?? [])],
  [/^\/v1\/runs\/([^/]+)\/replay\/status$/, (q, id) => replayStatus(id)],
  [/^\/v1\/runs\/([^/]+)\/highlights\/status$/, (q, id) => {
    if (!MEDIA_FIXTURES) return { state: 'missing' }
    const index = fixtureRuns.done.indexOf(id)
    if (index === 0) return { run_id: id, state: 'ready', windows: 4, duration_ms: 96_000 }
    if (index === 1) return { run_id: id, state: 'empty' }
    return { run_id: id, state: 'missing' }
  }],
  [/^\/v1\/runs\/([^/]+)\/studio\/packages$/, (q, id) => ({
    packages: MEDIA_FIXTURES && id === fixtureRuns.done[0]
      ? [{ hash: 'a'.repeat(64), created_at_unix_ms: Date.now() - 5_400_000, thumbnails: true, thumbnail_selected: true }]
      : []
  })],
  [/^\/v1\/programming$/, () => publicJSON('/v1/watch/programming')],
  [/^\/v1\/spectator\/control$/, () => (MEDIA_FIXTURES
    ? { featured_run_id: fixtureRuns.running[0], runs: Object.fromEntries(fixtureRuns.done.slice(3, 6).map((id) => [id, { visible: false }])) }
    : { runs: {} })],
  [/^\/v1\/(build|ui-config)$/, () => ({})]
]

// DNS-rebinding guard: a hostile page can resolve its own domain to 127.0.0.1,
// but its requests still carry that domain in Host. Only loopback names pass
// (the Vite proxy forwards the browser's localhost Host unchanged).
const LOOPBACK_HOST = /^(localhost|127\.0\.0\.1|\[::1\])(:\d+)?$/i

createServer(async (req, res) => {
  if (!LOOPBACK_HOST.test(req.headers.host || '')) return send(res, 403, { error: 'Forbidden host' })
  const url = new URL(req.url, 'http://localhost')
  try {
    if (req.method !== 'GET') return send(res, 403, { error: 'The dev backend is read-only; this action is disabled.' })
    if (url.pathname === '/frame' || url.pathname.startsWith('/maps/')) return await proxyPublic(req, res)
    for (const [pattern, handler] of routes) {
      const match = url.pathname.match(pattern)
      if (match) return send(res, 200, await handler(url.searchParams, ...match.slice(1).map(decodeURIComponent)))
    }
    send(res, 404, { error: `${url.pathname} is not available from the dev backend` })
  } catch (cause) {
    send(res, cause.status || 502, { error: cause.message })
  }
}).listen(PORT, '127.0.0.1', () => {
  console.log(`dev backend on http://127.0.0.1:${PORT}${MEDIA_FIXTURES ? ' (media fixtures)' : ''}`)
  if (MEDIA_FIXTURES) loadFixtureRuns().catch((cause) => console.error('media fixtures:', cause.message))
})
