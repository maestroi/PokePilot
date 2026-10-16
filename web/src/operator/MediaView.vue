<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ArrowTopRightOnSquareIcon } from '@heroicons/vue/20/solid'
import { getDashboard } from '../shared/api/client'
import { getBroadcastControl, getBroadcastStatus } from '../shared/api/broadcast'
import StatusPill from '../shared/components/StatusPill.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { isBroadcastOnAir, presentBroadcast } from '../shared/status'
import { operatorHref, spectatorURL } from '../shared/urls'
import MediaBroadcasts from './MediaBroadcasts.vue'
import MediaJobs from './MediaJobs.vue'
import MediaLibrary from './MediaLibrary.vue'
import MediaStudio from './MediaStudio.vue'
import { MEDIA_TABS, elapsedLabel, formatMediaHref, parseMediaState, tabHref, type BroadcastRow, type MediaState } from './mediaQuery'
import { middleTruncate } from './runs'
import { gameTitle } from './operations'

const props = defineProps<{ publicBase?: string }>()

// ---- URL state (tab in the hash, selection and filters in the query) ----------------
const location = ref({ hash: window.location.hash, search: window.location.search })
const state = computed(() => parseMediaState(location.value.hash, location.value.search))

// Legacy links (#media?job=…, #media/render-jobs, #media/replays) resolve to their
// tab; rewrite the address so reload and sharing use the canonical form.
function sync(): void {
  location.value = { hash: window.location.hash, search: window.location.search }
  if (!window.location.hash.startsWith('#media')) return
  const canonical = formatMediaHref(state.value, window.location.search)
  if (canonical !== window.location.search + window.location.hash) {
    window.history.replaceState(null, '', canonical)
    location.value = { hash: window.location.hash, search: window.location.search }
  }
}
function navigate(next: MediaState, replace = false): void {
  const url = formatMediaHref(next, window.location.search)
  if (replace) window.history.replaceState(null, '', url)
  else window.history.pushState(null, '', url)
  window.dispatchEvent(new PopStateEvent('popstate'))
}
onMounted(() => {
  sync()
  window.addEventListener('popstate', sync)
  window.addEventListener('hashchange', sync)
})
onUnmounted(() => {
  window.removeEventListener('popstate', sync)
  window.removeEventListener('hashchange', sync)
  window.clearTimeout(toastTimer)
})

// ---- Broadcasts: actual outbound state for every running run ------------------------
// The on-air strip and the Broadcasts tab share this. Only the replay host's
// reported state counts; a running game alone is never "on air".
const broadcasts = usePollingResource<BroadcastRow[]>(async (signal) => {
  const snapshot = await getDashboard({ active: true }, signal)
  const running = (snapshot.runs || [])
    .filter((run) => run.status === 'running')
    .sort((a, b) => Number(b.queued_at || 0) - Number(a.queued_at || 0))
  return Promise.all(running.map(async (run) => {
    try {
      return { run, status: await getBroadcastStatus(run.run_id, signal), error: '' }
    } catch (cause) {
      return { run, status: null, error: cause instanceof Error ? cause.message : 'Broadcast status unavailable' }
    }
  }))
}, { intervalMs: 3000 })
const destinations = usePollingResource((signal) => getBroadcastControl(signal), { intervalMs: 30000 })
const onAir = computed(() => (broadcasts.data.value || []).filter((row) => isBroadcastOnAir(row.status?.state)))
function destinationLabel(provider?: string): string {
  return destinations.data.value?.destinations.find((item) => item.provider === provider)?.label || provider || 'Destination'
}

// ---- Receipts (one polite live region, mirrored as a toast) --------------------------
const toast = ref('')
let toastTimer = 0
function announce(text: string): void {
  toast.value = text
  window.clearTimeout(toastTimer)
  toastTimer = window.setTimeout(() => { toast.value = '' }, 4000)
}

function openPublicSite(): void {
  if (!props.publicBase) {
    announce('Set POKEPILOT_PUBLIC_BASE_URL on the operator UI to enable public links.')
    return
  }
  window.open(spectatorURL(props.publicBase), '_blank', 'noopener,noreferrer')
}
</script>

<template>
  <div class="space-y-3">
    <header class="flex min-h-12 flex-wrap items-center gap-x-3 gap-y-2">
      <h1 class="text-page font-semibold">Media</h1>
      <div class="ml-auto flex items-center gap-2">
        <button type="button" class="ui-btn max-md:h-10" @click="openPublicSite">
          <ArrowTopRightOnSquareIcon class="size-4" aria-hidden="true" /> Open public site
        </button>
      </div>
    </header>

    <nav class="flex gap-1 overflow-x-auto border-b border-rule" aria-label="Media sections">
      <a
        v-for="tab in MEDIA_TABS"
        :key="tab.id"
        :href="tabHref(state, tab.id, location.search)"
        :aria-current="state.tab === tab.id ? 'page' : undefined"
        :class="state.tab === tab.id ? 'text-fg shadow-[inset_0_-2px_0_var(--color-accent)]' : 'text-fg-2 hover:text-fg'"
        class="flex h-9 shrink-0 items-center px-3 font-medium focus-visible:outline-offset-[-2px] max-md:h-10"
      >{{ tab.label }}</a>
    </nav>

    <!-- On-air strip: only while the replay host reports a broadcast as live -->
    <section v-if="onAir.length" class="border border-rule-strong bg-surface" aria-label="On air">
      <div
        v-for="row in onAir"
        :key="row.run.run_id"
        class="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-rule px-3 py-2 last:border-b-0"
      >
        <span class="font-semibold">On air</span>
        <StatusPill :status="presentBroadcast(row.status?.state)" />
        <span>{{ destinationLabel(row.status?.provider) }}</span>
        <span class="min-w-0 text-fg-2">
          Source <span class="font-mono text-fg" :title="row.run.run_id">{{ middleTruncate(row.run.run_id) }}</span>
          <template v-if="row.run.game"> · {{ gameTitle(row.run.game) }}</template>
        </span>
        <span class="font-mono text-meta text-fg-2" :title="row.status?.started_at_unix_ms ? new Date(row.status.started_at_unix_ms).toISOString() : ''">{{ elapsedLabel(row.status?.started_at_unix_ms) }}</span>
        <div class="ml-auto flex flex-wrap gap-2">
          <a :href="operatorHref('live', { run: row.run.run_id, job: '' }, location.search)" class="ui-btn ui-btn-sm max-md:h-10">Open source run</a>
          <a :href="formatMediaHref({ ...state, tab: 'broadcasts', run: row.run.run_id, job: '' }, location.search)" class="ui-btn ui-btn-sm max-md:h-10">Manage broadcast</a>
        </div>
      </div>
    </section>

    <MediaLibrary
      v-if="state.tab === 'library'"
      :state="state"
      :public-base="publicBase || ''"
      @navigate="navigate"
      @announce="announce"
    />
    <MediaStudio v-else-if="state.tab === 'studio'" :state="state" @navigate="navigate" />
    <MediaBroadcasts
      v-else-if="state.tab === 'broadcasts'"
      :state="state"
      :rows="broadcasts.data.value || []"
      :rows-state="broadcasts.state.value"
      :rows-error="broadcasts.error.value"
      :destinations="destinations.data.value?.destinations || []"
      @navigate="navigate"
      @refresh="broadcasts.retry(); destinations.retry()"
    />
    <MediaJobs v-else :state="state" @navigate="navigate" @announce="announce" />

    <div class="pointer-events-none fixed right-4 bottom-4 z-[80]" aria-live="polite" role="status">
      <div v-if="toast" class="pointer-events-auto rounded-md border border-rule-strong bg-raised px-4 py-2.5 shadow-[0_8px_24px_rgb(0_0_0/45%)]">{{ toast }}</div>
    </div>
  </div>
</template>
