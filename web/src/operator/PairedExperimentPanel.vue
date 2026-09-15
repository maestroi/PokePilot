<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { PlayIcon } from '@heroicons/vue/20/solid'
import { createExperiment, getExperiments, getModels } from '../shared/api/client'
import type { ExperimentView, ModelDeployment } from '../shared/api/types'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { defaultExperimentArms, deploymentOptionLabel } from './llmDeployments'

const fieldClass = 'mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400'

const {
  data: modelsData
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

const deployments = computed<ModelDeployment[]>(() => modelsData.value?.deployments ?? [])
const latest = computed<ExperimentView | null>(() => experimentData.value?.[0] ?? created.value)

const form = reactive({
  name: 'Brock · 27B vs 4B',
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
  max_frames: 0
})

watch(deployments, (next) => {
  if (!next.length) return
  const [armA, armB] = defaultExperimentArms(next)
  if (!form.arm_a) form.arm_a = armA
  if (!form.arm_b) form.arm_b = armB
}, { immediate: true })

const submitting = ref(false)
const error = ref('')
const created = ref<ExperimentView | null>(null)

function parseSeeds(value: string): number[] {
  return value.split(/[\s,]+/).map((part) => part.trim()).filter(Boolean).map((part) => Number(part)).filter((seed) => Number.isFinite(seed))
}

function pct(value: number | undefined): string {
  return `${Math.round((Number(value) || 0) * 100)}%`
}

function seconds(value: number | undefined): string {
  return `${Number(value || 0).toFixed(2)}s`
}

async function submit(): Promise<void> {
  if (submitting.value) return
  error.value = ''
  if (!form.arm_a || !form.arm_b || form.arm_a === form.arm_b) {
    error.value = 'Choose two different deployments for Arm A and Arm B.'
    return
  }
  submitting.value = true
  try {
    const seeds = parseSeeds(form.seeds)
    created.value = await createExperiment({
      name: form.name.trim() || 'Brock · 27B vs 4B',
      arm_a: { name: 'A', deployment: form.arm_a, max_parallel_workers: Number(form.arm_a_workers || 1) },
      arm_b: { name: 'B', deployment: form.arm_b, max_parallel_workers: Number(form.arm_b_workers || 1) },
      goal: form.goal.trim() || 'Earn the Boulder Badge.',
      starter: form.starter,
      seeds,
      seed_count: seeds.length ? seeds.length : Number(form.seed_count || 20),
      play_style: form.play_style,
      risk_tolerance: form.risk_tolerance,
      wild_encounters: form.wild_encounters,
      reasoning_effort: form.reasoning_effort,
      fps: Number(form.fps || 0),
      max_rounds: Number(form.max_rounds || 0),
      max_frames: Number(form.max_frames || 0)
    })
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
  return [
    { label: 'Boulder success', a: `${a.boulder_successes || 0}/${a.done || 0} · ${pct(a.success_rate)}`, b: `${b.boulder_successes || 0}/${b.done || 0} · ${pct(b.success_rate)}` },
    { label: 'Completed runs', a: `${a.done || 0}/${a.runs || 0}`, b: `${b.done || 0}/${b.runs || 0}` },
    { label: 'Rounds', a: String(a.rounds || 0), b: String(b.rounds || 0) },
    { label: 'Frames', a: String(a.frames || 0), b: String(b.frames || 0) },
    { label: 'Strategic calls', a: String(a.strategic_calls || 0), b: String(b.strategic_calls || 0) },
    { label: 'Avg / p50 / p95', a: `${seconds(a.avg_strategic_call_seconds)} / ${seconds(a.p50_strategic_call_seconds)} / ${seconds(a.p95_strategic_call_seconds)}`, b: `${seconds(b.avg_strategic_call_seconds)} / ${seconds(b.p50_strategic_call_seconds)} / ${seconds(b.p95_strategic_call_seconds)}` },
    { label: 'Prefill / decode TPS', a: `${Number(a.avg_prefill_tps || 0).toFixed(1)} / ${Number(a.avg_decode_tps || 0).toFixed(1)}`, b: `${Number(b.avg_prefill_tps || 0).toFixed(1)} / ${Number(b.avg_decode_tps || 0).toFixed(1)}` },
    { label: 'Prompt / completion', a: `${a.prompt_tokens || 0} / ${a.completion_tokens || 0}`, b: `${b.prompt_tokens || 0} / ${b.completion_tokens || 0}` },
    { label: 'Rejects / transport / fallbacks', a: `${a.rejected || 0} / ${a.transport_errors || 0} / ${a.fallbacks || 0}`, b: `${b.rejected || 0} / ${b.transport_errors || 0} / ${b.fallbacks || 0}` },
    { label: 'Plan exec / skipped', a: `${a.plan_executions || 0} / ${a.steps_skipped || 0}`, b: `${b.plan_executions || 0} / ${b.steps_skipped || 0}` }
  ]
})
</script>

<template>
  <Panel title="Paired LLM experiment" description="Matched A/B runs share seed and gameplay policy; only deployment differs." compact>
    <form v-if="deployments.length" class="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3" @submit.prevent="submit">
      <label class="block sm:col-span-2 xl:col-span-1">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Name</span>
        <input v-model="form.name" :class="fieldClass" />
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Arm A</span>
        <select v-model="form.arm_a" :class="fieldClass">
          <option v-for="deployment in deployments" :key="`a-${deployment.id}`" :value="deployment.id" :disabled="deployment.enabled === false">{{ deploymentOptionLabel(deployment) }}</option>
        </select>
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Arm B</span>
        <select v-model="form.arm_b" :class="fieldClass">
          <option v-for="deployment in deployments" :key="`b-${deployment.id}`" :value="deployment.id" :disabled="deployment.enabled === false">{{ deploymentOptionLabel(deployment) }}</option>
        </select>
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Arm A workers</span>
        <input v-model.number="form.arm_a_workers" type="number" min="1" max="16" :class="fieldClass" />
        <span class="mt-1 block text-[10px] text-slate-600">1 keeps this arm at maximum per-run model speed.</span>
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Arm B workers</span>
        <input v-model.number="form.arm_b_workers" type="number" min="1" max="16" :class="fieldClass" />
        <span class="mt-1 block text-[10px] text-slate-600">Runs above the cap remain queued.</span>
      </label>
      <div class="hidden xl:block"></div>
      <label class="block sm:col-span-2 xl:col-span-3">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Goal</span>
        <input v-model="form.goal" :class="fieldClass" />
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Starter</span>
        <select v-model="form.starter" :class="fieldClass">
          <option value="">Let LLM decide</option>
          <option value="squirtle">Squirtle</option>
          <option value="charmander">Charmander</option>
          <option value="bulbasaur">Bulbasaur</option>
        </select>
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Seed count</span>
        <input v-model.number="form.seed_count" type="number" min="1" max="1000" :class="fieldClass" />
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Seed list</span>
        <input v-model="form.seeds" placeholder="optional: 1, 7, 42" :class="fieldClass" />
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Play style</span>
        <select v-model="form.play_style" :class="fieldClass">
          <option value="adventure">Adventure</option>
          <option value="speedrun">Speedrun</option>
          <option value="completionist">Completionist</option>
          <option value="team_builder">Team Builder</option>
        </select>
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Risk tolerance</span>
        <select v-model="form.risk_tolerance" :class="fieldClass">
          <option value="balanced">Balanced</option>
          <option value="aggressive">Aggressive</option>
          <option value="cautious">Cautious</option>
        </select>
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Wild encounters</span>
        <select v-model="form.wild_encounters" :class="fieldClass">
          <option value="planner">Planner decides</option>
          <option value="fight">Fight every encounter</option>
        </select>
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Reasoning effort</span>
        <select v-model="form.reasoning_effort" :class="fieldClass">
          <option value="">Endpoint default</option>
          <option value="off">Off</option>
          <option value="low">Low</option>
          <option value="medium">Medium</option>
          <option value="high">High</option>
        </select>
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">FPS</span>
        <select v-model.number="form.fps" :class="fieldClass">
          <option :value="0">Max · uncapped</option>
          <option :value="60">1× · 60 FPS</option>
          <option :value="120">2× · 120 FPS</option>
          <option :value="240">4× · 240 FPS</option>
        </select>
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Round cap</span>
        <input v-model.number="form.max_rounds" type="number" min="0" :class="fieldClass" />
      </label>
      <label class="block">
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Frame cap</span>
        <input v-model.number="form.max_frames" type="number" min="0" :class="fieldClass" />
      </label>
      <div class="sm:col-span-2 xl:col-span-3 flex flex-col gap-3 border-t border-white/8 pt-3 sm:flex-row sm:items-center sm:justify-between">
        <p v-if="error" class="text-sm text-rose-300" role="alert">{{ error }}</p>
        <p v-else class="text-[11px] text-slate-600">Each seed queues one run per arm. 1 vs 1 is the fair benchmark default; higher caps trade per-run TPS for aggregate throughput.</p>
        <button type="submit" :disabled="submitting" class="inline-flex shrink-0 items-center justify-center gap-1.5 rounded-md bg-cyan-500 px-3 py-2 text-xs font-semibold text-white shadow-sm hover:bg-cyan-400 disabled:cursor-wait disabled:opacity-60">
          <PlayIcon class="size-4" aria-hidden="true" />
          {{ submitting ? 'Queueing pairs…' : 'Start paired experiment' }}
        </button>
      </div>
    </form>
    <p v-else class="py-6 text-center text-sm text-slate-500">Configure a model registry to start paired experiments.</p>

    <div class="mt-4 border-t border-white/8 pt-4">
      <ResourceState
        :state="latest ? 'ready' : experimentState"
        title="No paired experiment loaded yet"
        :message="experimentError || 'Start a comparison to see Boulder success, latency, and A/B wins here.'"
        :rows="4"
      >
        <div v-if="latest" class="space-y-3">
          <div class="flex flex-col gap-1 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">{{ latest.id }}</span>
              <h3 class="text-sm font-semibold text-white">{{ latest.name }}</h3>
            </div>
            <span class="font-mono text-[11px] text-slate-500">{{ latest.total_pairs || 0 }} pairs</span>
          </div>
          <div class="hidden grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)] gap-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase sm:grid">
            <span>Metric</span>
            <span>Arm A</span>
            <span>Arm B</span>
          </div>
          <div class="divide-y divide-white/8">
            <div v-for="metric in metrics" :key="metric.label" class="grid grid-cols-1 gap-1 py-2 sm:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,1fr)] sm:items-baseline">
              <span class="text-[11px] text-slate-500">{{ metric.label }}</span>
              <strong class="font-mono text-[11px] font-medium text-slate-200">{{ metric.a }}</strong>
              <strong class="font-mono text-[11px] font-medium text-slate-200">{{ metric.b }}</strong>
            </div>
          </div>
          <div class="grid grid-cols-3 gap-2 rounded-md border border-white/8 bg-black/20 px-3 py-3 text-center">
            <div>
              <div class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">A wins</div>
              <div class="mt-1 font-mono text-lg text-white">{{ latest.paired?.a_wins || 0 }}</div>
            </div>
            <div>
              <div class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Ties</div>
              <div class="mt-1 font-mono text-lg text-white">{{ latest.paired?.ties || 0 }}</div>
            </div>
            <div>
              <div class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">B wins</div>
              <div class="mt-1 font-mono text-lg text-white">{{ latest.paired?.b_wins || 0 }}</div>
            </div>
          </div>
        </div>
      </ResourceState>
    </div>
  </Panel>
</template>
