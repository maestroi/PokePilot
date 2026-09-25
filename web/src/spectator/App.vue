<script setup lang="ts">
import { computed, ref, watch, type Component } from 'vue'
import {
  ArrowPathIcon,
  ArrowsPointingOutIcon,
  BanknotesIcon,
  BookOpenIcon,
  CheckCircleIcon,
  ClockIcon,
  FlagIcon,
  LinkIcon,
  MapIcon,
  MapPinIcon,
  PlayIcon,
  SignalIcon,
  SparklesIcon,
  TrophyIcon,
  UserGroupIcon
} from '@heroicons/vue/20/solid'
import { getSpectatorSnapshot } from '../shared/api/spectator-client'
import type { SpectatorRun } from '../shared/api/spectator'
import AppShell from '../shared/components/AppShell.vue'
import BadgeIcon from '../shared/components/BadgeIcon.vue'
import PokemonPartyCard from '../shared/components/PokemonPartyCard.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import ModernSceneRenderer from '../shared/components/ModernSceneRenderer.vue'
import { useFramePump } from '../shared/composables/useFramePump'
import { useRenderStatePump } from '../shared/composables/useRenderStatePump'
import { usePollingResource } from '../shared/composables/usePollingResource'
import {
  goalProgress,
  isLiveRun,
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
import PublicHome from './PublicHome.vue'
import { MAP_CATALOG, mapEntry } from '../shared/mapCatalog'
import { runIDFromLocation, spectatorRunPath } from '../shared/urls'
import { elapsedRunSeconds, formatDuration } from '../shared/runTiming'
import { canRenderModernScene } from '../shared/semanticRenderer'
import { DEFAULT_RENDER_THEME_ID, renderThemeOptions, resolveRenderTheme } from '../shared/renderTheme'
import spectatorNightscapeUrl from './assets/spectator-nightscape.svg'
import spectatorLeagueBannerUrl from './assets/spectator-league-banner.svg'

type ActivityKind = 'decision' | 'area' | 'badge' | 'party' | 'dex' | 'milestone' | 'state'
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
const rendererMode = ref<RendererMode>(window.localStorage.getItem('pokepilot.spectator.renderer') === 'classic' ? 'classic' : 'modern')
const themeOptions = renderThemeOptions()
const storedThemeID = window.localStorage.getItem('pokepilot.spectator.theme') || DEFAULT_RENDER_THEME_ID
const initialThemeSelection = resolveRenderTheme(storedThemeID)
const selectedThemeID = ref(initialThemeSelection.theme.id)
const themeNotice = ref(initialThemeSelection.diagnostics.join(' '))
const activeTheme = computed(() => resolveRenderTheme(selectedThemeID.value).theme)
const playerRef = ref<HTMLElement | null>(null)
const activityFilter = ref<ActivityFilter>('all')
const activityFilters: ActivityFilter[] = ['all', 'milestones', 'decisions']
const activityByRun = ref<Record<string, ActivityItem[]>>({})
const previousRuns = new Map<string, SpectatorRun>()
const GYM_BADGES = ['Boulder', 'Cascade', 'Thunder', 'Rainbow', 'Soul', 'Marsh', 'Volcano', 'Earth'] as const

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

const runs = computed(() => snapshot.value?.runs ?? [])
const groupedRuns = computed(() => splitSpectatorRuns(runs.value))
const selectedRun = computed(() => preferredRun(
  groupedRuns.value.live,
  selectionPinned.value ? selectedRunID.value : ''
))
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
const showModern = computed(() => selectionPinned.value && rendererMode.value === 'modern' && semanticReady.value)
const frameEnabled = computed(() =>
  Boolean(frameRunID.value) && (!selectionPinned.value || rendererMode.value === 'classic' || !semanticReady.value)
)
const { frameURL, state: frameState, error: frameError } = useFramePump(frameRunID, frameEnabled, 50, frameContinuous)
const modernFallbackLabel = computed(() => {
  if (rendererMode.value !== 'modern' || showModern.value) return ''
  if (renderStateStatus.value === 'error') return 'Modern · semantic state reconnecting'
  if (renderState.value?.scene) return 'Modern · classic compatibility · ' + renderState.value.scene
  return ''
})
const modeClass = computed(() => `mode-${normalizePlayStyle(selectedRun.value)}`)
const selectedActivity = computed(() => {
  const run = selectedRun.value
  const activity = run ? activityByRun.value[run.run_id] || [] : []
  if (activityFilter.value === 'milestones') {
    return activity.filter((item) => ['area', 'badge', 'party', 'dex', 'milestone'].includes(item.kind))
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
  const visited = Number(selectedRun.value?.maps_visited || 0)
  return visited > 0 ? `${visited}/${MAP_CATALOG.length}` : `0/${MAP_CATALOG.length}`
})
const lastRefreshLabel = computed(() => {
  if (!lastUpdatedAt.value) return ''
  return new Date(lastUpdatedAt.value).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
})

const currentLocation = computed(() => {
  const run = selectedRun.value
  if (!run) return 'Waiting for location'
  return mapEntry(Number(run.map || 0))?.label || locationLabel(run)
})
const runtimeLabel = computed(() => {
  const run = selectedRun.value
  if (!run) return '—'
  const seconds = elapsedRunSeconds(run)
  return seconds > 0 ? formatDuration(seconds) : 'Just started'
})
const sceneStyle = computed(() => ({
  '--spectator-art': `url("${spectatorNightscapeUrl}")`
}))

const goalPercent = computed(() => {
  const run = selectedRun.value
  return run ? goalProgress(run) : 0
})

const leagueGoal = computed(() => {
  const run = selectedRun.value
  if (!run) return false
  const text = [run.goal, run.stats?.goal_summary, objectiveLabel(run)].filter(Boolean).join(' ').toLowerCase()
  return normalizePlayStyle(run) === 'speedrun' || /elite four|champion|hall of fame/.test(text)
})

const goalProgressCopy = computed(() => {
  const run = selectedRun.value
  if (!run) return { label: 'Waiting for goal', detail: 'Run objective is loading.' }

  const badges = run.player?.badges?.length || 0
  const stats = run.stats
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
    const badgeName = GYM_BADGES[badges] || 'Next'
    return {
      label: `${badges}/8 badges earned`,
      detail: `Next: ${badgeName} Badge`
    }
  }

  const current = Number(stats?.goal_current || 0)
  const target = Number(stats?.goal_target || 0)
  return {
    label: target > 0 ? `${current}/${target} goal progress` : 'Goal in progress',
    detail: stats?.goal_summary || objectiveLabel(run)
  }
})

type StatTone = 'cyan' | 'violet' | 'emerald' | 'amber'

interface PremiumStat {
  key: string
  label: string
  value: string
  hint: string
  icon: Component
  tone: StatTone
}

const statCards = computed<PremiumStat[]>(() => {
  const run = selectedRun.value
  if (!run) return []
  return [
    { key: 'maps', label: 'Maps visited', value: mapsLabel.value, hint: 'World', icon: MapIcon, tone: 'cyan' },
    { key: 'party', label: 'Party', value: `${run.player?.party?.length || 0}/6`, hint: 'Team', icon: UserGroupIcon, tone: 'violet' },
    { key: 'runtime', label: 'Runtime', value: runtimeLabel.value, hint: 'Live', icon: ClockIcon, tone: 'emerald' },
    { key: 'money', label: 'Money', value: moneyLabel(run), hint: 'Funds', icon: BanknotesIcon, tone: 'amber' }
  ]
})

type StretchState = 'done' | 'active' | 'todo'

interface StretchStep {
  key: string
  title: string
  detail: string
  value?: string
  state: StretchState
  icon: Component
}

const finalStretchSteps = computed<StretchStep[]>(() => {
  const run = selectedRun.value
  if (!run) return []

  const badges = run.player?.badges?.length || 0
  const location = currentLocation.value.toLowerCase()
  const milestones = (run.player?.milestones || []).map((value) => value.toLowerCase())
  const hasMilestone = (...needles: string[]) => milestones.some((value) => needles.some((needle) => value.includes(needle)))

  const hallOfFameDone = Boolean(run.stats?.goal_complete) || hasMilestone('hall of fame', 'main story complete')
  const championDone = hallOfFameDone || hasMilestone('champion defeated', 'league champion', 'champion')
  const eliteFourDone = championDone || hasMilestone('elite four')
  const plateauReached = eliteFourDone || championDone || hallOfFameDone || location.includes('indigo plateau')
  const victoryRoadActive = location.includes('victory road')

  return [
    {
      key: 'badges',
      title: 'Gym Badges',
      detail: badges >= 8 ? 'All eight badges are earned.' : `${8 - badges} badge${8 - badges === 1 ? '' : 's'} remain.`,
      value: `${badges}/8`,
      state: badges >= 8 ? 'done' : 'active',
      icon: CheckCircleIcon
    },
    {
      key: 'victory-road',
      title: 'Victory Road',
      detail: plateauReached ? 'Route to Indigo Plateau cleared.' : 'Navigate the final cave and reach Indigo Plateau.',
      state: plateauReached ? 'done' : badges >= 8 || victoryRoadActive ? 'active' : 'todo',
      icon: MapPinIcon
    },
    {
      key: 'elite-four',
      title: 'Elite Four',
      detail: 'Defeat all four members in sequence.',
      state: eliteFourDone ? 'done' : plateauReached ? 'active' : 'todo',
      icon: FlagIcon
    },
    {
      key: 'champion',
      title: 'Champion',
      detail: 'Win the final Champion battle.',
      state: championDone ? 'done' : eliteFourDone ? 'active' : 'todo',
      icon: TrophyIcon
    },
    {
      key: 'hall-of-fame',
      title: 'Hall of Fame',
      detail: 'The full speedrun objective finishes here.',
      state: hallOfFameDone ? 'done' : championDone ? 'active' : 'todo',
      icon: SparklesIcon
    }
  ]
})

function statToneClass(tone: StatTone): string {
  return `stat-tone-${tone}`
}

function stretchToneClass(state: StretchState): string {
  return `stretch-step-${state}`
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
        mapEntry(Number(run.map || 0))?.label || locationLabel(run)
      )
      if (run.decision) pushActivity(run.run_id, 'decision', 'Current decision', run.decision)
      else if (run.planner_waiting) pushActivity(run.run_id, 'state', 'Planner thinking', 'Choosing the first objective')
      else if (run.stop_so_far) pushActivity(run.run_id, 'state', 'Run state', run.stop_so_far)
      previousRuns.set(run.run_id, run)
      continue
    }

    if (run.planner_waiting && previous.decision) {
      pushActivity(run.run_id, 'state', 'Planner thinking', 'Choosing the next objective')
    }
    if (run.decision && run.decision !== previous.decision) {
      pushActivity(run.run_id, 'decision', 'Decision', run.decision)
    }
    if (run.map !== previous.map) {
      pushActivity(run.run_id, 'area', 'Entered area', mapEntry(Number(run.map || 0))?.label || locationLabel(run))
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

function activityIcon(kind: ActivityKind) {
  switch (kind) {
    case 'decision': return SparklesIcon
    case 'area': return MapPinIcon
    case 'badge': return TrophyIcon
    case 'party': return UserGroupIcon
    case 'dex': return BookOpenIcon
    case 'milestone': return FlagIcon
    default: return SignalIcon
  }
}

function activityTone(kind: ActivityKind): string {
  switch (kind) {
    case 'decision': return 'text-cyan-300 bg-cyan-300/10 ring-cyan-300/20'
    case 'area': return 'text-blue-300 bg-blue-300/10 ring-blue-300/20'
    case 'badge': return 'text-amber-300 bg-amber-300/10 ring-amber-300/20'
    case 'party': return 'text-violet-300 bg-violet-300/10 ring-violet-300/20'
    case 'dex': return 'text-rose-300 bg-rose-300/10 ring-rose-300/20'
    case 'milestone': return 'text-emerald-300 bg-emerald-300/10 ring-emerald-300/20'
    default: return 'text-slate-300 bg-white/5 ring-white/10'
  }
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
  const resolved = resolveRenderTheme(themeID)
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
    :show-intro="false"
  >
    <template #summary>
      <template v-if="snapshot">
        <span><strong class="text-white">{{ snapshot.summary.live }}</strong> live run{{ snapshot.summary.live === 1 ? '' : 's' }}</span>
      </template>
    </template>

    <template #actions>
      <button
        v-if="groupedRuns.live[0] && selectedRun?.run_id !== groupedRuns.live[0].run_id"
        type="button"
        class="inline-flex items-center gap-1.5 rounded-md bg-cyan-300 px-2.5 py-1.5 text-xs font-bold text-[#101820] hover:brightness-110"
        @click="selectRun(groupedRuns.live[0])"
      >
        <PlayIcon class="size-3.5" aria-hidden="true" />
        Watch live
      </button>
      <button
        v-else-if="groupedRuns.live[0]"
        type="button"
        class="inline-flex items-center gap-1.5 rounded-md bg-cyan-300/15 px-2.5 py-1.5 text-xs font-bold text-cyan-100 ring-1 ring-cyan-300/20 hover:bg-cyan-300/25"
        @click="selectRun(groupedRuns.live[0])"
      >
        <PlayIcon class="size-3.5" aria-hidden="true" />
        Watch live
      </button>
      <button
        v-if="selectedRun && selectionPinned"
        type="button"
        class="inline-flex items-center gap-1.5 rounded-md bg-white/8 px-2.5 py-1.5 text-xs font-semibold text-slate-200 ring-1 ring-white/10 hover:bg-white/12"
        @click="copyLink"
      >
        <LinkIcon class="size-3.5" aria-hidden="true" />
        {{ copyState || 'Share' }}
      </button>
    </template>

    <div v-if="!snapshot && state === 'loading'" class="mx-auto max-w-7xl space-y-3 py-3" aria-label="Loading spectator feed" aria-busy="true">
      <div class="h-28 animate-pulse rounded-xl bg-white/5 ring-1 ring-white/8" />
      <div class="grid gap-3 xl:grid-cols-[minmax(0,2.2fr)_minmax(19rem,0.8fr)]">
        <div class="h-[36rem] animate-pulse rounded-xl bg-white/5 ring-1 ring-white/8" />
        <div class="space-y-3">
          <div class="h-44 animate-pulse rounded-xl bg-white/5 ring-1 ring-white/8" />
          <div class="h-60 animate-pulse rounded-xl bg-white/5 ring-1 ring-white/8" />
        </div>
      </div>
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
      </div>
    </div>

    <div v-else-if="selectedRun" :class="['spectator-theme mx-auto max-w-[112rem] space-y-3', modeClass]" :style="sceneStyle">
      <div v-if="state === 'stale'" class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-amber-300/20 bg-amber-300/8 px-3 py-2 text-xs text-amber-100" role="status">
        <span><strong>Connection lost.</strong> Showing the last known run state while reconnecting automatically.</span>
        <span class="flex items-center gap-2 text-amber-200/70">
          <span v-if="lastRefreshLabel">Last update {{ lastRefreshLabel }}</span>
          <button type="button" class="font-semibold text-amber-100 hover:text-white" @click="refresh">Retry now</button>
        </span>
      </div>

      <PublicHome
        v-if="!selectionPinned && snapshot"
        :run="selectedRun"
        :live-runs="groupedRuns.live"
        :summary="snapshot.summary"
        :frame-url="frameURL"
        @select="selectRun"
      />

      <div v-if="selectionPinned" class="spectator-stage grid gap-3 xl:grid-cols-[18rem_minmax(0,1fr)_21rem]">
        <aside class="space-y-3">
          <section class="spectator-card spectator-hero-card overflow-hidden rounded-2xl border p-4 shadow-xl shadow-black/20">
            <div class="flex flex-wrap items-center gap-2">
              <span v-if="isLiveRun(selectedRun)" class="inline-flex items-center gap-1.5 rounded-full bg-red-500/12 px-2.5 py-1 text-[10px] font-black tracking-[0.11em] text-red-300 uppercase ring-1 ring-red-400/20">
                <span class="size-1.5 animate-pulse rounded-full bg-red-400" />
                Live run
              </span>
              <StatusBadge v-else :tone="runTone(selectedRun)">{{ runStatusLabel(selectedRun) }}</StatusBadge>
              <span class="mode-chip">{{ playStyleLabel(selectedRun) }}</span>
              <span v-if="selectedRun.purpose === 'debug_coverage'" class="mode-chip">Debug coverage</span>
            </div>

            <div class="mt-4 flex items-start gap-3">
              <div class="game-mark grid size-11 shrink-0 place-items-center rounded-xl ring-1 ring-white/10" aria-hidden="true">
                <span class="pokeball-mark" />
              </div>
              <div class="min-w-0">
                <h1 class="text-2xl font-black tracking-tight text-white">Pokémon Red</h1>
                <p class="mt-0.5 truncate text-sm text-slate-400">{{ playStyleLabel(selectedRun) }} · {{ selectedRun.player?.party?.[0]?.name || selectedRun.starter || 'new trainer' }}</p>
                <p class="mt-1 flex items-center gap-1.5 truncate text-[11px] text-slate-500">
                  <MapPinIcon class="size-3 shrink-0 text-cyan-200/60" aria-hidden="true" />
                  {{ currentLocation }}
                </p>
              </div>
            </div>

            <div class="spectator-art-banner mt-4 overflow-hidden rounded-xl border" aria-hidden="true">
              <img :src="spectatorLeagueBannerUrl" alt="" class="h-full w-full object-cover" />
              <div class="spectator-art-banner-glow" />
              <div class="spectator-art-banner-caption">
                <span>Road to the League</span>
                <strong>{{ currentLocation }}</strong>
              </div>
            </div>

            <div class="mt-5">
              <p class="text-sm leading-6 text-slate-300">{{ objectiveLabel(selectedRun) }}</p>
            </div>

            <div class="goal-progress-card mt-4 rounded-xl border p-3">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="flex items-center gap-1.5 text-[9px] font-black tracking-[0.11em] text-[var(--mode-accent)] uppercase">
                    <FlagIcon class="size-3.5" aria-hidden="true" />
                    Goal progress
                  </div>
                  <div class="mt-1 truncate text-[11px] text-slate-400">{{ goalProgressCopy.detail }}</div>
                </div>
                <div class="shrink-0 text-right">
                  <strong class="block font-mono text-lg text-white">{{ goalPercent.toFixed(0) }}%</strong>
                  <span class="text-[9px] text-slate-500">{{ goalProgressCopy.label }}</span>
                </div>
              </div>
              <div class="mt-3 h-2 overflow-hidden rounded-full bg-white/8 ring-1 ring-white/5">
                <div class="mode-progress goal-progress-fill h-full rounded-full transition-[width]" :style="{ width: goalPercent + '%' }" />
              </div>
            </div>

            <div class="mt-5">
              <div class="mb-2 flex items-center justify-between gap-2">
                <div class="text-[9px] font-black tracking-[0.11em] text-slate-500 uppercase">Gym badges</div>
                <span class="font-mono text-[10px] text-slate-500">{{ selectedRun.player?.badges?.length || 0 }}/8</span>
              </div>
              <div class="grid grid-cols-8 gap-1.5">
                <div
                  v-for="slot in 8"
                  :key="slot"
                  :class="[
                    selectedRun.player?.badges?.[slot - 1] ? 'badge-earned' : 'badge-empty',
                    'grid aspect-square place-items-center rounded-full'
                  ]"
                  :title="selectedRun.player?.badges?.[slot - 1] || 'Badge not earned yet'"
                >
                  <BadgeIcon
                    v-if="selectedRun.player?.badges?.[slot - 1]"
                    :name="selectedRun.player.badges[slot - 1]"
                    :size="26"
                  />
                </div>
              </div>
            </div>

            <div class="mt-5 grid grid-cols-2 gap-2">
              <div
                v-for="stat in statCards"
                :key="stat.key"
                :class="['premium-stat rounded-xl border p-3', statToneClass(stat.tone)]"
              >
                <div class="flex items-start justify-between gap-2">
                  <span :class="['premium-stat-icon', statToneClass(stat.tone)]">
                    <component :is="stat.icon" class="size-4" aria-hidden="true" />
                  </span>
                  <span class="text-[8px] font-bold tracking-[0.08em] text-slate-700 uppercase">{{ stat.hint }}</span>
                </div>
                <strong class="mt-2 block font-mono text-base text-white">{{ stat.value }}</strong>
                <span class="text-[10px] text-slate-500">{{ stat.label }}</span>
              </div>
            </div>
          </section>

          <section v-if="otherLiveRuns.length" class="spectator-card rounded-2xl border p-3">
            <div class="mb-2 flex items-center justify-between gap-2">
              <div>
                <h2 class="text-xs font-bold text-white">Other live runs</h2>
                <p class="mt-0.5 text-[10px] text-slate-600">Switch streams instantly.</p>
              </div>
              <span class="rounded-full bg-emerald-300/10 px-2 py-1 font-mono text-[9px] text-emerald-200 ring-1 ring-emerald-300/20">{{ groupedRuns.live.length }} live</span>
            </div>
            <div class="space-y-1.5">
              <button
                v-for="run in otherLiveRuns"
                :key="run.run_id"
                type="button"
                class="w-full rounded-lg bg-black/15 px-2.5 py-2 text-left ring-1 ring-white/8 transition hover:bg-white/6 hover:ring-white/15"
                @click="selectRun(run)"
              >
                <div class="flex items-center justify-between gap-2">
                  <strong class="truncate text-[11px] text-slate-300">{{ runTitle(run) }}</strong>
                  <span class="size-1.5 shrink-0 rounded-full bg-emerald-300" />
                </div>
                <div class="mt-1 truncate text-[9px] text-slate-600">{{ mapEntry(Number(run.map || 0))?.label || locationLabel(run) }}</div>
              </button>
            </div>
          </section>
        </aside>

        <main class="min-w-0 space-y-3">
          <section class="player-card overflow-hidden rounded-2xl border shadow-2xl shadow-black/30">
            <div
              ref="playerRef"
              :class="[
                'player-shell group relative overflow-hidden bg-black',
                theaterMode ? 'min-h-[78vh]' : 'min-h-[34rem] sm:min-h-[42rem] xl:min-h-[46rem]'
              ]"
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

              <div v-else class="absolute inset-0 grid place-items-center px-6 py-12 text-center">
                <div>
                  <div class="mx-auto flex size-14 items-center justify-center rounded-full border border-white/10 bg-white/5">
                    <span :class="['size-3 rounded-full', isLiveRun(selectedRun) ? 'animate-pulse bg-emerald-300' : 'bg-slate-600']" />
                  </div>
                  <p class="mt-3 text-sm font-semibold text-slate-300">Waiting for the live stream</p>
                  <p v-if="rendererMode === 'modern' && renderStateStatus === 'error' && renderStateError" class="mt-1 text-xs text-amber-300/80">{{ renderStateError }}</p>
                  <p v-else-if="frameState === 'error' && frameError" class="mt-1 text-xs text-amber-300/80">{{ frameError }}</p>
                </div>
              </div>

              <div class="pointer-events-none absolute inset-x-0 top-0 flex items-start justify-between gap-3 bg-gradient-to-b from-black/75 via-black/30 to-transparent px-3 py-3 sm:px-4">
                <div class="flex items-center gap-2">
                  <span class="inline-flex items-center gap-1.5 rounded-full bg-black/55 px-2.5 py-1 text-[10px] font-black tracking-[0.1em] text-white uppercase ring-1 ring-white/12">
                    <span class="size-1.5 animate-pulse rounded-full bg-red-400" />
                    Live
                  </span>
                  <span class="rounded-full bg-black/55 px-2.5 py-1 font-mono text-[10px] text-slate-200 ring-1 ring-white/12">{{ playSpeedLabel(selectedRun) }}</span>
                </div>
                <span class="rounded-full bg-black/55 px-2.5 py-1 text-[10px] font-bold text-slate-200 ring-1 ring-white/12">{{ currentLocation }}</span>
              </div>

              <div class="absolute right-3 top-14 z-10 flex flex-col items-end gap-2 opacity-85 transition-opacity group-hover:opacity-100 sm:right-4">
                <div class="flex overflow-hidden rounded-lg bg-black/65 text-[9px] font-black uppercase tracking-[0.08em] ring-1 ring-white/15 backdrop-blur-md">
                  <button
                    type="button"
                    :class="[rendererMode === 'modern' ? 'bg-cyan-300/20 text-cyan-100' : 'text-slate-400 hover:text-white', 'px-2.5 py-1.5 transition-colors']"
                    title="Render the live semantic world"
                    @click="setRendererMode('modern')"
                  >Modern</button>
                  <button
                    type="button"
                    :class="[rendererMode === 'classic' ? 'bg-white/15 text-white' : 'text-slate-400 hover:text-white', 'px-2.5 py-1.5 transition-colors']"
                    title="Show the classic emulator framebuffer"
                    @click="setRendererMode('classic')"
                  >Classic</button>
                </div>
                <label v-if="rendererMode === 'modern'" class="flex items-center gap-2 rounded-lg bg-black/65 px-2 py-1.5 text-[9px] text-slate-400 ring-1 ring-white/12 backdrop-blur-md">
                  <span class="font-black uppercase tracking-[0.08em]">Theme</span>
                  <select
                    :value="selectedThemeID"
                    class="max-w-36 bg-transparent text-[10px] font-semibold text-white outline-none"
                    title="Choose your spectator theme"
                    @change="onThemeSelect"
                  >
                    <option
                      v-for="theme in themeOptions"
                      :key="theme.id"
                      :value="theme.id"
                      class="bg-slate-950 text-white"
                    >{{ theme.name }}</option>
                  </select>
                </label>
                <div
                  v-if="themeNotice"
                  class="max-w-56 rounded-lg bg-amber-950/80 px-2.5 py-1.5 text-right text-[9px] font-semibold text-amber-200 ring-1 ring-amber-300/20"
                >{{ themeNotice }}</div>
                <button type="button" class="player-control" :title="theaterMode ? 'Exit theater mode' : 'Theater mode'" @click="theaterMode = !theaterMode">
                  <PlayIcon class="size-4" aria-hidden="true" />
                </button>
                <button type="button" class="player-control" title="Fullscreen" @click="fullscreenPlayer">
                  <ArrowsPointingOutIcon class="size-4" aria-hidden="true" />
                </button>
              </div>

              <div class="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/90 via-black/55 to-transparent px-3 pb-3 pt-20 sm:px-4 sm:pb-4">
                <div v-if="plannerState" class="inline-flex max-w-[90%] items-center gap-3 rounded-xl bg-black/60 px-3 py-2.5 ring-1 ring-cyan-300/20 backdrop-blur-md">
                  <div class="planner-spinner grid size-8 shrink-0 place-items-center rounded-full border border-cyan-300/25 bg-cyan-300/10">
                    <SparklesIcon class="size-4 text-cyan-200" aria-hidden="true" />
                  </div>
                  <div class="min-w-0">
                    <div class="text-[9px] font-black tracking-[0.11em] text-cyan-200 uppercase">{{ plannerState.title }}</div>
                    <div class="mt-0.5 flex items-center gap-2 text-xs text-white sm:text-sm">
                      <span class="truncate">{{ plannerState.detail }}</span>
                      <span class="planner-dots inline-flex shrink-0 gap-1" aria-hidden="true"><span /><span /><span /></span>
                    </div>
                  </div>
                </div>
                <div v-else class="inline-block max-w-[92%] rounded-xl bg-black/60 px-3 py-2.5 ring-1 ring-white/12 backdrop-blur-md">
                  <div class="text-[9px] font-black tracking-[0.1em] text-slate-400 uppercase">Latest decision</div>
                  <div class="mt-1 line-clamp-2 text-xs font-semibold leading-5 text-white sm:text-sm">{{ selectedRun.decision || 'Preparing the next objective' }}</div>
                </div>
              </div>

              <div v-if="modernFallbackLabel && frameURL" class="absolute left-3 top-14 rounded-full bg-black/70 px-2.5 py-1 text-[10px] font-semibold text-cyan-100 ring-1 ring-cyan-300/20">
                {{ modernFallbackLabel }}
              </div>
              <div v-else-if="!showModern && frameURL && frameState === 'error'" class="absolute left-3 top-14 rounded-full bg-amber-950/80 px-2.5 py-1 text-[10px] font-semibold text-amber-200 ring-1 ring-amber-300/20">
                Last frame · reconnecting
              </div>
            </div>
          </section>

          <section class="spectator-card rounded-2xl border p-3 sm:p-4">
            <div class="mb-3 flex items-end justify-between gap-3">
              <div>
                <h2 class="text-sm font-black text-white">Current Party</h2>
                <p class="mt-0.5 text-[10px] text-slate-600">Live health and levels.</p>
              </div>
              <span class="font-mono text-[10px] text-slate-500">{{ selectedRun.player?.party?.length || 0 }} / 6 slots</span>
            </div>
            <div v-if="selectedRun.player?.party?.length" class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              <PokemonPartyCard
                v-for="(mon, index) in selectedRun.player.party"
                :key="mon.name + '-' + index"
                :name="mon.name"
                :level="mon.level"
                :hp="mon.hp"
                :max-hp="mon.max_hp"
                :status="mon.status"
                :lead="index === 0"
              />
            </div>
            <p v-else class="py-5 text-center text-sm text-slate-500">Party data is not available yet.</p>
          </section>
        </main>

        <aside class="space-y-3">
          <section class="spectator-card rounded-2xl border p-4">
            <div class="flex items-center justify-between gap-3">
              <div class="flex items-center gap-2">
                <span class="size-2 animate-pulse rounded-full bg-red-400" />
                <div>
                  <h2 class="text-sm font-black text-white">Live Activity</h2>
                  <p class="mt-0.5 text-[10px] text-slate-600">What the run is doing right now.</p>
                </div>
              </div>
              <div class="flex rounded-lg bg-black/20 p-0.5 ring-1 ring-white/8">
                <button
                  v-for="filter in activityFilters"
                  :key="filter"
                  type="button"
                  :title="'Show ' + filter + ' activity'"
                  :class="[
                    activityFilter === filter ? 'bg-white/10 text-white' : 'text-slate-600 hover:text-slate-300',
                    'rounded-md px-2 py-1 text-[9px] font-bold capitalize transition-colors'
                  ]"
                  @click="activityFilter = filter"
                >
                  {{ filter === 'milestones' ? 'Progress' : filter }}
                </button>
              </div>
            </div>

            <div v-if="selectedActivity.length" class="activity-list relative mt-4 space-y-1">
              <div v-for="item in selectedActivity.slice(0, 9)" :key="item.id" class="activity-item relative grid grid-cols-[2rem_minmax(0,1fr)] gap-2.5 py-2">
                <span :class="[activityTone(item.kind), 'relative z-10 grid size-7 place-items-center rounded-lg ring-1']" :title="item.label">
                  <component :is="activityIcon(item.kind)" class="size-3.5" aria-hidden="true" />
                </span>
                <div class="min-w-0">
                  <div class="flex items-baseline justify-between gap-2">
                    <strong class="truncate text-[11px] text-slate-300">{{ item.label }}</strong>
                    <time class="shrink-0 font-mono text-[9px] text-slate-700">{{ activityTimeAgo(item) }}</time>
                  </div>
                  <p class="mt-0.5 text-[11px] leading-4 text-slate-500">{{ item.detail }}</p>
                </div>
              </div>
            </div>
            <div v-else class="mt-4 rounded-xl border border-dashed border-white/10 px-3 py-8 text-center">
              <SignalIcon class="mx-auto size-5 text-slate-700" aria-hidden="true" />
              <p class="mt-2 text-xs text-slate-500">Waiting for the next live event.</p>
            </div>
          </section>

          <section class="milestone-card overflow-hidden rounded-2xl border p-4">
            <div class="final-stretch-art mb-4 overflow-hidden rounded-xl border" aria-hidden="true">
              <img :src="spectatorLeagueBannerUrl" alt="" class="h-full w-full object-cover object-[72%_54%]" />
              <div class="final-stretch-art-shade" />
              <SparklesIcon class="final-stretch-art-sparkle size-5" />
            </div>
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="text-[9px] font-black tracking-[0.11em] text-cyan-200/70 uppercase">{{ leagueGoal ? 'Final stretch' : 'Main story' }}</div>
                <h2 class="mt-2 text-base font-black text-white">Elite Four & Hall of Fame</h2>
                <p class="mt-1 text-[11px] leading-5 text-slate-400">
                  {{ leagueGoal ? 'The run only reaches 100% after the Hall of Fame.' : 'Story milestones are tracked separately from the overall run goal.' }}
                </p>
              </div>
              <div class="final-stretch-mark grid size-11 shrink-0 place-items-center rounded-xl">
                <TrophyIcon class="size-5" aria-hidden="true" />
              </div>
            </div>

            <div class="stretch-list relative mt-4 space-y-2">
              <div
                v-for="step in finalStretchSteps"
                :key="step.key"
                :class="['stretch-step relative rounded-xl border p-3', stretchToneClass(step.state)]"
              >
                <div class="flex items-start gap-3">
                  <span :class="['stretch-step-icon relative z-10', stretchToneClass(step.state)]">
                    <component :is="step.icon" class="size-4" aria-hidden="true" />
                  </span>
                  <div class="min-w-0 flex-1">
                    <div class="flex items-center justify-between gap-2">
                      <strong class="text-[11px] text-slate-200">{{ step.title }}</strong>
                      <span v-if="step.value" class="font-mono text-[9px] text-slate-500">{{ step.value }}</span>
                      <span v-else-if="step.state === 'active'" class="rounded-full bg-cyan-300/10 px-1.5 py-0.5 text-[8px] font-bold text-cyan-200 ring-1 ring-cyan-300/15">In progress</span>
                      <span v-else-if="step.state === 'done'" class="text-[8px] font-bold text-emerald-300">Complete</span>
                      <span v-else class="text-[8px] font-bold text-slate-700">Pending</span>
                    </div>
                    <p class="mt-0.5 text-[10px] leading-4 text-slate-500">{{ step.detail }}</p>
                  </div>
                </div>
              </div>
            </div>
          </section>

          <section class="watching-card rounded-2xl border px-4 py-3">
            <div class="flex items-center gap-3">
              <div class="flex -space-x-1.5">
                <span class="viewer-dot bg-cyan-300" />
                <span class="viewer-dot bg-violet-300" />
                <span class="viewer-dot bg-emerald-300" />
              </div>
              <div>
                <strong class="block text-[11px] text-slate-300">Watching AI play classic games</strong>
                <span class="text-[10px] text-slate-600">Real gameplay · real progress · no operator controls</span>
              </div>
            </div>
          </section>
        </aside>
      </div>
    </div>
  </AppShell>
</template>

<style scoped>

.spectator-stage {
  align-items: start;
}

.spectator-card,
.player-card,
.milestone-card,
.watching-card {
  border-color: rgba(148, 163, 184, 0.12);
  background:
    linear-gradient(180deg, rgba(19, 30, 48, 0.92), rgba(8, 15, 28, 0.94)),
    rgba(8, 15, 28, 0.95);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.025);
}

.player-card {
  border-color: rgba(96, 165, 250, 0.22);
  background: rgba(3, 7, 18, 0.96);
}

.player-shell {
  box-shadow:
    inset 0 0 0 1px rgba(255, 255, 255, 0.025),
    inset 0 0 5rem rgba(15, 23, 42, 0.24);
}

.game-mark {
  color: #f8fafc;
  background:
    radial-gradient(circle at 30% 20%, rgba(255,255,255,.2), transparent 35%),
    linear-gradient(145deg, rgba(251,113,133,.88), rgba(59,130,246,.78));
}

.audience-stat {
  background: rgba(2, 6, 23, 0.28);
}

.badge-earned {
  background: rgba(251, 191, 36, 0.08);
  box-shadow: inset 0 0 0 1px rgba(251, 191, 36, 0.2);
}

.badge-empty {
  background: rgba(255, 255, 255, 0.025);
  box-shadow: inset 0 0 0 1px rgba(148, 163, 184, 0.12);
}

.player-control {
  display: grid;
  width: 2.25rem;
  height: 2.25rem;
  place-items: center;
  border: 1px solid rgba(255,255,255,.12);
  border-radius: .65rem;
  background: rgba(2, 6, 23, .72);
  color: rgb(226 232 240);
  backdrop-filter: blur(10px);
  transition: background .15s ease, border-color .15s ease, transform .15s ease;
}

.player-control:hover {
  border-color: rgba(103, 232, 249, .28);
  background: rgba(15, 23, 42, .9);
  transform: translateY(-1px);
}

.activity-list::before {
  position: absolute;
  top: .9rem;
  bottom: .9rem;
  left: .84rem;
  width: 1px;
  background: linear-gradient(to bottom, rgba(103,232,249,.38), rgba(148,163,184,.10));
  content: '';
}

.milestone-card {
  border-color: rgba(103, 232, 249, 0.16);
  background:
    radial-gradient(circle at 0% 0%, rgba(56, 189, 248, .12), transparent 18rem),
    linear-gradient(180deg, rgba(19, 30, 48, 0.94), rgba(8, 15, 28, 0.96));
}

.watching-card {
  background:
    linear-gradient(135deg, rgba(16,185,129,.07), rgba(59,130,246,.04)),
    rgba(8, 15, 28, 0.9);
}

.viewer-dot {
  display: block;
  width: 1.3rem;
  height: 1.3rem;
  border: 2px solid #0b1220;
  border-radius: 9999px;
  opacity: .9;
}

@media (min-width: 80rem) {
  .spectator-stage > aside {
    position: sticky;
    top: 4.5rem;
  }
}

.spectator-theme {
  --mode-accent: #67e8f9;
  --mode-soft: rgba(103, 232, 249, 0.1);
  --mode-border: rgba(103, 232, 249, 0.28);
}

.mode-speedrun {
  --mode-accent: #fb7185;
  --mode-soft: rgba(251, 113, 133, 0.1);
  --mode-border: rgba(251, 113, 133, 0.28);
}

.mode-adventure {
  --mode-accent: #67e8f9;
  --mode-soft: rgba(103, 232, 249, 0.1);
  --mode-border: rgba(103, 232, 249, 0.28);
}

.mode-completionist {
  --mode-accent: #c084fc;
  --mode-soft: rgba(192, 132, 252, 0.1);
  --mode-border: rgba(192, 132, 252, 0.3);
}

.mode-team_builder {
  --mode-accent: #6ee7b7;
  --mode-soft: rgba(110, 231, 183, 0.1);
  --mode-border: rgba(110, 231, 183, 0.28);
}

.mode-hero {
  border-color: var(--mode-border);
  background:
    radial-gradient(circle at 85% 0%, var(--mode-soft), transparent 32rem),
    #0d131c;
}

.mode-chip {
  border: 1px solid var(--mode-border);
  border-radius: 9999px;
  background: var(--mode-soft);
  padding: 0.2rem 0.55rem;
  color: var(--mode-accent);
  font-size: 0.625rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.mode-text {
  color: var(--mode-accent);
}

.mode-dot,
.mode-progress {
  background: var(--mode-accent);
}

.mode-metric {
  border-color: var(--mode-border);
}

.planner-spinner {
  position: relative;
  animation: planner-breathe 1.7s ease-in-out infinite;
}

.planner-spinner::after {
  position: absolute;
  inset: -0.35rem;
  border: 1px solid rgba(103, 232, 249, 0.22);
  border-radius: 9999px;
  content: '';
  animation: planner-ring 1.7s ease-out infinite;
}

.planner-dots > span {
  width: 0.25rem;
  height: 0.25rem;
  border-radius: 9999px;
  background: rgb(165 243 252);
  animation: planner-dot 1.15s ease-in-out infinite;
}

.planner-dots > span:nth-child(2) {
  animation-delay: 0.16s;
}

.planner-dots > span:nth-child(3) {
  animation-delay: 0.32s;
}

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
  animation: planner-ring 2.2s ease-out infinite;
}

.live-radar::after {
  animation-delay: 1.1s;
}

@keyframes planner-breathe {
  0%, 100% { transform: scale(0.96); box-shadow: 0 0 0 rgba(34, 211, 238, 0); }
  50% { transform: scale(1.04); box-shadow: 0 0 1.25rem rgba(34, 211, 238, 0.18); }
}

@keyframes planner-ring {
  0% { opacity: 0.75; transform: scale(0.72); }
  100% { opacity: 0; transform: scale(1.35); }
}

@keyframes planner-dot {
  0%, 70%, 100% { opacity: 0.3; transform: translateY(0); }
  35% { opacity: 1; transform: translateY(-0.18rem); }
}

:fullscreen {
  background: #05070a;
}

.spectator-theme {
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

.spectator-card,
.milestone-card,
.watching-card {
  backdrop-filter: blur(14px);
}

.spectator-art-banner {
  position: relative;
  height: 5.5rem;
  border-color: rgba(103,232,249,.16);
  background: #07101d;
  box-shadow:
    inset 0 1px 0 rgba(255,255,255,.04),
    0 10px 24px rgba(0,0,0,.18);
}

.spectator-art-banner img {
  opacity: .94;
  filter: saturate(1.08) contrast(1.03);
}

.spectator-art-banner-glow {
  position: absolute;
  inset: 0;
  background:
    linear-gradient(90deg, rgba(6,12,24,.08), rgba(6,12,24,.04) 55%, rgba(6,12,24,.16)),
    linear-gradient(180deg, transparent 38%, rgba(4,8,16,.72));
}

.spectator-art-banner-caption {
  position: absolute;
  right: .65rem;
  bottom: .55rem;
  left: .65rem;
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: .5rem;
  text-shadow: 0 1px 8px rgba(0,0,0,.8);
}

.spectator-art-banner-caption span {
  color: rgb(165 243 252 / .72);
  font-size: .48rem;
  font-weight: 800;
  letter-spacing: .11em;
  text-transform: uppercase;
}

.spectator-art-banner-caption strong {
  max-width: 62%;
  overflow: hidden;
  color: rgba(248,250,252,.9);
  font-size: .55rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.final-stretch-art {
  position: relative;
  height: 5rem;
  border-color: rgba(103,232,249,.15);
  background: #07101d;
  box-shadow: inset 0 1px 0 rgba(255,255,255,.035);
}

.final-stretch-art img {
  opacity: .9;
  filter: saturate(1.12) contrast(1.04);
}

.final-stretch-art-shade {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(circle at 82% 38%, rgba(103,232,249,.02), rgba(2,6,23,.06) 42%, rgba(2,6,23,.62)),
    linear-gradient(180deg, transparent 22%, rgba(3,7,18,.62));
}

.final-stretch-art-sparkle {
  position: absolute;
  top: .7rem;
  right: .8rem;
  color: rgb(207 250 254 / .9);
  filter: drop-shadow(0 0 10px rgba(103,232,249,.5));
}

.spectator-hero-card {
  border-color: var(--mode-border);
  background:
    linear-gradient(180deg, rgba(13, 23, 40, .74), rgba(6, 12, 24, .91)),
    var(--spectator-art) 24% 70% / 52rem auto no-repeat;
  box-shadow:
    inset 0 1px 0 rgba(255,255,255,.04),
    0 18px 44px rgba(0,0,0,.22);
}

.goal-progress-card {
  border-color: var(--mode-border);
  background:
    radial-gradient(circle at 100% 0%, var(--mode-soft), transparent 12rem),
    rgba(2, 6, 23, .48);
  box-shadow: inset 0 1px 0 rgba(255,255,255,.035);
}

.goal-progress-fill {
  box-shadow: 0 0 18px color-mix(in srgb, var(--mode-accent) 38%, transparent);
}

.pokeball-mark {
  position: relative;
  display: block;
  width: 1.65rem;
  height: 1.65rem;
  overflow: hidden;
  border: 2px solid rgba(255,255,255,.9);
  border-radius: 9999px;
  background: linear-gradient(to bottom, #fb7185 0 46%, #e2e8f0 46% 54%, #f8fafc 54% 100%);
  box-shadow: 0 0 18px rgba(251,113,133,.2);
}

.pokeball-mark::before {
  position: absolute;
  top: 50%;
  left: 0;
  width: 100%;
  height: 2px;
  background: rgba(15,23,42,.86);
  content: '';
  transform: translateY(-50%);
}

.pokeball-mark::after {
  position: absolute;
  top: 50%;
  left: 50%;
  width: .52rem;
  height: .52rem;
  border: 2px solid rgba(15,23,42,.9);
  border-radius: 9999px;
  background: white;
  content: '';
  transform: translate(-50%, -50%);
}

.premium-stat {
  position: relative;
  overflow: hidden;
  border-color: rgba(255,255,255,.08);
  background:
    linear-gradient(155deg, rgba(255,255,255,.035), transparent 55%),
    rgba(2,6,23,.35);
  box-shadow: inset 0 1px 0 rgba(255,255,255,.025);
}

.premium-stat::after {
  position: absolute;
  right: -1.6rem;
  bottom: -1.8rem;
  width: 5rem;
  height: 5rem;
  border-radius: 9999px;
  content: '';
  opacity: .12;
  filter: blur(10px);
}

.premium-stat-icon {
  display: grid;
  width: 2rem;
  height: 2rem;
  place-items: center;
  border: 1px solid currentColor;
  border-radius: .7rem;
  background: rgba(255,255,255,.035);
}

.stat-tone-cyan { color: rgb(165 243 252); }
.stat-tone-violet { color: rgb(221 214 254); }
.stat-tone-emerald { color: rgb(167 243 208); }
.stat-tone-amber { color: rgb(253 230 138); }
.premium-stat.stat-tone-cyan::after { background: rgb(34 211 238); }
.premium-stat.stat-tone-violet::after { background: rgb(168 85 247); }
.premium-stat.stat-tone-emerald::after { background: rgb(16 185 129); }
.premium-stat.stat-tone-amber::after { background: rgb(245 158 11); }

.final-stretch-mark {
  border: 1px solid rgba(103,232,249,.18);
  background:
    radial-gradient(circle at 35% 20%, rgba(255,255,255,.15), transparent 45%),
    rgba(34,211,238,.09);
  color: rgb(207 250 254);
  box-shadow: 0 0 26px rgba(34,211,238,.08);
}

.stretch-list::before {
  position: absolute;
  top: 1rem;
  bottom: 1rem;
  left: 1rem;
  width: 1px;
  background: linear-gradient(to bottom, rgba(52,211,153,.35), rgba(103,232,249,.22), rgba(148,163,184,.08));
  content: '';
}

.stretch-step {
  border-color: rgba(255,255,255,.075);
  background: rgba(2,6,23,.34);
  transition: border-color .15s ease, background .15s ease, transform .15s ease;
}

.stretch-step-icon {
  display: grid;
  width: 2rem;
  height: 2rem;
  flex: 0 0 2rem;
  place-items: center;
  border: 1px solid rgba(255,255,255,.08);
  border-radius: .72rem;
  background: rgba(15,23,42,.92);
}

.stretch-step-done {
  border-color: rgba(52,211,153,.18);
}
.stretch-step-done .stretch-step-icon {
  border-color: rgba(52,211,153,.25);
  background: rgba(16,185,129,.11);
  color: rgb(167 243 208);
}
.stretch-step-active {
  border-color: rgba(103,232,249,.22);
  background:
    radial-gradient(circle at 0% 50%, rgba(103,232,249,.09), transparent 10rem),
    rgba(2,6,23,.4);
}
.stretch-step-active .stretch-step-icon {
  border-color: rgba(103,232,249,.28);
  background: rgba(34,211,238,.1);
  color: rgb(165 243 252);
  box-shadow: 0 0 18px rgba(34,211,238,.08);
}
.stretch-step-todo {
  opacity: .78;
}
.stretch-step-todo .stretch-step-icon {
  color: rgb(100 116 139);
}

.milestone-card {
  background:
    linear-gradient(180deg, rgba(13, 23, 40, .80), rgba(6, 12, 24, .93)),
    var(--spectator-art) 80% 68% / 48rem auto no-repeat;
}


</style>
