<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ArrowPathIcon, ArrowsPointingOutIcon, LinkIcon } from '@heroicons/vue/20/solid'
import { getSpectatorSnapshot, spectatorReplayVideoURL } from '../shared/api/spectator-client'
import type { SpectatorRun } from '../shared/api/spectator'
import AppShell from '../shared/components/AppShell.vue'
import Panel from '../shared/components/Panel.vue'
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
  shortRunID,
  splitSpectatorRuns
} from './model'
import PartyProgress from './PartyProgress.vue'
import { policyLabel } from '../shared/playstyle'
import { bagMeter, dexMeter } from '../shared/playerProgress'

interface ActivityItem {
  id: string
  at: number
  label: string
  detail: string
}

const initialParams = new URLSearchParams(window.location.search)
const selectedRunID = ref(initialParams.get('run') || '')
const selectionPinned = ref(initialParams.has('run'))
const copyState = ref('')
const theaterMode = ref(false)
const playerRef = ref<HTMLElement | null>(null)
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
  runs.value,
  selectionPinned.value ? selectedRunID.value : ''
))
const liveRunID = computed(() => isLiveRun(selectedRun.value) ? selectedRun.value?.run_id || '' : '')
const frameEnabled = computed(() => Boolean(liveRunID.value))
const { frameURL, state: frameState, error: frameError } = useFramePump(liveRunID, frameEnabled)
const replayURL = computed(() => {
  const run = selectedRun.value
  return run?.status === 'done' && run.replay_ready ? spectatorReplayVideoURL(run.run_id) : ''
})
const modeClass = computed(() => `mode-${normalizePlayStyle(selectedRun.value)}`)
const selectedActivity = computed(() => selectedRun.value ? activityByRun.value[selectedRun.value.run_id] || [] : [])
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

watch(selectedRun, (run) => {
  if (run && !selectionPinned.value) selectedRunID.value = run.run_id
}, { immediate: true })

watch(runs, (nextRuns) => {
  for (const run of nextRuns) {
    const previous = previousRuns.get(run.run_id)
    if (!previous) {
      if (run.decision) pushActivity(run.run_id, 'Current decision', run.decision)
      else if (run.stop_so_far) pushActivity(run.run_id, 'Run state', run.stop_so_far)
      previousRuns.set(run.run_id, run)
      continue
    }

    if (run.decision && run.decision !== previous.decision) {
      pushActivity(run.run_id, 'Decision', run.decision)
    }
    if (run.map !== previous.map) {
      pushActivity(run.run_id, 'New area', locationLabel(run))
    }

    const beforeBadges = previous.player?.badges || []
    const afterBadges = run.player?.badges || []
    if (afterBadges.length > beforeBadges.length) {
      const earned = afterBadges.filter((badge) => !beforeBadges.includes(badge))
      pushActivity(run.run_id, 'Badge earned', earned.join(', ') || `${afterBadges.length} badges`)
    }

    const beforeParty = previous.player?.party || []
    const afterParty = run.player?.party || []
    if (afterParty.length > beforeParty.length) {
      const joined = afterParty.slice(beforeParty.length).map((mon) => mon.name).filter(Boolean)
      pushActivity(run.run_id, 'Pokémon joined', joined.join(', ') || `${afterParty.length}/6 party`)
    }

    const beforeDex = Number(previous.player?.dex_owned || 0)
    const afterDex = Number(run.player?.dex_owned || 0)
    if (afterDex > beforeDex) {
      pushActivity(run.run_id, 'Pokédex', `${afterDex} owned`)
    }

    const beforeMilestones = previous.player?.milestones || []
    const afterMilestones = run.player?.milestones || []
    if (afterMilestones.length > beforeMilestones.length) {
      const earned = afterMilestones.filter((beat) => !beforeMilestones.includes(beat))
      pushActivity(run.run_id, 'Milestone', earned.join(', ') || afterMilestones[afterMilestones.length - 1] || 'Progress')
    }

    if (previous.status !== 'done' && run.status === 'done') {
      pushActivity(run.run_id, 'Run finished', run.highlight || run.reason || 'Run complete')
    }
    previousRuns.set(run.run_id, run)
  }
}, { immediate: true })

function pushActivity(runID: string, label: string, detail: string): void {
  if (!detail) return
  const current = activityByRun.value[runID] || []
  const latest = current[0]
  if (latest?.label === label && latest.detail === detail) return
  activityByRun.value = {
    ...activityByRun.value,
    [runID]: [
      { id: `${Date.now()}-${label}-${detail}`, at: Date.now(), label, detail },
      ...current
    ].slice(0, 10)
  }
}

function selectRun(run: SpectatorRun): void {
  selectedRunID.value = run.run_id
  selectionPinned.value = true
  const url = new URL(window.location.href)
  url.searchParams.set('run', run.run_id)
  history.replaceState(null, '', url)
}

function refresh(): void {
  void retry()
}

async function copyLink(): Promise<void> {
  const run = selectedRun.value
  if (!run) return
  const url = new URL(window.location.href)
  url.searchParams.set('run', run.run_id)
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

function hpPercent(run: SpectatorRun, index: number): number {
  const mon = run.player?.party?.[index]
  if (!mon || mon.max_hp <= 0) return 0
  return Math.max(0, Math.min(100, 100 * mon.hp / mon.max_hp))
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
    eyebrow="PokéPilot"
    title="Spectator"
    subtitle=""
    mode="public"
    :show-intro="false"
  >
    <template #summary>
      <template v-if="snapshot">
        <span><strong class="text-white">{{ snapshot.summary.live }}</strong> live</span>
        <span><strong class="text-white">{{ snapshot.summary.queued }}</strong> queued</span>
        <span><strong class="text-white">{{ snapshot.summary.completed }}</strong> completed</span>
      </template>
    </template>

    <template #actions>
      <button
        v-if="selectedRun"
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
        <h2 class="mt-4 text-lg font-semibold text-white">Spectator feed reconnecting</h2>
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
      <div class="max-w-xl text-center">
        <span class="mx-auto block size-2.5 animate-pulse rounded-full bg-cyan-300" />
        <h2 class="mt-4 text-xl font-semibold text-white">No public run is live yet</h2>
        <p class="mt-2 text-sm leading-6 text-slate-400">The page is connected and will pick up the next run automatically.</p>
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

      <section class="mode-hero overflow-hidden rounded-xl border bg-[#0d131c] shadow-xl shadow-black/15">
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
              <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-500 uppercase">Party</div>
              <div class="mt-1 font-mono text-lg font-semibold text-white">{{ selectedRun.player?.party?.length || 0 }}/6</div>
            </div>
            <div class="bg-[#0b1119] px-3 py-2.5 text-center">
              <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-500 uppercase">Bag</div>
              <div class="mt-1 font-mono text-lg font-semibold text-white">{{ bagMeter(selectedRun.player) || '—' }}</div>
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
          <Panel title="Game" :description="isLiveRun(selectedRun) ? 'Live gameplay broadcast' : 'Public replay highlight'" compact>
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
              <video
                v-if="replayURL"
                :key="selectedRun.run_id"
                class="absolute inset-0 h-full w-full object-contain object-center [image-rendering:pixelated]"
                :src="replayURL"
                controls
                preload="metadata"
                playsinline
              />

              <img
                v-else-if="frameURL"
                :src="frameURL"
                :alt="`Live frame for ${selectedRun.run_id}`"
                class="absolute inset-0 h-full w-full object-contain object-center [image-rendering:pixelated]"
              />

              <div v-else class="absolute inset-0 grid place-items-center px-6 py-12 text-center">
                <div>
                  <div class="mx-auto flex size-12 items-center justify-center rounded-full border border-white/10 bg-white/5">
                    <span :class="['size-2.5 rounded-full', isLiveRun(selectedRun) ? 'animate-pulse bg-emerald-300' : 'bg-slate-600']" />
                  </div>
                  <p class="mt-3 text-sm font-medium text-slate-300">
                    {{ selectedRun.status === 'queued' ? 'Waiting for a worker' : selectedRun.status === 'done' ? 'Replay is not public for this run' : 'Waiting for a live frame' }}
                  </p>
                  <p v-if="frameState === 'error' && frameError" class="mt-1 text-xs text-amber-300/80">{{ frameError }}</p>
                </div>
              </div>

              <div class="pointer-events-none absolute inset-x-0 top-0 flex items-center justify-between bg-gradient-to-b from-black/65 to-transparent px-3 py-3 text-[10px] font-semibold tracking-[0.08em] uppercase">
                <span class="rounded bg-black/45 px-2 py-1 text-slate-200 ring-1 ring-white/10">{{ playStyleLabel(selectedRun) }}</span>
                <span class="rounded bg-black/45 px-2 py-1 font-mono text-slate-300 ring-1 ring-white/10">{{ locationLabel(selectedRun) }}</span>
              </div>

              <div class="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/80 via-black/45 to-transparent px-3 pb-3 pt-10 sm:px-4 sm:pb-4">
                <div class="flex items-end justify-between gap-4">
                  <div class="min-w-0">
                    <div class="text-[9px] font-semibold tracking-[0.1em] text-slate-400 uppercase">Latest decision</div>
                    <div class="mt-1 line-clamp-2 max-w-4xl text-xs leading-5 text-white sm:text-sm">{{ selectedRun.decision || 'Waiting for the next decision…' }}</div>
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
              <div v-for="(mon, index) in selectedRun.player.party" :key="`${mon.name}-${index}`" class="mode-party-card rounded-lg border bg-black/15 p-3">
                <div class="flex items-baseline justify-between gap-3">
                  <strong class="truncate text-sm text-white">{{ mon.name || 'Unknown' }}</strong>
                  <span class="font-mono text-[11px] text-slate-500">Lv {{ mon.level }}</span>
                </div>
                <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-white/8">
                  <div class="h-full rounded-full bg-emerald-400 transition-[width]" :style="{ width: `${hpPercent(selectedRun, index)}%` }" />
                </div>
                <div class="mt-1.5 flex justify-between font-mono text-[10px] text-slate-500">
                  <span>{{ mon.hp }}/{{ mon.max_hp }} HP</span>
                  <span>{{ mon.status || 'healthy' }}</span>
                </div>
              </div>
            </div>
            <p v-else class="py-5 text-center text-sm text-slate-500">Party data is not available yet.</p>

            <PartyProgress :run="selectedRun" />
          </Panel>

          <Panel title="Activity" description="A lightweight watch feed built from public run changes." compact>
            <div v-if="selectedActivity.length" class="divide-y divide-white/8">
              <div v-for="item in selectedActivity" :key="item.id" class="grid grid-cols-[4.5rem_minmax(0,1fr)] gap-3 py-2.5 first:pt-0 last:pb-0">
                <time class="font-mono text-[10px] text-slate-600">{{ activityTime(item) }}</time>
                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="mode-dot size-1.5 shrink-0 rounded-full" />
                    <strong class="text-xs text-slate-300">{{ item.label }}</strong>
                  </div>
                  <p class="mt-1 text-xs leading-5 text-slate-500">{{ item.detail }}</p>
                </div>
              </div>
            </div>
            <p v-else class="py-5 text-center text-xs text-slate-500">New decisions, areas, catches and badges will appear here.</p>
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

          <Panel title="Replay highlights" description="Completed public runs with a ready video." compact>
            <div v-if="groupedRuns.recent.length" class="space-y-1.5">
              <button
                v-for="run in groupedRuns.recent"
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
                <p class="mt-1 truncate text-[10px] text-slate-600">{{ shortRunID(run.run_id) }} · {{ routeLabel(run) }}</p>
              </button>
            </div>
            <p v-else class="py-4 text-center text-xs text-slate-500">No public replay highlights are available yet.</p>
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

.mode-party-card,
.mode-metric {
  border-color: var(--mode-border);
}

:fullscreen {
  background: #05070a;
}
</style>
