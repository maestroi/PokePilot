<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getBroadcastReplayStatus, getDashboard, getModels, getProductionStatus, getRun, retryProduction, getStudioPublications, type ProductionStatus, studioEditVideoURL } from '../shared/api/client'
import type { DashboardRun, ReplayStatus, StudioPublicationCandidate } from '../shared/api/types'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { deploymentOptionLabel, deploymentSelectable, servesStrategist } from './llmDeployments'
import { beginFullE2E, fullE2E, openE2EHistory, stopFullE2E } from './fullE2EManager'
import { isFullCampaignSource } from './e2eHistory'
import { observeE2E, historyLabel } from './e2eRunStatus'
import { formatMediaHref, type MediaState } from './mediaQuery'

const props = defineProps<{ state: MediaState }>()
const inspectedID = ref(props.state.run || fullE2E.runID)
const mode = ref<'new' | 'existing'>(props.state.run ? 'existing' : 'new')
const existingID = ref(props.state.run)
const starter = ref('squirtle')
const deployment = ref('')
const seed = ref(1)
const uploadPrivate = ref(fullE2E.uploadPrivate)
watch(() => fullE2E.uploadPrivate, value => { uploadPrivate.value = value })
const uploadAcknowledged = ref(false)
const working = computed(() => fullE2E.working)
const complete = computed(() => fullE2E.complete)
const failure = computed(() => fullE2E.failure)
const currentID = computed(() => fullE2E.runID)
const stages = computed(() => fullE2E.stages)
const outputs = computed(() => fullE2E.outputs)

const models = usePollingResource(getModels, { intervalMs: 30000 })
// Discover runs from the wall even when no browser receipt survived. Active
// and finished records are queried independently so active runs do not hide
// the run the operator launched before a browser reload.
const recent = usePollingResource(async signal => {
  const [e2eActive, e2eDone, legacyActive, legacyDone] = await Promise.all([
    getDashboard({ active: true, purpose: 'full_e2e', limit: 200 }, signal),
    getDashboard({ status: 'done', purpose: 'full_e2e', limit: 200 }, signal),
    getDashboard({ active: true, limit: 100 }, signal),
    getDashboard({ status: 'done', limit: 100 }, signal)
  ])
  const unique = new Map<string, DashboardRun>()
  // Only short compatibility discovery uses Champion goals; all explicitly
  // tagged E2E runs are fetched with indexed database queries first.
  for (const run of [...legacyDone.runs, ...legacyActive.runs, ...e2eDone.runs, ...e2eActive.runs]) unique.set(run.run_id, run)
  return { runs: [...unique.values()] }
}, { intervalMs: 30000 })
const selections = computed(() => (models.data.value?.deployments || []).filter(d => servesStrategist(d) && deploymentSelectable(d)))
watch(selections, value => {
  if (!value.some(item => item.id === deployment.value)) deployment.value = value[0]?.id || ''
}, { immediate: true })
watch(() => props.state.run, id => {
  if (id) {
    inspectedID.value = id
    if (!working.value) {
      existingID.value = id
      mode.value = 'existing'
    }
  }
})
watch(() => fullE2E.runID, id => {
  if (id && fullE2E.working) inspectedID.value = id
})

const foundSources = computed(() => {
  const map = new Map<string, { run: DashboardRun | null; saved: boolean; updatedAt: number }>()
  for (const saved of fullE2E.history) {
    map.set(saved.runID, { run: null, saved: true, updatedAt: saved.updatedAt })
  }
  for (const run of recent.data.value?.runs || []) {
    if (!map.has(run.run_id) && !isFullCampaignSource(run)) continue
    const previous = map.get(run.run_id)
    map.set(run.run_id, {
      run, saved: Boolean(previous?.saved),
      updatedAt: Math.max(previous?.updatedAt || 0, Number(run.ended_at || run.queued_at || 0) * 1000)
    })
  }
  if (fullE2E.runID && !map.has(fullE2E.runID)) map.set(fullE2E.runID, { run: null, saved: true, updatedAt: Date.now() })
  if (props.state.run && !map.has(props.state.run)) map.set(props.state.run, { run: null, saved: false, updatedAt: 0 })
  return [...map.entries()]
    // Durable E2E identity should stay visible above legacy Champion guesses,
    // even when the user has thousands of ordinary recent campaigns.
    .sort((a, b) =>
      Number(b[1].run?.purpose === 'full_e2e') - Number(a[1].run?.purpose === 'full_e2e') ||
      b[1].updatedAt - a[1].updatedAt
    )
    .map(([id, meta]) => ({ id, ...meta }))
})
type SelectedEvidence = {
  run: DashboardRun | null
  replay: ReplayStatus | null
  publications: StudioPublicationCandidate[] | null
  production: ProductionStatus | null
  replayError: string
  publicationsError: string
}
const evidence = usePollingResource<SelectedEvidence>(async signal => {
  const id = inspectedID.value
  if (!id) return { run: null, replay: null, publications: null, production: null, replayError: '', publicationsError: '' }
  const run = await getRun(id, signal)
  const [replay, pubs, production] = await Promise.allSettled([getBroadcastReplayStatus(id, signal), getStudioPublications(id), getProductionStatus(id, signal)])
  return {
    run,
    replay: replay.status === 'fulfilled' ? replay.value : null,
    publications: pubs.status === 'fulfilled' ? pubs.value.candidates : null,
    production: production.status === 'fulfilled' ? production.value : null,
    replayError: replay.status === 'rejected' ? String(replay.reason) : '',
    publicationsError: pubs.status === 'rejected' ? String(pubs.reason) : ''
  }
}, { intervalMs: 10000 })
watch(inspectedID, () => { void evidence.retry() })
// Never render stale evidence from a previously selected run as the outcome
// of a different one while its fetch is in progress.
const selectedEvidence = computed(() => evidence.data.value?.run?.run_id === inspectedID.value ? evidence.data.value : null)
const observed = computed(() => selectedEvidence.value?.run
  ? observeE2E(selectedEvidence.value.run, selectedEvidence.value.replay, selectedEvidence.value.publications)
  : null)
function inspectRun(id: string): void {
  if (!/^[-_a-zA-Z0-9]{3,100}$/.test(id)) return
  inspectedID.value = id
  existingID.value = id
  mode.value = 'existing'
  if (!fullE2E.working) openE2EHistory(id)
  const url = formatMediaHref({ ...props.state, tab: 'e2e', run: id, job: '' }, window.location.search)
  window.history.replaceState(null, '', url)
  window.dispatchEvent(new PopStateEvent('popstate'))
}
const retryingProduction = ref(false)
const productionRetryError = ref('')
async function retryFailedProduction(): Promise<void> {
  retryingProduction.value = true
  productionRetryError.value = ''
  try {
    await retryProduction(inspectedID.value)
    await evidence.retry()
  } catch (cause) {
    productionRetryError.value = cause instanceof Error ? cause.message : 'Retry failed'
  } finally {
    retryingProduction.value = false
  }
}
function selectedRunLink(section: 'live' | 'studio' | 'library'): string {
  if (section === 'live') return '?run=' + encodeURIComponent(inspectedID.value) + '#live'
  return formatMediaHref({ ...props.state, run: inspectedID.value, job: '', tab: section }, window.location.search)
}
function candidateLabel(candidate: StudioPublicationCandidate): string {
  return candidate.proposal.format + ' · ' + candidate.state + (candidate.provider_id ? ' · video ' + candidate.provider_id : '')
}

const sourceLink = computed(() => currentID.value
  ? formatMediaHref({ ...props.state, tab: 'studio', run: currentID.value, job: '' }, window.location.search)
  : '')
const libraryLink = computed(() => currentID.value
  ? formatMediaHref({ ...props.state, tab: 'library', run: currentID.value, job: '' }, window.location.search)
  : '')
const canStart = computed(() => !working.value && (mode.value === 'new' ? Boolean(deployment.value && Number.isSafeInteger(seed.value) && seed.value >= 0)
  : Boolean(existingID.value.trim())) && (!uploadPrivate.value || uploadAcknowledged.value))
const outcome = computed(() => working.value ? 'running' : fullE2E.lastStatus)
const stageCount = computed(() => stages.value.filter(s => s.state === 'ready').length)
const report = computed(() => ({
  version: 1, run_id: currentID.value, status: outcome.value,
  upload_private: fullE2E.uploadPrivate, stages: stages.value, outputs: outputs.value,
  error: failure.value || undefined
}))

function newID(): string {
  const bytes = new Uint32Array(2)
  crypto.getRandomValues(bytes)
  return 'e2e-' + bytes[0].toString(36) + bytes[1].toString(36)
}
async function start(): Promise<void> {
  if (!canStart.value) return
  const id = mode.value === 'new' ? newID() : existingID.value.trim()
  inspectedID.value = id
  await beginFullE2E({
    runID: id,
    fresh: mode.value === 'new',
    starter: starter.value,
    seed: seed.value,
    deployment: deployment.value,
    uploadPrivate: uploadPrivate.value,
    pollMS: 10000
  })
  // Explicit upload consent is never retained for a subsequent attempt.
  uploadAcknowledged.value = false
}
function stop(): void {
  stopFullE2E()
}
function downloadReport(): void {
  const payload = new Blob([JSON.stringify(report.value, null, 2) + '\n'], { type: 'application/json' })
  const href = URL.createObjectURL(payload)
  const link = document.createElement('a')
  link.href = href
  link.download = 'pokepilot-e2e-' + (currentID.value || 'report') + '.json'
  link.click()
  URL.revokeObjectURL(href)
}
</script>

<template>
  <div class="grid min-w-0 gap-3 xl:grid-cols-[minmax(0,400px)_minmax(0,1fr)]">
    <section class="ui-bay self-start" aria-labelledby="full-e2e-title">
      <div class="ui-bay-header">
        <h2 id="full-e2e-title">Full production test</h2>
        <span class="text-meta text-fg-2">Pokémon Red</span>
      </div>
      <div class="space-y-4 p-3">
        <p class="text-meta text-fg-2">
          Play through the Champion and verified Hall of Fame, render an episode and a portrait Short, then prepare private YouTube publication candidates. No videos upload unless explicitly enabled below.
        </p>
        <fieldset :disabled="working" class="space-y-3">
          <legend class="text-meta font-semibold">Source</legend>
          <div class="flex flex-wrap gap-x-5 gap-y-2">
            <label class="flex items-center gap-2 text-sm"><input v-model="mode" type="radio" value="new" /> Fresh full game</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="mode" type="radio" value="existing" /> Existing run</label>
          </div>
          <template v-if="mode === 'new'">
            <label class="block space-y-1 text-meta" for="e2e-model">
              <span>Strategist deployment</span>
              <select id="e2e-model" v-model="deployment" class="ui-input h-10 w-full">
                <option v-if="!selections.length" value="">No available strategist deployment</option>
                <option v-for="item in selections" :key="item.id" :value="item.id">{{ deploymentOptionLabel(item) }}</option>
              </select>
            </label>
            <div class="grid grid-cols-2 gap-3">
              <label class="space-y-1 text-meta" for="e2e-starter">
                <span>Starter</span>
                <select id="e2e-starter" v-model="starter" class="ui-input h-10 w-full">
                  <option value="squirtle">Squirtle</option>
                  <option value="bulbasaur">Bulbasaur</option>
                  <option value="charmander">Charmander</option>
                </select>
              </label>
              <label class="space-y-1 text-meta" for="e2e-seed">
                <span>Seed</span>
                <input id="e2e-seed" v-model.number="seed" type="number" min="0" step="1" class="ui-input h-10 w-full" />
              </label>
            </div>
          </template>
          <label v-else class="block space-y-1 text-meta" for="e2e-run">
            <span>Existing Red full-campaign run</span>
            <input id="e2e-run" v-model="existingID" class="ui-input h-10 w-full font-mono" list="e2e-recent-runs" placeholder="run-..." autocomplete="off" />
            <button type="button" class="ui-btn ui-btn-sm mt-2" :disabled="!/^[-_a-zA-Z0-9]{3,100}$/.test(existingID.trim())" @click="inspectRun(existingID.trim())">Inspect this run · no production actions</button>
            <datalist id="e2e-recent-runs">
              <option v-for="run in (recent.data.value?.runs || []).filter(r => r.game === 'pokemon-red')" :key="run.run_id" :value="run.run_id">{{ run.goal || 'No goal' }}</option>
            </datalist>
          </label>
        </fieldset>

        <div class="space-y-2 border-t border-rule pt-3">
          <label class="flex items-start gap-2">
            <input v-model="uploadPrivate" :disabled="working" type="checkbox" class="mt-1" @change="uploadAcknowledged = false" />
            <span><span class="font-semibold">Upload to YouTube privately</span>
              <span class="block text-meta text-fg-2">Off by default. With this enabled, the test approves quality-gated candidates and performs actual YouTube uploads.</span>
            </span>
          </label>
          <label v-if="uploadPrivate" class="flex items-start gap-2 border border-rule-strong bg-surface p-2 text-meta">
            <input v-model="uploadAcknowledged" :disabled="working" type="checkbox" class="mt-0.5" />
            <span>I confirm this will upload two real videos to the configured YouTube account, both requested as <strong>private</strong>.</span>
          </label>
          <p class="text-meta text-fg-3">The test cannot independently confirm YouTube visibility yet. Check the uploaded videos on your channel before considering that gate complete.</p>
        </div>

        <p v-if="working && fullE2E.uploadPrivate" class="text-meta text-warn">Private uploads were explicitly authorized when this workflow started. They cannot be turned off by changing the form; use Stop coordinating to prevent further dispatches.</p>
        <div class="flex flex-wrap gap-2">
          <button v-if="!working" type="button" class="ui-btn ui-btn-primary h-10" :disabled="!canStart" @click="start">
            {{ uploadPrivate ? 'Start full E2E + private upload' : 'Start full E2E (no upload)' }}
          </button>
          <button v-else type="button" class="ui-btn h-10" @click="stop">Stop coordinating</button>
          <button v-if="currentID" type="button" class="ui-btn h-10" @click="downloadReport">Export report</button>
        </div>
        <p class="text-meta text-fg-2">
          Runs and publication candidates remain stored on their respective servers. Browser receipts are saved locally and restored after reload, but closing the page interrupts further production coordination. Use the run list on the right to inspect server status, then resume without upload if necessary.
        </p>
      </div>
    </section>

    <div class="min-w-0 space-y-3">
      <section class="ui-bay" aria-labelledby="e2e-history-title">
        <div class="ui-bay-header">
          <h2 id="e2e-history-title">E2E runs and Champion candidates</h2>
          <button class="ui-btn ui-btn-sm" type="button" @click="recent.retry(); evidence.retry()">Refresh</button>
        </div>
        <div class="space-y-3 p-3">
          <p class="text-meta text-fg-2">
            Server-tagged runs show <strong>E2E · database</strong> and remain discoverable across browsers or redeployments. Old untagged Champion runs are listed as candidates, never automatically marked E2E. Browser-local receipts are supplementary.
          </p>
          <p v-if="recent.error.value" class="text-meta text-warn" role="alert">Farm history unavailable: {{ recent.error.value }}. Saved browser receipts remain accessible.</p>
          <p v-if="!foundSources.length" class="text-meta text-fg-2">No local E2E receipts or recent Champion runs found. Enter a run ID under Existing run to inspect an older run.</p>
          <ul v-else class="max-h-72 space-y-1 overflow-y-auto" aria-label="Recent production test sources">
            <li v-for="row in foundSources" :key="row.id">
              <button type="button" class="flex w-full min-w-0 flex-wrap items-center gap-x-3 gap-y-1 border border-rule p-2 text-left hover:bg-raised"
                :class="inspectedID === row.id ? 'bg-selected' : ''" :aria-pressed="inspectedID === row.id" @click="inspectRun(row.id)">
                <span class="min-w-0 flex-1 truncate font-mono text-meta" :title="row.id">{{ row.id }}</span>
                <span class="text-meta text-fg-2">{{ row.run?.status || 'Not in recent farm list' }}</span>
                <span class="text-meta" :class="row.run?.purpose === 'full_e2e' ? 'text-ok font-semibold' : row.saved ? 'text-accent' : 'text-fg-3'">{{ row.run?.purpose === 'full_e2e' ? 'E2E · database' : row.saved ? 'Started here' : 'Champion candidate' }}</span>
                <span v-if="row.saved" class="text-meta text-fg-2">
                  {{ historyLabel(fullE2E.history.find(e => e.runID === row.id)) }}
                </span>
              </button>
            </li>
          </ul>
        </div>
      </section>

      <section v-if="inspectedID" class="ui-bay" aria-labelledby="e2e-server-status">
        <div class="ui-bay-header">
          <h2 id="e2e-server-status">Server evidence · <span class="font-mono">{{ inspectedID }}</span></h2>
          <span class="text-meta text-fg-2">Refreshes every 10s</span>
        </div>
        <div class="space-y-3 p-3">
          <div v-if="evidence.error.value" class="text-sm text-warn" role="alert">Cannot load run: {{ evidence.error.value }}. Try Refresh or open Live.</div>
          <p v-else-if="!selectedEvidence?.run" class="text-meta text-fg-2">Loading farm and media records…</p>
          <template v-else>
            <p class="font-semibold" :class="observed?.tone === 'failed' ? 'text-danger' : observed?.tone === 'ready' ? 'text-ok' : 'text-fg'">
              {{ observed?.label }}
            </p>
            <p class="text-meta text-fg-2">{{ observed?.detail }}</p>
            <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-meta">
              <dt class="text-fg-3">Farm</dt>
              <dd>{{ selectedEvidence.run.status }} · {{ selectedEvidence.run.reason || 'no finish reason' }}</dd>
              <dt class="text-fg-3">Champion</dt>
              <dd>{{ selectedEvidence.run.stats?.goal_complete ? 'Goal flag recorded' : (selectedEvidence.run.player?.milestones || []).some(m => /hall[ _]of[ _]fame/i.test(m)) ? 'Hall of Fame milestone' : 'Not verified' }}</dd>
              <dt class="text-fg-3">Production</dt>
              <dd>{{ selectedEvidence.production ? selectedEvidence.production.stage.replace(/_/g, ' ') : 'No status' }}<span v-if="selectedEvidence.production?.verdict"> · {{ selectedEvidence.production.verdict }}</span><span v-if="selectedEvidence.production?.error" class="text-warn"> · {{ selectedEvidence.production.error }}</span>
                <button v-if="selectedEvidence.production?.done && selectedEvidence.production.stage.startsWith('failed_')" type="button" class="ui-btn ui-btn-sm ml-2" :disabled="retryingProduction" @click="retryFailedProduction">Retry production</button>
                <span v-if="productionRetryError" class="text-warn"> · {{ productionRetryError }}</span></dd>
              <template v-if="selectedEvidence.production?.upload_private || Object.keys(selectedEvidence.production?.videos || {}).length">
                <dt class="text-fg-3">Private upload</dt>
                <dd>
                  <span v-if="!Object.keys(selectedEvidence.production?.videos || {}).length">Opted in · waiting for candidates</span>
                  <a v-for="(href, format) in selectedEvidence.production?.videos" :key="format" :href="href.startsWith('http') ? href : 'https://youtu.be/' + href" target="_blank" rel="noopener" class="mr-3 text-accent hover:underline">{{ format }} ↗</a>
                </dd>
              </template>
              <dt class="text-fg-3">Replay</dt>
              <dd>{{ selectedEvidence.replay?.state || 'No status' }}<span v-if="selectedEvidence.replayError" class="text-warn"> · unavailable</span></dd>
              <dt class="text-fg-3">Publication</dt>
              <dd>{{ selectedEvidence.publications?.length ?? 'Unavailable' }} candidates<span v-if="selectedEvidence.publicationsError" class="text-warn"> · API error</span></dd>
            </dl>
            <div v-if="selectedEvidence.publications?.length" class="space-y-1 border-t border-rule pt-2">
              <p v-for="candidate in selectedEvidence.publications" :key="candidate.id" class="text-meta">
                {{ candidateLabel(candidate) }} · {{ candidate.proposal.privacy }}
                <a v-if="candidate.provider_url" :href="candidate.provider_url" target="_blank" rel="noopener noreferrer" class="text-accent underline">Open upload</a>
              </p>
            </div>
          </template>
          <div class="flex flex-wrap gap-2">
            <a :href="selectedRunLink('live')" class="ui-btn ui-btn-sm">Inspect game</a>
            <a :href="selectedRunLink('studio')" class="ui-btn ui-btn-sm">Open Studio</a>
            <a :href="selectedRunLink('library')" class="ui-btn ui-btn-sm">Open Library</a>
          </div>
          <p class="text-meta text-fg-3">The replay server produces media for finished E2E runs automatically and resumes after restarts; nothing here needs to stay open.</p>
        </div>
      </section>

      <section class="ui-bay" aria-labelledby="e2e-progress-title" aria-live="polite">
        <div class="ui-bay-header">
          <h2 id="e2e-progress-title">Browser coordination receipt</h2>
          <span class="text-meta text-fg-2">{{ stageCount }} stages ready</span>
        </div>
        <div class="space-y-3 p-3">
          <p v-if="currentID" class="min-w-0 break-all font-mono text-sm">{{ currentID }}</p>
          <div v-if="currentID" class="flex flex-wrap gap-2">
            <a :href="sourceLink" class="ui-btn ui-btn-sm">Open Studio</a>
            <a :href="libraryLink" class="ui-btn ui-btn-sm">Open recording</a>
            <a :href="'?run=' + encodeURIComponent(currentID) + '#live'" class="ui-btn ui-btn-sm">Inspect game</a>
          </div>
          <p v-if="!stages.length" class="py-5 text-center text-fg-2">Select a run above for its server-side status, or start/resume production to record browser stages here.</p>
          <p v-if="fullE2E.lastStatus === 'interrupted' && !working" class="text-meta text-warn">The previous browser coordinator stopped before it could confirm completion. Server evidence above may still show that the game or media jobs progressed.</p>
          <p v-if="fullE2E.lastUpdatedAt" class="text-meta text-fg-3">Last saved browser receipt: {{ new Date(fullE2E.lastUpdatedAt).toLocaleString() }}</p>
          <ol v-else class="space-y-2">
            <li v-for="stage in stages" :key="stage.id" class="flex gap-2 border-b border-rule pb-2 last:border-0">
              <span aria-hidden="true" :class="stage.state === 'ready' ? 'text-ok' : stage.state === 'failed' ? 'text-danger' : 'text-accent'">
                {{ stage.state === 'ready' ? '✓' : stage.state === 'failed' ? '✕' : '◌' }}
              </span>
              <div class="min-w-0 flex-1">
                <p class="font-medium">{{ stage.label }}</p>
                <p v-if="stage.detail" class="break-words text-meta text-fg-2">{{ stage.detail }}</p>
              </div>
              <span class="text-meta text-fg-2">{{ stage.state }}</span>
            </li>
          </ol>
          <p v-if="failure" class="border border-danger/40 p-3 text-danger break-words" role="alert">{{ failure }}</p>
          <p v-else-if="complete" class="border border-rule p-3 text-ok" role="status">Both edited formats passed {{ fullE2E.uploadPrivate ? 'private upload receipt checks' : 'the no-upload production test' }}.</p>
        </div>
      </section>

      <section v-if="outputs.length" class="ui-bay" aria-labelledby="e2e-output-title">
        <div class="ui-bay-header"><h2 id="e2e-output-title">Produced media</h2></div>
        <div class="grid gap-3 p-3 md:grid-cols-2">
          <article v-for="output in outputs" :key="output.format" class="min-w-0 space-y-2 border border-rule bg-surface p-3">
            <h3 class="font-semibold">{{ output.format === 'episode' ? 'Full episode' : 'YouTube Short' }}</h3>
            <p class="text-meta text-fg-2">{{ output.state }} · {{ output.privacy }}</p>
            <p class="truncate font-mono text-meta text-fg-3" :title="output.candidateID">Candidate {{ output.candidateID.slice(0, 12) }}</p>
            <div class="flex flex-wrap gap-2">
              <a :href="studioEditVideoURL(currentID, output.planHash)" target="_blank" rel="noopener noreferrer" class="ui-btn ui-btn-sm">Preview edit</a>
              <a v-if="output.videoURL" :href="output.videoURL" target="_blank" rel="noopener noreferrer" class="ui-btn ui-btn-sm">Open on YouTube ↗</a>
            </div>
          </article>
        </div>
      </section>
    </div>
  </div>
</template>
