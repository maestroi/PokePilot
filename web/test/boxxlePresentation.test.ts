import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { SpectatorRun } from '../src/shared/api/spectator.ts'
import { gameTitle, isBoxxleRun } from '../src/spectator/gamePresentation.ts'

// Boxxle has no map, starter, or party. Without its own title the spectator
// falls through to Pokémon Red.
test('spectator presentation recognises Boxxle runs', () => {
  const run = { game: 'boxxle' } as SpectatorRun
  assert.equal(isBoxxleRun(run), true)
  assert.equal(gameTitle(run), 'Boxxle')
  assert.equal(isBoxxleRun({ game: 'tetris' } as SpectatorRun), false)
})
