<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  deleteModelDeployment,
  getModels,
  patchDeploymentWorkers,
  saveModelDeployment,
  testModelDeployment
} from '../shared/api/client'
import type { ModelDeployment, ModelDeploymentInput } from '../shared/api/types'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { deploymentIdentity, deploymentStateLabel, deploymentStateTone } from './llmDeployments'

const compactFieldClass = 'mt-1 block w-20 rounded-md border-0 bg-white/6 px-2 py-1.5 text-right font-mono text-xs text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400'
const fieldClass = 'mt-1 block w-full rounded-md border-0 bg-white/6 px-3 py-2 text-sm text-slate-200 outline-1 -outline-offset-1 outline-white/10 focus:outline-2 focus:-outline-offset-2 focus:outline-cyan-400'

const {
  data,
  error,
  state,
  retry
} = usePollingResource(
  (signal) => getModels(signal),
  { intervalMs: 5000, isEmpty: (snapshot) => snapshot.deployments.length === 0 }
)

const deployments = computed(() => data.value?.deployments ?? [])
const workerDrafts = reactive<Record<string, number>>({})
const savingWorkersID = ref('')
const actionError = ref('')
const editorOpen = ref(false)
const editingID = ref('')
const editorBusy = ref(false)
const testResult = ref<ModelDeployment | null>(null)
const draft = ref<ModelDeploymentInput>(blankDeployment())

watch(deployments, (list) => {
  for (const deployment of list) {
    if (workerDrafts[deployment.id] === undefined) {
      workerDrafts[deployment.id] = Number(deployment.max_parallel_workers || 1)
    }
  }
}, { immediate: true })

function blankDeployment(): ModelDeploymentInput {
  return {
    id: '',
    label: '',
    model_id: '',
    revision: '',
    artifact: '',
    quantization: '',
    compute: '',
    endpoint: '',
    api_model: '',
    protocol: 'openai',
    enabled: true,
    discover: true,
    default_for: ['farm'],
    control_url: '',
    token_env: '',
    engine: 'llama.cpp',
    engine_version: '',
    engine_config: '',
    max_parallel_workers: 1,
    legacy_profile: 'auto'
  }
}

function loadedNote(deployment: ModelDeployment): string {
  if (deployment.loaded_deployment && deployment.loaded_deployment !== deployment.id) {
    return `currently loaded: ${deployment.loaded_deployment}`
  }
  const active = Number(deployment.active_leases || 0)
  const limit = Number(deployment.max_parallel_workers || 1)
  const queued = Number(deployment.queued || 0)
  const queueNote = queued ? ` · ${queued} queued` : ''
  return `${active}/${limit} workers${queueNote}`
}

function startAdd(): void {
  actionError.value = ''
  editingID.value = ''
  testResult.value = null
  draft.value = blankDeployment()
  editorOpen.value = true
}

function startEdit(deployment: ModelDeployment): void {
  actionError.value = ''
  editingID.value = deployment.id
  testResult.value = null
  draft.value = {
    id: deployment.id,
    label: deployment.label || '',
    model_id: deployment.model_id || '',
    revision: deployment.revision || '',
    artifact: deployment.artifact || '',
    quantization: deployment.quantization || '',
    compute: deployment.compute || '',
    endpoint: deployment.endpoint || '',
    api_model: deployment.api_model || '',
    protocol: deployment.protocol || 'openai',
    enabled: deployment.enabled !== false,
    discover: Boolean(deployment.discover),
    default_for: [...(deployment.default_for || [])],
    control_url: deployment.control_url || '',
    token_env: deployment.token_env || '',
    engine: deployment.engine || '',
    engine_version: deployment.engine_version || '',
    engine_config: deployment.engine_config || '',
    max_parallel_workers: Number(deployment.max_parallel_workers || 1),
    legacy_profile: deployment.legacy_profile || ''
  }
  editorOpen.value = true
}

// A choice API has no /models listing to discover and cannot be the farm
// strategist, so switching to it fills the fields a Jev row needs. The key
// itself is never entered here: token_env names the runner variable.
function applyProtocolDefaults(): void {
  const value = draft.value
  if (value.protocol !== 'typesafe-choice') return
  value.discover = false
  value.default_for = []
  value.legacy_profile = ''
  value.endpoint ||= 'https://api.typesafe.ai/v1'
  value.model_id ||= 'jev-latest'
  value.api_model ||= 'jev-latest'
  value.token_env ||= 'TYPESAFE_API_KEY'
  value.compute ||= 'TypeSafe cloud'
  value.engine = value.engine && value.engine !== 'llama.cpp' ? value.engine : 'typesafe'
}

function closeEditor(): void {
  editorOpen.value = false
  editingID.value = ''
  testResult.value = null
  actionError.value = ''
}

function hasRole(role: string): boolean {
  return (draft.value.default_for || []).includes(role)
}

function toggleRole(role: string): void {
  const roles = new Set(draft.value.default_for || [])
  if (roles.has(role)) roles.delete(role)
  else roles.add(role)
  draft.value.default_for = [...roles]
}

function cleanDraft(): ModelDeploymentInput {
  const value = { ...draft.value }
  value.id = value.id.trim()
  value.label = value.label.trim()
  value.model_id = value.model_id.trim()
  value.compute = value.compute.trim()
  value.endpoint = value.endpoint.trim().replace(/\/$/, '')
  value.api_model = value.api_model.trim()
  value.control_url = value.control_url?.trim().replace(/\/$/, '') || ''
  value.token_env = value.token_env?.trim() || ''
  value.engine = value.engine?.trim() || ''
  value.max_parallel_workers = Math.max(1, Number(value.max_parallel_workers || 1))
  return value
}

async function testDraft(): Promise<void> {
  if (editorBusy.value) return
  actionError.value = ''
  testResult.value = null
  editorBusy.value = true
  try {
    testResult.value = await testModelDeployment(cleanDraft())
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Unable to reach inference endpoint'
  } finally {
    editorBusy.value = false
  }
}

async function saveDraft(): Promise<void> {
  if (editorBusy.value) return
  actionError.value = ''
  editorBusy.value = true
  try {
    await saveModelDeployment(cleanDraft())
    closeEditor()
    await retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Unable to save inference endpoint'
  } finally {
    editorBusy.value = false
  }
}

async function removeDeployment(deployment: ModelDeployment): Promise<void> {
  if (!window.confirm(`Delete inference deployment "${deployment.label || deployment.id}"? Existing historical runs keep their recorded inference identity.`)) return
  actionError.value = ''
  try {
    await deleteModelDeployment(deployment.id)
    if (editingID.value === deployment.id) closeEditor()
    await retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Unable to delete inference endpoint'
  }
}

async function saveWorkers(id: string): Promise<void> {
  if (savingWorkersID.value) return
  actionError.value = ''
  savingWorkersID.value = id
  try {
    const updated = await patchDeploymentWorkers(id, Number(workerDrafts[id] || 1))
    workerDrafts[id] = Number(updated.max_parallel_workers || 1)
    void retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Unable to update worker cap'
  } finally {
    savingWorkersID.value = ''
  }
}
</script>

<template>
  <Panel title="LLM deployments" description="Add, test, and manage inference endpoints without redeploying PokéPilot." compact>
    <ResourceState
      :state="state"
      title="No deployments configured"
      :message="error || 'Add an OpenAI-compatible endpoint to start routing LLM runs.'"
      :rows="3"
    >
      <template #actions>
        <div class="flex gap-2">
          <button type="button" class="inline-flex items-center rounded-md bg-cyan-500 px-2.5 py-1.5 text-xs font-semibold text-white hover:bg-cyan-400" @click="startAdd">
            Add endpoint
          </button>
          <button type="button" class="inline-flex items-center rounded-md bg-white/10 px-2.5 py-1.5 text-xs font-semibold text-white ring-1 ring-white/10 hover:bg-white/15" @click="retry">
            Retry now
          </button>
        </div>
      </template>

      <div class="mb-3 flex items-center justify-between gap-3">
        <p class="text-[11px] text-slate-500">Discovery reads <span class="font-mono">/v1/models</span>; API keys remain in environment or Swarm secrets.</p>
        <button type="button" class="shrink-0 rounded-md bg-cyan-500 px-2.5 py-1.5 text-xs font-semibold text-white hover:bg-cyan-400" @click="startAdd">
          Add endpoint
        </button>
      </div>

      <div v-if="editorOpen" class="mb-4 rounded-lg border border-cyan-300/15 bg-cyan-300/5 p-4">
        <div class="mb-3 flex items-center justify-between">
          <div>
            <p class="text-sm font-semibold text-slate-100">{{ editingID ? 'Edit endpoint' : 'Add inference endpoint' }}</p>
            <p class="mt-0.5 text-[11px] text-slate-500">For llama.cpp/vLLM, leave discovery on and model fields can stay blank.</p>
          </div>
          <button type="button" class="text-xs text-slate-400 hover:text-white" @click="closeEditor">Close</button>
        </div>

        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <label class="block">
            <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">ID</span>
            <input v-model="draft.id" :disabled="Boolean(editingID)" placeholder="7900-primary" :class="fieldClass" />
          </label>
          <label class="block">
            <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Label</span>
            <input v-model="draft.label" placeholder="7900 XTX" :class="fieldClass" />
          </label>
          <label class="block">
            <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Compute</span>
            <input v-model="draft.compute" placeholder="RX 7900 XTX" :class="fieldClass" />
          </label>
          <label class="block">
            <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Protocol</span>
            <select v-model="draft.protocol" :class="fieldClass" @change="applyProtocolDefaults">
              <option value="openai">OpenAI-compatible (strategist or decisions)</option>
              <option value="typesafe-choice">TypeSafe choice API (decisions only)</option>
            </select>
          </label>
          <label class="block sm:col-span-2">
            <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">{{ draft.protocol === 'typesafe-choice' ? 'TypeSafe endpoint' : 'OpenAI-compatible endpoint' }}</span>
            <input v-model="draft.endpoint" :placeholder="draft.protocol === 'typesafe-choice' ? 'https://api.typesafe.ai/v1' : 'http://192.168.50.130:8002/v1'" :class="fieldClass" />
          </label>
          <label class="block">
            <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Workers</span>
            <input v-model.number="draft.max_parallel_workers" type="number" min="1" max="32" :class="fieldClass" />
          </label>
        </div>

        <div class="mt-3 flex flex-wrap items-center gap-x-5 gap-y-2 rounded-md border border-white/8 bg-black/10 px-3 py-2">
          <label class="flex items-center gap-2 text-xs text-slate-300">
            <input v-model="draft.enabled" type="checkbox" class="size-4 rounded border-white/20 bg-white/5 text-cyan-400 focus:ring-cyan-400" />
            Enabled
          </label>
          <label class="flex items-center gap-2 text-xs text-slate-300">
            <input v-model="draft.discover" type="checkbox" class="size-4 rounded border-white/20 bg-white/5 text-cyan-400 focus:ring-cyan-400" />
            Discover model automatically
          </label>
          <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Defaults</span>
          <label v-for="role in ['farm', 'experiment-a', 'experiment-b']" :key="role" class="flex items-center gap-1.5 text-xs text-slate-300">
            <input :checked="hasRole(role)" type="checkbox" class="size-4 rounded border-white/20 bg-white/5 text-cyan-400 focus:ring-cyan-400" @change="toggleRole(role)" />
            {{ role }}
          </label>
        </div>

        <details class="mt-3 rounded-md border border-white/8 bg-black/10 px-3 py-2">
          <summary class="cursor-pointer text-xs font-semibold text-slate-300">Advanced identity & control</summary>
          <div class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <label class="block">
              <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Model ID</span>
              <input v-model="draft.model_id" :disabled="draft.discover" placeholder="optional with discovery" :class="fieldClass" />
            </label>
            <label class="block">
              <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">API model</span>
              <input v-model="draft.api_model" :disabled="draft.discover" placeholder="optional with discovery" :class="fieldClass" />
            </label>
            <label class="block">
              <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Token env</span>
              <input v-model="draft.token_env" placeholder="MY_LLM_API_KEY" :class="fieldClass" />
            </label>
            <label class="block sm:col-span-2">
              <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Control URL</span>
              <input v-model="draft.control_url" placeholder="optional model-host control API" :class="fieldClass" />
            </label>
            <label class="block">
              <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Engine</span>
              <input v-model="draft.engine" placeholder="llama.cpp" :class="fieldClass" />
            </label>
            <label class="block">
              <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Legacy profile</span>
              <select v-model="draft.legacy_profile" :class="fieldClass">
                <option value="">None</option>
                <option value="auto">auto</option>
                <option value="gpu">gpu</option>
                <option value="default">default</option>
              </select>
            </label>
          </div>
        </details>

        <div class="mt-3 flex flex-wrap items-center gap-2">
          <button type="button" :disabled="editorBusy" class="rounded-md bg-white/10 px-2.5 py-1.5 text-xs font-semibold text-white ring-1 ring-white/10 hover:bg-white/15 disabled:opacity-50" @click="testDraft">
            Test connection
          </button>
          <button type="button" :disabled="editorBusy" class="rounded-md bg-cyan-500 px-2.5 py-1.5 text-xs font-semibold text-white hover:bg-cyan-400 disabled:opacity-50" @click="saveDraft">
            {{ editorBusy ? 'Working…' : 'Save endpoint' }}
          </button>
          <span v-if="testResult" class="text-xs text-emerald-300">
            Connected · {{ testResult.model_id || testResult.api_model }} · {{ testResult.endpoint }}
          </span>
        </div>
      </div>

      <ul class="divide-y divide-white/8">
        <li v-for="deployment in deployments" :key="deployment.id" class="py-3 first:pt-0 last:pb-0">
          <div class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-sm font-semibold text-slate-100">{{ deployment.label || deployment.id }}</span>
                <StatusBadge :tone="deploymentStateTone(deployment.state)">{{ deploymentStateLabel(deployment) }}</StatusBadge>
                <span v-for="role in deployment.default_for || []" :key="role" class="rounded bg-white/6 px-1.5 py-0.5 text-[10px] text-slate-400">{{ role }}</span>
              </div>
              <p class="mt-1 break-all font-mono text-[11px] text-slate-500">{{ deploymentIdentity(deployment) || deployment.id }}</p>
              <p class="mt-1 text-[11px] text-slate-500">{{ deployment.compute }} · {{ deployment.endpoint }}</p>
            </div>
            <div class="shrink-0 text-[11px] text-slate-500 sm:text-right">
              <p>{{ loadedNote(deployment) }}</p>
              <div class="mt-2 flex flex-wrap items-center justify-end gap-2">
                <input v-model.number="workerDrafts[deployment.id]" aria-label="Worker cap" type="number" min="1" max="32" :class="compactFieldClass" />
                <button
                  type="button"
                  class="rounded-md bg-white/10 px-2 py-1 text-[11px] font-semibold text-white ring-1 ring-white/10 hover:bg-white/15 disabled:opacity-50"
                  :disabled="savingWorkersID === deployment.id || Number(workerDrafts[deployment.id] || 0) === Number(deployment.max_parallel_workers || 1)"
                  @click="saveWorkers(deployment.id)"
                >
                  Apply workers
                </button>
                <button type="button" class="rounded-md bg-white/10 px-2 py-1 text-[11px] font-semibold text-white ring-1 ring-white/10 hover:bg-white/15" @click="startEdit(deployment)">
                  Edit
                </button>
                <button type="button" class="rounded-md bg-rose-300/10 px-2 py-1 text-[11px] font-semibold text-rose-200 ring-1 ring-rose-300/15 hover:bg-rose-300/15" @click="removeDeployment(deployment)">
                  Delete
                </button>
              </div>
              <p v-if="deployment.error" class="mt-1 text-rose-300">{{ deployment.error }}</p>
            </div>
          </div>
        </li>
      </ul>
      <p v-if="actionError" class="mt-3 text-[11px] text-rose-300">{{ actionError }}</p>
      <p class="mt-3 text-[11px] text-slate-600">Changes are persisted immediately. Historical runs keep their recorded inference identity even if a deployment is later edited or removed.</p>
    </ResourceState>
  </Panel>
</template>
