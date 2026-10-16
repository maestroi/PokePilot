import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { archiveStarter, partyMonIsStarter, starterPresentation } from '../src/operator/runs.ts'

test('pokémon runs show the resolved starter, and other games do not', () => {
  assert.deepEqual(starterPresentation({ run_id: 'r', status: 'done', game: 'pokemon-red', starter: 'mewtwo' }), {
    species: 'mewtwo',
    detail: ''
  })
  assert.deepEqual(starterPresentation({
    run_id: 'r',
    status: 'done',
    game: 'pokemon-red',
    starter: 'random',
    starter_species: 'oddish'
  }), { species: 'oddish', detail: 'random' })
  assert.deepEqual(starterPresentation({ run_id: 'r', status: 'done', game: 'pokemon-yellow' }), {
    species: 'pikachu',
    detail: ''
  })
  assert.deepEqual(starterPresentation({ run_id: 'r', status: 'done', game: 'pokemon-gold', starter: 'cyndaquil' }), {
    species: 'cyndaquil',
    detail: ''
  })
  assert.equal(starterPresentation({ run_id: 'r', status: 'done', game: 'pokemon-red', planner: 'llm' })?.species, 'LLM picks')
  assert.equal(starterPresentation({ run_id: 'r', status: 'done', game: 'tetris' }), null)
  assert.equal(starterPresentation({ run_id: 'r', status: 'done', game: 'boxxle' }), null)
  assert.equal(archiveStarter({ run_id: 'r', status: 'done', game: 'tetris' }), '')
  assert.equal(archiveStarter({ run_id: 'r', status: 'done', game: 'pokemon-red', starter: 'random', starter_species: 'mewtwo' }), 'mewtwo · random')
  const live = readFileSync(new URL('../src/operator/operations.ts', import.meta.url), 'utf8')
  assert.match(live, /starterPresentation\(run\)/)
})

test('the matching party member is marked as the starter', () => {
  assert.equal(partyMonIsStarter('Mewtwo', 'mewtwo'), true)
  assert.equal(partyMonIsStarter('oddish', 'mewtwo'), false)
  assert.equal(partyMonIsStarter('pikachu', 'LLM picks'), false)
  assert.equal(partyMonIsStarter('pikachu', 'random'), false)
})
