<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ArrowTopRightOnSquareIcon, Cog6ToothIcon, PlayIcon } from '@heroicons/vue/20/solid'
import { getBuildProvenance, getDashboard } from '../shared/api/client'
import { getOperatorUIConfig } from '../shared/api/spectator-control'
import { operatorPrimaryViews, operatorRoute, resolveSpectatorBase, spectatorURL, type OperatorView } from '../shared/urls'
import AppShell from '../shared/components/AppShell.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import type { AppNavItem } from '../shared/types'
import AnalyticsView from './AnalyticsView.vue'
import FailuresView from './FailuresView.vue'
import LiveView from './LiveView.vue'
import MediaView from './MediaView.vue'
import OperationsView from './OperationsView.vue'
import SettingsView from './SettingsView.vue'
import RunArchiveView from './RunArchiveView.vue'
import ToolsView from './ToolsView.vue'

const views = operatorPrimaryViews

const labels: Record<OperatorView, string> = {
  live: 'Live',
  runs: 'Runs',
  failures: 'Failures',
  analytics: 'Analytics',
  media: 'Media',
  operations: 'Operations',
  settings: 'Settings',
  'new-run': 'New run'
}

const descriptions: Record<OperatorView, string> = {
  live: 'Watch a selected run, inspect game state and semantic position, and open its persisted evidence and deterministic replay.',
  runs: 'Search completed runs, inspect persisted evidence, and clean up older history.',
  failures: 'Group, inspect, and investigate finished runs for a specific failure.',
  analytics: 'Run outcomes, badge progress, LLM workload and comparisons.',
  media: 'Discover replay media, manage rendering jobs, and control broadcasting and publishing.',
  operations: 'Fleet health, workers, queues, leases, and operational status.',
  settings: 'Configure model deployments, per-browser launch defaults, broadcast destination status and renderer preferences.',
  'new-run': 'Choose a game, settings and goal, then queue a new run.'
}

const eyebrows: Record<OperatorView, string> = {
  live: 'Monitor',
  runs: 'Archive',
  failures: 'Triage',
  analytics: 'Telemetry',
  media: 'Library',
  operations: 'System',
  settings: 'Configuration',
  'new-run': 'Launch'
}

const initialRoute = operatorRoute(window.location.hash)
const activeView = ref<OperatorView>(initialRoute.view)
const activeSection = ref(initialRoute.section)
const linkedRunID = ref(new URLSearchParams(window.location.search).get('run') || '')
const linkedJobID = ref(new URLSearchParams(window.location.search).get('job') || '')
const linkedExperimentID = ref(new URLSearchParams(window.location.search).get('experiment') || '')
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
  const next = operatorRoute(window.location.hash)
  activeView.value = next.view
  activeSection.value = next.section
  const query = new URLSearchParams(window.location.search)
  linkedRunID.value = query.get('run') || ''
  linkedJobID.value = query.get('job') || ''
  linkedExperimentID.value = query.get('experiment') || ''
  document.title = `RomPilot Admin · ${labels[next.view]}`
}

async function loadSpectatorURL(): Promise<void> {
  try {
    const config = await getOperatorUIConfig()
    configuredSpectatorURL.value = resolveSpectatorBase(config, window.location.href)
  } catch {
    configuredSpectatorURL.value = resolveSpectatorBase({}, window.location.href)
  }
}

function spectatorBaseURL(): string {
  return configuredSpectatorURL.value
}

function selectedRunID(): string {
  const explicit = new URLSearchParams(window.location.search).get('run')?.trim()
  if (explicit) return explicit
  return [...(fleet.data.value?.runs ?? [])]
    .filter((run) => run.status !== 'done')
    .sort((a, b) => Number(b.queued_at || 0) - Number(a.queued_at || 0))[0]?.run_id || ''
}

function openSelectedSpectator(): void {
  const base = spectatorBaseURL()
  if (!base) return
  window.open(spectatorURL(base, selectedRunID()), '_blank', 'noopener,noreferrer')
}

onMounted(() => {
  syncHash()
  void loadSpectatorURL()
  window.addEventListener('hashchange', syncHash)
  window.addEventListener('popstate', syncHash)
})
onUnmounted(() => {
  window.removeEventListener('hashchange', syncHash)
  window.removeEventListener('popstate', syncHash)
})
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
        title="Open the selected run on the public RomPilot site"
        @click="openSelectedSpectator"
      >
        <ArrowTopRightOnSquareIcon class="size-3.5" aria-hidden="true" />
        Open public
      </button>
      <a
        href="#new-run"
        class="inline-flex items-center gap-1 rounded-sm bg-[var(--poke-cyan)] px-2 py-1 text-[11px] font-bold text-[#101820] hover:brightness-110 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white"
      >
        <PlayIcon class="size-3.5" aria-hidden="true" />
        New run
      </a>
      <a
        href="#settings"
        :aria-current="activeView === 'settings' ? 'page' : undefined"
        class="inline-flex items-center gap-1 rounded-sm px-2 py-1 text-[11px] font-semibold text-[var(--poke-muted)] ring-1 ring-[var(--poke-border-strong)] hover:bg-white/5 hover:text-white focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--poke-cyan)]"
        title="Settings"
        aria-label="Settings"
      >
        <Cog6ToothIcon class="size-4" aria-hidden="true" />
        <span class="hidden xl:inline">Settings</span>
      </a>
    </template>

    <LiveView v-if="activeView === 'live'" :public-base="configuredSpectatorURL" />
    <RunArchiveView v-else-if="activeView === 'runs'" :run-id="linkedRunID" />
    <FailuresView v-else-if="activeView === 'failures'" />
    <AnalyticsView v-else-if="activeView === 'analytics'" :section="activeSection" :experiment-id="linkedExperimentID" />
    <MediaView v-else-if="activeView === 'media'" :section="activeSection" :run-id="linkedRunID" :job-id="linkedJobID" />
    <OperationsView v-else-if="activeView === 'operations'" />
    <SettingsView v-else-if="activeView === 'settings'" :section="activeSection" />
    <ToolsView v-else-if="activeView === 'new-run'" />
  </AppShell>
</template>
