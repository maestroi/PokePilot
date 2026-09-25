<script setup lang="ts">
import { computed } from 'vue'
import { CpuChipIcon } from '@heroicons/vue/20/solid'
import type { SpectatorStats } from '../shared/api/spectator'
import { decisionFeed, decisionKindRows, decisionPausedNote, latency, percent } from '../shared/decisionTelemetry'

// Public view of the run's fast decision engine. In shadow mode it is asked
// the same question as the built-in policy but never presses a button, so the
// card shows how often the two agreed and the last few calls side by side.
const props = defineProps<{ stats?: SpectatorStats }>()

const rows = computed(() => decisionKindRows(props.stats))
const judged = computed(() => rows.value.reduce((sum, row) => sum + row.judged, 0))
const agreed = computed(() => rows.value.reduce((sum, row) => sum + Math.round((row.agreement || 0) * row.judged), 0))
const failed = computed(() => rows.value.reduce((sum, row) => sum + row.errors, 0))
const calls = computed(() => rows.value.reduce((sum, row) => sum + row.calls, 0))
const typical = computed(() => rows.value[0]?.p50 || 0)
const shadow = computed(() => props.stats?.decision_mode !== 'active')
const recent = computed(() => decisionFeed(props.stats).slice(0, 5))
const pausedNote = computed(() => decisionPausedNote(props.stats))
</script>

<template>
  <section class="spectator-card rounded-2xl border p-4">
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <h2 class="flex items-center gap-1.5 text-sm font-black text-white">
          <CpuChipIcon class="size-4 text-cyan-300" aria-hidden="true" />
          Second opinion
        </h2>
        <p class="mt-0.5 text-[10px] leading-4 text-slate-600">
          {{ shadow
            ? 'A fast AI is asked every decision but never presses a button. Does it pick what the run actually did?'
            : 'A fast AI is making these decisions.' }}
        </p>
      </div>
      <span v-if="shadow" class="shrink-0 rounded-full bg-cyan-300/10 px-1.5 py-0.5 text-[8px] font-bold text-cyan-200 ring-1 ring-cyan-300/15">Watching only</span>
    </div>

    <div class="mt-3 grid grid-cols-3 gap-2 text-center">
      <div class="rounded-xl bg-black/20 px-2 py-2 ring-1 ring-white/5">
        <div class="font-mono text-base font-black text-white">{{ percent(judged ? agreed / judged : null) }}</div>
        <div class="text-[9px] text-slate-500">{{ judged ? `same pick, ${agreed} of ${judged}` : 'same pick' }}</div>
      </div>
      <div class="rounded-xl bg-black/20 px-2 py-2 ring-1 ring-white/5">
        <div class="font-mono text-base font-black text-white">{{ latency(typical) }}</div>
        <div class="text-[9px] text-slate-500">typical answer</div>
      </div>
      <div class="rounded-xl bg-black/20 px-2 py-2 ring-1 ring-white/5">
        <div class="font-mono text-base font-black" :class="failed ? 'text-amber-300' : 'text-white'">{{ failed }}</div>
        <div class="text-[9px] text-slate-500">no answer of {{ calls }}</div>
      </div>
    </div>

    <p v-if="pausedNote" class="mt-2 text-[10px] text-amber-300/80">{{ pausedNote }}</p>

    <ul v-if="recent.length" class="mt-3 space-y-1.5">
      <li v-for="(entry, index) in recent" :key="index" class="grid grid-cols-[1.25rem_minmax(0,1fr)_auto] items-start gap-2 text-[11px]">
        <span
          class="mt-0.5 grid size-4 place-items-center rounded-full text-[9px] font-black"
          :class="{
            'bg-emerald-400/15 text-emerald-300': entry.verdict === 'agreed',
            'bg-amber-400/15 text-amber-300': entry.verdict === 'disagreed',
            'bg-white/5 text-slate-500': entry.verdict === 'unusable' || entry.verdict === 'active'
          }"
          :title="entry.verdict === 'agreed' ? 'Same pick' : entry.verdict === 'disagreed' ? 'Different pick' : entry.failure ? 'No answer' : ''"
        >{{ entry.verdict === 'agreed' ? '✓' : entry.verdict === 'disagreed' ? '✗' : '·' }}</span>
        <div class="min-w-0">
          <div v-if="entry.failure" class="truncate text-slate-500">No answer ({{ entry.failure }})</div>
          <div v-else class="truncate text-slate-300" :title="entry.choice">AI: {{ entry.choice }}</div>
          <div v-if="entry.executed && entry.verdict !== 'agreed'" class="truncate text-slate-600" :title="entry.executed">Played: {{ entry.executed }}</div>
        </div>
        <span class="font-mono text-[9px] text-slate-600">{{ entry.failure ? '' : percent(entry.confidence) }}</span>
      </li>
    </ul>
  </section>
</template>
