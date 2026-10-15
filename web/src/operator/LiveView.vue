<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { Dialog, DialogPanel, DialogTitle, Menu, MenuButton, MenuItem, MenuItems } from '@headlessui/vue'
import { cancelRun, cloneRun, forceEndWorker, getDashboard, getProgramming, getRun, pauseProgramming, pauseRun, programmingEntryAction, replayVideoURL, resumeProgramming, resumeRun } from '../shared/api/client'
import type { DashboardRun, DashboardWorker, PartyMon, ProgrammingEntry } from '../shared/api/types'
import { getSpectatorControl, patchSpectatorRunControl } from '../shared/api/spectator-control'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import ModernSceneRenderer from '../shared/components/ModernSceneRenderer.vue'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
import { useFramePump } from '../shared/composables/useFramePump'
import { useRenderStatePump } from '../shared/composables/useRenderStatePump'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { presentReplay, presentRunStatus } from '../shared/status'
import { operatorHref, replayURL, spectatorURL } from '../shared/urls'
import SemanticMap from './SemanticMap.vue'
import LiveRail from './LiveRail.vue'
import RunTimeline from './RunTimeline.vue'
import RunStory from './RunStory.vue'
import LiveInspector from './LiveInspector.vue'
import { useRunEvidence, eventFrame } from './useRunEvidence'
import type { TimelineFilter } from './runTimelineFilters'
import {
  formatFrame,
  gameTitle,
  goalLabel,
  isLiveStatus,
  placeName,
  tileLabel
} from './operations'
import { isPlayStyleRun, playSpeedLabel, playStyleLabel } from '../shared/playstyle'
import { bagMeter, dexMeter, milestonesLabel } from '../shared/playerProgress'
import { canRenderModernScene } from '../shared/semanticRenderer'
import { DEFAULT_RENDER_THEME_ID, renderThemeOptions, resolveRenderTheme } from '../shared/renderTheme'

type RendererMode = 'modern' | 'classic'

const props = defineProps<{
  publicBase?: string
}>()

// ── Selection ──────────────────────────────────────────────────────────────
const params = new URLSearchParams(window.location.search)
const selectedRunID = ref(params.get('run') || '')
const selectionPinned = ref(Boolean(selectedRunID.value))

function restoreRunSelection(): void {
  const id = new URLSearchParams(window.location.search).get('run') || ''
  selectedRunID.value = id
  selectionPinned.value = Boolean(id)
}

const rendererMode = ref<RendererMode>(window.localStorage.getItem('pokepilot.operator.renderer') === 'classic' ? 'classic' : 'modern')
const themeOptions = renderThemeOptions()
const storedThemeID = window.localStorage.getItem('pokepilot.operator.theme') || DEFAULT_RENDER_THEME_ID
const initialThemeSelection = resolveRenderTheme(storedThemeID)
const selectedThemeID = ref(initialThemeSelection.theme.id)
const themeNotice = ref(initialThemeSelection.diagnostics.join(' '))
const activeTheme = computed(() => resolveRenderTheme(selectedThemeID.value).theme)
const historicalRun = ref<DashboardRun | null>(null)
const historicalError = ref('')
const pausing = ref(false)
const resuming = ref(false)
const canceling = ref(false)
const cloning = ref(false)
const spectatorUpdating = ref(false)
const confirmCancel = ref(false)
const forceEndTarget = ref<DashboardWorker | null>(null)
const forceEndBusy = ref(false)
const actionError = ref('')
const railOpen = ref(false)
let detailSerial = 0

const activeResource = usePollingResource((signal) => getDashboard({ active: true }, signal), { intervalMs: 2000 })
const recentResource = usePollingResource((signal) => getDashboard({ status: 'done', limit: 12 }, signal), { intervalMs: 10000 })
const spectatorResource = usePollingResource((signal) => getSpectatorControl(signal), { intervalMs: 5000 })
const programmingResource = usePollingResource((signal) => getProgramming(signal), { intervalMs: 2000 })
const programmingBusy = ref('')
const programmingError = ref('')

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

// Adopt a default only once the active list has answered, so a faster
// recent-runs response can't lock the page onto an ended run.
watch([selectedRun, () => activeResource.state.value], ([run, state]) => {
  if (run && !selectionPinned.value && !selectedRunID.value && state !== 'loading') selectedRunID.value = run.run_id
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

// ── Evidence, timeline and replay ─────────────────────────────────────────
const evidenceRunID = computed(() => selectedRun.value?.run_id || '')
const evidence = useRunEvidence(evidenceRunID)
const timelineFilter = ref<TimelineFilter>('all')
const timeline = ref<InstanceType<typeof RunTimeline> | null>(null)
const replayMode = ref(false)
const replayVideo = ref<HTMLVideoElement | null>(null)
const playbackRate = ref(Number(window.localStorage.getItem('pokepilot.replayPlaybackRate') || 1))
watch(evidenceRunID, () => {
  replayMode.value = false
  timelineFilter.value = 'all'
})

function seekReplayToSelection(): void {
  const node = replayVideo.value
  const event = evidence.timeline.value[evidence.selectedEvent.value]
  if (!node || !event || !(node.duration > 0)) return
  node.currentTime = node.duration * eventFrame(event) / evidence.totalFrames.value
}

function selectEvent(index: number): void {
  evidence.select(index)
  if (replayMode.value) seekReplayToSelection()
}

function syncEventFromVideo(): void {
  const node = replayVideo.value
  const rows = evidence.timeline.value
  if (!node || !(node.duration > 0) || !rows.length || node.paused) return
  const frame = evidence.totalFrames.value * node.currentTime / node.duration
  let nearest = 0
  for (let index = 1; index < rows.length; index++) {
    if (Math.abs(eventFrame(rows[index]) - frame) < Math.abs(eventFrame(rows[nearest]) - frame)) nearest = index
  }
  evidence.selectedEvent.value = nearest
  evidence.followLive.value = false
}

function applyPlaybackRate(): void {
  const allowed = [1, 2, 4, 8, 16]
  const value = allowed.includes(Number(playbackRate.value)) ? Number(playbackRate.value) : 1
  playbackRate.value = value
  window.localStorage.setItem('pokepilot.replayPlaybackRate', String(value))
  if (replayVideo.value) replayVideo.value.playbackRate = value
}
watch(playbackRate, () => { void nextTick(applyPlaybackRate) })

function watchReplay(): void {
  replayMode.value = true
  window.scrollTo({ top: 0 })
}

// ── Frames ────────────────────────────────────────────────────────────────
const selectedFrameID = computed(() => selectedRun.value?.run_id || '')
const isLiveFrame = computed(() => isLiveStatus(selectedRun.value?.status))
const frameEnabled = computed(() => Boolean(selectedFrameID.value) && selectedRun.value?.status !== 'queued' && !replayMode.value)
const renderEnabled = computed(() => frameEnabled.value && isLiveFrame.value)
const {
  renderState,
  state: renderStateStatus,
  error: renderStateError
} = useRenderStatePump(selectedFrameID, renderEnabled, 100, isLiveFrame)
const semanticReady = computed(() => canRenderModernScene(renderState.value))
const showModern = computed(() =>
  isLiveFrame.value &&
  rendererMode.value === 'modern' &&
  semanticReady.value &&
  renderStateStatus.value === 'ready'
)
const classicFrameEnabled = computed(() => frameEnabled.value && !showModern.value)
const { frameURL, state: frameState, error: frameError } = useFramePump(selectedFrameID, classicFrameEnabled, 50, isLiveFrame)
const modernFallbackLabel = computed(() => {
  if (!isLiveFrame.value || rendererMode.value !== 'modern' || showModern.value) return ''
  if (renderStateStatus.value === 'error') return 'Themed renderer unavailable · showing emulator'
  if (renderState.value?.scene) return `Themed renderer doesn't support ${renderState.value.scene} · showing emulator`
  return renderStateStatus.value === 'loading' ? 'Loading themed renderer · showing emulator' : ''
})

// ── Game state ────────────────────────────────────────────────────────────
const partySlots = computed<(PartyMon | null)[]>(() => {
  const members = selectedRun.value?.player?.party ?? []
  return Array.from({ length: 6 }, (_, index) => members[index] || null)
})
const badges = computed(() => selectedRun.value?.player?.badges ?? [])
const isTetrisRun = computed(() => selectedRun.value?.game === 'tetris')
const isBoxxleRun = computed(() => selectedRun.value?.game === 'boxxle')
const tetrisState = computed<Record<string, unknown>>(() => selectedRun.value?.game_state ?? {})
const tetrisDecision = computed<Record<string, unknown>>(() => selectedRun.value?.game_decision ?? {})
const tetrisBoard = computed<string[]>(() => {
  const board = tetrisState.value.board
  return Array.isArray(board) ? board.map((row) => String(row)) : []
})
const boxxleState = tetrisState
const boxxleDecision = tetrisDecision
const boxxleBoard = computed<string[]>(() => {
  const board = boxxleState.value.board
  return Array.isArray(board) ? board.map((row) => String(row)) : []
})
const boxxleBoardWidth = computed(() => {
  const width = Number(boxxleState.value.width || 0)
  return width > 0 ? width : (boxxleBoard.value[0]?.length || 1)
})
function record(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : null
}
const tetrisActive = computed(() => record(tetrisState.value.active))
const tetrisNext = computed(() => record(tetrisState.value.next))
const tetrisMetrics = computed(() => record(tetrisDecision.value.metrics))
const bagLabel = computed(() => bagMeter(selectedRun.value?.player))
const dexLabel = computed(() => dexMeter(selectedRun.value?.player))
const milestoneList = computed(() => milestonesLabel(selectedRun.value?.player))
const goalProgress = computed(() => {
  const stats = selectedRun.value?.stats
  if (stats?.goal_complete) return 100
  const current = Number(stats?.goal_current || 0)
  const target = Number(stats?.goal_target || 0)
  return target > 0 ? Math.max(0, Math.min(100, 100 * current / target)) : 0
})
const place = computed(() => selectedRun.value ? (placeName(selectedRun.value) || tileLabel(selectedRun.value)) : '')

function hpPercent(mon: PartyMon): number {
  if (!mon.max_hp) return 0
  return Math.max(0, Math.min(100, 100 * mon.hp / mon.max_hp))
}

function hpTone(mon: PartyMon): string {
  const percent = hpPercent(mon)
  if (!mon.max_hp || mon.hp === 0 || percent < 20) return 'bg-danger'
  if (percent < 50) return 'bg-warn'
  return 'bg-ok'
}

// ── Run actions ───────────────────────────────────────────────────────────
const status = computed(() => selectedRun.value ? presentRunStatus(selectedRun.value) : null)
const isRunning = computed(() => selectedRun.value?.status === 'running')
const isPausable = computed(() => ['queued', 'leased', 'running'].includes(String(selectedRun.value?.status || '')))
const isPaused = computed(() => selectedRun.value?.status === 'paused')
const isCancelable = computed(() => ['queued', 'leased', 'running', 'paused'].includes(String(selectedRun.value?.status || '')))
const isEnded = computed(() => selectedRun.value?.status === 'done')
const replayState = computed(() => evidence.replay.value?.state || '')
const replayProgress = computed(() => {
  const replay = evidence.replay.value
  const total = Number(replay?.segments || 0)
  return total ? 100 * Number(replay?.segments_done || 0) / total : undefined
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
const publicPageHref = computed(() => props.publicBase && selectedRun.value ? spectatorURL(props.publicBase, selectedRun.value.run_id) : '')
const publicReplayHref = computed(() => {
  const run = selectedRun.value
  if (!run?.replay_available || !props.publicBase) return ''
  return replayURL(props.publicBase, run.run_id)
})
const forceEndTitle = computed(() => forceEndTarget.value ? `Force quit worker ${forceEndTarget.value.addr}?` : 'Force quit worker?')
const forceEndMessage = computed(() => {
  const worker = forceEndTarget.value
  if (!worker) return ''
  const runID = worker.run_id || selectedRun.value?.run_id || ''
  return `This immediately terminates worker ${worker.addr} and force-ends run ${runID}. The run becomes terminal: it will not be retried and an endless successor will not be created.\n\nUse this only when pause/cancel cannot recover the run.`
})
const cancelMessage = computed(() => {
  const goal = selectedRun.value ? goalLabel(selectedRun.value) : ''
  return `The run stops and won't be retried.${goal ? ` The current goal, “${goal}”, ends unfinished.` : ''}`
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
  if (!run) return ''
  if (run.decision) return run.decision
  if (run.question) return 'Waiting for the planner reply'
  return 'Waiting for the first plan'
})
const elapsed = computed(() => {
  const run = selectedRun.value
  if (!run?.queued_at) return ''
  const end = run.ended_at || Date.now() / 1000
  const minutes = Math.max(0, Math.round((end - run.queued_at) / 60))
  const text = minutes >= 60 ? `${Math.floor(minutes / 60)}h ${String(minutes % 60).padStart(2, '0')}m` : `${minutes}m`
  return run.ended_at ? `Ran ${text}` : `${text} elapsed`
})

// Receipts: one polite live region, mirrored visually as a toast.
const toast = ref<{ text: string; runID?: string } | null>(null)
let toastTimer = 0
function announce(text: string, runID?: string): void {
  toast.value = { text, runID }
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => { toast.value = null }, 4000)
}

function selectRunID(runID: string): void {
  selectedRunID.value = runID
  selectionPinned.value = true
  railOpen.value = false
  const url = new URL(window.location.href)
  url.searchParams.set('run', runID)
  url.searchParams.delete('tab')
  history.replaceState(null, '', url)
}

function setRendererMode(mode: RendererMode): void {
  rendererMode.value = mode
  window.localStorage.setItem('pokepilot.operator.renderer', mode)
}

function onThemeSelect(event: Event): void {
  const target = event.target as HTMLSelectElement | null
  if (!target) return
  const resolved = resolveRenderTheme(target.value)
  selectedThemeID.value = resolved.theme.id
  themeNotice.value = resolved.diagnostics.join(' ')
  setRendererMode('modern')
  window.localStorage.setItem('pokepilot.operator.theme', resolved.theme.id)
}

function refresh(): void {
  void activeResource.retry()
  void recentResource.retry()
  void spectatorResource.retry()
  void programmingResource.retry()
}

async function runAction(busy: { value: boolean }, label: string, task: () => Promise<unknown>): Promise<boolean> {
  if (busy.value) return false
  busy.value = true
  actionError.value = ''
  try {
    await task()
    return true
  } catch (cause) {
    actionError.value = `${label} failed: ${cause instanceof Error ? cause.message : 'unknown error'}`
    return false
  } finally {
    busy.value = false
  }
}

async function pauseSelected(): Promise<void> {
  const run = selectedRun.value
  if (!run || !isPausable.value) return
  await runAction(pausing, 'Pause', async () => { await pauseRun(run.run_id); await activeResource.retry() })
}

async function resumeSelected(): Promise<void> {
  const run = selectedRun.value
  if (!run || !isPaused.value) return
  await runAction(resuming, 'Resume', async () => { await resumeRun(run.run_id); await activeResource.retry() })
}

async function confirmCancelSelected(): Promise<void> {
  const run = selectedRun.value
  if (!run || !isCancelable.value) return
  const ok = await runAction(canceling, 'Cancel', async () => { await cancelRun(run.run_id); await activeResource.retry() })
  confirmCancel.value = false
  if (ok) announce(`Run ${run.run_id} cancelled`)
}

async function runAgain(): Promise<void> {
  const run = selectedRun.value
  if (!run) return
  let newID = ''
  const ok = await runAction(cloning, 'Run again', async () => {
    newID = (await cloneRun(run.run_id)).run_id
    await Promise.all([activeResource.retry(), spectatorResource.retry()])
  })
  if (ok && newID) announce(`Queued ${newID} with the same settings`, newID)
}

async function toggleSpectatorSelected(): Promise<void> {
  const run = selectedRun.value
  if (!run || !spectatorControlReady.value) return
  const next = !spectatorVisible.value
  const ok = await runAction(spectatorUpdating, 'Public visibility update', async () => {
    await patchSpectatorRunControl(run.run_id, { visible: next })
    await spectatorResource.retry()
  })
  if (ok) announce(next ? 'Run is visible on the public site' : 'Run is hidden from the public site')
}

async function confirmForceEnd(): Promise<void> {
  const worker = forceEndTarget.value
  if (!worker) return
  const ok = await runAction(forceEndBusy, 'Force quit', async () => {
    await forceEndWorker(worker.addr)
    await Promise.all([activeResource.retry(), recentResource.retry()])
  })
  if (ok) forceEndTarget.value = null
}

async function copyRunID(): Promise<void> {
  const run = selectedRun.value
  if (!run) return
  try {
    await navigator.clipboard.writeText(run.run_id)
    announce('Run ID copied')
  } catch {
    announce('Couldn’t copy the run ID')
  }
}

async function toggleProgrammingPause(): Promise<void> {
  if (programmingBusy.value) return
  programmingBusy.value = 'pause'
  programmingError.value = ''
  try {
    if (programmingResource.data.value?.paused) await resumeProgramming()
    else await pauseProgramming()
    await programmingResource.retry()
  } catch (cause) {
    programmingError.value = cause instanceof Error ? cause.message : 'Programming update failed'
  } finally {
    programmingBusy.value = ''
  }
}

async function actOnProgramming(entry: ProgrammingEntry, action: 'skip' | 'retry'): Promise<void> {
  if (programmingBusy.value) return
  programmingBusy.value = entry.id + action
  programmingError.value = ''
  try {
    await programmingEntryAction(entry.id, action)
    await Promise.all([programmingResource.retry(), activeResource.retry()])
  } catch (cause) {
    programmingError.value = cause instanceof Error ? cause.message : 'Programming action failed'
  } finally {
    programmingBusy.value = ''
  }
}

// Keyboard: j/k runs, [ ] events, c copy, p pause/resume. Never destructive.
function onKeydown(event: KeyboardEvent): void {
  if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return
  const target = event.target as HTMLElement | null
  if (target?.closest('input, select, textarea, [contenteditable="true"], [role="dialog"], [role="menu"]')) return
  const runs = knownRuns.value
  const index = runs.findIndex((run) => run.run_id === selectedRun.value?.run_id)
  switch (event.key) {
    case 'j': if (runs[index + 1]) selectRunID(runs[index + 1].run_id); break
    case 'k': if (index > 0) selectRunID(runs[index - 1].run_id); break
    case '[': timeline.value?.step(-1); break
    case ']': timeline.value?.step(1); break
    case 'c': void copyRunID(); break
    case 'p':
      if (isRunning.value) void pauseSelected()
      else if (isPaused.value) void resumeSelected()
      break
    default: return
  }
  event.preventDefault()
}

onMounted(() => {
  window.addEventListener('popstate', restoreRunSelection)
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => {
  window.removeEventListener('popstate', restoreRunSelection)
  window.removeEventListener('keydown', onKeydown)
  window.clearTimeout(toastTimer)
})
const search = () => window.location.search
</script>

<template>
  <ResourceState
    :state="resourceState"
    title="Live run data unavailable"
    :message="historicalError || activeResource.error.value || recentResource.error.value || 'The console will recover automatically when the wall is reachable.'"
    :rows="8"
  >
    <template #actions>
      <button type="button" class="ui-btn ui-btn-sm" @click="refresh">Retry now</button>
    </template>

    <div v-if="selectedRun" class="grid grid-cols-1 gap-3 xl:grid-cols-[248px_minmax(0,1fr)]">
      <aside class="hidden overflow-hidden border border-rule-strong xl:sticky xl:top-[52px] xl:block xl:h-[calc(100vh-64px)]">
        <LiveRail
          :active="activeRuns"
          :recent="recentRuns"
          :selected-id="selectedRun.run_id"
          :programming="programmingResource.data.value"
          :programming-error="programmingError"
          @select="selectRunID"
          @toggle-programming="toggleProgrammingPause"
          @programming-action="actOnProgramming"
        />
      </aside>

      <div class="min-w-0 space-y-3">
        <!-- Run header -->
        <header class="flex min-h-12 flex-wrap items-center gap-x-3 gap-y-2 px-1">
          <button type="button" class="ui-btn ui-btn-sm xl:hidden" @click="railOpen = true">
            ☰ Runs <span class="text-fg-2">({{ activeRuns.length }} active)</span>
          </button>
          <span class="flex min-w-0 items-center gap-1">
            <h1 class="truncate font-mono text-[16px] leading-6 font-medium" :title="selectedRun.run_id">{{ selectedRun.run_id }}</h1>
            <button type="button" class="ui-btn ui-btn-sm ui-icon-btn" aria-label="Copy run ID (c)" title="Copy run ID (c)" @click="copyRunID">
              <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><rect x="9" y="9" width="11" height="11" rx="1.5" /><path d="M5 15V5a1 1 0 0 1 1-1h10" /></svg>
            </button>
          </span>
          <StatusPill v-if="status" :status="status" />
          <span class="ui-tag">{{ gameTitle(selectedRun.game) }}</span>
          <span class="ui-tag">{{ selectedRun.planner === 'llm' ? 'LLM' : 'Scripted' }}</span>
          <span v-if="isPlayStyleRun(selectedRun)" class="ui-tag">{{ playStyleLabel(selectedRun) }}</span>
          <span class="font-mono text-meta text-fg-2">{{ playSpeedLabel(selectedRun) }}</span>
          <a
            v-if="selectedRun.experiment_id"
            :href="operatorHref('analytics', { section: 'experiments', experiment: selectedRun.experiment_id }, search())"
            class="text-meta text-accent hover:underline"
          >Experiment {{ selectedRun.experiment_arm ? selectedRun.experiment_arm.toUpperCase() + ' · ' : '' }}{{ selectedRun.experiment_id }}</a>
          <span v-if="isPaused" class="text-meta text-warn">Safe to deploy and resume</span>

          <span class="ml-auto text-meta text-fg-2">{{ elapsed }}</span>
          <button v-if="isRunning" type="button" class="ui-btn ui-btn-primary" :disabled="pausing" title="Pause (p)" @click="pauseSelected">
            <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><rect x="3.5" y="3" width="3" height="10" /><rect x="9.5" y="3" width="3" height="10" /></svg>
            {{ pausing ? 'Pausing…' : 'Pause' }}
          </button>
          <button v-else-if="isPaused" type="button" class="ui-btn ui-btn-primary" :disabled="resuming" title="Resume (p)" @click="resumeSelected">
            ▶ {{ resuming ? 'Resuming…' : 'Resume' }}
          </button>
          <template v-else-if="isEnded">
            <button v-if="replayState === 'ready' && !replayMode" type="button" class="ui-btn ui-btn-primary" @click="watchReplay">▶ Watch replay</button>
            <button v-else-if="replayState === 'ready' && replayMode" type="button" class="ui-btn" @click="replayMode = false">Show final frame</button>
            <StatusPill v-else-if="replayState === 'generating'" :status="presentReplay('generating', replayProgress)" />
            <button
              v-else-if="replayState && replayState !== 'disabled'"
              type="button"
              class="ui-btn ui-btn-primary"
              :disabled="evidence.rendering.value"
              @click="evidence.requestReplay()"
            >{{ evidence.rendering.value ? 'Requesting…' : replayState === 'error' ? 'Retry replay' : 'Generate replay' }}</button>
          </template>

          <Menu as="div" class="relative">
            <MenuButton class="ui-btn ui-icon-btn border-control" aria-label="More run actions">
              <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><circle cx="3" cy="8" r="1.4" /><circle cx="8" cy="8" r="1.4" /><circle cx="13" cy="8" r="1.4" /></svg>
            </MenuButton>
            <MenuItems class="absolute right-0 z-30 mt-1 w-72 rounded-md border border-rule-strong bg-raised p-1 shadow-[0_8px_24px_rgb(0_0_0/45%)] focus:outline-none">
              <MenuItem v-slot="{ active }">
                <button type="button" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5 text-left" @click="copyRunID">
                  Copy run ID<span class="ml-auto font-mono text-meta text-fg-3">C</span>
                </button>
              </MenuItem>
              <MenuItem v-slot="{ active, disabled }" :disabled="cloning">
                <button type="button" :class="[active ? 'bg-selected' : '', disabled ? 'text-fg-3' : '']" class="flex h-8 w-full items-center rounded-[3px] px-2.5 text-left" @click="runAgain">
                  {{ cloning ? 'Queueing…' : 'Run again with same settings' }}
                </button>
              </MenuItem>
              <MenuItem v-slot="{ active, disabled }" :disabled="!spectatorControlReady || spectatorUpdating">
                <button
                  type="button"
                  role="menuitemcheckbox"
                  :aria-checked="spectatorVisible"
                  :class="[active ? 'bg-selected' : '', disabled ? 'text-fg-3' : '']"
                  class="flex min-h-8 w-full items-center rounded-[3px] px-2.5 py-1 text-left"
                  @click="toggleSpectatorSelected"
                >
                  <span>
                    Visible on public site
                    <span v-if="!spectatorControlReady" class="block text-meta text-fg-3">Public visibility controls unavailable</span>
                  </span>
                  <span class="ml-auto" :class="spectatorVisible ? 'text-ok' : 'text-fg-3'">{{ spectatorVisible ? '✓ On' : 'Off' }}</span>
                </button>
              </MenuItem>
              <MenuItem v-if="publicPageHref" v-slot="{ active }">
                <a :href="publicPageHref" target="_blank" rel="noopener" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5">Open public page ↗</a>
              </MenuItem>
              <MenuItem v-if="publicReplayHref" v-slot="{ active }">
                <a :href="publicReplayHref" target="_blank" rel="noopener" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5">Open public replay ↗</a>
              </MenuItem>
              <MenuItem v-slot="{ active }">
                <a :href="operatorHref('media', { section: 'library', run: selectedRun.run_id }, search())" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5">Open in Media</a>
              </MenuItem>
              <MenuItem v-if="isPausable && !isRunning" v-slot="{ active }">
                <button type="button" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5 text-left" @click="pauseSelected">Pause before it starts</button>
              </MenuItem>
              <template v-if="isCancelable || selectedWorker">
                <div class="my-1 h-px bg-rule" role="separator" />
                <MenuItem v-if="isCancelable" v-slot="{ active }">
                  <button type="button" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5 text-left text-danger" @click="confirmCancel = true">Cancel run…</button>
                </MenuItem>
                <MenuItem v-if="selectedWorker" v-slot="{ active }">
                  <button type="button" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5 text-left text-danger" @click="forceEndTarget = selectedWorker">Force quit worker…</button>
                </MenuItem>
              </template>
            </MenuItems>
          </Menu>
        </header>

        <p v-if="actionError" class="border border-danger/40 bg-danger/10 px-3 py-2 text-danger" role="alert">✕ {{ actionError }}</p>

        <!-- Game · Semantic map · Game state -->
        <!-- Row height scales with the viewport; the Game column follows it at the
             frame's 160:144 ratio (minus the 36px header) so the frame grows
             instead of letterboxing, and the map takes the remaining width. -->
        <div class="grid grid-cols-1 gap-px overflow-hidden border border-rule-strong bg-rule [--trio-h:clamp(340px,46vh,760px)] md:grid-cols-2 xl:h-[var(--trio-h)] xl:grid-cols-[calc((var(--trio-h)_-_36px)*160/144)_minmax(0,1fr)_300px] 2xl:grid-cols-[calc((var(--trio-h)_-_36px)*160/144)_minmax(0,1fr)_340px]">
          <section class="flex min-w-0 flex-col bg-surface xl:min-h-0" aria-label="Game">
            <header class="ui-bay-header">
              <h2>Game</h2>
              <span v-if="replayMode" class="ui-pill ui-tone-accent">▶ Replay</span>
              <StatusPill v-else-if="isLiveFrame" :status="{ label: 'Live', tone: 'ok', glyph: '●', live: true }" />
              <span v-else-if="isPaused" class="ui-pill ui-tone-warn">‖ Paused</span>
              <span v-else class="ui-pill ui-tone-neutral">■ Last frame</span>
            </header>
            <div class="group relative aspect-[160/144] min-h-52 w-full overflow-hidden bg-well xl:aspect-auto xl:min-h-0 xl:flex-1">
              <video
                v-if="replayMode"
                ref="replayVideo"
                :key="selectedRun.run_id"
                class="absolute inset-0 h-full w-full object-contain [image-rendering:pixelated]"
                :src="replayVideoURL(selectedRun.run_id)"
                controls
                autoplay
                preload="metadata"
                playsinline
                @loadedmetadata="applyPlaybackRate(); seekReplayToSelection()"
                @timeupdate="syncEventFromVideo"
              />
              <ModernSceneRenderer v-else-if="showModern && renderState" :state="renderState" :theme="activeTheme" />
              <img
                v-else-if="frameURL"
                :src="frameURL"
                :alt="`Game frame for ${selectedRun.run_id} at ${place}`"
                class="absolute inset-0 h-full w-full object-contain object-center [image-rendering:pixelated]"
              />
              <div v-else class="absolute inset-0 grid place-items-center px-4 text-center">
                <div>
                  <p class="text-fg-2">{{ selectedRun.status === 'queued' ? '○ Queued — starts when a worker picks it up' : isLiveFrame ? 'Waiting for live game state' : 'Last recorded frame unavailable' }}</p>
                  <p v-if="rendererMode === 'modern' && renderStateStatus === 'error' && renderStateError" class="mt-1 text-meta text-warn">▲ {{ renderStateError }}</p>
                  <p v-else-if="frameError" class="mt-1 text-meta text-warn">▲ {{ frameError }}</p>
                </div>
              </div>

              <div
                v-if="isLiveFrame && !replayMode"
                class="absolute bottom-2 left-2 flex flex-wrap items-end gap-1.5 opacity-100 transition-opacity xl:opacity-0 xl:group-focus-within:opacity-100 xl:group-hover:opacity-100"
              >
                <div class="flex overflow-hidden rounded-[3px] border border-control bg-well/90" role="group" aria-label="Renderer">
                  <button type="button" :aria-pressed="rendererMode === 'classic'" :class="rendererMode === 'classic' ? 'bg-selected text-fg' : 'text-fg-2 hover:text-fg'" class="h-7 px-2.5 text-meta font-medium" title="The authoritative emulator framebuffer" @click="setRendererMode('classic')">Emulator</button>
                  <button type="button" :aria-pressed="rendererMode === 'modern'" :class="rendererMode === 'modern' ? 'bg-selected text-fg' : 'text-fg-2 hover:text-fg'" class="h-7 px-2.5 text-meta font-medium" title="Shared semantic renderer with a theme" @click="setRendererMode('modern')">Themed</button>
                </div>
                <label v-if="rendererMode === 'modern'" class="flex h-7 items-center gap-1.5 rounded-[3px] border border-control bg-well/90 px-2">
                  <span class="text-meta text-fg-2">Theme</span>
                  <select :value="selectedThemeID" class="max-w-32 bg-transparent text-meta text-fg" @change="onThemeSelect">
                    <option v-for="theme in themeOptions" :key="theme.id" :value="theme.id" class="bg-raised">{{ theme.name }}</option>
                  </select>
                </label>
              </div>
              <label v-if="replayMode" class="absolute top-2 right-2 flex h-7 items-center gap-1.5 rounded-[3px] border border-control bg-well/90 px-2">
                <span class="text-meta text-fg-2">Speed</span>
                <select v-model.number="playbackRate" class="bg-transparent text-meta text-fg">
                  <option v-for="rate in [1, 2, 4, 8, 16]" :key="rate" :value="rate" class="bg-raised">{{ rate }}×</option>
                </select>
              </label>
              <p v-if="themeNotice && rendererMode === 'modern'" class="absolute top-2 right-2 max-w-56 rounded-[3px] bg-well/90 px-2 py-1 text-right text-meta text-warn">▲ {{ themeNotice }}</p>
              <p v-if="modernFallbackLabel && !replayMode" class="absolute top-2 left-2 rounded-[3px] bg-well/90 px-2 py-0.5 text-meta text-warn">{{ modernFallbackLabel }}</p>
              <p v-else-if="frameURL && frameState === 'error' && !replayMode" class="absolute right-2 bottom-2 rounded-[3px] bg-well/90 px-2 py-0.5 text-meta text-warn">▲ Last frame · reconnecting</p>
            </div>
          </section>

          <section class="flex min-w-0 flex-col bg-surface xl:min-h-0" :aria-label="isTetrisRun ? 'Tetris board' : isBoxxleRun ? 'Puzzle board' : 'Semantic map'">
            <header class="ui-bay-header">
              <h2>{{ isTetrisRun ? 'Tetris board' : isBoxxleRun ? 'Puzzle board' : 'Semantic map' }}</h2>
              <span class="truncate font-mono text-meta font-normal text-fg-2">{{ replayMode ? 'Final state' : tileLabel(selectedRun) }}</span>
            </header>
            <div v-if="isTetrisRun" class="flex h-64 min-h-52 items-center justify-center bg-well p-2 xl:h-auto xl:min-h-0 xl:flex-1">
              <div v-if="tetrisBoard.length" class="grid aspect-[10/18] h-full max-h-full grid-cols-10 grid-rows-[repeat(18,minmax(0,1fr))] gap-px border border-rule bg-black/60 p-px">
                <template v-for="(row, y) in tetrisBoard" :key="y">
                  <span v-for="(cell, x) in row.split('')" :key="`${y}-${x}`" :class="[cell === '#' ? 'bg-accent/80' : 'bg-white/[0.035]', 'min-h-0 min-w-0']" />
                </template>
              </div>
              <p v-else class="text-meta text-fg-3">Waiting for Tetris semantic state…</p>
            </div>
            <div v-else-if="isBoxxleRun" class="flex h-64 min-h-52 items-center justify-center bg-well p-2 xl:h-auto xl:min-h-0 xl:flex-1">
              <div v-if="boxxleBoard.length" class="grid h-full max-h-full gap-px border border-rule bg-black/60 p-px" :style="{ gridTemplateColumns: `repeat(${boxxleBoardWidth}, minmax(0, 1fr))` }">
                <template v-for="(row, y) in boxxleBoard" :key="y">
                  <span
                    v-for="(cell, x) in row.split('')"
                    :key="`${y}-${x}`"
                    :class="[
                      'flex min-h-0 min-w-0 items-center justify-center font-mono text-meta leading-none',
                      cell === '#' ? 'bg-rule-strong text-fg-2' : '',
                      cell === '$' ? 'bg-warn/80 text-canvas' : '',
                      cell === '*' ? 'bg-ok/80 text-canvas' : '',
                      cell === '+' ? 'bg-ok/25 text-ok' : '',
                      cell === '@' ? 'bg-accent/90 text-canvas' : '',
                      cell === '.' ? 'bg-white/[0.035] text-transparent' : ''
                    ]"
                  >{{ cell === '.' || cell === '#' ? '' : cell }}</span>
                </template>
              </div>
              <p v-else class="text-meta text-fg-3">Waiting for Boxxle semantic state…</p>
            </div>
            <div v-else class="relative h-64 min-h-52 xl:h-auto xl:min-h-0 xl:flex-1">
              <SemanticMap :map="selectedRun.map" :x="selectedRun.x" :y="selectedRun.y" :trail="selectedRun.trail" :sprites="selectedRun.sprites" />
            </div>
            <p v-if="replayMode" class="border-t border-rule px-3 py-1.5 text-meta text-fg-2">Final state — semantic history isn't available for replay moments.</p>
            <div v-else-if="isTetrisRun" class="flex flex-wrap gap-x-3 border-t border-rule px-3 py-1 text-meta text-fg-2">
              <span><b class="text-accent">■</b> locked cell</span>
              <span>active {{ tetrisActive?.piece || '—' }} r{{ tetrisActive?.rotation ?? '—' }}</span>
              <span>next {{ tetrisNext?.piece || '—' }}</span>
            </div>
            <div v-else-if="isBoxxleRun" class="flex flex-wrap gap-x-3 border-t border-rule px-3 py-1 text-meta text-fg-2">
              <span><b class="text-accent">@</b> player</span><span><b class="text-warn">$</b> crate</span><span><b class="text-ok">*</b> on goal</span><span><b class="text-ok">+</b> goal</span>
            </div>
            <div v-else class="flex flex-wrap gap-x-3 border-t border-rule px-3 py-1 text-meta text-fg-2">
              <span><b class="text-fg">@</b> player</span><span><b class="text-[var(--map-sprite)]">■</b> sprite</span><span><b class="text-[var(--map-trail)]">·</b> trail</span><span><b class="text-[var(--map-warp)]">W</b> warp</span><span><b class="text-fg">#</b> blocked</span>
            </div>
          </section>

          <section class="flex min-h-0 min-w-0 flex-col bg-surface md:col-span-2 xl:col-span-1" aria-label="Game state">
            <header class="ui-bay-header">
              <h2>Game state</h2>
              <span v-if="isTetrisRun" class="font-mono text-meta font-normal text-fg-2">score {{ Number(tetrisState.score || 0).toLocaleString() }}</span>
              <span v-else-if="isBoxxleRun" class="font-mono text-meta font-normal text-fg-2">{{ Number(boxxleState.levels || 0) }} solved</span>
              <span v-else class="font-mono text-meta font-normal text-fg-2">₽{{ Number(selectedRun.player?.money || 0).toLocaleString() }}</span>
            </header>
            <dl v-if="isTetrisRun" class="ui-kv min-h-0 flex-1 overflow-auto px-3">
              <dt>Lines</dt><dd class="font-mono">{{ Number(tetrisState.lines_cleared || 0) }}</dd>
              <dt>Level</dt><dd class="font-mono">{{ Number(tetrisState.level || 0) }}</dd>
              <dt>Active</dt><dd class="font-mono">{{ tetrisActive?.piece || '—' }} r{{ tetrisActive?.rotation ?? '—' }} · x{{ tetrisActive?.x ?? '—' }} y{{ tetrisActive?.y ?? '—' }}</dd>
              <dt>Next</dt><dd class="font-mono">{{ tetrisNext?.piece || '—' }} r{{ tetrisNext?.rotation ?? '—' }}</dd>
              <dt>Screen</dt><dd>{{ tetrisState.screen || '—' }}</dd>
              <dt>Mode</dt><dd>{{ tetrisState.mode || '—' }}</dd>
              <dt>Input</dt><dd>{{ tetrisState.ready_for_piece_input ? 'ready' : (tetrisState.clearing ? 'clearing' : (tetrisState.locking ? 'locking' : 'waiting')) }}</dd>
              <dt>Placement</dt><dd class="font-mono">r{{ tetrisDecision.rotation ?? '—' }} c{{ tetrisDecision.column ?? '—' }}</dd>
              <dt>Policy</dt><dd>{{ tetrisDecision.objective || '—' }}</dd>
              <dt>Evaluated</dt><dd class="font-mono">{{ tetrisDecision.considered ?? '—' }}</dd>
              <template v-if="tetrisMetrics"><dt>Board</dt><dd class="font-mono">holes {{ tetrisMetrics.holes ?? 0 }} · height {{ tetrisMetrics.max_height ?? 0 }} · bump {{ tetrisMetrics.bumpiness ?? 0 }}</dd></template>
            </dl>
            <dl v-else-if="isBoxxleRun" class="ui-kv min-h-0 flex-1 overflow-auto px-3">
              <dt>Screen</dt><dd>{{ boxxleState.screen || '—' }}</dd>
              <dt>Solved</dt><dd>{{ boxxleState.solved ? 'Yes' : 'No' }}</dd>
              <dt>Crates on goal</dt><dd class="font-mono">{{ boxxleState.crates_on_goal ?? 0 }}/{{ boxxleState.crates ?? 0 }}</dd>
              <dt>Pushes</dt><dd class="font-mono">{{ boxxleState.pushes ?? 0 }}</dd>
              <dt>Push</dt><dd class="font-mono">{{ boxxleDecision.dir || '—' }} ({{ (boxxleDecision.crate as { x?: number } | undefined)?.x ?? '—' }},{{ (boxxleDecision.crate as { y?: number } | undefined)?.y ?? '—' }})</dd>
              <dt>Fallback</dt><dd>{{ boxxleDecision.fallback || boxxleDecision.decision_fallback ? 'solver/policy' : 'none' }}</dd>
            </dl>
            <div v-else class="flex min-h-0 flex-1 flex-col overflow-auto">
              <div class="grid grid-cols-3 border-b border-rule">
                <div class="border-r border-rule px-3 py-1.5" :title="badges.join(', ')"><p class="ui-label">Badges</p><p class="font-mono font-medium">{{ badges.length }} / 8</p></div>
                <div class="border-r border-rule px-3 py-1.5"><p class="ui-label">Bag</p><p class="font-mono font-medium">{{ bagLabel || '—' }}</p></div>
                <div class="px-3 py-1.5"><p class="ui-label">Dex</p><p class="font-mono font-medium">{{ dexLabel || '—' }}</p></div>
              </div>
              <ul class="flex-1">
                <li v-for="(mon, index) in partySlots" :key="index" class="border-b border-rule px-3 py-1.5">
                  <template v-if="mon">
                    <div class="flex items-baseline gap-2">
                      <span class="min-w-0 flex-1 truncate font-semibold capitalize">{{ mon.name }}</span>
                      <span v-if="mon.status" class="text-meta text-danger">{{ mon.status }}</span>
                      <span class="text-meta text-fg-2">Lv {{ mon.level }}</span>
                      <span class="w-16 text-right font-mono text-meta text-fg">{{ mon.hp }}/{{ mon.max_hp }}</span>
                    </div>
                    <div class="ui-meter mt-1" role="meter" :aria-valuenow="mon.hp" aria-valuemin="0" :aria-valuemax="mon.max_hp" :aria-label="`${mon.name} HP`"><span :class="hpTone(mon)" :style="{ width: `${hpPercent(mon)}%` }" /></div>
                  </template>
                  <p v-else class="text-meta text-fg-3">Slot {{ index + 1 }} empty</p>
                </li>
              </ul>
              <p v-if="milestoneList" class="truncate px-3 py-1.5 text-meta text-fg-2" :title="milestoneList">Milestones: <span class="text-fg">{{ milestoneList }}</span></p>
            </div>
          </section>
        </div>

        <!-- Now band -->
        <section class="ui-bay grid grid-cols-1 md:grid-cols-2 xl:grid-cols-[minmax(0,1.3fr)_minmax(0,2fr)_minmax(0,1fr)_auto]" aria-label="Now">
          <div class="flex flex-col gap-1 border-b border-rule px-4 py-2.5 xl:border-r xl:border-b-0">
            <span class="ui-label">Objective</span>
            <span class="truncate font-semibold" :title="goalLabel(selectedRun)">{{ goalLabel(selectedRun) }}</span>
            <span v-if="selectedRun.stats?.goal_summary" class="truncate text-meta text-fg-2" :title="selectedRun.stats.goal_summary">{{ selectedRun.stats.goal_summary }}</span>
            <span v-if="selectedRun.stats?.goal_target" class="flex items-center gap-2">
              <span class="ui-meter"><span class="bg-ok" :style="{ width: `${goalProgress}%` }" /></span>
              <span class="font-mono text-meta">{{ selectedRun.stats.goal_current || 0 }} / {{ selectedRun.stats.goal_target }}</span>
            </span>
          </div>
          <div class="flex min-w-0 flex-col gap-1 border-b border-rule px-4 py-2.5 xl:border-r xl:border-b-0">
            <span class="ui-label">Latest decision</span>
            <span :class="selectedRun.decision ? '' : 'text-fg-3'" class="line-clamp-2 max-w-[72ch]" :title="latestDecision">{{ latestDecision }}</span>
          </div>
          <div class="flex flex-col gap-1 border-b border-rule px-4 py-2.5 md:border-b-0 xl:border-r">
            <span class="ui-label">Position</span>
            <span class="truncate font-semibold">{{ place }}</span>
            <span class="font-mono text-meta text-fg-2">({{ Number(selectedRun.x || 0) }}, {{ Number(selectedRun.y || 0) }})</span>
          </div>
          <div class="flex items-center gap-7 px-4 py-2.5">
            <div><p class="ui-label">Frame</p><p class="font-mono text-readout font-medium">{{ formatFrame(selectedRun.frame) }}</p></div>
            <div><p class="ui-label">Round</p><p class="font-mono text-readout font-medium">{{ selectedRun.stats?.round ?? selectedRun.stats?.rounds ?? '—' }}</p></div>
          </div>
        </section>

        <!-- Timeline transport -->
        <RunTimeline
          ref="timeline"
          v-model:filter="timelineFilter"
          :events="evidence.timeline.value"
          :total-frames="evidence.totalFrames.value"
          :selected-index="evidence.selectedEvent.value"
          :follow-live="evidence.followLive.value"
          :live="evidence.isLive.value"
          @select="selectEvent"
          @follow="evidence.setFollowLive"
        />

        <!-- Story + inspector -->
        <div class="grid grid-cols-1 gap-3 xl:h-[clamp(440px,60vh,1000px)] xl:grid-cols-[minmax(0,1fr)_minmax(400px,0.75fr)]">
          <RunStory
            class="max-h-[560px] xl:max-h-none"
            :events="evidence.timeline.value"
            :selected-index="evidence.selectedEvent.value"
            :filter="timelineFilter"
            :follow-live="evidence.followLive.value"
            :live="evidence.isLive.value"
            @select="selectEvent"
          />
          <LiveInspector
            class="max-h-[720px] xl:max-h-none"
            :run="selectedRun"
            :worker="selectedWorker"
            :evidence="evidence"
            @watch-replay="watchReplay"
          />
        </div>

        <ConfirmDialog
          :open="confirmCancel"
          :title="`Cancel run ${selectedRun.run_id}?`"
          :message="cancelMessage"
          confirm-label="Cancel run"
          cancel-label="Keep running"
          :busy="canceling"
          danger
          @close="confirmCancel = false"
          @confirm="confirmCancelSelected"
        />
        <ConfirmDialog
          :open="forceEndTarget !== null"
          :title="forceEndTitle"
          :message="forceEndMessage"
          confirm-label="Force quit worker"
          :busy="forceEndBusy"
          danger
          @close="forceEndBusy ? undefined : (forceEndTarget = null)"
          @confirm="confirmForceEnd"
        />
      </div>
    </div>

    <div v-else class="mx-auto max-w-md py-16 text-center">
      <p class="text-title font-semibold">No runs yet</p>
      <p class="mt-1 text-fg-2">Queue a run to watch it here.</p>
      <a :href="operatorHref('new-run', {}, search())" class="ui-btn ui-btn-primary mt-4">New run</a>
    </div>

    <!-- Runs drawer below xl -->
    <Dialog :open="railOpen" class="operator-root relative z-[60] xl:hidden" @close="railOpen = false">
      <div class="fixed inset-0 bg-black/60" aria-hidden="true" />
      <DialogPanel class="fixed inset-y-0 left-0 flex w-[min(320px,88vw)] flex-col border-r border-rule-strong bg-surface">
        <div class="flex h-11 items-center justify-between border-b border-rule px-3">
          <DialogTitle class="text-title font-semibold">Runs</DialogTitle>
          <button type="button" class="ui-btn ui-btn-sm ui-icon-btn" aria-label="Close runs" @click="railOpen = false">✕</button>
        </div>
        <LiveRail
          class="min-h-0 flex-1"
          :active="activeRuns"
          :recent="recentRuns"
          :selected-id="selectedRun?.run_id || ''"
          :programming="programmingResource.data.value"
          :programming-error="programmingError"
          @select="selectRunID"
          @toggle-programming="toggleProgrammingPause"
          @programming-action="actOnProgramming"
        />
      </DialogPanel>
    </Dialog>

    <div class="pointer-events-none fixed right-4 bottom-4 z-50" aria-live="polite" role="status">
      <div v-if="toast" class="pointer-events-auto flex items-center gap-3 rounded-md border border-rule-strong bg-raised px-4 py-2.5 shadow-[0_8px_24px_rgb(0_0_0/45%)]">
        <span>{{ toast.text }}</span>
        <button v-if="toast.runID" type="button" class="ui-btn ui-btn-sm" @click="selectRunID(toast.runID!); toast = null">Open</button>
      </div>
    </div>
  </ResourceState>
</template>
