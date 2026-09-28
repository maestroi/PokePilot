import assert from 'node:assert/strict'
import test from 'node:test'

import { parseSemanticReplay, semanticReplayStateAtMS } from '../src/shared/semanticReplay.ts'

function state(frame: number, x: number) {
  return {
    schema_version: 1,
    game: { id: 'pokemon-red', revision: 'test' },
    clock: { frame },
    scene: 'overworld',
    map: { id: 'route-1', width: 2, height: 1 },
    player: { id: 'player', kind: 'player', position: { x, y: 0 } }
  }
}

test('semantic replay seeks without replaying earlier samples', () => {
  const timeline = parseSemanticReplay({
    version: 1,
    schema_version: 1,
    frames_per_second: 59.7275,
    duration_ms: 300,
    layer_sets: [[{
      id: 'terrain', kind: 'terrain', width: 2, height: 1,
      cells: [{ kind: 'grass' }, { kind: 'path' }]
    }]],
    samples: [
      { at_ms: 0, layer_set: 1, state: state(10, 0) },
      { at_ms: 100, layer_set: 1, state: state(16, 1) },
      { at_ms: 200, layer_set: 1, state: state(22, 2) }
    ]
  })
  assert.equal(semanticReplayStateAtMS(timeline, 150)?.clock.frame, 16)
  assert.equal(semanticReplayStateAtMS(timeline, 150)?.player?.position.x, 1)
  assert.equal(semanticReplayStateAtMS(timeline, 150)?.layers?.[0].cells[0].kind, 'grass')
  assert.equal(semanticReplayStateAtMS(timeline, -1)?.clock.frame, 10)
  assert.equal(semanticReplayStateAtMS(timeline, 999)?.clock.frame, 22)
})

test('semantic replay parser rejects broken ordering and layer references', () => {
  assert.throws(() => parseSemanticReplay({
    version: 1, schema_version: 1, frames_per_second: 60, duration_ms: 100,
    samples: [
      { at_ms: 50, state: state(1, 0) },
      { at_ms: 40, state: state(2, 1) }
    ]
  }), /invalid time/)

  assert.throws(() => parseSemanticReplay({
    version: 1, schema_version: 1, frames_per_second: 60, duration_ms: 100,
    layer_sets: [],
    samples: [{ at_ms: 0, layer_set: 1, state: state(1, 0) }]
  }), /invalid layer set/)
})
