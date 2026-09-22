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
import Panel from '../shared/components/Panel.vue'
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
  playStyleTagline,
  preferredRun,
  routeLabel,
  runStatusLabel,
  runTitle,
  runTone,
  splitSpectatorRuns
} from './model'
import PartyProgress from './PartyProgress.vue'
import PublicHome from './PublicHome.vue'
import { policyLabel } from '../shared/playstyle'
import { dexMeter } from '../shared/playerProgress'
import { MAP_CATALOG } from '../shared/mapCatalog'
import { runIDFromLocation, spectatorRunPath } from '../shared/urls'

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

const modeMetrics = computed(() => {
  const run = selectedRun.value
  if (!run) return []
  const party = run.player?.party || []
  const badges = run.player?.badges?.length || 0
  const avgLevel = party.length
    ? Math.round(party.reduce((sum, mon) => sum + Number(mon.level || 0), 0) / party.length)
    : 0
  const totalHP = party.reduce((sum, mon) => sum + Number(mon.hp || 0), 0)
  const totalMaxHP = party.reduce((sum, mon) => sum + Number(mon.max_hp || 0), 0)
  const health = totalMaxHP > 0 ? Math.round(100 * totalHP / totalMaxHP) : 0

  switch (normalizePlayStyle(run)) {
    case 'adventure':
      return [
        { label: 'Journey', value: locationLabel(run), note: routeLabel(run) },
        { label: 'Party', value: `${party.length}/6`, note: party[0]?.name || 'building a team' },
        { label: 'Badges', value: String(badges), note: 'story milestones' }
      ]
    case 'completionist':
      return [
        { label: 'Goal', value: `${goalProgress(run).toFixed(0)}%`, note: objectiveLabel(run) },
        { label: 'Party', value: `${party.length}/6`, note: 'caught companions' },
        { label: 'Wild policy', value: policyLabel(run.wild_encounters || 'planner'), note: 'encounter behavior' }
      ]
    case 'team_builder':
      return [
        { label: 'Avg level', value: avgLevel ? `Lv ${avgLevel}` : '—', note: party[0]?.name || 'waiting for party data' },
        { label: 'Party health', value: `${health}%`, note: `${party.length}/6 slots filled` },
        { label: 'Badges', value: String(badges), note: 'progress while training' }
      ]
    default:
      return [
        { label: 'Goal pace', value: `${goalProgress(run).toFixed(0)}%`, note: objectiveLabel(run) },
        { label: 'Frame', value: Number(run.frame || 0).toLocaleString(), note: `${run.stats?.round ?? 0} planner rounds` },
        { label: 'Attempts', value: String(run.attempts || 1), note: `${playSpeedLabel(run)} play speed` }
      ]
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
      pushActivity(run.run_id, 'area', 'New area', locationLabel(run))
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

function activityTime(item: ActivityItem): string {
  return new Date(item.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
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
        <span><strong class="text-white">{{ snapshot.summary.live }}</strong> live now</span>
        <span v-if="selectedRun"><strong class="text-white">{{ mapsLabel }}</strong> maps explored</span>
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

      <section v-if="selectionPinned" class="mode-hero overflow-hidden rounded-xl border bg-[#0d131c] shadow-xl shadow-black/15">
        <div class="grid gap-5 px-4 py-4 sm:px-5 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <span v-if="isLiveRun(selectedRun)" class="inline-flex items-center gap-1.5 rounded-full bg-red-500/12 px-2 py-1 text-[10px] font-bold tracking-[0.12em] text-red-300 uppercase ring-1 ring-red-400/20">
                <span class="size-1.5 animate-pulse rounded-full bg-red-400" />
                Live
              </span>
              <StatusBadge v-else :tone="runTone(selectedRun)">{{ runStatusLabel(selectedRun) }}</StatusBadge>
              <span class="mode-chip">{{ playStyleLabel(selectedRun) }}</span>
              <span class="text-[11px] text-slate-500">{{ playStyleTagline(selectedRun) }}</span>
            </div>
            <h1 class="mt-2 truncate text-xl font-semibold tracking-tight text-white sm:text-2xl">{{ runTitle(selectedRun) }}</h1>
            <p class="mt-1 truncate text-sm text-slate-400">{{ routeLabel(selectedRun) }}</p>
            <div class="mt-3 flex min-w-0 items-start gap-2">
              <span class="mode-dot mt-1.5 size-2 shrink-0 rounded-full" />
              <div class="min-w-0">
                <p class="text-[10px] font-semibold tracking-[0.1em] text-slate-500 uppercase">Current objective</p>
                <p class="mt-0.5 truncate text-sm font-medium text-slate-200">{{ objectiveLabel(selectedRun) }}</p>
              </div>
            </div>
          </div>

          <div class="grid grid-cols-3 gap-px overflow-hidden rounded-lg bg-white/10 ring-1 ring-white/10 sm:grid-cols-6 lg:min-w-[36rem]">
            <div class="bg-[#0b1119] px-3 py-2.5 text-center">
              <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-500 uppercase">Badges</div>
              <div class="mt-1 font-mono text-lg font-semibold text-white">{{ selectedRun.player?.badges?.length || 0 }}</div>
            </div>
            <div class="bg-[#0b1119] px-3 py-2.5 text-center">
              <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-500 uppercase">Maps</div>
              <div class="mt-1 font-mono text-lg font-semibold text-white">{{ mapsLabel }}</div>
            </div>
            <div class="bg-[#0b1119] px-3 py-2.5 text-center">
              <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-500 uppercase">Party</div>
              <div class="mt-1 font-mono text-lg font-semibold text-white">{{ selectedRun.player?.party?.length || 0 }}/6</div>
            </div>
            <div class="bg-[#0b1119] px-3 py-2.5 text-center">
              <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-500 uppercase">Dex</div>
              <div class="mt-1 font-mono text-lg font-semibold text-white">{{ dexMeter(selectedRun.player) || '—' }}</div>
            </div>
            <div class="bg-[#0b1119] px-3 py-2.5 text-center">
              <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-500 uppercase">Money</div>
              <div class="mt-1 font-mono text-sm font-semibold text-white sm:text-base">{{ moneyLabel(selectedRun) }}</div>
            </div>
            <div class="bg-[#0b1119] px-3 py-2.5 text-center">
              <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-500 uppercase">Speed</div>
              <div class="mode-text mt-1 font-mono text-lg font-semibold">{{ playSpeedLabel(selectedRun) }}</div>
            </div>
          </div>
        </div>
      </section>

      <div :class="['grid grid-cols-1 gap-3', theaterMode ? '' : 'xl:grid-cols-[minmax(0,2.2fr)_minmax(19rem,0.8fr)]']">
        <div class="space-y-3">
          <Panel title="Game" description="Live gameplay broadcast" compact>
            <template #actions>
              <div class="flex items-center gap-1.5">
                <button type="button" class="rounded-md bg-white/7 px-2 py-1 text-[10px] font-semibold text-slate-300 ring-1 ring-white/10 hover:bg-white/12 hover:text-white" @click="theaterMode = !theaterMode">
                  {{ theaterMode ? 'Exit theater' : 'Theater' }}
                </button>
                <button type="button" class="inline-flex items-center gap-1 rounded-md bg-white/7 px-2 py-1 text-[10px] font-semibold text-slate-300 ring-1 ring-white/10 hover:bg-white/12 hover:text-white" @click="fullscreenPlayer">
                  <ArrowsPointingOutIcon class="size-3" aria-hidden="true" />
                  Fullscreen
                </button>
              </div>
            </template>

            <div
              ref="playerRef"
              :class="[
                'group relative overflow-hidden rounded-lg border border-white/10 bg-black shadow-inner shadow-black',
                theaterMode ? 'min-h-[72vh]' : 'min-h-[26rem] sm:min-h-[34rem] lg:min-h-[39rem]'
              ]"
            >
              <img
                v-if="frameURL"
                :src="frameURL"
                :alt="`Live frame for ${selectedRun.run_id}`"
                class="absolute inset-0 h-full w-full object-contain object-center [image-rendering:pixelated]"
              />

              <div v-else class="absolute inset-0 grid place-items-center px-6 py-12 text-center">
                <div>
                  <div class="mx-auto flex size-12 items-center justify-center rounded-full border border-white/10 bg-white/5">
                    <span :class="['size-2.5 rounded-full', isLiveRun(selectedRun) ? 'animate-pulse bg-emerald-300' : 'bg-slate-600']" />
                  </div>
                  <p class="mt-3 text-sm font-medium text-slate-300">Waiting for the live stream</p>
                  <p v-if="frameState === 'error' && frameError" class="mt-1 text-xs text-amber-300/80">{{ frameError }}</p>
                </div>
              </div>

              <div class="pointer-events-none absolute inset-x-0 top-0 flex items-center justify-between bg-gradient-to-b from-black/65 to-transparent px-3 py-3 text-[10px] font-semibold tracking-[0.08em] uppercase">
                <span class="rounded bg-black/45 px-2 py-1 text-slate-200 ring-1 ring-white/10">{{ playStyleLabel(selectedRun) }}</span>
                <span class="rounded bg-black/45 px-2 py-1 font-mono text-slate-300 ring-1 ring-white/10">{{ locationLabel(selectedRun) }}</span>
              </div>

              <div class="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 via-black/50 to-transparent px-3 pb-3 pt-14 sm:px-4 sm:pb-4">
                <div v-if="plannerState" class="flex items-end justify-between gap-4">
                  <div class="flex min-w-0 items-center gap-3">
                    <div class="planner-spinner grid size-9 shrink-0 place-items-center rounded-full border border-cyan-300/25 bg-cyan-300/10">
                      <SparklesIcon class="size-4 text-cyan-200" aria-hidden="true" />
                    </div>
                    <div class="min-w-0">
                      <div class="text-[9px] font-semibold tracking-[0.12em] text-cyan-200 uppercase">{{ plannerState.title }}</div>
                      <div class="mt-1 flex items-center gap-2 text-xs text-white sm:text-sm">
                        <span class="truncate">{{ plannerState.detail }}</span>
                        <span class="planner-dots inline-flex shrink-0 gap-1" aria-hidden="true">
                          <span />
                          <span />
                          <span />
                        </span>
                      </div>
                    </div>
                  </div>
                  <div class="shrink-0 rounded bg-black/50 px-2 py-1 font-mono text-[10px] text-cyan-100 ring-1 ring-cyan-300/20">LLM</div>
                </div>
                <div v-else class="flex items-end justify-between gap-4">
                  <div class="min-w-0">
                    <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-400 uppercase">Latest decision</div>
                    <div class="mt-1 line-clamp-2 max-w-4xl text-xs leading-5 text-white sm:text-sm">{{ selectedRun.decision || 'Preparing the next objective' }}</div>
                  </div>
                  <div class="shrink-0 rounded bg-black/50 px-2 py-1 font-mono text-[10px] text-slate-300 ring-1 ring-white/10">{{ playSpeedLabel(selectedRun) }}</div>
                </div>
              </div>

              <div v-if="frameURL && frameState === 'error'" class="absolute right-3 top-12 rounded-md bg-black/75 px-2 py-1 text-[10px] font-semibold text-amber-200 ring-1 ring-amber-300/20">
                Last frame · reconnecting
              </div>
            </div>
          </Panel>

          <Panel title="Party" :description="normalizePlayStyle(selectedRun) === 'team_builder' ? 'The team is the story in this run.' : 'Live party health and levels.'" compact>
            <div
              v-if="selectedRun.player?.party?.length"
              :class="[
                'grid gap-2',
                normalizePlayStyle(selectedRun) === 'team_builder' ? 'grid-cols-1 sm:grid-cols-2 xl:grid-cols-3' : 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3'
              ]"
            >
              <PokemonPartyCard
                v-for="(mon, index) in selectedRun.player.party"
                :key="`${mon.name}-${index}`"
                :name="mon.name"
                :level="mon.level"
                :hp="mon.hp"
                :max-hp="mon.max_hp"
                :status="mon.status"
                :lead="index === 0"
              />
            </div>
            <p v-else class="py-5 text-center text-sm text-slate-500">Party data is not available yet.</p>

            <PartyProgress :run="selectedRun" />
          </Panel>

          <Panel title="Activity" description="Live decisions and progression, grouped by event type." compact>
            <div class="mb-3 flex flex-wrap gap-1.5">
              <button
                v-for="filter in activityFilters"
                :key="filter"
                type="button"
                :class="[
                  activityFilter === filter
                    ? 'bg-cyan-300/12 text-cyan-100 ring-cyan-300/25'
                    : 'bg-white/5 text-slate-500 ring-white/8 hover:bg-white/8 hover:text-slate-300',
                  'rounded-full px-2.5 py-1 text-[10px] font-semibold capitalize ring-1 transition-colors'
                ]"
                @click="activityFilter = filter"
              >
                {{ filter }}
              </button>
            </div>
            <div v-if="selectedActivity.length" class="divide-y divide-white/8">
              <div v-for="item in selectedActivity" :key="item.id" class="grid grid-cols-[4rem_2rem_minmax(0,1fr)] items-start gap-2.5 py-2.5 first:pt-0 last:pb-0">
                <time class="pt-1 font-mono text-[10px] text-slate-600">{{ activityTime(item) }}</time>
                <span
                  :class="[activityTone(item.kind), 'grid size-7 place-items-center rounded-md ring-1']"
                  :title="item.label"
                >
                  <component :is="activityIcon(item.kind)" class="size-3.5" aria-hidden="true" />
                </span>
                <div class="min-w-0">
                  <strong class="text-xs text-slate-300">{{ item.label }}</strong>
                  <p class="mt-0.5 text-xs leading-5 text-slate-500">{{ item.detail }}</p>
                </div>
              </div>
            </div>
            <p v-else class="py-5 text-center text-xs text-slate-500">
              {{ activityFilter === 'all' ? 'New decisions, areas, catches and badges will appear here.' : 'No ' + activityFilter + ' events yet.' }}
            </p>
          </Panel>
        </div>

        <aside class="space-y-3">
          <Panel :title="`${playStyleLabel(selectedRun)} focus`" :description="playStyleTagline(selectedRun)" compact>
            <div class="grid gap-2 sm:grid-cols-3 xl:grid-cols-1">
              <div v-for="metric in modeMetrics" :key="metric.label" class="mode-metric rounded-lg border bg-black/10 px-3 py-2.5">
                <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-500 uppercase">{{ metric.label }}</div>
                <div class="mode-text mt-1 truncate font-mono text-base font-semibold">{{ metric.value }}</div>
                <div class="mt-0.5 truncate text-[10px] text-slate-600">{{ metric.note }}</div>
              </div>
            </div>
          </Panel>

          <Panel title="Objective" :description="objectiveLabel(selectedRun)" compact>
            <div class="h-2 overflow-hidden rounded-full bg-white/8">
              <div class="mode-progress h-full rounded-full transition-[width]" :style="{ width: `${goalProgress(selectedRun)}%` }" />
            </div>
            <div class="mt-2 flex items-center justify-between gap-3 font-mono text-[10px] text-slate-500">
              <span v-if="selectedRun.stats?.goal_target">{{ selectedRun.stats.goal_current || 0 }} / {{ selectedRun.stats.goal_target }}</span>
              <span v-else>goal-driven</span>
              <span>{{ goalProgress(selectedRun).toFixed(0) }}%</span>
            </div>
          </Panel>

          <Panel title="Run settings" description="Public-safe behavior profile." compact>
            <dl class="grid grid-cols-2 gap-x-3 gap-y-2 text-xs">
              <div>
                <dt class="text-[9px] font-semibold tracking-[0.08em] text-slate-600 uppercase">Play style</dt>
                <dd class="mode-text mt-0.5 font-semibold">{{ playStyleLabel(selectedRun) }}</dd>
              </div>
              <div>
                <dt class="text-[9px] font-semibold tracking-[0.08em] text-slate-600 uppercase">Speed</dt>
                <dd class="mt-0.5 font-mono font-semibold text-slate-300">{{ playSpeedLabel(selectedRun) }}</dd>
              </div>
              <div>
                <dt class="text-[9px] font-semibold tracking-[0.08em] text-slate-600 uppercase">Risk</dt>
                <dd class="mt-0.5 text-slate-300">{{ policyLabel(selectedRun.risk_tolerance || 'balanced') }}</dd>
              </div>
              <div>
                <dt class="text-[9px] font-semibold tracking-[0.08em] text-slate-600 uppercase">Wild encounters</dt>
                <dd class="mt-0.5 text-slate-300">{{ policyLabel(selectedRun.wild_encounters || 'planner') }}</dd>
              </div>
            </dl>
            <div class="mt-3 border-t border-white/8 pt-3">
              <div class="text-[9px] font-semibold tracking-[0.08em] text-slate-600 uppercase">Run ID</div>
              <div class="mt-1 break-all font-mono text-[10px] text-slate-500">{{ selectedRun.run_id }}</div>
            </div>
          </Panel>

          <Panel title="Live runs" description="Switch streams without leaving the page." compact>
            <div v-if="groupedRuns.live.length" class="space-y-1.5">
              <button
                v-for="run in groupedRuns.live"
                :key="run.run_id"
                type="button"
                :class="[
                  run.run_id === selectedRun.run_id ? 'bg-white/8 ring-white/15' : 'bg-black/10 ring-white/8 hover:bg-white/5',
                  'w-full rounded-md px-3 py-2 text-left ring-1 transition-colors'
                ]"
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
            <p v-else class="py-4 text-center text-xs text-slate-500">Nothing is running right now.</p>
          </Panel>


        </aside>
      </div>
    </div>
  </AppShell>
</template>

<style scoped>
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
