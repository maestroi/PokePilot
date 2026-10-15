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
// models, triage, analytics) answer with empty data; add them when a page needs them.
import { createServer } from 'node:http'

const PORT = Number(process.env.PORT || 18080)
const MCP_URL = process.env.POKEPILOT_MCP_URL || 'https://admin.rompilot.app/mcp'
const PUBLIC_API = process.env.POKEPILOT_PUBLIC_API || 'https://api.rompilot.app'
const TOKEN = process.env.POKEPILOT_MCP_TOKEN || ''
const READ_TOOLS = new Set(['pokepilot_list_runs', 'pokepilot_get_run', 'pokepilot_get_run_debug', 'pokepilot_get_run_artifacts'])

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
  const done = query.get('status') === 'done'
  const [list, watch] = await Promise.all([
    tool('pokepilot_list_runs', { limit: Number(query.get('limit') || 40), ...(done ? { status: 'done' } : {}) }),
    done ? null : publicJSON('/v1/watch')
  ])
  const publicRuns = new Map((watch?.runs ?? []).map((run) => [run.run_id, run]))
  const runs = (list.runs ?? [])
    .filter((run) => done ? run.status === 'done' : run.status !== 'done')
    .map((run) => {
      const extra = publicRuns.get(run.run_id)
      return extra ? { ...run, trail: extra.trail, sprites: extra.sprites, player: { ...extra.player, ...run.player } } : run
    })
  return { now: list.now, runs, workers: list.workers ?? [] }
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
  [/^\/v1\/runs\/[^/]+\/replay\/status$/, () => ({ state: 'missing' })],
  [/^\/v1\/runs\/[^/]+\/highlights\/status$/, () => ({ state: 'missing' })],
  [/^\/v1\/programming$/, () => publicJSON('/v1/watch/programming')],
  [/^\/v1\/spectator\/control$/, () => ({ runs: {} })],
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
}).listen(PORT, '127.0.0.1', () => console.log(`dev backend on http://127.0.0.1:${PORT}`))
