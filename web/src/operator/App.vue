<script setup lang="ts">
import AppShell, { type AppNavItem } from '../shared/components/AppShell.vue'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'

const navigation: AppNavItem[] = [
  { name: 'Live', href: '#live' },
  { name: 'Runs', href: '#runs' },
  { name: 'Failures', href: '#failures', badge: 0 },
  { name: 'Analytics', href: '#analytics' },
  { name: 'Operations', href: '#operations', current: true },
  { name: 'Tools', href: '#tools' }
]
</script>

<template>
  <AppShell
    eyebrow="PokéPilot"
    title="Operator console"
    subtitle="Parallel Vue migration shell. The current operator remains authoritative while sections move across incrementally."
    mode="private"
    :navigation="navigation"
  >
    <div class="grid grid-cols-1 gap-3 xl:grid-cols-3">
      <Panel title="Operations" description="Workers, active attempts, queue, health, and inference routing." class="xl:col-span-2">
        <template #actions><StatusBadge tone="info">First target</StatusBadge></template>
        <div class="grid grid-cols-2 gap-px overflow-hidden rounded-md bg-white/10 ring-1 ring-white/10 sm:grid-cols-4">
          <div v-for="metric in [['Workers', '—'], ['Active', '—'], ['Queued', '—'], ['Failures', '—']]" :key="metric[0]" class="bg-[#0b111a] px-4 py-4">
            <span class="block text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">{{ metric[0] }}</span>
            <strong class="mt-1 block font-mono text-xl font-semibold text-white">{{ metric[1] }}</strong>
          </div>
        </div>
      </Panel>

      <Panel title="Connection state" description="Scoped refresh behavior instead of whole-page reloads.">
        <ResourceState state="stale" message="Example: keep the last dashboard snapshot visible while a background refresh retries.">
          <div class="rounded-md border border-white/10 bg-black/10 px-3 py-3 text-sm text-slate-400">
            Last known data remains mounted here.
          </div>
        </ResourceState>
      </Panel>

      <Panel title="Runs" description="Archive filtering, pagination, cleanup, and run selection." class="xl:col-span-2">
        <div class="overflow-hidden rounded-md border border-white/10">
          <table class="min-w-full divide-y divide-white/10 text-left">
            <thead class="bg-white/[0.025]">
              <tr>
                <th v-for="heading in ['Run', 'State', 'Goal', 'Updated']" :key="heading" class="px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">{{ heading }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-white/10 bg-black/10">
              <tr>
                <td class="px-3 py-3 font-mono text-xs text-slate-300">run-example</td>
                <td class="px-3 py-3"><StatusBadge tone="neutral">Placeholder</StatusBadge></td>
                <td class="px-3 py-3 text-sm text-slate-400">Existing API contract stays unchanged.</td>
                <td class="px-3 py-3 text-xs text-slate-500">—</td>
              </tr>
            </tbody>
          </table>
        </div>
      </Panel>

      <Panel title="Shared foundation" description="Operator and spectator consume the same presentation primitives.">
        <ul class="space-y-2 text-sm text-slate-400">
          <li>Vue 3 + strict TypeScript</li>
          <li>Tailwind CSS 4</li>
          <li>Headless UI for accessible interaction</li>
          <li>Heroicons for shared iconography</li>
        </ul>
      </Panel>
    </div>
  </AppShell>
</template>
