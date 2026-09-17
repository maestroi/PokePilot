<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ArrowTopRightOnSquareIcon, PlayIcon } from '@heroicons/vue/20/solid'
import { getBuildProvenance, getDashboard } from '../shared/api/client'
import { getOperatorUIConfig } from '../shared/api/spectator-control'
import AppShell from '../shared/components/AppShell.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import type { AppNavItem } from '../shared/types'
import AnalyticsView from './AnalyticsView.vue'
import FailuresView from './FailuresView.vue'
import LiveView from './LiveView.vue'
import OperationsView from './OperationsView.vue'
import RunArchiveView from './RunArchiveView.vue'
import SpectatorView from './SpectatorView.vue'
import ToolsView from './ToolsView.vue'

const views = ['live', 'runs', 'failures', 'analytics', 'operations', 'spectator', 'tools'] as const
type OperatorView = typeof views[number]

const labels: Record<OperatorView, string> = {
  live: 'Live',
  runs: 'Runs',
  failures: 'Failures',
  analytics: 'Analytics',
  operations: 'Operations',
  spectator: 'Spectator',
  tools: 'Tools'
}

const descriptions: Record<OperatorView, string> = {
  live: 'Watch a selected run, inspect game state and semantic position, and open its persisted evidence and deterministic replay.',
  runs: 'Search completed runs, then bulk-delete older history or every run that still matches one failure.',
  failures: 'Group, inspect, investigate, and delete finished runs for a specific failure — including issues that are already solved.',
  analytics: 'Farm outcomes, badge progress, LLM workload, and endless-run experiments.',
  operations: 'Fleet health, workers, LLM deployments, paired experiments, and recent outcomes.',
  spectator: 'Choose which runs are public, feature one as the default audience view, or open a run directly in spectator mode.',
  tools: 'Queue a new scripted or goal-driven run.'
}

const eyebrows: Record<OperatorView, string> = {
  live: 'Monitor',
  runs: 'Archive',
  failures: 'Triage',
  analytics: 'Telemetry',
  operations: 'System',
  spectator: 'Audience',
  tools: 'Control'
}

function hashView(): OperatorView {
  const value = window.location.hash.replace(/^#/, '') as OperatorView
  return views.includes(value) ? value : 'live'
}

const activeView = ref<OperatorView>(hashView())
const configuredSpectatorURL = ref('')
const navigation = computed<AppNavItem[]>(() => views.map((view) => ({
  name: labels[view],
  href: `#${view}`,
  current: activeView.value === view
})))

const fleet = usePollingResource(
  (signal) => getDashboard({ active: true }, signal),
  { intervalMs: 4000 }
)
const build = usePollingResource(
  (signal) => getBuildProvenance(signal),
  { intervalMs: 300000, isEmpty: (info) => !info.version }
)

const liveCount = computed(() => (fleet.data.value?.runs ?? []).filter((run) => run.status === 'running').length)
const queuedCount = computed(() => (fleet.data.value?.runs ?? []).filter((run) => run.status === 'queued' || run.status === 'leased').length)
const idleCount = computed(() => (fleet.data.value?.workers ?? []).filter((worker) => !worker.run_id).length)
const connected = computed(() => fleet.state.value === 'ready' || fleet.state.value === 'refreshing' || fleet.state.value === 'stale')
const buildHref = computed(() => build.data.value?.pr_url || build.data.value?.commit_url || '')
const buildLabel = computed(() => {
  const info = build.data.value
  if (!info?.version) return ''
  const short = info.version.slice(0, 7)
  return info.pr_number ? `#${info.pr_number} · ${short}` : short
})
const buildTitle = computed(() => {
  const info = build.data.value
  if (!info) return ''
  return info.title || (info.pr_number ? `Deployment from PR #${info.pr_number}` : `Deployment ${info.version}`)
})

function syncHash(): void {
  const next = hashView()
  activeView.value = next
  document.title = `PokéPilot · ${labels[next]}`
}

async function loadSpectatorURL(): Promise<void> {
  try {
    configuredSpectatorURL.value = (await getOperatorUIConfig()).spectator_url?.trim() || ''
  } catch {
    configuredSpectatorURL.value = ''
  }
}

function spectatorBaseURL(): string {
  if (configuredSpectatorURL.value) return configuredSpectatorURL.value
  const current = new URL(window.location.href)
  if (current.port === '18080') {
    current.port = '18081'
    current.pathname = '/'
    current.search = ''
    current.hash = ''
    return current.toString()
  }
  return 'https://pokemon.maestroi.cc'
}

function selectedRunID(): string {
  const explicit = new URLSearchParams(window.location.search).get('run')?.trim()
  if (explicit) return explicit
  return [...(fleet.data.value?.runs ?? [])]
    .filter((run) => run.status !== 'done')
    .sort((a, b) => Number(b.queued_at || 0) - Number(a.queued_at || 0))[0]?.run_id || ''
}

function openSelectedSpectator(): void {
  const target = new URL(spectatorBaseURL(), window.location.href)
  const runID = selectedRunID()
  if (runID) target.searchParams.set('run', runID)
  else target.searchParams.delete('run')
  window.open(target.toString(), '_blank', 'noopener,noreferrer')
}

onMounted(() => {
  syncHash()
  void loadSpectatorURL()
  window.addEventListener('hashchange', syncHash)
})
onUnmounted(() => window.removeEventListener('hashchange', syncHash))
</script>

<template>
  <AppShell
    :eyebrow="eyebrows[activeView]"
    :title="labels[activeView]"
    :subtitle="descriptions[activeView]"
    mode="private"
    :navigation="navigation"
    :show-intro="activeView !== 'live'"
  >
    <template #summary>
      <span class="inline-flex items-center gap-1.5">
        <span
          :class="[
            connected && fleet.state.value !== 'stale' ? 'border-[var(--poke-green)] bg-[var(--poke-green)]' : 'border-[var(--poke-amber)] bg-transparent',
            'size-1.5 rounded-full border'
          ]"
          aria-hidden="true"
        />
        <b class="font-semibold text-[var(--poke-text)]">{{ connected ? (fleet.state.value === 'stale' ? 'Stale' : 'Connected') : 'Connecting' }}</b>
      </span>
      <span><b class="font-semibold text-[var(--poke-text)]">{{ liveCount }}</b> live</span>
      <span><b class="font-semibold text-[var(--poke-text)]">{{ queuedCount }}</b> queued</span>
      <span><b class="font-semibold text-[var(--poke-text)]">{{ idleCount }}</b> idle</span>
      <span v-if="fleet.data.value?.wall_version" class="font-mono text-[10px] text-[var(--poke-dim)]" :title="`Wall ${fleet.data.value.wall_version}`">wall {{ fleet.data.value.wall_version.slice(0, 7) }}</span>
      <a
        v-if="buildHref && buildLabel"
        :href="buildHref"
        target="_blank"
        rel="noopener"
        class="font-mono text-[10px] text-[var(--poke-cyan)] hover:underline"
        :title="buildTitle"
      >
        ui {{ buildLabel }}
      </a>
      <span v-else-if="buildLabel" class="font-mono text-[10px] text-[var(--poke-dim)]" :title="buildTitle">ui {{ buildLabel }}</span>
    </template>

    <template #actions>
      <button
        v-if="activeView === 'live'"
        type="button"
        class="inline-flex items-center gap-1 rounded-sm bg-white/10 px-2 py-1 text-[11px] font-bold text-white ring-1 ring-white/10 hover:bg-white/15"
        title="Open the selected run in the public spectator view"
        @click="openSelectedSpectator"
      >
        <ArrowTopRightOnSquareIcon class="size-3.5" aria-hidden="true" />
        Open spectator
      </button>
      <a
        v-if="activeView === 'live'"
        href="#tools"
        class="inline-flex items-center gap-1 rounded-sm bg-[var(--poke-cyan)] px-2 py-1 text-[11px] font-bold text-[#101820] hover:brightness-110"
      >
        <PlayIcon class="size-3.5" aria-hidden="true" />
        New run
      </a>
    </template>

    <LiveView v-if="activeView === 'live'" />
    <RunArchiveView v-else-if="activeView === 'runs'" />
    <FailuresView v-else-if="activeView === 'failures'" />
    <AnalyticsView v-else-if="activeView === 'analytics'" />
    <OperationsView v-else-if="activeView === 'operations'" />
    <SpectatorView v-else-if="activeView === 'spectator'" />
    <ToolsView v-else-if="activeView === 'tools'" />
  </AppShell>
</template>
