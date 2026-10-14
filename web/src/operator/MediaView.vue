<script setup lang="ts">
import { computed } from 'vue'
import { FilmIcon, QueueListIcon, SignalIcon } from '@heroicons/vue/20/solid'
import Panel from '../shared/components/Panel.vue'
import { operatorHref } from '../shared/urls'
import BroadcastPanel from './BroadcastPanel.vue'
import RenderJobsPanel from './RenderJobsPanel.vue'
import SpectatorView from './SpectatorView.vue'

type MediaTab = 'library' | 'broadcasts' | 'jobs'
const props = defineProps<{ section?: string; runID?: string; jobID?: string }>()
const activeTab = computed<MediaTab>(() => tabs.some((tab) => tab.id === props.section) ? props.section as MediaTab : 'library')
const tabs: Array<{ id: MediaTab; label: string; description: string }> = [
  { id: 'library', label: 'Library', description: 'Replay discovery and public visibility' },
  { id: 'broadcasts', label: 'Broadcasts', description: 'Twitch, YouTube Live and RTMP controls' },
  { id: 'jobs', label: 'Jobs', description: 'Render queue, progress and failures' }
]

function operatorSearch(): string { return window.location.search }
</script>

<template>
  <div class="space-y-3">
    <nav
      class="grid grid-cols-1 gap-1 rounded-lg border border-[var(--poke-border)] bg-[var(--poke-panel)] p-1 sm:grid-cols-2 xl:grid-cols-3"
      aria-label="Media management"
    >
      <a
        v-for="tab in tabs"
        :key="tab.id"
        :href="operatorHref('media', { section: tab.id }, operatorSearch())"
        :aria-current="activeTab === tab.id ? 'page' : undefined"
        :class="[
          activeTab === tab.id
            ? 'bg-white/10 text-white ring-1 ring-white/10'
            : 'text-[var(--poke-muted)] hover:bg-white/5 hover:text-white',
          'rounded-md px-3 py-2 text-left transition'
        ]"
      >
        <span class="block text-xs font-bold">{{ tab.label }}</span>
        <span class="mt-0.5 block text-[10px] leading-4 text-[var(--poke-muted)]">{{ tab.description }}</span>
      </a>
    </nav>

    <div v-if="activeTab === 'library'" class="space-y-3">
      <Panel
        title="Replay library & publishing"
        description="Public replay availability belongs to the run; this workspace controls which runs are exposed and featured."
        compact
      >
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="flex items-start gap-2.5">
            <FilmIcon class="mt-0.5 size-4 shrink-0 text-violet-300" aria-hidden="true" />
            <p class="max-w-3xl text-xs leading-5 text-[var(--poke-muted)]">
              Open a run from Live or Runs for run-scoped inspection. Use the controls below for public visibility and featured placement; rendered output is tracked under Render Jobs.
            </p>
          </div>
        </div>
      </Panel>
      <SpectatorView :run-id="runID" />
    </div>

    <div v-else-if="activeTab === 'broadcasts'" class="space-y-3">
      <Panel
        title="Live destinations"
        description="Broadcasting is independent from gameplay and uses server-side credentials on the replay host."
        compact
      >
        <div class="flex items-start gap-2.5 text-xs leading-5 text-[var(--poke-muted)]">
          <SignalIcon class="mt-0.5 size-4 shrink-0 text-fuchsia-300" aria-hidden="true" />
          <p>Choose a running run and a configured destination. Stream keys are never stored in run metadata or returned to this browser.</p>
        </div>
      </Panel>
      <BroadcastPanel />
    </div>

    <div v-else-if="activeTab === 'jobs'" class="space-y-3">
      <Panel
        title="Render pipeline"
        description="Replay rendering is media work, while worker and service health remain in Operations."
        compact
      >
        <div class="flex items-start gap-2.5 text-xs leading-5 text-[var(--poke-muted)]">
          <QueueListIcon class="mt-0.5 size-4 shrink-0 text-cyan-300" aria-hidden="true" />
          <p>Durable jobs survive process restarts and report queue, worker, stage, progress and failure state here.</p>
        </div>
      </Panel>
      <RenderJobsPanel :run-id="runID" :job-id="jobID" />
    </div>

  </div>
</template>
