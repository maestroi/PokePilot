import assert from 'node:assert/strict'
import test from 'node:test'
import { publicCapabilitiesForRun, supportsPublicCapability } from '../src/shared/publicCapabilities.ts'

test('pokemon profiles expose the world explorer capability', () => {
  for (const game of ['pokemon-red', 'pokemon-blue', 'pokemon-yellow', 'pokemon-gold', 'pokemon-silver']) {
    const capabilities = publicCapabilitiesForRun({ game })
    assert.equal(capabilities.includes('worldMap'), true, game)
    assert.equal(capabilities.includes('replay'), true, game)
  }
})

test('tetris does not expose the pokemon world explorer', () => {
  const capabilities = publicCapabilitiesForRun({ game: 'tetris', game_state: { kind: 'tetris' } })
  assert.deepEqual(capabilities, ['live', 'replay', 'stats'])
  assert.equal(supportsPublicCapability({ game: 'tetris' }, 'worldMap'), false)
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
