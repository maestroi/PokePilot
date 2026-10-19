import { reactive } from 'vue'
import {
  actStudioPublication, assessStudioRun, createEditedStudioPackage, createRun,
  createStudioEditPlan, createStudioPublication, getRun,
  getStudioEditStatus, getStudioPublications, getYouTubePublishStatus,
  renderBroadcastReplay, getBroadcastReplayStatus, renderStudioEdit,
  renderStudioThumbnails, selectStudioThumbnail
} from '../shared/api/client'
import { runFullE2E, type FullE2EAPI, type FullE2EOptions, type FullE2EOutput, type FullE2EStage } from './fullProduction'
import { readE2EHistory, storeE2EEntry, writeE2EHistory, type E2EHistoryEntry, type E2EHistoryState } from './e2eHistory'

// Restore only historical receipts, never a live worker or upload authority.
// The server still owns gameplay, media artifacts and publication records.
const previouslySaved = readE2EHistory()
const latest = previouslySaved[0]
export const fullE2E = reactive({
  runID: latest?.runID || '',
  working: false,
  complete: latest?.state === 'passed',
  failure: latest?.failure || '',
  lastStatus: (latest?.state || 'not_started') as E2EHistoryState | 'not_started',
  lastUpdatedAt: latest?.updatedAt || 0,
  uploadPrivate: false,
  stages: (latest?.stages || []) as FullE2EStage[],
  outputs: (latest?.outputs || []) as FullE2EOutput[],
  history: previouslySaved as E2EHistoryEntry[]
})

function saveE2EReceipt(): void {
  if (!fullE2E.runID) return
  const previous = fullE2E.history.find(entry => entry.runID === fullE2E.runID)
  const now = Date.now()
  fullE2E.lastUpdatedAt = now
  const next: E2EHistoryEntry = {
    runID: fullE2E.runID,
    state: fullE2E.lastStatus === 'not_started' ? 'interrupted' : fullE2E.lastStatus,
    startedAt: previous?.startedAt || now,
    updatedAt: now,
    uploadRequested: fullE2E.uploadPrivate,
    stages: fullE2E.stages.map(s => ({ ...s })),
    outputs: fullE2E.outputs.map(o => ({ ...o })),
    failure: fullE2E.failure
  }
  fullE2E.history = storeE2EEntry(fullE2E.history, next)
  writeE2EHistory(fullE2E.history)
}

export function openE2EHistory(runID: string): void {
  if (fullE2E.working) return
  const entry = fullE2E.history.find(item => item.runID === runID)
  fullE2E.runID = runID
  fullE2E.lastStatus = entry?.state || 'not_started'
  fullE2E.lastUpdatedAt = entry?.updatedAt || 0
  fullE2E.complete = entry?.state === 'passed'
  fullE2E.failure = entry?.failure || ''
  fullE2E.uploadPrivate = false // private-upload opt-in must never be replayed
  fullE2E.stages = entry?.stages.map(s => ({ ...s })) || []
  fullE2E.outputs = entry?.outputs.map(o => ({ ...o })) || []
}

export const fullE2EAPI: FullE2EAPI = {
  createRun,
  getRun,
  renderBroadcast: renderBroadcastReplay,
  broadcastStatus: getBroadcastReplayStatus,
  assess: assessStudioRun,
  createPlan: createStudioEditPlan,
  renderEdit: renderStudioEdit,
  editStatus: getStudioEditStatus,
  createPackage: createEditedStudioPackage,
  renderThumbnails: (id, hash) => renderStudioThumbnails(id, hash, 1, false),
  selectThumbnail: selectStudioThumbnail,
  createCandidate: createStudioPublication,
  publicationAction: actStudioPublication,
  publications: getStudioPublications,
  youtubeStatus: getYouTubePublishStatus
}

type BeginOptions = Omit<FullE2EOptions, 'signal' | 'onStage' | 'onOutput'>
let controller: AbortController | null = null

function onStage(stage: FullE2EStage): void {
  const index = fullE2E.stages.findIndex(s => s.id === stage.id)
  if (index < 0) fullE2E.stages.push(stage)
  else fullE2E.stages.splice(index, 1, stage)
  saveE2EReceipt()
}

function onOutput(output: FullE2EOutput): void {
  const index = fullE2E.outputs.findIndex(item => item.format === output.format)
  if (index < 0) fullE2E.outputs.push(output)
  else fullE2E.outputs.splice(index, 1, output)
  saveE2EReceipt()
}

export function stopFullE2E(): void {
  controller?.abort()
}

export async function beginFullE2E(options: BeginOptions): Promise<void> {
  // Avoid duplicate workflows and real uploads from two components in the
  // same browser. Reloads and multi-instance coordination require #2794.
  if (fullE2E.working) return
  fullE2E.working = true
  fullE2E.complete = false
  fullE2E.failure = ''
  fullE2E.runID = options.runID
  fullE2E.lastStatus = 'working'
  fullE2E.uploadPrivate = options.uploadPrivate
  fullE2E.stages.splice(0)
  fullE2E.outputs.splice(0)
  saveE2EReceipt()
  const activeController = new AbortController()
  controller = activeController
  try {
    const outputs = await runFullE2E(fullE2EAPI, {
      ...options,
      signal: activeController.signal,
      onStage,
      onOutput
    })
    fullE2E.complete = outputs.length === 2 && (!options.uploadPrivate || outputs.every(o => o.state === 'published'))
    fullE2E.lastStatus = fullE2E.complete ? 'passed' : 'failed'
    if (!fullE2E.complete) fullE2E.failure = 'The full E2E acceptance gate was not satisfied.'
  } catch (error) {
    fullE2E.failure = error instanceof Error ? error.message : 'Production pipeline failed'
    fullE2E.lastStatus = activeController.signal.aborted ? 'interrupted' : 'failed'
  } finally {
    if (controller === activeController) controller = null
    fullE2E.working = false
    saveE2EReceipt()
  }
}
