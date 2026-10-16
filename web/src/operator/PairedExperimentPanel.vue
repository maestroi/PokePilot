<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { ArrowPathIcon, ClipboardDocumentIcon, PlayIcon, PlusIcon, XMarkIcon } from '@heroicons/vue/20/solid'
import { createExperiment, getExperiment, getExperiments, getModels } from '../shared/api/client'
import type { ExperimentView, ModelDeployment } from '../shared/api/types'
import { GOAL_OPTIONS } from '../shared/goals'
import ActionMenu, { type ActionMenuItem } from '../shared/components/ActionMenu.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { defaultExperimentArms, deploymentOptionLabel, strategistDeployments } from './llmDeployments'
import { parseExperimentSeeds, experimentProgress, presentExperimentStage } from './experimentPresentation'
import { copyReceipt } from './mediaQuery'
import { formatInteger, middleTruncate } from './runs'
import { operatorHref } from '../shared/urls'

const props = defineProps<{ experimentId?: string }>()
// The page header's "Start paired experiment" opens the setup form in the detail pane.
const creating = defineModel<boolean>('creating', { default: false })

const fieldClass = 'mt-1 block h-8 w-full rounded-[3px] border border-control bg-canvas px-2 text-fg placeholder:text-fg-3 disabled:text-fg-3 max-md:h-10'

const {
  data: modelsData,
  error: modelsError,
  state: modelsState,
  retry: retryModels
} = usePollingResource(
  (signal) => getModels(signal),
  { intervalMs: 5000, isEmpty: (snapshot) => snapshot.deployments.length === 0 }
)

const {
  data: experimentData,
  error: experimentError,
  state: experimentState,
  retry: retryExperiments
} = usePollingResource(
  (signal) => getExperiments(signal),
  { intervalMs: 5000, isEmpty: (experiments) => experiments.length === 0 }
)

// A selected detail endpoint is available even when the list is empty or stale.
const selectedResource = usePollingResource<ExperimentView | null>(
  (signal) => props.experimentId ? getExperiment(props.experimentId, signal) : Promise.resolve(null),
  { intervalMs: 5000 }
)
watch(() => props.experimentId, () => {
  showAllPairs.value = false
  void selectedResource.retry()
})

// Experiments compare strategists; decision-only deployments cannot be an arm.
const deployments = computed<ModelDeployment[]>(() => strategistDeployments(modelsData.value?.deployments ?? []))
const usableDeployments = computed(() => deployments.value.filter((deployment) =>
  deployment.enabled !== false && !['unavailable', 'failed'].includes(String(deployment.state || '').toLowerCase())
))
const history = computed<ExperimentView[]>(() => experimentData.value ?? [])
const listLoaded = computed(() => experimentState.value !== 'loading' && experimentState.value !== 'error' && experimentState.value !== 'empty')
const latest = computed<ExperimentView | null>(() => props.experimentId
  ? (selectedResource.data.value?.id === props.experimentId ? selectedResource.data.value : null)
    ?? history.value.find((item) => item.id === props.experimentId)
    ?? (created.value?.id === props.experimentId ? created.value : null)
  : null)
const paneOpen = computed(() => creating.value || Boolean(props.experimentId))
const showAllPairs = ref(false)
const pairRows = computed(() => latest.value?.pairs ?? [])
const visiblePairs = computed(() => showAllPairs.value ? pairRows.value : pairRows.value.slice(0, 30))
const progress = computed(() => experimentProgress(latest.value))
const detailState = computed(() => {
  if (latest.value) return (experimentState.value === 'stale' || selectedResource.state.value === 'stale') ? 'stale' : 'ready'
  return selectedResource.state.value === 'loading' || selectedResource.state.value === 'refreshing' ? 'loading' : selectedResource.state.value === 'error' ? 'error' : 'empty'
})
const detailError = computed(() => selectedResource.error.value || experimentError.value)

const form = reactive({
  name: 'Brock · 27B vs 4B',
  game: 'pokemon-red',
  arm_a: '',
  arm_b: '',
  arm_a_workers: 1,
  arm_b_workers: 1,
  goal: 'Earn the Boulder Badge.',
  starter: '',
  seed_count: 20,
  seeds: '',
  play_style: 'adventure',
  risk_tolerance: 'balanced',
  wild_encounters: 'planner',
  reasoning_effort: 'medium',
  fps: 0,
  max_rounds: 0,
  max_frames: 0,
  recovery_profile: 'resilient' as 'strict' | 'resilient'
})
const isYellow = computed(() => form.game === 'pokemon-yellow')

watch(usableDeployments, (next) => {
  const [armA, armB] = defaultExperimentArms(next)
  if (!next.some((deployment) => deployment.id === form.arm_a)) form.arm_a = armA
  if (!next.some((deployment) => deployment.id === form.arm_b) || form.arm_a === form.arm_b) form.arm_b = armB
}, { immediate: true })

const submitting = ref(false)
const error = ref('')
const created = ref<ExperimentView | null>(null)
const receipt = ref('')

function pct(value: number | undefined): string {
  return `${Math.round((Number(value) || 0) * 100)}%`
}

function seconds(value: number | undefined): string {
  return `${Number(value || 0).toFixed(2)}s`
}

function compactIdentity(value: string | undefined): string {
  if (!value) return 'missing'
  return value.length > 18 ? `${value.slice(0, 10)}…${value.slice(-6)}` : value
}

function deploymentIdentity(side: 'arm_a' | 'arm_b'): string {
  const identity = latest.value?.identity?.[side]
  if (!identity) return 'missing deployment identity'
  const revision = identity.revision || identity.artifact || 'revision unknown'
  return `${identity.model_id || identity.deployment_id} · ${revision} · ${identity.compute}`
}

function armsLabel(experiment: ExperimentView): string {
  return `${experiment.request?.arm_a?.deployment || 'unknown model'} vs ${experiment.request?.arm_b?.deployment || 'unknown model'}`
}

function resultLabel(experiment: ExperimentView): string {
  const paired = experiment.paired
  if (!paired?.comparable_pairs) return '—'
  return `A ${paired.a_wins || 0} · tie ${paired.ties || 0} · B ${paired.b_wins || 0}`
}

const excludedPairs = computed(() => latest.value?.pairs?.filter((pair) => !pair.comparable) ?? [])

function workerLimit(deploymentID: string): number {
  return Math.max(1, Number(deployments.value.find((deployment) => deployment.id === deploymentID)?.max_parallel_workers || 1))
}

// ---- Selection (URL-addressable via ?experiment=) ------------------------------------
const paneEl = ref<HTMLElement | null>(null)
function navigate(id: string): void {
  // A bare "#…" href would keep the old query, so clearing needs the pathname.
  const href = operatorHref('analytics', { section: 'experiments', experiment: id, run: '' }, window.location.search)
  window.history.pushState(null, '', href.startsWith('#') ? window.location.pathname + href : href)
  window.dispatchEvent(new PopStateEvent('popstate'))
}
function selectExperiment(id: string): void {
  creating.value = false
  if (id !== props.experimentId) navigate(id)
  void nextTick(() => paneEl.value?.focus())
}
function closePane(): void {
  const id = props.experimentId
  creating.value = false
  error.value = ''
  if (id) navigate('')
  void nextTick(() => document.querySelector<HTMLElement>(`[data-experiment-row="${CSS.escape(id || '')}"]`)?.focus())
}
watch(creating, (open) => { if (open) void nextTick(() => paneEl.value?.focus()) })
function onKeydown(event: KeyboardEvent): void {
  if (!paneOpen.value || event.key !== 'Escape' || event.metaKey || event.ctrlKey || event.altKey) return
  const target = event.target as HTMLElement | null
  if (target?.closest('input, select, textarea, [role="menu"]')) return
  event.preventDefault()
  closePane()
}
onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => window.removeEventListener('keydown', onKeydown))

function announce(text: string): void {
  receipt.value = text
  window.setTimeout(() => { if (receipt.value === text) receipt.value = '' }, 4000)
}
async function copy(value: string, noun: string): Promise<void> {
  announce(await copyReceipt(value, noun))
}
function membersHref(id: string): string {
  return operatorHref('runs', { experiment: id, run: '' }, window.location.search)
}
function menu(experiment: ExperimentView): ActionMenuItem[] {
  return [
    { label: 'Open member runs in Runs', href: membersHref(experiment.id) },
    { label: 'Copy experiment ID', select: () => copy(experiment.id, 'Experiment ID') }
  ]
}

async function submit(): Promise<void> {
  if (submitting.value) return
  error.value = ''
  if (!usableDeployments.value.some((deployment) => deployment.id === form.arm_a) || !usableDeployments.value.some((deployment) => deployment.id === form.arm_b) || form.arm_a === form.arm_b) {
    error.value = 'Choose two different deployments for Arm A and Arm B.'
    return
  }
  const armALimit = workerLimit(form.arm_a)
  const armBLimit = workerLimit(form.arm_b)
  if (!Number.isSafeInteger(form.arm_a_workers) || !Number.isSafeInteger(form.arm_b_workers) || Number(form.arm_a_workers) < 1 || Number(form.arm_a_workers) > armALimit || Number(form.arm_b_workers) < 1 || Number(form.arm_b_workers) > armBLimit) {
    error.value = `Worker count exceeds deployment capacity (A max ${armALimit}, B max ${armBLimit}).`
    return
  }
  const parsedSeeds = parseExperimentSeeds(form.seeds, form.seed_count)
  if (parsedSeeds.error) {
    error.value = parsedSeeds.error
    return
  }
  submitting.value = true
  try {
    const seeds = parsedSeeds.seeds
    created.value = await createExperiment({
      name: form.name.trim() || 'Brock · 27B vs 4B',
      arm_a: { name: 'A', deployment: form.arm_a, max_parallel_workers: Number(form.arm_a_workers || 1) },
      arm_b: { name: 'B', deployment: form.arm_b, max_parallel_workers: Number(form.arm_b_workers || 1) },
      game: form.game,
      goal: form.goal.trim() || 'Earn the Boulder Badge.',
      starter: isYellow.value ? 'pikachu' : form.starter,
      seeds,
      seed_count: seeds.length ? seeds.length : Number(form.seed_count || 20),
      play_style: form.play_style,
      risk_tolerance: form.risk_tolerance,
      wild_encounters: form.wild_encounters,
      reasoning_effort: form.reasoning_effort,
      fps: Number(form.fps || 0),
      max_rounds: Number(form.max_rounds || 0),
      max_frames: Number(form.max_frames || 0),
      recovery_profile: form.recovery_profile
    })
    creating.value = false
    showAllPairs.value = false
    if (created.value?.id) {
      window.history.pushState(null, '', operatorHref('analytics', { section: 'experiments', experiment: created.value.id, run: '' }, window.location.search))
      window.dispatchEvent(new PopStateEvent('popstate'))
      announce('Paired experiment queued')
    }
    // Refresh independently: a transient list failure must not turn a
    // successfully queued comparison into a false "creation failed" error.
    void retryExperiments()
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Unable to create experiment'
  } finally {
    submitting.value = false
  }
}

const metrics = computed(() => {
  const experiment = latest.value
  if (!experiment) return []
  const a = experiment.arm_a || {}
  const b = experiment.arm_b || {}
  const rows = [
    { label: 'Goal success', a: `${a.goal_successes || 0}/${a.done || 0} · ${pct(a.success_rate)}`, b: `${b.goal_successes || 0}/${b.done || 0} · ${pct(b.success_rate)}` }
  ]
  if ((experiment.request?.goal || '').toLowerCase().includes('boulder')) {
    rows.push({ label: 'Boulder Badge', a: `${a.boulder_successes || 0}/${a.done || 0}`, b: `${b.boulder_successes || 0}/${b.done || 0}` })
  }
  rows.push(
    { label: 'Completed comparable runs', a: `${a.done || 0}/${a.runs || 0}`, b: `${b.done || 0}/${b.runs || 0}` },
    { label: 'Median rounds / frames to goal', a: `${Number(a.median_rounds_to_goal || 0).toFixed(0)} / ${a.median_frames_to_goal || 0}`, b: `${Number(b.median_rounds_to_goal || 0).toFixed(0)} / ${b.median_frames_to_goal || 0}` },
    { label: 'Avg wall-clock run', a: seconds(a.avg_run_seconds), b: seconds(b.avg_run_seconds) },
    { label: 'Blackouts / objective failures', a: `${a.blackouts || 0} / ${a.objective_failures || 0}`, b: `${b.blackouts || 0} / ${b.objective_failures || 0}` },
    { label: 'Stagnation / plan-exhaustion replans', a: `${a.stagnation_replans || 0} / ${a.plan_exhaustion_replans || 0}`, b: `${b.stagnation_replans || 0} / ${b.plan_exhaustion_replans || 0}` },
    { label: 'Strategic calls / rejected', a: `${a.strategic_calls || 0} / ${a.strategic_rejected || 0}`, b: `${b.strategic_calls || 0} / ${b.strategic_rejected || 0}` },
    { label: 'Plan steps produced / executed / skipped', a: `${a.plan_steps_produced || 0} / ${a.plan_executions || 0} / ${a.steps_skipped || 0}`, b: `${b.plan_steps_produced || 0} / ${b.plan_executions || 0} / ${b.steps_skipped || 0}` },
    { label: 'Plan execution fraction', a: pct(a.plan_execution_fraction), b: pct(b.plan_execution_fraction) },
    { label: 'Strategic avg / p50 / p95', a: `${seconds(a.avg_strategic_call_seconds)} / ${seconds(a.p50_strategic_call_seconds)} / ${seconds(a.p95_strategic_call_seconds)}`, b: `${seconds(b.avg_strategic_call_seconds)} / ${seconds(b.p50_strategic_call_seconds)} / ${seconds(b.p95_strategic_call_seconds)}` },
    { label: 'Planner time', a: seconds(a.strategic_seconds), b: seconds(b.strategic_seconds) },
    { label: 'Prefill / decode TPS', a: `${Number(a.avg_prefill_tps || 0).toFixed(1)} / ${Number(a.avg_decode_tps || 0).toFixed(1)}`, b: `${Number(b.avg_prefill_tps || 0).toFixed(1)} / ${Number(b.avg_decode_tps || 0).toFixed(1)}` },
    { label: 'Prompt / completion tokens', a: `${a.prompt_tokens || 0} / ${a.completion_tokens || 0}`, b: `${b.prompt_tokens || 0} / ${b.completion_tokens || 0}` },
    { label: 'Reply rejects / transport / fallback', a: `${a.rejected || 0} / ${a.transport_errors || 0} / ${a.fallbacks || 0}`, b: `${b.rejected || 0} / ${b.transport_errors || 0} / ${b.fallbacks || 0}` }
  )
  return rows
})
function operatorSearch(): string { return window.location.search }
</script>

<template>
  <div :class="paneOpen ? 'xl:grid xl:grid-cols-[minmax(0,1fr)_400px] xl:gap-3 min-[1440px]:grid-cols-[minmax(0,1fr)_440px]' : ''">
    <!-- Paired experiments list -->
    <section class="ui-bay min-w-0 xl:self-start" aria-labelledby="paired-title">
      <div class="ui-bay-header">
        <h2 id="paired-title" class="flex items-center gap-2">Paired experiments <span class="ui-tag font-normal">Paired · matched seeds</span></h2>
        <span class="flex items-center gap-2 text-meta font-normal text-fg-2">
          <span v-if="history.length">{{ formatInteger(history.length) }} · newest first</span>
          <button type="button" class="ui-btn ui-btn-sm ui-icon-btn max-md:size-10" aria-label="Refresh experiments" title="Refresh" @click="retryExperiments()">
            <ArrowPathIcon class="size-4" aria-hidden="true" />
          </button>
        </span>
      </div>

      <ResourceState v-if="experimentState === 'stale'" state="stale" class="p-3" title="Showing the last loaded experiments" :message="`Refresh failed: ${experimentError}`">
        <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="retryExperiments()">Retry</button></template>
      </ResourceState>

      <!-- Desktop / tablet table -->
      <div class="hidden md:block" role="region" aria-label="Paired experiments table">
        <table class="w-full table-fixed border-collapse text-left">
          <thead>
            <tr class="h-9 border-b border-rule-strong bg-raised text-meta text-fg-2">
              <th class="w-52 px-3 font-medium">Status</th>
              <th class="px-3 font-medium">Experiment</th>
              <th class="w-44 px-3 font-medium" :class="paneOpen ? 'hidden min-[1680px]:table-cell' : 'max-lg:hidden'">Pairs</th>
              <th class="w-44 px-3 font-medium" :class="paneOpen ? 'hidden min-[1680px]:table-cell' : 'max-xl:hidden'">Result</th>
              <th class="w-11 pr-2"><span class="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody v-if="experimentState === 'loading'" aria-busy="true">
            <tr v-for="n in 4" :key="n" class="h-14 border-b border-rule">
              <td colspan="5" class="px-3"><div class="h-4 bg-raised motion-safe:animate-pulse" :class="n % 2 ? 'w-3/4' : 'w-1/2'" /></td>
            </tr>
          </tbody>
          <tbody v-else-if="listLoaded">
            <tr
              v-for="experiment in history"
              :key="experiment.id"
              :data-experiment-row="experiment.id"
              tabindex="0"
              :aria-selected="experiment.id === experimentId"
              :class="experiment.id === experimentId ? 'bg-selected shadow-[inset_2px_0_0_var(--color-accent)]' : 'hover:bg-raised'"
              class="h-14 cursor-pointer border-b border-rule align-middle focus-visible:outline-offset-[-2px]"
              @click="selectExperiment(experiment.id)"
              @keydown.enter.self="selectExperiment(experiment.id)"
            >
              <td class="px-3"><StatusPill :status="presentExperimentStage(experiment)" class="max-w-full overflow-hidden" /></td>
              <td class="px-3">
                <a
                  :href="operatorHref('analytics', { section: 'experiments', experiment: experiment.id }, operatorSearch())"
                  class="block truncate font-semibold hover:underline"
                  :title="experiment.name || experiment.id"
                  @click.prevent
                >{{ experiment.name || experiment.id }}</a>
                <div class="truncate text-meta text-fg-2" :title="armsLabel(experiment)">{{ armsLabel(experiment) }}</div>
              </td>
              <td class="px-3" :class="paneOpen ? 'hidden min-[1680px]:table-cell' : 'max-lg:hidden'">
                <div class="font-mono text-meta text-fg-2">{{ experimentProgress(experiment).completed }}/{{ experimentProgress(experiment).total }} done</div>
                <div class="ui-meter mt-1.5"><span class="bg-accent" :style="{ width: `${experimentProgress(experiment).total ? 100 * experimentProgress(experiment).completed / experimentProgress(experiment).total : 0}%` }" /></div>
              </td>
              <td class="px-3 font-mono text-meta" :class="paneOpen ? 'hidden min-[1680px]:table-cell' : 'max-xl:hidden'">{{ resultLabel(experiment) }}</td>
              <td class="pr-2 text-right" @click.stop @keydown.stop>
                <ActionMenu :label="`More actions for experiment ${experiment.name || experiment.id}`" :items="menu(experiment)" />
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Narrow list (< 768px) -->
      <div class="md:hidden">
        <div v-if="experimentState === 'loading'" class="space-y-4 p-3" aria-busy="true">
          <div v-for="n in 3" :key="n" class="h-12 bg-raised motion-safe:animate-pulse" />
        </div>
        <ul v-else-if="listLoaded" role="list" aria-label="Paired experiments">
          <li
            v-for="experiment in history"
            :key="experiment.id"
            :class="experiment.id === experimentId ? 'bg-selected shadow-[inset_2px_0_0_var(--color-accent)]' : ''"
            class="flex gap-3 border-b border-rule px-3 py-2.5"
          >
            <div class="min-w-0 flex-1">
              <div class="flex min-w-0 items-center gap-2">
                <StatusPill :status="presentExperimentStage(experiment)" class="shrink-0" />
                <span class="ml-auto shrink-0 font-mono text-meta text-fg-2">{{ experimentProgress(experiment).completed }}/{{ experimentProgress(experiment).total }} done</span>
              </div>
              <button type="button" class="mt-1 block w-full truncate text-left font-semibold text-accent" @click="selectExperiment(experiment.id)">{{ experiment.name || experiment.id }}</button>
              <div class="mt-0.5 truncate text-meta text-fg-2">{{ armsLabel(experiment) }}</div>
            </div>
            <ActionMenu class="shrink-0" :label="`More actions for experiment ${experiment.name || experiment.id}`" :items="menu(experiment)" />
          </li>
        </ul>
      </div>

      <ResourceState v-if="experimentState === 'error'" state="error" class="m-3" title="Couldn’t load paired experiments" :message="`${experimentError}. A linked experiment still opens in the pane.`">
        <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="retryExperiments()">Retry</button></template>
      </ResourceState>
      <div v-else-if="experimentState === 'empty'" class="flex flex-col items-center gap-2 px-4 py-12 text-center">
        <span class="text-[20px] leading-5 text-fg-3" aria-hidden="true">○</span>
        <p class="text-title font-semibold">No paired experiments yet</p>
        <p class="text-fg-2">A paired experiment runs the same seeds on two models so their results compare fairly.</p>
        <button type="button" class="ui-btn mt-1 max-md:h-10" @click="creating = true"><PlusIcon class="size-4" aria-hidden="true" /> Start paired experiment</button>
      </div>
    </section>

    <!-- Detail pane: docked ≥ 1280px, overlay sheet below, full screen < 768px -->
    <template v-if="paneOpen">
      <div class="fixed inset-0 z-40 bg-black/50 xl:hidden" aria-hidden="true" @click="closePane" />
      <aside
        ref="paneEl"
        tabindex="-1"
        :aria-label="creating ? 'Start paired experiment' : 'Experiment details'"
        class="fixed inset-0 z-50 border-rule-strong shadow-[0_8px_24px_rgb(0_0_0/45%)] outline-none md:left-auto md:w-[440px] md:border-l xl:sticky xl:top-14 xl:z-auto xl:h-[calc(100vh-168px)] xl:self-start xl:w-auto xl:border xl:shadow-none"
      >
        <!-- Setup form -->
        <form v-if="creating" class="flex h-full min-h-0 flex-col bg-surface" aria-labelledby="create-title" @submit.prevent="submit">
          <header class="flex min-h-12 shrink-0 items-center gap-2 border-b border-rule bg-raised px-3 py-2">
            <h2 id="create-title" class="min-w-0 truncate text-title font-semibold">Start paired experiment</h2>
            <button type="button" class="ui-btn ui-btn-sm ui-icon-btn ml-auto shrink-0" aria-label="Close setup" title="Close (Esc)" @click="closePane">
              <XMarkIcon class="size-4" aria-hidden="true" />
            </button>
          </header>

          <div v-if="usableDeployments.length < 2" class="flex flex-1 flex-col items-start gap-3 p-4" role="status">
            <p v-if="modelsState === 'loading'" class="text-fg-2">Loading configured models…</p>
            <template v-else>
              <p class="text-title">{{ modelsError ? 'Model registry unavailable' : 'Two strategist models are needed' }}</p>
              <p class="text-fg-2">{{ modelsError || 'At least two enabled, available strategist deployments are required to start a comparison.' }}</p>
              <div class="flex gap-2">
                <a href="#settings/models" class="ui-btn">Configure models</a>
                <button type="button" class="ui-btn ui-btn-quiet" @click="retryModels()">Retry models</button>
              </div>
            </template>
          </div>

          <template v-else>
            <div class="min-h-0 flex-1 overflow-y-auto px-3 pb-4">
              <p class="mt-3 text-meta text-fg-2">Both arms get the same seeds, game, policies and concurrency rules. Each seed queues one run per arm.</p>

              <fieldset class="mt-3">
                <legend class="mb-1 text-title font-semibold">Arms</legend>
                <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <label class="block">
                    <span class="ui-label">Arm A</span>
                    <select v-model="form.arm_a" :class="fieldClass">
                      <option v-for="deployment in usableDeployments" :key="`a-${deployment.id}`" :value="deployment.id">{{ deploymentOptionLabel(deployment) }}</option>
                    </select>
                  </label>
                  <label class="block">
                    <span class="ui-label">Arm B</span>
                    <select v-model="form.arm_b" :class="fieldClass">
                      <option v-for="deployment in usableDeployments" :key="`b-${deployment.id}`" :value="deployment.id">{{ deploymentOptionLabel(deployment) }}</option>
                    </select>
                  </label>
                  <label class="block">
                    <span class="ui-label">Arm A workers</span>
                    <input v-model.number="form.arm_a_workers" type="number" min="1" :max="workerLimit(form.arm_a)" :class="fieldClass" />
                  </label>
                  <label class="block">
                    <span class="ui-label">Arm B workers</span>
                    <input v-model.number="form.arm_b_workers" type="number" min="1" :max="workerLimit(form.arm_b)" :class="fieldClass" />
                  </label>
                </div>
                <p class="mt-1 text-meta text-fg-2">1 vs 1 is the fair default and keeps per-run model speed at maximum. Runs above a cap stay queued.</p>
              </fieldset>

              <fieldset class="mt-5">
                <legend class="mb-1 text-title font-semibold">Benchmark</legend>
                <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <label class="block sm:col-span-2">
                    <span class="ui-label">Name</span>
                    <input v-model="form.name" :class="fieldClass" />
                  </label>
                  <label class="block">
                    <span class="ui-label">Game</span>
                    <select v-model="form.game" :class="fieldClass">
                      <option value="pokemon-red">Pokémon Red</option>
                      <option value="pokemon-blue">Pokémon Blue</option>
                      <option value="pokemon-yellow">Pokémon Yellow</option>
                    </select>
                  </label>
                  <label class="block">
                    <span class="ui-label">Starter</span>
                    <input v-if="isYellow" value="Pikachu · scripted" disabled :class="fieldClass" />
                    <select v-else v-model="form.starter" :class="fieldClass">
                      <option value="">Let LLM decide</option>
                      <option value="squirtle">Squirtle</option>
                      <option value="charmander">Charmander</option>
                      <option value="bulbasaur">Bulbasaur</option>
                    </select>
                  </label>
                  <label class="block sm:col-span-2">
                    <span class="ui-label">Goal</span>
                    <select v-model="form.goal" :class="fieldClass">
                      <option v-for="goal in GOAL_OPTIONS" :key="goal || 'free'" :value="goal">{{ goal || 'Free play (no automatic stop)' }}</option>
                    </select>
                  </label>
                  <label class="block">
                    <span class="ui-label">Seed count</span>
                    <input v-model.number="form.seed_count" type="number" min="1" max="1000" :class="fieldClass" />
                  </label>
                  <label class="block">
                    <span class="ui-label">Seed list</span>
                    <input v-model="form.seeds" placeholder="Optional: 1, 7, 42" :class="fieldClass" />
                  </label>
                </div>
                <p class="mt-1 text-meta text-fg-2">Both arms run the same cartridge. The goal stays independent from starter and play style.</p>
              </fieldset>

              <fieldset class="mt-5">
                <legend class="mb-1 text-title font-semibold">Policy</legend>
                <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <label class="block">
                    <span class="ui-label">Play style</span>
                    <select v-model="form.play_style" :class="fieldClass">
                      <option value="adventure">Adventure</option>
                      <option value="speedrun">Speedrun</option>
                      <option value="completionist">Completionist</option>
                      <option value="team_builder">Team Builder</option>
                    </select>
                  </label>
                  <label class="block">
                    <span class="ui-label">Risk tolerance</span>
                    <select v-model="form.risk_tolerance" :class="fieldClass">
                      <option value="balanced">Balanced</option>
                      <option value="aggressive">Aggressive</option>
                      <option value="cautious">Cautious</option>
                    </select>
                  </label>
                  <label class="block">
                    <span class="ui-label">Wild encounters</span>
                    <select v-model="form.wild_encounters" :class="fieldClass">
                      <option value="planner">Planner decides</option>
                      <option value="fight">Fight every encounter</option>
                    </select>
                  </label>
                  <label class="block">
                    <span class="ui-label">Reasoning effort</span>
                    <select v-model="form.reasoning_effort" :class="fieldClass">
                      <option value="">Endpoint default</option>
                      <option value="off">Off</option>
                      <option value="low">Low</option>
                      <option value="medium">Medium</option>
                      <option value="high">High</option>
                    </select>
                  </label>
                  <label class="block sm:col-span-2">
                    <span class="ui-label">Recovery</span>
                    <select v-model="form.recovery_profile" :class="fieldClass">
                      <option value="resilient">Resilient · measure recovery burden</option>
                      <option value="strict">Strict · fail on bounded recovery</option>
                    </select>
                  </label>
                </div>
                <p class="mt-1 text-meta text-fg-2">Both arms use the same recovery policy, so weaker models still compare by time and recoveries to goal.</p>
              </fieldset>

              <fieldset class="mt-5">
                <legend class="mb-1 text-title font-semibold">Limits</legend>
                <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
                  <label class="block">
                    <span class="ui-label">FPS</span>
                    <select v-model.number="form.fps" :class="fieldClass">
                      <option :value="0">Max · uncapped</option>
                      <option :value="60">1× · 60 FPS</option>
                      <option :value="120">2× · 120 FPS</option>
                      <option :value="240">4× · 240 FPS</option>
                    </select>
                  </label>
                  <label class="block">
                    <span class="ui-label">Round cap</span>
                    <input v-model.number="form.max_rounds" type="number" min="0" :class="fieldClass" />
                  </label>
                  <label class="block">
                    <span class="ui-label">Frame cap</span>
                    <input v-model.number="form.max_frames" type="number" min="0" :class="fieldClass" />
                  </label>
                </div>
                <p class="mt-1 text-meta text-fg-2">0 means no cap.</p>
              </fieldset>
            </div>
            <footer class="shrink-0 border-t border-rule px-3 py-2">
              <p v-if="error" class="mb-2 border border-danger/40 bg-danger/10 px-3 py-2 text-danger" role="alert">✕ {{ error }}</p>
              <div class="flex flex-wrap items-center gap-2">
                <button type="submit" :disabled="submitting" :aria-busy="submitting" class="ui-btn ui-btn-primary max-md:h-10">
                  <PlayIcon class="size-4" aria-hidden="true" />
                  {{ submitting ? 'Queueing pairs…' : 'Start paired experiment' }}
                </button>
                <button type="button" class="ui-btn ui-btn-quiet max-md:h-10" @click="closePane">Cancel</button>
              </div>
            </footer>
          </template>
        </form>

        <!-- Selected experiment -->
        <section v-else class="flex h-full min-h-0 flex-col bg-surface" aria-labelledby="experiment-pane-title">
          <header class="flex min-h-12 shrink-0 items-center gap-2 border-b border-rule bg-raised px-3 py-2">
            <div class="min-w-0">
              <h2 id="experiment-pane-title" class="truncate text-title font-semibold" :title="latest?.name || experimentId">{{ latest?.name || experimentId }}</h2>
              <div class="flex min-w-0 items-center gap-1 text-meta text-fg-2">
                <span class="truncate font-mono" :title="experimentId">{{ middleTruncate(experimentId || '', 14, 6) }}</span>
                <button type="button" class="ui-btn ui-btn-sm ui-icon-btn size-6 shrink-0" aria-label="Copy experiment ID" title="Copy experiment ID" @click="copy(experimentId || '', 'Experiment ID')">
                  <ClipboardDocumentIcon class="size-4" aria-hidden="true" />
                </button>
              </div>
            </div>
            <StatusPill v-if="latest" :status="presentExperimentStage(latest)" class="ml-auto shrink-0" />
            <button type="button" class="ui-btn ui-btn-sm ui-icon-btn shrink-0" :class="latest ? '' : 'ml-auto'" aria-label="Close experiment details" title="Close (Esc)" @click="closePane">
              <XMarkIcon class="size-4" aria-hidden="true" />
            </button>
          </header>

          <div v-if="!latest && detailState === 'loading'" class="flex-1 space-y-3 p-4" aria-busy="true" aria-label="Loading experiment">
            <div v-for="n in 6" :key="n" class="h-4 bg-raised motion-safe:animate-pulse" :class="n % 3 === 0 ? 'w-2/3' : 'w-full'" />
          </div>
          <div v-else-if="!latest" class="flex flex-1 flex-col items-start gap-3 p-4" role="status">
            <p class="text-title">Selected experiment unavailable</p>
            <p class="text-fg-2">{{ detailError || 'This experiment no longer exists or could not be retrieved. Select another comparison or retry.' }}</p>
            <div class="flex gap-2">
              <button type="button" class="ui-btn" @click="retryExperiments(); selectedResource.retry()">Retry</button>
              <button type="button" class="ui-btn ui-btn-quiet" @click="closePane">Close</button>
            </div>
          </div>

          <template v-else>
            <div class="min-h-0 flex-1 overflow-y-auto px-3 pb-4">
              <ResourceState v-if="detailState === 'stale'" state="stale" class="mt-3" title="Showing the last loaded results" :message="detailError || 'Refresh failed.'" />

              <h3 class="mt-3 mb-1 text-title font-semibold">Progress</h3>
              <div class="flex items-center gap-3">
                <div class="ui-meter" role="progressbar" :aria-valuenow="progress.completed" :aria-valuemin="0" :aria-valuemax="Math.max(1, progress.total)" aria-label="Completed matched pairs">
                  <span class="bg-accent" :style="{ width: progress.total ? (100 * progress.completed / progress.total) + '%' : '0%' }" />
                </div>
                <span class="font-mono text-meta text-fg-2">{{ progress.completed }} / {{ progress.total }} pairs</span>
              </div>
              <p class="mt-1 text-meta text-fg-2">{{ progress.inProgress }} in progress · {{ progress.excluded }} excluded</p>

              <h3 class="mt-5 mb-1 text-title font-semibold">Result</h3>
              <dl class="grid grid-cols-[1.4fr_1fr_1fr_1fr_1fr] gap-px border border-rule bg-rule text-center">
                <div v-for="item in [
                  ['Comparable', latest.paired?.comparable_pairs || 0],
                  ['A wins', latest.paired?.a_wins || 0],
                  ['Ties', latest.paired?.ties || 0],
                  ['B wins', latest.paired?.b_wins || 0],
                  ['Excluded', latest.paired?.excluded_pairs || 0]
                ]" :key="item[0]" class="bg-surface px-1 py-2">
                  <dt class="ui-label">{{ item[0] }}</dt>
                  <dd class="m-0 font-mono font-medium" :class="item[0] === 'Excluded' && item[1] ? 'text-warn' : ''">{{ item[1] }}</dd>
                </div>
              </dl>
              <div v-if="excludedPairs.length" class="mt-2 border border-warn/40 bg-warn/10 px-3 py-2" role="status">
                <p class="text-warn">▲ Excluded non-comparable seeds</p>
                <ul class="mt-1 space-y-0.5 font-mono text-meta text-fg-2">
                  <li v-for="pair in excludedPairs" :key="pair.seed">seed {{ pair.seed }} · {{ pair.non_comparable_reason || 'comparability identity differs' }}</li>
                </ul>
                <p class="mt-1 text-meta text-fg-2">Excluded pairs never contribute to the aggregate metrics above.</p>
              </div>

              <h3 class="mt-5 mb-1 text-title font-semibold">Arm comparison</h3>
              <table class="w-full table-fixed border-collapse text-left">
                <thead>
                  <tr class="h-8 border-b border-rule-strong text-meta text-fg-2">
                    <th class="font-medium">Metric</th>
                    <th class="w-[30%] font-medium">Arm A</th>
                    <th class="w-[30%] font-medium">Arm B</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="metric in metrics" :key="metric.label" class="border-b border-rule align-top">
                    <td class="py-1.5 pr-2 text-meta text-fg-2">{{ metric.label }}</td>
                    <td class="py-1.5 pr-2 font-mono text-meta break-words">{{ metric.a }}</td>
                    <td class="py-1.5 font-mono text-meta break-words">{{ metric.b }}</td>
                  </tr>
                </tbody>
              </table>

              <h3 class="mt-5 mb-1 text-title font-semibold">Setup</h3>
              <dl class="ui-kv">
                <dt>Arm A</dt><dd>{{ latest.request?.arm_a?.deployment || 'unknown model' }} <span class="text-meta text-fg-2">· {{ latest.request?.arm_a?.max_parallel_workers || 1 }} workers</span></dd>
                <dt>Arm B</dt><dd>{{ latest.request?.arm_b?.deployment || 'unknown model' }} <span class="text-meta text-fg-2">· {{ latest.request?.arm_b?.max_parallel_workers || 1 }} workers</span></dd>
                <dt>Goal</dt><dd>{{ latest.request?.goal || 'No goal reported' }}</dd>
                <dt>Matched policy</dt><dd>{{ latest.request?.play_style || '—' }} · {{ latest.request?.risk_tolerance || '—' }} · {{ latest.request?.wild_encounters || '—' }}</dd>
                <dt>Recovery</dt><dd>{{ latest.request?.recovery_profile || '—' }}</dd>
                <dt>Reasoning effort</dt><dd>{{ latest.request?.reasoning_effort || 'Endpoint default' }}</dd>
                <template v-if="latest.created_at"><dt>Created</dt><dd class="font-mono text-meta">{{ latest.created_at }}</dd></template>
              </dl>

              <h3 class="mt-5 mb-1 text-title font-semibold">Benchmark identity</h3>
              <dl class="ui-kv">
                <dt>Game</dt><dd>{{ latest.identity?.game || latest.request?.game || 'game missing' }}</dd>
                <dt>Git revision</dt><dd class="font-mono text-meta" :title="latest.identity?.git_revision">{{ compactIdentity(latest.identity?.git_revision) }}</dd>
                <dt>ROM</dt><dd class="font-mono text-meta" :title="latest.identity?.rom_identity">{{ compactIdentity(latest.identity?.rom_identity) }}</dd>
                <dt>Prompt</dt><dd class="font-mono text-meta" :title="latest.identity?.prompt_identity">{{ compactIdentity(latest.identity?.prompt_identity) }}</dd>
                <dt>Arm A model</dt><dd class="text-meta">{{ deploymentIdentity('arm_a') }}</dd>
                <dt>Arm B model</dt><dd class="text-meta">{{ deploymentIdentity('arm_b') }}</dd>
              </dl>

              <template v-if="pairRows.length">
                <h3 class="mt-5 mb-1 flex items-baseline justify-between text-title font-semibold">Matched seeds <span class="text-meta font-normal text-fg-2">{{ pairRows.length }} pairs</span></h3>
                <table class="w-full table-fixed border-collapse text-left">
                  <thead>
                    <tr class="h-8 border-b border-rule-strong text-meta text-fg-2">
                      <th class="w-16 font-medium">Seed</th>
                      <th class="font-medium">Arm A</th>
                      <th class="font-medium">Arm B</th>
                      <th class="w-[34%] font-medium">Comparison</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="pair in visiblePairs" :key="pair.seed" class="h-8 border-b border-rule text-meta">
                      <td class="font-mono">{{ pair.seed }}</td>
                      <td class="truncate pr-2">
                        <a v-if="pair.run_id_a" :href="operatorHref('live', { run: pair.run_id_a, experiment: latest.id }, operatorSearch())" class="text-accent hover:underline" :title="pair.run_id_a">A · {{ pair.status_a || 'queued' }}</a>
                        <span v-else class="text-fg-2">A · {{ pair.status_a || 'unavailable' }}</span>
                      </td>
                      <td class="truncate pr-2">
                        <a v-if="pair.run_id_b" :href="operatorHref('live', { run: pair.run_id_b, experiment: latest.id }, operatorSearch())" class="text-accent hover:underline" :title="pair.run_id_b">B · {{ pair.status_b || 'queued' }}</a>
                        <span v-else class="text-fg-2">B · {{ pair.status_b || 'unavailable' }}</span>
                      </td>
                      <td class="truncate" :class="pair.comparable ? '' : 'text-warn'" :title="pair.non_comparable_reason || ''">
                        {{ !pair.comparable ? (pair.non_comparable_reason || 'Excluded') : pair.winner ? ('Winner: ' + (pair.winner === 'tie' ? 'Tie' : pair.winner.toUpperCase())) : 'In progress' }}
                      </td>
                    </tr>
                  </tbody>
                </table>
                <button v-if="pairRows.length > 30" type="button" class="ui-btn ui-btn-sm ui-btn-quiet mt-1" @click="showAllPairs = !showAllPairs">
                  {{ showAllPairs ? 'Show first 30 pairs' : 'Show all ' + pairRows.length + ' pairs' }}
                </button>
              </template>
            </div>
            <footer class="flex shrink-0 flex-wrap items-center gap-2 border-t border-rule px-3 py-2">
              <a :href="membersHref(latest.id)" class="ui-btn max-md:h-10">Open member runs in Runs</a>
            </footer>
          </template>
        </section>
      </aside>
    </template>

    <div class="pointer-events-none fixed right-4 bottom-4 z-[80]" aria-live="polite" role="status">
      <div v-if="receipt" class="pointer-events-auto rounded-md border border-rule-strong bg-raised px-4 py-2.5 shadow-[0_8px_24px_rgb(0_0_0/45%)]">{{ receipt }}</div>
    </div>
  </div>
</template>
