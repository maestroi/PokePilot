import assert from 'node:assert/strict'
import { test } from 'node:test'
import type { SpectatorRun } from '../src/shared/api/spectator.ts'
import { gameTitle, isTetrisRun, tetrisLocationLabel, tetrisRouteLabel, tetrisRunTitle } from '../src/spectator/gamePresentation.ts'
import { preferredRun, type SelectableSpectatorRun } from '../src/spectator/preferredRun.ts'

function run(run_id: string, status: string, queued_at: number, featured = false): SelectableSpectatorRun {
  return { run_id, status, queued_at, featured }
}

test('preferredRun keeps an explicitly selected run pinned', () => {
  const runs = [
    run('run-a', 'running', 10),
    run('run-b', 'running', 20, true)
  ]

  assert.equal(preferredRun(runs, 'run-a')?.run_id, 'run-a')
})

test('preferredRun does not switch when a pinned run temporarily disappears', () => {
  const runs = [
    run('run-b', 'running', 20, true),
    run('run-c', 'leased', 30)
  ]

  assert.equal(preferredRun(runs, 'run-a'), null)
})

test('preferredRun chooses the featured run when selection is not pinned', () => {
  const runs = [
    run('run-a', 'running', 30),
    run('run-b', 'running', 20, true),
    run('run-c', 'queued', 40)
  ]

  assert.equal(preferredRun(runs)?.run_id, 'run-b')
})

test('preferredRun still follows the newest live run when nothing is featured', () => {
  const runs = [
    run('run-a', 'running', 10),
    run('run-b', 'running', 20),
    run('run-c', 'queued', 30)
  ]

  assert.equal(preferredRun(runs)?.run_id, 'run-b')
})


test('Tetris spectator labels use game-owned state instead of Pokémon chrome', () => {
  const tetris: SpectatorRun = {
    run_id: 'tetris-live',
    status: 'running',
    game: 'tetris',
    goal: 'score:10000',
    game_state: {
      kind: 'tetris',
      mode: 'type-a',
      screen: 'playing',
      score: 573,
      lines_cleared: 7,
      level: 0,
      active: { piece: 'T', rotation: 1, x: 3, y: 4 },
      next: { piece: 'L' }
    }
  }

  assert.equal(isTetrisRun(tetris), true)
  assert.equal(gameTitle(tetris), 'Tetris')
  assert.equal(tetrisRunTitle(tetris), 'Tetris · 573 pts · 7 lines')
  assert.equal(tetrisRouteLabel(tetris), 'score:10000')
  assert.equal(tetrisLocationLabel(tetris), 'Level 0 · 7 lines')
})

test('Pokémon spectator labels keep their existing presentation', () => {
  const pokemon: SpectatorRun = {
    run_id: 'red-live',
    status: 'running',
    game: 'pokemon-red',
    starter: 'bulbasaur',
    player: { money: 0, badges: ['Boulder'], party: [] }
  }

  assert.equal(isTetrisRun(pokemon), false)
  assert.equal(gameTitle(pokemon), 'Pokémon Red')
})
