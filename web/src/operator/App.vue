<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import AppShell from '../shared/components/AppShell.vue'
import Panel from '../shared/components/Panel.vue'
import type { AppNavItem } from '../shared/types'
import OperationsView from './OperationsView.vue'
import RunArchiveView from './RunArchiveView.vue'

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
  live: 'Watch and inspect a selected run.',
  runs: 'Search completed runs with server-side filters and paging.',
  failures: 'Group and investigate actionable run failures.',
  analytics: 'Campaign progress, outcomes, and experiments.',
  operations: 'Fleet health, workers, active attempts, queue/leases, and recent outcomes update independently without reloading the page.',
  tools: 'Create a run and configure its objective.'
}

function hashView(): OperatorView {
  const value = window.location.hash.replace(/^#/, '') as OperatorView
  return views.includes(value) ? value : 'operations'
}

const activeView = ref<OperatorView>(hashView())
const migrated = new Set<OperatorView>(['operations', 'runs'])

const navigation = computed<AppNavItem[]>(() => views.map((view) => ({
  name: labels[view],
  href: `#${view}`,
  current: activeView.value === view
})))

const eyebrow = computed(() => activeView.value === 'operations' ? 'System' : activeView.value === 'runs' ? 'Archive' : 'Migration')
const legacyURL = computed(() => `/${window.location.search}#${activeView.value}`)

function syncHash(): void {
  activeView.value = hashView()
}

onMounted(() => window.addEventListener('hashchange', syncHash))
onUnmounted(() => window.removeEventListener('hashchange', syncHash))
</script>

<template>
  <AppShell
    :eyebrow="eyebrow"
    :title="labels[activeView]"
    :subtitle="descriptions[activeView]"
    mode="private"
    :navigation="navigation"
  >
    <OperationsView v-if="activeView === 'operations'" />
    <RunArchiveView v-else-if="activeView === 'runs'" />

    <Panel v-else :title="`${labels[activeView]} is still on the legacy surface`" description="This view has not been cut over to Vue yet." compact>
      <p class="max-w-2xl text-sm leading-6 text-slate-400">
        The migration is deliberately view-by-view. The existing operator implementation remains authoritative for this section while its Vue replacement is built and tested.
      </p>
      <a
        :href="legacyURL"
        class="mt-4 inline-flex rounded-md bg-cyan-500 px-3 py-2 text-xs font-semibold text-white shadow-sm hover:bg-cyan-400"
      >
        Open {{ labels[activeView] }} in legacy UI
      </a>
    </Panel>
  </AppShell>
</template>
