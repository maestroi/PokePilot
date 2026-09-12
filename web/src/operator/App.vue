<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ArrowTopRightOnSquareIcon } from '@heroicons/vue/20/solid'
import AppShell from '../shared/components/AppShell.vue'
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
  >
    <template #actions>
      <a
        :href="legacyURL"
        class="hidden items-center gap-1.5 rounded-md bg-white/6 px-2.5 py-1.5 text-[11px] font-semibold text-slate-400 ring-1 ring-white/8 hover:bg-white/10 hover:text-white sm:inline-flex"
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
