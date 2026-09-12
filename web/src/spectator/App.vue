<script setup lang="ts">
import AppShell from '../shared/components/AppShell.vue'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
</script>

<template>
  <AppShell
    eyebrow="PokéPilot"
    title="Spectator"
    subtitle="Public read-only shell sharing the operator visual system without inheriting operator capabilities."
    mode="public"
  >
    <div class="grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,2fr)_minmax(18rem,1fr)]">
      <Panel title="Live run" description="Game frame, objective, party, progress, and public-safe run story.">
        <template #actions><StatusBadge tone="success">Read-only</StatusBadge></template>
        <div class="grid aspect-[10/7] place-items-center rounded-md border border-white/10 bg-black/30">
          <div class="text-center">
            <div class="mx-auto size-10 rounded-lg border border-white/10 bg-white/5" />
            <p class="mt-3 text-sm font-medium text-slate-300">Live game surface</p>
            <p class="mt-1 text-xs text-slate-500">Existing spectator frame path will mount here.</p>
          </div>
        </div>
      </Panel>

      <div class="space-y-3">
        <Panel title="Run state" description="Shared presentation, public-safe data only.">
          <dl class="space-y-3">
            <div v-for="fact in [['Objective', 'Waiting for a public run'], ['Location', '—'], ['Party', '—']]" :key="fact[0]" class="flex items-start justify-between gap-4 border-b border-white/10 pb-3 last:border-0 last:pb-0">
              <dt class="text-xs font-medium text-slate-500">{{ fact[0] }}</dt>
              <dd class="text-right text-sm text-slate-300">{{ fact[1] }}</dd>
            </div>
          </dl>
        </Panel>

        <Panel title="Loading behavior" description="Initial load is local to the resource, not the document.">
          <ResourceState state="loading" :rows="3" />
        </Panel>
      </div>
    </div>
  </AppShell>
</template>
