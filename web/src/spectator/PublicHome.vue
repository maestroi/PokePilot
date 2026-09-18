<script setup lang="ts">
import { computed } from 'vue'
import {
  ArrowRightIcon,
  BoltIcon,
  BugAntIcon,
  GlobeAltIcon,
  PlayIcon,
  QueueListIcon,
  SignalIcon,
  SparklesIcon,
  TrophyIcon
} from '@heroicons/vue/20/solid'
import type { SpectatorRun, SpectatorSummary } from '../shared/api/spectator'
import {
  goalProgress,
  isLiveRun,
  locationLabel,
  objectiveLabel,
  playSpeedLabel,
  playStyleLabel,
  routeLabel,
  runStatusLabel,
  runTitle
} from './model'

const props = defineProps<{
  run: SpectatorRun
  liveRuns: SpectatorRun[]
  recentRuns: SpectatorRun[]
  summary: SpectatorSummary
  frameURL: string
  replayURL: string
}>()

const emit = defineEmits<{
  select: [run: SpectatorRun]
}>()

const badges = computed(() => props.run.player?.badges?.length || 0)
const party = computed(() => props.run.player?.party?.length || 0)
const replayCount = computed(() => props.recentRuns.filter((run) => run.replay_ready).length)
const visibleLiveRuns = computed(() => props.liveRuns.filter((run) => isLiveRun(run)).slice(0, 5))
const goal = computed(() => goalProgress(props.run))
const headlineAccent = computed(() => isLiveRun(props.run) ? 'live.' : props.run.status === 'done' ? 'on replay.' : 'in progress.')

function watch(run: SpectatorRun): void {
  emit('select', run)
}
</script>

<template>
  <section class="public-home relative isolate overflow-hidden rounded-[1.75rem] border border-white/10 bg-[#060b16] shadow-2xl shadow-black/35">
    <div class="public-home-grid pointer-events-none absolute inset-0 opacity-30" />
    <div class="pointer-events-none absolute -left-36 top-8 size-[34rem] rounded-full bg-blue-500/10 blur-3xl" />
    <div class="pointer-events-none absolute -right-24 -top-24 size-[32rem] rounded-full bg-violet-500/12 blur-3xl" />
    <div class="pointer-events-none absolute bottom-0 left-1/3 size-[24rem] rounded-full bg-cyan-400/8 blur-3xl" />

    <div class="relative grid gap-8 px-5 pb-6 pt-7 sm:px-7 sm:pt-9 xl:grid-cols-[minmax(0,0.92fr)_minmax(42rem,1.08fr)] xl:items-center xl:px-10 xl:pb-9 xl:pt-11">
      <div class="max-w-3xl">
        <div class="inline-flex items-center gap-2 rounded-full border border-cyan-300/15 bg-cyan-300/8 px-3 py-1.5 text-[10px] font-bold tracking-[0.14em] text-cyan-200 uppercase">
          <SparklesIcon class="size-3.5" aria-hidden="true" />
          Games play themselves. You watch the story.
        </div>

        <h1 class="mt-5 max-w-3xl text-4xl font-black tracking-[-0.045em] text-white sm:text-5xl lg:text-6xl xl:text-[4.4rem] xl:leading-[0.98]">
          Watch AI play games,
          <span class="public-home-accent block">{{ headlineAccent }}</span>
        </h1>

        <p class="mt-5 max-w-2xl text-sm leading-7 text-slate-300 sm:text-base">
          RomPilot runs game playthroughs with autonomous agents and broadcasts the interesting parts live:
          decisions, progression, party state, objectives, world position, and the mistakes that make every run different.
        </p>

        <div class="mt-6 flex flex-wrap gap-3">
          <button
            type="button"
            class="group inline-flex items-center gap-2 rounded-xl bg-gradient-to-r from-cyan-400 via-blue-500 to-violet-500 px-5 py-3 text-sm font-black text-white shadow-lg shadow-blue-500/20 transition hover:-translate-y-0.5 hover:brightness-110"
            @click="watch(run)"
          >
            <PlayIcon class="size-4" aria-hidden="true" />
            {{ isLiveRun(run) ? 'Watch live' : 'Watch featured replay' }}
            <ArrowRightIcon class="size-4 transition-transform group-hover:translate-x-0.5" aria-hidden="true" />
          </button>
          <a
            href="/explore"
            class="inline-flex items-center gap-2 rounded-xl border border-white/12 bg-white/6 px-5 py-3 text-sm font-bold text-slate-100 transition hover:border-cyan-300/25 hover:bg-white/10"
          >
            <GlobeAltIcon class="size-4 text-cyan-300" aria-hidden="true" />
            Explore the world
          </a>
        </div>

        <div class="mt-7 flex flex-wrap items-center gap-x-5 gap-y-2 text-[11px] text-slate-500">
          <span class="inline-flex items-center gap-1.5">
            <SignalIcon class="size-3.5 text-emerald-300" aria-hidden="true" />
            {{ summary.live }} live now
          </span>
          <span>{{ summary.completed }} completed public runs</span>
          <a href="/replays" class="font-semibold text-slate-300 hover:text-white">Replay library →</a>
        </div>
      </div>

      <div class="relative">
        <div class="absolute -inset-3 rounded-[2rem] bg-gradient-to-r from-cyan-400/18 via-blue-500/20 to-violet-500/18 blur-xl" />
        <div class="relative overflow-hidden rounded-[1.35rem] border border-cyan-300/20 bg-[#08111f]/95 p-3 shadow-2xl shadow-blue-950/50 ring-1 ring-white/8 sm:p-4">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
            <div class="flex min-w-0 items-center gap-2.5">
              <span
                :class="[
                  'inline-flex shrink-0 items-center gap-1.5 rounded-full px-2.5 py-1 text-[10px] font-black tracking-[0.12em] uppercase ring-1',
                  isLiveRun(run)
                    ? 'bg-emerald-400/12 text-emerald-200 ring-emerald-300/25'
                    : 'bg-violet-400/12 text-violet-200 ring-violet-300/25'
                ]"
              >
                <span :class="['size-1.5 rounded-full', isLiveRun(run) ? 'animate-pulse bg-emerald-300' : 'bg-violet-300']" />
                {{ isLiveRun(run) ? 'Live' : 'Featured' }}
              </span>
              <div class="min-w-0">
                <div class="truncate text-sm font-extrabold text-white">{{ runTitle(run) }}</div>
                <div class="truncate text-[10px] text-slate-500">{{ routeLabel(run) }}</div>
              </div>
            </div>
            <div class="shrink-0 rounded-lg border border-white/8 bg-black/20 px-2.5 py-1.5 font-mono text-[10px] font-bold text-cyan-200">
              {{ playSpeedLabel(run) }}
            </div>
          </div>

          <div class="grid gap-3 lg:grid-cols-[minmax(0,1.35fr)_minmax(16rem,0.65fr)]">
            <div class="relative min-h-[18rem] overflow-hidden rounded-xl border border-white/10 bg-black/55 shadow-inner sm:min-h-[22rem]">
              <video
                v-if="replayURL"
                :key="run.run_id"
                :src="replayURL"
                class="absolute inset-0 h-full w-full object-contain object-center [image-rendering:pixelated]"
                controls
                preload="metadata"
                playsinline
              />
              <img
                v-else-if="frameURL"
                :src="frameURL"
                :alt="`Live frame for ${run.run_id}`"
                class="absolute inset-0 h-full w-full object-contain object-center [image-rendering:pixelated]"
              />
              <div v-else class="absolute inset-0 grid place-items-center">
                <div class="text-center">
                  <div class="mx-auto flex size-12 items-center justify-center rounded-2xl border border-white/10 bg-white/5">
                    <PlayIcon class="size-5 text-cyan-300" aria-hidden="true" />
                  </div>
                  <p class="mt-3 text-xs font-semibold text-slate-400">
                    {{ run.status === 'queued' ? 'Waiting for a worker' : 'Waiting for the next public frame' }}
                  </p>
                </div>
              </div>

              <div class="pointer-events-none absolute inset-x-0 top-0 flex items-center justify-between bg-gradient-to-b from-black/70 to-transparent px-3 py-3">
                <span class="rounded-md bg-black/55 px-2 py-1 text-[9px] font-bold tracking-[0.08em] text-white uppercase ring-1 ring-white/10">{{ playStyleLabel(run) }}</span>
                <span class="rounded-md bg-black/55 px-2 py-1 font-mono text-[9px] text-slate-300 ring-1 ring-white/10">{{ locationLabel(run) }}</span>
              </div>
              <div class="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/85 via-black/45 to-transparent px-3 pb-3 pt-12">
                <div class="text-[9px] font-bold tracking-[0.1em] text-slate-400 uppercase">Latest decision</div>
                <div class="mt-1 line-clamp-2 text-xs font-semibold leading-5 text-white">
                  {{ run.decision || 'The agent is preparing its next move…' }}
                </div>
              </div>
            </div>

            <div class="flex flex-col gap-3">
              <div class="rounded-xl border border-white/8 bg-white/[0.035] p-3.5">
                <div class="flex items-center justify-between gap-3">
                  <span class="text-[9px] font-bold tracking-[0.12em] text-slate-500 uppercase">Current objective</span>
                  <span class="font-mono text-[10px] text-cyan-200">{{ goal.toFixed(0) }}%</span>
                </div>
                <p class="mt-2 line-clamp-3 text-xs font-semibold leading-5 text-slate-200">{{ objectiveLabel(run) }}</p>
                <div class="mt-3 h-1.5 overflow-hidden rounded-full bg-white/8">
                  <div class="h-full rounded-full bg-gradient-to-r from-cyan-300 via-blue-400 to-violet-400" :style="{ width: `${goal}%` }" />
                </div>
              </div>

              <div class="grid grid-cols-2 gap-2">
                <div class="rounded-xl border border-white/8 bg-white/[0.035] p-3">
                  <TrophyIcon class="size-4 text-amber-300" aria-hidden="true" />
                  <div class="mt-2 font-mono text-xl font-black text-white">{{ badges }}</div>
                  <div class="text-[9px] font-bold tracking-[0.1em] text-slate-500 uppercase">Badges</div>
                </div>
                <div class="rounded-xl border border-white/8 bg-white/[0.035] p-3">
                  <QueueListIcon class="size-4 text-violet-300" aria-hidden="true" />
                  <div class="mt-2 font-mono text-xl font-black text-white">{{ party }}/6</div>
                  <div class="text-[9px] font-bold tracking-[0.1em] text-slate-500 uppercase">Party</div>
                </div>
                <div class="rounded-xl border border-white/8 bg-white/[0.035] p-3">
                  <BoltIcon class="size-4 text-cyan-300" aria-hidden="true" />
                  <div class="mt-2 font-mono text-xl font-black text-white">{{ run.stats?.round ?? 0 }}</div>
                  <div class="text-[9px] font-bold tracking-[0.1em] text-slate-500 uppercase">Rounds</div>
                </div>
                <div class="rounded-xl border border-white/8 bg-white/[0.035] p-3">
                  <SignalIcon class="size-4 text-emerald-300" aria-hidden="true" />
                  <div class="mt-2 font-mono text-xl font-black text-white">{{ runStatusLabel(run) }}</div>
                  <div class="text-[9px] font-bold tracking-[0.1em] text-slate-500 uppercase">Status</div>
                </div>
              </div>

              <button
                type="button"
                class="mt-auto inline-flex items-center justify-center gap-2 rounded-xl border border-cyan-300/15 bg-cyan-300/8 px-4 py-2.5 text-xs font-extrabold text-cyan-100 transition hover:bg-cyan-300/14"
                @click="watch(run)"
              >
                Open run details
                <ArrowRightIcon class="size-3.5" aria-hidden="true" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="relative grid grid-cols-2 border-y border-white/8 bg-[#07101d]/80 sm:grid-cols-4">
      <div class="public-stat">
        <SignalIcon class="size-5 text-emerald-300" aria-hidden="true" />
        <div>
          <strong>{{ summary.live }}</strong>
          <span>Live runs</span>
        </div>
      </div>
      <div class="public-stat">
        <TrophyIcon class="size-5 text-amber-300" aria-hidden="true" />
        <div>
          <strong>{{ summary.completed }}</strong>
          <span>Completed</span>
        </div>
      </div>
      <div class="public-stat">
        <QueueListIcon class="size-5 text-blue-300" aria-hidden="true" />
        <div>
          <strong>{{ summary.queued }}</strong>
          <span>Queued</span>
        </div>
      </div>
      <div class="public-stat">
        <PlayIcon class="size-5 text-violet-300" aria-hidden="true" />
        <div>
          <strong>{{ replayCount }}</strong>
          <span>Recent replays</span>
        </div>
      </div>
    </div>

    <div class="relative px-5 py-6 sm:px-7 xl:px-10">
      <div class="flex flex-wrap items-end justify-between gap-3">
        <div>
          <div class="flex items-center gap-2">
            <span class="size-2 animate-pulse rounded-full bg-emerald-300 shadow-[0_0_18px_rgba(110,231,183,.8)]" />
            <h2 class="text-lg font-black tracking-tight text-white">Live now</h2>
          </div>
          <p class="mt-1 text-xs text-slate-500">Jump between autonomous runs without leaving the spectator.</p>
        </div>
        <a href="/replays" class="text-xs font-bold text-cyan-200 hover:text-white">Browse completed runs →</a>
      </div>

      <div v-if="visibleLiveRuns.length" class="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-5">
        <button
          v-for="candidate in visibleLiveRuns"
          :key="candidate.run_id"
          type="button"
          :class="[
            'group min-w-0 overflow-hidden rounded-xl border p-3 text-left transition',
            candidate.run_id === run.run_id
              ? 'border-cyan-300/30 bg-cyan-300/8 shadow-lg shadow-cyan-950/25'
              : 'border-white/8 bg-white/[0.035] hover:-translate-y-0.5 hover:border-white/16 hover:bg-white/[0.055]'
          ]"
          @click="watch(candidate)"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="inline-flex items-center gap-1.5 text-[9px] font-black tracking-[0.1em] text-emerald-200 uppercase">
              <span class="size-1.5 animate-pulse rounded-full bg-emerald-300" />
              Live
            </span>
            <span class="font-mono text-[9px] text-slate-600">{{ playSpeedLabel(candidate) }}</span>
          </div>
          <div class="mt-3 line-clamp-2 text-xs font-extrabold leading-5 text-white">{{ runTitle(candidate) }}</div>
          <div class="mt-1 truncate text-[10px] text-slate-500">{{ routeLabel(candidate) }}</div>
          <div class="mt-3 flex items-center justify-between gap-2 text-[9px] text-slate-500">
            <span>{{ candidate.player?.badges?.length || 0 }} badges</span>
            <ArrowRightIcon class="size-3 text-slate-600 transition group-hover:translate-x-0.5 group-hover:text-cyan-200" aria-hidden="true" />
          </div>
        </button>
      </div>
      <div v-else class="mt-4 rounded-xl border border-white/8 bg-white/[0.025] p-5 text-center text-xs text-slate-500">
        No run is broadcasting right now. The featured replay stays available while RomPilot waits for the next run.
      </div>

      <div class="mt-6 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <a href="/explore" class="public-feature group">
          <GlobeAltIcon class="size-5 text-cyan-300" aria-hidden="true" />
          <div>
            <strong>World explorer</strong>
            <span>Follow the map, routes, objects and live position.</span>
          </div>
          <ArrowRightIcon class="ml-auto size-4 text-slate-600 transition group-hover:translate-x-0.5 group-hover:text-cyan-200" aria-hidden="true" />
        </a>
        <a href="/replays" class="public-feature group">
          <PlayIcon class="size-5 text-violet-300" aria-hidden="true" />
          <div>
            <strong>Replay library</strong>
            <span>Watch completed public runs again from the start.</span>
          </div>
          <ArrowRightIcon class="ml-auto size-4 text-slate-600 transition group-hover:translate-x-0.5 group-hover:text-violet-200" aria-hidden="true" />
        </a>
        <div class="public-feature">
          <BoltIcon class="size-5 text-blue-300" aria-hidden="true" />
          <div>
            <strong>Autonomous runs</strong>
            <span>Goals, policies and decisions evolve while the game runs.</span>
          </div>
        </div>
        <div class="public-feature">
          <BugAntIcon class="size-5 text-rose-300" aria-hidden="true" />
          <div>
            <strong>Emergent moments</strong>
            <span>Progress, recoveries and failures become part of the story.</span>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.public-home {
  --home-cyan: #5ee7ff;
  --home-blue: #5d7cff;
  --home-violet: #9f6bff;
}

.public-home-grid {
  background-image:
    linear-gradient(rgba(148, 163, 184, 0.055) 1px, transparent 1px),
    linear-gradient(90deg, rgba(148, 163, 184, 0.055) 1px, transparent 1px);
  background-size: 42px 42px;
  mask-image: linear-gradient(to bottom, black 0%, black 58%, transparent 100%);
}

.public-home-accent {
  background: linear-gradient(90deg, var(--home-cyan), #62a8ff 47%, #c283ff 78%, #78f5bd);
  background-clip: text;
  color: transparent;
  text-shadow: 0 0 38px rgba(93, 124, 255, 0.18);
}

.public-stat {
  display: flex;
  min-height: 5.25rem;
  align-items: center;
  justify-content: center;
  gap: 0.8rem;
  padding: 1rem;
  border-right: 1px solid rgba(255, 255, 255, 0.07);
}

.public-stat:nth-child(4) {
  border-right: 0;
}

.public-stat strong,
.public-stat span {
  display: block;
}

.public-stat strong {
  color: white;
  font-family: ui-monospace, "SFMono-Regular", Consolas, monospace;
  font-size: 1.05rem;
  font-weight: 800;
}

.public-stat span {
  margin-top: 0.1rem;
  color: #64748b;
  font-size: 0.625rem;
  font-weight: 700;
  letter-spacing: 0.07em;
  text-transform: uppercase;
}

.public-feature {
  display: flex;
  min-height: 5.6rem;
  align-items: flex-start;
  gap: 0.85rem;
  border: 1px solid rgba(255, 255, 255, 0.075);
  border-radius: 0.9rem;
  background: rgba(255, 255, 255, 0.028);
  padding: 1rem;
  transition: 160ms ease;
}

a.public-feature:hover {
  transform: translateY(-2px);
  border-color: rgba(94, 231, 255, 0.18);
  background: rgba(255, 255, 255, 0.045);
}

.public-feature strong,
.public-feature span {
  display: block;
}

.public-feature strong {
  color: #f8fafc;
  font-size: 0.75rem;
  font-weight: 800;
}

.public-feature span {
  margin-top: 0.3rem;
  color: #64748b;
  font-size: 0.675rem;
  line-height: 1.45;
}

@media (max-width: 639px) {
  .public-stat {
    border-bottom: 1px solid rgba(255, 255, 255, 0.07);
  }

  .public-stat:nth-child(2) {
    border-right: 0;
  }

  .public-stat:nth-child(3),
  .public-stat:nth-child(4) {
    border-bottom: 0;
  }
}
</style>
