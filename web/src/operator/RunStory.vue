<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import {
  eventDetail,
  eventGlyphs,
  eventKind,
  eventKindLabels,
  eventMatchesFilter,
  eventTime,
  eventTitle,
  type EventKind,
  type TimelineFilter,
  type TimelineRow
} from './runTimelineFilters'

const props = defineProps<{
  events: TimelineRow[]
  selectedIndex: number
  filter: TimelineFilter
  followLive: boolean
  live: boolean
}>()

const emit = defineEmits<{ select: [index: number] }>()

const list = ref<HTMLElement | null>(null)
const kindColor: Record<EventKind, string> = {
  decision: 'text-fg-2', checkpoint: 'text-warn', progress: 'text-ok', failure: 'text-danger',
  recovery: 'text-warn', skill: 'text-accent', system: 'text-fg-3', event: 'text-fg-3'
}

const entries = computed(() => props.events
  .map((event, index) => ({ event, index, kind: eventKind(event) }))
  .filter(({ event }) => eventMatchesFilter(event, props.filter))
  .reverse())

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function scrollSelected(): void {
  void nextTick(() => {
    // Scroll the list only; scrollIntoView would also move the page.
    const container = list.value
    const node = container?.querySelector<HTMLElement>('[aria-current="true"]')
    if (!container || !node) return
    const top = node.offsetTop
    const bottom = top + node.offsetHeight
    if (top < container.scrollTop) container.scrollTop = top
    else if (bottom > container.scrollTop + container.clientHeight) container.scrollTop = bottom - container.clientHeight
  })
}

watch(() => props.selectedIndex, scrollSelected)
</script>

<template>
  <section class="ui-bay flex min-h-0 flex-col" aria-label="Run story">
    <header class="ui-bay-header">
      <h2>Run story</h2>
      <span class="text-meta font-normal text-fg-2">{{ entries.length }} events · newest first{{ live && followLive ? ' · following live' : '' }}</span>
    </header>
    <ol ref="list" class="relative min-h-0 flex-1 overflow-y-auto">
      <li v-if="!entries.length" class="px-3 py-8 text-center text-meta text-fg-3">
        {{ events.length ? 'No events match this filter.' : 'No persisted semantic events for this run yet.' }}
      </li>
      <li
        v-for="{ event, index, kind } in entries"
        :key="index"
        :aria-current="index === selectedIndex ? 'true' : undefined"
        :class="index === selectedIndex ? 'bg-selected shadow-[inset_2px_0_0_var(--color-accent)]' : ''"
        class="border-b border-rule"
      >
        <button
          type="button"
          class="grid w-full grid-cols-[4.5rem_1rem_minmax(0,1fr)] items-baseline gap-2 px-3 py-2 text-left hover:bg-raised"
          :aria-expanded="index === selectedIndex"
          @click="emit('select', index)"
        >
          <span class="font-mono text-meta text-fg-3">{{ eventTime(event) }}</span>
          <span :class="kindColor[kind]" class="text-[12px]" aria-hidden="true">{{ eventGlyphs[kind] }}</span>
          <span class="min-w-0 truncate">
            <span class="mr-1.5 text-meta text-fg-2">{{ eventKindLabels[kind] }}</span>{{ eventTitle(event) }}
          </span>
        </button>
        <div v-if="index === selectedIndex" class="pr-3 pb-3 pl-3 sm:pl-[6.75rem]">
          <dl class="ui-kv max-w-[72ch]">
            <template v-if="text(event.decision)">
              <dt>Decision</dt><dd class="text-ok">{{ text(event.decision) }}</dd>
            </template>
            <dt>Detail</dt>
            <dd class="whitespace-pre-wrap">{{ eventDetail(event) }}</dd>
            <template v-if="Number(event.frame)">
              <dt>Frame</dt><dd class="font-mono">{{ Number(event.frame).toLocaleString() }}</dd>
            </template>
            <template v-if="Number(event.round)">
              <dt>Round</dt><dd class="font-mono">{{ event.round }}</dd>
            </template>
            <template v-if="Number(event.attempt)">
              <dt>Attempt</dt><dd class="font-mono">{{ event.attempt }}</dd>
            </template>
          </dl>
        </div>
      </li>
    </ol>
  </section>
</template>
