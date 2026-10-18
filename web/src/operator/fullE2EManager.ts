import { reactive } from 'vue'
import {
  actStudioPublication, assessStudioRun, createEditedStudioPackage, createRun,
  createStudioEditPlan, createStudioPublication, getRun,
  getStudioEditStatus, getStudioPublications, getYouTubePublishStatus,
  renderBroadcastReplay, getBroadcastReplayStatus, renderStudioEdit,
  renderStudioThumbnails, selectStudioThumbnail
} from '../shared/api/client'
import { runFullE2E, type FullE2EAPI, type FullE2EOptions, type FullE2EOutput, type FullE2EStage } from './fullProduction'

// This coordinator is owned by the operator application, not the Media tab.
// Route changes leave the same in-progress async workflow running. The wall,
// replay workers and uploader own their own jobs, but browser closing/reloading
// still stops the coordinator; persistent server-side coordination is #2794.
export const fullE2E = reactive({
  runID: '',
  working: false,
  complete: false,
  failure: '',
  uploadPrivate: false,
  stages: [] as FullE2EStage[],
  outputs: [] as FullE2EOutput[]
})

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
}

function onOutput(output: FullE2EOutput): void {
  const index = fullE2E.outputs.findIndex(item => item.format === output.format)
  if (index < 0) fullE2E.outputs.push(output)
  else fullE2E.outputs.splice(index, 1, output)
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
  fullE2E.uploadPrivate = options.uploadPrivate
  fullE2E.stages.splice(0)
  fullE2E.outputs.splice(0)
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
  } catch (error) {
    fullE2E.failure = error instanceof Error ? error.message : 'Production pipeline failed'
  } finally {
    if (controller === activeController) controller = null
    fullE2E.working = false
  }
}
