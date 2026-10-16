<script setup lang="ts">
import { computed, toRef } from 'vue'
import { getDashboard } from '../shared/api/client'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { presentOutcome, presentReplay } from '../shared/status'
import { highlightManifestURL, highlightVideoURL } from '../shared/api/client'
import { formatMediaHref, type MediaState } from './mediaQuery'
import { gameTitle } from './operations'
import { middleTruncate, timeAgo } from './runs'
import { useRunMedia } from './useRunMedia'

// Studio: derived outputs made from a finished run's recording. Only the
// highlight reel exists as a backend capability today, so that is all it offers.
const props = defineProps<{ state: MediaState }>()
const emit = defineEmits<{ navigate: [next: MediaState, replace?: boolean] }>()

const recent = usePollingResource((signal) => getDashboard({ status: 'done', limit: 48 }, signal), {
  intervalMs: 30000,
  isEmpty: (snapshot) => !snapshot.runs?.length
})
const sources = computed(() => [...(recent.data.value?.runs || [])].sort((a, b) => Number(b.ended_at || 0) - Number(a.ended_at || 0)))
const runID = computed(() => props.state.run)
const source = computed(() => sources.value.find((run) => run.run_id === runID.value) || null)
const media = useRunMedia(runID)
const highlights = computed(() => media.highlights.value)
const canGenerate = computed(() => Boolean(highlights.value && !['ready', 'generating', 'empty', 'disabled'].includes(highlights.value.state)))

function select(id: string): void {
  emit('navigate', { ...props.state, run: id }, Boolean(props.state.run))
}
function duration(): string {
  const total = Math.max(0, Math.round(Number(highlights.value?.duration_ms || 0) / 1000))
  return total >= 60 ? `${Math.floor(total / 60)}m ${total % 60}s` : `${total}s`
}
const search = () => window.location.search
</script>

<template>
  <div class="grid gap-3 xl:grid-cols-[minmax(0,420px)_minmax(0,1fr)]">
    <section class="ui-bay min-w-0" aria-labelledby="studio-sources-title">
      <div class="ui-bay-header"><h2 id="studio-sources-title">Source recording</h2></div>
      <ResourceState v-if="recent.state.value === 'error' || recent.state.value === 'loading'" :state="recent.state.value" class="m-3" title="Couldn’t load finished runs" :message="`${recent.error.value}. Retry when the wall is reachable.`" :rows="5">
        <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="recent.retry()">Retry</button></template>
      </ResourceState>
      <p v-else-if="!sources.length" class="px-3 py-12 text-center text-fg-2">Finished runs appear here. Their recordings are the source for Studio outputs.</p>
      <ul v-else role="listbox" aria-label="Source recording" class="max-h-[60vh] overflow-y-auto">
        <li
          v-for="run in sources"
          :key="run.run_id"
          role="option"
          tabindex="0"
          :aria-selected="run.run_id === runID"
          :class="run.run_id === runID ? 'bg-selected shadow-[inset_2px_0_0_var(--color-accent)]' : 'hover:bg-raised'"
          class="flex min-h-14 cursor-pointer items-center gap-3 border-b border-rule px-3 py-2 last:border-b-0 focus-visible:outline-offset-[-2px]"
          @click="select(run.run_id)"
          @keydown.enter.prevent="select(run.run_id)"
          @keydown.space.prevent="select(run.run_id)"
        >
          <div class="min-w-0 flex-1">
            <div class="truncate font-mono" :title="run.run_id">{{ middleTruncate(run.run_id) }}</div>
            <div class="truncate text-meta text-fg-2">{{ run.game ? gameTitle(run.game) : 'Game' }} · {{ timeAgo(Number(run.ended_at || 0)) }}</div>
          </div>
          <StatusPill :status="presentOutcome(run)" class="shrink-0" />
        </li>
      </ul>
    </section>

    <section class="ui-bay min-w-0" aria-labelledby="studio-create-title">
      <div class="ui-bay-header">
        <h2 id="studio-create-title" class="min-w-0 truncate">Highlight reel<template v-if="runID"> · <span class="font-mono font-medium" :title="runID">{{ middleTruncate(runID) }}</span></template></h2>
        <StatusPill v-if="runID && highlights" :status="presentReplay(highlights.state, undefined, 'Highlights')" />
      </div>
      <div v-if="!runID" class="flex flex-col items-center gap-2 px-4 py-12 text-center">
        <span class="text-[20px] leading-5 text-fg-3" aria-hidden="true">○</span>
        <p class="text-title font-semibold">Choose a source recording</p>
        <p class="text-fg-2">A highlight reel is cut from the run’s recorded milestones and kept linked to its source run.</p>
      </div>
      <div v-else class="space-y-3 p-3">
        <video
          v-if="highlights?.state === 'ready'"
          :key="runID"
          :src="highlightVideoURL(runID)"
          controls
          preload="metadata"
          class="aspect-video w-full max-w-3xl bg-well"
          :aria-label="`Highlight reel of ${runID}`"
        />
        <p class="max-w-[72ch] text-fg-2">
          <template v-if="!highlights">Checking highlight status…</template>
          <template v-else-if="highlights.state === 'empty'">No highlight-worthy events were found in this run.</template>
          <template v-else-if="highlights.state === 'disabled'">Highlight rendering isn’t enabled on the replay host.</template>
          <template v-else-if="highlights.state === 'missing'">No reel yet. Generating one cuts the run’s recorded milestones into a short video.</template>
          <template v-else>
            {{ highlights.windows || 0 }} moment{{ Number(highlights.windows || 0) === 1 ? '' : 's' }} · {{ duration() }}
            <template v-if="highlights.state === 'generating'"> · {{ highlights.windows_done || 0 }}/{{ highlights.windows || 0 }} rendered</template>
          </template>
        </p>
        <p v-if="highlights?.last_error || highlights?.error" class="border border-danger/40 bg-danger/10 px-3 py-2 text-danger" role="status">✕ {{ highlights.last_error || highlights.error }}</p>
        <p v-if="media.highlightError.value" class="text-meta text-warn" role="status">▲ {{ media.highlightError.value }}</p>
        <div class="flex flex-wrap items-center gap-2">
          <button v-if="canGenerate" type="button" class="ui-btn ui-btn-primary max-md:h-10" :disabled="media.renderingHighlights.value" @click="media.requestHighlights()">
            {{ media.renderingHighlights.value ? 'Requesting…' : highlights?.state === 'error' || highlights?.state === 'failed' ? 'Retry highlights' : 'Generate highlights' }}
          </button>
          <a v-if="highlights?.state === 'ready'" :href="highlightManifestURL(runID)" class="ui-btn max-md:h-10">Manifest</a>
          <a v-if="highlights?.job_id" :href="formatMediaHref({ ...state, tab: 'jobs', job: highlights.job_id }, search())" class="ui-btn ui-btn-quiet max-md:h-10">Open render job</a>
          <a :href="formatMediaHref({ ...state, tab: 'library' }, search())" class="ui-btn ui-btn-quiet max-md:h-10">Open in Library</a>
        </div>
        <p v-if="source" class="text-meta text-fg-2">Source: {{ source.game ? gameTitle(source.game) : 'Game' }} · {{ source.goal || 'Free play' }}</p>
      </div>
    </section>
  </div>
</template>
