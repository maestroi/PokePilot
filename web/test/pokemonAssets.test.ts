import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  itemDisplayName,
  itemSpriteUrl,
  itemToken,
  itemVisualKind,
  pokemonDexNumber,
  pokemonSpriteUrl
} from '../src/shared/pokemonAssets.ts'

test('resolves generation one species names to red/blue sprites', () => {
  assert.equal(pokemonDexNumber('pidgeotto'), 17)
  assert.equal(pokemonDexNumber('Mr. Mime'), 122)
  assert.equal(pokemonDexNumber('Nidoran♀'), 29)
  assert.equal(pokemonDexNumber('Nidoran♂'), 32)
  assert.match(pokemonSpriteUrl('butterfree') || '', /generation-i\/red-blue\/12\.png$/)
})

test('unknown nicknames fall back instead of inventing a species sprite', () => {
  assert.equal(pokemonDexNumber('BoulderCascade'), null)
  assert.equal(pokemonSpriteUrl('BoulderCascade'), null)
})

test('resolves common bag items and keeps machines as themed local fallbacks', () => {
  assert.match(itemSpriteUrl('dome fossil') || '', /items\/dome-fossil\.png$/)
  assert.match(itemSpriteUrl('S.S. Ticket') || '', /items\/ss-ticket\.png$/)
  assert.equal(itemVisualKind('tm24'), 'tm')
  assert.equal(itemVisualKind('HM01'), 'hm')
  assert.equal(itemSpriteUrl('tm24'), null)
  assert.equal(itemToken('tm24'), 'TM24')
  assert.equal(itemToken('hm1'), 'HM01')
})

test('formats game item names for compact spectator cards', () => {
  assert.equal(itemDisplayName('s.s. ticket'), 'S.S. Ticket')
  assert.equal(itemDisplayName('pokeball'), 'Poké Ball')
  assert.equal(itemDisplayName('tm11'), 'TM11')
  assert.equal(itemDisplayName('dome fossil'), 'Dome Fossil')
})
