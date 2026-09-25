import assert from 'node:assert/strict'
import test from 'node:test'

import { canRenderModernScene, canRenderOverworld, modernSceneKind, semanticViewport } from '../src/shared/semanticRenderer.ts'
import type { RenderState } from '../src/shared/api/renderstate.ts'

function state(): RenderState {
  return {
    schema_version: 1,
    game: { id: 'pokemon-red', revision: 'en-us-rev0' },
    clock: { frame: 1 },
    scene: 'overworld',
    capabilities: ['map', 'player', 'layers', 'entities'],
    map: { id: 'route 1', width: 20, height: 36 },
    player: { kind: 'player', position: { x: 10, y: 30 }, facing: 'up' },
    layers: [{
      id: 'terrain',
      kind: 'terrain',
      width: 20,
      height: 36,
      cells: Array.from({ length: 20 * 36 }, () => ({ kind: 'path' }))
    }]
  }
}

test('modern renderer requires an overworld semantic surface', () => {
  const good = state()
  assert.equal(canRenderOverworld(good), true)

  const battle = { ...good, scene: 'battle' }
  assert.equal(canRenderOverworld(battle), false)

  const missing = { ...good, capabilities: ['map', 'player'] }
  assert.equal(canRenderOverworld(missing), false)

  const malformed = state()
  malformed.layers![0].cells.pop()
  assert.equal(canRenderOverworld(malformed), false)
})

test('camera follows the player while clamping to map edges', () => {
  const middle = state()
  const viewport = semanticViewport(middle, 320, 288, 32)
  assert.ok(viewport.startY > 0)
  assert.ok(viewport.endY <= 36)
  assert.ok(middle.player!.position.y >= viewport.startY)
  assert.ok(middle.player!.position.y < viewport.endY)

  const top = state()
  top.player!.position = { x: 0, y: 0 }
  const topViewport = semanticViewport(top, 320, 288, 32)
  assert.equal(topViewport.startX, 0)
  assert.equal(topViewport.startY, 0)

  const bottom = state()
  bottom.player!.position = { x: 19, y: 35 }
  const bottomViewport = semanticViewport(bottom, 320, 288, 32)
  assert.equal(bottomViewport.endX, 20)
  assert.equal(bottomViewport.endY, 36)
})


test('modern scene selection keeps supported non-overworld scenes semantic', () => {
  const battle = state()
  battle.scene = 'battle'
  battle.capabilities = ['battle']
  battle.battle = {
    kind: 'wild',
    actors: [
      { role: 'player', name: 'Charmander', hp: 20, max_hp: 30 },
      { role: 'opponent', name: 'Pidgey', hp: 9, max_hp: 12 }
    ]
  }
  assert.equal(modernSceneKind(battle), 'battle')
  assert.equal(canRenderModernScene(battle), true)

  const dialogue = state()
  dialogue.scene = 'dialogue'
  dialogue.capabilities = ['dialogue']
  dialogue.dialogue = { text: 'Welcome to the world of Pokémon!' }
  assert.equal(modernSceneKind(dialogue), 'dialogue')
  assert.equal(canRenderModernScene(dialogue), true)

  const menu = state()
  menu.scene = 'menu'
  menu.capabilities = ['menu']
  menu.menu = { title: 'ITEM POKEMON EXIT', cursor: 1, entries: [{ id: '0' }, { id: '1' }, { id: '2' }] }
  assert.equal(modernSceneKind(menu), 'menu')
  assert.equal(canRenderModernScene(menu), true)

  const unsupported = state()
  unsupported.scene = 'transition'
  unsupported.capabilities = ['transition']
  assert.equal(modernSceneKind(unsupported), '')
  assert.equal(canRenderModernScene(unsupported), false)
})
