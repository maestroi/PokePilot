<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ArrowPathIcon, ArrowTopRightOnSquareIcon, EyeIcon, EyeSlashIcon, StarIcon } from '@heroicons/vue/20/solid'
import { getDashboard } from '../shared/api/client'
import {
  getOperatorUIConfig,
  getSpectatorControl,
  patchSpectatorRunControl
} from '../shared/api/spectator-control'
import { resolveSpectatorBase, spectatorURL } from '../shared/urls'
import type { DashboardRun } from '../shared/api/types'
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

function spectatorBaseURL(): string {
  return configuredSpectatorURL.value
}

function openSpectator(runID = ''): void {
  const base = spectatorBaseURL()
  if (!base) {
    actionError.value = 'Set POKEPILOT_PUBLIC_BASE_URL on the operator UI to enable spectator links.'
    return
  }
  window.open(spectatorURL(base, runID), '_blank', 'noopener,noreferrer')
}

async function refresh(): Promise<void> {
  actionError.value = ''
  await Promise.allSettled([
    activeResource.retry(),
    recentResource.retry(),
    controlResource.retry()
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
    actionError.value = cause instanceof Error ? cause.message : 'Spectator update failed'
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
    actionError.value = cause instanceof Error ? cause.message : 'Spectator update failed'
  } finally {
    busyRunID.value = ''
  }
}
</script>

<template>
  <ResourceState
    :state="resourceState"
    title="Spectator controls unavailable"
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
            Hidden runs never cross the public RomPilot API. Featured runs become the default view for unpinned spectators.
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
        <div class="grid grid-cols-[minmax(10rem,1.35fr)_7rem_8rem_minmax(12rem,1fr)_auto] gap-2 border-b border-[var(--poke-border)] bg-[#0f141c] px-2.5 py-1.5 text-[9px] tracking-[0.07em] text-[var(--poke-muted)] uppercase">
          <span>Run</span>
          <span>Status</span>
          <span>When</span>
          <span>Goal</span>
          <span class="text-right">Spectator</span>
        </div>

        <div
          v-for="run in runs"
          :key="run.run_id"
          class="grid grid-cols-[minmax(10rem,1.35fr)_7rem_8rem_minmax(12rem,1fr)_auto] items-center gap-2 border-b border-[var(--poke-border)] px-2.5 py-2 last:border-b-0"
        >
          <div class="min-w-0">
            <div class="flex items-center gap-1.5">
              <strong class="truncate font-mono text-[11px] text-white" :title="run.run_id">{{ run.run_id }}</strong>
              <StatusBadge v-if="featuredRunID === run.run_id" tone="success">featured</StatusBadge>
            </div>
            <span class="mt-0.5 block truncate text-[10px] text-[var(--poke-muted)]">{{ run.starter || 'Pokémon Red' }}</span>
          </div>

          <div><StatusBadge :tone="statusTone(run.status)">{{ run.status }}</StatusBadge></div>
          <span class="font-mono text-[10px] text-[var(--poke-muted)]">{{ formatWhen(run.status === 'done' ? run.ended_at : run.queued_at) }}</span>
          <span class="truncate text-[11px] text-[var(--poke-text)]" :title="run.goal || ''">{{ run.goal || 'Free play' }}</span>

          <div class="flex items-center justify-end gap-1">
            <button
              type="button"
              :disabled="Boolean(busyRunID)"
              :class="[
                isVisible(run.run_id) ? 'text-[var(--poke-green)] ring-[var(--poke-green)]/40' : 'text-[var(--poke-muted)] ring-[var(--poke-border-strong)]',
                'inline-flex items-center gap-1 rounded-sm px-1.5 py-1 text-[10px] font-bold ring-1 hover:bg-white/5 disabled:opacity-50'
              ]"
              :title="isVisible(run.run_id) ? 'Hide this run from the public spectator' : 'Show this run in the public spectator'"
              @click="setVisible(run, !isVisible(run.run_id))"
            >
              <EyeIcon v-if="isVisible(run.run_id)" class="size-3" aria-hidden="true" />
              <EyeSlashIcon v-else class="size-3" aria-hidden="true" />
              {{ isVisible(run.run_id) ? 'Visible' : 'Hidden' }}
            </button>

            <button
              type="button"
              :disabled="Boolean(busyRunID)"
              :class="[
                featuredRunID === run.run_id ? 'bg-[#31402e] text-[var(--poke-green)] ring-[#52664c]' : 'text-[var(--poke-text)] ring-[var(--poke-border-strong)]',
                'inline-flex items-center gap-1 rounded-sm px-1.5 py-1 text-[10px] font-bold ring-1 hover:bg-white/5 disabled:opacity-50'
              ]"
              :title="featuredRunID === run.run_id ? 'Stop featuring this run' : 'Make this the default spectator run'"
              @click="setFeatured(run, featuredRunID !== run.run_id)"
            >
              <StarIcon class="size-3" aria-hidden="true" />
              {{ featuredRunID === run.run_id ? 'Featured' : 'Feature' }}
            </button>

            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-sm px-1.5 py-1 text-[10px] font-bold text-[var(--poke-cyan)] ring-1 ring-[var(--poke-border-strong)] hover:bg-white/5"
              @click="openSpectator(run.run_id)"
            >
              <ArrowTopRightOnSquareIcon class="size-3" aria-hidden="true" /> View spectator
            </button>
          </div>
        </div>
      </section>
    </div>
  </ResourceState>
</template>
