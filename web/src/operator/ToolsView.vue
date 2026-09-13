<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ArrowRightIcon, PlayIcon } from '@heroicons/vue/20/solid'
import { createRun } from '../shared/api/client'
import type { RunSpec } from '../shared/api/types'
import Panel from '../shared/components/Panel.vue'

const goalOptions = [
  'Earn the Boulder Badge.',
  'Earn 2 badges.',
  'Earn 3 badges.',
  'Earn 4 badges.',
  'Earn 5 badges.',
  'Earn 6 badges.',
  'Earn 7 badges.',
  'Earn all 8 badges.',
  'Beat the Elite Four and Champion.',
  ''
]

const form = reactive<RunSpec>({
  run_id: '',
  seed: 0,
  planner: 'llm',
  starter: '',
  dest: '',
  goal: 'Earn the Boulder Badge.',
  llm_profile: 'auto',
  play_style: 'adventure',
  reasoning_effort: '',
  fps: 60,
  max_rounds: 0,
  max_frames: 0,
  endless: false,
  random_seed: false
})

const submitting = ref(false)
const error = ref('')
const createdRunID = ref('')
const isLLM = computed(() => form.planner === 'llm')

function runURL(runID: string): string {
  const url = new URL('/', window.location.origin)
  url.searchParams.set('run', runID)
  url.hash = 'live'
  return url.toString()
}

async function submit(): Promise<void> {
  if (submitting.value) return
  submitting.value = true
  error.value = ''
  createdRunID.value = ''
  try {
    // Give new UI-created runs an ID before POST so optional run-scoped wire
    // metadata (including play_style) stays attached through queue/lease.
    const runID = form.run_id.trim() || `run-${crypto.randomUUID()}`
    const spec: RunSpec = {
      ...form,
      run_id: runID,
      starter: form.starter.trim(),
      dest: isLLM.value ? '' : form.dest.trim(),
      goal: isLLM.value ? form.goal.trim() : '',
      llm_profile: isLLM.value ? form.llm_profile : '',
      play_style: isLLM.value ? form.play_style : '',
      reasoning_effort: isLLM.value ? form.reasoning_effort : ''
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
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Starter</span>
          <select v-model="form.starter" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option value="">{{ isLLM ? 'Let LLM decide' : 'Default starter' }}</option>
            <option value="squirtle">Squirtle</option>
            <option value="charmander">Charmander</option>
            <option value="bulbasaur">Bulbasaur</option>
          </select>
        </label>

        <label v-if="!isLLM" class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Destination</span>
          <input v-model="form.dest" placeholder="viridian pokemon center" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" />
        </label>

        <label v-if="isLLM" class="block sm:col-span-2">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Goal</span>
          <select v-model="form.goal" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option v-for="goal in goalOptions" :key="goal || 'free'" :value="goal">{{ goal || 'Free play (no automatic stop)' }}</option>
          </select>
        </label>

        <label v-if="isLLM" class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Play style</span>
          <select v-model="form.play_style" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400">
            <option value="adventure">Adventure · natural play</option>
            <option value="speedrun">Speedrun · progression first</option>
            <option value="completionist">Completionist · explore and collect</option>
            <option value="team_builder">Team Builder · catches and training</option>
          </select>
          <span class="mt-1 block text-[11px] text-slate-600">Changes gameplay priorities only; it does not change the LLM hardware route.</span>
        </label>

        <label v-if="isLLM" class="block">
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

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Seed</span>
          <input v-model.number="form.seed" type="number" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 font-mono text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" />
        </label>

        <label class="block">
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">FPS</span>
          <input v-model.number="form.fps" type="number" min="0" class="mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 font-mono text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400" />
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
