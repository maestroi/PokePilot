<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getBroadcastReplayStatus, getDashboard, getProductionStatus, getRun, requestProduction, retryProduction, getStudioPublications, type ProductionStatus, studioEditVideoURL } from '../shared/api/client'
import type { DashboardRun, ReplayStatus, StudioPublicationCandidate } from '../shared/api/types'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { operatorHref } from '../shared/urls'
import { isFullCampaignSource, observeE2E } from './e2eRunStatus'
import { formatMediaHref, type MediaState } from './mediaQuery'

// Full E2E is owned by pokereplay's production loop. This page inspects
// server records and sends operator decisions (retry, private-upload opt-in)
// back to the server; it coordinates nothing itself.
const props = defineProps<{ state: MediaState }>()
const inspectedID = ref(props.state.run)
const existingID = ref(props.state.run)
const newRunHref = operatorHref('new-run', {}, window.location.search)

// Discover runs from the wall: tagged E2E runs first, then older untagged
// Champion campaigns as candidates.
const recent = usePollingResource(async signal => {
  const [e2eActive, e2eDone, legacyActive, legacyDone] = await Promise.all([
    getDashboard({ active: true, purpose: 'full_e2e', limit: 200 }, signal),
    getDashboard({ status: 'done', purpose: 'full_e2e', limit: 200 }, signal),
    getDashboard({ active: true, limit: 100 }, signal),
    getDashboard({ status: 'done', limit: 100 }, signal)
  ])
  const unique = new Map<string, DashboardRun>()
  for (const run of [...legacyDone.runs, ...legacyActive.runs, ...e2eDone.runs, ...e2eActive.runs]) unique.set(run.run_id, run)
  return { runs: [...unique.values()] }
}, { intervalMs: 30000 })
watch(() => props.state.run, id => {
  if (id) {
    inspectedID.value = id
    existingID.value = id
  }
})

const foundSources = computed(() => (recent.data.value?.runs || [])
  .filter(isFullCampaignSource)
  .sort((a, b) =>
    Number(b.purpose === 'full_e2e') - Number(a.purpose === 'full_e2e') ||
    Number(b.ended_at || b.queued_at || 0) - Number(a.ended_at || a.queued_at || 0)
  ))
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
watch(inspectedID, () => {
  uploadOptIn.value = false
  uploadAcknowledged.value = false
  actionError.value = ''
  void evidence.retry()
})
// Never render stale evidence from a previously selected run as the outcome
// of a different one while its fetch is in progress.
const selectedEvidence = computed(() => evidence.data.value?.run?.run_id === inspectedID.value ? evidence.data.value : null)
const observed = computed(() => selectedEvidence.value?.run
  ? observeE2E(selectedEvidence.value.run, selectedEvidence.value.replay, selectedEvidence.value.publications)
  : null)
const production = computed(() => selectedEvidence.value?.production || null)
const productionFailed = computed(() => Boolean(production.value?.done && production.value.stage.startsWith('failed_')))
const canOptIn = computed(() => {
  const p = production.value
  return selectedEvidence.value?.run?.purpose === 'full_e2e' && !p?.upload_private && !(p?.done && p.stage !== 'ready_for_review')
})
function inspectRun(id: string): void {
  if (!/^[-_a-zA-Z0-9]{3,100}$/.test(id)) return
  inspectedID.value = id
  existingID.value = id
  const url = formatMediaHref({ ...props.state, tab: 'e2e', run: id, job: '' }, window.location.search)
  window.history.replaceState(null, '', url)
  window.dispatchEvent(new PopStateEvent('popstate'))
}

const acting = ref(false)
const actionError = ref('')
const uploadOptIn = ref(false)
const uploadAcknowledged = ref(false)
async function act(action: () => Promise<unknown>): Promise<void> {
  acting.value = true
  actionError.value = ''
  try {
    await action()
    await evidence.retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Request failed'
  } finally {
    acting.value = false
  }
}
function retryFailedProduction(): Promise<void> {
  return act(() => retryProduction(inspectedID.value))
}
function optIntoPrivateUpload(): Promise<void> {
  if (!uploadAcknowledged.value) return Promise.resolve()
  return act(async () => {
    await requestProduction(inspectedID.value, true)
    uploadOptIn.value = false
    uploadAcknowledged.value = false
  })
}
function selectedRunLink(section: 'live' | 'studio' | 'library'): string {
  if (section === 'live') return '?run=' + encodeURIComponent(inspectedID.value) + '#live'
  return formatMediaHref({ ...props.state, run: inspectedID.value, job: '', tab: section }, window.location.search)
}
function candidateLabel(candidate: StudioPublicationCandidate): string {
  return candidate.proposal.format + ' · ' + candidate.state + (candidate.provider_id ? ' · video ' + candidate.provider_id : '')
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
          A Full E2E run plays through the Champion. When it finishes, the replay server renders the broadcast replay, an episode and a portrait Short, packages them, and creates private publication candidates. With private upload opted in, it also uploads both as private YouTube videos. Nothing here needs to stay open.
        </p>
        <a :href="newRunHref" class="ui-btn ui-btn-primary h-10">Start a Full E2E run</a>
        <p class="text-meta text-fg-3">Choose <strong>Run workflow → Full E2E</strong> on New run; private upload is opted in there.</p>
        <label class="block space-y-1 border-t border-rule pt-3 text-meta" for="e2e-run">
          <span>Inspect a run</span>
          <input id="e2e-run" v-model="existingID" class="ui-input h-10 w-full font-mono" list="e2e-recent-runs" placeholder="run-..." autocomplete="off" />
          <button type="button" class="ui-btn ui-btn-sm mt-2" :disabled="!/^[-_a-zA-Z0-9]{3,100}$/.test(existingID.trim())" @click="inspectRun(existingID.trim())">Inspect</button>
          <datalist id="e2e-recent-runs">
            <option v-for="run in (recent.data.value?.runs || []).filter(r => r.game === 'pokemon-red')" :key="run.run_id" :value="run.run_id">{{ run.goal || 'No goal' }}</option>
          </datalist>
        </label>
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
            Runs tagged <strong>E2E</strong> are produced by the server. Old untagged Champion runs are listed as candidates, never automatically marked E2E.
          </p>
          <p v-if="recent.error.value" class="text-meta text-warn" role="alert">Farm history unavailable: {{ recent.error.value }}.</p>
          <p v-if="!foundSources.length" class="text-meta text-fg-2">No E2E or recent Champion runs found. Enter a run ID under Inspect a run.</p>
          <ul v-else class="max-h-72 space-y-1 overflow-y-auto" aria-label="Recent production test sources">
            <li v-for="run in foundSources" :key="run.run_id">
              <button type="button" class="flex w-full min-w-0 flex-wrap items-center gap-x-3 gap-y-1 border border-rule p-2 text-left hover:bg-raised"
                :class="inspectedID === run.run_id ? 'bg-selected' : ''" :aria-pressed="inspectedID === run.run_id" @click="inspectRun(run.run_id)">
                <span class="min-w-0 flex-1 truncate font-mono text-meta" :title="run.run_id">{{ run.run_id }}</span>
                <span class="text-meta text-fg-2">{{ run.status }}</span>
                <span class="text-meta" :class="run.purpose === 'full_e2e' ? 'text-ok font-semibold' : 'text-fg-3'">{{ run.purpose === 'full_e2e' ? 'E2E' : 'Champion candidate' }}</span>
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
              <dd>{{ production ? production.stage.replace(/_/g, ' ') : 'No status' }}<span v-if="production?.verdict"> · {{ production.verdict }}</span><span v-if="production?.error" class="text-warn"> · {{ production.error }}</span>
                <button v-if="productionFailed" type="button" class="ui-btn ui-btn-sm ml-2" :disabled="acting" @click="retryFailedProduction">Retry production</button></dd>
              <dt class="text-fg-3">Private upload</dt>
              <dd>
                <template v-if="Object.keys(production?.videos || {}).length">
                  <a v-for="(href, format) in production?.videos" :key="format" :href="href.startsWith('http') ? href : 'https://youtu.be/' + href" target="_blank" rel="noopener" class="mr-3 text-accent hover:underline">{{ format }} ↗</a>
                </template>
                <span v-else-if="production?.upload_private">Opted in · uploads after both edits pass the quality gate</span>
                <template v-else-if="canOptIn">
                  <span>Not requested</span>
                  <button v-if="!uploadOptIn" type="button" class="ui-btn ui-btn-sm ml-2" @click="uploadOptIn = true">Upload privately…</button>
                </template>
                <span v-else>Not requested</span>
              </dd>
              <dt class="text-fg-3">Replay</dt>
              <dd>{{ selectedEvidence.replay?.state || 'No status' }}<span v-if="selectedEvidence.replayError" class="text-warn"> · unavailable</span></dd>
              <dt class="text-fg-3">Publication</dt>
              <dd>{{ selectedEvidence.publications?.length ?? 'Unavailable' }} candidates<span v-if="selectedEvidence.publicationsError" class="text-warn"> · API error</span></dd>
            </dl>
            <div v-if="uploadOptIn && canOptIn" class="space-y-2 border border-rule-strong bg-surface p-2 text-meta">
              <label class="flex items-start gap-2">
                <input v-model="uploadAcknowledged" type="checkbox" class="mt-0.5" />
                <span>I confirm the server will upload two real videos to the configured YouTube account, both as <strong>private</strong>.</span>
              </label>
              <div class="flex gap-2">
                <button type="button" class="ui-btn ui-btn-sm ui-btn-primary" :disabled="!uploadAcknowledged || acting" @click="optIntoPrivateUpload">Opt in to private upload</button>
                <button type="button" class="ui-btn ui-btn-sm" @click="uploadOptIn = false">Cancel</button>
              </div>
            </div>
            <p v-if="actionError" class="text-meta text-warn" role="alert">{{ actionError }}</p>
            <div v-if="Object.keys(production?.plans || {}).length" class="flex flex-wrap gap-2 border-t border-rule pt-2">
              <a v-for="(hash, format) in production?.plans" :key="format" :href="studioEditVideoURL(inspectedID, hash)" target="_blank" rel="noopener noreferrer" class="ui-btn ui-btn-sm">Preview {{ format }}</a>
            </div>
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


    </div>
  </div>
</template>
