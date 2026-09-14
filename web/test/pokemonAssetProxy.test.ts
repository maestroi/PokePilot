import assert from 'node:assert/strict'
import { test } from 'node:test'
import { browserPokemonAssetUrl } from '../src/shared/pokemonAssetProxy.ts'

test('rewrites approved PokeAPI artwork to same-origin pokeui assets', () => {
  assert.equal(
    browserPokemonAssetUrl('https://raw.githubusercontent.com/PokeAPI/sprites/master/sprites/pokemon/versions/generation-i/red-blue/17.png'),
    '/poke-assets/sprites/pokemon/versions/generation-i/red-blue/17.png'
  )
  assert.equal(
    browserPokemonAssetUrl('https://raw.githubusercontent.com/PokeAPI/sprites/master/sprites/items/dome-fossil.png'),
    '/poke-assets/sprites/items/dome-fossil.png'
  )
  assert.equal(
    browserPokemonAssetUrl('https://raw.githubusercontent.com/PokeAPI/sprites/master/sprites/badges/1.png'),
    '/poke-assets/sprites/badges/1.png'
  )
})

test('does not rewrite unrelated image sources', () => {
  assert.equal(browserPokemonAssetUrl(null), null)
  assert.equal(browserPokemonAssetUrl('/frame?run=abc'), '/frame?run=abc')
  assert.equal(browserPokemonAssetUrl('https://example.com/image.png'), 'https://example.com/image.png')
})
