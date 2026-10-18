<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getDashboard, getModels, studioEditVideoURL } from '../shared/api/client'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { deploymentOptionLabel, deploymentSelectable, servesStrategist } from './llmDeployments'
import { beginFullE2E, fullE2E, stopFullE2E } from './fullE2EManager'
import { formatMediaHref, type MediaState } from './mediaQuery'

const props = defineProps<{ state: MediaState }>()
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
const recent = usePollingResource(signal => getDashboard({ status: 'done', limit: 60 }, signal), { intervalMs: 30000 })
const selections = computed(() => (models.data.value?.deployments || []).filter(d => servesStrategist(d) && deploymentSelectable(d)))
watch(selections, value => {
  if (!value.some(item => item.id === deployment.value)) deployment.value = value[0]?.id || ''
}, { immediate: true })
watch(() => props.state.run, id => {
  if (id && !working.value) {
    existingID.value = id
    mode.value = 'existing'
  }
})

const sourceLink = computed(() => currentID.value
  ? formatMediaHref({ ...props.state, tab: 'studio', run: currentID.value, job: '' }, window.location.search)
  : '')
const libraryLink = computed(() => currentID.value
  ? formatMediaHref({ ...props.state, tab: 'library', run: currentID.value, job: '' }, window.location.search)
  : '')
const canStart = computed(() => !working.value && (mode.value === 'new' ? Boolean(deployment.value && Number.isSafeInteger(seed.value) && seed.value >= 0)
  : Boolean(existingID.value.trim())) && (!uploadPrivate.value || uploadAcknowledged.value))
const outcome = computed(() => complete.value ? 'passed' : failure.value ? 'failed' : working.value ? 'running' : 'not_started')
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
          You can switch between PokePilot screens while the coordinator runs. Keep this browser page open: closing or reloading it stops further coordination (not already-dispatched server jobs). Reattach using the run ID to resume.
        </p>
      </div>
    </section>

    <div class="min-w-0 space-y-3">
      <section class="ui-bay" aria-labelledby="e2e-progress-title" aria-live="polite">
        <div class="ui-bay-header">
          <h2 id="e2e-progress-title">End-to-end progress</h2>
          <span class="text-meta text-fg-2">{{ stageCount }} stages ready</span>
        </div>
        <div class="space-y-3 p-3">
          <p v-if="currentID" class="min-w-0 break-all font-mono text-sm">{{ currentID }}</p>
          <div v-if="currentID" class="flex flex-wrap gap-2">
            <a :href="sourceLink" class="ui-btn ui-btn-sm">Open Studio</a>
            <a :href="libraryLink" class="ui-btn ui-btn-sm">Open recording</a>
            <a :href="'?run=' + encodeURIComponent(currentID) + '#live'" class="ui-btn ui-btn-sm">Inspect game</a>
          </div>
          <p v-if="!stages.length" class="py-5 text-center text-fg-2">Choose a source and start a test to see each verified step here.</p>
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
