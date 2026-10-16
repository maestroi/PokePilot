<script setup lang="ts">
import { computed, ref } from 'vue'
import { getDashboard, getMediaRenderJobs } from '../shared/api/client'
import type { DashboardRun } from '../shared/api/types'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import VideoPlayer from '../shared/components/VideoPlayer.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { presentOutcome, presentReplay } from '../shared/status'
import { highlightManifestURL, highlightVideoURL, replayVideoURL } from '../shared/api/client'
import { formatMediaHref, latestJobsByKind, presentRenderMark, studioSteps, type MediaState, type RenderKind } from './mediaQuery'
import { gameTitle } from './operations'
import { archiveWhen, middleTruncate, progressLabel, timeAgo } from './runs'
import { useRunMedia } from './useRunMedia'
import { useStudio, type StudioFormat } from './useStudio'
import StudioThumbnails from './StudioThumbnails.vue'
import StudioPublications from './StudioPublications.vue'

// Studio turns a finished run's recording into a published video. The
// pipeline shows every step in order with the next one's action inline; the
// sections below it hold the detail of the steps that have something to show.
const props = defineProps<{ state: MediaState }>()
const emit = defineEmits<{ navigate: [next: MediaState, replace?: boolean] }>()

const recent = usePollingResource((signal) => getDashboard({ status: 'done', limit: 48 }, signal), {
  intervalMs: 30000,
  isEmpty: (snapshot) => !snapshot.runs?.length
})
// What has been rendered per run, from the render job list (newest job of each kind).
const jobs = usePollingResource((signal) => getMediaRenderJobs(200, signal), { intervalMs: 10000 })
const jobsByRun = computed(() => latestJobsByKind(jobs.data.value?.jobs || []))
const MARKS: { kind: RenderKind; label: string }[] = [{ kind: 'video', label: 'Video' }, { kind: 'highlight', label: 'Reel' }, { kind: 'edit', label: 'Edit' }]
const TONE_TEXT: Record<string, string> = { ok: 'text-ok', danger: 'text-danger', accent: 'text-accent', warn: 'text-warn', neutral: 'text-fg-3' }
function mark(run: DashboardRun, kind: RenderKind) {
  return presentRenderMark(jobsByRun.value.get(run.run_id)?.[kind])
}
// The outcome as a short note: a finished goal shows its progress, anything else its reason.
function outcome(run: DashboardRun): { text: string; tone: string } {
  const status = presentOutcome(run)
  return { text: status.tone === 'ok' ? `✓ ${progress(run) || 'Goal complete'}` : `${status.glyph} ${status.label}`, tone: TONE_TEXT[status.tone] || 'text-fg-2' }
}
// Runs without a stored recording can't make anything; hide them by default.
const sourceFilter = ref<'recorded' | 'all'>('recorded')
const allSources = computed(() => [...(recent.data.value?.runs || [])].sort((a, b) => Number(b.ended_at || 0) - Number(a.ended_at || 0)))
const sources = computed(() => sourceFilter.value === 'all' ? allSources.value : allSources.value.filter((run) => run.replay_available || run.run_id === runID.value))
const runID = computed(() => props.state.run)
const source = computed(() => allSources.value.find((run) => run.run_id === runID.value) || null)
const media = useRunMedia(runID)
const studio = useStudio(runID, media.packages, () => { void media.refreshPackages() })
const highlights = computed(() => media.highlights.value)
const pipeline = computed(() => studioSteps({
  replayData: Boolean(source.value?.replay_available),
  replay: media.replay.value,
  highlights: highlights.value,
  packagesError: media.packagesError.value,
  assessment: studio.assessment.value,
  pkg: studio.pkg.value,
  profile: studio.pkg.value?.plan_hash ? studio.profile.value : '',
  edit: studio.edit.value,
  thumbnails: { rendered: studio.thumbnails.value?.manifest.candidates.length || 0, selected: Boolean(studio.selectedThumbnail.value) },
  publication: studio.publication.value
}))
const hasStep = (id: string) => pipeline.value.steps.some((step) => step.id === id)
const format = ref<StudioFormat>('episode')

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
    <section class="ui-bay min-w-0 self-start" aria-labelledby="studio-sources-title">
      <div class="ui-bay-header">
        <h2 id="studio-sources-title">Source recording</h2>
        <select v-model="sourceFilter" class="ui-input h-7 text-meta" aria-label="Which finished runs to list">
          <option value="recorded">With recording</option>
          <option value="all">All finished</option>
        </select>
      </div>
      <ResourceState v-if="recent.state.value === 'error' || recent.state.value === 'loading'" :state="recent.state.value" class="m-3" title="Couldn’t load finished runs" :message="`${recent.error.value}. Retry when the wall is reachable.`" :rows="5">
        <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="recent.retry()">Retry</button></template>
      </ResourceState>
      <p v-else-if="!sources.length" class="px-3 py-12 text-center text-fg-2">
        {{ allSources.length ? 'No finished run has a stored recording.' : 'Finished runs appear here. Their recordings are the source for Studio.' }}
      </p>
      <ul v-else role="listbox" aria-label="Source recording" class="max-h-[70vh] overflow-y-auto">
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
            <div class="truncate font-medium" :title="`${run.game ? gameTitle(run.game) : 'Unknown game'} · ${run.goal || 'Free play'}`">{{ run.game ? gameTitle(run.game) : 'Unknown game' }} · {{ run.goal || 'Free play' }}</div>
            <div class="truncate text-meta text-fg-2">
              <span class="font-mono" :title="run.run_id">{{ middleTruncate(run.run_id) }}</span>
              · <span :title="archiveWhen(run)">ended {{ timeAgo(Number(run.ended_at || 0)) }}</span>
              · <span :class="outcome(run).tone" :title="presentOutcome(run).label">{{ outcome(run).text }}</span>
              <template v-if="!run.replay_available"> · no recording</template>
            </div>
          </div>
          <dl class="grid w-28 shrink-0 grid-cols-[auto_1fr] gap-x-1.5 text-meta leading-4" :aria-label="`Renders of ${run.run_id}`">
            <template v-for="item in MARKS" :key="item.kind">
              <dt class="text-fg-3">{{ item.label }}</dt>
              <dd class="font-mono tabular-nums" :class="TONE_TEXT[mark(run, item.kind).tone]" :title="`${item.label}: ${mark(run, item.kind).label}`">
                {{ mark(run, item.kind).tone === 'accent' ? `${mark(run, item.kind).glyph} ${mark(run, item.kind).label}` : mark(run, item.kind).glyph }}
              </dd>
            </template>
          </dl>
        </li>
      </ul>
    </section>

    <div class="min-w-0 space-y-3">
      <section class="ui-bay min-w-0" aria-labelledby="studio-pipeline-title">
        <div class="ui-bay-header">
          <h2 id="studio-pipeline-title" class="min-w-0 truncate">Pipeline<template v-if="runID"> · <span class="font-mono font-medium" :title="runID">{{ middleTruncate(runID) }}</span></template></h2>
          <a v-if="runID" :href="formatMediaHref({ ...state, tab: 'library' }, search())" class="ui-btn ui-btn-sm ui-btn-quiet">Open in Library</a>
        </div>
        <div v-if="!runID" class="flex flex-col items-center gap-2 px-4 py-12 text-center">
          <span class="text-[20px] leading-5 text-fg-3" aria-hidden="true">○</span>
          <p class="text-title font-semibold">Choose a source recording</p>
          <p class="text-fg-2">You’ll see how far it is from a published video and what to do next.</p>
        </div>
        <div v-else class="p-3">
          <p v-if="source" class="mb-2 text-fg-2">
            {{ source.game ? gameTitle(source.game) : 'Unknown game' }} · {{ source.goal || 'Free play' }} · <span :title="archiveWhen(source)">ended {{ timeAgo(Number(source.ended_at || 0)) }}</span>
          </p>
          <ol class="border border-rule" aria-label="Publishing steps">
            <li
              v-for="(step, index) in pipeline.steps"
              :key="step.id"
              :aria-current="step.id === pipeline.current ? 'step' : undefined"
              class="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-rule px-3 py-2 last:border-b-0"
              :class="step.id === pipeline.current ? 'bg-selected shadow-[inset_2px_0_0_var(--color-accent)]' : ''"
            >
              <span class="w-5 shrink-0 text-center font-mono text-meta" :class="step.done ? 'text-ok' : 'text-fg-3'" aria-hidden="true">{{ step.done ? '✓' : index + 1 }}</span>
              <div class="min-w-0 flex-1 basis-60">
                <div class="font-medium">{{ step.label }}<span v-if="step.optional" class="ml-2 text-meta font-normal text-fg-3">optional</span></div>
                <p v-if="step.detail" class="text-meta text-fg-2 md:truncate" :title="step.detail">{{ step.detail }}</p>
              </div>
              <StatusPill :status="step.status" class="shrink-0 max-md:ml-8" />
              <div v-if="step.id === pipeline.current" class="flex w-full flex-wrap items-center gap-2 pl-8">
                <template v-if="step.id === 'video' && media.replay.value?.state !== 'generating'">
                  <button type="button" class="ui-btn ui-btn-primary ui-btn-sm max-md:h-10" :disabled="media.rendering.value" @click="media.requestReplay()">
                    {{ media.rendering.value ? 'Requesting…' : step.status.tone === 'danger' ? 'Retry replay video' : 'Render replay video' }}
                  </button>
                </template>
                <button v-else-if="step.id === 'verdict'" type="button" class="ui-btn ui-btn-primary ui-btn-sm max-md:h-10" :disabled="studio.busy.value" @click="studio.assess()">
                  {{ studio.busy.value ? 'Assessing…' : 'Assess run' }}
                </button>
                <template v-else-if="step.id === 'package'">
                  <button v-if="studio.pkg.value?.plan_hash && studio.edit.value?.state !== 'generating'" type="button" class="ui-btn ui-btn-primary ui-btn-sm max-md:h-10" :disabled="studio.busy.value || !hasStep('video') || media.replay.value?.state !== 'ready'" @click="studio.retryEdit()">
                    Render edited video
                  </button>
                  <template v-if="!studio.pkg.value?.plan_hash">
                    <select v-if="studio.verdict.value === 'full_episode'" v-model="format" class="ui-input h-7 text-meta max-md:h-10" aria-label="Output format" :disabled="studio.busy.value">
                      <option value="episode">Full episode · landscape</option>
                      <option value="short">Short · portrait</option>
                    </select>
                    <button type="button" class="ui-btn ui-btn-primary ui-btn-sm max-md:h-10" :disabled="studio.busy.value" @click="studio.prepare(format)">
                      {{ studio.busy.value ? 'Preparing…' : studio.verdict.value === 'short_only' ? 'Prepare Short package' : 'Prepare package' }}
                    </button>
                  </template>
                  <span v-if="media.replay.value?.state !== 'ready'" class="text-meta text-fg-2">The edit renders once the replay video is ready.</span>
                </template>
                <span v-else-if="step.id === 'thumbnail'" class="text-meta text-fg-2">Generate thumbnails and pick one below.</span>
                <span v-else-if="step.id === 'publish'" class="text-meta text-fg-2">Queue, approve and publish below.</span>
              </div>
            </li>
          </ol>
          <p v-if="pipeline.unavailable.length" class="mt-2 max-w-[72ch] text-meta text-fg-2">
            <span aria-hidden="true">■</span> {{ pipeline.unavailable.join(' ') }}
          </p>
          <p v-if="studio.error.value" class="mt-2 border border-danger/40 bg-danger/10 px-3 py-2 text-danger [overflow-wrap:anywhere]" role="alert">✕ {{ studio.error.value }}</p>
          <p v-else-if="studio.message.value" class="mt-2 text-meta text-fg-2" role="status">{{ studio.message.value }}</p>
          <p v-if="media.replayError.value" class="mt-2 text-meta text-warn" role="status">▲ {{ media.replayError.value }}</p>
        </div>
      </section>

      <section v-if="runID && media.replay.value?.state === 'ready'" class="ui-bay min-w-0" aria-labelledby="studio-replay-title">
        <div class="ui-bay-header">
          <h2 id="studio-replay-title" class="min-w-0 truncate">Replay video</h2>
        </div>
        <div class="p-3">
          <div class="relative max-w-3xl">
            <VideoPlayer
              :key="runID"
              :src="replayVideoURL(runID)"
              controls
              preload="metadata"
              class="aspect-video max-h-[45vh] w-full bg-well"
              :aria-label="`Replay video of ${runID}`"
            />
          </div>
        </div>
      </section>

      <section v-if="runID && hasStep('highlights')" class="ui-bay min-w-0" aria-labelledby="studio-create-title">
        <div class="ui-bay-header">
          <h2 id="studio-create-title" class="min-w-0 truncate">Highlight reel</h2>
          <StatusPill v-if="highlights" :status="presentReplay(highlights.state, undefined, 'Highlights')" />
        </div>
        <div class="space-y-3 p-3">
          <div v-if="highlights?.state === 'ready'" class="relative max-w-3xl">
          <VideoPlayer
            :key="runID"
            :src="highlightVideoURL(runID)"
            controls
            preload="metadata"
            class="aspect-video max-h-[45vh] w-full max-w-3xl bg-well"
            :aria-label="`Highlight reel of ${runID}`"
          />
          </div>
          <p class="max-w-[72ch] text-fg-2">
            <template v-if="highlights?.state === 'missing'">No reel yet. Generating one cuts the run’s recorded milestones into a short video.</template>
            <template v-else-if="highlights">
              {{ highlights.windows || 0 }} moment{{ Number(highlights.windows || 0) === 1 ? '' : 's' }} · {{ duration() }}
              <template v-if="highlights.state === 'generating'"> · {{ highlights.windows_done || 0 }}/{{ highlights.windows || 0 }} rendered</template>
            </template>
          </p>
          <p v-if="highlights?.last_error || highlights?.error" class="border border-danger/40 bg-danger/10 px-3 py-2 text-danger" role="status">✕ {{ highlights.last_error || highlights.error }}</p>
          <p v-if="media.highlightError.value" class="text-meta text-warn" role="status">▲ {{ media.highlightError.value }}</p>
          <div class="flex flex-wrap items-center gap-2">
            <button v-if="canGenerate" type="button" class="ui-btn max-md:h-10" :disabled="media.renderingHighlights.value" @click="media.requestHighlights()">
              {{ media.renderingHighlights.value ? 'Requesting…' : highlights?.state === 'error' || highlights?.state === 'failed' ? 'Retry highlights' : 'Generate highlights' }}
            </button>
            <a v-if="highlights?.state === 'ready'" :href="highlightManifestURL(runID)" class="ui-btn max-md:h-10">Manifest</a>
            <a v-if="highlights?.job_id" :href="formatMediaHref({ ...state, tab: 'jobs', job: highlights.job_id }, search())" class="ui-btn ui-btn-quiet max-md:h-10">Open render job</a>
          </div>
        </div>
      </section>

      <StudioThumbnails v-if="runID && studio.pkg.value && hasStep('package')" :run-i-d="runID" :studio="studio" :packages="media.packages.value || []" />
      <StudioPublications v-if="runID && (studio.pkg.value || studio.candidates.value.length)" :run-i-d="runID" :studio="studio" />
    </div>
  </div>
</template>
