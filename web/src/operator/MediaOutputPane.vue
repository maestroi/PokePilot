<script setup lang="ts">
import { computed, ref, toRef, watch } from 'vue'
import { ClipboardDocumentIcon, EyeIcon, EyeSlashIcon, StarIcon, XMarkIcon } from '@heroicons/vue/20/solid'
import { highlightManifestURL, highlightVideoURL, replayVideoURL } from '../shared/api/client'
import type { DashboardRun } from '../shared/api/types'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import { presentRecording, presentRunStatus } from '../shared/status'
import { canFeature, canOpenPublic, isLiveRun, publicActionLabel, runOutputs, visibilityLabel, visibilityTitle } from './mediaQuery'
import MediaOutputsList from './MediaOutputsList.vue'
import { gameTitle } from './operations'
import type { RecordingSummary } from './runRecording'
import { timeAgo } from './runs'
import { useRunMedia } from './useRunMedia'

// Library detail: play the run's outputs in-pane, see their state, and manage
// publication beside them (public site visibility, YouTube). Generation of
// derived outputs lives in Studio; job lifecycle lives in Jobs.
const props = defineProps<{
  runId: string
  run: DashboardRun | null
  loading: boolean
  recording: RecordingSummary | null
  visible: boolean
  featured: boolean
  busy: boolean
  jobsHref: string
  studioHref: string
  sourceHref: string
}>()
const emit = defineEmits<{
  close: []
  copy: [value: string, noun: string]
  visible: [value: boolean]
  feature: [value: boolean]
  'open-public': []
}>()

const media = useRunMedia(toRef(props, 'runId'))
// The replay host's status wins; the list summary only bridges until it loads.
const replayReady = computed(() => media.replay.value ? media.replay.value.state === 'ready' : Boolean(props.recording?.canWatch))
const highlightsReady = computed(() => media.highlights.value?.state === 'ready')
const mode = ref<'full' | 'highlights'>('full')
watch(() => props.runId, () => { mode.value = 'full'; confirmCancel.value = false; confirmPublish.value = false })
const playerSource = computed(() => {
  if (mode.value === 'highlights' && highlightsReady.value) return highlightVideoURL(props.runId)
  return replayReady.value ? replayVideoURL(props.runId) : ''
})

const headerStatus = computed(() => {
  if (!props.run) return null
  return isLiveRun(props.run) ? presentRunStatus(props.run) : props.recording ? presentRecording(props.recording) : null
})
const outputs = computed(() => runOutputs({
  replayData: Boolean(props.run?.replay_available),
  live: Boolean(props.run && isLiveRun(props.run)),
  replay: media.replay.value,
  highlights: media.highlights.value,
  packages: media.packages.value,
  packagesError: media.packagesError.value
}))
const hasOutput = (id: string) => outputs.value.rows.some((row) => row.id === id)
// Why the player has nothing to play, from the replay host's own status.
const well = computed(() => {
  const run = props.run
  if (run && isLiveRun(run)) return { title: 'This run is still live', detail: 'Its replay video can be rendered after it ends. Watch it now in Live.' }
  const state = media.replay.value?.state
  if (state === 'generating') return { title: 'Rendering the replay video', detail: 'It plays here when the render job is done.' }
  if (state === 'missing' && run?.replay_available) return { title: 'No replay video yet', detail: 'The replay data is stored. Render the video below.' }
  if (!run?.replay_available) return { title: 'No replay data', detail: 'No recording was stored for this run, so there is nothing to play.' }
  return { title: props.recording?.label || 'No replay video yet', detail: props.recording?.detail || '' }
})
const canGenerate = computed(() => {
  const state = media.replay.value?.state
  return Boolean(props.run && !isLiveRun(props.run) && state && state !== 'disabled' && state !== 'generating' && state !== 'ready')
})

const youtubePrivacy = ref<'private' | 'unlisted' | 'public'>('unlisted')
const confirmCancel = ref(false)
const confirmPublish = ref(false)
</script>

<template>
  <section class="flex h-full min-h-0 flex-col bg-surface" aria-labelledby="media-pane-title">
    <header class="flex min-h-12 shrink-0 items-center gap-2 border-b border-rule bg-raised px-3 py-2">
      <h2 id="media-pane-title" class="min-w-0 truncate font-mono text-title font-medium" :title="runId">
        <span class="sr-only">Media for run </span>{{ runId }}
      </h2>
      <button type="button" class="ui-btn ui-btn-sm ui-icon-btn shrink-0" aria-label="Copy run ID" title="Copy run ID" @click="emit('copy', runId, 'Run ID')">
        <ClipboardDocumentIcon class="size-4" aria-hidden="true" />
      </button>
      <StatusPill v-if="headerStatus" :status="headerStatus" class="ml-auto shrink-0" />
      <button type="button" class="ui-btn ui-btn-sm ui-icon-btn shrink-0" :class="headerStatus ? '' : 'ml-auto'" aria-label="Close media details" title="Close (Esc)" @click="emit('close')">
        <XMarkIcon class="size-4" aria-hidden="true" />
      </button>
    </header>

    <div v-if="!run && loading" class="flex-1 space-y-3 p-4" aria-busy="true" aria-label="Loading media">
      <div class="aspect-video bg-raised motion-safe:animate-pulse" />
      <div v-for="n in 4" :key="n" class="h-4 bg-raised motion-safe:animate-pulse" :class="n % 2 ? 'w-2/3' : 'w-full'" />
    </div>
    <div v-else-if="!run" class="flex flex-1 flex-col items-start gap-3 p-4" role="status">
      <p class="text-title">Run not found. It may have been deleted.</p>
      <button type="button" class="ui-btn" @click="emit('close')">Close</button>
    </div>

    <template v-else>
      <div class="min-h-0 flex-1 overflow-y-auto pb-4">
        <!-- Player -->
        <div class="border-b border-rule bg-well">
          <video
            v-if="playerSource"
            :key="playerSource"
            :src="playerSource"
            controls
            preload="metadata"
            class="aspect-video w-full bg-well"
            :aria-label="`${mode === 'highlights' ? 'Highlight reel' : 'Replay video'} of ${runId}`"
          />
          <div v-else class="flex aspect-video flex-col items-center justify-center gap-1 px-6 text-center">
            <span class="text-[20px] leading-5 text-fg-3" aria-hidden="true">▶</span>
            <p class="text-fg">{{ well.title }}</p>
            <p class="max-w-[48ch] text-meta text-fg-2">{{ well.detail }}</p>
          </div>
        </div>
        <div v-if="highlightsReady && replayReady" class="flex items-center gap-2 px-3 pt-3">
          <div class="inline-flex border border-rule-strong" role="group" aria-label="Output">
            <button type="button" :aria-pressed="mode === 'full'" :class="mode === 'full' ? 'bg-selected text-fg' : 'text-fg-2 hover:bg-raised'" class="h-7 border-r border-rule-strong px-3" @click="mode = 'full'">Replay video</button>
            <button type="button" :aria-pressed="mode === 'highlights'" :class="mode === 'highlights' ? 'bg-selected text-fg' : 'text-fg-2 hover:bg-raised'" class="h-7 px-3" @click="mode = 'highlights'">Highlight reel</button>
          </div>
        </div>

        <div class="px-3">
          <!-- What exists for this run, then actions per output -->
          <h3 class="mt-4 mb-1 text-title font-semibold">Outputs</h3>
          <MediaOutputsList :outputs="outputs" />

          <template v-if="hasOutput('video')">
          <h3 class="mt-5 text-title font-semibold">Replay video</h3>
          <div class="mt-2 flex flex-wrap gap-2">
            <button v-if="canGenerate" type="button" class="ui-btn ui-btn-sm max-md:h-10" :disabled="media.rendering.value" @click="media.requestReplay()">
              {{ media.rendering.value ? 'Requesting…' : media.replay.value?.state === 'error' ? 'Retry replay video' : 'Render replay video' }}
            </button>
            <button
              v-if="media.replay.value?.state === 'generating' && media.replay.value.job_id"
              type="button"
              class="ui-btn ui-btn-sm ui-btn-danger max-md:h-10"
              :disabled="media.cancellingReplay.value"
              @click="confirmCancel = true"
            >Cancel generation…</button>
            <a v-if="replayReady" :href="replayVideoURL(runId)" target="_blank" rel="noopener" class="ui-btn ui-btn-sm ui-btn-quiet max-md:h-10">Open video file ↗</a>
            <a v-if="jobsHref" :href="jobsHref" class="ui-btn ui-btn-sm ui-btn-quiet max-md:h-10">Open render job</a>
          </div>
          </template>
          <p v-if="media.replayError.value" class="mt-2 text-meta text-warn" role="status">▲ {{ media.replayError.value }}</p>

          <!-- Highlight reel and packages are made in Studio -->
          <template v-if="hasOutput('highlights') || hasOutput('package')">
            <h3 class="mt-5 text-title font-semibold">Studio outputs</h3>
            <div class="mt-2 flex flex-wrap gap-2">
              <a :href="studioHref" class="ui-btn ui-btn-sm max-md:h-10">Open in Studio</a>
              <a v-if="highlightsReady" :href="highlightManifestURL(runId)" class="ui-btn ui-btn-sm ui-btn-quiet max-md:h-10">Highlight manifest</a>
            </div>
          </template>

          <!-- Public site: spectator visibility, not an outbound broadcast -->
          <h3 class="mt-5 text-title font-semibold">Public site</h3>
          <div class="mt-2 flex flex-wrap items-center gap-2">
            <button type="button" class="ui-btn ui-btn-sm max-md:h-10" :aria-pressed="visible" :disabled="busy" :title="visibilityTitle(run, visible)" @click="emit('visible', !visible)">
              <EyeIcon v-if="visible" class="size-4 text-ok" aria-hidden="true" />
              <EyeSlashIcon v-else class="size-4" aria-hidden="true" />
              {{ visibilityLabel(run, visible) }}
            </button>
            <button
              v-if="canFeature(run) || featured"
              type="button"
              class="ui-btn ui-btn-sm max-md:h-10"
              :aria-pressed="featured"
              :disabled="busy"
              :title="featured ? 'Stop featuring this run' : 'Feature this live run as the default public watch view'"
              @click="emit('feature', !featured)"
            >
              <StarIcon class="size-4" :class="featured ? 'text-ok' : ''" aria-hidden="true" />
              {{ featured ? (canFeature(run) ? 'Featured' : 'Clear feature') : 'Feature live' }}
            </button>
            <span v-if="!canOpenPublic(run, visible)" class="text-meta text-fg-2">{{ publicActionLabel(run, visible) }}</span>
          </div>

          <!-- YouTube -->
          <template v-if="media.youtube.value?.configured && replayReady">
            <h3 class="mt-5 text-title font-semibold">YouTube · legacy replay upload</h3>
            <p class="mt-1 text-meta text-warn">Direct replay uploads bypass the Studio quality and approval queue. Use <a :href="studioHref" class="underline">Media Studio</a> for reviewed episodes and Shorts.</p>
            <div class="mt-2 flex flex-wrap items-center gap-2">
              <a
                v-if="media.youtube.value.publication.state === 'published' && media.youtube.value.publication.video_url"
                :href="media.youtube.value.publication.video_url"
                target="_blank"
                rel="noopener"
                class="ui-btn ui-btn-sm"
              >Published ({{ media.youtube.value.publication.privacy }}) ↗</a>
              <span v-else-if="media.youtube.value.publication.state === 'uploading'" class="ui-pill ui-tone-accent"><span aria-hidden="true">◐</span> Uploading</span>
              <template v-else>
                <span v-if="['failed', 'interrupted'].includes(media.youtube.value.publication.state)" class="ui-pill ui-tone-danger" :title="media.youtube.value.publication.error"><span aria-hidden="true">✕</span> {{ media.youtube.value.publication.state }}</span>
                <label class="flex items-center gap-2">
                  <span class="ui-label">Visibility</span>
                  <select v-model="youtubePrivacy" class="h-7 rounded-[3px] border border-control bg-canvas px-2 text-fg max-md:h-10">
                    <option value="private">Private</option><option value="unlisted">Unlisted</option><option value="public">Public</option>
                  </select>
                </label>
                <button type="button" class="ui-btn ui-btn-sm max-md:h-10" :disabled="media.publishing.value" @click="confirmPublish = true">
                  {{ media.youtube.value.publication.state === 'none' ? 'Publish to YouTube…' : 'Retry upload…' }}
                </button>
              </template>
            </div>
            <p v-if="media.youtubeError.value" class="mt-2 text-meta text-warn" role="status">▲ {{ media.youtubeError.value }}</p>
          </template>

          <!-- Source -->
          <h3 class="mt-5 mb-1 text-title font-semibold">Source run</h3>
          <dl class="ui-kv">
            <dt>Run</dt><dd><a :href="sourceHref" class="font-mono break-all text-accent hover:underline">{{ run.run_id }}</a></dd>
            <template v-if="run.game"><dt>Game</dt><dd>{{ gameTitle(run.game) }}</dd></template>
            <dt>Goal</dt><dd>{{ run.goal || 'Free play' }}</dd>
            <dt>{{ isLiveRun(run) ? 'Status' : 'Outcome' }}</dt><dd><StatusPill :status="presentRunStatus(run)" /></dd>
            <dt>{{ isLiveRun(run) ? 'Queued' : 'Ended' }}</dt><dd>{{ timeAgo(Number(isLiveRun(run) ? run.queued_at : run.ended_at) || 0) }}</dd>
          </dl>
        </div>
      </div>
      <footer class="flex shrink-0 flex-wrap items-center gap-2 border-t border-rule px-3 py-2">
        <button v-if="canOpenPublic(run, visible)" type="button" class="ui-btn ui-btn-primary max-md:h-10" @click="emit('open-public')">{{ publicActionLabel(run, visible) }} ↗</button>
        <a :href="sourceHref" class="ui-btn max-md:h-10">Open source run</a>
      </footer>
    </template>

    <ConfirmDialog
      :open="confirmCancel"
      :title="`Cancel replay generation for ${runId}?`"
      message="The generation job stops. You can generate the replay again later from here or from Media."
      confirm-label="Cancel generation"
      cancel-label="Keep generating"
      :busy="media.cancellingReplay.value"
      danger
      @close="confirmCancel = false"
      @confirm="media.cancelReplay().finally(() => { confirmCancel = false })"
    />
    <ConfirmDialog
      :open="confirmPublish"
      :title="`Upload the replay of ${runId} to YouTube?`"
      :message="`The video is uploaded as ${youtubePrivacy}.`"
      confirm-label="Upload"
      :busy="media.publishing.value"
      @close="confirmPublish = false"
      @confirm="media.publishYouTube(youtubePrivacy).finally(() => { confirmPublish = false })"
    />
  </section>
</template>
