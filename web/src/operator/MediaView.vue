<script setup lang="ts">
import { computed, ref } from 'vue'
import { FilmIcon, PaintBrushIcon, QueueListIcon, SignalIcon } from '@heroicons/vue/20/solid'
import Panel from '../shared/components/Panel.vue'
import { publicRenderThemeOptions, renderThemeOptions } from '../shared/renderTheme'
import BroadcastPanel from './BroadcastPanel.vue'
import RenderJobsPanel from './RenderJobsPanel.vue'
import SpectatorView from './SpectatorView.vue'

type MediaTab = 'replays' | 'broadcasts' | 'render-jobs' | 'themes'

const activeTab = ref<MediaTab>('replays')
const tabs: Array<{ id: MediaTab; label: string; description: string }> = [
  { id: 'replays', label: 'Replays', description: 'Public publishing and replay visibility' },
  { id: 'broadcasts', label: 'Broadcasts', description: 'Twitch, YouTube Live and RTMP controls' },
  { id: 'render-jobs', label: 'Render Jobs', description: 'Queue, progress, workers and failures' },
  { id: 'themes', label: 'Assets / Themes', description: 'Renderer inventory and public-safe themes' }
]

const allThemes = renderThemeOptions()
const publicThemes = publicRenderThemeOptions()
const localOnlyThemes = computed(() => allThemes.filter((theme) => !publicThemes.some((candidate) => candidate.id === theme.id)))

function selectTab(tab: MediaTab): void {
  activeTab.value = tab
}
</script>

<template>
  <div class="space-y-3">
    <nav
      class="grid grid-cols-1 gap-1 rounded-lg border border-[var(--poke-border)] bg-[var(--poke-panel)] p-1 sm:grid-cols-2 xl:grid-cols-4"
      aria-label="Media management"
    >
      <button
        v-for="tab in tabs"
        :key="tab.id"
        type="button"
        :aria-current="activeTab === tab.id ? 'page' : undefined"
        :class="[
          activeTab === tab.id
            ? 'bg-white/10 text-white ring-1 ring-white/10'
            : 'text-[var(--poke-muted)] hover:bg-white/5 hover:text-white',
          'rounded-md px-3 py-2 text-left transition'
        ]"
        @click="selectTab(tab.id)"
      >
        <span class="block text-xs font-bold">{{ tab.label }}</span>
        <span class="mt-0.5 block text-[10px] leading-4 text-[var(--poke-muted)]">{{ tab.description }}</span>
      </button>
    </nav>

    <div v-if="activeTab === 'replays'" class="space-y-3">
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
      <SpectatorView />
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

    <div v-else-if="activeTab === 'render-jobs'" class="space-y-3">
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
      <RenderJobsPanel />
    </div>

    <div v-else class="space-y-3">
      <Panel
        title="Renderer assets & themes"
        description="Installed renderer themes and which ones are safe to expose on the public spectator and replay surfaces."
        compact
      >
        <div class="mb-3 flex items-start gap-2.5 text-xs leading-5 text-[var(--poke-muted)]">
          <PaintBrushIcon class="mt-0.5 size-4 shrink-0 text-amber-300" aria-hidden="true" />
          <p>Theme ingestion and provenance stay centralized; public surfaces only receive themes marked for public distribution.</p>
        </div>

        <div class="grid gap-3 lg:grid-cols-2">
          <section class="rounded-lg border border-white/8 bg-black/15 p-3">
            <div class="flex items-center justify-between gap-3">
              <h3 class="text-xs font-bold text-white">Public-safe</h3>
              <span class="font-mono text-[10px] text-emerald-300">{{ publicThemes.length }}</span>
            </div>
            <ul class="mt-2 divide-y divide-white/8">
              <li v-for="theme in publicThemes" :key="theme.id" class="py-2 first:pt-0 last:pb-0">
                <div class="flex items-center justify-between gap-3">
                  <strong class="text-xs text-slate-200">{{ theme.name }}</strong>
                  <span class="font-mono text-[9px] text-slate-600">v{{ theme.version }}</span>
                </div>
                <p class="mt-0.5 text-[10px] leading-4 text-slate-500">{{ theme.description || theme.id }}</p>
              </li>
            </ul>
          </section>

          <section class="rounded-lg border border-white/8 bg-black/15 p-3">
            <div class="flex items-center justify-between gap-3">
              <h3 class="text-xs font-bold text-white">Local / operator only</h3>
              <span class="font-mono text-[10px] text-amber-300">{{ localOnlyThemes.length }}</span>
            </div>
            <ul v-if="localOnlyThemes.length" class="mt-2 divide-y divide-white/8">
              <li v-for="theme in localOnlyThemes" :key="theme.id" class="py-2 first:pt-0 last:pb-0">
                <div class="flex items-center justify-between gap-3">
                  <strong class="text-xs text-slate-200">{{ theme.name }}</strong>
                  <span class="font-mono text-[9px] text-slate-600">v{{ theme.version }}</span>
                </div>
                <p class="mt-0.5 text-[10px] leading-4 text-slate-500">{{ theme.description || theme.id }}</p>
              </li>
            </ul>
            <p v-else class="mt-2 text-xs text-slate-500">Every installed theme is currently public-safe.</p>
          </section>
        </div>
      </Panel>
    </div>
  </div>
</template>
