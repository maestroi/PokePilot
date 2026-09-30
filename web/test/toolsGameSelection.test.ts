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

test('tools run form shows Yellow scripted Pikachu explicitly', () => {
  assert.match(toolsSource, /<option value="pokemon-yellow">Pokémon Yellow<\/option>/)
  assert.ok(toolsSource.includes("form.game === 'pokemon-yellow'"))
  assert.ok(toolsSource.includes("if (isYellow.value) return 'pikachu'"))
  assert.ok(toolsSource.includes('value="Pikachu · scripted"'))
  assert.ok(toolsSource.includes('!isYellow.value && !isGen2.value && isSpecificStarter.value'))
})


test('tools run form exposes Gold and Silver with native Gen2 starters', () => {
  assert.match(toolsSource, /<option value="pokemon-gold">Pokémon Gold<\/option>/)
  assert.match(toolsSource, /<option value="pokemon-silver">Pokémon Silver<\/option>/)
  assert.ok(toolsSource.includes("form.game === 'pokemon-gold' || form.game === 'pokemon-silver'"))
  assert.ok(toolsSource.includes('<option value="chikorita">Chikorita</option>'))
  assert.ok(toolsSource.includes('<option value="cyndaquil">Cyndaquil</option>'))
  assert.ok(toolsSource.includes('<option value="totodile">Totodile</option>'))
  assert.ok(toolsSource.includes("form.goal = 'progress:gs_supported_frontier'"))
  assert.ok(toolsSource.includes('Experimental · play as far as currently supported'))
  assert.ok(toolsSource.includes('they do not imply the full game is complete'))
  assert.ok(toolsSource.includes("isGen2.value) return starterMode.value === 'default' ? '' : starterMode.value"))
})
