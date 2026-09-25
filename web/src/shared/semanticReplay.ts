import type { RenderState, RenderTileLayer } from './api/renderstate'

export const SEMANTIC_REPLAY_VERSION = 1

export interface SemanticReplaySample {
  at_ms: number
  attempt?: number
  layer_set?: number
  state: RenderState
}

export interface SemanticReplayTimeline {
  version: number
  schema_version: number
  run_id?: string
  frames_per_second: number
  duration_ms: number
  layer_sets?: RenderTileLayer[][]
  samples: SemanticReplaySample[]
}

export function parseSemanticReplay(input: unknown): SemanticReplayTimeline {
  if (!input || typeof input !== 'object') throw new Error('semantic replay is not an object')
  const timeline = input as Partial<SemanticReplayTimeline>
  if (timeline.version !== SEMANTIC_REPLAY_VERSION) {
    throw new Error(`semantic replay version ${String(timeline.version)} is unsupported`)
  }
  if (timeline.schema_version !== 1) {
    throw new Error(`RenderState schema ${String(timeline.schema_version)} is unsupported`)
  }
  if (!Number.isFinite(timeline.frames_per_second) || Number(timeline.frames_per_second) <= 0) {
    throw new Error('semantic replay frame rate is invalid')
  }
  if (!Number.isFinite(timeline.duration_ms) || Number(timeline.duration_ms) < 0) {
    throw new Error('semantic replay duration is invalid')
  }
  if (!Array.isArray(timeline.samples)) throw new Error('semantic replay samples are missing')
  if (timeline.layer_sets !== undefined && !Array.isArray(timeline.layer_sets)) {
    throw new Error('semantic replay layer sets are invalid')
  }

  let previous = -1
  const layerSets = timeline.layer_sets || []
  for (const [index, sample] of timeline.samples.entries()) {
    if (!sample || typeof sample !== 'object' || !sample.state) {
      throw new Error(`semantic replay sample ${index} is invalid`)
    }
    const at = Number(sample.at_ms)
    if (!Number.isFinite(at) || at < previous || at < 0 || at > Number(timeline.duration_ms)) {
      throw new Error(`semantic replay sample ${index} has invalid time`)
    }
    const layerSet = Number(sample.layer_set || 0)
    if (!Number.isInteger(layerSet) || layerSet < 0 || layerSet > layerSets.length) {
      throw new Error(`semantic replay sample ${index} has invalid layer set`)
    }
    previous = at
  }
  return timeline as SemanticReplayTimeline
}

export function semanticReplaySampleAtMS(timeline: SemanticReplayTimeline | null, atMS: number): SemanticReplaySample | null {
  if (!timeline?.samples.length) return null
  const target = Number.isFinite(atMS) ? atMS : 0
  let low = 0
  let high = timeline.samples.length
  while (low < high) {
    const mid = (low + high) >>> 1
    if (timeline.samples[mid].at_ms <= target) low = mid + 1
    else high = mid
  }
  return timeline.samples[Math.max(0, low - 1)]
}

export function semanticReplayStateAtMS(timeline: SemanticReplayTimeline | null, atMS: number): RenderState | null {
  const sample = semanticReplaySampleAtMS(timeline, atMS)
  if (!sample || !timeline) return null
  const layerSet = Number(sample.layer_set || 0)
  const layers = layerSet > 0 ? timeline.layer_sets?.[layerSet - 1] : undefined
  return {
    ...sample.state,
    layers: layers || sample.state.layers
  }
}
