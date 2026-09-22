<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  ArrowPathIcon,
  ArrowsPointingOutIcon,
  BoltIcon,
  BugAntIcon,
  GlobeAltIcon,
  LinkIcon,
  PlayIcon,
  QueueListIcon,
  SignalIcon,
  SparklesIcon,
  TrophyIcon
} from '@heroicons/vue/20/solid'
import { getSpectatorSnapshot } from '../shared/api/spectator-client'
import type { SpectatorRun } from '../shared/api/spectator'
import AppShell from '../shared/components/AppShell.vue'
import BadgeIcon from '../shared/components/BadgeIcon.vue'
import PokemonPartyCard from '../shared/components/PokemonPartyCard.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { useFramePump } from '../shared/composables/useFramePump'
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

type ActivityKind = 'decision' | 'area' | 'badge' | 'party' | 'dex' | 'milestone' | 'state'
type ActivityFilter = 'all' | 'milestones' | 'decisions'

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
const frameRunID = computed(() => {
  const run = selectedRun.value
  return run && isLiveRun(run) ? run.run_id : ''
})
const frameEnabled = computed(() => Boolean(frameRunID.value))
const frameContinuous = computed(() => isLiveRun(selectedRun.value))
const { frameURL, state: frameState, error: frameError } = useFramePump(frameRunID, frameEnabled, 50, frameContinuous)
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
const nextMilestone = computed(() => {
  const run = selectedRun.value
  if (!run) return null
  const badges = run.player?.badges?.length || 0
  if (badges < 8) {
    const badgeName = GYM_BADGES[badges] || 'Next'
    return {
      eyebrow: 'Next milestone',
      title: badgeName + ' Badge',
      detail: (8 - badges) + ' gym badge' + (8 - badges === 1 ? '' : 's') + ' remain before the Indigo Plateau.',
      progress: badges + 1 + ' of 8 badges'
    }
  }
  return {
    eyebrow: 'Final stretch',
    title: 'Elite Four & Hall of Fame',
    detail: 'All gym badges are earned. The next major public milestone is entering the Hall of Fame.',
    progress: 'All 8 badges earned'
  }
})

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
    case 'area': return GlobeAltIcon
    case 'badge': return TrophyIcon
    case 'party': return QueueListIcon
    case 'dex': return BugAntIcon
    case 'milestone': return BoltIcon
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

    <div v-else-if="selectedRun" :class="['spectator-theme mx-auto max-w-[112rem] space-y-3', modeClass]">
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
          <section class="spectator-card overflow-hidden rounded-2xl border p-4 shadow-xl shadow-black/20">
            <div class="flex flex-wrap items-center gap-2">
              <span v-if="isLiveRun(selectedRun)" class="inline-flex items-center gap-1.5 rounded-full bg-red-500/12 px-2.5 py-1 text-[10px] font-black tracking-[0.11em] text-red-300 uppercase ring-1 ring-red-400/20">
                <span class="size-1.5 animate-pulse rounded-full bg-red-400" />
                Live run
              </span>
              <StatusBadge v-else :tone="runTone(selectedRun)">{{ runStatusLabel(selectedRun) }}</StatusBadge>
              <span class="mode-chip">{{ playStyleLabel(selectedRun) }}</span>
            </div>

            <div class="mt-4 flex items-start gap-3">
              <div class="game-mark grid size-11 shrink-0 place-items-center rounded-xl ring-1 ring-white/10">
                <span class="text-2xl" aria-hidden="true">◉</span>
              </div>
              <div class="min-w-0">
                <h1 class="text-2xl font-black tracking-tight text-white">Pokémon Red</h1>
                <p class="mt-0.5 truncate text-sm text-slate-400">{{ playStyleLabel(selectedRun) }} · {{ selectedRun.player?.party?.[0]?.name || selectedRun.starter || 'new trainer' }}</p>
                <p class="mt-1 truncate text-[11px] text-slate-600">{{ currentLocation }}</p>
              </div>
            </div>

            <div class="mt-5">
              <p class="text-sm leading-6 text-slate-300">{{ objectiveLabel(selectedRun) }}</p>
              <div class="mt-3 h-2 overflow-hidden rounded-full bg-white/8">
                <div class="mode-progress h-full rounded-full transition-[width]" :style="{ width: goalProgress(selectedRun) + '%' }" />
              </div>
              <div class="mt-1.5 flex items-center justify-between font-mono text-[10px] text-slate-500">
                <span>{{ selectedRun.player?.badges?.length || 0 }} / 8 badges</span>
                <span>{{ goalProgress(selectedRun).toFixed(0) }}%</span>
              </div>
            </div>

            <div class="mt-5">
              <div class="mb-2 text-[9px] font-black tracking-[0.11em] text-slate-500 uppercase">Gym badges</div>
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
              <div class="audience-stat rounded-xl border border-white/8 p-3">
                <GlobeAltIcon class="size-4 text-cyan-200/80" aria-hidden="true" />
                <strong class="mt-2 block font-mono text-base text-white">{{ mapsLabel }}</strong>
                <span class="text-[10px] text-slate-500">Maps visited</span>
              </div>
              <div class="audience-stat rounded-xl border border-white/8 p-3">
                <QueueListIcon class="size-4 text-violet-200/80" aria-hidden="true" />
                <strong class="mt-2 block font-mono text-base text-white">{{ selectedRun.player?.party?.length || 0 }}/6</strong>
                <span class="text-[10px] text-slate-500">Party</span>
              </div>
              <div class="audience-stat rounded-xl border border-white/8 p-3">
                <SignalIcon class="size-4 text-emerald-200/80" aria-hidden="true" />
                <strong class="mt-2 block font-mono text-base text-white">{{ runtimeLabel }}</strong>
                <span class="text-[10px] text-slate-500">Runtime</span>
              </div>
              <div class="audience-stat rounded-xl border border-white/8 p-3">
                <TrophyIcon class="size-4 text-amber-200/80" aria-hidden="true" />
                <strong class="mt-2 block font-mono text-base text-white">{{ moneyLabel(selectedRun) }}</strong>
                <span class="text-[10px] text-slate-500">Money</span>
              </div>
            </div>
          </section>

          <section v-if="groupedRuns.live.length > 1" class="spectator-card rounded-2xl border p-3">
            <div class="mb-2 flex items-center justify-between gap-2">
              <div>
                <h2 class="text-xs font-bold text-white">Other live runs</h2>
                <p class="mt-0.5 text-[10px] text-slate-600">Switch streams instantly.</p>
              </div>
              <span class="rounded-full bg-emerald-300/10 px-2 py-1 font-mono text-[9px] text-emerald-200 ring-1 ring-emerald-300/20">{{ groupedRuns.live.length }} live</span>
            </div>
            <div class="space-y-1.5">
              <button
                v-for="run in groupedRuns.live.filter((item) => item.run_id !== selectedRun.run_id).slice(0, 3)"
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
              <img
                v-if="frameURL"
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
                  <p v-if="frameState === 'error' && frameError" class="mt-1 text-xs text-amber-300/80">{{ frameError }}</p>
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

              <div class="absolute right-3 top-14 z-10 flex flex-col gap-2 opacity-80 transition-opacity group-hover:opacity-100 sm:right-4">
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

              <div v-if="frameURL && frameState === 'error'" class="absolute left-3 top-14 rounded-full bg-amber-950/80 px-2.5 py-1 text-[10px] font-semibold text-amber-200 ring-1 ring-amber-300/20">
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

          <section v-if="nextMilestone" class="milestone-card overflow-hidden rounded-2xl border p-4">
            <div class="text-[9px] font-black tracking-[0.11em] text-cyan-200/70 uppercase">{{ nextMilestone.eyebrow }}</div>
            <div class="mt-3 flex items-start gap-3">
              <div class="grid size-12 shrink-0 place-items-center rounded-xl bg-cyan-300/10 ring-1 ring-cyan-300/20">
                <TrophyIcon class="size-6 text-cyan-100" aria-hidden="true" />
              </div>
              <div class="min-w-0">
                <h2 class="text-base font-black text-white">{{ nextMilestone.title }}</h2>
                <p class="mt-1 line-clamp-3 text-[11px] leading-5 text-slate-400">{{ nextMilestone.detail }}</p>
              </div>
            </div>
            <div class="mt-4 h-2 overflow-hidden rounded-full bg-white/8">
              <div class="mode-progress h-full rounded-full" :style="{ width: Math.min(100, ((selectedRun.player?.badges?.length || 0) / 8) * 100) + '%' }" />
            </div>
            <div class="mt-2 flex items-center justify-between text-[10px]">
              <span class="text-slate-600">{{ nextMilestone.progress }}</span>
              <span class="font-mono text-slate-500">{{ selectedRun.player?.badges?.length || 0 }}/8</span>
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
</style>
