<script setup lang="ts">
import { computed, ref, toRef, watch } from 'vue'
import { ClipboardDocumentIcon, EyeIcon, EyeSlashIcon, StarIcon, XMarkIcon } from '@heroicons/vue/20/solid'
import { highlightManifestURL, highlightVideoURL, replayVideoURL } from '../shared/api/client'
import type { DashboardRun } from '../shared/api/types'
import ConfirmDialog from '../shared/components/ConfirmDialog.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import { presentRecording, presentReplay, presentRunStatus } from '../shared/status'
import { canFeature, canOpenPublic, isLiveRun, publicActionLabel, visibilityLabel, visibilityTitle } from './mediaQuery'
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
const replayReady = computed(() => Boolean(props.recording?.canWatch) || media.replay.value?.state === 'ready')
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
const replayStatus = computed(() => {
  const replay = media.replay.value
  const total = Number(replay?.segments || 0)
  return presentReplay(replay?.state, total ? 100 * Number(replay?.segments_done || 0) / total : undefined)
})
const canGenerate = computed(() => {
  const state = media.replay.value?.state
  return Boolean(props.run && !isLiveRun(props.run) && state && state !== 'disabled' && state !== 'generating' && state !== 'ready')
})
function highlightSummary(): string {
  const h = media.highlights.value
  if (!h || h.state === 'missing') return 'Not generated'
  if (h.state === 'empty') return 'No highlight-worthy events were found.'
  const total = Math.max(0, Math.round(Number(h.duration_ms || 0) / 1000))
  const length = total >= 60 ? `${Math.floor(total / 60)}m ${total % 60}s` : `${total}s`
  return `${h.windows || 0} moment${Number(h.windows || 0) === 1 ? '' : 's'} · ${length}`
}

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
            :aria-label="`${mode === 'highlights' ? 'Highlight reel' : 'Full replay'} of ${runId}`"
          />
          <div v-else class="flex aspect-video flex-col items-center justify-center gap-1 px-6 text-center">
            <span class="text-[20px] leading-5 text-fg-3" aria-hidden="true">▶</span>
            <p class="text-fg">{{ isLiveRun(run) ? 'This run is still live' : recording?.label || 'No replay yet' }}</p>
            <p class="max-w-[48ch] text-meta text-fg-2">{{ isLiveRun(run) ? 'Its replay can be generated after it ends. Watch it now in Live.' : recording?.detail }}</p>
          </div>
        </div>
        <div v-if="highlightsReady && replayReady" class="flex items-center gap-2 px-3 pt-3">
          <div class="inline-flex border border-rule-strong" role="group" aria-label="Output">
            <button type="button" :aria-pressed="mode === 'full'" :class="mode === 'full' ? 'bg-selected text-fg' : 'text-fg-2 hover:bg-raised'" class="h-7 border-r border-rule-strong px-3" @click="mode = 'full'">Full replay</button>
            <button type="button" :aria-pressed="mode === 'highlights'" :class="mode === 'highlights' ? 'bg-selected text-fg' : 'text-fg-2 hover:bg-raised'" class="h-7 px-3" @click="mode = 'highlights'">Highlights</button>
          </div>
        </div>

        <div class="px-3">
          <!-- Full replay -->
          <div class="mt-4 flex items-center gap-2">
            <h3 class="text-title font-semibold">Full replay</h3>
            <StatusPill v-if="media.replay.value" :status="replayStatus" />
          </div>
          <p class="mt-1 max-w-[72ch] text-meta text-fg-2">Generated from the run’s canonical recording.</p>
          <div class="mt-2 flex flex-wrap gap-2">
            <button v-if="canGenerate" type="button" class="ui-btn ui-btn-sm max-md:h-10" :disabled="media.rendering.value" @click="media.requestReplay()">
              {{ media.rendering.value ? 'Requesting…' : media.replay.value?.state === 'error' ? 'Retry replay' : 'Generate replay' }}
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
          <p v-if="media.replayError.value" class="mt-2 text-meta text-warn" role="status">▲ {{ media.replayError.value }}</p>

          <!-- Highlights (generated in Studio) -->
          <div class="mt-5 flex items-center gap-2">
            <h3 class="text-title font-semibold">Highlights</h3>
            <StatusPill v-if="media.highlights.value" :status="presentReplay(media.highlights.value.state, undefined, 'Highlights')" />
          </div>
          <p class="mt-1 text-meta text-fg-2">{{ highlightSummary() }}</p>
          <div class="mt-2 flex flex-wrap gap-2">
            <a :href="studioHref" class="ui-btn ui-btn-sm max-md:h-10">{{ highlightsReady ? 'Open in Studio' : 'Create in Studio' }}</a>
            <a v-if="highlightsReady" :href="highlightManifestURL(runId)" class="ui-btn ui-btn-sm ui-btn-quiet max-md:h-10">Manifest</a>
          </div>

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
            <h3 class="mt-5 text-title font-semibold">YouTube</h3>
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
