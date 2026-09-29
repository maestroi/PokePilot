import assert from 'node:assert/strict'
import fs from 'node:fs'
import test from 'node:test'

import type { RenderState } from '../src/shared/api/renderstate.ts'
import { resolvePublicRenderTheme } from '../src/shared/renderTheme.ts'
import { modernSceneKind } from '../src/shared/semanticRenderer.ts'

const fixtures = JSON.parse(
  fs.readFileSync(new URL('../../renderstate/testdata/modern-scenes.json', import.meta.url), 'utf8')
) as Record<string, RenderState>

test('browser ModernSceneRenderer contract accepts the shared headless fixtures', () => {
  for (const kind of ['overworld', 'dialogue', 'menu', 'battle'] as const) {
    assert.equal(modernSceneKind(fixtures[kind]), kind)
  }
})

test('headless public media theme is the same public-safe browser theme', () => {
  const { theme, diagnostics } = resolvePublicRenderTheme('kenney-tiny-town')
  assert.equal(theme.id, 'kenney-tiny-town')
  assert.equal(theme.version, 1)
  assert.equal(theme.distribution, 'public')
  assert.equal(theme.allowGameArtFallbacks, false)
  assert.deepEqual(diagnostics, [])
})
