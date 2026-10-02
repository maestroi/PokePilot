import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { SpectatorRun } from '../src/shared/api/spectator.ts'
import { boxxleLocationLabel, boxxleRouteLabel, boxxleRunTitle, gameTitle, isBoxxleRun } from '../src/spectator/gamePresentation.ts'

// Boxxle has no map, starter, or party. Without its own title the spectator
// falls through to Pokémon Red.
test('spectator presentation recognises Boxxle runs', () => {
  const run = {
    game: 'boxxle',
    goal: 'early',
    game_state: { kind: 'boxxle', screen: 'puzzle', levels: 2, pushes: 18, crates: 3, crates_on_goal: 1 }
  } as SpectatorRun
  assert.equal(isBoxxleRun(run), true)
  assert.equal(isBoxxleRun({ game_state: { kind: 'boxxle' } } as SpectatorRun), true)
  assert.equal(gameTitle(run), 'Boxxle')
  assert.equal(boxxleRunTitle(run), 'Boxxle · 2 solved · 18 pushes')
  assert.equal(boxxleRouteLabel(run), 'early')
  assert.equal(boxxleLocationLabel(run), 'Puzzle 3 · 1/3 on goal')
  assert.equal(isBoxxleRun({ game: 'tetris' } as SpectatorRun), false)
})
