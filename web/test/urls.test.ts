import assert from 'node:assert/strict'
import test from 'node:test'
import {
  LEGACY_PUBLIC_HOST,
  PRODUCTION_ADMIN_ORIGIN,
  PRODUCTION_API_ORIGIN,
  PRODUCTION_PUBLIC_ORIGIN,
  liveHTTPURL,
  localSpectatorBase,
  publicBaseURL,
  resolveSpectatorBase,
  runIDFromLocation,
  spectatorRunPath,
  spectatorURL,
  websocketURL
} from '../src/shared/urls.ts'

test('production spectator URLs use rompilot.app', () => {
  const config = {
    public_base_url: PRODUCTION_PUBLIC_ORIGIN,
    spectator_url: `https://${LEGACY_PUBLIC_HOST}`,
    admin_base_url: PRODUCTION_ADMIN_ORIGIN,
    api_base_url: PRODUCTION_API_ORIGIN
  }
  assert.equal(publicBaseURL(config), PRODUCTION_PUBLIC_ORIGIN)
  assert.equal(spectatorURL(config.public_base_url, 'run-123'), 'https://rompilot.app/runs/run-123')
  assert.doesNotMatch(spectatorURL(config.public_base_url, 'run-123'), /maestroi\.cc/)
  assert.equal(PRODUCTION_ADMIN_ORIGIN, 'https://admin.rompilot.app')
})

test('local development keeps localhost ports', () => {
  assert.equal(resolveSpectatorBase({}, 'http://127.0.0.1:18080/#live'), 'http://127.0.0.1:18081')
  assert.equal(resolveSpectatorBase({}, 'https://admin.rompilot.app/'), '')
  assert.equal(spectatorRunPath('run-9'), '/runs/run-9')
  assert.equal(runIDFromLocation('/runs/run-9'), 'run-9')
  assert.equal(runIDFromLocation('/', '?run=legacy'), 'legacy')
})

test('live websocket URLs follow the http scheme', () => {
  assert.equal(
    websocketURL(PRODUCTION_API_ORIGIN, '/v1/watch/live'),
    'wss://api.rompilot.app/v1/watch/live'
  )
  assert.equal(
    websocketURL('http://127.0.0.1:18081', '/v1/watch/live'),
    'ws://127.0.0.1:18081/v1/watch/live'
  )
  assert.equal(liveHTTPURL('', '/v1/watch'), '/v1/watch')
  assert.match(websocketURL(PRODUCTION_PUBLIC_ORIGIN, '/v1/watch'), /^wss:\/\//)
})
