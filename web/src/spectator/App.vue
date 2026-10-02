<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  ArrowPathIcon,
  ArrowsPointingOutIcon,
  LinkIcon,
  PlayIcon,
  SignalIcon
} from '@heroicons/vue/20/solid'
import { getSpectatorProgramming, getSpectatorSnapshot } from '../shared/api/spectator-client'
import type { SpectatorDecisionRecord, SpectatorRun } from '../shared/api/spectator'
import AppShell from '../shared/components/AppShell.vue'
import PokemonSprite from '../shared/components/PokemonSprite.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import ModernSceneRenderer from '../shared/components/ModernSceneRenderer.vue'
import { useFramePump } from '../shared/composables/useFramePump'
import { useRenderStatePump } from '../shared/composables/useRenderStatePump'
import { usePollingResource } from '../shared/composables/usePollingResource'
import {
  gameTitle,
  goalProgress,
  isBoxxleRun,
  isLiveRun,
  isTetrisRun,
  locationLabel,
  normalizePlayStyle,
  objectiveLabel,
  playSpeedLabel,
  playStyleLabel,
  preferredRun,
  routeLabel,
  runStatusLabel,
  runTitle,
  runTone,
  splitSpectatorRuns
} from './model'
import BroadcastLoadingScene from './BroadcastLoadingScene.vue'
import PublicHome from './PublicHome.vue'
import RouteLine from './RouteLine.vue'
import WorldExplorer from './WorldExplorer.vue'
import { gameRouteLine } from './routeLine'
import { mapCatalogForGame, mapEntryForGame, spectatorNativeMap } from '../shared/mapCatalog'
import { replayPath, runIDFromLocation, spectatorRunPath } from '../shared/urls'
import { elapsedRunSeconds, formatDuration } from '../shared/runTiming'
import { canRenderModernScene } from '../shared/semanticRenderer'
import { publicCapabilitiesForRun } from '../shared/publicCapabilities'
import { PUBLIC_RENDER_THEME_ID, publicRenderThemeOptions, resolvePublicRenderTheme } from '../shared/renderTheme'
import { replayIsRendering, replayRenderProgress, replayRenderStage } from '../replays/model'
import spectatorNightscapeUrl from './assets/spectator-nightscape.svg'

type ActivityKind = 'decision' | 'area' | 'badge' | 'party' | 'dex' | 'milestone' | 'game' | 'state'
type ActivityFilter = 'all' | 'milestones' | 'decisions'
type RendererMode = 'modern' | 'classic'

interface ActivityItem {
  id: string
  at: number
  kind: ActivityKind
  label: string
  detail: string
}

const selectedRunID = ref(runIDFromLocation(window.location.pathname, window.location.search))
const selectionPinned = ref(Boolean(selectedRunID.value))
const copyState = ref('')
const theaterMode = ref(false)
const rendererMode = ref<RendererMode>(window.localStorage.getItem('pokepilot.spectator.renderer') === 'modern' ? 'modern' : 'classic')
const themeOptions = publicRenderThemeOptions()
const storedThemeID = window.localStorage.getItem('pokepilot.spectator.theme') || PUBLIC_RENDER_THEME_ID
const initialThemeSelection = resolvePublicRenderTheme(storedThemeID)
const selectedThemeID = ref(initialThemeSelection.theme.id)
const themeNotice = ref(initialThemeSelection.diagnostics.join(' '))
const activeTheme = computed(() => resolvePublicRenderTheme(selectedThemeID.value).theme)
const playerRef = ref<HTMLElement | null>(null)
const activityFilter = ref<ActivityFilter>('all')
const activityFilters: ActivityFilter[] = ['all', 'milestones', 'decisions']
const activityByRun = ref<Record<string, ActivityItem[]>>({})
const previousRuns = new Map<string, SpectatorRun>()

const {
  data: snapshot,
  error,
  state,
  lastUpdatedAt,
  retry
} = usePollingResource(
  (signal) => getSpectatorSnapshot(signal),
  {
    intervalMs: 2000,
    isEmpty: (value) => value.runs.length === 0
  }
)

const { data: programming } = usePollingResource(
  (signal) => getSpectatorProgramming(signal),
  { intervalMs: 3000 }
)

const runs = computed(() => snapshot.value?.runs ?? [])
const groupedRuns = computed(() => splitSpectatorRuns(runs.value))
const selectedRun = computed(() => {
  if (selectionPinned.value && selectedRunID.value) {
    return runs.value.find((run) => run.run_id === selectedRunID.value) || null
  }
  return preferredRun(groupedRuns.value.live, '')
})
const selectedReplayHref = computed(() => {
  const run = selectedRun.value
  return run?.replay_ready ? replayPath(run.run_id) : ''
})
const selectedReplayRendering = computed(() => Boolean(selectedRun.value && replayIsRendering(selectedRun.value)))
const selectedPublicCapabilities = computed(() => publicCapabilitiesForRun(selectedRun.value))
const isTetrisSelected = computed(() => isTetrisRun(selectedRun.value))
const isBoxxleSelected = computed(() => isBoxxleRun(selectedRun.value))
const isPuzzleSelected = computed(() => isTetrisSelected.value || isBoxxleSelected.value)
const tetrisState = computed(() => selectedRun.value?.game_state)
const boxxleState = computed(() => selectedRun.value?.game_state)
const boxxleBoardRows = computed(() => (boxxleState.value?.board || []).map((row) => String(row).split('')))
const tetrisBoardRows = computed(() =>
  (tetrisState.value?.board || []).slice(0, 18).map((row) => row.slice(0, 10).split(''))
)
const tetrisModeLabel = computed(() => formatGameToken(tetrisState.value?.mode || 'type-a'))
const tetrisActivePiece = computed(() => tetrisState.value?.active?.piece || '—')
const tetrisNextPiece = computed(() => tetrisState.value?.next?.piece || '—')
const tetrisDecisionRecords = computed(() => [...(selectedRun.value?.stats?.decision_records || [])].reverse())
const tetrisLatestDecision = computed(() => tetrisDecisionRecords.value[0])
const tetrisDecisionIdentity = computed(() => {
  const run = selectedRun.value
  if (!run) return '—'
  const configured = run.decision_engine
  const served = run.stats?.decision_model || tetrisLatestDecision.value?.model
  const label = configured?.label || configured?.deployment
  const parts = [label, served || configured?.model].filter(Boolean)
  return [...new Set(parts)].join(' · ') || 'Jev'
})
const tetrisDecisionBackend = computed(() =>
  selectedRun.value?.stats?.decision_backend
  || tetrisLatestDecision.value?.backend
  || selectedRun.value?.decision_engine?.backend
  || ''
)
const tetrisDecisionMode = computed(() =>
  selectedRun.value?.stats?.decision_mode || selectedRun.value?.decision_engine?.mode || 'active'
)
const tetrisDecisionCalls = computed(() => Number(selectedRun.value?.stats?.decision_calls || 0))
const tetrisDecisionConfidence = computed(() =>
  Number(tetrisLatestDecision.value?.confidence ?? selectedRun.value?.stats?.decision_confidence ?? 0)
)
const tetrisDecisionReference = computed(() => {
  const stats = selectedRun.value?.stats
  const agreed = Number(stats?.decision_reference_agreements || 0)
  const disagreed = Number(stats?.decision_reference_disagreements || 0)
  const judged = agreed + disagreed
  return {
    agreed,
    disagreed,
    judged,
    rate: judged > 0 ? agreed / judged : null
  }
})
const tetrisDecisionAgreement = computed(() => tetrisDecisionReference.value.rate)
const tetrisDecisionVisible = computed(() =>
  isTetrisSelected.value
  && Boolean(selectedRun.value?.decision_engine?.backend || tetrisDecisionCalls.value > 0)
)
const tetrisDecisionFallbackRate = computed(() => {
  const calls = tetrisDecisionCalls.value
  return calls > 0 ? Number(selectedRun.value?.stats?.decision_fallbacks || 0) / calls : null
})
const tetrisLatestChoice = computed(() =>
  tetrisLatestDecision.value?.choice_label
  || tetrisLatestDecision.value?.choice
  || selectedRun.value?.stats?.decision_choice
  || 'Waiting for first Jev choice'
)
const otherLiveRuns = computed(() => {
  const selectedID = selectedRun.value?.run_id || ''
  return groupedRuns.value.live.filter((run) => run.run_id !== selectedID).slice(0, 3)
})
const frameRunID = computed(() => {
  const run = selectedRun.value
  return run && isLiveRun(run) ? run.run_id : ''
})
const frameContinuous = computed(() => isLiveRun(selectedRun.value))
const renderEnabled = computed(() => Boolean(frameRunID.value))
const {
  renderState,
  state: renderStateStatus,
  error: renderStateError
} = useRenderStatePump(frameRunID, renderEnabled, 100, frameContinuous)
const semanticReady = computed(() => canRenderModernScene(renderState.value))
const showModern = computed(() =>
  selectionPinned.value && !isPuzzleSelected.value && rendererMode.value === 'modern' && semanticReady.value
)
const frameEnabled = computed(() =>
  Boolean(frameRunID.value) && (isPuzzleSelected.value || !selectionPinned.value || rendererMode.value === 'classic' || !semanticReady.value)
)
const { frameURL, state: frameState, error: frameError } = useFramePump(frameRunID, frameEnabled, 50, frameContinuous)
const modernFallbackLabel = computed(() => {
  if (isPuzzleSelected.value || rendererMode.value !== 'modern' || showModern.value) return ''
  if (renderStateStatus.value === 'error') return `${activeTheme.value.name} · semantic state reconnecting`
  if (renderState.value?.scene) return `${activeTheme.value.name} · classic compatibility · ${renderState.value.scene}`
  return ''
})
const modeClass = computed(() => `mode-${normalizePlayStyle(selectedRun.value)}`)
const gameClass = computed(() => isTetrisSelected.value ? 'game-tetris' : isBoxxleSelected.value ? 'game-boxxle' : 'game-pokemon')
const selectedActivity = computed(() => {
  const run = selectedRun.value
  const activity = run ? activityByRun.value[run.run_id] || [] : []
  if (activityFilter.value === 'milestones') {
    return activity.filter((item) => ['area', 'badge', 'party', 'dex', 'milestone', 'game'].includes(item.kind))
  }
  if (activityFilter.value === 'decisions') {
    return activity.filter((item) => item.kind === 'decision')
  }
  return activity
})
const plannerState = computed(() => {
  const run = selectedRun.value
  if (!run || !isLiveRun(run) || run.decision) return null
  if (!run.planner_waiting) {
    return {
      title: 'Agent starting',
      detail: 'Building the first objective menu'
    }
  }
  const options = Number(run.planner_options || 0)
  return {
    title: 'Planner thinking',
    detail: options > 0 ? `Evaluating ${options} available objectives` : 'Waiting for the model response'
  }
})
const mapsLabel = computed(() => {
  const run = selectedRun.value
  const visited = Number(run?.maps_visited || 0)
  const total = mapCatalogForGame(run?.game).length
  return total > 0 ? `${visited}/${total}` : String(visited)
})
const lastRefreshLabel = computed(() => {
  if (!lastUpdatedAt.value) return ''
  return new Date(lastUpdatedAt.value).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
})

const currentLocation = computed(() => {
  const run = selectedRun.value
  if (!run) return 'Waiting for state'
  return displayLocation(run)
})
const runtimeLabel = computed(() => {
  const run = selectedRun.value
  if (!run) return '—'
  const seconds = elapsedRunSeconds(run)
  return seconds > 0 ? formatDuration(seconds) : 'Just started'
})
const sceneStyle = computed(() => ({
  '--spectator-art': isPuzzleSelected.value ? 'none' : `url("${spectatorNightscapeUrl}")`
}))

const goalPercent = computed(() => {
  const run = selectedRun.value
  if (!run) return 0
  if (isBoxxleRun(run)) {
    const crates = Number(run.game_state?.crates || 0)
    if (crates <= 0) return run.game_state?.solved ? 100 : 0
    return Math.max(0, Math.min(100, 100 * Number(run.game_state?.crates_on_goal || 0) / crates))
  }
  if (!isTetrisRun(run)) return goalProgress(run)
  if (run.game_state?.complete) return 100
  const match = (run.goal || '').trim().toLowerCase().match(/^(score|lines):(\d+)$/)
  if (!match) return 0
  const target = Number(match[2] || 0)
  if (target <= 0) return 0
  const current = match[1] === 'score'
    ? Number(run.game_state?.score || 0)
    : Number(run.game_state?.lines_cleared || 0)
  return Math.max(0, Math.min(100, 100 * current / target))
})

const leagueGoal = computed(() => {
  const run = selectedRun.value
  if (!run || isTetrisRun(run) || isBoxxleRun(run)) return false
  const text = [run.goal, run.stats?.goal_summary, objectiveLabel(run)].filter(Boolean).join(' ').toLowerCase()
  return normalizePlayStyle(run) === 'speedrun' || /elite four|champion|hall of fame/.test(text)
})

const goalProgressCopy = computed(() => {
  const run = selectedRun.value
  if (!run) return { label: 'Waiting for goal', detail: 'Run objective is loading.' }

  const stats = run.stats
  if (isBoxxleRun(run)) {
    const state = run.game_state
    const levels = Number(state?.levels || 0)
    const onGoal = Number(state?.crates_on_goal || 0)
    const crates = Number(state?.crates || 0)
    return {
      label: `${levels} puzzle${levels === 1 ? '' : 's'} solved`,
      detail: crates > 0 ? `${onGoal}/${crates} crates on goal` : 'Autonomous puzzle play'
    }
  }
  if (isTetrisRun(run)) {
    const state = run.game_state
    const score = Number(state?.score || 0)
    const lines = Number(state?.lines_cleared || 0)
    const match = (run.goal || '').trim().toLowerCase().match(/^(score|lines):(\d+)$/)
    if (state?.complete || stats?.goal_complete) {
      return {
        label: 'Goal complete',
        detail: `${score.toLocaleString()} points · ${lines} lines cleared`
      }
    }
    if (match) {
      const target = Number(match[2] || 0)
      const current = match[1] === 'score' ? score : lines
      const unit = match[1] === 'score' ? 'points' : 'lines'
      return {
        label: `${current.toLocaleString()}/${target.toLocaleString()} ${unit}`,
        detail: `${Math.max(0, target - current).toLocaleString()} ${unit} remaining`
      }
    }
    return {
      label: 'Stack in progress',
      detail: `${score.toLocaleString()} points · ${lines} lines cleared`
    }
  }

  const badges = run.player?.badges?.length || 0
  if (stats?.goal_complete) {
    return {
      label: 'Goal complete',
      detail: stats.goal_summary || 'The run objective is complete.'
    }
  }

  if (leagueGoal.value && badges >= 8) {
    return {
      label: '8/8 badges earned',
      detail: 'Next: Elite Four, Champion & Hall of Fame'
    }
  }

  if (leagueGoal.value && badges < 8) {
    const line = gameRouteLine(run)
    return {
      label: `${badges}/8 badges earned`,
      detail: `Next: ${line.nextGoal}`
    }
  }

  const current = Number(stats?.goal_current || 0)
  const target = Number(stats?.goal_target || 0)
  return {
    label: target > 0 ? `${current}/${target} goal progress` : 'Goal in progress',
    detail: stats?.goal_summary || objectiveLabel(run)
  }
})

interface StageFact {
  key: string
  label: string
  value: string
}

const statCards = computed<StageFact[]>(() => {
  const run = selectedRun.value
  if (!run) return []
  if (isBoxxleRun(run)) {
    return [
      { key: 'solved', label: 'Puzzles solved', value: String(Number(run.game_state?.levels || 0)) },
      { key: 'crates', label: 'On goal', value: `${Number(run.game_state?.crates_on_goal || 0)}/${Number(run.game_state?.crates || 0)}` },
      { key: 'pushes', label: 'Pushes', value: String(Number(run.game_state?.pushes || 0)) },
      { key: 'runtime', label: 'Runtime', value: runtimeLabel.value }
    ]
  }
  if (isTetrisRun(run)) {
    return [
      { key: 'score', label: 'Score', value: Number(run.game_state?.score || 0).toLocaleString() },
      { key: 'lines', label: 'Lines cleared', value: String(Number(run.game_state?.lines_cleared || 0)) },
      { key: 'level', label: 'Level', value: String(Number(run.game_state?.level || 0)) },
      { key: 'runtime', label: 'Runtime', value: runtimeLabel.value }
    ]
  }
  return [
    { key: 'maps', label: 'Maps visited', value: mapsLabel.value },
    { key: 'party', label: 'Party', value: `${run.player?.party?.length || 0}/6` },
    { key: 'runtime', label: 'Runtime', value: runtimeLabel.value },
    { key: 'money', label: 'Money', value: moneyLabel(run) }
  ]
})

const routeLine = computed(() => gameRouteLine(selectedRun.value))

function hpPercent(mon: { hp: number, max_hp: number }): number {
  return mon.max_hp > 0 ? Math.max(0, Math.min(100, 100 * mon.hp / mon.max_hp)) : 0
}

function hpTone(mon: { hp: number, max_hp: number }): string {
  const percent = hpPercent(mon)
  return percent <= 20 ? 'hp-low' : percent <= 50 ? 'hp-mid' : 'hp-high'
}

function percentLabel(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '—'
  return `${Math.round(value * 100)}%`
}

function decisionLatencyLabel(seconds: number | undefined): string {
  const value = Number(seconds || 0)
  if (value <= 0) return '—'
  return value < 1 ? `${Math.round(value * 1000)}ms` : `${value.toFixed(1)}s`
}

function latestTypedDecision(run: SpectatorRun): SpectatorDecisionRecord | undefined {
  const records = run.stats?.decision_records || []
  return records[records.length - 1]
}

function typedDecisionFingerprint(record: SpectatorDecisionRecord | undefined): string {
  if (!record) return ''
  return [record.kind, record.choice, record.choice_label, record.confidence, record.duration_seconds, record.fallback].join('|')
}

function typedDecisionActivityDetail(record: SpectatorDecisionRecord): string {
  const choice = record.choice_label || record.choice || 'fallback'
  const confidence = record.fallback ? 'fallback' : percentLabel(Number(record.confidence || 0))
  const latency = decisionLatencyLabel(record.duration_seconds)
  return [choice, confidence, latency].filter(Boolean).join(' · ')
}

watch([selectedRun, selectionPinned], ([run, pinned]) => {
  if (run && !pinned) selectedRunID.value = run.run_id
  document.title = run
    ? (pinned ? `RomPilot · ${runTitle(run)}` : 'RomPilot · Watch AI play games live')
    : 'RomPilot'
}, { immediate: true })

watch(runs, (nextRuns) => {
  for (const run of nextRuns) {
    if (!isLiveRun(run)) continue
    const previous = previousRuns.get(run.run_id)
    if (!previous) {
      pushActivity(
        run.run_id,
        'area',
        'Watching from',
        displayLocation(run)
      )
      if (run.decision) pushActivity(run.run_id, 'decision', 'Current decision', run.decision)
      else if (run.planner_waiting) pushActivity(run.run_id, 'state', 'Planner thinking', 'Choosing the first objective')
      else if (run.stop_so_far) pushActivity(run.run_id, 'state', 'Run state', run.stop_so_far)
      const typed = latestTypedDecision(run)
      if (isTetrisRun(run) && typed) {
        pushActivity(run.run_id, 'decision', 'Jev decision', typedDecisionActivityDetail(typed))
      }
      previousRuns.set(run.run_id, run)
      continue
    }

    if (run.planner_waiting && previous.decision) {
      pushActivity(run.run_id, 'state', 'Planner thinking', 'Choosing the next objective')
    }
    if (run.decision && run.decision !== previous.decision) {
      pushActivity(run.run_id, 'decision', 'Decision', run.decision)
    }
    if (!isTetrisRun(run) && spectatorNativeMap(run) !== spectatorNativeMap(previous)) {
      pushActivity(run.run_id, 'area', 'Entered area', displayLocation(run))
    }
    if (isTetrisRun(run)) {
      const beforeLines = Number(previous.game_state?.lines_cleared || 0)
      const afterLines = Number(run.game_state?.lines_cleared || 0)
      if (afterLines > beforeLines) {
        pushActivity(
          run.run_id,
          'game',
          'Lines cleared',
          `${afterLines} total · ${Number(run.game_state?.score || 0).toLocaleString()} points`
        )
      }
      const typed = latestTypedDecision(run)
      const typedCallAdvanced = Number(run.stats?.decision_calls || 0) > Number(previous.stats?.decision_calls || 0)
      if (typed && (typedCallAdvanced || typedDecisionFingerprint(typed) !== typedDecisionFingerprint(latestTypedDecision(previous)))) {
        pushActivity(run.run_id, 'decision', 'Jev decision', typedDecisionActivityDetail(typed))
      }
    }

    const beforeBadges = previous.player?.badges || []
    const afterBadges = run.player?.badges || []
    if (afterBadges.length > beforeBadges.length) {
      const earned = afterBadges.filter((badge) => !beforeBadges.includes(badge))
      pushActivity(run.run_id, 'badge', 'Badge earned', earned.join(', ') || `${afterBadges.length} badges`)
    }

    const beforeParty = previous.player?.party || []
    const afterParty = run.player?.party || []
    if (afterParty.length > beforeParty.length) {
      const joined = afterParty.slice(beforeParty.length).map((mon) => mon.name).filter(Boolean)
      pushActivity(run.run_id, 'party', 'Pokémon joined', joined.join(', ') || `${afterParty.length}/6 party`)
    }

    const beforeDex = Number(previous.player?.dex_owned || 0)
    const afterDex = Number(run.player?.dex_owned || 0)
    if (afterDex > beforeDex) {
      pushActivity(run.run_id, 'dex', 'Pokédex updated', `${afterDex} owned`)
    }

    const beforeMilestones = previous.player?.milestones || []
    const afterMilestones = run.player?.milestones || []
    if (afterMilestones.length > beforeMilestones.length) {
      const earned = afterMilestones.filter((beat) => !beforeMilestones.includes(beat))
      pushActivity(run.run_id, 'milestone', 'Milestone', earned.join(', ') || afterMilestones[afterMilestones.length - 1] || 'Progress')
    }

    previousRuns.set(run.run_id, run)
  }
}, { immediate: true })

function pushActivity(runID: string, kind: ActivityKind, label: string, detail: string): void {
  if (!detail) return
  const current = activityByRun.value[runID] || []
  const latest = current[0]
  if (latest?.kind === kind && latest.label === label && latest.detail === detail) return
  activityByRun.value = {
    ...activityByRun.value,
    [runID]: [
      { id: `${Date.now()}-${kind}-${label}-${detail}`, at: Date.now(), kind, label, detail },
      ...current
    ].slice(0, 24)
  }
}

function formatGameToken(value: string): string {
  return value
    .replaceAll('_', ' ')
    .replaceAll('-', ' ')
    .replace(/\b\w/g, (letter) => letter.toUpperCase())
}

function displayLocation(run: SpectatorRun): string {
  if (isTetrisRun(run)) return locationLabel(run)
  return mapEntryForGame(run.game, spectatorNativeMap(run))?.label || locationLabel(run)
}

function selectRun(run: SpectatorRun): void {
  selectedRunID.value = run.run_id
  selectionPinned.value = true
  history.replaceState(null, '', spectatorRunPath(run.run_id))
}

function refresh(): void {
  void retry()
}

async function copyLink(): Promise<void> {
  const run = selectedRun.value
  if (!run) return
  const url = new URL(spectatorRunPath(run.run_id), window.location.origin)
  try {
    await navigator.clipboard.writeText(url.toString())
    copyState.value = 'Copied'
  } catch {
    copyState.value = 'Copy failed'
  }
  window.setTimeout(() => { copyState.value = '' }, 1500)
}

function setRendererMode(mode: RendererMode): void {
  rendererMode.value = mode
  window.localStorage.setItem('pokepilot.spectator.renderer', mode)
}

function setTheme(themeID: string): void {
  const resolved = resolvePublicRenderTheme(themeID)
  selectedThemeID.value = resolved.theme.id
  themeNotice.value = resolved.diagnostics.join(' ')
  rendererMode.value = 'modern'
  window.localStorage.setItem('pokepilot.spectator.renderer', 'modern')
  window.localStorage.setItem('pokepilot.spectator.theme', resolved.theme.id)
}

function onThemeSelect(event: Event): void {
  const target = event.target as HTMLSelectElement | null
  if (target) setTheme(target.value)
}

async function fullscreenPlayer(): Promise<void> {
  if (!playerRef.value) return
  try {
    if (document.fullscreenElement) await document.exitFullscreen()
    else await playerRef.value.requestFullscreen()
  } catch {
    // Fullscreen can be blocked by browser policy; theater mode remains available.
  }
}

function moneyLabel(run: SpectatorRun): string {
  return `₽${Number(run.player?.money || 0).toLocaleString()}`
}

function activityTimeAgo(item: ActivityItem): string {
  const seconds = Math.max(0, Math.floor((Date.now() - item.at) / 1000))
  if (seconds < 45) return 'now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return minutes + 'm ago'
  const hours = Math.floor(minutes / 60)
  return hours + 'h ago'
}
</script>

<template>
  <AppShell
    eyebrow="Live"
    title="Watch"
    subtitle=""
    mode="public"
    :public-capabilities="selectedPublicCapabilities"
    :show-intro="false"
  >
    <template #summary>
      <template v-if="snapshot">
        <span><strong class="text-white">{{ snapshot.summary.live }}</strong> live run{{ snapshot.summary.live === 1 ? '' : 's' }}</span>
        <span v-if="programming?.up_next"><strong class="text-white">Up next:</strong> {{ programming.up_next.challenge_name }}</span>
      </template>
    </template>

    <template #actions>
      <a
        v-if="selectionPinned && selectedReplayHref"
        :href="selectedReplayHref"
        class="spectator-action inline-flex items-center gap-1.5 rounded-md bg-violet-300 px-2.5 py-1.5 text-xs font-bold text-[#101820] hover:brightness-110"
        aria-label="Watch replay"
        title="Watch replay"
      >
        <PlayIcon class="size-3.5" aria-hidden="true" />
        <span class="spectator-action-label">Watch replay</span>
      </a>
      <span
        v-else-if="selectionPinned && selectedReplayRendering && selectedRun"
        class="spectator-action spectator-action-status inline-flex items-center gap-1.5 rounded-md bg-violet-300/10 px-2.5 py-1.5 text-xs font-bold text-violet-100 ring-1 ring-violet-300/20"
        role="status"
        aria-label="Replay rendering"
        title="Replay rendering"
      >
        <ArrowPathIcon class="size-3.5 motion-safe:animate-spin" aria-hidden="true" />
        <span class="spectator-action-label">{{ replayRenderProgress(selectedRun) }}</span>
      </span>
      <button
        v-if="(!selectionPinned || selectedRun?.status !== 'done') && groupedRuns.live[0] && selectedRun?.run_id !== groupedRuns.live[0].run_id"
        type="button"
        class="spectator-action inline-flex items-center gap-1.5 rounded-md bg-cyan-300 px-2.5 py-1.5 text-xs font-bold text-[#101820] hover:brightness-110"
        aria-label="Watch live"
        title="Watch live"
        @click="selectRun(groupedRuns.live[0])"
      >
        <PlayIcon class="size-3.5" aria-hidden="true" />
        <span class="spectator-action-label">Watch live</span>
      </button>
      <button
        v-else-if="(!selectionPinned || selectedRun?.status !== 'done') && groupedRuns.live[0]"
        type="button"
        class="spectator-action inline-flex items-center gap-1.5 rounded-md bg-cyan-300/15 px-2.5 py-1.5 text-xs font-bold text-cyan-100 ring-1 ring-cyan-300/20 hover:bg-cyan-300/25"
        aria-label="Watch live"
        title="Watch live"
        @click="selectRun(groupedRuns.live[0])"
      >
        <PlayIcon class="size-3.5" aria-hidden="true" />
        <span class="spectator-action-label">Watch live</span>
      </button>
      <button
        v-if="selectedRun && selectionPinned"
        type="button"
        class="spectator-action inline-flex items-center gap-1.5 rounded-md bg-white/8 px-2.5 py-1.5 text-xs font-semibold text-slate-200 ring-1 ring-white/10 hover:bg-white/12"
        :aria-label="copyState || 'Share'"
        :title="copyState || 'Share'"
        @click="copyLink"
      >
        <LinkIcon class="size-3.5" aria-hidden="true" />
        <span class="spectator-action-label">{{ copyState || 'Share' }}</span>
      </button>
    </template>

    <div v-if="!snapshot && state === 'loading'" class="mx-auto max-w-7xl py-3">
      <BroadcastLoadingScene mode="page" state="loading" />
    </div>

    <div v-else-if="!snapshot && state === 'error'" class="grid min-h-[70vh] place-items-center px-3 py-12">
      <div class="w-full max-w-xl rounded-2xl border border-amber-300/20 bg-[#151922] p-7 text-center shadow-2xl shadow-black/20">
        <div class="mx-auto flex size-12 items-center justify-center rounded-full bg-amber-300/10 ring-1 ring-amber-300/20">
          <span class="size-2.5 animate-pulse rounded-full bg-amber-300" />
        </div>
        <h2 class="mt-4 text-lg font-semibold text-white">RomPilot is reconnecting</h2>
        <p class="mx-auto mt-2 max-w-md text-sm leading-6 text-slate-400">
          {{ error || 'The public feed is temporarily unavailable. This page retries automatically.' }}
        </p>
        <button type="button" class="mt-5 inline-flex items-center gap-2 rounded-md bg-white/8 px-3 py-2 text-sm font-semibold text-white ring-1 ring-white/10 hover:bg-white/12" @click="refresh">
          <ArrowPathIcon class="size-4" aria-hidden="true" />
          Retry now
        </button>
      </div>
    </div>

    <div v-else-if="snapshot && !selectedRun" class="grid min-h-[65vh] place-items-center px-3 py-12">
      <div
        v-if="selectionPinned && selectedRunID"
        class="w-full max-w-3xl rounded-2xl border border-amber-300/20 bg-[#151922] p-5 shadow-2xl shadow-black/20 sm:p-7"
      >
        <div class="text-center">
          <div class="mx-auto flex size-12 items-center justify-center rounded-full bg-amber-300/10 ring-1 ring-amber-300/20">
            <span class="size-2.5 rounded-full bg-amber-300" />
          </div>
          <h2 class="mt-4 text-xl font-semibold text-white">This run stopped broadcasting</h2>
          <p class="mx-auto mt-2 max-w-xl text-sm leading-6 text-slate-400">
            It may have completed, paused, stalled, or stopped. Spectator mode only keeps actively running sessions in the live feed.
          </p>
          <div class="mx-auto mt-4 max-w-xl rounded-lg bg-black/20 px-3 py-2 ring-1 ring-white/8">
            <div class="text-[9px] font-semibold tracking-[0.08em] text-slate-600 uppercase">Requested run</div>
            <div class="mt-1 break-all font-mono text-[11px] text-slate-400">{{ selectedRunID }}</div>
          </div>
        </div>

        <div v-if="groupedRuns.live.length" class="mt-6">
          <section v-if="groupedRuns.live.length">
            <div class="mb-2 flex items-center justify-between gap-3">
              <h3 class="text-xs font-semibold text-slate-300">Available now</h3>
              <span class="text-[10px] text-slate-600">{{ groupedRuns.live.length }} run{{ groupedRuns.live.length === 1 ? '' : 's' }}</span>
            </div>
            <div class="space-y-1.5">
              <button
                v-for="run in groupedRuns.live.slice(0, 4)"
                :key="run.run_id"
                type="button"
                class="w-full rounded-md bg-black/10 px-3 py-2 text-left ring-1 ring-white/8 transition-colors hover:bg-white/5 hover:ring-white/15"
                @click="selectRun(run)"
              >
                <div class="flex items-center justify-between gap-3">
                  <strong class="truncate text-xs text-slate-300">{{ runTitle(run) }}</strong>
                  <StatusBadge :tone="runTone(run)">{{ runStatusLabel(run) }}</StatusBadge>
                </div>
                <div class="mt-1 flex items-center justify-between gap-3 text-[10px] text-slate-600">
                  <span class="truncate">{{ routeLabel(run) }}</span>
                  <span class="shrink-0 font-mono">{{ playSpeedLabel(run) }}</span>
                </div>
              </button>
            </div>
          </section>


        </div>

        <div v-else class="mt-6 rounded-lg border border-white/8 bg-black/10 px-4 py-4 text-center">
          <p class="text-sm text-slate-400">No other public runs are available right now.</p>
          <p class="mt-1 text-xs text-slate-600">The page will keep checking in case this session reappears or another run starts.</p>
        </div>

        <div class="mt-5 flex justify-center">
          <button type="button" class="inline-flex items-center gap-2 rounded-md bg-white/8 px-3 py-2 text-sm font-semibold text-white ring-1 ring-white/10 hover:bg-white/12" @click="refresh">
            <ArrowPathIcon class="size-4" aria-hidden="true" />
            Refresh status
          </button>
        </div>
      </div>

      <div v-else class="max-w-xl text-center">
        <div class="live-radar mx-auto grid size-16 place-items-center rounded-full border border-cyan-300/20 bg-cyan-300/5">
          <SignalIcon class="size-6 text-cyan-200" aria-hidden="true" />
        </div>
        <h2 class="mt-5 text-xl font-semibold text-white">No run is broadcasting right now</h2>
        <p class="mt-2 text-sm leading-6 text-slate-400">Spectator mode is connected. It will automatically switch to the next live run when one starts.</p>
        <div v-if="programming?.up_next" class="mt-5 rounded-xl bg-white/5 px-4 py-3 text-left ring-1 ring-white/10">
          <div class="text-[10px] font-bold tracking-[0.12em] text-cyan-300 uppercase">Up next</div>
          <div class="mt-1 text-sm font-semibold text-white">{{ programming.up_next.challenge_name }}</div>
          <div class="mt-1 text-xs text-slate-500">Challenge {{ programming.up_next.challenge_id }} · v{{ programming.up_next.challenge_version }}</div>
        </div>
      </div>
    </div>

    <div v-else-if="selectedRun" :class="['spectator-theme mx-auto max-w-[112rem] space-y-3', modeClass, gameClass, { 'is-stage': selectionPinned }]" :style="sceneStyle">
      <div v-if="programming?.up_next" class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-cyan-300/15 bg-cyan-300/5 px-3 py-2 text-xs">
        <span class="text-slate-400"><strong class="mr-1 text-cyan-200">Up next</strong>{{ programming.up_next.challenge_name }}</span>
        <span class="text-[10px] text-slate-600">{{ programming.up_next.challenge_id }} · v{{ programming.up_next.challenge_version }}</span>
      </div>

      <div v-if="state === 'stale'" class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-amber-300/20 bg-amber-300/8 px-3 py-2 text-xs text-amber-100" role="status">
        <span><strong>Connection lost.</strong> Showing the last known run state while reconnecting automatically.</span>
        <span class="flex items-center gap-2 text-amber-200/70">
          <span v-if="lastRefreshLabel">Last update {{ lastRefreshLabel }}</span>
          <button type="button" class="font-semibold text-amber-100 hover:text-white" @click="refresh">Retry now</button>
        </span>
      </div>

      <section
        v-if="selectionPinned && selectedRun.status === 'done'"
        class="flex flex-col gap-3 rounded-xl border border-violet-300/15 bg-violet-300/5 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
      >
        <div class="min-w-0">
          <div class="text-[10px] font-black tracking-[0.1em] text-violet-200 uppercase">
            {{ selectedRun.replay_ready ? 'Replay ready' : selectedReplayRendering ? 'Replay rendering' : 'Run complete' }}
          </div>
          <p class="mt-1 text-xs text-slate-400">
            <template v-if="selectedRun.replay_ready">This completed run is available in the public replay player.</template>
            <template v-else-if="selectedReplayRendering">{{ replayRenderStage(selectedRun) }} · {{ replayRenderProgress(selectedRun) }}</template>
            <template v-else>The run has ended. Replay media is not available yet.</template>
          </p>
        </div>
        <a
          v-if="selectedReplayHref"
          :href="selectedReplayHref"
          class="inline-flex shrink-0 items-center justify-center gap-1.5 rounded-lg bg-violet-300 px-3 py-2 text-xs font-black text-[#101820] hover:brightness-110"
        >
          <PlayIcon class="size-4" aria-hidden="true" />
          Watch replay
        </a>
        <span
          v-else-if="selectedReplayRendering"
          class="shrink-0 font-mono text-[10px] text-violet-200"
          role="status"
        >
          {{ replayRenderProgress(selectedRun) }}
        </span>
      </section>

      <PublicHome
        v-if="!selectionPinned && snapshot"
        :run="selectedRun"
        :live-runs="groupedRuns.live"
        :summary="snapshot.summary"
        :frame-url="frameURL"
        :frame-state="frameState"
        @select="selectRun"
      />

      <div v-if="selectionPinned" class="stage">
        <header class="stage-head">
          <div class="stage-title">
            <span :class="isPuzzleSelected ? 'tetris-mark' : 'pokeball-mark'" aria-hidden="true" />
            <h1>{{ gameTitle(selectedRun) }}</h1>
            <span class="stage-sub">{{ isTetrisSelected ? tetrisModeLabel : isBoxxleSelected ? 'Autonomous puzzle play' : playStyleLabel(selectedRun) + ' · ' + (selectedRun.player?.party?.[0]?.name || selectedRun.starter || 'new trainer') }}</span>
          </div>
          <div class="stage-status">
            <span v-if="isLiveRun(selectedRun)" class="live-tag"><i />Live</span>
            <StatusBadge v-else :tone="runTone(selectedRun)">{{ runStatusLabel(selectedRun) }}</StatusBadge>
            <span>{{ playSpeedLabel(selectedRun) }}</span>
            <span v-if="selectedRun.purpose === 'debug_coverage'">Debug coverage</span>
          </div>
        </header>

        <div class="stage-body">
          <aside v-if="!isPuzzleSelected" class="stat-rail" aria-label="Run stats">
            <dl class="facts">
              <div v-for="stat in statCards" :key="stat.key">
                <dt>{{ stat.label }}</dt>
                <dd>{{ stat.value }}</dd>
              </div>
            </dl>

            <section v-if="otherLiveRuns.length" class="others rail-others">
              <div class="block-head">
                <h2>Other live runs</h2>
                <span>{{ groupedRuns.live.length }} live</span>
              </div>
              <ul>
                <li v-for="run in otherLiveRuns" :key="run.run_id">
                  <button type="button" @click="selectRun(run)">
                    <strong>{{ runTitle(run) }}</strong>
                    <span>{{ displayLocation(run) }}</span>
                  </button>
                </li>
              </ul>
            </section>
          </aside>

          <div class="screen-col">
            <section class="bezel" :aria-label="'Game screen for ' + gameTitle(selectedRun)">
              <div
                ref="playerRef"
                :class="['screen group', showModern ? 'is-wide' : 'is-gb', { 'is-theater': theaterMode }]"
              >
                <ModernSceneRenderer
                  v-if="showModern && renderState"
                  :state="renderState"
                  :theme="activeTheme"
                />

                <img
                  v-else-if="frameURL"
                  :src="frameURL"
                  :alt="'Live frame for ' + selectedRun.run_id"
                  class="absolute inset-0 h-full w-full object-contain object-center [image-rendering:pixelated]"
                />

                <div v-else class="screen-wait">
                  <p>Waiting for the live stream</p>
                  <p v-if="rendererMode === 'modern' && renderStateStatus === 'error' && renderStateError" class="warn">{{ renderStateError }}</p>
                  <p v-else-if="frameState === 'error' && frameError" class="warn">{{ frameError }}</p>
                </div>

                <div
                  v-if="plannerState && !isTetrisSelected"
                  class="thinking-overlay"
                  role="status"
                  aria-live="polite"
                  aria-atomic="true"
                >
                  <div class="thinking-card">
                    <span class="thinking-dots" aria-hidden="true"><i /><i /><i /></span>
                    <span class="thinking-copy">
                      <strong>{{ plannerState.title }}</strong>
                      <small>{{ plannerState.detail }}</small>
                    </span>
                  </div>
                </div>

                <div class="screen-controls">
                  <div v-if="!isPuzzleSelected" class="seg" role="group" aria-label="Renderer">
                    <button type="button" :aria-pressed="rendererMode === 'modern'" title="Render the live semantic world" @click="setRendererMode('modern')">Gold / Silver</button>
                    <button type="button" :aria-pressed="rendererMode === 'classic'" title="Show the classic emulator framebuffer" @click="setRendererMode('classic')">Classic</button>
                  </div>
                  <label v-if="!isPuzzleSelected && rendererMode === 'modern'" class="theme-pick">
                    <span>Theme</span>
                    <select :value="selectedThemeID" title="Choose your spectator theme" @change="onThemeSelect">
                      <option v-for="theme in themeOptions" :key="theme.id" :value="theme.id">{{ theme.name }}</option>
                    </select>
                  </label>
                  <p v-if="!isPuzzleSelected && themeNotice" class="warn-chip">{{ themeNotice }}</p>
                  <button type="button" class="icon-btn" :title="theaterMode ? 'Exit theater mode' : 'Theater mode'" @click="theaterMode = !theaterMode">
                    <PlayIcon class="size-4" aria-hidden="true" />
                  </button>
                  <button type="button" class="icon-btn" title="Fullscreen" @click="fullscreenPlayer">
                    <ArrowsPointingOutIcon class="size-4" aria-hidden="true" />
                  </button>
                </div>

                <p v-if="modernFallbackLabel && frameURL" class="screen-note">{{ modernFallbackLabel }}</p>
                <p v-else-if="!showModern && frameURL && frameState === 'error'" class="screen-note warn">Last frame · reconnecting</p>
              </div>
              <div class="bezel-label">
                <span>{{ isPuzzleSelected || !showModern ? 'Classic framebuffer' : activeTheme.name }}</span>
                <span>{{ currentLocation }}</span>
              </div>
            </section>
            <div v-if="isPuzzleSelected" class="readout">
              <dl class="facts">
                <div v-for="stat in statCards" :key="stat.key">
                  <dt>{{ stat.label }}</dt>
                  <dd>{{ stat.value }}</dd>
                </div>
              </dl>
              <section v-if="isBoxxleSelected" class="board-block" aria-label="Board">
                <div class="block-head">
                  <h2>Board</h2>
                  <span>{{ boxxleState?.solved ? 'Solved' : (boxxleState?.screen || 'puzzle') }}</span>
                </div>
                <div class="board-row">
                  <div v-if="boxxleBoardRows.length" class="boxxle-board" aria-label="Boxxle board">
                    <div v-for="(row, y) in boxxleBoardRows" :key="y" class="boxxle-board-row">
                      <span
                        v-for="(cell, x) in row"
                        :key="x"
                        :class="['boxxle-cell', 'boxxle-cell-' + (cell === '#' ? 'wall' : cell === '$' ? 'crate' : cell === '*' ? 'crate-goal' : cell === '+' ? 'goal' : cell === '@' ? 'player' : 'floor')]"
                      >{{ cell === '.' || cell === '#' ? '' : cell }}</span>
                    </div>
                  </div>
                  <p v-else class="muted">Waiting for board</p>
                </div>
              </section>
              <section v-if="isTetrisSelected" class="board-block" aria-label="Board">
                <div class="block-head">
                  <h2>Board</h2>
                  <span>{{ tetrisState?.game_over ? 'Game over' : tetrisState?.paused ? 'Paused' : tetrisState?.clearing ? 'Clearing' : 'Playing' }}</span>
                </div>
                <div class="board-row">
                  <div v-if="tetrisBoardRows.length" class="tetris-board" aria-label="Tetris board">
                    <div v-for="(row, y) in tetrisBoardRows" :key="y" class="tetris-board-row">
                      <span
                        v-for="(cell, x) in row"
                        :key="x"
                        :class="['tetris-cell', cell === '#' ? 'tetris-cell-filled' : 'tetris-cell-empty']"
                      />
                    </div>
                  </div>
                  <p v-else class="muted">Waiting for board</p>
                  <dl class="pieces">
                    <div><dt>Active</dt><dd class="piece-active">{{ tetrisActivePiece }}</dd></div>
                    <div><dt>Next</dt><dd class="piece-next">{{ tetrisNextPiece }}</dd></div>
                  </dl>
                </div>
              </section>
            </div>
          </div>

          <aside class="side">
            <section class="now">
              <h2>Now</h2>
              <p class="now-place">{{ currentLocation }}</p>
              <p v-if="plannerState" class="muted">{{ plannerState.title }} · {{ plannerState.detail }}</p>
            </section>

            <section class="goal" aria-label="Goal progress">
              <h2>Goal</h2>
              <div class="goal-row">
                <span>{{ objectiveLabel(selectedRun) }}</span>
                <strong>{{ goalPercent.toFixed(0) }}%</strong>
              </div>
              <div
                :class="['goal-bar', { 'is-blocks': isPuzzleSelected }]"
                role="progressbar"
                aria-label="Goal progress"
                aria-valuemin="0"
                aria-valuemax="100"
                :aria-valuenow="Math.round(goalPercent)"
              >
                <i :style="{ width: goalPercent + '%' }" />
              </div>
              <p class="muted">{{ goalProgressCopy.label }} · {{ goalProgressCopy.detail }}</p>
              <div v-if="!isPuzzleSelected" class="next-goal">
                <span>Next goal</span>
                <strong>{{ routeLine.nextGoal }}</strong>
              </div>
            </section>

            <section>
              <h2>Latest decision</h2>
              <p class="say">{{ isTetrisSelected && tetrisDecisionVisible ? tetrisLatestChoice : selectedRun.decision || 'Preparing the next objective' }}</p>
            </section>

            <section v-if="tetrisDecisionVisible" class="jev">
              <div class="block-head">
                <h2>Placement model</h2>
                <span>{{ tetrisDecisionCalls }} calls</span>
              </div>
              <p class="muted">{{ tetrisDecisionIdentity }} · {{ formatGameToken(tetrisDecisionBackend || 'jev') }} · {{ formatGameToken(tetrisDecisionMode) }}</p>
              <dl class="ruled">
                <div><dt>Latest confidence</dt><dd>{{ tetrisDecisionCalls ? percentLabel(tetrisDecisionConfidence) : '—' }}</dd></div>
                <div>
                  <dt>Policy agreement</dt>
                  <dd>
                    {{ percentLabel(tetrisDecisionAgreement) }}
                    <small v-if="tetrisDecisionReference.judged">{{ tetrisDecisionReference.agreed }}/{{ tetrisDecisionReference.judged }}</small>
                  </dd>
                </div>
                <div><dt>Fallback rate</dt><dd>{{ percentLabel(tetrisDecisionFallbackRate) }}</dd></div>
                <div><dt>Avg latency</dt><dd>{{ decisionLatencyLabel(selectedRun.stats?.decision_avg_seconds) }}</dd></div>
              </dl>
              <h3 v-if="tetrisDecisionRecords.length" class="sub-head">Recent Jev choices</h3>
              <ul v-if="tetrisDecisionRecords.length" class="ruled choices">
                <li v-for="(decision, index) in tetrisDecisionRecords.slice(0, 4)" :key="`${decision.choice || decision.choice_label}-${index}`">
                  <span>{{ decision.choice_label || decision.choice || 'fallback' }}</span>
                  <span :class="{ warn: decision.fallback }">{{ decision.fallback ? 'fallback' : percentLabel(Number(decision.confidence || 0)) }}</span>
                </li>
              </ul>
              <p class="muted fine">Policy agreement compares Jev with PokePilot's deterministic best-placement scorer. It is a reference metric, not ground-truth accuracy.</p>
            </section>

            <section class="activity">
              <div class="block-head">
                <h2>Activity</h2>
                <div class="tabs" role="group" aria-label="Filter activity">
                  <button
                    v-for="filter in activityFilters"
                    :key="filter"
                    type="button"
                    :aria-pressed="activityFilter === filter"
                    @click="activityFilter = filter"
                  >{{ filter === 'milestones' ? 'Progress' : filter }}</button>
                </div>
              </div>
              <ol v-if="selectedActivity.length" class="log">
                <li v-for="item in selectedActivity.slice(0, 9)" :key="item.id">
                  <time>{{ activityTimeAgo(item) }}</time>
                  <span><strong>{{ item.label }}</strong> {{ item.detail }}</span>
                </li>
              </ol>
              <p v-else class="muted">Waiting for the next live event.</p>
            </section>

            <section v-if="!isTetrisSelected && otherLiveRuns.length" class="others mobile-other-runs">
              <div class="block-head">
                <h2>Other live runs</h2>
                <span>{{ groupedRuns.live.length }} live</span>
              </div>
              <ul>
                <li v-for="run in otherLiveRuns" :key="run.run_id">
                  <button type="button" @click="selectRun(run)">
                    <strong>{{ runTitle(run) }}</strong>
                    <span>{{ displayLocation(run) }}</span>
                  </button>
                </li>
              </ul>
            </section>
          </aside>
        </div>

        <WorldExplorer
          v-if="!isPuzzleSelected && selectedPublicCapabilities.includes('worldMap')"
          :run="selectedRun"
        />

            <section v-if="!isPuzzleSelected" class="route-block" aria-label="Road to the League">
              <div class="block-head">
                <h2>Road to the League</h2>
                <span>{{ routeLine.completedGoals }} of {{ routeLine.goalCount }} goals</span>
              </div>
              <RouteLine :line="routeLine" />
            </section>

        <section v-if="!isPuzzleSelected" class="party" aria-label="Party">
          <div class="block-head">
            <h2>Party</h2>
            <span>{{ selectedRun.player?.party?.length || 0 }} of 6</span>
          </div>
          <ul v-if="selectedRun.player?.party?.length" class="roster">
            <li v-for="(mon, index) in selectedRun.player.party" :key="mon.name + '-' + index">
              <PokemonSprite :name="mon.name" :size="48" :fainted="mon.hp <= 0" />
              <div class="mon">
                <span class="mon-name">
                  <strong>{{ mon.name || 'Unknown' }}</strong>
                  <span>Lv {{ mon.level }}</span>
                </span>
                <span class="hp" role="img" :aria-label="'HP ' + mon.hp + ' of ' + mon.max_hp">
                  <i :class="hpTone(mon)" :style="{ width: hpPercent(mon) + '%' }" />
                </span>
                <span class="mon-meta">
                  {{ mon.hp }}/{{ mon.max_hp }}
                  <template v-if="mon.hp <= 0"> · fainted</template>
                  <template v-else-if="mon.status && mon.status.toLowerCase() !== 'healthy'"> · {{ mon.status }}</template>
                  <template v-if="index === 0"> · lead</template>
                </span>
              </div>
            </li>
          </ul>
          <p v-else class="muted">Party data is not available yet.</p>
        </section>
      </div>
    </div>
  </AppShell>
</template>

<style scoped>
:fullscreen {
  background: #05070a;
}

.spectator-action-label {
  display: inline;
}

.game-pokemon .mobile-other-runs {
  display: none;
}

/* The stage sits directly on the page; only PublicHome keeps the night-scape backdrop. */
.spectator-theme.is-stage {
  background: none;
  box-shadow: none;
}

.spectator-theme.is-stage::before {
  display: none;
}

.spectator-theme {
  --mode-accent: #67e8f9;
  --mode-soft: rgba(103, 232, 249, 0.1);
  --mode-border: rgba(103, 232, 249, 0.28);
  position: relative;
  border-radius: 1.5rem;
  background:
    linear-gradient(180deg, rgba(6, 11, 21, .48), rgba(6, 11, 21, .82)),
    var(--spectator-art) center 72% / cover no-repeat;
  box-shadow: 0 26px 80px rgba(0,0,0,.18);
}

.spectator-theme::before {
  position: absolute;
  inset: -1rem;
  z-index: -1;
  border-radius: 2rem;
  background:
    radial-gradient(circle at 10% 70%, var(--mode-soft), transparent 24rem),
    radial-gradient(circle at 92% 76%, rgba(103,232,249,.08), transparent 22rem);
  content: '';
  filter: blur(10px);
  pointer-events: none;
}

.mode-speedrun { --mode-accent: #fb7185; --mode-soft: rgba(251, 113, 133, 0.1); --mode-border: rgba(251, 113, 133, 0.28); }
.mode-adventure { --mode-accent: #67e8f9; --mode-soft: rgba(103, 232, 249, 0.1); --mode-border: rgba(103, 232, 249, 0.28); }
.mode-completionist { --mode-accent: #c084fc; --mode-soft: rgba(192, 132, 252, 0.1); --mode-border: rgba(192, 132, 252, 0.3); }
.mode-team_builder { --mode-accent: #6ee7b7; --mode-soft: rgba(110, 231, 183, 0.1); --mode-border: rgba(110, 231, 183, 0.28); }

.live-radar {
  position: relative;
}

.live-radar::before,
.live-radar::after {
  position: absolute;
  inset: -0.55rem;
  border: 1px solid rgba(103, 232, 249, 0.18);
  border-radius: 9999px;
  content: '';
  animation: radar-ring 2.2s ease-out infinite;
}

.live-radar::after {
  animation-delay: 1.1s;
}

@keyframes radar-ring {
  0% { opacity: 0.75; transform: scale(0.72); }
  100% { opacity: 0; transform: scale(1.35); }
}

/* Stage tokens. Pokémon is the default; Tetris overrides them below. */
.stage {
  --page: var(--poke-bg);
  --bone: #e8e6dc;
  --dusk: #9098b3;
  --rule: #2a3254;
  --accent: #f0c43a;
  --live: #ef6a5e;
  --bezel: #2b3050;
  display: grid;
  gap: 1.5rem;
  color: var(--bone);
  font-family: ui-rounded, "Avenir Next", "Segoe UI", system-ui, sans-serif;
  font-size: 0.95rem;
  line-height: 1.5;
}

.game-tetris .stage {
  --rule: #26303a;
  --accent: #4fd6e8;
  --bezel: #1a1d26;
  --i: #4fd6e8;
  --o: #f0c43a;
  --t: #a77bdb;
  --s: #5fd068;
  --z: #ef5a5a;
  --j: #4a7be0;
  --l: #f0903a;
  padding: 1.25rem;
  background:
    linear-gradient(var(--rule) 1px, transparent 1px) 0 0 / 1.5rem 1.5rem,
    linear-gradient(90deg, var(--rule) 1px, transparent 1px) 0 0 / 1.5rem 1.5rem;
  background-color: color-mix(in srgb, var(--page) 88%, black);
  background-blend-mode: soft-light;
}

.stage h1,
.stage h2 {
  margin: 0;
}

.stage-head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.5rem 1.5rem;
}

.stage-title {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.25rem 0.75rem;
  min-width: 0;
}

.stage-title h1 {
  font-size: clamp(2rem, 5vw, 3.25rem);
  font-stretch: 68%;
  font-weight: 800;
  line-height: 1;
}

.game-tetris .stage-title h1 {
  letter-spacing: 0.06em;
  font-stretch: 85%;
}

.stage-sub {
  color: var(--dusk);
}

.stage-status {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.25rem 1rem;
  color: var(--dusk);
  font-size: 0.875rem;
}

.live-tag {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  color: var(--bone);
  font-weight: 600;
}

.live-tag i {
  width: 0.6rem;
  height: 0.6rem;
  border-radius: 50%;
  background: var(--live);
  animation: live-pulse 1.8s ease-in-out infinite;
}

@keyframes live-pulse {
  50% { opacity: 0.3; }
}

.pokeball-mark {
  position: relative;
  display: block;
  width: 1.65rem;
  height: 1.65rem;
  overflow: hidden;
  border: 2px solid var(--bone);
  border-radius: 9999px;
  background: linear-gradient(to bottom, #ef6a5e 0 46%, #e2e8f0 46% 54%, #f8fafc 54% 100%);
}

.pokeball-mark::before {
  position: absolute;
  top: 50%;
  left: 0;
  width: 100%;
  height: 2px;
  background: #0c1020;
  content: '';
  transform: translateY(-50%);
}

.pokeball-mark::after {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 0.52rem;
  height: 0.52rem;
  border: 2px solid #0c1020;
  border-radius: 9999px;
  background: white;
  content: '';
  transform: translate(-50%, -50%);
}

/* A T-tetromino in the piece colors. */
.tetris-mark {
  display: block;
  flex: none;
  width: 0.55rem;
  height: 0.55rem;
  margin-right: 1.1rem;
  background: var(--t);
  box-shadow: 0.6rem 0 var(--t), 1.2rem 0 var(--t), 0.6rem 0.6rem var(--t);
}

.stage-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 22rem;
  gap: 2rem;
  align-items: start;
}

.screen-col,
.side {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 1.5rem;
  min-width: 0;
}

.block-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.75rem;
  margin-bottom: 0.5rem;
  color: var(--dusk);
  font-size: 0.85rem;
}

.stage h2 {
  color: var(--dusk);
  font-size: 0.95rem;
  font-stretch: 85%;
  font-weight: 700;
}

.route-block .block-head h2,
.party .block-head h2 {
  color: var(--bone);
  font-size: 1rem;
}

.muted {
  margin: 0.25rem 0 0;
  color: var(--dusk);
  font-size: 0.875rem;
}

.sub-head {
  margin: 0.75rem 0 0;
  color: var(--dusk);
  font-size: 0.8rem;
  font-weight: 600;
}

.sub-head + .ruled {
  margin-top: 0.25rem;
}

.muted.fine {
  font-size: 0.75rem;
}

.warn {
  color: #f0c43a;
}

/* Game screen */
.bezel {
  padding: 0.75rem 0.75rem 1rem;
  border-radius: 6px 6px 22px 6px;
  background: var(--bezel);
}

.game-tetris .bezel {
  border-radius: 6px;
  box-shadow: inset 0 0 0 2px var(--rule);
}

.screen {
  position: relative;
  width: 100%;
  margin-inline: auto;
  overflow: hidden;
  background: #000;
}

.screen.is-gb { aspect-ratio: 10 / 9; }
.screen.is-wide { aspect-ratio: 4 / 3; }
.screen.is-theater { max-height: 88vh; }

.screen-wait {
  position: absolute;
  inset: 0;
  display: grid;
  align-content: center;
  justify-items: center;
  gap: 0.25rem;
  padding: 1.5rem;
  color: var(--dusk);
  text-align: center;
}

.screen-wait p { margin: 0; }

.screen-controls {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  z-index: 10;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 0.375rem;
  opacity: 0.85;
  transition: opacity 0.15s;
}

.screen:hover .screen-controls,
.screen:focus-within .screen-controls {
  opacity: 1;
}

.seg,
.theme-pick {
  display: flex;
  align-items: center;
  overflow: hidden;
  border-radius: 3px;
  background: rgb(0 0 0 / 70%);
  font-size: 0.7rem;
}

.seg button {
  padding: 0.35rem 0.6rem;
  color: var(--dusk);
  background: none;
  border: 0;
  cursor: pointer;
}

.seg button[aria-pressed='true'] {
  color: var(--page);
  background: var(--bone);
}

.theme-pick {
  gap: 0.5rem;
  padding: 0.25rem 0.5rem;
  color: var(--dusk);
}

.theme-pick select {
  max-width: 9rem;
  background: transparent;
  color: var(--bone);
  border: 0;
  font-size: 0.75rem;
}

.icon-btn {
  display: grid;
  width: 2rem;
  height: 2rem;
  place-items: center;
  color: var(--bone);
  background: rgb(0 0 0 / 70%);
  border: 0;
  border-radius: 3px;
  cursor: pointer;
}

.icon-btn:hover,
.seg button:hover {
  background: rgb(255 255 255 / 15%);
}

.warn-chip {
  max-width: 14rem;
  margin: 0;
  padding: 0.25rem 0.5rem;
  border-radius: 3px;
  background: rgb(60 40 0 / 85%);
  color: #f0c43a;
  font-size: 0.7rem;
  text-align: right;
}

.screen-note {
  position: absolute;
  left: 0.5rem;
  bottom: 0.5rem;
  margin: 0;
  padding: 0.2rem 0.5rem;
  border-radius: 3px;
  background: rgb(0 0 0 / 70%);
  color: var(--bone);
  font-size: 0.7rem;
}

.bezel-label {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  margin-top: 0.6rem;
  color: var(--dusk);
  font-size: 0.8rem;
  font-stretch: 75%;
  font-weight: 600;
  letter-spacing: 0.04em;
}

button:focus-visible,
select:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

/* Facts, party, side column */
.facts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(7.5rem, 1fr));
  margin: 0;
  border-top: 1px solid var(--dusk);
}

.facts div {
  padding: 0.75rem 1rem 0.75rem 0;
  border-bottom: 1px solid var(--rule);
}

.facts dt {
  color: var(--dusk);
  font-size: 0.8rem;
}

.facts dd {
  margin: 0.125rem 0 0;
  font-size: 1.6rem;
  font-stretch: 78%;
  font-weight: 700;
  line-height: 1.2;
}

.game-tetris .facts dd {
  font-size: 2rem;
}

.game-tetris .facts div:nth-child(1) { box-shadow: inset 0 -3px var(--o); }
.game-tetris .facts div:nth-child(2) { box-shadow: inset 0 -3px var(--i); }
.game-tetris .facts div:nth-child(3) { box-shadow: inset 0 -3px var(--t); }
.game-tetris .facts div:nth-child(4) { box-shadow: inset 0 -3px var(--s); }

.roster {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--dusk);
  list-style: none;
}

.readout {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 1.25rem;
  min-width: 0;
}

.roster li {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.6rem 1rem 0.6rem 0;
  border-bottom: 1px solid var(--rule);
}

.mon {
  display: grid;
  flex: 1;
  gap: 0.2rem;
  min-width: 0;
}

.mon-name {
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
}

.mon-name span,
.mon-meta {
  color: var(--dusk);
  font-size: 0.8rem;
}

.hp {
  display: block;
  height: 5px;
  background: var(--rule);
}

.hp i {
  display: block;
  height: 100%;
}

.hp .hp-high { background: #7bc96f; }
.hp .hp-mid { background: #f0c43a; }
.hp .hp-low { background: #ef6a5e; }

.now-place {
  margin: 0;
  font-size: 1.9rem;
  font-stretch: 78%;
  font-weight: 700;
  line-height: 1.1;
}

.goal-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.75rem;
}

.goal-row strong {
  font-size: 2rem;
  font-stretch: 78%;
  font-weight: 800;
  line-height: 1;
}

.goal-bar {
  height: 6px;
  margin: 0.6rem 0 0.25rem;
  background: var(--rule);
}

.goal-bar i {
  display: block;
  height: 100%;
  background: var(--accent);
  transition: width 0.4s;
}

/* Tetris progress is drawn as a row of tetromino-colored blocks. */
.goal-bar.is-blocks {
  height: 12px;
}

.goal-bar.is-blocks i {
  background: linear-gradient(90deg, var(--i) 0 14.28%, var(--j) 0 28.56%, var(--t) 0 42.84%, var(--s) 0 57.12%, var(--o) 0 71.4%, var(--l) 0 85.68%, var(--z) 0);
  background-size: 100% 100%;
}

.say {
  max-width: 34ch;
  margin: 0;
  font-size: 1.05rem;
  line-height: 1.55;
}

.board-row {
  display: flex;
  gap: 1rem;
  align-items: flex-start;
}

.tetris-board {
  display: grid;
  flex: none;
  gap: 1px;
  width: 8rem;
  padding: 4px;
  background: #06080d;
  box-shadow: inset 0 0 0 2px var(--rule);
}

.tetris-board-row {
  display: grid;
  grid-template-columns: repeat(10, minmax(0, 1fr));
  gap: 1px;
}

.tetris-cell {
  aspect-ratio: 1;
}

.tetris-cell-empty {
  background: rgb(148 163 184 / 5%);
}


.game-boxxle .stage {
  --o: #67e8f9;
  --i: #fbbf24;
  --t: #34d399;
  --s: #a78bfa;
}
.boxxle-board {
  display: grid;
  gap: 1px;
  background: rgba(255,255,255,0.08);
  padding: 1px;
  border: 1px solid rgba(255,255,255,0.12);
}
.boxxle-board-row {
  display: flex;
}
.boxxle-cell {
  width: 14px;
  height: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-family: ui-monospace, monospace;
  font-size: 9px;
  line-height: 1;
}
.boxxle-cell-wall { background: rgba(100,116,139,0.9); }
.boxxle-cell-floor { background: rgba(255,255,255,0.04); }
.boxxle-cell-crate { background: rgba(251,191,36,0.8); color: #422006; }
.boxxle-cell-crate-goal { background: rgba(52,211,153,0.8); color: #064e3b; }
.boxxle-cell-goal { background: rgba(6,78,59,0.8); color: #a7f3d0; }
.boxxle-cell-player { background: rgba(103,232,249,0.9); color: #083344; }

.tetris-cell-filled {
  background: var(--o);
  box-shadow: inset 0 0 0 1px rgb(255 255 255 / 25%);
}

.pieces {
  display: grid;
  gap: 0.75rem;
  margin: 0;
}

.pieces dt {
  color: var(--dusk);
  font-size: 0.8rem;
}

.pieces dd {
  margin: 0;
  font-size: 2rem;
  font-stretch: 78%;
  font-weight: 800;
  line-height: 1.1;
}

.piece-active { color: var(--o); }
.piece-next { color: var(--i); }

.ruled {
  margin: 0.5rem 0 0;
  padding: 0;
  border-top: 1px solid var(--dusk);
  list-style: none;
}

.ruled > div,
.ruled > li {
  display: flex;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.4rem 0;
  border-bottom: 1px solid var(--rule);
  font-size: 0.875rem;
}

.ruled dt,
.ruled small {
  color: var(--dusk);
}

.ruled dd {
  margin: 0;
}

.tabs {
  display: flex;
  gap: 0.875rem;
}

.tabs button {
  padding: 2px 0;
  color: var(--dusk);
  background: none;
  border: 0;
  border-bottom: 2px solid transparent;
  cursor: pointer;
  font-size: 0.85rem;
  text-transform: capitalize;
}

.tabs button[aria-pressed='true'] {
  color: var(--bone);
  border-bottom-color: var(--accent);
}

.log {
  margin: 0;
  padding: 0;
  list-style: none;
}

.log li {
  display: grid;
  grid-template-columns: 3.4rem minmax(0, 1fr);
  gap: 0.6rem;
  padding: 0.6rem 0;
  border-top: 1px solid var(--rule);
  font-size: 0.9rem;
}

.log li:first-child {
  border-top-color: var(--dusk);
}

.log time {
  color: var(--dusk);
}

.log strong {
  font-weight: 600;
}

.others ul {
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--dusk);
  list-style: none;
}

.others button {
  display: grid;
  gap: 0.125rem;
  width: 100%;
  padding: 0.6rem 0;
  color: inherit;
  text-align: left;
  background: none;
  border: 0;
  border-bottom: 1px solid var(--rule);
  cursor: pointer;
}

.others button:hover strong {
  text-decoration: underline;
}

.others button span {
  color: var(--dusk);
  font-size: 0.85rem;
}

/* Wide screens: the game screen is sized to leave room for the readout beside it and the route below. */
@media (min-width: 64rem) {
  .screen-col {
    grid-template-columns: min(100%, max(20rem, calc(52vh * 10 / 9 + 1.5rem))) minmax(0, 1fr);
    align-items: start;
  }

  .screen-col > .bezel { grid-column: 1; grid-row: 1; }
  .screen-col > .readout { grid-column: 2; grid-row: 1; }
  .screen-col > .route-block { grid-column: 1 / -1; grid-row: 2; }

  .readout .facts { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .readout .roster { grid-template-columns: repeat(2, minmax(0, 1fr)); }

  /* Theater mode gives the screen the whole column back. */
  .screen-col:has(.is-theater) { grid-template-columns: minmax(0, 1fr); }
  .screen-col:has(.is-theater) > .bezel,
  .screen-col:has(.is-theater) > .readout,
  .screen-col:has(.is-theater) > .route-block { grid-column: 1; grid-row: auto; }
}

@media (max-width: 64rem) {
  .stage-body {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (prefers-reduced-motion: reduce) {
  .live-tag i,
  .live-radar::before,
  .live-radar::after {
    animation: none;
  }

  .goal-bar i {
    transition: none;
  }
}

/* Pokémon run detail: a modern trainer HUD rather than a generic dashboard. */
.game-pokemon .stage {
  --page: #080b18;
  --bone: #f3efd9;
  --dusk: #9ca7c6;
  --rule: #323b62;
  --accent: #f2ca52;
  --live: #ef5e57;
  --bezel: #232947;
  gap: 1rem;
  padding: clamp(0.8rem, 1.6vw, 1.45rem);
  border: 1px solid rgba(99, 111, 168, 0.34);
  border-radius: 1.35rem;
  background:
    radial-gradient(circle at 9% 0%, rgba(239, 94, 87, 0.09), transparent 21rem),
    radial-gradient(circle at 88% 6%, rgba(242, 202, 82, 0.06), transparent 20rem),
    linear-gradient(180deg, rgba(19, 24, 49, 0.82), rgba(8, 11, 24, 0.96));
  box-shadow:
    0 24px 70px rgba(0, 0, 0, 0.28),
    inset 0 1px rgba(255, 255, 255, 0.035);
}

.game-pokemon .stage-head {
  position: relative;
  align-items: center;
  padding: 0.85rem 1rem 0.95rem;
  overflow: hidden;
  border: 1px solid rgba(110, 123, 183, 0.34);
  border-radius: 1rem;
  background:
    linear-gradient(90deg, rgba(239, 94, 87, 0.09), transparent 28%),
    rgba(11, 15, 32, 0.7);
  box-shadow: inset 0 1px rgba(255, 255, 255, 0.035);
}

.game-pokemon .stage-head::after {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 3px;
  background: linear-gradient(90deg, #e84f4a 0 16%, #f2ca52 16% 38%, #5968a8 38% 100%);
  content: "";
  opacity: 0.9;
}

.game-pokemon .stage-title {
  gap: 0.35rem 0.8rem;
}

.game-pokemon .stage-title h1 {
  color: #fffaf0;
  font-family: ui-rounded, "Avenir Next", "Trebuchet MS", "Segoe UI", system-ui, sans-serif;
  font-size: clamp(2rem, 4vw, 3rem);
  font-stretch: normal;
  font-weight: 850;
  letter-spacing: -0.045em;
  text-shadow: 0 2px 18px rgba(0, 0, 0, 0.24);
}

.game-pokemon .stage-sub {
  padding: 0.25rem 0.55rem;
  border: 1px solid rgba(242, 202, 82, 0.16);
  border-radius: 999px;
  background: rgba(242, 202, 82, 0.065);
  color: #d5cda9;
  font-size: 0.78rem;
  font-weight: 700;
}

.game-pokemon .stage-status {
  gap: 0.4rem;
}

.game-pokemon .stage-status > span {
  padding: 0.27rem 0.55rem;
  border: 1px solid rgba(148, 163, 184, 0.13);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.035);
  color: #aeb8d4;
  font-size: 0.76rem;
  font-weight: 700;
}

.game-pokemon .stage-status .live-tag {
  border-color: rgba(239, 94, 87, 0.24);
  background: rgba(239, 94, 87, 0.09);
  color: #ffe5df;
}

.game-pokemon .stage-body {
  grid-template-columns: minmax(0, 1fr) minmax(19rem, 22rem);
  gap: 1rem;
}

.game-pokemon .screen-col,
.game-pokemon .side {
  gap: 1rem;
}

.game-pokemon .bezel {
  position: relative;
  padding: 0.85rem 0.85rem 1rem;
  overflow: hidden;
  border: 1px solid rgba(126, 139, 198, 0.38);
  border-top: 3px solid rgba(242, 202, 82, 0.76);
  border-radius: 1rem 1rem 1.85rem 1rem;
  background:
    linear-gradient(145deg, rgba(61, 70, 117, 0.94), rgba(30, 35, 65, 0.98)),
    var(--bezel);
  box-shadow:
    0 16px 32px rgba(0, 0, 0, 0.28),
    inset 0 1px rgba(255, 255, 255, 0.07);
}

.game-pokemon .bezel::after {
  position: absolute;
  right: 1.2rem;
  bottom: 0.5rem;
  width: 2.5rem;
  height: 3px;
  border-radius: 999px;
  background: rgba(239, 94, 87, 0.72);
  box-shadow: -0.8rem 0 rgba(242, 202, 82, 0.72);
  content: "";
}

.game-pokemon .screen {
  border: 1px solid rgba(0, 0, 0, 0.8);
  border-radius: 0.55rem;
  box-shadow:
    0 0 0 3px rgba(4, 7, 18, 0.62),
    inset 0 0 0 1px rgba(255, 255, 255, 0.04);
}

.game-pokemon .bezel-label {
  color: #b8c0da;
  font-size: 0.72rem;
  font-stretch: normal;
  font-weight: 750;
  letter-spacing: 0.055em;
  text-transform: uppercase;
}

.game-pokemon .route-block,
.game-pokemon .party,
.game-pokemon .board-block {
  padding: 0.72rem 0.9rem;
  border: 1px solid rgba(101, 114, 172, 0.28);
  border-radius: 0.9rem;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.026), transparent 45%),
    rgba(9, 13, 29, 0.62);
  box-shadow: inset 0 1px rgba(255, 255, 255, 0.025);
}

.game-pokemon .route-block {
  border-top-color: rgba(242, 202, 82, 0.46);
}

.game-pokemon .party {
  border-top-color: rgba(239, 94, 87, 0.4);
}

.game-pokemon .block-head {
  margin-bottom: 0.5rem;
}

.game-pokemon .route-block .block-head h2,
.game-pokemon .party .block-head h2 {
  color: #f3efd9;
  font-family: ui-rounded, "Avenir Next", "Trebuchet MS", "Segoe UI", system-ui, sans-serif;
  font-size: 0.92rem;
  font-weight: 850;
  letter-spacing: 0.01em;
}

.game-pokemon .facts {
  gap: 0.55rem;
  border-top: 0;
}

.game-pokemon .facts div {
  min-width: 0;
  padding: 0.72rem 0.8rem;
  border: 1px solid rgba(101, 114, 172, 0.24);
  border-radius: 0.75rem;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.025), transparent),
    rgba(12, 16, 34, 0.66);
}

.game-pokemon .facts dt {
  color: #8f9bbb;
  font-size: 0.68rem;
  font-weight: 800;
  letter-spacing: 0.075em;
  text-transform: uppercase;
}

.game-pokemon .facts dd {
  margin-top: 0.18rem;
  color: #fffaf0;
  font-size: 1.35rem;
  font-stretch: normal;
  font-weight: 850;
}

.game-pokemon .roster {
  gap: 0.55rem;
  border-top: 0;
}

.game-pokemon .roster li {
  padding: 0.55rem 0.65rem;
  border: 1px solid rgba(101, 114, 172, 0.2);
  border-radius: 0.7rem;
  background: rgba(255, 255, 255, 0.023);
}

.game-pokemon .mon-name strong {
  color: #f8f3df;
  font-size: 0.92rem;
}

.game-pokemon .hp {
  height: 6px;
  overflow: hidden;
  border-radius: 999px;
  background: #252e4e;
}

.game-pokemon .hp i {
  border-radius: inherit;
}

.game-pokemon .side > section {
  padding: 0.9rem 1rem;
  border: 1px solid rgba(101, 114, 172, 0.25);
  border-radius: 0.9rem;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.024), transparent 48%),
    rgba(9, 13, 29, 0.64);
  box-shadow: inset 0 1px rgba(255, 255, 255, 0.025);
}

.game-pokemon .side > section > h2,
.game-pokemon .side > section .block-head h2 {
  color: #aeb9d7;
  font-size: 0.7rem;
  font-weight: 850;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.game-pokemon .side .now {
  border-top: 3px solid rgba(242, 202, 82, 0.78);
  background:
    radial-gradient(circle at 95% 0%, rgba(242, 202, 82, 0.08), transparent 9rem),
    rgba(12, 16, 34, 0.74);
}

.game-pokemon .now-place {
  margin-top: 0.2rem;
  color: #fffaf0;
  font-size: clamp(1.45rem, 3vw, 1.85rem);
  font-stretch: normal;
  font-weight: 850;
  letter-spacing: -0.025em;
}

.game-pokemon .goal {
  border-top: 3px solid rgba(239, 94, 87, 0.72);
}

.game-pokemon .goal-row span {
  color: #d4daf0;
  font-size: 0.9rem;
  font-weight: 750;
}

.game-pokemon .goal-row strong {
  color: #f2ca52;
  font-size: 1.65rem;
  font-stretch: normal;
}

.game-pokemon .goal-bar {
  height: 8px;
  overflow: hidden;
  border-radius: 999px;
  background: #242d4b;
  box-shadow: inset 0 1px 2px rgba(0, 0, 0, 0.35);
}

.game-pokemon .goal-bar i {
  border-radius: inherit;
  background: linear-gradient(90deg, #e8544f, #f2ca52);
  box-shadow: 0 0 12px rgba(242, 202, 82, 0.18);
}

.game-pokemon .say {
  max-width: none;
  margin-top: 0.2rem;
  color: #eef1ff;
  font-size: 0.96rem;
  font-weight: 650;
  line-height: 1.5;
}

.game-pokemon .tabs {
  gap: 0.7rem;
}

.game-pokemon .tabs button {
  padding: 0.2rem 0;
  font-size: 0.75rem;
  font-weight: 750;
}

.game-pokemon .log li {
  grid-template-columns: 3.2rem minmax(0, 1fr);
  padding: 0.55rem 0;
  border-top-color: rgba(101, 114, 172, 0.2);
  font-size: 0.82rem;
}

.game-pokemon .log li:first-child {
  border-top-color: rgba(242, 202, 82, 0.2);
}

.game-pokemon .log time {
  color: #7f8aaa;
  font-variant-numeric: tabular-nums;
}

.game-pokemon .others ul {
  border-top-color: rgba(101, 114, 172, 0.28);
}

.game-pokemon .others button {
  border-bottom-color: rgba(101, 114, 172, 0.2);
}

@media (max-width: 64rem) {
  .game-pokemon .stage-body {
    grid-template-columns: minmax(0, 1fr);
  }

  .game-pokemon .side {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .game-pokemon .side .activity,
  .game-pokemon .side .others {
    grid-column: 1 / -1;
  }
}

@media (max-width: 42rem) {
  .spectator-action {
    width: 2.25rem;
    height: 2.25rem;
    justify-content: center;
    padding: 0;
  }

  .spectator-action-label {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }

  .game-pokemon .stage {
    gap: 0.75rem;
    padding: 0;
    border: 0;
    border-radius: 0;
    background: none;
    box-shadow: none;
  }

  .game-pokemon .stage-head {
    align-items: flex-start;
    padding: 0.7rem 0.75rem 0.8rem;
    border-radius: 0.85rem;
  }

  .game-pokemon .stage-title h1 {
    font-size: 1.55rem;
  }

  .game-pokemon .stage-sub {
    width: 100%;
    border: 0;
    padding: 0;
    background: transparent;
  }

  .game-pokemon .stage-status {
    width: 100%;
  }

  .game-pokemon .side {
    grid-template-columns: minmax(0, 1fr);
  }

  .game-pokemon .side .activity,
  .game-pokemon .side .others {
    grid-column: auto;
  }

  .game-pokemon .facts {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .game-pokemon .facts dd {
    font-size: 1.15rem;
  }

  .game-pokemon .bezel {
    padding: 0.4rem 0.4rem 0.7rem;
    border-radius: 0.75rem 0.75rem 1.2rem 0.75rem;
  }

  .game-pokemon .screen-controls {
    top: 0.3rem;
    right: 0.3rem;
    gap: 0.25rem;
  }

  .game-pokemon .screen-controls .seg {
    font-size: 0.62rem;
  }

  .game-pokemon .screen-controls .seg button {
    padding: 0.25rem 0.4rem;
  }

  .game-pokemon .screen-controls .icon-btn {
    width: 1.75rem;
    height: 1.75rem;
  }

  .game-pokemon .bezel-label {
    gap: 0.5rem;
    font-size: 0.62rem;
  }

  .game-pokemon .bezel-label span {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .game-pokemon .route-block,
  .game-pokemon .party,
  .game-pokemon .side > section {
    padding: 0.75rem;
  }
}


/* Centered Pokémon stage: equal support rails keep the game screen on the visual axis. */
.game-pokemon .stage-body {
  align-items: start;
}

.game-pokemon .stat-rail {
  min-width: 0;
}

.game-pokemon .stat-rail .facts {
  grid-template-columns: minmax(0, 1fr);
  gap: 0.7rem;
}

.game-pokemon .stat-rail .facts div {
  display: flex;
  min-height: 5.2rem;
  flex-direction: column;
  justify-content: center;
  padding: 0.8rem 0.9rem;
}

.game-pokemon .stat-rail .facts div:nth-child(1) { border-left: 3px solid rgba(239, 94, 87, 0.9); }
.game-pokemon .stat-rail .facts div:nth-child(2) { border-left: 3px solid rgba(242, 202, 82, 0.9); }
.game-pokemon .stat-rail .facts div:nth-child(3) { border-left: 3px solid rgba(72, 179, 255, 0.9); }
.game-pokemon .stat-rail .facts div:nth-child(4) { border-left: 3px solid rgba(192, 132, 252, 0.9); }

.game-pokemon .screen-col {
  width: 100%;
  max-width: 48rem;
  grid-template-columns: minmax(0, 1fr);
  justify-self: center;
  gap: 1rem;
}

.game-pokemon .stage > .route-block,
.game-pokemon .stage > .party {
  width: 100%;
}

.game-pokemon .stage > .party .roster {
  grid-template-columns: repeat(6, minmax(0, 1fr));
}

.game-pokemon .stage > .party .roster li {
  align-items: center;
  min-width: 0;
  padding: 0.65rem 0.7rem;
}

.thinking-overlay {
  position: absolute;
  inset: 0;
  z-index: 6;
  display: grid;
  place-items: end center;
  padding: 1rem;
  pointer-events: none;
  background:
    linear-gradient(180deg, transparent 45%, rgba(5, 8, 19, 0.12) 72%, rgba(5, 8, 19, 0.48));
}

.thinking-overlay::before {
  position: absolute;
  inset: 0;
  background: linear-gradient(105deg, transparent 35%, rgba(242, 202, 82, 0.07) 48%, transparent 61%);
  content: "";
  transform: translateX(-70%);
  animation: thinking-scan 2.4s ease-in-out infinite;
}

.thinking-card {
  position: relative;
  display: flex;
  max-width: min(28rem, 92%);
  align-items: center;
  gap: 0.8rem;
  padding: 0.65rem 0.85rem;
  border: 1px solid rgba(242, 202, 82, 0.36);
  border-radius: 0.8rem;
  background: rgba(8, 11, 24, 0.84);
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.34), inset 0 1px rgba(255, 255, 255, 0.04);
  backdrop-filter: blur(8px);
}

.thinking-dots {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 0.24rem;
  width: 2.35rem;
  justify-content: center;
}

.thinking-dots i {
  width: 0.42rem;
  height: 0.42rem;
  border-radius: 999px;
  background: #f2ca52;
  box-shadow: 0 0 10px rgba(242, 202, 82, 0.32);
  animation: thinking-dot 1.15s ease-in-out infinite;
}

.thinking-dots i:nth-child(2) { animation-delay: 0.14s; }
.thinking-dots i:nth-child(3) { animation-delay: 0.28s; }

.thinking-copy {
  display: grid;
  min-width: 0;
  gap: 0.08rem;
}

.thinking-copy strong {
  color: #fff7dc;
  font-size: 0.8rem;
  font-weight: 850;
  letter-spacing: 0.025em;
}

.thinking-copy small {
  overflow: hidden;
  color: #aab5d2;
  font-size: 0.7rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@keyframes thinking-dot {
  0%, 60%, 100% { opacity: 0.4; transform: translateY(0) scale(0.9); }
  30% { opacity: 1; transform: translateY(-0.28rem) scale(1); }
}

@keyframes thinking-scan {
  0% { transform: translateX(-75%); }
  55%, 100% { transform: translateX(75%); }
}

.game-pokemon .rail-others {
  padding: 0.8rem 0.9rem;
  border: 1px solid rgba(101, 114, 172, 0.25);
  border-radius: 0.8rem;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.022), transparent 50%),
    rgba(9, 13, 29, 0.62);
}

.game-pokemon .rail-others .block-head {
  margin-bottom: 0.35rem;
}

.game-pokemon .rail-others .block-head h2 {
  color: #aeb9d7;
  font-size: 0.68rem;
  font-weight: 850;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.game-pokemon .rail-others button {
  padding: 0.5rem 0;
}

.game-pokemon .rail-others button strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.game-pokemon .next-goal {
  display: grid;
  gap: 0.12rem;
  margin-top: 0.65rem;
  padding-top: 0.55rem;
  border-top: 1px solid rgba(101, 114, 172, 0.22);
}

.game-pokemon .next-goal span {
  color: #7f8aaa;
  font-size: 0.62rem;
  font-weight: 850;
  letter-spacing: 0.09em;
  text-transform: uppercase;
}

.game-pokemon .next-goal strong {
  color: #f5e9b6;
  font-size: 0.78rem;
  line-height: 1.35;
}

.game-pokemon .activity .log {
  line-height: 1.42;
}

.game-pokemon .activity .log li {
  grid-template-columns: 2.7rem minmax(0, 1fr);
}

.game-pokemon .activity .tabs {
  gap: 0.55rem;
}

@media (min-width: 72rem) {
  .game-pokemon .stage-body {
    width: 100%;
    grid-template-columns: minmax(15rem, 20rem) minmax(34rem, 52rem) minmax(21rem, 26rem);
    justify-content: center;
    gap: 0.9rem;
  }

  .game-pokemon .screen-col {
    max-width: 52rem;
  }

  /*
   * Keep the context rail tied to the gameplay viewport, but give Activity
   * enough horizontal room to read like a feed rather than a narrow log.
   */
  .game-pokemon .side {
    height: min(38rem, calc(100vh - 11rem));
    min-height: 31rem;
    grid-template-rows: max-content max-content max-content minmax(0, 1fr);
    overflow: hidden;
  }

  .game-pokemon .side .activity {
    display: grid;
    min-height: 0;
    grid-template-rows: max-content minmax(0, 1fr);
    overflow: hidden;
  }

  .game-pokemon .side .activity .log {
    min-height: 0;
    overflow-y: auto;
    overscroll-behavior: contain;
    padding-right: 0.3rem;
    scrollbar-width: thin;
    scrollbar-color: rgba(242, 202, 82, 0.28) transparent;
  }

  .game-pokemon .stat-rail {
    display: grid;
    align-content: start;
    gap: 0.8rem;
  }

  .game-pokemon .rail-others {
    min-height: 0;
    max-height: 12rem;
    overflow: hidden;
  }

  .game-pokemon .rail-others ul {
    max-height: 8.5rem;
    overflow-y: auto;
    overscroll-behavior: contain;
    padding-right: 0.2rem;
    scrollbar-width: thin;
    scrollbar-color: rgba(148, 163, 184, 0.24) transparent;
  }
}

@media (max-width: 90rem) {
  .game-pokemon .stage > .party .roster {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 72rem) {
  .game-pokemon .stage-body {
    grid-template-columns: minmax(0, 1fr);
  }

  .game-pokemon .side {
    height: auto;
    min-height: 0;
    overflow: visible;
  }

  .game-pokemon .side .activity,
  .game-pokemon .side .others,
  .game-pokemon .side .activity .log,
  .game-pokemon .side .others ul {
    max-height: none;
    overflow: visible;
  }

  .game-pokemon .rail-others,
  .game-pokemon .rail-others ul {
    max-height: none;
    overflow: visible;
  }

  .game-pokemon .screen-col {
    order: 1;
    max-width: 52rem;
  }

  .game-pokemon .stat-rail {
    order: 2;
  }

  .game-pokemon .stat-rail .facts {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }

  .game-pokemon .side {
    order: 3;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .game-pokemon .side .activity,
  .game-pokemon .side .others {
    grid-column: 1 / -1;
  }
}

@media (max-width: 42rem) {
  .game-pokemon .stage,
  .game-pokemon .stage-body,
  .game-pokemon .screen-col,
  .game-pokemon .stat-rail,
  .game-pokemon .side,
  .game-pokemon .route-block,
  .game-pokemon .party {
    width: 100%;
    max-width: 100%;
    min-width: 0;
  }

  .game-pokemon .screen {
    max-width: 100%;
  }

  .game-pokemon .now-place,
  .game-pokemon .goal-row,
  .game-pokemon .goal-row strong,
  .game-pokemon .say,
  .game-pokemon .next-goal strong,
  .game-pokemon .ruled dd,
  .game-pokemon .others strong,
  .game-pokemon .others span {
    min-width: 0;
    max-width: 100%;
    overflow-wrap: anywhere;
  }

  .game-pokemon .goal-row {
    align-items: flex-start;
  }

  .game-pokemon .goal-row strong {
    font-size: 1.45rem;
    line-height: 1.2;
  }

  .game-pokemon .stat-rail .facts,
  .game-pokemon .stage > .party .roster {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .game-pokemon .stat-rail .facts div {
    min-height: 4rem;
    padding: 0.6rem 0.7rem;
  }

  .game-pokemon .rail-others {
    display: none;
  }

  .game-pokemon .mobile-other-runs {
    display: block;
  }

  .game-pokemon .screen-col,
  .game-pokemon .side {
    gap: 0.75rem;
  }

  .game-pokemon .side {
    grid-template-columns: minmax(0, 1fr);
  }

  .game-pokemon .side .activity,
  .game-pokemon .side .others {
    grid-column: auto;
  }

  .thinking-overlay {
    padding: 0.6rem;
  }

  .thinking-card {
    max-width: 100%;
    padding: 0.55rem 0.65rem;
  }

  .thinking-copy small {
    white-space: normal;
  }
}

@media (prefers-reduced-motion: reduce) {
  .thinking-overlay::before,
  .thinking-dots i {
    animation: none;
  }

  .thinking-dots i {
    opacity: 0.85;
  }
}
</style>
