import type { DashboardRun } from '../shared/api/types'
import type { FullE2EOutput, FullE2EStage } from './fullProduction'

export const E2E_HISTORY_KEY = 'pokepilot.operator.full-e2e.v1'
export type E2EHistoryState = 'working' | 'interrupted' | 'passed' | 'failed'
export interface E2EHistoryEntry {
  runID: string
  state: E2EHistoryState
  startedAt: number
  updatedAt: number
  uploadRequested: boolean
  stages: FullE2EStage[]
  outputs: FullE2EOutput[]
  failure: string
}

const validID = (id: string) => /^[-_a-zA-Z0-9]{3,100}$/.test(id)
const validStates: E2EHistoryState[] = ['working', 'interrupted', 'passed', 'failed']

// Never trust storage blindly: older versions and stale/corrupted snapshots
// should not break the entire operator UI. A 'working' state is a historical
// browser receipt, NOT evidence a background coordinator survived reload.
export function parseE2EHistory(value: string | null, now = Date.now()): E2EHistoryEntry[] {
  if (!value) return []
  try {
    const source: unknown = JSON.parse(value)
    if (!Array.isArray(source)) return []
    const seen = new Set<string>()
    const entries: E2EHistoryEntry[] = []
    for (const item of source) {
      if (!item || typeof item !== 'object') continue
      const raw = item as Partial<E2EHistoryEntry>
      if (typeof raw.runID !== 'string' || !validID(raw.runID) || seen.has(raw.runID)) continue
      if (!validStates.includes(raw.state as E2EHistoryState)) continue
      if (!Number.isFinite(raw.startedAt) || !Number.isFinite(raw.updatedAt)) continue
      const startedAt = Number(raw.startedAt)
      const updatedAt = Number(raw.updatedAt)
      if (startedAt < 0 || updatedAt < startedAt || updatedAt > now + 86400000) continue
      seen.add(raw.runID)
      entries.push({
        runID: raw.runID,
        state: raw.state === 'working' ? 'interrupted' : raw.state,
        startedAt, updatedAt,
        uploadRequested: raw.uploadRequested === true,
        stages: Array.isArray(raw.stages) ? raw.stages.filter(s => s && typeof s.id === 'string' && typeof s.label === 'string').slice(-30) : [],
        outputs: Array.isArray(raw.outputs) ? raw.outputs.filter(o => o && (o.format === 'episode' || o.format === 'short')).slice(-2) : [],
        failure: typeof raw.failure === 'string' ? raw.failure.slice(0, 1000) : ''
      })
    }
    return entries.sort((a, b) => b.updatedAt - a.updatedAt).slice(0, 100)
  } catch {
    return []
  }
}

export function storeE2EEntry(items: E2EHistoryEntry[], next: E2EHistoryEntry): E2EHistoryEntry[] {
  return [next, ...items.filter(item => item.runID !== next.runID)]
    .sort((a, b) => b.updatedAt - a.updatedAt).slice(0, 100)
}

export function readE2EHistory(): E2EHistoryEntry[] {
  try {
    return typeof localStorage === 'undefined' ? [] : parseE2EHistory(localStorage.getItem(E2E_HISTORY_KEY))
  } catch {
    return []
  }
}

export function writeE2EHistory(entries: E2EHistoryEntry[]): void {
  try {
    if (typeof localStorage !== 'undefined') localStorage.setItem(E2E_HISTORY_KEY, JSON.stringify(entries))
  } catch {
    // Browsers without storage permissions can still run the E2E test.
  }
}

export function isFullCampaignSource(run: Pick<DashboardRun, 'run_id' | 'game' | 'goal'>): boolean {
  return run.game === 'pokemon-red' &&
    (('purpose' in run && run.purpose === 'full_e2e') ||
    run.run_id.startsWith('e2e-') || ['elite-four', 'beat the elite four and champion.', 'beat the elite four and champion'].includes((run.goal || '').trim().toLowerCase()))
}
