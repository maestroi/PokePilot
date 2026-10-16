<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Cog6ToothIcon } from '@heroicons/vue/20/solid'
import { getBuildProvenance, getDashboard } from '../shared/api/client'
import { getOperatorUIConfig } from '../shared/api/spectator-control'
import { operatorPrimaryViews, operatorRoute, resolveSpectatorBase, type OperatorView } from '../shared/urls'
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
  failures: '',
  analytics: 'Run outcomes, badge progress, LLM workload and comparisons.',
  media: '',
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
    class="operator-root"
    :eyebrow="eyebrows[activeView]"
    :title="labels[activeView]"
    :subtitle="descriptions[activeView]"
    mode="private"
    :navigation="navigation"
    :show-intro="activeView !== 'live' && activeView !== 'runs' && activeView !== 'failures' && activeView !== 'media'"
  >
    <template #summary>
      <a :href="'#operations'" class="inline-flex items-center gap-1.5 text-meta text-fg-2 hover:text-fg" title="Open Operations">
        <span :class="connected && fleet.state.value !== 'stale' ? 'text-ok' : 'text-warn'" aria-hidden="true">{{ connected && fleet.state.value !== 'stale' ? '●' : '▲' }}</span>
        <span class="text-fg">{{ connected ? (fleet.state.value === 'stale' ? 'Stale' : 'Connected') : 'Connecting' }}</span>
        · {{ liveCount }} live · {{ queuedCount }} queued · {{ idleCount }} idle
      </a>
      <span v-if="fleet.data.value?.wall_version" class="font-mono text-meta text-fg-3" :title="`Wall ${fleet.data.value.wall_version}`">wall {{ fleet.data.value.wall_version.slice(0, 7) }}</span>
      <a v-if="buildHref && buildLabel" :href="buildHref" target="_blank" rel="noopener" class="font-mono text-meta text-fg-3 hover:text-accent" :title="buildTitle">ui {{ buildLabel }}</a>
      <span v-else-if="buildLabel" class="font-mono text-meta text-fg-3" :title="buildTitle">ui {{ buildLabel }}</span>
    </template>

    <template #actions>
      <a href="#new-run" class="ui-btn ui-btn-sm">
        <span aria-hidden="true">+</span> New run
      </a>
      <a
        href="#settings"
        :aria-current="activeView === 'settings' ? 'page' : undefined"
        :class="activeView === 'settings' ? 'text-fg bg-selected' : ''"
        class="ui-btn ui-btn-sm ui-icon-btn"
        title="Settings"
        aria-label="Settings"
      >
        <Cog6ToothIcon class="size-4" aria-hidden="true" />
      </a>
    </template>

    <LiveView v-if="activeView === 'live'" :public-base="configuredSpectatorURL" />
    <RunArchiveView v-else-if="activeView === 'runs'" :run-id="linkedRunID" />
    <FailuresView v-else-if="activeView === 'failures'" />
    <AnalyticsView v-else-if="activeView === 'analytics'" :section="activeSection" :experiment-id="linkedExperimentID" />
    <MediaView v-else-if="activeView === 'media'" :public-base="configuredSpectatorURL" />
    <OperationsView v-else-if="activeView === 'operations'" />
    <SettingsView v-else-if="activeView === 'settings'" :section="activeSection" />
    <ToolsView v-else-if="activeView === 'new-run'" />
  </AppShell>
</template>
