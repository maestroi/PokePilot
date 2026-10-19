import type { DashboardRun, ReplayStatus, StudioPublicationCandidate } from '../shared/api/types'
import { isVerifiedChampion } from './fullProduction'
import type { E2EHistoryEntry } from './e2eHistory'

export type E2EObservedTone = 'working' | 'ready' | 'failed' | 'unknown'
export interface E2EObservedStatus {
  tone: E2EObservedTone
  label: string
  detail: string
}

// Only farm and replay server records count for this live verdict.
// Browser history is intentionally not used to claim success.
export function observeE2E(
  run: DashboardRun,
  replay: ReplayStatus | null,
  publications: StudioPublicationCandidate[] | null
): E2EObservedStatus {
  if (run.status !== 'done') {
    if (['queued', 'leased', 'running', 'paused'].includes(run.status)) {
      return { tone: 'working', label: 'Game ' + run.status, detail: 'The farm has not reported a finished run. Video production is not yet verified.' }
    }
    return { tone: 'unknown', label: 'Game status: ' + run.status, detail: 'Inspect the game in Live to determine what happened.' }
  }
  if (!isVerifiedChampion(run)) {
    return { tone: 'failed', label: 'Game finished without Champion proof', detail: run.detail || run.reason || 'No semantic Hall of Fame completion recorded.' }
  }
  const full = publications?.filter(c => c.proposal.privacy === 'private' && c.proposal.format === 'episode') || []
  const shorts = publications?.filter(c => c.proposal.privacy === 'private' && c.proposal.format === 'short') || []
  const published = (list: StudioPublicationCandidate[]) => list.some(c => c.state === 'published' && Boolean(c.provider_id))
  if (published(full) && published(shorts)) {
    return { tone: 'ready', label: 'Both private upload receipts recorded', detail: 'The publication service recorded an episode and a Short with YouTube video IDs. Provider-side visibility is not independently verified.' }
  }
  if (publications?.some(c => ['blocked', 'failed'].includes(c.state))) {
    const c = publications.find(c => ['blocked', 'failed'].includes(c.state))!
    return { tone: 'failed', label: 'Publication blocked or failed', detail: c.reason || c.state }
  }
  const usable = (list: StudioPublicationCandidate[]) => list.some(c => ['queued', 'approved', 'publishing', 'published'].includes(c.state))
  if (usable(full) && usable(shorts)) {
    return { tone: 'ready', label: 'Episode + Short in publication queue', detail: 'Both private candidates exist. This does not prove that either video has been uploaded or externally verified.' }
  }
  if (replay?.state === 'ready' && Number(replay.size || 0) > 0) {
    return { tone: 'working', label: 'Replay rendered; production incomplete', detail: 'A broadcast video exists, but two private episode/Short publication candidates have not been confirmed.' }
  }
  if (replay && ['failed', 'error', 'cancelled'].includes(replay.state)) {
    return { tone: 'failed', label: 'Replay render failed', detail: replay.error || replay.state }
  }
  return { tone: 'working', label: 'Champion complete; media pending', detail: 'Game goal is verified, but no completed E2E media output has been observed.' }
}

export function historyLabel(entry: E2EHistoryEntry | undefined): string {
  if (!entry) return 'Possible E2E source · origin unverified'
  switch (entry.state) {
    case 'passed': return 'Previous browser test passed'
    case 'failed': return 'Previous browser test failed'
    case 'interrupted': return 'Browser coordination interrupted'
    default: return 'Started in this browser · last status unknown'
  }
}
