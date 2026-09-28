import assert from 'node:assert/strict'
import test from 'node:test'

import {
  MAX_THEME_ASSET_BYTES,
  RENDER_THEME_SCHEMA_VERSION,
  validateThemeAssetFile,
  validateThemeAssetReference,
  validateThemePack
} from '../src/shared/renderTheme.ts'

function themeWithAsset(reference: string) {
  return {
    schemaVersion: RENDER_THEME_SCHEMA_VERSION,
    id: 'community-test',
    name: 'Community Test',
    version: 1,
    tileSize: 32,
    tiles: {
      unknown: { fill: '#111111' },
      path: { fill: '#222222' },
      wall: { fill: '#333333' }
    },
    assets: {
      tiles: { path: reference }
    }
  }
}

test('theme asset references are local image paths with no traversal', () => {
  assert.equal(
    validateThemeAssetReference('/theme-assets/community-test/tiles/world.png?palette=green#tile=0,0,16'),
    null
  )
  for (const reference of [
    'https://example.com/theme.png',
    '//example.com/theme.png',
    '/theme-assets/community-test/../secret.png',
    '/theme-assets/community-test/%2e%2e/secret.png',
    '/theme-assets/community-test/tiles\\secret.png',
    '/theme-assets/community-test/tiles/unsafe.svg',
    'data:image/png;base64,AAAA'
  ]) {
    assert.ok(validateThemeAssetReference(reference), reference)
    const validation = validateThemePack(themeWithAsset(reference))
    assert.equal(validation.ok, false, reference)
    assert.match(validation.errors.join(' '), /assets\.tiles\.path/)
  }
})

test('community asset file policy rejects unsafe names, types, and sizes', () => {
  assert.deepEqual(validateThemeAssetFile({
    name: 'sprites/player.webp',
    size: 128_000,
    type: 'image/webp'
  }), [])

  assert.ok(validateThemeAssetFile({
    name: '../player.png',
    size: 100,
    type: 'image/png'
  }).length > 0)
  assert.ok(validateThemeAssetFile({
    name: 'player.svg',
    size: 100,
    type: 'image/svg+xml'
  }).length > 0)
  assert.ok(validateThemeAssetFile({
    name: 'player.png',
    size: MAX_THEME_ASSET_BYTES + 1,
    type: 'image/png'
  }).length > 0)
})
