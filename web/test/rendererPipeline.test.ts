import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const spectator = readFileSync(new URL('../src/spectator/App.vue', import.meta.url), 'utf8')
const operator = readFileSync(new URL('../src/operator/LiveView.vue', import.meta.url), 'utf8')

test('public spectator and operator live view share the semantic renderer pipeline', () => {
  for (const [name, source] of [['spectator', spectator], ['operator', operator]] as const) {
    assert.match(source, /ModernSceneRenderer/, name)
    assert.match(source, /useRenderStatePump/, name)
    assert.match(source, /canRenderModernScene/, name)
    assert.match(source, /resolveRenderTheme/, name)
    assert.match(source, /<ModernSceneRenderer/, name)
  }
})

test('operator live view retains explicit classic fallback and viewer-local theme choice', () => {
  assert.match(operator, /useFramePump/)
  assert.match(operator, /setRendererMode\('classic'\)/)
  assert.match(operator, /setRendererMode\('modern'\)/)
  assert.match(operator, /pokepilot\.operator\.theme/)
  assert.match(operator, /classic fallback/)
})
