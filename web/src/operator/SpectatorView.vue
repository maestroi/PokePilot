<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ArrowPathIcon, ArrowTopRightOnSquareIcon, EyeIcon, EyeSlashIcon, StarIcon } from '@heroicons/vue/20/solid'
import { getDashboard, getMediaRenderJobs } from '../shared/api/client'
import {
  getOperatorUIConfig,
  getSpectatorControl,
  patchSpectatorRunControl
} from '../shared/api/spectator-control'
import { replayURL, resolveSpectatorBase, spectatorURL } from '../shared/urls'
import type { DashboardRun, MediaRenderJob } from '../shared/api/types'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { formatWhen, statusTone } from './operations'

const activeResource = usePollingResource(
  (signal) => getDashboard({ active: true }, signal),
  { intervalMs: 2000 }
)
const recentResource = usePollingResource(
  (signal) => getDashboard({ status: 'done', limit: 24 }, signal),
  { intervalMs: 10000 }
)
const controlResource = usePollingResource(
  (signal) => getSpectatorControl(signal),
  { intervalMs: 2000 }
)
const mediaResource = usePollingResource(
  (signal) => getMediaRenderJobs(100, signal),
  { intervalMs: 4000 }
)

const configuredSpectatorURL = ref('')
const actionError = ref('')
const busyRunID = ref('')

onMounted(async () => {
  try {
    configuredSpectatorURL.value = resolveSpectatorBase(await getOperatorUIConfig(), window.location.href)
  } catch {
    configuredSpectatorURL.value = resolveSpectatorBase({}, window.location.href)
  }
})

const runs = computed<DashboardRun[]>(() => {
  const seen = new Set<string>()
  return [...(activeResource.data.value?.runs || []), ...(recentResource.data.value?.runs || [])]
    .filter((run) => {
      if (!run.run_id || seen.has(run.run_id)) return false
      seen.add(run.run_id)
      return true
    })
    .sort((a, b) => {
      if (a.status !== 'done' && b.status === 'done') return -1
      if (a.status === 'done' && b.status !== 'done') return 1
      const aTime = Number(a.status === 'done' ? a.ended_at : a.queued_at) || 0
      const bTime = Number(b.status === 'done' ? b.ended_at : b.queued_at) || 0
      return bTime - aTime
    })
})

const featuredRunID = computed(() => controlResource.data.value?.featured_run_id || '')
const mediaJobByRun = computed(() => {
  const out = new Map<string, MediaRenderJob>()
  for (const job of mediaResource.data.value?.jobs || []) {
    const current = out.get(job.run_id)
    if (!current || Number(job.updated_at_unix_ms || 0) > Number(current.updated_at_unix_ms || 0)) {
      out.set(job.run_id, job)
    }
  }
  return out
})
const resourceState = computed(() => {
  if (controlResource.state.value === 'error') return 'error'
  if (activeResource.state.value === 'error' && recentResource.state.value === 'error') return 'error'
  if (controlResource.state.value === 'loading' || (activeResource.state.value === 'loading' && recentResource.state.value === 'loading')) return 'loading'
  if (!runs.value.length) return 'empty'
  if (controlResource.state.value === 'stale' || activeResource.state.value === 'stale') return 'stale'
  return 'ready'
})

function isVisible(runID: string): boolean {
  return controlResource.data.value?.runs?.[runID]?.visible !== false
}

function mediaJob(run: DashboardRun): MediaRenderJob | undefined {
  return mediaJobByRun.value.get(run.run_id)
}

function canFeature(run: DashboardRun): boolean {
  return run.status === 'running' || run.status === 'leased'
}

function showFeatureControl(run: DashboardRun): boolean {
  return canFeature(run) || featuredRunID.value === run.run_id
}

function visibilityLabel(run: DashboardRun): string {
  const visible = isVisible(run.run_id)
  if (run.status === 'queued') return visible ? 'Publish on start' : 'Keep private'
  if (run.status === 'done') {
    if (run.replay_available) return visible ? 'Replay public' : 'Replay private'
    return visible ? 'Publish replay' : 'Keep private'
  }
  if (run.status === 'paused') return visible ? 'Public on resume' : 'Keep private'
  return visible ? 'Public live' : 'Private live'
}

function visibilityTitle(run: DashboardRun): string {
  const visible = isVisible(run.run_id)
  if (run.status === 'queued') {
    return visible ? 'This run will become public when it starts. Click to keep it private.' : 'Keep this run private when it starts. Click to publish on start.'
  }
  if (run.status === 'done') {
    if (run.replay_available) return visible ? 'This replay is published on the public site. Click to make it private.' : 'This replay is private. Click to publish it.'
    return visible ? 'Publish this replay automatically when media becomes available.' : 'Keep this completed run and any future replay private.'
  }
  return visible ? 'This live run is exposed on the public site. Click to hide it.' : 'This live run is private. Click to publish it.'
}

function renderPercent(job?: MediaRenderJob): number | null {
  if (!job) return null
  const total = Number(job.segments_total || 0)
  const done = Number(job.segments_done || 0)
  if (total <= 0) return null
  return Math.max(0, Math.min(100, Math.round(done * 100 / total)))
}

function renderStateLabel(run: DashboardRun): string {
  const job = mediaJob(run)
  if (!job) return run.status === 'done' && !run.replay_available ? 'No replay' : ''
  if (job.state === 'ready') return run.replay_available ? 'Replay ready' : 'Replay syncing'
  if (job.state === 'failed') return 'Render failed'
  if (job.state === 'cancelled') return 'Render cancelled'
  if (job.state === 'queued') return 'Render queued'
  const percent = renderPercent(job)
  return percent === null ? 'Rendering' : `Rendering ${percent}%`
}

function canOpenPublic(run: DashboardRun): boolean {
  if (!isVisible(run.run_id)) return false
  if (run.status === 'done') return Boolean(run.replay_available)
  return run.status !== 'queued'
}

function publicActionLabel(run: DashboardRun): string {
  if (!isVisible(run.run_id)) return run.status === 'done' ? 'Private replay' : 'Private'
  if (run.status === 'queued') return 'Not live'
  if (run.status === 'done') return run.replay_available ? 'Open replay' : renderStateLabel(run)
  return 'Open live'
}

function spectatorBaseURL(): string {
  return configuredSpectatorURL.value
}

function openSpectator(runID = ''): void {
  const base = spectatorBaseURL()
  if (!base) {
    actionError.value = 'Set POKEPILOT_PUBLIC_BASE_URL on the operator UI to enable public links.'
    return
  }
  window.open(spectatorURL(base, runID), '_blank', 'noopener,noreferrer')
}

function openPublicRun(run: DashboardRun): void {
  if (!canOpenPublic(run)) return
  const base = spectatorBaseURL()
  if (!base) {
    actionError.value = 'Set POKEPILOT_PUBLIC_BASE_URL on the operator UI to enable public links.'
    return
  }
  const url = run.status === 'done'
    ? replayURL(base, run.run_id)
    : spectatorURL(base, run.run_id)
  window.open(url, '_blank', 'noopener,noreferrer')
}

async function refresh(): Promise<void> {
  actionError.value = ''
  await Promise.allSettled([
    activeResource.retry(),
    recentResource.retry(),
    controlResource.retry(),
    mediaResource.retry()
  ])
}

async function setVisible(run: DashboardRun, visible: boolean): Promise<void> {
  if (busyRunID.value) return
  busyRunID.value = run.run_id
  actionError.value = ''
  try {
    await patchSpectatorRunControl(run.run_id, { visible })
    await controlResource.retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Public visibility update failed'
  } finally {
    busyRunID.value = ''
  }
}

async function setFeatured(run: DashboardRun, featured: boolean): Promise<void> {
  if (busyRunID.value) return
  busyRunID.value = run.run_id
  actionError.value = ''
  try {
    await patchSpectatorRunControl(run.run_id, { featured })
    await controlResource.retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Public visibility update failed'
  } finally {
    busyRunID.value = ''
  }
}
</script>

<template>
  <ResourceState
    :state="resourceState"
    title="Public publishing controls unavailable"
    :message="controlResource.error.value || activeResource.error.value || recentResource.error.value || 'No runs are available yet.'"
    :rows="7"
  >
    <template #actions>
      <button type="button" class="inline-flex items-center gap-1.5 rounded-sm bg-white/10 px-2 py-1 text-[11px] font-semibold text-white ring-1 ring-white/10 hover:bg-white/15" @click="refresh">
        <ArrowPathIcon class="size-3.5" aria-hidden="true" /> Refresh
      </button>
    </template>

    <div class="space-y-2">
      <section class="flex flex-wrap items-center justify-between gap-3 border border-[var(--poke-border)] bg-[var(--poke-panel)] px-3 py-2.5">
        <div>
          <h2 class="text-sm font-semibold text-white">Public site</h2>
          <p class="mt-0.5 text-[11px] text-[var(--poke-muted)]">
            Visibility controls live broadcasts and completed replay publication. Feature applies only to currently live runs.
          </p>
        </div>
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-sm bg-[var(--poke-cyan)] px-2.5 py-1.5 text-[11px] font-bold text-[#101820] hover:brightness-110"
          @click="openSpectator()"
        >
          <ArrowTopRightOnSquareIcon class="size-3.5" aria-hidden="true" /> Open public site
        </button>
      </section>

      <div v-if="actionError" class="border border-[#654047] bg-[#352529] px-2.5 py-2 text-[12px] text-[#e4b5b7]" role="alert">{{ actionError }}</div>

      <section class="overflow-hidden border border-[var(--poke-border)] bg-[var(--poke-panel)]">
        <div class="hidden grid-cols-[minmax(10rem,1.35fr)_7rem_8rem_minmax(12rem,1fr)_auto] gap-2 border-b border-[var(--poke-border)] bg-[#0f141c] px-2.5 py-1.5 text-[9px] tracking-[0.07em] text-[var(--poke-muted)] uppercase lg:grid">
          <span>Run</span>
          <span>Status</span>
          <span>When</span>
          <span>Goal</span>
          <span class="text-right">Public</span>
        </div>

        <div
          v-for="run in runs"
          :key="run.run_id"
          class="grid grid-cols-1 gap-3 border-b border-[var(--poke-border)] px-3 py-3 last:border-b-0 lg:grid-cols-[minmax(10rem,1.35fr)_7rem_8rem_minmax(12rem,1fr)_auto] lg:items-center lg:gap-2 lg:px-2.5 lg:py-2"
        >
          <div class="min-w-0">
            <div class="flex items-center gap-1.5">
              <strong class="truncate font-mono text-[11px] text-white" :title="run.run_id">{{ run.run_id }}</strong>
              <StatusBadge v-if="featuredRunID === run.run_id" tone="success">featured</StatusBadge>
            </div>
            <span class="mt-0.5 block truncate text-[10px] text-[var(--poke-muted)]">{{ run.starter || 'Pokémon Red' }}</span>
          </div>

          <div class="flex items-center justify-between gap-3 lg:block">
            <span class="text-[9px] tracking-[0.07em] text-[var(--poke-muted)] uppercase lg:hidden">Status</span>
            <StatusBadge :tone="statusTone(run.status)">{{ run.status }}</StatusBadge>
          </div>
          <div class="flex items-center justify-between gap-3 lg:block">
            <span class="text-[9px] tracking-[0.07em] text-[var(--poke-muted)] uppercase lg:hidden">When</span>
            <span class="font-mono text-[10px] text-[var(--poke-muted)]">{{ formatWhen(run.status === 'done' ? run.ended_at : run.queued_at) }}</span>
          </div>
          <div class="flex min-w-0 items-center justify-between gap-3 lg:block">
            <span class="shrink-0 text-[9px] tracking-[0.07em] text-[var(--poke-muted)] uppercase lg:hidden">Goal</span>
            <span class="truncate text-[11px] text-[var(--poke-text)]" :title="run.goal || ''">{{ run.goal || 'Free play' }}</span>
          </div>

          <div class="flex flex-wrap items-center justify-start gap-1 lg:justify-end">
            <button
              type="button"
              :disabled="Boolean(busyRunID)"
              :class="[
                isVisible(run.run_id) ? 'text-[var(--poke-green)] ring-[var(--poke-green)]/40' : 'text-[var(--poke-muted)] ring-[var(--poke-border-strong)]',
                'inline-flex items-center gap-1 rounded-sm px-1.5 py-1 text-[10px] font-bold ring-1 hover:bg-white/5 disabled:opacity-50'
              ]"
              :title="visibilityTitle(run)"
              @click="setVisible(run, !isVisible(run.run_id))"
            >
              <EyeIcon v-if="isVisible(run.run_id)" class="size-3" aria-hidden="true" />
              <EyeSlashIcon v-else class="size-3" aria-hidden="true" />
              {{ visibilityLabel(run) }}
            </button>

            <button
              v-if="showFeatureControl(run)"
              type="button"
              :disabled="Boolean(busyRunID) || (!canFeature(run) && featuredRunID !== run.run_id)"
              :class="[
                featuredRunID === run.run_id ? 'bg-[#31402e] text-[var(--poke-green)] ring-[#52664c]' : 'text-[var(--poke-text)] ring-[var(--poke-border-strong)]',
                'inline-flex items-center gap-1 rounded-sm px-1.5 py-1 text-[10px] font-bold ring-1 hover:bg-white/5 disabled:opacity-50'
              ]"
              :title="featuredRunID === run.run_id ? 'Stop featuring this run' : 'Feature this live run as the default public watch view'"
              @click="setFeatured(run, featuredRunID !== run.run_id)"
            >
              <StarIcon class="size-3" aria-hidden="true" />
              {{ featuredRunID === run.run_id ? (canFeature(run) ? 'Featured' : 'Clear feature') : 'Feature live' }}
            </button>

            <button
              type="button"
              :disabled="!canOpenPublic(run)"
              :class="[
                canOpenPublic(run) ? 'text-[var(--poke-cyan)] hover:bg-white/5' : 'cursor-not-allowed text-[var(--poke-muted)] opacity-60',
                'inline-flex items-center gap-1 rounded-sm px-1.5 py-1 text-[10px] font-bold ring-1 ring-[var(--poke-border-strong)]'
              ]"
              :title="canOpenPublic(run) ? (run.status === 'done' ? 'Open the published replay' : 'Open this live public run') : publicActionLabel(run)"
              @click="openPublicRun(run)"
            >
              <ArrowTopRightOnSquareIcon v-if="canOpenPublic(run)" class="size-3" aria-hidden="true" />
              <ArrowPathIcon v-else-if="mediaJob(run) && ['queued', 'preparing', 'rendering', 'assembling', 'uploading'].includes(mediaJob(run)?.state || '')" class="size-3" aria-hidden="true" />
              {{ publicActionLabel(run) }}
            </button>
          </div>
        </div>
      </section>
    </div>
  </ResourceState>
</template>
