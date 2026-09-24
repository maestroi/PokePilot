<script setup lang="ts">
import DecisionTelemetry from './DecisionTelemetry.vue'
import { hasDecisionTelemetry } from './decisionTelemetry'
import { computed, ref, watch } from 'vue'
import { ArrowPathIcon, EyeIcon, EyeSlashIcon, NoSymbolIcon, PauseIcon, PlayIcon, Square2StackIcon } from '@heroicons/vue/20/solid'
import { cancelRun, cloneRun, forceEndWorker, getDashboard, getRun, pauseRun, resumeRun } from '../shared/api/client'
import type { DashboardRun, DashboardStats, DashboardWorker, PartyMon } from '../shared/api/types'
import { getSpectatorControl, patchSpectatorRunControl } from '../shared/api/spectator-control'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { useFramePump } from '../shared/composables/useFramePump'
import { usePollingResource } from '../shared/composables/usePollingResource'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
import InspectorPanel from './InspectorPanel.vue'
import SemanticMap from './SemanticMap.vue'
import {
  fpsLabel,
  formatFrame,
  formatWhen,
  gameMediaLabel,
  goalLabel,
  howText,
  isLiveStatus,
  llmProfileLabel,
  railFacts,
  railStatusLabel,
  reasoningEffortLabel,
  decisionEngineLabel,
  starterLabel,
  statNumber,
  statsLine,
  statusTone,
  tileLabel
} from './operations'
import {
  isPlayStyleRun,
  playSpeedLabel,
  playStyleLabel,
  playStyleTagline,
  policyLabel
} from '../shared/playstyle'
import { bagItemsLabel, bagMeter, dexDetail, dexMeter, milestonesLabel } from '../shared/playerProgress'

const params = new URLSearchParams(window.location.search)
const selectedRunID = ref(params.get('run') || '')
const selectionPinned = ref(Boolean(selectedRunID.value))
const historicalRun = ref<DashboardRun | null>(null)
const historicalError = ref('')
const pausing = ref(false)
const resuming = ref(false)
const canceling = ref(false)
const cloning = ref(false)
const spectatorUpdating = ref(false)
const forceEndTarget = ref<DashboardWorker | null>(null)
const forceEndBusy = ref(false)
const actionError = ref('')
const copyState = ref('')
const cloneState = ref('')
let detailSerial = 0

const activeResource = usePollingResource(
  (signal) => getDashboard({ active: true }, signal),
  { intervalMs: 2000 }
)

const recentResource = usePollingResource(
  (signal) => getDashboard({ status: 'done', limit: 8 }, signal),
  { intervalMs: 10000 }
)

const spectatorResource = usePollingResource(
  (signal) => getSpectatorControl(signal),
  { intervalMs: 5000 }
)

const activeRuns = computed(() => [...(activeResource.data.value?.runs ?? [])]
  .filter((run) => run.status !== 'done')
  .sort((a, b) => Number(b.queued_at || 0) - Number(a.queued_at || 0)))
const recentRuns = computed(() => [...(recentResource.data.value?.runs ?? [])]
  .filter((run) => run.status === 'done')
  .sort((a, b) => Number(b.ended_at || 0) - Number(a.ended_at || 0)))
const knownRuns = computed(() => [...activeRuns.value, ...recentRuns.value])

const selectedRun = computed<DashboardRun | null>(() => {
  const id = selectedRunID.value
  if (id) {
    const known = knownRuns.value.find((run) => run.run_id === id)
    if (known) return known
    if (historicalRun.value?.run_id === id) return historicalRun.value
  }
  return activeRuns.value[0] || recentRuns.value[0] || null
})

watch(selectedRun, (run) => {
  if (run && !selectionPinned.value) selectedRunID.value = run.run_id
}, { immediate: true })

watch(selectedRunID, async (runID) => {
  const id = ++detailSerial
  historicalRun.value = null
  historicalError.value = ''
  if (!runID || knownRuns.value.some((run) => run.run_id === runID)) return
  try {
    const run = await getRun(runID)
    if (id === detailSerial) historicalRun.value = run
  } catch (cause) {
    if (id === detailSerial) historicalError.value = cause instanceof Error ? cause.message : 'Run not found'
  }
}, { immediate: true })

const selectedFrameID = computed(() => selectedRun.value?.run_id || '')
const isLiveFrame = computed(() => isLiveStatus(selectedRun.value?.status))
const frameEnabled = computed(() => Boolean(selectedFrameID.value) && selectedRun.value?.status !== 'queued')
const { frameURL, state: frameState, error: frameError } = useFramePump(selectedFrameID, frameEnabled, 50, isLiveFrame)
const gameLabel = computed(() => gameMediaLabel(selectedRun.value?.status))

const partySlots = computed<(PartyMon | null)[]>(() => {
  const members = selectedRun.value?.player?.party ?? []
  return Array.from({ length: 6 }, (_, index) => members[index] || null)
})
const badges = computed(() => selectedRun.value?.player?.badges ?? [])
const bagLabel = computed(() => bagMeter(selectedRun.value?.player))
const dexLabel = computed(() => dexMeter(selectedRun.value?.player))
const dexInfo = computed(() => dexDetail(selectedRun.value?.player))
const bagList = computed(() => bagItemsLabel(selectedRun.value?.player))
const milestoneList = computed(() => milestonesLabel(selectedRun.value?.player))
const showProgressLists = computed(() => Boolean(bagLabel.value || dexLabel.value))
const goalProgress = computed(() => {
  const stats = selectedRun.value?.stats
  if (stats?.goal_complete) return 100
  const current = Number(stats?.goal_current || 0)
  const target = Number(stats?.goal_target || 0)
  return target > 0 ? Math.max(0, Math.min(100, 100 * current / target)) : 0
})
const isPausable = computed(() => {
  const status = selectedRun.value?.status
  return status === 'queued' || status === 'leased' || status === 'running'
})
const isPaused = computed(() => selectedRun.value?.status === 'paused')
const isCancelable = computed(() => {
  const status = selectedRun.value?.status
  return status === 'queued' || status === 'leased' || status === 'running' || status === 'paused'
})
const selectedWorker = computed<DashboardWorker | null>(() => {
  const runID = selectedRun.value?.run_id
  if (!runID) return null
  return activeResource.data.value?.workers?.find((worker) => worker.run_id === runID) ?? null
})
const spectatorVisible = computed(() => {
  const runID = selectedRun.value?.run_id
  if (!runID) return true
  return spectatorResource.data.value?.runs?.[runID]?.visible ?? true
})
const spectatorControlReady = computed(() => Boolean(spectatorResource.data.value))
const forceEndTitle = computed(() => forceEndTarget.value ? `Force quit worker ${forceEndTarget.value.addr}?` : 'Force quit worker?')
const forceEndMessage = computed(() => {
  const worker = forceEndTarget.value
  if (!worker) return ''
  const runID = worker.run_id || selectedRun.value?.run_id || ''
  return `This immediately terminates worker ${worker.addr} and force-ends run ${runID}. The run becomes terminal: it will not be retried and an endless successor will not be created.\n\nUse this only when pause/cancel cannot recover the run.`
})
const resourceState = computed(() => {
  if (activeResource.state.value === 'error' && recentResource.state.value === 'error') return 'error'
  if (activeResource.state.value === 'loading' && recentResource.state.value === 'loading') return 'loading'
  if (activeResource.state.value === 'stale' || recentResource.state.value === 'stale') return 'stale'
  if (!selectedRun.value && (activeResource.state.value === 'empty' || recentResource.state.value === 'empty')) return 'empty'
  return 'ready'
})
const latestDecision = computed(() => {
  const run = selectedRun.value
  if (!run) return '—'
  if (run.decision) return run.decision
  if (run.question) return 'Waiting for planner response'
  return 'Waiting for first decision'
})
const settingsRows = computed(() => {
  const run = selectedRun.value
  if (!run) return []
  const rows: [string, string][] = [
    ['how', howText(run)],
    ['starter', starterLabel(run)],
    ['goal', goalLabel(run)]
  ]
  if (isPlayStyleRun(run)) {
    rows.push(
      ['play style', playStyleLabel(run)],
      ['purpose', run.purpose === 'debug_coverage' ? 'debug coverage' : 'normal'],
      ['speed', playSpeedLabel(run)],
      ['risk', policyLabel(run.risk_tolerance, 'balanced')],
      ['wild encounters', policyLabel(run.wild_encounters, 'planner')]
    )
    rows.push(
      ['model', llmProfileLabel(run)],
      ['reasoning', reasoningEffortLabel(run)],
      ['decision engine', decisionEngineLabel(run)],
      ['recovery mode', run.recovery_profile || 'strict']
    )
  } else {
    rows.push(['speed', playSpeedLabel(run)], ['walk to', run.dest || '—'])
  }
  if (selectedWorker.value) rows.push(['worker', selectedWorker.value.addr])
  rows.push(
    ['seed', String(run.seed ?? 0)],
    ['keep going', run.endless ? (run.random_seed ? 'yes, random seed' : 'yes, same seed') : ''],
    ['queued', formatWhen(run.queued_at)],
    ['ended', formatWhen(run.ended_at)],
    ['round cap', run.planner === 'llm' ? (run.max_rounds ? String(run.max_rounds) : 'none (goal-driven)') : '']
  )
  return rows.filter(([, value]) => value)
})
const stateRows = computed(() => {
  const run = selectedRun.value
  if (!run) return []
  if (run.status === 'done') {
    return [
      ['ended', run.reason || 'done'],
      ['detail', run.detail || ''],
      ['last map', tileLabel(run)],
      ['frame', String(run.frame ?? 0)],
      ['fps', fpsLabel(run)],
      ['attempts', String(run.attempts ?? 0)],
      ['recoveries', run.recovery_attempts ? String(run.recovery_attempts) : '']
    ].filter(([, value]) => value)
  }
  if (run.status === 'paused') {
    return [
      ['status', 'paused'],
      ['why', run.stop_so_far || 'paused by operator'],
      ['detail', run.detail || ''],
      ['map', tileLabel(run)],
      ['frame', String(run.frame ?? 0)],
      ['attempts', String(run.attempts ?? 0)],
      ['recoveries', run.recovery_attempts ? String(run.recovery_attempts) : '']
    ].filter(([, value]) => value)
  }
  return [
    ['status', run.status],
    ['map', tileLabel(run)],
    ['frame', String(run.frame ?? 0)],
    ['fps', fpsLabel(run)],
    ['attempt', String((run.attempts ?? 0) + 1)],
    ['recoveries', run.recovery_attempts ? String(run.recovery_attempts) : ''],
    ['so far', run.stop_so_far || '']
  ].filter(([, value]) => value)
})
const playRows = computed(() => {
  const run = selectedRun.value
  const stats = run?.stats
  if (!run || !stats || run.planner === 'scripted') return []
  const seconds = (value: unknown) => `${Number(value || 0).toFixed(1)}s`
  const average = (value: unknown) => Number(value || 0) > 0 ? seconds(value) : '—'
  const model = [stats.model || '—', stats.backend].filter(Boolean).join(' · ')
  return [
    ['round', `${stats.round ?? '—'}${stats.rounds_left ? ` (${stats.rounds_left} left)` : ''}`],
    ['model', model],
    ['repeat picks', `${statNumber(stats, 'repeats')} of ${stats.rounds ?? 0}`],
    ['call latency', `${seconds(stats.last_seconds)} last / ${seconds(stats.avg_seconds)} all avg`],
    ['latency avg', `${average(stats.successful_avg_seconds)} ok / ${average(stats.rejected_avg_seconds)} rejected / ${average(stats.strategic_avg_seconds)} strategist`],
    ['offered', `${statNumber(stats, 'avg_offered').toFixed(1)} avg`],
    ['tokens', `${stats.prompt_tokens ?? 0} / ${stats.completion_tokens ?? 0}`],
    ['rejected', String(stats.rejected ?? 0)],
    ['transport', String(statNumber(stats, 'transport'))],
    ['fallbacks', String(statNumber(stats, 'fallbacks'))]
  ]
})
const playChoices = computed(() => {
  const choices = selectedRun.value?.stats?.choices
  return Array.isArray(choices) ? choices as { objective?: string; count?: number }[] : []
})
const playTop = computed(() => playChoices.value[0]?.count || 1)

function selectRunID(runID: string): void {
  selectedRunID.value = runID
  selectionPinned.value = true
  const url = new URL(window.location.href)
  url.searchParams.set('run', runID)
  history.replaceState(null, '', url)
}

function selectRun(run: DashboardRun): void {
  selectRunID(run.run_id)
}

function hpPercent(mon: PartyMon): number {
  if (!mon.max_hp) return 0
  return Math.max(0, Math.min(100, 100 * mon.hp / mon.max_hp))
}

function hpTone(mon: PartyMon): string {
  const percent = hpPercent(mon)
  if (!mon.max_hp || mon.hp === 0 || percent < 20) return 'bg-[var(--poke-red)]'
  if (percent < 50) return 'bg-[var(--poke-amber)]'
  return 'bg-[var(--poke-green)]'
}

function showActiveHeading(index: number): boolean {
  return index === 0 && knownRuns.value[0]?.status !== 'done'
}

function showEndedHeading(index: number): boolean {
  const run = knownRuns.value[index]
  const previous = knownRuns.value[index - 1]
  return run?.status === 'done' && previous?.status !== 'done'
}

function refresh(): void {
  void activeResource.retry()
  void recentResource.retry()
  void spectatorResource.retry()
}

async function pauseSelected(): Promise<void> {
  const run = selectedRun.value
  if (!run || !isPausable.value || pausing.value) return
  pausing.value = true
  actionError.value = ''
  try {
    await pauseRun(run.run_id)
    await activeResource.retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Pause failed'
  } finally {
    pausing.value = false
  }
}

async function resumeSelected(): Promise<void> {
  const run = selectedRun.value
  if (!run || !isPaused.value || resuming.value) return
  resuming.value = true
  actionError.value = ''
  try {
    await resumeRun(run.run_id)
    await activeResource.retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Resume failed'
  } finally {
    resuming.value = false
  }
}

async function cancelSelected(): Promise<void> {
  const run = selectedRun.value
  if (!run || !isCancelable.value || canceling.value) return
  canceling.value = true
  actionError.value = ''
  try {
    await cancelRun(run.run_id)
    await activeResource.retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Cancel failed'
  } finally {
    canceling.value = false
  }
}

async function cloneSelected(): Promise<void> {
  const run = selectedRun.value
  if (!run || cloning.value) return
  cloning.value = true
  cloneState.value = ''
  actionError.value = ''
  try {
    const result = await cloneRun(run.run_id)
    await Promise.all([activeResource.retry(), spectatorResource.retry()])
    cloneState.value = 'Cloned'
    selectRunID(result.run_id)
    window.setTimeout(() => { cloneState.value = '' }, 1800)
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Clone failed'
  } finally {
    cloning.value = false
  }
}

async function toggleSpectatorSelected(): Promise<void> {
  const run = selectedRun.value
  if (!run || spectatorUpdating.value || !spectatorControlReady.value) return
  spectatorUpdating.value = true
  actionError.value = ''
  try {
    await patchSpectatorRunControl(run.run_id, { visible: !spectatorVisible.value })
    await spectatorResource.retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Spectator visibility update failed'
  } finally {
    spectatorUpdating.value = false
  }
}

function requestForceEndSelected(): void {
  if (selectedWorker.value) forceEndTarget.value = selectedWorker.value
}

function closeForceEnd(): void {
  if (!forceEndBusy.value) forceEndTarget.value = null
}

async function confirmForceEnd(): Promise<void> {
  const worker = forceEndTarget.value
  if (!worker || forceEndBusy.value) return
  forceEndBusy.value = true
  actionError.value = ''
  try {
    await forceEndWorker(worker.addr)
    forceEndTarget.value = null
    await Promise.all([activeResource.retry(), recentResource.retry()])
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Force quit failed'
  } finally {
    forceEndBusy.value = false
  }
}

async function copyRunID(): Promise<void> {
  const run = selectedRun.value
  if (!run) return
  try {
    await navigator.clipboard.writeText(run.run_id)
    copyState.value = 'Copied'
  } catch {
    copyState.value = 'Copy failed'
  }
  window.setTimeout(() => { copyState.value = '' }, 1500)
}

function warnPlay(stats: DashboardStats | undefined, key: string): boolean {
  if (key === 'repeat picks') return Number(stats?.rounds || 0) > 3 && statNumber(stats, 'repeats') * 2 >= Number(stats?.rounds || 0)
  if (key === 'rejected' || key === 'transport' || key === 'fallbacks') return statNumber(stats, key) > 0
  return false
}
</script>

<template>
  <ResourceState
    :state="resourceState"
    title="Live run data unavailable"
    :message="historicalError || activeResource.error.value || recentResource.error.value || 'The console will recover automatically when the wall is reachable.'"
    :rows="8"
  >
    <template #actions>
      <button type="button" class="inline-flex items-center gap-1.5 rounded-sm bg-white/10 px-2 py-1 text-[11px] font-semibold text-white ring-1 ring-white/10 hover:bg-white/15" @click="refresh">
        <ArrowPathIcon class="size-3.5" aria-hidden="true" /> Retry now
      </button>
    </template>

    <div v-if="selectedRun" class="grid grid-cols-1 gap-2 xl:grid-cols-[13.125rem_minmax(0,1fr)]">
      <aside class="overflow-hidden border border-[var(--poke-border)] bg-[#101620] xl:sticky xl:top-12 xl:h-[calc(100vh-3.5rem)]">
        <div class="flex h-12 items-center justify-between border-b border-[var(--poke-border)] px-2.5">
          <div>
            <h2 class="text-[13px] font-semibold text-white">Runs</h2>
            <p class="text-[10px] text-[var(--poke-muted)]">Active first, then recent</p>
          </div>
        </div>
        <div class="max-h-56 overflow-y-auto xl:max-h-none xl:h-[calc(100%-3rem)]">
          <p v-if="!knownRuns.length" class="px-2.5 py-3 text-[12px] text-[var(--poke-muted)]">No runs yet.</p>
          <template v-for="(run, index) in knownRuns" :key="run.run_id">
            <p v-if="showActiveHeading(index)" class="poke-kicker px-2.5 pt-2 pb-1">Active</p>
            <p v-else-if="showEndedHeading(index)" class="poke-kicker px-2.5 pt-2 pb-1">Ended</p>
            <button
              type="button"
              :class="[
                selectedRun.run_id === run.run_id
                  ? 'border-l-[var(--poke-cyan)] bg-[#242f40]'
                  : 'border-l-transparent hover:bg-[var(--poke-panel)]',
                'grid min-h-14 w-full grid-cols-[3.75rem_minmax(0,1fr)] gap-2 border-b border-[var(--poke-border)] border-l-2 px-1.5 py-1.5 text-left'
              ]"
              @click="selectRun(run)"
            >
              <span class="relative grid h-14 place-items-center overflow-hidden bg-[#0c1118]">
                <img
                  v-if="run.status !== 'queued'"
                  :src="`/frame?run=${encodeURIComponent(run.run_id)}`"
                  alt=""
                  :class="[
                    'h-full w-full object-contain [image-rendering:pixelated]',
                    run.status === 'done' ? 'opacity-55 grayscale' : ''
                  ]"
                />
                <span v-else class="font-mono text-[9px] text-[var(--poke-dim)]">{{ run.status }}</span>
                <span
                  v-if="run.status === 'running'"
                  class="absolute top-1 left-1 size-1.5 rounded-full bg-[var(--poke-green)] motion-safe:animate-pulse"
                  aria-hidden="true"
                />
              </span>
              <span class="min-w-0">
                <span class="flex flex-wrap items-center gap-1">
                  <StatusBadge :tone="statusTone(run.status)">{{ railStatusLabel(run) }}</StatusBadge>
                  <StatusBadge v-if="isPlayStyleRun(run)" tone="info">{{ playStyleLabel(run) }}</StatusBadge>
                  <StatusBadge v-if="run.planner === 'llm' && run.purpose === 'debug_coverage'" tone="warning">debug coverage</StatusBadge>
                  <StatusBadge v-if="run.planner === 'llm' && run.recovery_profile" :tone="run.recovery_profile === 'resilient' ? 'warning' : 'neutral'">{{ run.recovery_profile }}</StatusBadge>
                </span>
                <span class="mt-0.5 block truncate font-mono text-[10px] font-bold text-white" :title="run.run_id">{{ run.run_id }}</span>
                <span class="block truncate text-[10px] text-[var(--poke-muted)]">{{ tileLabel(run) }}</span>
                <span
                  :class="[
                    run.status === 'running' ? 'text-[var(--poke-green)]' : 'text-[var(--poke-muted)]',
                    'block truncate text-[10px]'
                  ]"
                >{{ railFacts(run) }}</span>
                <span v-if="statsLine(run)" class="block truncate text-[10px] text-[var(--poke-dim)]">{{ statsLine(run) }}</span>
              </span>
            </button>
          </template>
        </div>
      </aside>

      <div class="min-w-0 space-y-2">
        <div class="grid h-auto grid-cols-1 gap-px overflow-hidden border border-[var(--poke-border)] bg-[var(--poke-border)] xl:h-[clamp(300px,36vh,330px)] xl:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)_minmax(13rem,0.55fr)]">
          <section class="flex min-w-0 flex-col bg-[var(--poke-panel)] xl:min-h-0">
            <header class="flex h-8 shrink-0 items-center justify-between border-b border-[var(--poke-border)] bg-[#0f141c] px-2.5">
              <h3 class="text-xs font-semibold text-white">Game</h3>
              <span class="inline-flex items-center gap-1.5 font-mono text-[10px]">
                <span
                  v-if="isLiveFrame"
                  class="size-1.5 rounded-full bg-[var(--poke-green)] motion-safe:animate-pulse"
                  aria-hidden="true"
                />
                <span :class="isLiveFrame ? 'font-semibold text-[var(--poke-green)]' : 'text-[var(--poke-muted)]'">{{ gameLabel }}</span>
              </span>
            </header>
            <div class="relative aspect-[160/144] min-h-52 w-full max-h-[min(52vh,26.875rem)] overflow-hidden bg-[#0c1118] xl:aspect-auto xl:h-auto xl:max-h-none xl:min-h-0 xl:flex-1">
              <img
                v-if="frameURL"
                :src="frameURL"
                :alt="`Game frame for ${selectedRun.run_id}`"
                class="absolute inset-0 h-full w-full object-contain object-center [image-rendering:pixelated]"
              />
              <div v-else class="absolute inset-0 grid place-items-center px-4 text-center">
                <div>
                  <span :class="['mx-auto block size-1.5 rounded-full', isLiveFrame ? 'bg-[var(--poke-green)] motion-safe:animate-pulse' : 'bg-[var(--poke-dim)]']" />
                  <p class="mt-2 text-[12px] text-[var(--poke-muted)]">{{ isLiveFrame ? 'Waiting for a live frame' : 'Last recorded frame unavailable.' }}</p>
                  <p v-if="frameError" class="mt-1 text-[11px] text-[var(--poke-amber)]">{{ frameError }}</p>
                </div>
              </div>
              <div
                v-if="frameURL"
                :class="[
                  isLiveFrame ? 'text-[var(--poke-green)]' : 'text-[var(--poke-muted)]',
                  'absolute top-2 left-2 inline-flex items-center gap-1 bg-black/75 px-1.5 py-0.5 text-[10px] font-bold'
                ]"
              >
                <span
                  v-if="isLiveFrame"
                  class="size-1.5 rounded-full bg-[var(--poke-green)] motion-safe:animate-pulse"
                  aria-hidden="true"
                />
                {{ isLiveFrame ? 'Live' : 'Ended' }}
              </div>
              <div v-if="frameURL && frameState === 'error'" class="absolute right-2 bottom-2 bg-black/70 px-1.5 py-0.5 text-[10px] text-[var(--poke-amber)]">Last frame · reconnecting</div>
            </div>
          </section>

          <section class="flex min-w-0 flex-col bg-[var(--poke-panel)] xl:min-h-0">
            <header class="flex h-8 shrink-0 items-center justify-between border-b border-[var(--poke-border)] bg-[#0f141c] px-2.5">
              <h3 class="text-xs font-semibold text-white">Semantic map</h3>
              <span class="font-mono text-[10px] text-[var(--poke-muted)]">{{ tileLabel(selectedRun) }}</span>
            </header>
            <div class="h-64 min-h-52 xl:h-auto xl:min-h-0 xl:flex-1">
              <SemanticMap :map="selectedRun.map" :x="selectedRun.x" :y="selectedRun.y" :trail="selectedRun.trail" :sprites="selectedRun.sprites" />
            </div>
            <div class="flex flex-wrap gap-x-3 gap-y-0.5 border-t border-[var(--poke-border)] px-2.5 py-1 text-[10px] text-[var(--poke-muted)]">
              <span><b class="text-[var(--poke-text)]">@</b> player</span>
              <span><b class="text-[var(--poke-text)]">■</b> sprite</span>
              <span><b class="text-[var(--poke-text)]">·</b> trail</span>
              <span><b class="text-[var(--poke-text)]">W</b> warp</span>
              <span><b class="text-[var(--poke-text)]">#</b> blocked</span>
            </div>
          </section>

          <section class="flex min-h-0 min-w-0 flex-col bg-[var(--poke-panel)]">
            <header class="flex h-8 shrink-0 items-center justify-between border-b border-[var(--poke-border)] bg-[#0f141c] px-2.5">
              <h3 class="text-xs font-semibold text-white">Game state</h3>
              <span class="flex max-w-[70%] flex-wrap justify-end gap-x-1.5 font-mono text-[10px] text-[var(--poke-amber)]">
                <span>₽{{ Number(selectedRun.player?.money || 0).toLocaleString() }}</span>
                <span v-if="bagLabel">bag {{ bagLabel }}</span>
                <span v-if="dexLabel">dex {{ dexLabel }}</span>
                <span>{{ badges.length ? badges.join(', ') : 'no badges' }}</span>
              </span>
            </header>
            <div class="flex min-h-52 flex-1 flex-col overflow-auto xl:min-h-0">
              <div class="grid flex-1 grid-cols-2 gap-px bg-[var(--poke-border)]">
                <div
                  v-for="(mon, index) in partySlots"
                  :key="index"
                  :class="[mon ? 'bg-[var(--poke-panel)]' : 'bg-[var(--poke-panel)] text-[var(--poke-dim)]', 'grid min-h-[38px] grid-cols-[minmax(0,1fr)_auto] content-center gap-x-1.5 gap-y-0.5 px-1.5 py-1 text-[10px]']"
                >
                  <template v-if="mon">
                    <strong class="truncate text-[var(--poke-text)]">{{ mon.name }}</strong>
                    <span>Lv.{{ mon.level }}</span>
                    <span>{{ mon.hp }}/{{ mon.max_hp }}</span>
                    <span v-if="mon.status" class="text-[var(--poke-red)]">{{ mon.status }}</span>
                    <span v-else />
                    <div class="col-span-2 h-0.5 bg-[#263240]"><div :class="['h-full', hpTone(mon)]" :style="{ width: `${hpPercent(mon)}%` }" /></div>
                  </template>
                  <template v-else>
                    <span>Empty slot</span>
                    <span class="font-mono">{{ index + 1 }}/6</span>
                  </template>
                </div>
              </div>
              <div v-if="showProgressLists" class="space-y-0.5 border-t border-[var(--poke-border)] bg-[#0f141c] px-1.5 py-1 text-[10px] leading-4">
                <p v-if="bagLabel" class="min-w-0 truncate" :title="bagList">
                  <span class="text-[var(--poke-muted)]">Bag {{ bagLabel }}</span>
                  <span class="text-[var(--poke-text)]"> {{ bagList }}</span>
                </p>
                <p v-if="dexInfo" class="min-w-0 truncate">
                  <span class="text-[var(--poke-muted)]">Dex {{ dexLabel }}</span>
                  <span class="text-[var(--poke-text)]"> {{ dexInfo }}</span>
                </p>
                <p class="min-w-0 truncate" :title="milestoneList">
                  <span class="text-[var(--poke-muted)]">Milestones</span>
                  <span class="text-[var(--poke-text)]"> {{ milestoneList }}</span>
                </p>
              </div>
            </div>
          </section>
        </div>

        <div class="grid overflow-hidden border border-[var(--poke-border)] bg-[var(--poke-panel)] xl:grid-cols-[minmax(10rem,1.1fr)_minmax(7rem,0.65fr)_minmax(9rem,0.9fr)_minmax(13rem,1.45fr)_5rem_4.25rem_auto]">
          <div class="min-w-0 border-b border-[var(--poke-border)] px-2.5 py-1.5 xl:border-r xl:border-b-0">
            <span class="poke-kicker">Run state</span>
            <strong class="mt-0.5 block truncate font-mono text-[11px]" :title="selectedRun.run_id">{{ selectedRun.run_id }}</strong>
            <div class="mt-1 flex flex-wrap items-center gap-1">
              <StatusBadge :tone="statusTone(selectedRun.status)">{{ selectedRun.status }}</StatusBadge>
              <StatusBadge v-if="isPlayStyleRun(selectedRun)" tone="info">{{ playStyleLabel(selectedRun) }}</StatusBadge>
              <span class="font-mono text-[10px] text-[var(--poke-muted)]">{{ playSpeedLabel(selectedRun) }}</span>
              <span
                v-if="isLiveFrame"
                class="inline-flex items-center gap-1 text-[10px] font-semibold text-[var(--poke-green)]"
              >
                <span class="size-1.5 rounded-full bg-[var(--poke-green)] motion-safe:animate-pulse" aria-hidden="true" />
                Live
              </span>
              <span v-else-if="selectedRun.status === 'paused'" class="text-[10px] font-semibold text-[var(--poke-amber)]">Paused · safe to deploy and resume</span>
              <span v-else-if="selectedRun.status === 'done'" class="text-[10px] text-[var(--poke-muted)]">Ended</span>
              <StatusBadge v-if="selectedRun.replay_available" tone="success">replay</StatusBadge>
            </div>
            <p v-if="isPlayStyleRun(selectedRun)" class="mt-0.5 truncate text-[10px] text-[var(--poke-muted)]">{{ playStyleTagline(selectedRun) }}</p>
          </div>
          <div class="min-w-0 border-b border-[var(--poke-border)] px-2.5 py-1.5 xl:border-r xl:border-b-0">
            <span class="text-[9px] tracking-[0.04em] text-[var(--poke-muted)] uppercase">Location</span>
            <strong class="mt-0.5 block truncate font-mono text-[11px]">{{ tileLabel(selectedRun) }}</strong>
          </div>
          <div class="min-w-0 border-b border-[var(--poke-border)] px-2.5 py-1.5 xl:border-r xl:border-b-0">
            <span class="text-[9px] tracking-[0.04em] text-[var(--poke-muted)] uppercase">Objective</span>
            <strong class="mt-0.5 block truncate text-[11px] text-[var(--poke-amber)]">{{ goalLabel(selectedRun) }}</strong>
            <div v-if="selectedRun.stats?.goal_summary" class="mt-1 grid grid-cols-[minmax(0,1fr)_auto] gap-x-1.5">
              <span class="truncate text-[9px] text-[var(--poke-muted)]">{{ selectedRun.stats.goal_summary }}</span>
              <span class="font-mono text-[9px] text-[var(--poke-green)]">{{ selectedRun.stats.goal_target ? `${selectedRun.stats.goal_current || 0} / ${selectedRun.stats.goal_target}` : 'in progress' }}</span>
              <div class="col-span-2 mt-0.5 h-0.5 bg-[#0f141c]"><div class="h-full bg-[var(--poke-green)]" :style="{ width: `${goalProgress}%` }" /></div>
            </div>
          </div>
          <div class="min-w-0 border-b border-[var(--poke-border)] px-2.5 py-1.5 xl:border-r xl:border-b-0">
            <span class="text-[9px] tracking-[0.04em] text-[var(--poke-muted)] uppercase">Latest decision</span>
            <strong class="mt-0.5 block line-clamp-2 text-[11px] leading-4">{{ latestDecision }}</strong>
          </div>
          <div class="min-w-0 border-b border-[var(--poke-border)] px-2.5 py-1.5 xl:border-r xl:border-b-0">
            <span class="text-[9px] tracking-[0.04em] text-[var(--poke-muted)] uppercase">Frame</span>
            <strong class="mt-0.5 block font-mono text-[11px]">{{ formatFrame(selectedRun.frame) }}</strong>
          </div>
          <div class="min-w-0 border-b border-[var(--poke-border)] px-2.5 py-1.5 xl:border-r xl:border-b-0">
            <span class="text-[9px] tracking-[0.04em] text-[var(--poke-muted)] uppercase">Round</span>
            <strong class="mt-0.5 block font-mono text-[11px]">{{ selectedRun.stats?.round ?? selectedRun.stats?.rounds ?? '—' }}</strong>
          </div>
          <div class="flex flex-wrap items-center justify-end gap-1 px-2 py-1.5">
            <button type="button" class="rounded-sm px-1.5 py-1 text-[10px] font-bold ring-1 ring-[var(--poke-border-strong)] hover:bg-white/5" @click="copyRunID">{{ copyState || 'Copy' }}</button>
            <button type="button" :disabled="cloning" class="inline-flex items-center gap-1 rounded-sm px-1.5 py-1 text-[10px] font-bold ring-1 ring-[var(--poke-border-strong)] hover:bg-white/5 disabled:opacity-50" @click="cloneSelected">
              <Square2StackIcon class="size-3" aria-hidden="true" /> {{ cloning ? 'Cloning…' : (cloneState || 'Clone') }}
            </button>
            <button
              type="button"
              :disabled="spectatorUpdating || !spectatorControlReady"
              :title="spectatorControlReady ? (spectatorVisible ? 'Visible in spectator mode. Click to hide.' : 'Hidden from spectator mode. Click to show.') : 'Spectator controls unavailable.'"
              :class="[
                spectatorVisible ? 'bg-[#18362f] text-[var(--poke-green)] ring-[#315f52]' : 'bg-[#2b3038] text-[var(--poke-muted)] ring-[var(--poke-border-strong)]',
                'inline-flex items-center gap-1 rounded-sm px-1.5 py-1 text-[10px] font-bold ring-1 hover:brightness-110 disabled:opacity-50'
              ]"
              @click="toggleSpectatorSelected"
            >
              <EyeIcon v-if="spectatorVisible" class="size-3" aria-hidden="true" />
              <EyeSlashIcon v-else class="size-3" aria-hidden="true" />
              {{ spectatorUpdating ? 'Updating…' : (spectatorVisible ? 'Spectator visible' : 'Spectator hidden') }}
            </button>
            <button v-if="isPausable" type="button" :disabled="pausing" class="inline-flex items-center gap-1 rounded-sm bg-[#3b3222] px-1.5 py-1 text-[10px] font-bold text-[var(--poke-amber)] ring-1 ring-[#6b5632] hover:brightness-110 disabled:opacity-50" @click="pauseSelected">
              <PauseIcon class="size-3" aria-hidden="true" /> {{ pausing ? 'Pausing…' : 'Pause' }}
            </button>
            <button v-if="isPaused" type="button" :disabled="resuming" class="inline-flex items-center gap-1 rounded-sm bg-[#18362f] px-1.5 py-1 text-[10px] font-bold text-[var(--poke-green)] ring-1 ring-[#315f52] hover:brightness-110 disabled:opacity-50" @click="resumeSelected">
              <PlayIcon class="size-3" aria-hidden="true" /> {{ resuming ? 'Resuming…' : 'Resume' }}
            </button>
            <button v-if="isCancelable" type="button" :disabled="canceling" class="inline-flex items-center gap-1 rounded-sm bg-[#352529] px-1.5 py-1 text-[10px] font-bold text-[#e4b5b7] ring-1 ring-[#654047] hover:brightness-110 disabled:opacity-50" @click="cancelSelected">
              <NoSymbolIcon class="size-3" aria-hidden="true" /> {{ canceling ? 'Canceling…' : 'Cancel' }}
            </button>
            <button
              v-if="selectedWorker"
              type="button"
              :disabled="forceEndBusy"
              :title="`Force quit worker ${selectedWorker.addr} for this run`"
              class="inline-flex items-center gap-1 rounded-sm bg-[#4a2026] px-1.5 py-1 text-[10px] font-bold text-[#ffd4d6] ring-1 ring-[#8b3f4a] hover:brightness-110 disabled:opacity-50"
              @click="requestForceEndSelected"
            >
              <NoSymbolIcon class="size-3" aria-hidden="true" /> Force quit
            </button>
          </div>
        </div>

        <div class="overflow-hidden border border-[var(--poke-border)] bg-[var(--poke-border)]">
          <div class="grid gap-px xl:grid-cols-4 xl:min-h-[260px]">
            <section class="min-w-0 bg-[var(--poke-panel)] p-2">
              <h3 class="mb-1.5 text-[9px] tracking-[0.07em] text-[var(--poke-muted)] uppercase">Settings</h3>
              <dl class="grid grid-cols-[minmax(4.25rem,0.42fr)_minmax(0,1fr)] gap-x-2 gap-y-1 text-[11px]">
                <template v-for="[label, value] in settingsRows" :key="label">
                  <dt class="text-[var(--poke-muted)]">{{ label }}</dt>
                  <dd class="min-w-0 overflow-hidden break-words text-[var(--poke-text)]">{{ value }}</dd>
                </template>
              </dl>
            </section>

            <section class="min-w-0 bg-[var(--poke-panel)] p-2">
              <h3 class="mb-1.5 text-[9px] tracking-[0.07em] text-[var(--poke-muted)] uppercase">{{ selectedRun.status === 'done' ? 'Outcome' : 'Current state' }}</h3>
              <dl class="grid grid-cols-[minmax(4.25rem,0.42fr)_minmax(0,1fr)] gap-x-2 gap-y-1 text-[11px]">
                <template v-for="[label, value] in stateRows" :key="label">
                  <dt class="text-[var(--poke-muted)]">{{ label }}</dt>
                  <dd class="min-w-0 overflow-hidden break-words text-[var(--poke-text)]">{{ value }}</dd>
                </template>
              </dl>
            </section>
            <section class="min-h-[260px] overflow-auto bg-[var(--poke-panel)] p-2">
              <h3 class="mb-1.5 text-[9px] tracking-[0.07em] text-[var(--poke-muted)] uppercase">Plan</h3>
              <div class="text-[9px] text-[var(--poke-muted)]">question</div>
              <pre class="mt-0.5 max-h-24 overflow-auto whitespace-pre-wrap font-mono text-[11px] leading-4 text-[var(--poke-text)]">{{ selectedRun.question || 'waiting for the first plan' }}</pre>
              <div class="mt-2 text-[9px] text-[var(--poke-muted)]">decision</div>
              <p :class="[selectedRun.decision ? 'text-[var(--poke-green)]' : 'text-[var(--poke-muted)]', 'mt-0.5 text-[11px] font-semibold']">{{ selectedRun.decision || (selectedRun.question ? 'waiting for reply' : 'waiting for the first plan') }}</p>
              <details v-if="selectedRun.raw" class="mt-2">
                <summary class="cursor-pointer text-[11px] text-[var(--poke-cyan)]">raw exchange</summary>
                <pre class="mt-1 max-h-40 overflow-auto whitespace-pre-wrap bg-[#121720] p-2 font-mono text-[10px] leading-4 text-[var(--poke-muted)]">{{ selectedRun.raw }}</pre>
              </details>
            </section>

            <section class="min-h-[260px] overflow-auto bg-[var(--poke-panel)] p-2">
              <h3 class="mb-1.5 text-[9px] tracking-[0.07em] text-[var(--poke-muted)] uppercase">Play</h3>
              <template v-if="playRows.length">
                <div v-for="[label, value] in playRows" :key="label" class="flex justify-between gap-2 text-[11px]">
                  <span class="text-[var(--poke-muted)]">{{ label }}</span>
                  <span :class="warnPlay(selectedRun.stats, label) ? 'text-[var(--poke-amber)]' : ''">{{ value }}</span>
                </div>
                <p v-if="selectedRun.stats?.intent" class="mt-2 text-[11px] text-[var(--poke-cyan)]">"{{ selectedRun.stats.intent }}" ({{ selectedRun.stats.intent_age }} rounds)</p>
                <div v-if="playChoices.length" class="mt-2 space-y-0.5">
                  <div v-for="choice in playChoices" :key="String(choice.objective)" class="relative flex justify-between overflow-hidden text-[11px]">
                    <span class="absolute inset-y-0 left-0 bg-[#233744]" :style="{ width: `${100 * Number(choice.count || 0) / playTop}%` }" />
                    <span class="relative">{{ choice.objective }}</span>
                    <span class="relative">{{ choice.count }}</span>
                  </div>
                </div>
              </template>
              <p v-else class="text-[11px] text-[var(--poke-muted)]">Play telemetry appears for LLM runs.</p>
            </section>
          </div>
          <div v-if="hasDecisionTelemetry(selectedRun.stats)" class="border-t border-[var(--poke-border)]">
            <DecisionTelemetry :stats="selectedRun.stats" />
          </div>
          <div v-if="selectedRun.trace" class="flex items-baseline gap-2.5 border-t border-[var(--poke-border)] bg-[var(--poke-panel)] px-2.5 py-1">
            <h3 class="shrink-0 text-[9px] tracking-[0.07em] text-[var(--poke-muted)] uppercase">Last event</h3>
            <pre class="min-w-0 flex-1 overflow-hidden font-mono text-[11px] leading-4 text-[var(--poke-text)] line-clamp-2">{{ selectedRun.trace }}</pre>
          </div>
        </div>

        <div v-if="actionError" class="border border-[#654047] bg-[#352529] px-2.5 py-2 text-[12px] text-[#e4b5b7]" role="alert">{{ actionError }}</div>

        <InspectorPanel :run-id="selectedRun.run_id" />

        <ConfirmDialog
          :open="forceEndTarget !== null"
          :title="forceEndTitle"
          :message="forceEndMessage"
          confirm-label="Force quit worker"
          :busy="forceEndBusy"
          danger
          @close="closeForceEnd"
          @confirm="confirmForceEnd"
        />
      </div>
    </div>

    <div v-else class="border border-dashed border-[var(--poke-border)] px-6 py-12 text-center">
      <p class="text-sm font-medium text-white">No run selected</p>
      <p class="mt-1 text-[12px] text-[var(--poke-muted)]">Queue a run from Tools or choose one from the Runs archive.</p>
      <a href="#tools" class="mt-3 inline-flex rounded-sm bg-[var(--poke-cyan)] px-2.5 py-1.5 text-[11px] font-bold text-[#101820] hover:brightness-110">Start a run</a>
    </div>
  </ResourceState>
</template>