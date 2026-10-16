<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { studioThumbnailURL } from '../shared/api/client'
import type { StudioPublicationCandidate } from '../shared/api/types'
import StatusPill from '../shared/components/StatusPill.vue'
import { presentPublication } from './mediaQuery'
import { middleTruncate } from './runs'
import type { Studio } from './useStudio'

// Queue the open package for YouTube, then approve and publish it: two
// separate human steps. Every candidate of the run is listed with its state.
const props = defineProps<{ runID: string; studio: Studio }>()
const titleID = ref('')
const privacy = ref<'private' | 'unlisted' | 'public'>('private')
const notBefore = ref('')

const pkg = computed(() => props.studio.pkg.value)
const isShort = computed(() => props.studio.profile.value === 'short')
const busy = computed(() => props.studio.busy.value)
const titleOptions = computed(() => isShort.value ? (pkg.value?.shorts || []) : (pkg.value?.titles || []))
const title = computed(() => titleOptions.value.find((option) => option.id === titleID.value)?.title || '')
const thumbnail = computed(() => pkg.value && props.studio.selectedThumbnail.value ? studioThumbnailURL(props.runID, pkg.value.hash, props.studio.selectedThumbnail.value) : '')
const queued = computed(() => props.studio.publication.value && props.studio.publication.value.state !== 'skipped')
const blocker = computed(() => !pkg.value?.plan_hash ? 'This package has no edit plan.'
  : queued.value ? 'This package is already queued; see Candidates.'
  : !isShort.value && !thumbnail.value ? 'Pick a thumbnail first: an episode can’t pass the quality gate without one.'
  : !titleID.value ? 'Choose a title.' : '')
const candidates = computed(() => [...props.studio.candidates.value].sort((a, b) => Number(b.updated_at_unix || 0) - Number(a.updated_at_unix || 0)))
watch(titleOptions, (options) => {
  if (!options.some((option) => option.id === titleID.value)) titleID.value = options[0]?.id || ''
}, { immediate: true })

function queue(): void {
  const unix = notBefore.value ? Math.floor(new Date(notBefore.value).getTime() / 1000) : 0
  void props.studio.queue({ titleID: titleID.value, privacy: privacy.value, notBeforeUnix: Number.isFinite(unix) ? unix : 0 })
}
// A rendering candidate always fails the gate, so approval waits for the edit.
function canApprove(c: StudioPublicationCandidate): boolean {
  return !c.approved && ['queued', 'blocked', 'failed'].includes(c.state)
}
function canPublish(c: StudioPublicationCandidate): boolean {
  return c.approved && (c.state === 'approved' || c.state === 'failed')
}
function canSkip(c: StudioPublicationCandidate): boolean {
  return c.state !== 'publishing' && c.state !== 'published' && c.state !== 'skipped'
}
function candidateTitle(c: StudioPublicationCandidate): string {
  if (c.proposal.format === 'archive') return 'Archive only · no upload'
  const options = c.proposal.format === 'short' ? pkg.value?.shorts : pkg.value?.titles
  return (c.proposal.package_hash === pkg.value?.hash && options?.find((o) => o.id === c.proposal.title_id)?.title) || `${c.proposal.format === 'short' ? 'Short' : 'Episode'} ${middleTruncate(c.id, 6, 4)}`
}
</script>

<template>
  <section class="ui-bay min-w-0" aria-labelledby="studio-publications-title">
    <div class="ui-bay-header">
      <h2 id="studio-publications-title">Publication</h2>
      <span class="text-meta text-fg-2">YouTube · approve, then publish</span>
    </div>
    <div class="space-y-4 p-3">
      <div v-if="pkg" class="grid gap-4 lg:grid-cols-[minmax(0,320px)_minmax(0,1fr)]">
        <figure class="min-w-0" aria-label="How it will look on YouTube">
          <img v-if="thumbnail" :src="thumbnail" alt="Picked thumbnail" class="aspect-video w-full bg-well object-cover" />
          <div v-else class="flex aspect-video w-full items-center justify-center bg-well text-meta text-fg-3">{{ isShort ? 'Short · no thumbnail needed' : 'No thumbnail picked' }}</div>
          <figcaption class="mt-2">
            <p class="line-clamp-2 font-semibold">{{ title || 'No title chosen' }}</p>
            <p class="text-meta text-fg-2">{{ isShort ? 'Short' : 'Episode' }} · {{ privacy }}</p>
          </figcaption>
        </figure>
        <div class="min-w-0 space-y-3">
          <label class="flex flex-col gap-1 text-meta" for="studio-pub-title">
            Title
            <select id="studio-pub-title" v-model="titleID" class="ui-input h-10 w-full min-w-0" :disabled="busy">
              <option v-for="option in titleOptions" :key="option.id" :value="option.id">{{ option.title }}</option>
            </select>
          </label>
          <div class="flex flex-wrap gap-3">
            <label class="flex flex-col gap-1 text-meta" for="studio-pub-privacy">
              Privacy
              <select id="studio-pub-privacy" v-model="privacy" class="ui-input h-10" :disabled="busy">
                <option value="private">Private</option><option value="unlisted">Unlisted</option><option value="public">Public</option>
              </select>
            </label>
            <label class="flex flex-col gap-1 text-meta" for="studio-pub-schedule">
              Not before (optional)
              <input id="studio-pub-schedule" v-model="notBefore" type="datetime-local" class="ui-input h-10" :disabled="busy" />
            </label>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button class="ui-btn ui-btn-primary h-10" type="button" :disabled="busy || Boolean(blocker)" @click="queue">Queue for approval</button>
            <span v-if="blocker" class="text-meta text-fg-2">{{ blocker }}</span>
          </div>
        </div>
      </div>

      <div class="space-y-2" :class="pkg ? 'border-t border-rule pt-3' : ''">
        <h3 class="font-semibold">Candidates</h3>
        <p v-if="!candidates.length" class="text-meta text-fg-2">Nothing queued for this run yet.</p>
        <article v-for="candidate in candidates" :key="candidate.id" class="space-y-2 border border-rule bg-surface p-3">
          <div class="flex flex-wrap items-center gap-2">
            <span class="min-w-0 flex-1 truncate font-medium" :title="candidate.id">{{ candidateTitle(candidate) }}</span>
            <span v-if="candidate.proposal.format !== 'archive'" class="text-meta text-fg-2">{{ candidate.proposal.privacy }}</span>
            <span v-if="candidate.proposal.not_before_unix" class="text-meta text-fg-2">after {{ new Date(candidate.proposal.not_before_unix * 1000).toLocaleString() }}</span>
            <StatusPill :status="presentPublication(candidate.state)" />
          </div>
          <p v-if="candidate.reason" class="text-meta text-fg-2">{{ candidate.reason }}</p>
          <p v-if="candidate.thumbnail_status === 'failed'" class="text-meta text-warn">▲ Video uploaded; the thumbnail didn’t. Retry it without uploading the video again.</p>
          <div class="flex flex-wrap gap-2">
            <button v-if="canApprove(candidate)" type="button" class="ui-btn ui-btn-sm max-md:h-10" :disabled="busy" @click="studio.act(candidate, 'approve')">Approve</button>
            <button v-if="canPublish(candidate)" type="button" class="ui-btn ui-btn-sm ui-btn-primary max-md:h-10" :disabled="busy" @click="studio.act(candidate, 'publish')">
              {{ candidate.state === 'failed' ? 'Retry upload' : 'Publish to YouTube' }}
            </button>
            <button v-if="candidate.state === 'published' && candidate.thumbnail_status === 'failed'" type="button" class="ui-btn ui-btn-sm max-md:h-10" :disabled="busy" @click="studio.act(candidate, 'retry-thumbnail')">Retry thumbnail</button>
            <button v-if="canSkip(candidate) && candidate.proposal.format !== 'archive'" type="button" class="ui-btn ui-btn-sm ui-btn-quiet max-md:h-10" :disabled="busy" @click="studio.act(candidate, 'reject')">Skip</button>
            <a v-if="candidate.provider_url" :href="candidate.provider_url" target="_blank" rel="noopener noreferrer" class="ui-btn ui-btn-sm max-md:h-10">Open on YouTube ↗</a>
          </div>
        </article>
      </div>
    </div>
  </section>
</template>
