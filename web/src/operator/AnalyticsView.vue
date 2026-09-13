<script setup lang="ts">
import { computed } from 'vue'
import { ArrowPathIcon } from '@heroicons/vue/20/solid'
import { getStats } from '../shared/api/client'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { HOSTED_API_PRICING_UPDATED, hostedApiEquivalentCosts } from '../shared/hostedApiPricing'

type Row = Record<string, any>
type StatsPayload = Record<string, any>

const resource = usePollingResource(
  (signal) => getStats(signal) as Promise<StatsPayload>,
  { intervalMs: 5000 }
)

const stats = computed<StatsPayload>(() => resource.data.value || {})
const llm = computed<StatsPayload>(() => stats.value.llm || {})

function num(value: unknown): number {
  const parsed = Number(value || 0)
  return Number.isFinite(parsed) ? parsed : 0
}

function nfmt(value: unknown): string {
  return num(value).toLocaleString()
}

function pct(value: unknown, total: unknown): string {
  const denominator = num(total)
  if (!denominator) return '—'
  return `${(100 * num(value) / denominator).toFixed(num(value) && num(value) < denominator ? 1 : 0)}%`
}

function seconds(value: unknown): string {
  const n = num(value)
  return n > 0 ? `${n.toFixed(1)}s` : '—'
}

function usd(value: unknown): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(num(value))
}

function array(value: unknown): Row[] {
  return Array.isArray(value) ? value as Row[] : []
}

function profileLabel(value: unknown): string {
  switch (String(value || '').toLowerCase()) {
    case 'auto': return '7900 XTX → CPU'
    case 'gpu': return 'RTX 4090'
    case 'default': return 'CPU only'
    case 'legacy': return 'Legacy / unspecified'
    default: return String(value || 'Unknown')
  }
}

const kpis = computed(() => {
  const s = stats.value
  const missing = Math.max(0, num(s.settled_runs) - num(s.usable_progress_runs))
  return [
    { label: 'Completed attempts', value: nfmt(s.completed_attempts), note: `${nfmt(s.settled_runs)} settled run records` },
    { label: 'Objective wins', value: `${nfmt(s.goal_wins)} / ${nfmt(s.goal_tracked_runs)}`, note: pct(s.goal_wins, s.goal_tracked_runs) },
    { label: 'Reached ≥1 badge', value: `${nfmt(s.at_least_one_badge)} / ${nfmt(s.usable_progress_runs)}`, note: pct(s.at_least_one_badge, s.usable_progress_runs) },
    { label: 'Best badge count', value: `${nfmt(s.best_badges)} / 8`, note: 'highest observed' },
    { label: 'Retry failures', value: nfmt(s.retryable_failure_attempts), note: 'including failures hidden by retries' },
    { label: 'No progress data', value: nfmt(missing), note: 'settled without final player snapshot' }
  ]
})

const llmKpis = computed(() => [
  { label: 'Calls', value: nfmt(llm.value.calls), note: `${nfmt(llm.value.tracked_runs)} run snapshots` },
  { label: 'Average call', value: seconds(llm.value.avg_seconds), note: 'weighted endpoint latency' },
  { label: 'Rejected', value: `${nfmt(llm.value.rejected)} · ${pct(llm.value.rejected, llm.value.calls)}`, note: 'validation / transport / reply errors' },
  { label: 'Token spend', value: `${nfmt(llm.value.prompt_tokens)} / ${nfmt(llm.value.completion_tokens)}`, note: 'prompt / completion' }
])

const hasTokenSpend = computed(() => num(llm.value.prompt_tokens) > 0 || num(llm.value.completion_tokens) > 0)
const hostedApiEstimates = computed(() => hostedApiEquivalentCosts(llm.value.prompt_tokens, llm.value.completion_tokens))
const badgeRows = computed(() => array(stats.value.badge_distribution).filter((row) => num(row.count) > 0))
const reasonRows = computed(() => array(stats.value.terminal_reasons))
const maxBadgeCount = computed(() => Math.max(1, ...badgeRows.value.map((row) => num(row.count))))
const maxReasonCount = computed(() => Math.max(1, ...reasonRows.value.map((row) => num(row.count))))
const profiles = computed(() => array(llm.value.profiles))
const models = computed(() => array(llm.value.latest_models))
const experiments = computed(() => array(stats.value.endless_experiments))

function retry(): void {
  void resource.retry()
}
</script>

<template>
  <div class="space-y-3">
    <ResourceState :state="resource.state.value" title="Analytics unavailable" :message="resource.error.value || 'Farm statistics will recover automatically.'" :rows="7">
      <template #actions>
        <button type="button" class="inline-flex items-center gap-1.5 rounded-md bg-white/10 px-2.5 py-1.5 text-xs font-semibold text-white ring-1 ring-white/10 hover:bg-white/15" @click="retry">
          <ArrowPathIcon class="size-3.5" aria-hidden="true" /> Retry now
        </button>
      </template>

      <Panel title="Run outcomes" :description="`${nfmt(stats.completed_attempts)} completed attempts · ${nfmt(stats.settled_runs)} settled runs`" compact>
        <template #actions>
          <button type="button" class="inline-flex items-center gap-1.5 rounded-md bg-white/8 px-2.5 py-1.5 text-xs font-semibold text-slate-200 ring-1 ring-white/10 hover:bg-white/12" @click="retry">
            <ArrowPathIcon class="size-3.5" aria-hidden="true" /> Refresh
          </button>
        </template>
        <div class="grid grid-cols-2 gap-px overflow-hidden rounded-lg bg-white/10 ring-1 ring-white/10 xl:grid-cols-3 2xl:grid-cols-6">
          <div v-for="item in kpis" :key="item.label" class="bg-[#0b111a] px-4 py-4">
            <dt class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">{{ item.label }}</dt>
            <dd class="mt-1 font-mono text-lg font-semibold tabular-nums text-white">{{ item.value }}</dd>
            <p class="mt-1 text-[11px] leading-4 text-slate-600">{{ item.note }}</p>
          </div>
        </div>
      </Panel>

      <Panel title="LLM workload" description="Latency, token spend, rejection rate, routing profiles, and latest serving models." compact>
        <div class="grid grid-cols-2 gap-px overflow-hidden rounded-lg bg-white/10 ring-1 ring-white/10 xl:grid-cols-4">
          <div v-for="item in llmKpis" :key="item.label" class="bg-[#0b111a] px-4 py-4">
            <dt class="text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">{{ item.label }}</dt>
            <dd class="mt-1 font-mono text-lg font-semibold tabular-nums text-white">{{ item.value }}</dd>
            <p class="mt-1 text-[11px] leading-4 text-slate-600">{{ item.note }}</p>
          </div>
        </div>

        <div v-if="hasTokenSpend" class="mt-3 overflow-hidden rounded-lg border border-white/10 bg-black/15">
          <div class="flex flex-wrap items-start justify-between gap-2 border-b border-white/8 px-3 py-2.5">
            <div>
              <strong class="block text-xs font-semibold text-slate-200">If this ran on hosted APIs…</strong>
              <span class="mt-0.5 block text-[10px] leading-4 text-slate-600">Equivalent cost for the token volume above using standard uncached text-token list pricing.</span>
            </div>
            <span class="rounded bg-white/5 px-2 py-1 text-[9px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Pricing {{ HOSTED_API_PRICING_UPDATED }}</span>
          </div>
          <div class="grid grid-cols-1 gap-px bg-white/8 sm:grid-cols-3">
            <div v-for="estimate in hostedApiEstimates" :key="estimate.model" class="bg-[#0b111a] px-3 py-3">
              <span class="block text-[9px] font-semibold tracking-[0.08em] text-slate-600 uppercase">{{ estimate.provider }}</span>
              <strong class="mt-0.5 block text-xs text-slate-300">{{ estimate.model }}</strong>
              <span class="mt-2 block font-mono text-xl font-semibold tabular-nums text-white">{{ usd(estimate.costUsd) }}</span>
              <span class="mt-1 block font-mono text-[9px] text-slate-600">${{ estimate.inputPerMillion }}/M in · ${{ estimate.outputPerMillion }}/M out</span>
            </div>
          </div>
          <div class="border-t border-white/8 px-3 py-2 text-[10px] text-slate-600">
            Local inference: <strong class="font-mono font-medium text-slate-400">$0 hosted API spend</strong> · hardware and electricity are not included in this comparison.
          </div>
        </div>

        <div class="mt-3 grid grid-cols-2 gap-2 lg:grid-cols-3 2xl:grid-cols-6">
          <div v-for="item in [
            ['Successful avg', seconds(llm.successful_avg_seconds)],
            ['Rejected avg', seconds(llm.rejected_avg_seconds)],
            ['Strategist avg', seconds(llm.strategic_avg_seconds)],
            ['Planner mix', `${nfmt(llm.fast_calls)} fast · ${nfmt(llm.strategic_calls)} strategist`],
            ['Repeat picks', `${nfmt(llm.repeats)} / ${nfmt(llm.rounds)} · ${pct(llm.repeats, llm.rounds)}`],
            ['Reliability', `${nfmt(llm.transport)} transport · ${nfmt(llm.fallbacks)} fallback · ${nfmt(llm.failovers)} failover`]
          ]" :key="item[0]" class="rounded-md border border-white/8 bg-black/15 px-3 py-2.5">
            <span class="block text-[10px] font-semibold tracking-[0.08em] text-slate-600 uppercase">{{ item[0] }}</span>
            <strong class="mt-1 block font-mono text-xs font-medium text-slate-300">{{ item[1] }}</strong>
          </div>
        </div>

        <div class="mt-4 grid grid-cols-1 gap-3 2xl:grid-cols-2">
          <div class="overflow-hidden rounded-md border border-white/8">
            <div class="border-b border-white/8 bg-white/[0.025] px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Routing profiles</div>
            <div v-if="profiles.length" class="overflow-x-auto">
              <table class="min-w-full divide-y divide-white/8 text-left text-xs">
                <thead><tr><th class="px-3 py-2 text-slate-600">Profile</th><th class="px-3 py-2 text-right text-slate-600">Runs</th><th class="px-3 py-2 text-right text-slate-600">Calls</th><th class="px-3 py-2 text-right text-slate-600">Avg</th><th class="px-3 py-2 text-right text-slate-600">Rejected</th></tr></thead>
                <tbody class="divide-y divide-white/8">
                  <tr v-for="profile in profiles" :key="String(profile.profile)">
                    <td class="px-3 py-2.5 text-slate-300">{{ profileLabel(profile.profile) }}</td>
                    <td class="px-3 py-2.5 text-right font-mono text-slate-500">{{ nfmt(profile.runs) }}</td>
                    <td class="px-3 py-2.5 text-right font-mono text-slate-500">{{ nfmt(profile.calls) }}</td>
                    <td class="px-3 py-2.5 text-right font-mono text-slate-500">{{ seconds(profile.avg_seconds) }}</td>
                    <td class="px-3 py-2.5 text-right font-mono text-slate-500">{{ nfmt(profile.rejected) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p v-else class="px-3 py-6 text-center text-xs text-slate-600">No routing-profile telemetry yet.</p>
          </div>

          <div class="overflow-hidden rounded-md border border-white/8">
            <div class="border-b border-white/8 bg-white/[0.025] px-3 py-2 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase">Latest serving models</div>
            <div v-if="models.length" class="divide-y divide-white/8">
              <div v-for="model in models" :key="String(model.name)" class="flex items-center justify-between gap-4 px-3 py-2.5">
                <div class="min-w-0">
                  <strong class="block truncate text-xs text-slate-300" :title="String(model.name || '')">{{ model.name }}</strong>
                  <span class="text-[11px] text-slate-600">{{ model.backend || 'backend not reported' }}</span>
                </div>
                <span class="font-mono text-[11px] text-slate-500">{{ nfmt(model.runs) }} runs</span>
              </div>
            </div>
            <p v-else class="px-3 py-6 text-center text-xs text-slate-600">No serving-model snapshots yet.</p>
          </div>
        </div>
      </Panel>

      <div class="grid grid-cols-1 gap-3 xl:grid-cols-2">
        <Panel title="Badge distribution" description="Final badge counts among runs with usable progress snapshots." compact>
          <div v-if="badgeRows.length" class="space-y-3">
            <div v-for="row in badgeRows" :key="String(row.badges)" class="grid grid-cols-[7rem_minmax(0,1fr)_4.5rem] items-center gap-3">
              <span class="text-xs text-slate-400">{{ row.badges }} badge{{ num(row.badges) === 1 ? '' : 's' }}</span>
              <div class="h-2 overflow-hidden rounded-full bg-white/8"><div class="h-full rounded-full bg-cyan-300" :style="{ width: `${Math.max(2, 100 * num(row.count) / maxBadgeCount)}%` }" /></div>
              <span class="text-right font-mono text-[11px] text-slate-500">{{ nfmt(row.count) }} · {{ pct(row.count, stats.usable_progress_runs) }}</span>
            </div>
          </div>
          <p v-else class="py-6 text-center text-sm text-slate-500">No badge data yet.</p>
        </Panel>

        <Panel title="Terminal outcomes" description="How settled runs ended." compact>
          <div v-if="reasonRows.length" class="space-y-3">
            <div v-for="row in reasonRows" :key="String(row.name)" class="grid grid-cols-[7rem_minmax(0,1fr)_4.5rem] items-center gap-3">
              <span class="truncate text-xs text-slate-400" :title="String(row.name || '')">{{ row.name }}</span>
              <div class="h-2 overflow-hidden rounded-full bg-white/8"><div class="h-full rounded-full bg-amber-300" :style="{ width: `${Math.max(2, 100 * num(row.count) / maxReasonCount)}%` }" /></div>
              <span class="text-right font-mono text-[11px] text-slate-500">{{ nfmt(row.count) }} · {{ pct(row.count, stats.settled_runs) }}</span>
            </div>
          </div>
          <p v-else class="py-6 text-center text-sm text-slate-500">No terminal outcomes yet.</p>
        </Panel>
      </div>

      <Panel title="Endless experiments" description="Successor runs with identical endless settings are grouped as one benchmark." compact>
        <div v-if="experiments.length" class="-mx-3 overflow-x-auto sm:-mx-4">
          <table class="min-w-full divide-y divide-white/10 text-left text-xs">
            <thead><tr><th class="px-3 py-2 text-slate-600 sm:px-4">Experiment</th><th class="px-3 py-2 text-right text-slate-600">Attempts</th><th class="px-3 py-2 text-right text-slate-600">Best</th><th class="px-3 py-2 text-right text-slate-600">≥1 badge</th><th class="px-3 py-2 text-right text-slate-600">Wins</th><th class="px-3 py-2 text-right text-slate-600 sm:pr-4">Retry failures</th></tr></thead>
            <tbody class="divide-y divide-white/8">
              <tr v-for="experiment in experiments" :key="String(experiment.key)">
                <td class="max-w-lg px-3 py-3 sm:px-4"><strong class="block truncate text-slate-300">{{ experiment.goal || experiment.planner || 'endless run' }}</strong><span class="mt-0.5 block truncate font-mono text-[10px] text-slate-600">{{ experiment.key }}</span></td>
                <td class="px-3 py-3 text-right font-mono text-slate-500">{{ nfmt(experiment.completed_attempts) }}</td>
                <td class="px-3 py-3 text-right font-mono text-slate-500">{{ nfmt(experiment.best_badges) }}/8</td>
                <td class="px-3 py-3 text-right font-mono text-slate-500">{{ pct(experiment.at_least_one_badge, experiment.usable_progress_runs) }}</td>
                <td class="px-3 py-3 text-right font-mono text-slate-500">{{ pct(experiment.goal_wins, experiment.goal_tracked_runs) }}</td>
                <td class="px-3 py-3 text-right font-mono text-slate-500 sm:pr-4">{{ nfmt(experiment.retryable_failure_attempts) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else class="py-6 text-center text-sm text-slate-500">No endless experiments recorded yet.</p>
      </Panel>
    </ResourceState>
  </div>
</template>
