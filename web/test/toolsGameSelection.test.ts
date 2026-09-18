import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe('operator game selection', () => {
  it('offers Red, Blue, and Yellow and sends the selected game', () => {
    const source = readFileSync(new URL('../src/operator/ToolsView.vue', import.meta.url), 'utf8')
    expect(source).toContain("game: 'pokemon-red'")
    expect(source).toContain('value="pokemon-red"')
    expect(source).toContain('value="pokemon-blue"')
    expect(source).toContain('value="pokemon-yellow"')
    expect(source).toContain('game: form.game')
  })

  it('does not offer Red-style starter replacement for Yellow', () => {
    const source = readFileSync(new URL('../src/operator/ToolsView.vue', import.meta.url), 'utf8')
    expect(source).toContain("form.game === 'pokemon-yellow'")
    expect(source).toContain('if (isYellow.value) return')
    expect(source).toContain(':disabled="isYellow"')
  })
})
