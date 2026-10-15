<script setup lang="ts">
import { computed, watch } from 'vue'
import { artifactContentURL, getRunArtifacts, getRunCheckpoints, getRunDebug } from '../shared/api/client'
import type { RunArtifact } from '../shared/api/types'
import ResourceState from '../shared/components/ResourceState.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'

const props = defineProps<{ runID: string }>()
type EventRow = Record<string, unknown>
type Evidence = { events: EventRow[]; checkpoints: unknown[]; artifacts: RunArtifact[]; errors: string[] }

const evidence = usePollingResource<Evidence>(
  async (signal) => {
    const [debug, checkpoints, artifacts] = await Promise.allSettled([
      getRunDebug(props.runID, signal),
      getRunCheckpoints(props.runID, signal),
      getRunArtifacts(props.runID, signal)
    ])
    if (debug.status === 'rejected' && checkpoints.status === 'rejected' && artifacts.status === 'rejected') {
      throw debug.reason
    }
    const data = debug.status === 'fulfilled' ? debug.value : {}
    return {
      events: Array.isArray(data.timeline)
        ? data.timeline.filter((event): event is EventRow => Boolean(event) && typeof event === 'object').slice(-12)
        : [],
      checkpoints: checkpoints.status === 'fulfilled' ? checkpoints.value : [],
      artifacts: artifacts.status === 'fulfilled' ? artifacts.value : [],
      errors: [
        debug.status === 'rejected' ? 'Recorded timeline unavailable' : '',
        checkpoints.status === 'rejected' ? 'Checkpoint list unavailable' : '',
        artifacts.status === 'rejected' ? 'Artifact list unavailable' : ''
      ].filter(Boolean)
    }
  },
  { intervalMs: 30000 }
)
watch(() => props.runID, () => { void evidence.retry() })
const events = computed(() => evidence.data.value?.events || [])
function description(event: EventRow): string {
  return String(event.summary || event.description || event.detail || event.type || event.kind || 'Recorded event')
}
function source(event: EventRow): string {
  return String(event.kind || event.type || 'event')
}
function frame(event: EventRow): string {
  const n = Number(event.frame)
  return Number.isFinite(n) && n >= 0 ? `frame ${n.toLocaleString()}` : 'frame not recorded'
}
function retry(): void { void evidence.retry() }
</script>

<template>
  <div class="mt-3 rounded-sm border border-white/10 bg-black/10 px-3 py-3">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h3 class="text-xs font-semibold text-white">Persisted evidence</h3>
        <p class="mt-1 text-[11px] text-slate-500">Recorded events, checkpoints and artifacts from this finished run. These are not live game telemetry or semantic annotations for every video frame.</p>
      </div>
      <button type="button" class="text-xs text-cyan-200 hover:underline focus-visible:outline-2 focus-visible:outline-cyan-300" @click="retry">Refresh evidence</button>
    </div>
    <ResourceState :state="evidence.state.value" title="Run evidence unavailable" :message="evidence.error.value || 'No recorded evidence returned by the server.'" :rows="3">
      <template #actions><button type="button" class="text-xs text-cyan-200 hover:underline" @click="retry">Retry evidence</button></template>
      <div v-if="evidence.data.value?.errors?.length" class="mt-2 text-[11px] text-amber-200" role="status">{{ evidence.data.value.errors.join(' · ') }}</div>
      <dl class="mt-3 grid grid-cols-3 gap-px overflow-hidden rounded-sm bg-white/10">
        <div v-for="item in [
          ['Recent events', events.length],
          ['Checkpoints', evidence.data.value?.checkpoints?.length || 0],
          ['Artifacts', evidence.data.value?.artifacts?.length || 0]
        ]" :key="item[0]" class="bg-[#0b111a] p-2.5">
          <dt class="text-[10px] text-slate-500">{{ item[0] }}</dt>
          <dd class="mt-1 font-mono text-lg text-white">{{ item[1] }}</dd>
        </div>
      </dl>
      <div class="mt-3 grid gap-4 lg:grid-cols-2">
        <section>
          <h4 class="text-[11px] font-semibold text-slate-300">Last recorded events</h4>
          <ol v-if="events.length" class="mt-2 max-h-60 space-y-2 overflow-y-auto">
            <li v-for="(event, index) in events" :key="index" class="border-l border-white/10 pl-3">
              <div class="text-[10px] text-slate-500">{{ source(event) }} · {{ frame(event) }}</div>
              <p class="mt-0.5 line-clamp-3 break-words text-[11px] text-slate-300">{{ description(event) }}</p>
            </li>
          </ol>
          <p v-else class="mt-2 text-[11px] text-slate-500">No recorded timeline events available.</p>
        </section>
        <section>
          <h4 class="text-[11px] font-semibold text-slate-300">Stored artifacts</h4>
          <ul v-if="evidence.data.value?.artifacts?.length" class="mt-2 max-h-60 divide-y divide-white/8 overflow-y-auto">
            <li v-for="artifact in evidence.data.value.artifacts" :key="artifact.name" class="py-1.5">
              <a :href="artifactContentURL(runID, artifact.name)" class="block break-all text-[11px] text-cyan-200 hover:underline focus-visible:outline-2 focus-visible:outline-cyan-300">{{ artifact.name }}</a>
              <span class="text-[10px] text-slate-500">{{ artifact.media_type || artifact.storage || 'recorded artifact' }}</span>
            </li>
          </ul>
          <p v-else class="mt-2 text-[11px] text-slate-500">No artifacts listed.</p>
        </section>
      </div>
    </ResourceState>
  </div>
</template>
