<script setup lang="ts">
import { computed } from 'vue'

export type TimelineRow = Record<string, unknown>

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

function number(value: unknown): number {
  const parsed = Number(value || 0)
  return Number.isFinite(parsed) ? parsed : 0
}

function text(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

export function eventFrame(event: TimelineRow): number {
  return number(event.frame)
}

function eventRound(event: TimelineRow): number {
  return number(event.round)
}

function eventKind(event: TimelineRow): 'decision' | 'checkpoint' | 'progress' | 'failure' | 'event' {
  const value = `${text(event.kind)} ${text(event.type)}`.toLowerCase()
  if (value.includes('fail') || value.includes('lost') || value.includes('error')) return 'failure'
  if (value.includes('checkpoint')) return 'checkpoint'
  if (value.includes('progress') || value.includes('finish')) return 'progress'
  if (value.includes('decision')) return 'decision'
  return 'event'
}

function humanize(value: string): string {
  return value.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase())
}

function eventTitle(event: TimelineRow): string {
  const kind = eventKind(event)
  const checkpoint = text(event.checkpoint) || text(event.name)
  if (kind === 'checkpoint' && checkpoint) return checkpoint
  if (kind === 'decision') return text(event.decision) || 'Planner decision'
  if (kind === 'failure') return text(event.message) || text(event.detail) || 'Run failed'
  if (kind === 'progress') return text(event.message) || 'Progress recorded'
  return text(event.message) || humanize(text(event.type) || text(event.kind) || 'Recorded event')
}

function eventDetail(event: TimelineRow): string {
  const question = text(event.question)
  const decision = text(event.decision)
  const message = text(event.message)
  const detail = text(event.detail)
  const progress = text(event.progress)
  if (question && decision) return question
  if (detail && detail !== eventTitle(event)) return detail
  if (message && message !== eventTitle(event)) return message
  if (progress) return progress
  if (question) return question
  if (eventKind(event) === 'checkpoint') {
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
    default: return '·'
  }
}

function markerClasses(event: TimelineRow): string {
  switch (eventKind(event)) {
    case 'checkpoint': return 'border-amber-300/50 bg-amber-300/15 text-amber-200 hover:bg-amber-300/25'
    case 'failure': return 'border-rose-300/60 bg-rose-400/15 text-rose-200 hover:bg-rose-400/25'
    case 'progress': return 'border-emerald-300/50 bg-emerald-300/15 text-emerald-200 hover:bg-emerald-300/25'
    case 'decision': return 'border-cyan-300/50 bg-cyan-300/15 text-cyan-100 hover:bg-cyan-300/25'
    default: return 'border-slate-500/50 bg-slate-500/15 text-slate-300 hover:bg-slate-500/25'
  }
}

function compactFrame(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(value >= 10_000_000 ? 0 : 1)}m`
  if (value >= 1_000) return `${(value / 1_000).toFixed(value >= 100_000 ? 0 : 1)}k`
  return String(Math.round(value))
}

const layout = computed(() => {
  const maximum = Math.max(1, number(props.totalFrames), ...props.events.map(eventFrame))
  const positions = props.events.map((event) => Math.max(0, Math.min(100, 100 * eventFrame(event) / maximum)))
  const lanes: number[] = []
  const laneEnds: number[] = []
  const collisionDistance = 1.75
  for (const position of positions) {
    let lane = laneEnds.findIndex((end) => position - end >= collisionDistance)
    if (lane < 0) lane = laneEnds.length
    lanes.push(lane)
    laneEnds[lane] = position
  }
  return { positions, lanes, laneCount: Math.max(1, laneEnds.length), totalFrames: maximum }
})

const ticks = computed(() => [0, 25, 50, 75, 100].map((percent) => ({
  percent,
  label: compactFrame(layout.value.totalFrames * percent / 100)
})))

const selected = computed(() => props.events[props.selectedIndex] || null)

function when(event: TimelineRow): string {
  const frame = eventFrame(event)
  if (frame) return `frame ${frame.toLocaleString()}`
  const round = eventRound(event)
  return round ? `round ${round}` : 'recorded'
}
</script>

<template>
  <section class="rounded-md border border-white/8 bg-black/10 p-3">
    <div class="flex flex-wrap items-start justify-between gap-2">
      <div>
        <span class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Playback</span>
        <h3 class="mt-0.5 text-sm font-semibold text-slate-200">Run timeline</h3>
      </div>
      <span class="font-mono text-[10px] text-slate-500">
        {{ selected ? `${when(selected)} · ${eventTitle(selected)}` : `${layout.totalFrames.toLocaleString()} frames` }}
      </span>
    </div>

    <div v-if="events.length" class="mt-3">
      <div class="relative overflow-hidden rounded-md border border-white/8 bg-black/20 px-3 pt-5 pb-3" :style="{ minHeight: `${76 + (layout.laneCount - 1) * 16}px` }">
        <div class="absolute inset-x-3 bottom-7 h-px bg-white/15" />
        <div v-for="tick in ticks" :key="tick.percent" class="absolute bottom-2 -translate-x-1/2 font-mono text-[9px] text-slate-700" :style="{ left: `${tick.percent}%` }">
          <span class="mb-1 block h-1.5 w-px bg-white/15" />{{ tick.label }}
        </div>
        <button
          v-for="(event, index) in events"
          :key="`${eventFrame(event)}-${index}`"
          type="button"
          :title="`${when(event)} · ${eventTitle(event)}`"
          :aria-current="index === selectedIndex ? 'true' : undefined"
          :class="[
            markerClasses(event),
            index === selectedIndex ? 'scale-110 ring-2 ring-white/35' : '',
            'absolute z-10 grid size-5 -translate-x-1/2 place-items-center rounded-full border text-[9px] font-bold shadow-sm transition'
          ]"
          :style="{ left: `${layout.positions[index]}%`, bottom: `${21 + layout.lanes[index] * 16}px` }"
          @click="emit('select', index)"
        >
          {{ markerSymbol(event) }}
        </button>
      </div>
      <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-[9px] font-medium text-slate-600">
        <span>○ Decision</span><span>■ Checkpoint</span><span>● Progress</span><span>◆ Failure</span>
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
            <strong class="block truncate text-[11px] font-semibold text-slate-300">{{ eventTitle(event) }}</strong>
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
          <strong class="mt-2 block text-xs text-slate-200">{{ eventTitle(selected) }}</strong>
          <p class="mt-1 max-h-28 overflow-auto whitespace-pre-wrap text-[11px] leading-5 text-slate-400">{{ eventDetail(selected) }}</p>
        </template>
        <p v-else class="py-5 text-center text-xs text-slate-600">Choose a marker to inspect that point in the run.</p>
      </div>
    </div>
  </section>
</template>
