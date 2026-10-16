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
    tool('pokepilot_list_runs', { limit: Number(query.get('limit') || 40) }),
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
  const list = await tool('pokepilot_list_runs', { limit: 40, status: 'done' })
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
  if (MEDIA_FIXTURES) runs = runs.map((run) => (fixtureReplayReady(run.run_id) ? { ...run, replay_available: true } : run))
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
function fixtureJobs() {
  const now = Date.now()
  const states = [
    ['ready', { segments_total: 24, segments_done: 24, result_size: 182_000_000, finished_at_unix_ms: now - 3_600_000 }],
    ['ready', { segments_total: 18, segments_done: 18, result_size: 96_500_000, finished_at_unix_ms: now - 7_200_000 }],
    ['ready', { segments_total: 30, segments_done: 30, result_size: 240_000_000, finished_at_unix_ms: now - 9_000_000 }],
    ['rendering', { stage: 'rendering', segments_total: 40, segments_done: 17, worker_id: 'render-vm-1', lease_expires_at_unix_ms: now + 60_000 }],
    ['failed', { failure_class: 'renderer_crash', retry_count: 2, last_error: 'ffmpeg exited with status 1: segment 12 frame decode failed' }],
    ['queued', {}],
    ['cancelled', {}]
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

const routes = [
  [/^\/v1\/dashboard$/, (q) => dashboard(q)],
  [/^\/v1\/runs\/([^/]+)$/, (q, id) => tool('pokepilot_get_run', { run_id: id })],
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
  [/^\/v1\/triage$/, () => tool('pokepilot_get_triage', { include_resolved: true })
    .catch(() => tool('pokepilot_get_triage', {}))
    .then((value) => value?.groups ?? [])],
  [/^\/v1\/runs\/([^/]+)\/replay\/status$/, (q, id) => replayStatus(id)],
  [/^\/v1\/runs\/[^/]+\/highlights\/status$/, () => ({ state: 'missing' })],
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
