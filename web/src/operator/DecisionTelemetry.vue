<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { DashboardStats, DecisionEngineSpec } from '../shared/api/types'
import {
  decisionCalibration,
  decisionFeed,
  decisionIdleNote,
  decisionKindRows,
  latency,
  percent
} from './decisionTelemetry'

// Per-run typed-decision telemetry: the stored summary (every call, fixed
// size) plus, when showFeed is set, the live feed of the most recent calls,
// which is never stored. engine is the run's selection, used to explain an
// empty panel before the first call.
const props = withDefaults(defineProps<{
  stats?: DashboardStats
  engine?: DecisionEngineSpec
  showFeed?: boolean
}>(), {
  stats: undefined,
  engine: undefined,
  showFeed: true
})

const rows = computed(() => decisionKindRows(props.stats))
const feed = computed(() => (props.showFeed ? decisionFeed(props.stats) : []))
const totalCalls = computed(() => rows.value.reduce((sum, row) => sum + row.calls, 0))
const mode = computed(() => String(props.stats?.decision_mode || props.engine?.mode || ''))
const model = computed(() => [
  props.stats?.decision_model || props.engine?.inference?.label || props.engine?.deployment,
  props.stats?.decision_backend || props.engine?.backend
].filter(Boolean).join(' · '))

// Calibration is shown for one kind at a time; default to the busiest kind
// that has shadow verdicts.
const calibrationKind = ref('')
watch(rows, (next) => {
  if (next.some((row) => row.kind === calibrationKind.value)) return
  calibrationKind.value = (next.find((row) => row.judged > 0) || next[0])?.kind || ''
}, { immediate: true })
const calibration = computed(() => decisionCalibration(props.stats?.decision_summary?.kinds?.[calibrationKind.value]))
const hasVerdicts = computed(() => calibration.value.some((bin) => bin.judged > 0))

const verdictClass: Record<string, string> = {
  agreed: 'text-[var(--poke-green)]',
  disagreed: 'text-[var(--poke-amber)]',
  active: 'text-[var(--poke-cyan)]',
  unusable: 'text-[var(--poke-muted)]'
}
const verdictGlyph: Record<string, string> = { agreed: '✓', disagreed: '✗', active: 'acted', unusable: '—' }
</script>

<template>
  <section class="min-w-0 bg-[var(--poke-panel)] p-2">
    <div class="mb-1.5 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5">
      <h3 class="text-[9px] tracking-[0.07em] text-[var(--poke-muted)] uppercase">Fast decisions</h3>
      <span class="text-[10px] text-[var(--poke-muted)]">
        {{ [mode, model].filter(Boolean).join(' · ') || '—' }} · {{ totalCalls }} calls
      </span>
    </div>

    <p v-if="!rows.length" class="text-[11px] text-[var(--poke-muted)]">{{ decisionIdleNote(engine) }}</p>

    <div v-else class="overflow-x-auto">
      <table class="w-full min-w-[30rem] text-[11px]">
        <thead>
          <tr class="text-left text-[9px] text-[var(--poke-muted)] uppercase">
            <th class="py-0.5 pr-2 font-normal">kind</th>
            <th class="py-0.5 pr-2 text-right font-normal">calls</th>
            <th class="py-0.5 pr-2 text-right font-normal">agreement</th>
            <th class="py-0.5 pr-2 text-right font-normal">p50 / p95</th>
            <th class="py-0.5 pr-2 text-right font-normal">fallbacks</th>
            <th class="py-0.5 font-normal">engine picks most</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="row in rows"
            :key="row.kind"
            class="cursor-pointer border-t border-[var(--poke-border)]"
            :class="row.kind === calibrationKind ? 'bg-white/[0.03]' : ''"
            @click="calibrationKind = row.kind"
          >
            <td class="py-0.5 pr-2 text-[var(--poke-text)]">{{ row.label }}</td>
            <td class="py-0.5 pr-2 text-right font-mono">{{ row.calls }}</td>
            <td class="py-0.5 pr-2 text-right font-mono">
              {{ percent(row.agreement) }}<span v-if="row.judged" class="text-[var(--poke-muted)]"> of {{ row.judged }}</span>
            </td>
            <td class="py-0.5 pr-2 text-right font-mono">{{ latency(row.p50) }} / {{ latency(row.p95) }}</td>
            <td class="py-0.5 pr-2 text-right font-mono" :class="row.fallbacks ? 'text-[var(--poke-amber)]' : ''">{{ row.fallbacks }}</td>
            <td class="max-w-[14rem] truncate py-0.5 text-[var(--poke-muted)]">
              {{ row.topEngine.map(([label, count]) => `${label} ×${count}`).join(', ') || '—' }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="hasVerdicts" class="mt-2">
      <div class="text-[9px] text-[var(--poke-muted)]">agreement by confidence · {{ rows.find((row) => row.kind === calibrationKind)?.label }}</div>
      <div class="mt-1 flex h-12 items-end gap-px" role="img" :aria-label="`Agreement rate per confidence bucket for ${calibrationKind}`">
        <div
          v-for="bin in calibration"
          :key="bin.from"
          class="relative flex h-full flex-1 flex-col justify-end bg-white/[0.03]"
          :title="`${Math.round(bin.from * 100)}–${Math.round(bin.to * 100)}% confident: ${bin.judged ? `${bin.agreed}/${bin.judged} agreed` : 'no shadow verdicts'} (${bin.answers} answers)`"
        >
          <div v-if="bin.rate !== null" class="bg-[var(--poke-cyan)]" :style="{ height: `${Math.max(4, bin.rate * 100)}%` }" />
        </div>
      </div>
      <div class="mt-0.5 flex justify-between text-[9px] text-[var(--poke-muted)]"><span>0% confident</span><span>100%</span></div>
    </div>

    <div v-if="feed.length" class="mt-2">
      <div class="text-[9px] text-[var(--poke-muted)]">
        live feed · last {{ feed.length }} calls (not stored){{ stats?.decision_records_dropped ? ` · ${stats.decision_records_dropped} earlier calls are in the summary only` : '' }}
      </div>
      <div class="mt-1 max-h-48 overflow-auto">
        <table class="w-full min-w-[30rem] text-[11px]">
          <tbody>
            <tr v-for="(entry, index) in feed" :key="index" class="border-t border-[var(--poke-border)] align-top">
              <td class="py-0.5 pr-2 whitespace-nowrap text-[var(--poke-muted)]">{{ entry.kind }}</td>
              <td class="py-0.5 pr-2 text-[var(--poke-text)]" :title="entry.error">{{ entry.error ? 'error' : entry.choice }}</td>
              <td class="py-0.5 pr-2 text-right font-mono">{{ entry.error ? '—' : percent(entry.confidence) }}</td>
              <td class="py-0.5 pr-2 text-[var(--poke-muted)]">{{ entry.executed ? `ran: ${entry.executed}` : '' }}</td>
              <td class="py-0.5 pr-2 text-center" :class="verdictClass[entry.verdict]">{{ verdictGlyph[entry.verdict] }}</td>
              <td class="py-0.5 text-right font-mono text-[var(--poke-muted)]">{{ latency(entry.seconds) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>
