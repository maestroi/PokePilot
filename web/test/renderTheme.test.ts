import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import {
  DEFAULT_RENDER_THEME_ID,
  RENDER_THEME_SCHEMA_VERSION,
  RenderThemeRegistry,
  renderThemeOptions,
  resolveRenderTheme,
  validateThemePack
} from '../src/shared/renderTheme.ts'
import { parseTileImageReference } from '../src/shared/tileAssets.ts'
import townProvenance from '../public/theme-assets/kenney-tiny-town/provenance.json' with { type: 'json' }
import dungeonProvenance from '../public/theme-assets/kenney-tiny-dungeon/provenance.json' with { type: 'json' }

function minimalTheme(overrides: Record<string, unknown> = {}) {
  return {
    schemaVersion: RENDER_THEME_SCHEMA_VERSION,
    id: 'test-theme',
    name: 'Test Theme',
    version: 1,
    tileSize: 32,
    tiles: {
      unknown: { fill: '#111111', pattern: 'unknown' },
      path: { fill: '#222222', pattern: 'path' },
      wall: { fill: '#333333', pattern: 'wall' }
    },
    ...overrides
  }
}

test('bundled theme packs are installed and independently selectable', () => {
  const options = renderThemeOptions()
  assert.equal(options.length >= 3, true)
  assert.deepEqual(
    options.map((theme) => theme.id).sort(),
    ['kenney-tiny-town', 'retro-16', 'rompilot-modern']
  )

  const modern = resolveRenderTheme('rompilot-modern').theme
  const retro = resolveRenderTheme('retro-16').theme
  assert.notEqual(modern.id, retro.id)
  assert.notEqual(modern.tiles.path.fill, retro.tiles.path.fill)
  assert.notEqual(modern.tileSize, retro.tileSize)
  const kenney = resolveRenderTheme('kenney-tiny-town').theme
  assert.ok(kenney.assets.tiles['path.center'])
})

test('Kenney atlas references stay inside their licensed bundled images', () => {
  const theme = resolveRenderTheme('kenney-tiny-town').theme
  const atlases = new Map([
    ['/theme-assets/kenney-tiny-town/tilemap.png', townProvenance],
    ['/theme-assets/kenney-tiny-dungeon/tilemap.png', dungeonProvenance]
  ])
  const dimensions = new Map<string, { width: number; height: number }>()
  for (const [url, provenance] of atlases) {
    const image = readFileSync(new URL(`../public${url}`, import.meta.url))
    const digest = createHash('sha256').update(image).digest('hex')
    assert.equal(`sha256:${digest}`, provenance.files['tilemap.png'])
    assert.equal(provenance.license, 'CC0-1.0')
    dimensions.set(url, { width: image.readUInt32BE(16), height: image.readUInt32BE(20) })
  }
  for (const reference of Object.values(theme.assets.tiles).concat(Object.values(theme.assets.objects))) {
    const tile = parseTileImageReference(reference)
    assert.ok(tile)
    const atlas = dimensions.get(tile.url)
    assert.ok(atlas)
    assert.ok(tile?.source)
    assert.ok(tile.source.x + tile.source.size <= atlas.width)
    assert.ok(tile.source.y + tile.source.size <= atlas.height)
  }
})

test('missing optional semantic assets inherit from the default theme', () => {
  const retro = resolveRenderTheme('retro-16')
  assert.deepEqual(retro.diagnostics, [])
  assert.equal(retro.theme.id, 'retro-16')

  const modern = resolveRenderTheme(DEFAULT_RENDER_THEME_ID).theme
  assert.equal(retro.theme.tiles.sign.fill, modern.tiles.sign.fill)
  assert.equal(retro.theme.tiles.warp.fill, modern.tiles.warp.fill)
  assert.equal(retro.theme.tiles.ledge.fill, modern.tiles.ledge.fill)
})

test('unknown theme selection fails safe with a useful diagnostic', () => {
  const resolved = resolveRenderTheme('does-not-exist')
  assert.equal(resolved.theme.id, DEFAULT_RENDER_THEME_ID)
  assert.match(resolved.diagnostics.join(' '), /does-not-exist/)
  assert.match(resolved.diagnostics.join(' '), /using RomPilot Modern/)
})

test('incompatible and malformed packs are rejected instead of installed', () => {
  const incompatible = validateThemePack(minimalTheme({ schemaVersion: 99 }))
  assert.equal(incompatible.ok, false)
  assert.match(incompatible.errors.join(' '), /incompatible/)

  const missingRequired = validateThemePack(minimalTheme({
    tiles: {
      unknown: { fill: '#111111' },
      path: { fill: '#222222' }
    }
  }))
  assert.equal(missingRequired.ok, false)
  assert.match(missingRequired.errors.join(' '), /tiles\.wall is required/)

  const registry = new RenderThemeRegistry()
  assert.equal(registry.install(minimalTheme(), true).installed, true)
  const bad = registry.install(minimalTheme({ id: 'Bad ID' }))
  assert.equal(bad.installed, false)
  assert.match(bad.diagnostics.join(' '), /lowercase letters/)
  assert.deepEqual(registry.list().map((theme) => theme.id), ['test-theme'])
})

test('optional omissions produce diagnostics but remain installable', () => {
  const validation = validateThemePack(minimalTheme())
  assert.equal(validation.ok, true)
  assert.match(validation.warnings.join(' '), /default theme fallback/)
})


test('battle theme tokens are validated and inherited', () => {
  const modern = resolveRenderTheme('rompilot-modern').theme
  const retro = resolveRenderTheme('retro-16').theme
  assert.ok(modern.battle.background)
  assert.ok(retro.battle.background)
  assert.notEqual(modern.battle.background, retro.battle.background)

  const invalid = validateThemePack(minimalTheme({
    battle: { background: 42 }
  }))
  assert.equal(invalid.ok, false)
  assert.match(invalid.errors.join(' '), /battle\.background/)
})
