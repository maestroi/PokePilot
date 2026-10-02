import assert from 'node:assert/strict'
import test from 'node:test'
import { publicCapabilitiesForRun, supportsPublicCapability } from '../src/shared/publicCapabilities.ts'

test('supported Pokemon world profiles expose the world explorer', () => {
  assert.equal(supportsPublicCapability({ game: 'pokemon-red' }, 'worldMap'), true)
  assert.equal(supportsPublicCapability({ game: 'pokemon-gold' }, 'worldMap'), true)
  assert.equal(supportsPublicCapability({ game: 'pokemon-silver' }, 'worldMap'), true)
  for (const game of ['pokemon-blue', 'pokemon-yellow', 'tetris', 'boxxle']) {
    assert.equal(supportsPublicCapability({ game }, 'worldMap'), false, game)
  }
})

test('tetris does not expose the pokemon world explorer', () => {
  const capabilities = publicCapabilitiesForRun({ game: 'tetris', game_state: { kind: 'tetris' } })
  assert.deepEqual(capabilities, ['live', 'replay', 'stats'])
  assert.equal(supportsPublicCapability({ game: 'tetris' }, 'worldMap'), false)
})

test('boxxle does not expose the pokemon world explorer', () => {
  const capabilities = publicCapabilitiesForRun({ game: 'boxxle', game_state: { kind: 'boxxle' } })
  assert.deepEqual(capabilities, ['live', 'replay', 'stats'])
  assert.equal(supportsPublicCapability({ game: 'boxxle' }, 'worldMap'), false)
})

test('profile-advertised capabilities override the fallback matrix', () => {
  const capabilities = publicCapabilitiesForRun({
    game: 'future-game',
    public_capabilities: ['live', 'replay', 'worldMap', 'unknown']
  })
  assert.deepEqual(capabilities, ['live', 'replay', 'worldMap'])
})

test('unknown games stay on the generic watch and replay shell', () => {
  assert.deepEqual(publicCapabilitiesForRun({ game: 'future-game' }), ['live', 'replay'])
})
