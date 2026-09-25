<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ArrowRightIcon, PlayIcon } from '@heroicons/vue/20/solid'
import { createRun, getModels } from '../shared/api/client'
import type { DecisionEngineSpec, ModelDeployment, RunSpec } from '../shared/api/types'
import { GOAL_OPTIONS } from '../shared/goals'
import { defaultGoalForPlayStyle } from '../shared/playstyle'
import Panel from '../shared/components/Panel.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { defaultFarmDeployment, deploymentOptionLabel, deploymentSelectable, preferredDeployment, servesStrategist } from './llmDeployments'

type StarterMode =
  | 'default'
  | 'squirtle'
  | 'charmander'
  | 'bulbasaur'
  | 'random'
  | 'random:basic'
  | 'random:any'
  | 'mew'
  | 'mewtwo'
  | 'specific'

const form = reactive<RunSpec>({
  run_id: '',
  seed: 0,
  game: 'pokemon-red',
  planner: 'llm',
  starter: '',
  dest: '',
  goal: defaultGoalForPlayStyle('adventure'),
  llm_profile: 'auto',
  llm_deployment: '',
  play_style: 'adventure',
  purpose: 'normal',
  risk_tolerance: 'balanced',
  wild_encounters: 'planner',
  reasoning_effort: '',
  fps: 60,
  max_rounds: 0,
  max_frames: 0,
  recovery_profile: 'resilient',
  endless: false,
  random_seed: false
})

// The fast typed-decision engine is chosen independently of the strategist,
// from the same model registry. decisionTarget is 'off', 'deployment:<id>',
// or (only when no registry is configured) 'env:<backend>', which uses the
// runner's own decision endpoint. Off sends no selection at all.
const decisionTarget = ref('off')
const decision = reactive({
  mode: 'shadow' as 'shadow' | 'active',
  battles: true,
  objectives: false,
  failures: true,
  min_confidence: 0.65
})
const decisionSelected = computed(() => decisionTarget.value !== 'off')
const decisionShadow = computed(() => decision.mode === 'shadow')

function decisionRequest(): DecisionEngineSpec | undefined {
  if (!isLLM.value || !decisionSelected.value) return undefined
  const [kind, id] = splitDecisionTarget(decisionTarget.value)
  const target: Pick<DecisionEngineSpec, 'backend' | 'deployment'> = kind === 'deployment'
    ? { backend: decisionBackendFor(deployments.value.find((d) => d.id === id)), deployment: id }
    : { backend: id as DecisionEngineSpec['backend'] }
  // Battle decisions are observational only; active runs never send them.
  return { ...target, ...decision, battles: decisionShadow.value && decision.battles }
}

function splitDecisionTarget(target: string): [string, string] {
  const at = target.indexOf(':')
  return at < 0 ? [target, ''] : [target.slice(0, at), target.slice(at + 1)]
}

function decisionBackendFor(deployment: ModelDeployment | undefined): DecisionEngineSpec['backend'] {
  return deployment?.protocol === 'typesafe-choice' ? 'jev' : 'system-one'
}

const { data: modelsData } = usePollingResource(
  (signal) => getModels(signal),
  { intervalMs: 5000, isEmpty: (snapshot) => snapshot.deployments.length === 0 }
)
const deployments = computed<ModelDeployment[]>(() => modelsData.value?.deployments ?? [])
const strategists = computed(() => deployments.value.filter(servesStrategist))
const hasDeployments = computed(() => strategists.value.length > 0)
// Any registered deployment can back the decision engine: a choice API like
// Jev, or an OpenAI-compatible model used through typed choices.
const decisionDeployments = computed(() => deployments.value.filter((d) => d.enabled !== false))
const selectedDecisionDeployment = computed(() => {
  const [kind, id] = splitDecisionTarget(decisionTarget.value)
  return kind === 'deployment' ? deployments.value.find((d) => d.id === id) : undefined
})
const selectedDeployment = computed(() => deployments.value.find((deployment) => deployment.id === form.llm_deployment))

watch(deployments, (next) => {
  const [kind, id] = splitDecisionTarget(decisionTarget.value)
  if (kind === 'deployment' && !next.some((d) => d.id === id && d.enabled !== false)) decisionTarget.value = 'off'
  if (!next.length) {
    form.llm_deployment = ''
    return
  }
  form.llm_deployment = preferredDeployment(next, form.llm_deployment || defaultFarmDeployment(next))
}, { immediate: true })

const submitting = ref(false)
const error = ref('')
const createdRunID = ref('')
const starterMode = ref<StarterMode>('default')
const specificStarter = ref('')
const isLLM = computed(() => form.planner === 'llm')
const isSpecificStarter = computed(() => starterMode.value === 'specific')

function starterRequest(): string {
  if (starterMode.value === 'specific') return specificStarter.value.trim()
  if (starterMode.value === 'default') return isLLM.value ? '' : 'squirtle'
  return starterMode.value
}

function runURL(runID: string): string {
  const url = new URL('/', window.location.origin)
  url.searchParams.set('run', runID)
  url.hash = 'live'
  return url.toString()
}

async function submit(): Promise<void> {
  if (submitting.value) return
  error.value = ''
  if (isSpecificStarter.value && !specificStarter.value.trim()) {
    error.value = 'Enter the Gen I Pokémon you want to use as the starter.'
    return
  }

  submitting.value = true
  createdRunID.value = ''
  try {
    const spec: RunSpec = {
      ...form,
      run_id: form.run_id.trim(),
      game: form.game,
      starter: starterRequest(),
      dest: isLLM.value ? '' : form.dest.trim(),
      goal: isLLM.value ? form.goal.trim() : '',
      llm_profile: isLLM.value && !hasDeployments.value ? form.llm_profile : '',
      llm_deployment: isLLM.value && hasDeployments.value ? form.llm_deployment : undefined,
      play_style: isLLM.value ? form.play_style : '',
      purpose: isLLM.value ? form.purpose : '',
      risk_tolerance: isLLM.value ? form.risk_tolerance : '',
      wild_encounters: isLLM.value ? form.wild_encounters : '',
      reasoning_effort: isLLM.value ? form.reasoning_effort : '',
      decision_engine: decisionRequest(),
      recovery_profile: isLLM.value ? form.recovery_profile : 'strict'
    }
    const response = await createRun(spec)
    const returnedID = typeof response.run_id === 'string' ? response.run_id : ''
    createdRunID.value = returnedID || spec.run_id
    if (!createdRunID.value) {
      error.value = 'Run was queued, but the wall did not return a run id.'
    }
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : 'Unable to create run'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="grid grid-cols-1 gap-3 2xl:grid-cols-[minmax(0,2fr)_minmax(18rem,1fr)]">
    <Panel title="New run" description="Queue a scripted walk or goal-driven LLM run without leaving the console." compact>
      <form class="grid grid-cols-1 gap-4 sm:grid-cols-2" @submit.prevent="submit">
        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Run id</span>
          <input v-model="form.run_id" placeholder="Leave blank to generate one" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" />
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Mode</span>
          <select v-model="form.planner" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option value="llm">Play the game</option>
            <option value="scripted">Walk to a place</option>
          </select>
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Game</span>
          <select v-model="form.game" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option value="pokemon-red">Pokémon Red</option>
            <option value="pokemon-blue">Pokémon Blue</option>
          </select>
          <span class="mt-1 block text-[11px] text-slate-600">The worker leases the matching mounted cartridge. Only games with a registered runtime profile are selectable.</span>
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Starter</span>
          <select v-model="starterMode" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <optgroup label="Default">
              <option value="default">{{ isLLM ? 'Let LLM decide' : 'Default · Squirtle' }}</option>
            </optgroup>
            <optgroup label="Vanilla starters">
              <option value="squirtle">Squirtle</option>
              <option value="charmander">Charmander</option>
              <option value="bulbasaur">Bulbasaur</option>
            </optgroup>
            <optgroup label="Starter experiments">
              <option value="random">Random · reasonable pool</option>
              <option value="random:basic">Random · basic / unevolved pool</option>
              <option value="random:any">Random · any non-glitch Gen I Pokémon</option>
              <option value="mew">Mew</option>
              <option value="mewtwo">Mewtwo</option>
              <option value="specific">Specific Pokémon…</option>
            </optgroup>
          </select>
          <span class="mt-1 block text-[11px] text-slate-600">Random choices are deterministic from the run seed. Pick Specific Pokémon for any other Gen I species.</span>
        </label>

        <label v-if="isSpecificStarter" class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Specific Pokémon</span>
          <input v-model="specificStarter" placeholder="e.g. pikachu, dragonite, snorlax" autocomplete="off" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" />
          <span class="mt-1 block text-[11px] text-slate-600">Enter any valid Generation I Pokémon name.</span>
        </label>

        <label v-if="!isLLM" class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Destination</span>
          <input v-model="form.dest" placeholder="viridian pokemon center" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" />
        </label>

        <label v-if="isLLM" class="block sm:col-span-2">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Goal</span>
          <select v-model="form.goal" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option v-for="goal in GOAL_OPTIONS" :key="goal || 'free'" :value="goal">{{ goal || 'Free play (no automatic stop)' }}</option>
          </select>
          <span class="mt-1 block text-[11px] text-slate-600">What ends the run. Goal is independent from play style and run purpose.</span>
        </label>

        <label v-if="isLLM" class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Play style</span>
          <select v-model="form.play_style" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option value="adventure">Adventure · natural play</option>
            <option value="speedrun">Speedrun · progression first</option>
            <option value="completionist">Completionist · explore and collect</option>
            <option value="team_builder">Team Builder · catches and training</option>
          </select>
          <span class="mt-1 block text-[11px] text-slate-600">How the agent values legal objectives. Completionist plays thoroughly but does not perform interactions only for test coverage.</span>
        </label>

        <label v-if="isLLM" class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Run purpose</span>
          <select v-model="form.purpose" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option value="normal">Normal · play the game</option>
            <option value="debug_coverage">Debug coverage · exercise new interactions</option>
          </select>
          <span class="mt-1 block text-[11px] text-slate-600">Debug coverage deliberately favors newly reachable maps, NPCs, trainers, pickups and interaction flows to expose bugs.</span>
        </label>

        <label v-if="isLLM" class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Risk tolerance</span>
          <select v-model="form.risk_tolerance" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option value="aggressive">Aggressive · push on longer</option>
            <option value="balanced">Balanced · protect progress</option>
            <option value="cautious">Cautious · heal early and often</option>
          </select>
          <span class="mt-1 block text-[11px] text-slate-600">Balanced and Cautious value PokéCenter recovery before avoidable blackouts.</span>
        </label>

        <label v-if="isLLM" class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Wild encounters</span>
          <select v-model="form.wild_encounters" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option value="planner">Planner decides · fight or flee</option>
            <option value="fight">Fight every encounter · never flee</option>
          </select>
          <span class="mt-1 block text-[11px] text-slate-600">Fight every encounter forces travel battles, producing more natural training.</span>
        </label>

        <label v-if="isLLM && hasDeployments" class="block sm:col-span-2">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Deployment</span>
          <select v-model="form.llm_deployment" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option
              v-for="deployment in strategists"
              :key="deployment.id"
              :value="deployment.id"
              :disabled="!deploymentSelectable(deployment)"
            >
              {{ deploymentOptionLabel(deployment) }}
            </option>
          </select>
          <span class="mt-1 block text-[11px] text-slate-600">
            {{ selectedDeployment ? `${selectedDeployment.model_id} on ${selectedDeployment.compute}` : 'Model identity plus compute placement.' }}
            Unavailable, failed, or busy hosts stay listed but cannot be queued until they are ready.
          </span>
        </label>

        <label v-else-if="isLLM" class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">LLM profile</span>
          <select v-model="form.llm_profile" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option value="auto">7900 XTX · default · CPU after 120s</option>
            <option value="gpu">RTX 4090 · manual</option>
            <option value="default">CPU only · manual</option>
          </select>
          <span class="mt-1 block text-[11px] text-slate-600">Auto uses the normal farm routing policy; manual profiles pin the requested route.</span>
        </label>

        <label v-if="isLLM" class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Reasoning effort</span>
          <select v-model="form.reasoning_effort" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option value="">Auto (endpoint default)</option>
            <option value="off">Off</option>
            <option value="low">Low</option>
            <option value="medium">Medium</option>
            <option value="high">High</option>
          </select>
        </label>

        <label v-if="isLLM" class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Fast decision engine</span>
          <select v-model="decisionTarget" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option value="off">Off</option>
            <option
              v-for="deployment in decisionDeployments"
              :key="`decision-${deployment.id}`"
              :value="`deployment:${deployment.id}`"
              :disabled="!deploymentSelectable(deployment)"
            >
              {{ deploymentOptionLabel(deployment) }}
            </option>
            <template v-if="!deployments.length">
              <option value="env:jev">TypeSafe Jev (runner endpoint)</option>
              <option value="env:system-one">Local System-1 (runner endpoint)</option>
            </template>
          </select>
          <span class="mt-1 block text-[11px] text-slate-600">
            {{ selectedDecisionDeployment
              ? `${selectedDecisionDeployment.api_model} via ${selectedDecisionDeployment.endpoint}`
              : 'Separate from the strategist. Pick any registered deployment; its key stays in the runner environment named by token_env.' }}
          </span>
        </label>

        <label v-if="isLLM && decisionSelected" class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Mode</span>
          <select v-model="decision.mode" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option value="shadow">Shadow</option>
            <option value="active">Active</option>
          </select>
          <span class="mt-1 block text-[11px] text-slate-600">Shadow asks the engine and records its answer and agreement; the strategist and deterministic policy still decide. Active lets accepted answers steer objectives and recovery.</span>
        </label>

        <fieldset v-if="isLLM && decisionSelected" class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">{{ decisionShadow ? 'Decision engine observes' : 'Decision engine decides' }}</span>
          <label class="mt-2 flex items-center gap-2 text-sm" :class="decisionShadow ? 'text-slate-300' : 'text-slate-600'">
            <input v-model="decision.battles" type="checkbox" :disabled="!decisionShadow" class="rounded border-white/10 bg-white/6" />
            Battles <span v-if="!decisionShadow" class="text-[11px]">(shadow only)</span>
          </label>
          <label class="mt-1 flex items-center gap-2 text-sm text-slate-300">
            <input v-model="decision.objectives" type="checkbox" class="rounded border-white/10 bg-white/6" />
            Objective selection
          </label>
          <label class="mt-1 flex items-center gap-2 text-sm text-slate-300">
            <input v-model="decision.failures" type="checkbox" class="rounded border-white/10 bg-white/6" />
            Failure recovery
          </label>
          <label class="mt-2 block">
            <span class="text-[11px] text-slate-500">Min confidence</span>
            <input v-model.number="decision.min_confidence" type="number" min="0" max="1" step="0.05" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400 font-mono" />
          </label>
          <span class="mt-1 block text-[11px] text-slate-600">Answers below the threshold fall back to the strategist and deterministic policy.</span>
        </fieldset>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Seed</span>
          <input v-model.number="form.seed" type="number" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 font-mono text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" />
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Play speed</span>
          <select v-model.number="form.fps" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option :value="60">1× · 60 FPS</option>
            <option :value="120">2× · 120 FPS</option>
            <option :value="240">4× · 240 FPS</option>
            <option :value="480">8× · 480 FPS</option>
            <option :value="0">Max · uncapped</option>
          </select>
          <span class="mt-1 block text-[11px] text-slate-600">Max keeps the existing 0 FPS wire value and runs as fast as the worker can emulate.</span>
        </label>

        <label v-if="isLLM" class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Recovery</span>
          <select v-model="form.recovery_profile" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option value="resilient">Resilient · keep pursuing the goal</option>
            <option value="strict">Strict · stop after bounded recovery</option>
          </select>
          <span class="mt-1 block text-[11px] text-slate-600">Resilient escalates from local resume to progressively older milestone checkpoints instead of ending the campaign on a stuck/failed attempt.</span>
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Max rounds</span>
          <input v-model.number="form.max_rounds" type="number" min="0" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 font-mono text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" />
          <span class="mt-1 block text-[11px] text-slate-600">0 = normal goal-driven / uncapped.</span>
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Max frames</span>
          <input v-model.number="form.max_frames" type="number" min="0" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 font-mono text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" />
          <span class="mt-1 block text-[11px] text-slate-600">0 = runner safety default.</span>
        </label>

        <div class="sm:col-span-2 flex flex-wrap gap-4 rounded-md border border-white/8 bg-black/10 px-3 py-3">
          <label class="flex items-center gap-2 text-sm text-slate-300">
            <input v-model="form.endless" type="checkbox" class="size-4 rounded border-white/20 bg-white/5 text-cyan-400 focus:ring-cyan-400" />
            Endless successor runs
          </label>
          <label class="flex items-center gap-2 text-sm text-slate-300">
            <input v-model="form.random_seed" type="checkbox" :disabled="!form.endless" class="size-4 rounded border-white/20 bg-white/5 text-cyan-400 focus:ring-cyan-400 disabled:opacity-40" />
            Fresh seed per successor
          </label>
        </div>

        <div class="sm:col-span-2 flex items-center justify-between gap-4 border-t border-white/8 pt-4">
          <p v-if="error" class="text-sm text-rose-300" role="alert">{{ error }}</p>
          <p v-else class="text-xs text-slate-500">The wall validates the spec and keeps the runner contract authoritative.</p>
          <button type="submit" :disabled="submitting" class="inline-flex shrink-0 items-center gap-1.5 rounded-md bg-cyan-500 px-3 py-2 text-xs font-semibold text-white shadow-sm hover:bg-cyan-400 disabled:cursor-wait disabled:opacity-60">
            <PlayIcon class="size-4" aria-hidden="true" />
            {{ submitting ? 'Queueing…' : 'Queue run' }}
          </button>
        </div>
      </form>
    </Panel>

    <Panel title="Queued run" description="Jump straight into Live after the wall accepts the spec." compact>
      <div v-if="createdRunID" class="rounded-md border border-emerald-300/15 bg-emerald-300/8 p-4">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-emerald-200 uppercase">Queued</span>
        <p class="mt-2 break-all font-mono text-xs text-slate-200">{{ createdRunID }}</p>
        <a :href="runURL(createdRunID)" class="mt-4 inline-flex items-center gap-1.5 rounded-md bg-white/8 px-2.5 py-1.5 text-xs font-semibold text-white ring-1 ring-white/10 hover:bg-white/12">
          Open Live
          <ArrowRightIcon class="size-3.5" aria-hidden="true" />
        </a>
      </div>
      <div v-else class="py-8 text-center text-sm text-slate-500">
        Queue a run to get a direct Live link here.
      </div>
    </Panel>
  </div>
</template>