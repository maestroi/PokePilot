<script setup lang="ts">
import { computed } from 'vue'
import { getModels } from '../shared/api/client'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { deploymentIdentity, deploymentStateLabel, deploymentStateTone } from './llmDeployments'

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

function loadedNote(deployment: (typeof deployments.value)[number]): string {
  if (deployment.loaded_deployment && deployment.loaded_deployment !== deployment.id) {
    return `currently loaded: ${deployment.loaded_deployment}`
  }
  const active = Number(deployment.active_leases || 0)
  const limit = Number(deployment.max_parallel_workers || 1)
  const queued = Number(deployment.queued || 0)
  const queueNote = queued ? ` · ${queued} queued` : ''
  return `${active}/${limit} workers${queueNote}`
}
</script>

<template>
  <Panel title="LLM deployments" description="Model identity, compute placement, and host lifecycle." compact>
    <ResourceState
      :state="state"
      title="No deployments configured"
      :message="error || 'The wall has no POKEPILOT_MODEL_REGISTRY. Legacy LLM profiles remain available in Tools.'"
      :rows="3"
    >
      <template #actions>
        <button type="button" class="inline-flex items-center gap-1.5 rounded-md bg-white/10 px-2.5 py-1.5 text-xs font-semibold text-white ring-1 ring-white/10 hover:bg-white/15" @click="retry">
          Retry now
        </button>
      </template>

      <ul class="divide-y divide-white/8">
        <li v-for="deployment in deployments" :key="deployment.id" class="py-3 first:pt-0 last:pb-0">
          <div class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-sm font-semibold text-slate-100">{{ deployment.label || deployment.id }}</span>
                <StatusBadge :tone="deploymentStateTone(deployment.state)">{{ deploymentStateLabel(deployment) }}</StatusBadge>
              </div>
              <p class="mt-1 break-all font-mono text-[11px] text-slate-500">{{ deploymentIdentity(deployment) || deployment.id }}</p>
              <p class="mt-1 text-[11px] text-slate-500">{{ deployment.compute }} · {{ deployment.endpoint }}</p>
            </div>
            <div class="shrink-0 text-[11px] text-slate-500 sm:max-w-[16rem] sm:text-right">
              <p v-if="loadedNote(deployment)">{{ loadedNote(deployment) }}</p>
              <p v-if="deployment.error" class="text-rose-300">{{ deployment.error }}</p>
            </div>
          </div>
        </li>
      </ul>
      <p class="mt-3 text-[11px] text-slate-600">Each deployment queues above its worker cap instead of oversubscribing inference. Free capacity on other models keeps leasing independently.</p>
    </ResourceState>
  </Panel>
</template>
