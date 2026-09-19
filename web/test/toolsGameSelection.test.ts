import { readFileSync } from 'node:fs'
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

describe('operator game selection', () => {
  it('offers Red, Blue, and Yellow and sends the selected game', () => {
    const source = readFileSync(new URL('../src/operator/ToolsView.vue', import.meta.url), 'utf8')
    assert.ok(source.includes("game: 'pokemon-red'"))
    assert.ok(source.includes('value="pokemon-red"'))
    assert.ok(source.includes('value="pokemon-blue"'))
    assert.ok(source.includes('value="pokemon-yellow"'))
    assert.ok(source.includes('game: form.game'))
  })

  it('does not offer Red-style starter replacement for Yellow', () => {
    const source = readFileSync(new URL('../src/operator/ToolsView.vue', import.meta.url), 'utf8')
    assert.ok(source.includes("form.game === 'pokemon-yellow'"))
    assert.ok(source.includes('if (isYellow.value) return'))
    assert.ok(source.includes(':disabled="isYellow"'))
  })
})
