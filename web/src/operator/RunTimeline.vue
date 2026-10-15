<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import {
  eventGlyphs,
  eventKind,
  eventKindLabels,
  eventMatchesFilter,
  eventTime,
  eventTitle,
  timelineFilterCounts,
  timelineFilterLabels,
  type EventKind,
  type TimelineFilter,
  type TimelineRow
} from './runTimelineFilters'

// Transport strip: owns chronological navigation for the selected run.
// The story list and the inspector render the selected event.
const props = withDefaults(defineProps<{
  events: TimelineRow[]
  totalFrames?: number
  selectedIndex?: number
  filter: TimelineFilter
  followLive: boolean
  live: boolean
}>(), {
  totalFrames: 0,
  selectedIndex: -1
})

const emit = defineEmits<{
  select: [index: number]
  'update:filter': [filter: TimelineFilter]
  follow: [value: boolean]
}>()

const MIN_ZOOM = 1
const MAX_ZOOM = 32
const zoom = ref(MIN_ZOOM)
const viewport = ref<HTMLElement | null>(null)

const kindColor: Record<EventKind, string> = {
  decision: 'text-fg-2',
  checkpoint: 'text-warn',
  progress: 'text-ok',
  failure: 'text-danger',
  recovery: 'text-warn',
  skill: 'text-accent',
  system: 'text-fg-3',
  event: 'text-fg-3'
}

const kindPriority: Record<EventKind, number> = {
  failure: 7, checkpoint: 6, recovery: 5, progress: 4, decision: 3, skill: 2, system: 1, event: 0
}

function frameOf(event: TimelineRow): number {
  const value = Number(event.frame || 0)
  return Number.isFinite(value) ? value : 0
}

const counts = computed(() => timelineFilterCounts(props.events))
const filterOptions = computed(() => (Object.keys(timelineFilterLabels) as TimelineFilter[])
  .filter((value) => value === 'all' || counts.value[value] > 0)
  .map((value) => ({ value, label: `${timelineFilterLabels[value]} (${counts.value[value]})` })))

const visible = computed(() => props.events
  .map((event, index) => ({ event, index }))
  .filter(({ event }) => eventMatchesFilter(event, props.filter)))

type Mark = { position: number; indices: number[]; representative: number }

const maximum = computed(() => Math.max(1, Number(props.totalFrames) || 0, ...props.events.map(frameOf)))

const marks = computed<Mark[]>(() => {
  const sorted = visible.value
    .map(({ event, index }) => ({ index, position: Math.max(0, Math.min(100, 100 * frameOf(event) / maximum.value)) }))
    .sort((a, b) => a.position - b.position || a.index - b.index)
  const distance = 1.6 / zoom.value
  const result: Mark[] = []
  let last = -Infinity
  for (const item of sorted) {
    const previous = result[result.length - 1]
    if (previous && item.position - last < distance) {
      previous.indices.push(item.index)
      const current = eventKind(props.events[previous.representative])
      const candidate = eventKind(props.events[item.index])
      if (kindPriority[candidate] >= kindPriority[current]) previous.representative = item.index
    } else {
      result.push({ position: item.position, indices: [item.index], representative: item.index })
    }
    last = item.position
  }
  return result
})

const playhead = computed(() => {
  const event = props.events[props.selectedIndex]
  if (!event || props.followLive) return 100
  return Math.max(0, Math.min(100, 100 * frameOf(event) / maximum.value))
})

const ticks = computed(() => [0, 0.25, 0.5, 0.75, 1].map((ratio) => {
  const frame = maximum.value * ratio
  const label = frame >= 1_000_000 ? `${(frame / 1_000_000).toFixed(1)}m` : frame >= 1000 ? `${Math.round(frame / 1000)}k` : String(Math.round(frame))
  return { percent: ratio * 100, label: ratio === 1 && props.live ? 'now' : label }
}))

function markLabel(mark: Mark): string {
  if (mark.indices.length > 1) return `${mark.indices.length} events around ${eventTime(props.events[mark.representative])}. Activate to zoom in.`
  const event = props.events[mark.representative]
  return `${eventTime(event)}, ${eventKindLabels[eventKind(event)].toLowerCase()}: ${eventTitle(event)}`
}

function scrollTo(ratio: number): void {
  void nextTick(() => {
    const node = viewport.value
    if (!node) return
    const max = Math.max(0, node.scrollWidth - node.clientWidth)
    node.scrollLeft = Math.max(0, Math.min(max, ratio * node.scrollWidth - node.clientWidth / 2))
  })
}

function setZoom(next: number, anchor?: number): void {
  zoom.value = Math.max(MIN_ZOOM, Math.min(MAX_ZOOM, next))
  scrollTo(anchor ?? playhead.value / 100)
}

function activate(mark: Mark): void {
  if (mark.indices.length > 1 && zoom.value < MAX_ZOOM) {
    setZoom(zoom.value * 2, mark.position / 100)
    return
  }
  emit('select', mark.representative)
}

function step(direction: -1 | 1): void {
  const list = visible.value.map(({ index }) => index)
  if (!list.length) return
  const position = list.indexOf(props.selectedIndex)
  const next = position < 0 ? (direction > 0 ? list[0] : list[list.length - 1]) : list[Math.max(0, Math.min(list.length - 1, position + direction))]
  emit('select', next)
}

function onFilter(event: Event): void {
  const value = (event.target as HTMLSelectElement).value as TimelineFilter
  emit('update:filter', value)
  const current = props.events[props.selectedIndex]
  if (current && eventMatchesFilter(current, value)) return
  const last = props.events.map((row, index) => ({ row, index })).filter(({ row }) => eventMatchesFilter(row, value)).pop()
  if (last) emit('select', last.index)
}

watch(() => props.selectedIndex, () => {
  if (zoom.value > MIN_ZOOM) scrollTo(playhead.value / 100)
})

defineExpose({ step })
</script>

<template>
  <section class="ui-bay flex flex-wrap items-center gap-x-3 gap-y-2 px-3 py-2" aria-label="Run timeline">
    <label class="flex items-center gap-2">
      <span class="ui-label">Events</span>
      <select
        :value="filter"
        class="h-7 rounded-[3px] border border-control bg-surface px-2 text-body text-fg"
        @change="onFilter"
      >
        <option v-for="option in filterOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
      </select>
    </label>

    <div
      v-if="events.length"
      ref="viewport"
      class="order-last min-w-0 basis-full overflow-x-auto overscroll-x-contain md:order-none md:basis-0 md:flex-1"
      tabindex="0"
      aria-label="Timeline track. Scroll horizontally when zoomed."
    >
      <div class="relative h-11 min-w-full" :style="{ width: `${zoom * 100}%` }">
        <div class="absolute inset-x-2 top-[14px] h-0.5 bg-rule-strong" />
        <div class="absolute left-2 top-[14px] h-0.5 bg-accent/45" :style="{ width: `calc(${playhead}% - 16px * ${playhead / 100})` }" />
        <button
          v-for="(mark, i) in marks"
          :key="`${mark.representative}-${i}`"
          type="button"
          :aria-label="markLabel(mark)"
          :title="markLabel(mark)"
          :aria-current="mark.indices.includes(selectedIndex) ? 'true' : undefined"
          :class="[
            mark.indices.length > 1 ? '' : kindColor[eventKind(events[mark.representative])],
            mark.indices.includes(selectedIndex) ? 'bg-selected outline-2 outline-accent' : 'hover:bg-raised',
            mark.indices.length > 1 ? 'min-w-6 border border-rule-strong bg-raised px-1 text-meta font-semibold text-fg-2' : 'w-[22px]',
            'absolute top-[4px] grid h-[22px] -translate-x-1/2 place-items-center rounded-[3px] text-[12px] leading-none'
          ]"
          :style="{ left: `calc(8px + (100% - 16px) * ${mark.position / 100})` }"
          @click="activate(mark)"
        >
          <span v-if="mark.indices.length > 1">{{ mark.indices.length }}</span>
          <span v-else aria-hidden="true">{{ eventGlyphs[eventKind(events[mark.representative])] }}</span>
        </button>
        <div
          class="pointer-events-none absolute top-0 bottom-3 w-0.5 bg-accent"
          :style="{ left: `calc(8px + (100% - 16px) * ${playhead / 100})` }"
          aria-hidden="true"
        />
        <span
          v-for="tick in ticks"
          :key="tick.percent"
          class="absolute bottom-0 font-mono text-meta text-fg-3"
          :class="tick.percent === 0 ? '' : tick.percent === 100 ? '-translate-x-full' : '-translate-x-1/2'"
          :style="{ left: `calc(8px + (100% - 16px) * ${tick.percent / 100})` }"
        >{{ tick.label }}</span>
      </div>
    </div>
    <p v-else class="flex-1 text-meta text-fg-3">No persisted events for this run yet.</p>

    <div class="flex items-center gap-1">
      <div v-if="events.length" class="flex items-center" role="group" aria-label="Timeline zoom">
        <button type="button" class="ui-btn ui-btn-sm ui-icon-btn" :disabled="zoom <= MIN_ZOOM" aria-label="Zoom out" @click="setZoom(zoom / 2)">−</button>
        <span class="w-8 text-center font-mono text-meta text-fg-2">{{ zoom }}×</span>
        <button type="button" class="ui-btn ui-btn-sm ui-icon-btn" :disabled="zoom >= MAX_ZOOM" aria-label="Zoom in" @click="setZoom(zoom * 2)">+</button>
      </div>
      <button type="button" class="ui-btn ui-btn-sm ui-icon-btn" aria-label="Previous event ([)" title="Previous event ([)" :disabled="!events.length" @click="step(-1)">◀</button>
      <button type="button" class="ui-btn ui-btn-sm ui-icon-btn" aria-label="Next event (])" title="Next event (])" :disabled="!events.length" @click="step(1)">▶</button>
      <button
        v-if="live"
        type="button"
        class="ui-btn ui-btn-sm"
        :class="followLive ? 'bg-selected' : ''"
        :aria-pressed="followLive"
        @click="emit('follow', !followLive)"
      >
        <span :class="followLive ? 'text-ok' : 'text-fg-3'" aria-hidden="true">●</span>
        Follow live
      </button>
    </div>
  </section>
</template>
