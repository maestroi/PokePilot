<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { artifactContentURL, highlightManifestURL, highlightVideoURL } from '../shared/api/client'
import type { DashboardRun, DashboardStats, DashboardWorker } from '../shared/api/types'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import { presentReplay } from '../shared/status'
import { operatorHref } from '../shared/urls'
import { isPlayStyleRun, playSpeedLabel, playStyleLabel, policyLabel } from '../shared/playstyle'
import DecisionTelemetry from './DecisionTelemetry.vue'
import { showDecisionTelemetry } from './decisionTelemetry'
import {
  decisionEngineLabel,
  fpsLabel,
  formatWhen,
  goalLabel,
  howText,
  llmProfileLabel,
  reasoningEffortLabel,
  starterLabel,
  statNumber,
  tileLabel
} from './operations'
import {
  eventDetail,
  eventGlyphs,
  eventKind,
  eventKindLabels,
  eventTime,
  eventTitle
} from './runTimelineFilters'
import type { RunEvidence } from './useRunEvidence'

const props = defineProps<{
  run: DashboardRun
  worker: DashboardWorker | null
  evidence: RunEvidence
}>()

const emit = defineEmits<{ 'watch-replay': [] }>()

const tabs = [
  { id: 'event', label: 'Event' },
  { id: 'planner', label: 'Planner' },
  { id: 'media', label: 'Media' },
  { id: 'checkpoints', label: 'Checkpoints' },
  { id: 'artifacts', label: 'Artifacts' },
  { id: 'details', label: 'Details' }
] as const
type TabID = typeof tabs[number]['id']

function initialTab(): TabID {
  const value = new URLSearchParams(window.location.search).get('tab')
  return tabs.some((tab) => tab.id === value) ? value as TabID : 'event'
}
const activeTab = ref<TabID>(initialTab())

function setTab(id: TabID): void {
  activeTab.value = id
  const url = new URL(window.location.href)
  url.searchParams.set('tab', id)
  history.replaceState(history.state, '', url)
}

function onTabKey(event: KeyboardEvent, index: number): void {
  const delta = event.key === 'ArrowRight' ? 1 : event.key === 'ArrowLeft' ? -1 : 0
  if (!delta) return
  event.preventDefault()
  const next = tabs[(index + delta + tabs.length) % tabs.length]
  setTab(next.id)
  document.getElementById(`inspector-tab-${next.id}`)?.focus()
}

const e = props.evidence
const selected = computed(() => e.timeline.value[e.selectedEvent.value] ?? null)
const selectedKind = computed(() => selected.value ? eventKind(selected.value) : 'event')

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

// Run details: the former Settings and Outcome / Current state panels.
const settingsRows = computed<[string, string][]>(() => {
  const run = props.run
  const rows: [string, string][] = [
    ['How', howText(run)],
    ['Starter', starterLabel(run)],
    ['Goal', goalLabel(run)]
  ]
  if (isPlayStyleRun(run)) {
    rows.push(
      ['Play style', playStyleLabel(run)],
      ['Purpose', run.purpose === 'debug_coverage' ? 'Debug coverage' : 'Normal'],
      ['Speed', playSpeedLabel(run)],
      ['Risk', policyLabel(run.risk_tolerance, 'balanced')],
      ['Wild encounters', policyLabel(run.wild_encounters, 'planner')],
      ['Model', llmProfileLabel(run)],
      ['Reasoning', reasoningEffortLabel(run)],
      ['Decision engine', decisionEngineLabel(run)],
      ['Recovery mode', run.recovery_profile || 'strict']
    )
  } else {
    rows.push(['Speed', playSpeedLabel(run)], ['Walk to', run.dest || '—'])
  }
  if (props.worker) rows.push(['Worker', props.worker.addr])
  rows.push(
    ['Seed', String(run.seed ?? 0)],
    ['Keep going', run.endless ? (run.random_seed ? 'Yes, random seed' : 'Yes, same seed') : ''],
    ['Queued', formatWhen(run.queued_at)],
    ['Ended', formatWhen(run.ended_at)],
    ['Round cap', run.planner === 'llm' ? (run.max_rounds ? String(run.max_rounds) : 'None (goal-driven)') : '']
  )
  return rows.filter(([, value]) => value)
})

const stateRows = computed<[string, string][]>(() => {
  const run = props.run
  const recoveries = run.recovery_attempts ? String(run.recovery_attempts) : ''
  const rows: [string, string][] = run.status === 'done'
    ? [['Ended', run.reason || 'done'], ['Detail', run.detail || ''], ['Last map', tileLabel(run)], ['Frame', String(run.frame ?? 0)], ['Speed', fpsLabel(run)], ['Attempts', String(run.attempts ?? 0)], ['Recoveries', recoveries]]
    : run.status === 'paused'
      ? [['Why paused', run.stop_so_far || 'Paused by operator'], ['Detail', run.detail || ''], ['Map', tileLabel(run)], ['Frame', String(run.frame ?? 0)], ['Attempts', String(run.attempts ?? 0)], ['Recoveries', recoveries]]
      : [['Map', tileLabel(run)], ['Frame', String(run.frame ?? 0)], ['Speed', fpsLabel(run)], ['Attempt', String((run.attempts ?? 0) + 1)], ['Recoveries', recoveries], ['So far', run.stop_so_far || '']]
  return rows.filter(([, value]) => value)
})

// Planner: the former Plan and Play panels.
const playRows = computed<[string, string, boolean][]>(() => {
  const run = props.run
  const stats = run.stats
  if (!stats || run.planner === 'scripted') return []
  const seconds = (value: unknown) => `${Number(value || 0).toFixed(1)}s`
  const average = (value: unknown) => Number(value || 0) > 0 ? seconds(value) : '—'
  const warn = (key: string) => statNumber(stats as DashboardStats, key) > 0
  return [
    ['Round', `${stats.round ?? '—'}${stats.rounds_left ? ` (${stats.rounds_left} left)` : ''}`, false],
    ['Model', [stats.model || '—', stats.backend].filter(Boolean).join(' · '), false],
    ['Repeat picks', `${statNumber(stats, 'repeats')} of ${stats.rounds ?? 0}`, Number(stats.rounds || 0) > 3 && statNumber(stats, 'repeats') * 2 >= Number(stats.rounds || 0)],
    ['Call latency', `${seconds(stats.last_seconds)} last · ${seconds(stats.avg_seconds)} avg`, false],
    ['Latency by outcome', `${average(stats.successful_avg_seconds)} ok · ${average(stats.rejected_avg_seconds)} rejected · ${average(stats.strategic_avg_seconds)} strategist`, false],
    ['Offered', `${statNumber(stats, 'avg_offered').toFixed(1)} avg`, false],
    ['Tokens', `${Number(stats.prompt_tokens ?? 0).toLocaleString()} in · ${Number(stats.completion_tokens ?? 0).toLocaleString()} out`, false],
    ['Rejected', String(stats.rejected ?? 0), warn('rejected')],
    ['Transport errors', String(statNumber(stats, 'transport')), warn('transport')],
    ['Fallbacks', String(statNumber(stats, 'fallbacks')), warn('fallbacks')]
  ]
})

const playChoices = computed(() => {
  const choices = props.run.stats?.choices
  return Array.isArray(choices) ? choices as { objective?: string; count?: number }[] : []
})
const playTop = computed(() => playChoices.value[0]?.count || 1)

// Media.
const replayStatus = computed(() => {
  const replay = e.replay.value
  const total = Number(replay?.segments || 0)
  const progress = total ? 100 * Number(replay?.segments_done || 0) / total : undefined
  return presentReplay(replay?.state, progress)
})
const youtubePrivacy = ref<'private' | 'unlisted' | 'public'>('unlisted')
const confirmCancelReplay = ref(false)
const confirmPublish = ref(false)

function highlightDuration(): string {
  const total = Math.max(0, Math.round(Number(e.highlights.value?.duration_ms || 0) / 1000))
  return total >= 60 ? `${Math.floor(total / 60)}m ${total % 60}s` : `${total}s`
}

function checkpointLabel(value: unknown, index: number): string {
  const row = (value && typeof value === 'object' ? value : {}) as Record<string, unknown>
  return String(row.name || row.kind || row.label || row.key || `Checkpoint ${index + 1}`)
}

function checkpointField(value: unknown, key: string): unknown {
  return value && typeof value === 'object' ? (value as Record<string, unknown>)[key] : undefined
}

function artifactMeta(artifact: { media_type?: string; size?: number; storage?: string; location?: string }): string {
  return [artifact.media_type, artifact.size ? `${Number(artifact.size).toLocaleString()} bytes` : '', artifact.storage || artifact.location].filter(Boolean).join(' · ') || 'Artifact'
}

const search = () => window.location.search

watch(() => props.run.run_id, () => { confirmCancelReplay.value = false; confirmPublish.value = false })
</script>

<template>
  <section class="ui-bay flex min-h-0 flex-col" aria-label="Inspector">
    <div class="flex shrink-0 gap-1 overflow-x-auto border-b border-rule px-2" role="tablist" aria-label="Inspector sections">
      <button
        v-for="(tab, index) in tabs"
        :id="`inspector-tab-${tab.id}`"
        :key="tab.id"
        type="button"
        role="tab"
        :aria-selected="activeTab === tab.id"
        :aria-controls="`inspector-panel-${tab.id}`"
        :tabindex="activeTab === tab.id ? 0 : -1"
        :class="activeTab === tab.id ? 'border-accent text-fg' : 'border-transparent text-fg-2 hover:text-fg'"
        class="h-10 shrink-0 border-b-2 px-2.5 text-body font-medium whitespace-nowrap"
        @click="setTab(tab.id)"
        @keydown="onTabKey($event, index)"
      >{{ tab.label }}</button>
    </div>

    <div
      :id="`inspector-panel-${activeTab}`"
      role="tabpanel"
      :aria-labelledby="`inspector-tab-${activeTab}`"
      class="min-h-0 flex-1 overflow-y-auto px-4 py-3"
    >
      <p v-if="e.error.value" class="mb-3 border border-warn/40 bg-warn/10 px-3 py-2 text-meta text-warn" role="status">
        ▲ Recorded evidence unavailable: {{ e.error.value }}
      </p>

      <!-- Event -->
      <template v-if="activeTab === 'event'">
        <template v-if="selected">
          <div class="flex items-center gap-2">
            <span class="text-[12px]" aria-hidden="true">{{ eventGlyphs[selectedKind] }}</span>
            <h3 class="text-title font-semibold">{{ eventKindLabels[selectedKind] }}</h3>
            <span class="font-mono text-meta text-fg-2">{{ eventTime(selected) }}</span>
          </div>
          <p class="mt-1 max-w-[72ch] font-medium">{{ eventTitle(selected) }}</p>
          <dl class="ui-kv mt-3">
            <template v-if="text(selected.decision)"><dt>Decision</dt><dd class="text-ok">{{ text(selected.decision) }}</dd></template>
            <template v-if="text(selected.source)"><dt>Source</dt><dd>{{ text(selected.source) }}</dd></template>
            <template v-if="Number(selected.frame)"><dt>Frame</dt><dd class="font-mono">{{ Number(selected.frame).toLocaleString() }}</dd></template>
            <template v-if="Number(selected.round)"><dt>Round</dt><dd class="font-mono">{{ selected.round }}</dd></template>
            <template v-if="Number(selected.attempt)"><dt>Attempt</dt><dd class="font-mono">{{ selected.attempt }}</dd></template>
            <template v-if="Number(selected.recovery_attempt)"><dt>Recovery attempt</dt><dd class="font-mono">{{ selected.recovery_attempt }}</dd></template>
          </dl>
          <h4 class="ui-label mt-4">Detail</h4>
          <pre class="mt-1 max-h-72 overflow-auto bg-canvas p-3 font-mono text-meta whitespace-pre-wrap text-fg">{{ eventDetail(selected) }}</pre>
          <p v-if="selectedKind === 'checkpoint'" class="mt-3 text-meta text-fg-2">Restarting from a checkpoint isn't available yet.</p>
          <a
            v-if="selectedKind === 'failure'"
            :href="operatorHref('failures', { run: run.run_id }, search())"
            class="ui-btn ui-btn-sm mt-3"
          >Open in Failures</a>
        </template>
        <p v-else class="py-8 text-center text-meta text-fg-3">
          {{ e.loading.value ? 'Loading recorded events…' : 'Choose an event on the timeline or in the story to inspect it.' }}
        </p>
      </template>

      <!-- Planner -->
      <template v-else-if="activeTab === 'planner'">
        <h3 class="ui-label">Latest decision</h3>
        <p :class="run.decision ? 'text-ok' : 'text-fg-3'" class="mt-1 max-w-[72ch] font-medium">
          {{ run.decision || (run.question ? 'Waiting for the planner reply' : 'Waiting for the first plan') }}
        </p>
        <template v-if="run.stats?.intent">
          <h3 class="ui-label mt-4">Intent</h3>
          <p class="mt-1">“{{ run.stats.intent }}” <span class="text-meta text-fg-2">· {{ run.stats.intent_age }} rounds</span></p>
        </template>
        <h3 class="ui-label mt-4">Question</h3>
        <pre class="mt-1 max-h-64 overflow-auto bg-canvas p-3 font-mono text-meta whitespace-pre-wrap text-fg">{{ run.question || 'No question asked yet.' }}</pre>
        <details v-if="run.raw" class="mt-2">
          <summary class="cursor-pointer text-meta font-medium text-accent">Show raw exchange</summary>
          <pre class="mt-1 max-h-96 overflow-auto bg-canvas p-3 font-mono text-meta whitespace-pre-wrap text-fg-2">{{ run.raw }}</pre>
        </details>

        <template v-if="playRows.length">
          <h3 class="ui-label mt-5">Play telemetry</h3>
          <dl class="ui-kv mt-1">
            <template v-for="[label, value, warn] in playRows" :key="label">
              <dt>{{ label }}</dt>
              <dd :class="warn ? 'text-warn' : ''" class="font-mono">{{ warn ? '▲ ' : '' }}{{ value }}</dd>
            </template>
          </dl>
        </template>
        <p v-else class="mt-5 text-meta text-fg-3">Play telemetry appears for LLM runs.</p>

        <template v-if="playChoices.length">
          <h3 class="ui-label mt-5">Objective choices</h3>
          <ul class="mt-1 space-y-1">
            <li v-for="choice in playChoices" :key="String(choice.objective)" class="grid grid-cols-[minmax(0,1fr)_7rem_2.5rem] items-center gap-2">
              <span class="truncate" :title="String(choice.objective)">{{ choice.objective }}</span>
              <span class="ui-meter"><span class="bg-accent" :style="{ width: `${100 * Number(choice.count || 0) / playTop}%` }" /></span>
              <span class="text-right font-mono text-meta">{{ choice.count }}</span>
            </li>
          </ul>
        </template>
        <div v-if="showDecisionTelemetry(run.stats, run.decision_engine)" class="mt-5 border border-rule">
          <DecisionTelemetry :stats="run.stats" :engine="run.decision_engine" />
        </div>
      </template>

      <!-- Replay & media -->
      <template v-else-if="activeTab === 'media'">
        <div class="flex items-center gap-2">
          <h3 class="text-title font-semibold">Replay</h3>
          <StatusPill :status="replayStatus" />
        </div>
        <p class="mt-1 max-w-[72ch] text-meta text-fg-2">Generated from the canonical run.gbrun recording. It plays inside the Game bay.</p>
        <div class="mt-3 flex flex-wrap gap-2">
          <button v-if="e.replay.value?.state === 'ready'" type="button" class="ui-btn ui-btn-sm" @click="emit('watch-replay')">▶ Watch in Game bay</button>
          <button
            v-else-if="e.replay.value && e.replay.value.state !== 'disabled' && e.replay.value.state !== 'generating'"
            type="button"
            class="ui-btn ui-btn-sm"
            :disabled="e.rendering.value"
            @click="e.requestReplay()"
          >{{ e.rendering.value ? 'Requesting…' : e.replay.value.state === 'error' ? 'Retry replay' : 'Generate replay' }}</button>
          <button
            v-if="e.replay.value?.state === 'generating' && e.replay.value.job_id"
            type="button"
            class="ui-btn ui-btn-sm ui-btn-danger"
            :disabled="e.cancellingReplay.value"
            @click="confirmCancelReplay = true"
          >Cancel generation…</button>
          <a
            :href="operatorHref('media', e.replay.value?.job_id ? { section: 'jobs', run: run.run_id, job: e.replay.value.job_id } : { section: 'library', run: run.run_id }, search())"
            class="ui-btn ui-btn-sm ui-btn-quiet"
          >Open in Media</a>
        </div>
        <p v-if="e.replayError.value" class="mt-2 text-meta text-warn" role="status">▲ {{ e.replayError.value }}</p>

        <div class="mt-6 flex items-center gap-2">
          <h3 class="text-title font-semibold">Highlights</h3>
          <StatusPill v-if="e.highlights.value" :status="presentReplay(e.highlights.value.state)" />
        </div>
        <p class="mt-1 max-w-[72ch] text-meta text-fg-2">
          <template v-if="e.highlights.value?.state === 'empty'">No highlight-worthy events were found.</template>
          <template v-else-if="e.highlights.value && e.highlights.value.state !== 'missing'">
            {{ e.highlights.value.windows || 0 }} window{{ Number(e.highlights.value.windows || 0) === 1 ? '' : 's' }} · {{ highlightDuration() }}
            <template v-if="e.highlights.value.state === 'generating'"> · {{ e.highlights.value.windows_done || 0 }}/{{ e.highlights.value.windows || 0 }} rendered</template>
          </template>
          <template v-else>A short reel built from badges, gyms, evolutions, blackouts, recoveries and run completion.</template>
        </p>
        <div class="mt-3 flex flex-wrap gap-2">
          <button
            v-if="e.highlights.value && !['ready', 'generating', 'empty', 'disabled'].includes(e.highlights.value.state)"
            type="button"
            class="ui-btn ui-btn-sm"
            :disabled="e.renderingHighlights.value"
            @click="e.requestHighlights()"
          >{{ e.renderingHighlights.value ? 'Requesting…' : 'Generate highlights' }}</button>
          <template v-if="e.highlights.value?.state === 'ready'">
            <a :href="highlightVideoURL(run.run_id)" class="ui-btn ui-btn-sm">Open highlight reel</a>
            <a :href="highlightManifestURL(run.run_id)" class="ui-btn ui-btn-sm ui-btn-quiet">Manifest</a>
          </template>
        </div>
        <p v-if="e.highlightError.value" class="mt-2 text-meta text-warn" role="status">▲ {{ e.highlightError.value }}</p>

        <template v-if="e.youtube.value?.configured && e.replay.value?.state === 'ready'">
          <h3 class="mt-6 text-title font-semibold">YouTube</h3>
          <div class="mt-2 flex flex-wrap items-center gap-2">
            <a
              v-if="e.youtube.value.publication.state === 'published' && e.youtube.value.publication.video_url"
              :href="e.youtube.value.publication.video_url"
              target="_blank"
              rel="noopener"
              class="ui-btn ui-btn-sm"
            >Published ({{ e.youtube.value.publication.privacy }}) ↗</a>
            <span v-else-if="e.youtube.value.publication.state === 'uploading'" class="ui-pill ui-tone-accent">◐ Uploading</span>
            <template v-else>
              <span v-if="['failed', 'interrupted'].includes(e.youtube.value.publication.state)" class="ui-pill ui-tone-danger" :title="e.youtube.value.publication.error">✕ {{ e.youtube.value.publication.state }}</span>
              <label class="flex items-center gap-2">
                <span class="ui-label">Visibility</span>
                <select v-model="youtubePrivacy" class="h-7 rounded-[3px] border border-control bg-surface px-2 text-body">
                  <option value="private">Private</option><option value="unlisted">Unlisted</option><option value="public">Public</option>
                </select>
              </label>
              <button type="button" class="ui-btn ui-btn-sm" :disabled="e.publishing.value" @click="confirmPublish = true">
                {{ e.youtube.value.publication.state === 'none' ? 'Publish to YouTube…' : 'Retry upload…' }}
              </button>
            </template>
          </div>
          <p v-if="e.youtubeError.value" class="mt-2 text-meta text-warn" role="status">▲ {{ e.youtubeError.value }}</p>
        </template>
      </template>

      <!-- Checkpoints -->
      <template v-else-if="activeTab === 'checkpoints'">
        <p class="mb-3 text-meta text-fg-2">Restarting from a checkpoint isn't available yet.</p>
        <ul v-if="e.checkpoints.value.length" class="border-t border-rule">
          <li v-for="(checkpoint, index) in e.checkpoints.value.slice(0, 60)" :key="index" class="border-b border-rule py-2">
            <div class="flex items-center gap-2">
              <span class="text-warn" aria-hidden="true">⚑</span>
              <span class="min-w-0 flex-1 truncate font-medium">{{ checkpointLabel(checkpoint, index) }}</span>
              <span v-if="Number(checkpointField(checkpoint, 'frame'))" class="font-mono text-meta text-fg-2">f {{ Number(checkpointField(checkpoint, 'frame')).toLocaleString() }}</span>
              <span v-if="checkpointField(checkpoint, 'replayable')" class="ui-pill ui-tone-ok">✓ Paired state</span>
            </div>
            <details class="mt-1 pl-6">
              <summary class="cursor-pointer text-meta text-accent">Raw</summary>
              <pre class="mt-1 max-h-48 overflow-auto bg-canvas p-2 font-mono text-meta whitespace-pre-wrap text-fg-2">{{ JSON.stringify(checkpoint, null, 2) }}</pre>
            </details>
          </li>
        </ul>
        <p v-else class="py-6 text-center text-meta text-fg-3">No persisted checkpoints exposed for this run.</p>
      </template>

      <!-- Artifacts -->
      <template v-else-if="activeTab === 'artifacts'">
        <ul v-if="e.artifacts.value.length" class="border-t border-rule">
          <li v-for="artifact in e.artifacts.value" :key="artifact.name" class="flex items-start gap-3 border-b border-rule py-2">
            <div class="min-w-0 flex-1">
              <p class="truncate font-mono text-meta font-medium" :title="artifact.name">{{ artifact.name }}</p>
              <p class="text-meta text-fg-2">{{ artifactMeta(artifact) }}</p>
              <details v-if="artifact.sha256">
                <summary class="cursor-pointer text-meta text-accent">sha256</summary>
                <p class="font-mono text-meta break-all text-fg-2 select-all">{{ artifact.sha256 }}</p>
              </details>
            </div>
            <a :href="artifactContentURL(run.run_id, artifact.name)" class="ui-btn ui-btn-sm shrink-0">Open</a>
          </li>
        </ul>
        <p v-else class="py-6 text-center text-meta text-fg-3">No artifacts indexed.</p>
      </template>

      <!-- Run details -->
      <template v-else>
        <h3 class="ui-label">Settings</h3>
        <dl class="ui-kv mt-1">
          <template v-for="[label, value] in settingsRows" :key="label">
            <dt>{{ label }}</dt><dd>{{ value }}</dd>
          </template>
        </dl>
        <h3 class="ui-label mt-5">{{ run.status === 'done' ? 'Outcome' : 'Current state' }}</h3>
        <dl class="ui-kv mt-1">
          <template v-for="[label, value] in stateRows" :key="label">
            <dt>{{ label }}</dt><dd>{{ value }}</dd>
          </template>
          <template v-if="run.experiment_id">
            <dt>Experiment</dt>
            <dd><a :href="operatorHref('analytics', { section: 'experiments', experiment: run.experiment_id }, search())" class="text-accent hover:underline">{{ run.experiment_arm ? run.experiment_arm.toUpperCase() + ' · ' : '' }}{{ run.experiment_id }}</a></dd>
          </template>
        </dl>
        <template v-if="run.trace">
          <h3 class="ui-label mt-5">Last trace line</h3>
          <p class="mt-1 font-mono text-meta break-words">{{ run.trace }}</p>
        </template>
        <details class="mt-5">
          <summary class="cursor-pointer text-meta font-medium text-accent">Show raw debug bundle</summary>
          <pre class="mt-1 max-h-96 overflow-auto bg-canvas p-3 font-mono text-meta whitespace-pre-wrap text-fg-2">{{ e.debug.value ? JSON.stringify(e.debug.value, null, 2) : 'No debug bundle loaded.' }}</pre>
        </details>
      </template>
    </div>

    <ConfirmDialog
      :open="confirmCancelReplay"
      :title="`Cancel replay generation for ${run.run_id}?`"
      message="The generation job stops. You can generate the replay again later from here or from Media."
      confirm-label="Cancel generation"
      cancel-label="Keep generating"
      :busy="e.cancellingReplay.value"
      danger
      @close="confirmCancelReplay = false"
      @confirm="e.cancelReplay().finally(() => { confirmCancelReplay = false })"
    />
    <ConfirmDialog
      :open="confirmPublish"
      :title="`Upload the replay of ${run.run_id} to YouTube?`"
      :message="`The video is uploaded as ${youtubePrivacy}.`"
      confirm-label="Upload"
      :busy="e.publishing.value"
      @close="confirmPublish = false"
      @confirm="e.publishYouTube(youtubePrivacy).finally(() => { confirmPublish = false })"
    />
  </section>
</template>
