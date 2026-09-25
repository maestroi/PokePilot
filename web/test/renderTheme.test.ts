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
import pokegoldProvenance from '../public/theme-assets/pokegold-gen2/provenance.json' with { type: 'json' }

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
  assert.equal(options.length >= 2, true)
  assert.deepEqual(
    options.map((theme) => theme.id).sort(),
    ['kenney-tiny-town', 'pokegold-gen2']
  )

  const gen2 = resolveRenderTheme('pokegold-gen2').theme
  const kenney = resolveRenderTheme('kenney-tiny-town').theme
  assert.equal(gen2.id, DEFAULT_RENDER_THEME_ID)
  assert.ok(gen2.assets.tiles.grass.includes('/theme-assets/pokegold-gen2/kanto.png'))
  assert.ok(gen2.assets.characters.player.includes('/theme-assets/pokegold-gen2/sprites/red.png'))
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

test('secondary themes inherit omitted presentation assets from the Gen-II default', () => {
  const kenney = resolveRenderTheme('kenney-tiny-town')
  assert.deepEqual(kenney.diagnostics, [])
  const gen2 = resolveRenderTheme(DEFAULT_RENDER_THEME_ID).theme
  assert.equal(kenney.theme.assets.characters.player, gen2.assets.characters.player)
  assert.equal(kenney.theme.assets.characters.npc, gen2.assets.characters.npc)
})

test('unknown theme selection fails safe with a useful diagnostic', () => {
  const resolved = resolveRenderTheme('does-not-exist')
  assert.equal(resolved.theme.id, DEFAULT_RENDER_THEME_ID)
  assert.match(resolved.diagnostics.join(' '), /does-not-exist/)
  assert.match(resolved.diagnostics.join(' '), /using Gold \/ Silver/)
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
  const gen2 = resolveRenderTheme('pokegold-gen2').theme
  const kenney = resolveRenderTheme('kenney-tiny-town').theme
  assert.ok(gen2.battle.background)
  assert.ok(kenney.battle.background)
  assert.notEqual(gen2.battle.background, kenney.battle.background)

  const invalid = validateThemePack(minimalTheme({
    battle: { background: 42 }
  }))
  assert.equal(invalid.ok, false)
  assert.match(invalid.errors.join(' '), /battle\.background/)
})


test('Gold/Silver assets keep explicit upstream provenance without inventing a license', () => {
  const theme = resolveRenderTheme('pokegold-gen2').theme
  const atlas = parseTileImageReference(theme.assets.tiles.grass)
  assert.ok(atlas)
  assert.equal(atlas?.source?.size, 8)
  assert.equal(atlas?.repeat, 2)
  assert.match(atlas?.url || '', /palette=bg-green/)
  assert.equal(pokegoldProvenance.sourceRef, '0f087a51e36cbd38f33e5055754614578246ceff')
  assert.equal(pokegoldProvenance.license, 'NOASSERTION')
  assert.equal(pokegoldProvenance.files['kanto.png'].upstreamBlob, 'a3036406eb796220493ba42e0ae7b6a0d45548e0')
})
