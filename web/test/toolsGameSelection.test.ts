import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const toolsSource = readFileSync(new URL('../src/operator/ToolsView.vue', import.meta.url), 'utf8')
const typesSource = readFileSync(new URL('../src/shared/api/types.ts', import.meta.url), 'utf8')

test('tools run form selects and submits a supported game', () => {
  assert.match(typesSource, /export interface RunSpec[\s\S]*\bgame: string/)
  assert.match(toolsSource, /game: 'pokemon-red'/)
  assert.match(toolsSource, /v-model="form\.game"/)
  assert.match(toolsSource, /<option value="pokemon-red">Pokémon Red<\/option>/)
  assert.match(toolsSource, /<option value="pokemon-blue">Pokémon Blue<\/option>/)
  assert.match(toolsSource, /game: form\.game/)
})
