<script setup lang="ts">
import { computed } from 'vue'
import { getDashboard } from '../shared/api/client'
import type { DashboardRun } from '../shared/api/types'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { presentOutcome, presentReplay } from '../shared/status'
import { highlightManifestURL, highlightVideoURL } from '../shared/api/client'
import { formatMediaHref, runOutputs, type MediaState } from './mediaQuery'
import MediaOutputsList from './MediaOutputsList.vue'
import { gameTitle } from './operations'
import { archiveWhen, middleTruncate, progressLabel, timeAgo } from './runs'
import { useRunMedia } from './useRunMedia'
import StudioThumbnails from './StudioThumbnails.vue'
import StudioPublications from './StudioPublications.vue'

// Studio: derived outputs made from a finished run's recording: the highlight
// reel, episode packages and their thumbnails. Outputs the host can't make for
// the selected run are not offered; they collapse into one reason line.
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
const outputs = computed(() => runOutputs({
  replayData: Boolean(source.value?.replay_available),
  live: false,
  replay: media.replay.value,
  highlights: highlights.value,
  packages: media.packages.value,
  packagesError: media.packagesError.value
}))
const showHighlights = computed(() => outputs.value.rows.some((row) => row.id === 'highlights'))
const showPackages = computed(() => outputs.value.rows.some((row) => row.id === 'package'))
function progress(run: DashboardRun): string {
  const label = progressLabel(run)
  return label === 'No structured progress' ? '' : label
}
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
            <div class="truncate font-medium">{{ run.game ? gameTitle(run.game) : 'Unknown game' }} · {{ run.goal || 'Free play' }}</div>
            <div class="truncate text-meta text-fg-2">
              <span class="font-mono" :title="run.run_id">{{ middleTruncate(run.run_id) }}</span>
              · <span :title="archiveWhen(run)">ended {{ timeAgo(Number(run.ended_at || 0)) }}</span>
              <template v-if="progress(run)"> · {{ progress(run) }}</template>
            </div>
          </div>
          <StatusPill :status="presentOutcome(run)" class="shrink-0" />
        </li>
      </ul>
    </section>

    <div class="min-w-0 space-y-3">
    <section class="ui-bay min-w-0" aria-labelledby="studio-outputs-title">
      <div class="ui-bay-header">
        <h2 id="studio-outputs-title" class="min-w-0 truncate">Outputs<template v-if="runID"> · <span class="font-mono font-medium" :title="runID">{{ middleTruncate(runID) }}</span></template></h2>
        <StatusPill v-if="source" :status="presentOutcome(source)" />
      </div>
      <div v-if="!runID" class="flex flex-col items-center gap-2 px-4 py-12 text-center">
        <span class="text-[20px] leading-5 text-fg-3" aria-hidden="true">○</span>
        <p class="text-title font-semibold">Choose a source recording</p>
        <p class="text-fg-2">You’ll see what already exists for that run and what can still be made from it.</p>
      </div>
      <div v-else class="space-y-3 p-3">
        <p v-if="source" class="text-fg-2">
          {{ source.game ? gameTitle(source.game) : 'Unknown game' }} · {{ source.goal || 'Free play' }} · <span :title="archiveWhen(source)">ended {{ timeAgo(Number(source.ended_at || 0)) }}</span>
        </p>
        <MediaOutputsList :outputs="outputs" />
        <a :href="formatMediaHref({ ...state, tab: 'library' }, search())" class="ui-btn ui-btn-quiet max-md:h-10">Open in Library</a>
      </div>
    </section>

    <section v-if="runID && (showHighlights || !highlights)" class="ui-bay min-w-0" aria-labelledby="studio-create-title">
      <div class="ui-bay-header">
        <h2 id="studio-create-title" class="min-w-0 truncate">Highlight reel</h2>
        <StatusPill v-if="highlights" :status="presentReplay(highlights.state, undefined, 'Highlights')" />
      </div>
      <div class="space-y-3 p-3">
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
        </div>
      </div>
    </section>
    </div>
    <StudioThumbnails
      v-if="runID && showPackages"
      :run-i-d="runID"
      :packages="media.packages.value || []"
      @changed="media.refreshPackages()"
    />
    <StudioPublications
      v-if="runID"
      :run-i-d="runID"
      :packages="media.packages.value || []"
      @changed="media.refreshPackages()"
    />
  </div>
</template>
