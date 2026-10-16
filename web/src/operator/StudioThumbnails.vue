<script setup lang="ts">
import { computed, ref } from 'vue'
import { studioEditVideoURL, studioThumbnailURL } from '../shared/api/client'
import type { StudioPackageSummary } from '../shared/api/types'
import { timeAgo } from './runs'
import type { Studio, StudioFormat } from './useStudio'

// The open package: its titles, the edited video and the thumbnail pick.
// Titles and branding render deterministically; nothing publishes from here.
const props = defineProps<{ runID: string; studio: Studio; packages: StudioPackageSummary[] }>()
const variants = ref(2)
const generatedArt = ref(false)
const another = ref<StudioFormat>('episode')

const pkg = computed(() => props.studio.pkg.value)
const isShort = computed(() => props.studio.profile.value === 'short')
const titles = computed(() => isShort.value ? (pkg.value?.shorts || []).map((s) => ({ id: s.id, title: s.title, note: s.hook })) : (pkg.value?.titles || []).map((t) => ({ id: t.id, title: t.title, note: t.angle })))
const concepts = computed(() => new Map((pkg.value?.thumbnail_concepts || []).map((c) => [c.id, c])))
const candidates = computed(() => props.studio.thumbnails.value?.manifest.candidates || [])
const selection = computed(() => props.studio.selectedThumbnail.value)
const busy = computed(() => props.studio.busy.value)

function packageLabel(item: StudioPackageSummary): string {
  const when = item.created_at_unix_ms ? `Made ${timeAgo(Number(item.created_at_unix_ms) / 1000)}` : 'Package'
  return `${when} · ${item.thumbnail_selected ? 'thumbnail picked' : item.thumbnails ? 'thumbnails rendered' : 'no thumbnails'}`
}
</script>

<template>
  <section v-if="pkg" class="ui-bay min-w-0" aria-labelledby="studio-package-title">
    <div class="ui-bay-header">
      <h2 id="studio-package-title">{{ isShort ? 'Short' : 'Episode' }} package</h2>
      <select
        v-if="packages.length > 1"
        :value="pkg.hash"
        class="ui-input h-7 max-w-[60%] text-meta"
        aria-label="Stored packages"
        :disabled="busy"
        @change="studio.open(($event.target as HTMLSelectElement).value)"
      >
        <option v-for="item in packages" :key="item.hash" :value="item.hash" :title="item.hash">{{ packageLabel(item) }}</option>
      </select>
    </div>
    <div class="space-y-4 p-3">
      <div class="grid gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <div class="min-w-0">
          <h3 class="mb-1 text-meta font-semibold text-fg-2">{{ isShort ? 'Short titles' : 'Episode titles' }}</h3>
          <ul class="space-y-1">
            <li v-for="item in titles" :key="item.id" class="min-w-0">
              <p class="font-medium">{{ item.title }}</p>
              <p v-if="item.note" class="text-meta text-fg-2">{{ item.note }}</p>
            </li>
          </ul>
          <p v-if="!pkg.plan_hash" class="mt-2 text-meta text-warn">▲ Made without an edit plan, so it can’t be published. Prepare a new package.</p>
        </div>
        <div v-if="pkg.plan_hash" class="min-w-0">
          <h3 class="mb-1 text-meta font-semibold text-fg-2">Edited video</h3>
          <video
            v-if="studio.edit.value?.state === 'ready'"
            :key="pkg.plan_hash"
            :src="studioEditVideoURL(runID, pkg.plan_hash)"
            controls
            preload="metadata"
            class="max-h-[45vh] w-full bg-well"
            :class="isShort ? 'aspect-[9/16] max-w-60' : 'aspect-video'"
            :aria-label="`Edited ${isShort ? 'Short' : 'episode'} of ${runID}`"
          />
          <p v-else class="text-meta text-fg-2">
            {{ studio.edit.value?.state === 'generating' ? 'Rendering…' : studio.edit.value?.error || 'Not rendered yet.' }}
          </p>
        </div>
      </div>

      <div class="space-y-3 border-t border-rule pt-3">
        <div class="flex flex-wrap items-end gap-3">
          <h3 class="mr-auto font-semibold">Thumbnails<span v-if="isShort" class="ml-2 text-meta font-normal text-fg-3">optional for Shorts</span></h3>
          <label class="flex flex-col gap-1 text-meta" for="thumbnail-variants">
            Variants per concept
            <select id="thumbnail-variants" v-model.number="variants" class="ui-input h-10 min-w-24" :disabled="busy">
              <option :value="1">1</option>
              <option :value="2">2</option>
              <option :value="3">3</option>
            </select>
          </label>
          <label class="inline-flex h-10 items-center gap-2 text-sm">
            <input v-model="generatedArt" type="checkbox" :disabled="busy" />
            AI artwork where configured
          </label>
          <button class="ui-btn h-10" :class="candidates.length ? '' : 'ui-btn-primary'" type="button" :disabled="busy || !pkg.thumbnail_concepts?.length" @click="studio.renderThumbnails(variants, generatedArt)">
            {{ busy ? 'Working…' : candidates.length ? 'Regenerate' : 'Generate thumbnails' }}
          </button>
        </div>
        <p v-if="!candidates.length" class="text-meta text-fg-2">{{ pkg.thumbnail_concepts.length }} concepts grounded in the run, ready to render at 1280 × 720.</p>
        <div v-else class="grid gap-3 md:grid-cols-2 2xl:grid-cols-3" aria-label="Thumbnail candidates">
          <article
            v-for="candidate in candidates"
            :key="candidate.id"
            class="min-w-0 border bg-surface"
            :class="selection === candidate.id ? 'border-accent ring-1 ring-accent' : 'border-rule'"
          >
            <a :href="studioThumbnailURL(runID, pkg.hash, candidate.id)" target="_blank" rel="noopener noreferrer" title="View full size">
              <img
                :src="studioThumbnailURL(runID, pkg.hash, candidate.id)"
                :alt="`Thumbnail option ${candidate.variant}: ${concepts.get(candidate.concept_id)?.subject || 'verified run moment'}`"
                class="aspect-video w-full bg-well object-contain"
                loading="lazy"
              />
            </a>
            <div class="flex items-center gap-2 p-2">
              <div class="min-w-0 flex-1">
                <p class="truncate font-medium">{{ concepts.get(candidate.concept_id)?.subject || candidate.concept_id }} · v{{ candidate.variant }}</p>
                <p class="text-meta text-fg-2">{{ candidate.artwork_status === 'generated' ? 'AI art' : 'Built-in art' }} · {{ Math.ceil(candidate.bytes / 1024) }} KB</p>
              </div>
              <span v-if="selection === candidate.id" class="text-meta font-semibold text-accent">✓ Picked</span>
              <button v-else type="button" class="ui-btn ui-btn-sm" :disabled="busy" @click="studio.pickThumbnail(candidate.id)">Pick</button>
            </div>
          </article>
        </div>
      </div>

      <details class="border-t border-rule pt-3">
        <summary class="cursor-pointer text-meta text-accent">Prepare another package</summary>
        <div class="mt-2 flex flex-wrap items-center gap-2">
          <select v-if="studio.verdict.value !== 'short_only'" v-model="another" class="ui-input h-10" aria-label="Format of the new package" :disabled="busy">
            <option value="episode">Full episode · landscape</option>
            <option value="short">Short · portrait</option>
          </select>
          <button type="button" class="ui-btn h-10" :disabled="busy" @click="studio.prepare(another)">{{ busy ? 'Preparing…' : 'Prepare' }}</button>
        </div>
      </details>
    </div>
  </section>
</template>
