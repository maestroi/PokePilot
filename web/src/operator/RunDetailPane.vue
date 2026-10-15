<script setup lang="ts">
import { computed, watch } from 'vue'
import { ClipboardDocumentIcon, XMarkIcon } from '@heroicons/vue/20/solid'
import { getRun, replayVideoURL } from '../shared/api/client'
import type { DashboardRun, MediaRenderJob } from '../shared/api/types'
import StatusPill from '../shared/components/StatusPill.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { presentOutcome, presentRunStatus } from '../shared/status'
import { operatorHref } from '../shared/urls'
import DecisionTelemetry from './DecisionTelemetry.vue'
import { showDecisionTelemetry } from './decisionTelemetry'
import { decisionEngineLabel, gameTitle } from './operations'
import RunActionMenu from './RunActionMenu.vue'
import RunHistoricalEvidence from './RunHistoricalEvidence.vue'
import { recordingMediaHref, recordingSummary } from './runRecording'
import {
  archiveWhen,
  archiveWhere,
  computeName,
  deploymentName,
  experimentLabel,
  formatDuration,
  formatInteger,
  middleTruncate,
  modelName,
  plannerLabel,
  plannerSeconds,
  progressLabel,
  rounds,
  safeIssueURL,
  statNumber,
  strategicCalls
} from './runs'

const props = defineProps<{
  runId: string
  row: DashboardRun | null
  mediaJob?: MediaRenderJob
  mediaState: 'ready' | 'loading' | 'stale' | 'error'
}>()
const emit = defineEmits<{ close: []; inspect: [runID: string]; copy: [runID: string]; delete: [run: DashboardRun] }>()

// Rows already carry the archive snapshot; the fetch covers deep links to runs
// outside the current page and keeps the pane fresh.
const focusedResource = usePollingResource(
  (signal) => props.runId ? getRun(props.runId, signal) : Promise.resolve(null),
  { intervalMs: 30000 }
)
watch(() => props.runId, () => { void focusedResource.retry() })

const run = computed(() => props.row
  || (focusedResource.data.value?.run_id === props.runId ? focusedResource.data.value : null))
const status = computed(() => run.value ? (run.value.status === 'done' ? presentOutcome(run.value) : presentRunStatus(run.value)) : null)
const recording = computed(() => run.value ? recordingSummary(run.value, props.mediaJob, props.mediaState) : null)
const mediaHref = computed(() => recording.value ? recordingMediaHref(props.runId, recording.value, window.location.search) : '')
const notFound = computed(() => !run.value && focusedResource.state.value === 'error')

function liveHref(id: string): string {
  return operatorHref('live', { run: id, job: '', output: '' })
}
</script>

<template>
  <section class="flex h-full min-h-0 flex-col bg-surface" aria-labelledby="run-pane-title">
    <header class="flex min-h-12 shrink-0 items-center gap-2 border-b border-rule bg-raised px-3 py-2">
      <h2 id="run-pane-title" class="min-w-0 truncate font-mono text-title font-medium" :title="runId">
        <span class="sr-only">Run </span>{{ middleTruncate(runId, 16, 6) }}
      </h2>
      <button type="button" class="ui-btn ui-btn-sm ui-icon-btn shrink-0" aria-label="Copy run ID" title="Copy run ID" @click="emit('copy', runId)">
        <ClipboardDocumentIcon class="size-4" aria-hidden="true" />
      </button>
      <StatusPill v-if="status" :status="status" class="ml-auto" />
      <button type="button" class="ui-btn ui-btn-sm ui-icon-btn shrink-0" :class="status ? '' : 'ml-auto'" aria-label="Close run details" title="Close (Esc)" @click="emit('close')">
        <XMarkIcon class="size-4" aria-hidden="true" />
      </button>
    </header>

    <div v-if="notFound" class="flex flex-1 flex-col items-start gap-3 p-4" role="status">
      <p class="text-title">Run not found. It may have been deleted.</p>
      <details class="max-w-[72ch] text-fg-2">
        <summary class="cursor-pointer text-meta">Details</summary>
        <p class="mt-1 font-mono text-meta break-words">{{ focusedResource.error.value }}</p>
      </details>
      <button type="button" class="ui-btn" @click="emit('close')">Close</button>
    </div>

    <div v-else-if="!run" class="flex-1 space-y-3 p-4" aria-busy="true" aria-label="Loading run">
      <div v-for="n in 6" :key="n" class="h-4 bg-raised motion-safe:animate-pulse" :class="n % 3 === 0 ? 'w-2/3' : 'w-full'" />
    </div>

    <template v-else>
      <div class="min-h-0 flex-1 overflow-y-auto px-3 pb-4">
        <p class="pt-3 text-fg-2">
          {{ gameTitle(run.game) }} · {{ plannerLabel(run) }}<template v-if="run.starter"> · {{ run.starter }}</template>
        </p>
        <p v-if="run.status !== 'done'" class="mt-3 border border-warn/40 bg-warn/10 px-3 py-2 text-warn" role="status">
          ▲ This run is still active. Inspect it in Live for current state; Runs only keeps finished history.
        </p>

        <h3 class="mt-4 mb-1 text-title font-semibold">Outcome</h3>
        <dl class="ui-kv">
          <dt>Result</dt><dd>{{ status?.label }}</dd>
          <dt>Finished</dt><dd class="font-mono">{{ archiveWhen(run) }}</dd>
          <template v-if="run.issue?.issue_number">
            <dt>Linked issue</dt>
            <dd>
              <a v-if="safeIssueURL(run)" :href="safeIssueURL(run)" target="_blank" rel="noopener" class="text-accent hover:underline">#{{ run.issue.issue_number }} ↗</a>
              <span v-else>#{{ run.issue.issue_number }}</span>
            </dd>
          </template>
        </dl>
        <p v-if="run.detail" class="mt-2 max-w-[72ch] whitespace-pre-wrap break-words text-fg-2">{{ run.detail }}</p>

        <h3 class="mt-5 mb-1 text-title font-semibold">Progress</h3>
        <dl class="ui-kv">
          <dt>Goal</dt><dd>{{ run.goal || '—' }}</dd>
          <dt>Progress</dt><dd>{{ progressLabel(run) }}</dd>
          <dt>Location</dt><dd>{{ archiveWhere(run) }}</dd>
          <dt>Badges</dt><dd>{{ run.player?.badges?.join(' · ') || '—' }}</dd>
          <dt>Milestones</dt><dd>{{ run.player?.milestones?.join(' · ') || '—' }}</dd>
        </dl>

        <h3 class="mt-5 mb-1 text-title font-semibold">Model</h3>
        <dl class="ui-kv">
          <dt>Model</dt><dd>{{ modelName(run) || (run.planner === 'scripted' ? 'Scripted' : '—') }}</dd>
          <dt>Deployment</dt><dd class="font-mono">{{ deploymentName(run) || '—' }}</dd>
          <dt>Compute</dt><dd>{{ computeName(run) || '—' }}</dd>
          <dt>Quantization</dt><dd>{{ run.inference?.quantization || '—' }}</dd>
          <dt>Reasoning effort</dt><dd>{{ run.reasoning_effort || 'Default' }}</dd>
          <dt>Decision engine</dt><dd>{{ decisionEngineLabel(run) }}</dd>
          <dt>Seed</dt><dd class="font-mono">{{ run.seed ?? '—' }}</dd>
          <dt>Experiment</dt>
          <dd>
            <a v-if="run.experiment_id" :href="operatorHref('analytics', { section: 'experiments', experiment: run.experiment_id }, '')" class="text-accent hover:underline">{{ experimentLabel(run) }}</a>
            <span v-else>—</span>
          </dd>
        </dl>

        <h3 class="mt-5 mb-1 text-title font-semibold">Performance</h3>
        <dl class="ui-kv [&>dd]:text-right [&>dd]:font-mono">
          <dt>Rounds</dt><dd>{{ formatInteger(rounds(run)) }}</dd>
          <dt>Frames</dt><dd>{{ formatInteger(Number(run.frame || 0)) }}</dd>
          <dt>Runtime</dt><dd>{{ formatDuration(Number(run.runtime_seconds || 0)) }}</dd>
          <dt>Planner time</dt><dd>{{ formatDuration(plannerSeconds(run)) }}</dd>
          <dt>Strategic calls</dt><dd>{{ formatInteger(strategicCalls(run)) }}</dd>
          <dt>Plans executed</dt><dd>{{ formatInteger(statNumber(run, 'plan_executions')) }}</dd>
          <dt>Steps skipped</dt><dd>{{ formatInteger(statNumber(run, 'steps_skipped')) }}</dd>
          <dt>Rejected</dt><dd>{{ formatInteger(statNumber(run, 'rejected')) }}</dd>
          <dt>Prompt tokens</dt><dd>{{ formatInteger(statNumber(run, 'prompt_tokens')) }}</dd>
          <dt>Completion tokens</dt><dd>{{ formatInteger(statNumber(run, 'completion_tokens')) }}</dd>
        </dl>

        <h3 class="mt-5 mb-1 text-title font-semibold">Party</h3>
        <ul v-if="run.player?.party?.length" class="divide-y divide-rule border-b border-rule">
          <li v-for="(mon, index) in run.player.party" :key="index" class="flex justify-between py-1.5">
            <span>{{ mon.name }}</span><span class="font-mono text-fg-2">L{{ mon.level }}</span>
          </li>
        </ul>
        <p v-else class="text-fg-3">No party recorded.</p>

        <template v-if="showDecisionTelemetry(run.stats, run.decision_engine)">
          <h3 class="mt-5 mb-1 text-title font-semibold">Decisions</h3>
          <div class="border border-rule">
            <DecisionTelemetry :stats="run.stats" :engine="run.decision_engine" :show-feed="false" />
          </div>
        </template>

        <template v-if="run.status === 'done'">
          <h3 class="mt-5 mb-1 text-title font-semibold">Evidence</h3>
          <RunHistoricalEvidence v-if="run.status === 'done'" :run-id="run.run_id" />
        </template>
      </div>

      <footer class="flex shrink-0 flex-wrap items-center gap-2 border-t border-rule bg-raised px-3 py-2">
        <a :href="liveHref(run.run_id)" class="ui-btn ui-btn-primary" @click.prevent="emit('inspect', run.run_id)">Inspect in Live</a>
        <a v-if="recording?.canWatch" :href="replayVideoURL(run.run_id)" target="_blank" rel="noopener" class="ui-btn">▶ Watch replay</a>
        <a v-else-if="recording" :href="mediaHref" class="ui-btn" :title="recording.detail">{{ recording.label }}</a>
        <RunActionMenu class="ml-auto" up :run-id="run.run_id" :media-href="mediaHref" @inspect="emit('inspect', run.run_id)" @copy="emit('copy', run.run_id)" @delete="emit('delete', run)" />
      </footer>
    </template>
  </section>
</template>
