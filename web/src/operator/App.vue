<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ArrowTopRightOnSquareIcon, PlayIcon } from '@heroicons/vue/20/solid'
import { getDashboard } from '../shared/api/client'
import AppShell from '../shared/components/AppShell.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import type { AppNavItem } from '../shared/types'
import AnalyticsView from './AnalyticsView.vue'
import FailuresView from './FailuresView.vue'
import LiveView from './LiveView.vue'
import OperationsView from './OperationsView.vue'
import RunArchiveView from './RunArchiveView.vue'
import ToolsView from './ToolsView.vue'

const views = ['live', 'runs', 'failures', 'analytics', 'operations', 'tools'] as const
type OperatorView = typeof views[number]

const labels: Record<OperatorView, string> = {
  live: 'Live',
  runs: 'Runs',
  failures: 'Failures',
  analytics: 'Analytics',
  operations: 'Operations',
  tools: 'Tools'
}

const descriptions: Record<OperatorView, string> = {
  live: 'Watch a selected run, inspect game state and semantic position, and open its persisted evidence and deterministic replay.',
  runs: 'Search completed runs with server-side filters and paging.',
  failures: 'Group, inspect, and investigate actionable run failures.',
  analytics: 'Farm outcomes, badge progress, LLM workload, and endless-run experiments.',
  operations: 'Fleet health, workers, active attempts, queue/leases, and recent outcomes.',
  tools: 'Queue a new scripted or goal-driven run.'
}

const eyebrows: Record<OperatorView, string> = {
  live: 'Monitor',
  runs: 'Archive',
  failures: 'Triage',
  analytics: 'Telemetry',
  operations: 'System',
  tools: 'Control'
}

function hashView(): OperatorView {
  const value = window.location.hash.replace(/^#/, '') as OperatorView
  return views.includes(value) ? value : 'live'
}

const activeView = ref<OperatorView>(hashView())
const navigation = computed<AppNavItem[]>(() => views.map((view) => ({
  name: labels[view],
  href: `#${view}`,
  current: activeView.value === view
})))
const legacyURL = computed(() => {
  const url = new URL('/legacy/', window.location.origin)
  url.search = window.location.search
  url.hash = activeView.value
  return url.toString()
})

const fleet = usePollingResource(
  (signal) => getDashboard({ active: true }, signal),
  { intervalMs: 4000 }
)

const liveCount = computed(() => (fleet.data.value?.runs ?? []).filter((run) => run.status === 'running').length)
const queuedCount = computed(() => (fleet.data.value?.runs ?? []).filter((run) => run.status === 'queued' || run.status === 'leased').length)
const idleCount = computed(() => (fleet.data.value?.workers ?? []).filter((worker) => !worker.run_id).length)
const connected = computed(() => fleet.state.value === 'ready' || fleet.state.value === 'refreshing' || fleet.state.value === 'stale')

function syncHash(): void {
  const next = hashView()
  activeView.value = next
  document.title = `PokéPilot · ${labels[next]}`
}

onMounted(() => {
  syncHash()
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
      <span v-if="fleet.data.value?.wall_version" class="font-mono text-[10px] text-[var(--poke-dim)]">{{ fleet.data.value.wall_version.slice(0, 7) }}</span>
    </template>

    <template #actions>
      <a
        v-if="activeView === 'live'"
        href="#tools"
        class="inline-flex items-center gap-1 rounded-sm bg-[var(--poke-cyan)] px-2 py-1 text-[11px] font-bold text-[#101820] hover:brightness-110"
      >
        <PlayIcon class="size-3.5" aria-hidden="true" />
        New run
      </a>
      <a
        :href="legacyURL"
        class="hidden items-center gap-1 rounded-sm px-2 py-1 text-[11px] font-semibold text-[var(--poke-muted)] ring-1 ring-[var(--poke-border-strong)] hover:bg-white/5 hover:text-white sm:inline-flex"
        title="Temporary fallback while the Vue cutover is validated"
      >
        Legacy
        <ArrowTopRightOnSquareIcon class="size-3.5" aria-hidden="true" />
      </a>
    </template>

    <LiveView v-if="activeView === 'live'" />
    <RunArchiveView v-else-if="activeView === 'runs'" />
    <FailuresView v-else-if="activeView === 'failures'" />
    <AnalyticsView v-else-if="activeView === 'analytics'" />
    <OperationsView v-else-if="activeView === 'operations'" />
    <ToolsView v-else-if="activeView === 'tools'" />
  </AppShell>
</template>
