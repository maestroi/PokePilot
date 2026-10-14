import type { ExperimentPairResult, ExperimentView } from '../shared/api/types'

export type ParsedExperimentSeeds = { seeds: number[]; error: string }

// Reject malformed/duplicate seeds rather than silently dropping them and
// creating an experiment different from the comparison the operator requested.
export function parseExperimentSeeds(value: string, seedCount: number): ParsedExperimentSeeds {
  if (!Number.isSafeInteger(seedCount) || seedCount < 1 || seedCount > 1000) {
    return { seeds: [], error: 'Seed count must be a whole number from 1 to 1000.' }
  }
  const parts = value.trim().split(/[\s,]+/).filter(Boolean)
  if (!parts.length) return { seeds: [], error: '' }
  if (parts.length > 1000) {
    return { seeds: [], error: 'A comparison can have at most 1000 seeds.' }
  }
  const seeds: number[] = []
  const seen = new Set<number>()
  for (const part of parts) {
    if (!/^-?\d+$/.test(part)) {
      return { seeds: [], error: 'Seed list must contain only whole numbers separated by spaces or commas.' }
    }
    const seed = Number(part)
    if (!Number.isSafeInteger(seed)) {
      return { seeds: [], error: 'Every seed must be a safe whole number.' }
    }
    if (seen.has(seed)) {
      return { seeds: [], error: 'Seeds must be unique so every pair has a distinct run.' }
    }
    seeds.push(seed)
    seen.add(seed)
  }
  return { seeds, error: '' }
}

export type ExperimentProgress = {
  total: number
  completed: number
  excluded: number
  inProgress: number
}

function finished(pair: ExperimentPairResult): boolean {
  return pair.status_a === 'done' && pair.status_b === 'done'
}

export function experimentProgress(value: ExperimentView | null): ExperimentProgress {
  if (!value) return { total: 0, completed: 0, excluded: 0, inProgress: 0 }
  const pairs = value.pairs || []
  const total = Math.max(pairs.length, Number(value.total_pairs || 0))
  const excluded = pairs.length ? pairs.filter((pair) => !pair.comparable).length : Number(value.paired?.excluded_pairs || 0)
  const completed = pairs.length
    ? pairs.filter((pair) => pair.comparable && finished(pair)).length
    : Number(value.paired?.completed_pairs || 0)
  return { total, completed, excluded, inProgress: Math.max(0, total - completed - excluded) }
}

export function experimentStage(value: ExperimentView | null): string {
  if (!value) return 'Unavailable'
  const progress = experimentProgress(value)
  if (!progress.total) return 'No pairs'
  if (progress.excluded === progress.total) return 'Excluded · no comparable pairs'
  if (progress.inProgress > 0) return 'In progress'
  if (progress.excluded) return 'Completed · some pairs excluded'
  return 'Completed'
}
