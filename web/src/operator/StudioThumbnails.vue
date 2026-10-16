<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  assessStudioRun,
  createStudioPackage,
  getStudioPackage,
  getStudioThumbnails,
  renderStudioThumbnails,
  selectStudioThumbnail,
  studioThumbnailURL
} from '../shared/api/client'
import type { StudioPackage, StudioPackageSummary, StudioThumbnailList } from '../shared/api/types'
import { middleTruncate, timeAgo } from './runs'

const props = defineProps<{ runID: string; packages: StudioPackageSummary[] }>()
const emit = defineEmits<{ changed: [] }>()
const packageHash = ref('')
const pkg = ref<StudioPackage | null>(null)
const thumbnails = ref<StudioThumbnailList | null>(null)
const busy = ref(false)
const error = ref('')
const message = ref('')
const variants = ref(2)
const generatedArt = ref(false)
let generation = 0

const isValidHash = computed(() => /^[0-9a-f]{64}$/.test(packageHash.value.trim()))
const concepts = computed(() => new Map((pkg.value?.thumbnail_concepts || []).map(c => [c.id, c])))
const candidates = computed(() => thumbnails.value?.manifest.candidates || [])
const selection = computed(() => thumbnails.value?.selected_id || '')

watch(() => props.runID, () => {
  generation++
  packageHash.value = ''
  pkg.value = null
  thumbnails.value = null
  error.value = ''
  message.value = ''
})

function friendly(errorValue: unknown): string {
  return errorValue instanceof Error ? errorValue.message : 'Thumbnail service unavailable'
}

async function prepare(): Promise<void> {
  if (!props.runID || busy.value) return
  const run = props.runID
  const id = ++generation
  busy.value = true
  error.value = ''
  message.value = ''
  pkg.value = null
  thumbnails.value = null
  try {
    const result = await assessStudioRun(run)
    if (id !== generation) return
    const created = await createStudioPackage(run, result.assessment.hash)
    if (id !== generation) return
    pkg.value = created.package
    packageHash.value = created.package.hash
    try {
      thumbnails.value = await getStudioThumbnails(run, created.package.hash)
    } catch {
      thumbnails.value = null
    }
    message.value = 'Verified package created from the completed run. Generate thumbnails to review its concepts.'
    emit('changed')
  } catch (cause) {
    if (id === generation) error.value = friendly(cause)
  } finally {
    if (id === generation) busy.value = false
  }
}

async function open(hash: string): Promise<void> {
  packageHash.value = hash
  await load()
}

async function load(): Promise<void> {
  if (!props.runID || !isValidHash.value) return
  const id = ++generation
  const hash = packageHash.value.trim()
  pkg.value = null
  thumbnails.value = null
  error.value = ''
  message.value = ''
  busy.value = true
  try {
    const p = await getStudioPackage(props.runID, hash)
    if (id !== generation) return
    pkg.value = p
    try {
      thumbnails.value = await getStudioThumbnails(props.runID, hash)
    } catch {
      thumbnails.value = null // A valid package need not have thumbnails yet.
    }
  } catch (cause) {
    if (id === generation) error.value = friendly(cause)
  } finally {
    if (id === generation) busy.value = false
  }
}

async function render(): Promise<void> {
  if (!props.runID || !pkg.value || busy.value) return
  const id = ++generation
  const hash = pkg.value.hash
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    const result = await renderStudioThumbnails(props.runID, hash, variants.value, generatedArt.value)
    if (id !== generation) return
    thumbnails.value = result
    const fallbacks = result.manifest.candidates.filter(c => c.artwork_status === 'fallback' || c.artwork_status === 'unconfigured')
    message.value = fallbacks.length
      ? 'Branded images are ready. Optional generated artwork was unavailable for some concepts; those used the built-in illustration.'
      : `${result.manifest.candidates.length} thumbnails generated and ready for review.`
    emit('changed')
  } catch (cause) {
    if (id === generation) error.value = friendly(cause)
  } finally {
    if (id === generation) busy.value = false
  }
}

async function pick(candidateID: string): Promise<void> {
  if (!props.runID || !pkg.value || busy.value) return
  const id = ++generation
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    await selectStudioThumbnail(props.runID, pkg.value.hash, candidateID)
    if (id !== generation) return
    thumbnails.value = await getStudioThumbnails(props.runID, pkg.value.hash)
    message.value = 'Thumbnail selected for review. This does not publish the video.'
    emit('changed')
  } catch (cause) {
    if (id === generation) error.value = friendly(cause)
  } finally {
    if (id === generation) busy.value = false
  }
}
</script>

<template>
  <section class="ui-bay min-w-0 xl:col-span-2" aria-labelledby="thumbnail-review-title">
    <div class="ui-bay-header">
      <h2 id="thumbnail-review-title">Episode package &amp; thumbnails</h2>
      <span class="text-meta text-fg-2">Review-only · 1280 × 720 JPEG</span>
    </div>
    <div class="space-y-4 p-3">
      <p class="max-w-[72ch] text-fg-2">
        A package holds titles, description and thumbnail concepts grounded in the run. Titles and branding render deterministically; nothing is published from here.
      </p>
      <template v-if="runID">
        <ul v-if="packages.length" class="border border-rule" aria-label="Stored packages">
          <li v-for="item in packages" :key="item.hash" class="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-rule px-3 py-2 last:border-b-0" :class="pkg?.hash === item.hash ? 'bg-selected' : ''">
            <span class="font-mono" :title="item.hash">{{ middleTruncate(item.hash, 10, 6) }}</span>
            <span class="text-meta text-fg-2">{{ timeAgo(Number(item.created_at_unix_ms || 0) / 1000) }}</span>
            <span class="ui-pill" :class="item.thumbnails ? 'ui-tone-ok' : 'ui-tone-neutral'">
              <span aria-hidden="true">{{ item.thumbnails ? '✓' : '○' }}</span> {{ item.thumbnails ? (item.thumbnail_selected ? 'Thumbnails · one selected' : 'Thumbnails rendered') : 'No thumbnails' }}
            </span>
            <button type="button" class="ui-btn ui-btn-sm ml-auto max-md:h-10" :disabled="busy || pkg?.hash === item.hash" @click="open(item.hash)">{{ pkg?.hash === item.hash ? 'Open' : 'Review' }}</button>
          </li>
        </ul>
        <div class="flex flex-wrap items-center gap-2">
          <button type="button" class="ui-btn h-10" :class="packages.length ? '' : 'ui-btn-primary'" :disabled="busy" @click="prepare">
            {{ busy && !pkg ? 'Preparing…' : packages.length ? 'Prepare another package' : 'Prepare package from this run' }}
          </button>
          <span class="text-meta text-fg-2">Runs marked archive-only or do-not-publish are not packaged.</span>
        </div>
        <details>
          <summary class="cursor-pointer text-meta text-accent">Load a package by hash</summary>
        <div class="mt-2 flex flex-wrap items-end gap-2">
          <label class="flex min-w-[min(100%,360px)] flex-1 flex-col gap-1 text-meta font-medium" for="thumbnail-package-hash">
            Content package SHA-256
            <input
              id="thumbnail-package-hash"
              v-model.trim="packageHash"
              class="ui-input h-10 w-full min-w-0 font-mono text-sm"
              type="text"
              autocomplete="off"
              spellcheck="false"
              maxlength="64"
              placeholder="Paste a hash from Studio packaging"
              :disabled="busy"
              @keydown.enter.prevent="load"
            />
          </label>
          <button class="ui-btn h-10" type="button" :disabled="!isValidHash || busy" @click="load">
            {{ busy && !pkg ? 'Loading…' : 'Load package' }}
          </button>
        </div>
        </details>
        <p v-if="error" class="border border-danger/40 bg-danger/10 px-3 py-2 text-danger" role="alert">{{ error }}</p>
        <p v-if="message" class="text-fg-2" role="status">{{ message }}</p>
        <template v-if="pkg">
          <div class="flex flex-wrap items-end gap-3 border-t border-rule pt-3">
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
              Request AI artwork where configured
            </label>
            <button class="ui-btn ui-btn-primary h-10" type="button" :disabled="busy || !pkg.thumbnail_concepts?.length" @click="render">
              {{ busy ? 'Working…' : candidates.length ? 'Regenerate thumbnails' : 'Generate thumbnails' }}
            </button>
          </div>
          <p v-if="!candidates.length" class="text-fg-2">
            {{ pkg.thumbnail_concepts.length }} concepts ready. Generate thumbnails to preview them.
          </p>
          <div v-else class="grid gap-3 md:grid-cols-2 2xl:grid-cols-3" aria-label="Thumbnail candidates">
            <article
              v-for="candidate in candidates"
              :key="candidate.id"
              class="min-w-0 border bg-surface"
              :class="selection === candidate.id ? 'border-accent ring-1 ring-accent' : 'border-rule'"
            >
              <img
                :src="studioThumbnailURL(runID, pkg.hash, candidate.id)"
                :alt="`Thumbnail option ${candidate.variant}: ${concepts.get(candidate.concept_id)?.subject || 'verified run moment'}`"
                class="aspect-video w-full bg-well object-contain"
                loading="lazy"
              />
              <div class="space-y-2 p-3">
                <div class="flex items-center justify-between gap-2">
                  <p class="min-w-0 truncate font-medium">{{ concepts.get(candidate.concept_id)?.subject || candidate.concept_id }} · v{{ candidate.variant }}</p>
                  <span v-if="selection === candidate.id" class="text-meta font-semibold text-accent">Selected</span>
                </div>
                <p class="text-meta text-fg-2">
                  {{ candidate.artwork_status === 'generated' ? 'AI art + fixed branding' : 'Built-in artwork + fixed branding' }}
                  · {{ Math.ceil(candidate.bytes / 1024) }} KB
                </p>
                <div class="flex items-center gap-2">
                  <button type="button" class="ui-btn ui-btn-sm" :disabled="busy || selection === candidate.id" @click="pick(candidate.id)">
                    {{ selection === candidate.id ? 'Selected for review' : 'Select candidate' }}
                  </button>
                  <a class="ui-btn ui-btn-sm ui-btn-quiet" :href="studioThumbnailURL(runID, pkg.hash, candidate.id)" target="_blank" rel="noopener noreferrer">View full size</a>
                </div>
              </div>
            </article>
          </div>
        </template>
      </template>
    </div>
  </section>
</template>
