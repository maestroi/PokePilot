<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'

type TimelineRow = Record<string, unknown>

const props = withDefaults(defineProps<{
  events: TimelineRow[]
  totalFrames?: number
  selectedIndex?: number
}>(), {
  totalFrames: 0,
  selectedIndex: -1
})

const emit = defineEmits<{
  select: [index: number]
}>()

const MIN_ZOOM = 1
const MAX_ZOOM = 32
const zoom = ref(MIN_ZOOM)
const timelineScroll = ref<HTMLElement | null>(null)

function number(value: unknown): number {
  const parsed = Number(value || 0)
  return Number.isFinite(parsed) ? parsed : 0
}

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function eventFrame(event: TimelineRow): number {
  return number(event.frame)
}

function eventRound(event: TimelineRow): number {
  return number(event.round)
}

type EventKind = 'decision' | 'checkpoint' | 'progress' | 'failure' | 'recovery' | 'skill' | 'system' | 'event'

function eventKind(event: TimelineRow): EventKind {
  const source = text(event.source).toLowerCase()
  const value = `${text(event.kind)} ${text(event.type)} ${text(event.message)} ${text(event.detail)}`.toLowerCase()
  if (value.includes('fail') || value.includes('lost') || value.includes('error')) return 'failure'
  if (value.includes('checkpoint')) return 'checkpoint'
  if (source === 'recovery' || value.includes('recover') || value.includes('rollback') || value.includes('resume')) return 'recovery'
  if (source === 'milestone' || value.includes('progress') || value.includes('finish') || value.includes('badge')) return 'progress'
  if (source === 'llm' || value.includes('decision') || value.includes('planning')) return 'decision'
  if (source === 'skill') return 'skill'
  if (source === 'system') return 'system'
  return 'event'
}

function humanize(value: string): string {
  return value.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase())
}

function eventTitle(event: TimelineRow): string {
  const kind = eventKind(event)
  const message = text(event.message)
  const checkpoint = text(event.checkpoint) || text(event.name)
  if (kind === 'checkpoint' && checkpoint) return checkpoint
  if (message) return message
  if (kind === 'decision') return 'LLM decision'
  if (kind === 'failure') return 'Attempt failed'
  if (kind === 'recovery') return 'Recovery action'
  if (kind === 'skill') return 'Skill execution'
  if (kind === 'system') return 'System event'
  if (kind === 'progress') return text(event.type).toLowerCase().includes('finish') ? 'Run finished' : 'Progress recorded'
  return humanize(text(event.type) || text(event.kind) || 'Recorded event')
}

function eventDetail(event: TimelineRow): string {
  const question = text(event.question)
  const decision = text(event.decision)
  const message = text(event.message)
  const detail = text(event.detail)
  const progress = text(event.progress)
  const kind = eventKind(event)
  if (kind === 'decision') return decision || detail || question || message || 'The planner state changed.'
  if (kind === 'failure') return detail || message || question || 'The current attempt stopped with a failure.'
  if (kind === 'recovery') return detail || message || 'The recovery supervisor changed how the campaign will continue.'
  if (kind === 'skill') return detail || message || 'Deterministic gameplay execution.'
  if (kind === 'system') return detail || message || 'Run lifecycle event.'
  if (kind === 'progress') return progress || detail || message || 'Run progress was persisted.'
  if (detail && detail !== eventTitle(event)) return detail
  if (message && message !== eventTitle(event)) return message
  if (question) return question
  if (kind === 'checkpoint') {
    return event.replayable ? 'Restartable checkpoint with paired agent state.' : 'Persisted checkpoint evidence.'
  }
  return 'No additional semantic detail was persisted.'
}

function markerSymbol(event: TimelineRow): string {
  switch (eventKind(event)) {
    case 'checkpoint': return '■'
    case 'failure': return '◆'
    case 'progress': return '●'
    case 'decision': return '○'
    case 'recovery': return '↻'
    case 'skill': return '⚙'
    case 'system': return '·'
    default: return '·'
  }
}

function markerClasses(event: TimelineRow): string {
  switch (eventKind(event)) {
    case 'checkpoint': return 'border-amber-300/50 bg-amber-300/15 text-amber-200 hover:bg-amber-300/25'
    case 'failure': return 'border-rose-300/60 bg-rose-400/15 text-rose-200 hover:bg-rose-400/25'
    case 'progress': return 'border-emerald-300/50 bg-emerald-300/15 text-emerald-200 hover:bg-emerald-300/25'
    case 'decision': return 'border-cyan-300/50 bg-cyan-300/15 text-cyan-100 hover:bg-cyan-300/25'
    case 'recovery': return 'border-amber-300/60 bg-amber-300/15 text-amber-100 hover:bg-amber-300/25'
    case 'skill': return 'border-violet-300/50 bg-violet-300/10 text-violet-200 hover:bg-violet-300/20'
    case 'system': return 'border-slate-500/50 bg-slate-500/15 text-slate-300 hover:bg-slate-500/25'
    default: return 'border-slate-500/50 bg-slate-500/15 text-slate-300 hover:bg-slate-500/25'
  }
}

function compactFrame(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(value >= 10_000_000 ? 0 : 1)}m`
  if (value >= 1_000) return `${(value / 1_000).toFixed(value >= 100_000 ? 0 : 1)}k`
  return String(Math.round(value))
}

type TimelineMark = {
  position: number
  eventIndices: number[]
  representativeIndex: number
  firstFrame: number
  lastFrame: number
}

function eventPriority(event: TimelineRow): number {
  switch (eventKind(event)) {
    case 'failure': return 7
    case 'checkpoint': return 6
    case 'recovery': return 5
    case 'progress': return 4
    case 'decision': return 3
    case 'skill': return 2
    case 'system': return 1
    default: return 0
  }
}

const layout = computed(() => {
  const maximum = Math.max(1, number(props.totalFrames), ...props.events.map(eventFrame))
  const positions = props.events.map((event) => Math.max(0, Math.min(100, 100 * eventFrame(event) / maximum)))
  const collisionDistance = 2.2 / zoom.value
  const sorted = positions
    .map((position, index) => ({ position, index }))
    .sort((a, b) => a.position - b.position || a.index - b.index)

  const marks: TimelineMark[] = []
  let lastPosition = -Infinity

  for (const item of sorted) {
    const previous = marks[marks.length - 1]
    if (previous && item.position - lastPosition < collisionDistance) {
      previous.eventIndices.push(item.index)
      previous.position = previous.eventIndices.reduce((sum, index) => sum + positions[index], 0) / previous.eventIndices.length
      previous.firstFrame = Math.min(previous.firstFrame, eventFrame(props.events[item.index]))
      previous.lastFrame = Math.max(previous.lastFrame, eventFrame(props.events[item.index]))
      const currentRepresentative = props.events[previous.representativeIndex]
      const candidate = props.events[item.index]
      if (
        eventPriority(candidate) > eventPriority(currentRepresentative) ||
        (eventPriority(candidate) === eventPriority(currentRepresentative) && item.index > previous.representativeIndex)
      ) {
        previous.representativeIndex = item.index
      }
    } else {
      marks.push({
        position: item.position,
        eventIndices: [item.index],
        representativeIndex: item.index,
        firstFrame: eventFrame(props.events[item.index]),
        lastFrame: eventFrame(props.events[item.index])
      })
    }
    lastPosition = item.position
  }

  return { positions, marks, totalFrames: maximum }
})

const ticks = computed(() => {
  const divisions = Math.min(80, Math.max(4, Math.round(4 * zoom.value)))
  return Array.from({ length: divisions + 1 }, (_, index) => {
    const percent = 100 * index / divisions
    return {
      percent,
      label: compactFrame(layout.value.totalFrames * percent / 100)
    }
  })
})

const selected = computed(() => props.events[props.selectedIndex] || null)
const canZoomIn = computed(() => zoom.value < MAX_ZOOM)
const canZoomOut = computed(() => zoom.value > MIN_ZOOM)

function focusRatio(): number {
  const event = selected.value || props.events[props.events.length - 1]
  if (!event) return 0.5
  return Math.max(0, Math.min(1, eventFrame(event) / layout.value.totalFrames))
}

function scrollToRatio(ratio: number): void {
  void nextTick(() => {
    const viewport = timelineScroll.value
    if (!viewport) return
    const clamped = Math.max(0, Math.min(1, ratio))
    const maximum = Math.max(0, viewport.scrollWidth - viewport.clientWidth)
    viewport.scrollLeft = Math.max(0, Math.min(maximum, clamped * viewport.scrollWidth - viewport.clientWidth / 2))
  })
}

function setZoom(nextZoom: number, anchorOverride?: number): void {
  const viewport = timelineScroll.value
  const anchor = anchorOverride ?? (viewport && viewport.scrollWidth > viewport.clientWidth
    ? (viewport.scrollLeft + viewport.clientWidth / 2) / viewport.scrollWidth
    : focusRatio())
  zoom.value = Math.max(MIN_ZOOM, Math.min(MAX_ZOOM, nextZoom))
  scrollToRatio(anchor)
}

function activateMark(mark: TimelineMark): void {
  if (mark.eventIndices.length === 1) {
    emit('select', mark.eventIndices[0])
    return
  }
  if (canZoomIn.value) {
    setZoom(zoom.value * 2, mark.position / 100)
    return
  }
  emit('select', mark.representativeIndex)
}

function markTitle(mark: TimelineMark): string {
  if (mark.eventIndices.length === 1) {
    const event = props.events[mark.eventIndices[0]]
    return `${when(event)} · ${eventTitle(event)}`
  }
  const frameRange = mark.firstFrame === mark.lastFrame
    ? `frame ${mark.firstFrame.toLocaleString()}`
    : `frames ${mark.firstFrame.toLocaleString()}–${mark.lastFrame.toLocaleString()}`
  return `${mark.eventIndices.length} events · ${frameRange} · click to zoom`
}

function zoomIn(): void {
  setZoom(zoom.value * 2)
}

function zoomOut(): void {
  setZoom(zoom.value / 2)
}

function fitTimeline(): void {
  setZoom(MIN_ZOOM)
}

watch(() => props.selectedIndex, (index) => {
  if (index < 0 || zoom.value <= MIN_ZOOM) return
  const event = props.events[index]
  if (event) scrollToRatio(eventFrame(event) / layout.value.totalFrames)
})

function sourceLabel(event: TimelineRow): string {
  const source = text(event.source).toLowerCase()
  if (source === 'llm') return 'LLM'
  if (source === 'skill') return 'Skill'
  if (source === 'recovery') return 'Recovery'
  if (source === 'milestone') return 'Milestone'
  if (source === 'system') return 'System'
  return ''
}

function when(event: TimelineRow): string {
  const frame = eventFrame(event)
  if (frame) return `frame ${frame.toLocaleString()}`
  const round = eventRound(event)
  if (round) return `round ${round}`
  const at = number(event.at)
  if (at) return new Date(at * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  return 'recorded'
}
</script>

<template>
  <section class="rounded-md border border-white/8 bg-black/10 p-3">
    <div class="flex flex-wrap items-start justify-between gap-2">
      <div>
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Live + recorded</span>
        <h3 class="mt-0.5 text-sm font-semibold text-slate-200">Run activity</h3>
      </div>
      <div class="flex max-w-full flex-wrap items-center justify-end gap-2">
        <span class="max-w-full truncate font-mono text-[10px] text-slate-500 xl:max-w-xl">
          {{ selected ? `${when(selected)} · ${eventTitle(selected)}` : `${layout.totalFrames.toLocaleString()} frames` }}
        </span>
        <div v-if="events.length" class="flex items-center overflow-hidden rounded-md border border-white/8 bg-black/20" role="group" aria-label="Timeline zoom">
          <button
            type="button"
            class="grid h-6 min-w-6 place-items-center border-r border-white/8 px-1.5 text-xs font-semibold text-slate-400 hover:bg-white/5 hover:text-slate-200 disabled:cursor-not-allowed disabled:opacity-30"
            :disabled="!canZoomOut"
            aria-label="Zoom out timeline"
            title="Zoom out"
            @click="zoomOut"
          >−</button>
          <span class="min-w-9 px-1.5 text-center font-mono text-[9px] text-slate-500">{{ zoom }}×</span>
          <button
            type="button"
            class="grid h-6 min-w-6 place-items-center border-l border-white/8 px-1.5 text-xs font-semibold text-slate-400 hover:bg-white/5 hover:text-slate-200 disabled:cursor-not-allowed disabled:opacity-30"
            :disabled="!canZoomIn"
            aria-label="Zoom in timeline"
            title="Zoom in"
            @click="zoomIn"
          >+</button>
          <button
            v-if="zoom > 1"
            type="button"
            class="h-6 border-l border-white/8 px-2 text-[9px] font-semibold text-slate-500 hover:bg-white/5 hover:text-slate-200"
            title="Fit the full run"
            @click="fitTimeline"
          >Fit</button>
        </div>
      </div>
    </div>

    <div v-if="events.length" class="mt-3">
      <div
        ref="timelineScroll"
        class="overflow-x-auto overscroll-x-contain rounded-md border border-white/8 bg-black/20 focus:outline-none focus:ring-1 focus:ring-cyan-300/30"
        tabindex="0"
        aria-label="Run activity timeline. Scroll horizontally when zoomed."
      >
        <div
          class="relative h-[76px] px-3 pt-5 pb-3"
          :style="{
            width: `${zoom * 100}%`,
            minWidth: '100%'
          }"
        >
        <div class="absolute inset-x-3 bottom-7 h-px bg-white/15" />
        <div v-for="tick in ticks" :key="tick.percent" class="absolute bottom-2 -translate-x-1/2 font-mono text-[9px] text-slate-700" :style="{ left: `${tick.percent}%` }">
          <span class="mb-1 block h-1.5 w-px bg-white/15" />{{ tick.label }}
        </div>
        <button
          v-for="(mark, markIndex) in layout.marks"
          :key="`${mark.firstFrame}-${mark.lastFrame}-${markIndex}`"
          type="button"
          :title="markTitle(mark)"
          :aria-current="mark.eventIndices.includes(selectedIndex) ? 'true' : undefined"
          :class="[
            markerClasses(events[mark.representativeIndex]),
            mark.eventIndices.includes(selectedIndex) ? 'scale-110 ring-2 ring-white/35' : '',
            mark.eventIndices.length > 1 ? 'min-w-6 px-1.5' : 'size-5',
            'absolute z-10 grid h-5 -translate-x-1/2 place-items-center rounded-full border text-[9px] font-bold shadow-sm transition'
          ]"
          :style="{ left: `${mark.position}%`, bottom: '21px' }"
          @click="activateMark(mark)"
        >
          <span v-if="mark.eventIndices.length > 1">{{ mark.eventIndices.length }}</span>
          <span v-else>{{ markerSymbol(events[mark.representativeIndex]) }}</span>
        </button>
        </div>
      </div>
      <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-[9px] font-medium text-slate-600">
        <span>○ LLM</span><span>⚙ Skill</span><span>↻ Recovery</span><span>● Milestone</span><span>■ Checkpoint</span><span>◆ Failure</span>
        <span class="text-slate-700">number = clustered events</span>
        <span v-if="zoom > 1" class="ml-auto text-slate-700">Scroll horizontally to pan</span>
      </div>
    </div>
    <p v-else class="mt-3 py-5 text-center text-xs text-slate-600">No persisted semantic events for this run.</p>

    <div v-if="events.length" class="mt-3 grid gap-3 xl:grid-cols-[minmax(0,1.15fr)_minmax(16rem,0.85fr)]">
      <div class="max-h-56 overflow-y-auto rounded-md border border-white/8 bg-black/10">
        <button
          v-for="(event, index) in events"
          :key="`story-${index}`"
          type="button"
          :aria-current="index === selectedIndex ? 'true' : undefined"
          :class="[
            index === selectedIndex ? 'bg-cyan-300/8' : 'hover:bg-white/[0.025]',
            'grid w-full grid-cols-[5rem_1rem_minmax(0,1fr)] gap-2 border-b border-white/8 px-3 py-2 text-left last:border-b-0'
          ]"
          @click="emit('select', index)"
        >
          <span class="font-mono text-[9px] text-slate-600">{{ when(event) }}</span>
          <span class="text-[10px] text-cyan-200/80">{{ markerSymbol(event) }}</span>
          <span class="min-w-0">
            <span class="flex min-w-0 items-center gap-1.5">
              <span v-if="sourceLabel(event)" class="shrink-0 rounded-sm border border-white/8 bg-white/[0.035] px-1 py-0.5 text-[8px] font-semibold tracking-[0.05em] text-slate-500 uppercase">{{ sourceLabel(event) }}</span>
              <strong class="block min-w-0 truncate text-[11px] font-semibold text-slate-300">{{ eventTitle(event) }}</strong>
            </span>
            <span class="mt-0.5 block truncate text-[10px] text-slate-600">{{ eventDetail(event) }}</span>
          </span>
        </button>
      </div>

      <div class="rounded-md border border-white/8 bg-black/15 p-3">
        <template v-if="selected">
          <div class="flex items-center justify-between gap-3">
            <span class="text-[10px] font-semibold tracking-[0.08em] text-cyan-300/70 uppercase">Selected event</span>
            <span class="font-mono text-[9px] text-slate-600">{{ when(selected) }}</span>
          </div>
          <div class="mt-2 flex items-center gap-2">
            <span v-if="sourceLabel(selected)" class="rounded-sm border border-white/8 bg-white/[0.035] px-1.5 py-0.5 text-[9px] font-semibold tracking-[0.05em] text-slate-500 uppercase">{{ sourceLabel(selected) }}</span>
            <strong class="block text-xs text-slate-200">{{ eventTitle(selected) }}</strong>
          </div>
          <p class="mt-1 max-h-28 overflow-auto whitespace-pre-wrap text-[11px] leading-5 text-slate-400">{{ eventDetail(selected) }}</p>
          <p v-if="number(selected.attempt) || number(selected.recovery_attempt)" class="mt-2 font-mono text-[9px] text-slate-600">
            <span v-if="number(selected.attempt)">attempt {{ number(selected.attempt) }}</span>
            <span v-if="number(selected.recovery_attempt)"> · recovery {{ number(selected.recovery_attempt) }}</span>
          </p>
        </template>
        <p v-else class="py-5 text-center text-xs text-slate-600">Choose a marker to inspect that point in the run.</p>
      </div>
    </div>
  </section>
</template>
