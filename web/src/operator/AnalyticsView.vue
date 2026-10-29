<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ArrowPathIcon, PlusIcon } from '@heroicons/vue/20/solid'
import { getStats } from '../shared/api/client'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusPill from '../shared/components/StatusPill.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { hostedApiEquivalentCosts, loadHostedApiPrices, type HostedApiPrice } from '../shared/hostedApiPricing'
import { presentOutcome } from '../shared/status'
import { operatorHref } from '../shared/urls'
import PairedExperimentPanel from './PairedExperimentPanel.vue'
import LeaderboardPanel from './LeaderboardPanel.vue'

const props = defineProps<{ section?: string; experimentId?: string }>()
const activeSection = computed(() => props.section === 'experiments' ? 'experiments' : 'overview')
const TABS = [{ id: 'overview', label: 'Overview' }, { id: 'experiments', label: 'Experiments' }]

// The setup form lives in the Experiments detail pane; the page header owns its entry point.
const creating = ref(false)
watch(() => props.experimentId, (id) => { if (id) creating.value = false })

type Row = Record<string, any>
type StatsPayload = Record<string, any>

const resource = usePollingResource(
  (signal) => getStats(signal) as Promise<StatsPayload>,
  { intervalMs: 5000 }
)

const stats = computed<StatsPayload>(() => resource.data.value || {})
const llm = computed<StatsPayload>(() => stats.value.llm || {})
const loaded = computed(() => resource.state.value !== 'loading' && resource.state.value !== 'error' && resource.state.value !== 'empty')

function num(value: unknown): number {
  const parsed = Number(value || 0)
  return Number.isFinite(parsed) ? parsed : 0
}

function nfmt(value: unknown): string {
  return num(value).toLocaleString('en-US')
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
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD', minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(num(value))
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

// One-line health strip of readouts (no KPI tiles, DESIGN §5).
const health = computed(() => {
  const s = stats.value
  const missing = Math.max(0, num(s.settled_runs) - num(s.usable_progress_runs))
  return [
    { label: 'Completed attempts', value: nfmt(s.completed_attempts), title: `${nfmt(s.settled_runs)} settled run records` },
    { label: 'Objective wins', value: `${nfmt(s.goal_wins)} / ${nfmt(s.goal_tracked_runs)} · ${pct(s.goal_wins, s.goal_tracked_runs)}`, title: 'Among runs that tracked a goal' },
    { label: '≥1 badge', value: `${nfmt(s.at_least_one_badge)} / ${nfmt(s.usable_progress_runs)} · ${pct(s.at_least_one_badge, s.usable_progress_runs)}`, title: 'Among runs with a final progress snapshot' },
    { label: 'Best badges', value: nfmt(s.best_badges), title: 'Highest badge count observed' },
    { label: 'Retry failures', value: nfmt(s.retryable_failure_attempts), title: 'Including failures hidden by retries' },
    { label: 'No progress data', value: nfmt(missing), title: 'Settled without a final player snapshot' }
  ]
})

// Terminal reasons as one stacked bar + table. Tone comes from the shared status
// model, so colour only appears where it means goal complete or failure.
const outcomeRows = computed(() => {
  const total = num(stats.value.settled_runs) || array(stats.value.terminal_reasons).reduce((sum, row) => sum + num(row.count), 0)
  return array(stats.value.terminal_reasons)
    .filter((row) => num(row.count) > 0)
    .sort((a, b) => num(b.count) - num(a.count))
    .map((row) => ({
      name: String(row.name || ''),
      count: num(row.count),
      share: total ? 100 * num(row.count) / total : 0,
      status: presentOutcome({ reason: String(row.name || '') })
    }))
})
const SEGMENT_FILL: Record<string, string> = { ok: 'bg-ok', danger: 'bg-danger', warn: 'bg-warn', accent: 'bg-accent', neutral: 'bg-fg-3' }

const badgeRows = computed(() => array(stats.value.badge_distribution).filter((row) => num(row.count) > 0))
const maxBadgeCount = computed(() => Math.max(1, ...badgeRows.value.map((row) => num(row.count))))

const workload = computed(() => {
  const l = llm.value
  return [
    ['Calls', `${nfmt(l.calls)} · ${nfmt(l.tracked_runs)} run snapshots`],
    ['Average call', `${seconds(l.avg_seconds)} weighted endpoint latency`],
    ['Successful / rejected avg', `${seconds(l.successful_avg_seconds)} / ${seconds(l.rejected_avg_seconds)}`],
    ['Strategist avg', seconds(l.strategic_avg_seconds)],
    ['Rejected', `${nfmt(l.rejected)} · ${pct(l.rejected, l.calls)}`],
    ['Planner mix', `${nfmt(l.fast_calls)} fast · ${nfmt(l.strategic_calls)} strategist`],
    ['Repeat picks', `${nfmt(l.repeats)} / ${nfmt(l.rounds)} · ${pct(l.repeats, l.rounds)}`],
    ['Reliability', `${nfmt(l.transport)} transport · ${nfmt(l.fallbacks)} fallback · ${nfmt(l.failovers)} failover`],
    ['Prompt / completion tokens', `${nfmt(l.prompt_tokens)} / ${nfmt(l.completion_tokens)}`]
  ]
})
const hasTokenSpend = computed(() => num(llm.value.prompt_tokens) > 0 || num(llm.value.completion_tokens) > 0)
const catalogPrices = ref<HostedApiPrice[]>([])
const catalogLoading = ref(false)
const catalogError = ref('')
const catalogStale = ref(false)
const catalogFetchedAt = ref<number | null>(null)
const modelSearch = ref('')
const modelSort = ref('popular')
const visibleModelCount = ref(10)
// Blank fields use live farm totals; override them to compare a different token budget.
const comparisonPromptTokens = ref('')
const comparisonCompletionTokens = ref('')
const estimatePromptTokens = computed(() => comparisonPromptTokens.value === '' ? num(llm.value.prompt_tokens) : num(comparisonPromptTokens.value))
const estimateCompletionTokens = computed(() => comparisonCompletionTokens.value === '' ? num(llm.value.completion_tokens) : num(comparisonCompletionTokens.value))
const hostedApiEstimates = computed(() => hostedApiEquivalentCosts(estimatePromptTokens.value, estimateCompletionTokens.value, catalogPrices.value))
const filteredHostedEstimates = computed(() => {
  const query = modelSearch.value.trim().toLowerCase()
  const rows = hostedApiEstimates.value.filter(row =>
    !query || [row.id, row.model, row.provider].some(part => part.toLowerCase().includes(query)))
  if (modelSort.value === 'newest') rows.sort((a, b) => (b.created || 0) - (a.created || 0))
  if (modelSort.value === 'cheapest') rows.sort((a, b) =>
    (estimatePromptTokens.value + estimateCompletionTokens.value > 0
      ? a.costUsd - b.costUsd
      : (a.inputPerMillion + a.outputPerMillion) - (b.inputPerMillion + b.outputPerMillion)))
  if (modelSort.value === 'context') rows.sort((a, b) => (b.contextLength || 0) - (a.contextLength || 0))
  return rows
})
const visibleHostedEstimates = computed(() => filteredHostedEstimates.value.slice(0, visibleModelCount.value))
watch([modelSearch, modelSort], () => { visibleModelCount.value = 10 })

async function refreshHostedApiPrices(force = false): Promise<void> {
  if (catalogLoading.value) return
  catalogLoading.value = true
  catalogError.value = ''
  try {
    const catalog = await loadHostedApiPrices(force)
    catalogPrices.value = catalog.models
    catalogFetchedAt.value = catalog.fetchedAt
    catalogStale.value = catalog.stale
    if (catalog.stale) catalogError.value = 'Refresh failed; showing previously fetched rates.'
  } catch (error) {
    catalogError.value = error instanceof Error ? error.message : 'Could not load OpenRouter prices'
  } finally {
    catalogLoading.value = false
  }
}
// Only the overview fetches the external catalog; stats polling and other tabs never wait on it.
watch(activeSection, section => {
  if (section === 'overview' && !catalogPrices.value.length) void refreshHostedApiPrices()
}, { immediate: true })

function usdEstimate(value: number): string {
  return value > 0 && value < 0.01 ? `${value.toFixed(6)}` : usd(value)
}
function pricePerMillion(value: number): string {
  return `${value.toLocaleString('en-US', { maximumFractionDigits: 4 })}`
}
function contextLabel(value: number | null): string {
  if (!value) return 'Context unknown'
  return `${nfmt(value)} ctx`
}
const profiles = computed(() => array(llm.value.profiles))
const models = computed(() => array(llm.value.latest_models))
const endless = computed(() => array(stats.value.endless_experiments))

function retry(): void {
  void resource.retry()
}
function operatorSearch(): string { return window.location.search }
</script>

<template>
  <div class="space-y-3">
    <header class="flex min-h-12 flex-wrap items-center gap-x-3 gap-y-2">
      <h1 class="text-page font-semibold">Analytics</h1>
      <span v-if="loaded" class="text-fg-2">{{ nfmt(stats.completed_attempts) }} completed attempts</span>
      <span v-if="resource.state.value === 'refreshing'" class="inline-flex items-center gap-1.5 text-meta text-fg-2" role="status">
        <span class="size-1.5 bg-accent motion-safe:animate-pulse" aria-hidden="true" /> Refreshing
      </span>
      <div class="ml-auto flex items-center gap-2">
        <button type="button" class="ui-btn ui-icon-btn max-md:size-10" aria-label="Refresh analytics" title="Refresh" @click="retry">
          <ArrowPathIcon class="size-4" aria-hidden="true" />
        </button>
        <button
          v-if="activeSection === 'experiments' && !creating"
          type="button"
          class="ui-btn ui-btn-primary max-md:h-10"
          @click="creating = true"
        >
          <PlusIcon class="size-4" aria-hidden="true" /> Start paired experiment
        </button>
      </div>
    </header>

    <nav class="flex gap-1 overflow-x-auto border-b border-rule" aria-label="Analytics sections">
      <a
        v-for="tab in TABS"
        :key="tab.id"
        :href="operatorHref('analytics', { section: tab.id }, operatorSearch())"
        :aria-current="activeSection === tab.id ? 'page' : undefined"
        :class="activeSection === tab.id ? 'text-fg shadow-[inset_0_-2px_0_var(--color-accent)]' : 'text-fg-2 hover:text-fg'"
        class="flex h-9 shrink-0 items-center px-3 font-medium focus-visible:outline-offset-[-2px] max-md:h-10"
      >{{ tab.label }}</a>
    </nav>

    <template v-if="activeSection === 'experiments'">
      <PairedExperimentPanel v-model:creating="creating" :experiment-id="experimentId" />

      <section class="ui-bay" aria-labelledby="endless-title">
        <div class="ui-bay-header">
          <h2 id="endless-title" class="flex items-center gap-2">Endless-run cohorts <span class="ui-tag font-normal">Endless · not matched</span></h2>
          <span class="text-meta font-normal text-fg-2">{{ endless.length ? `${endless.length} cohorts` : '' }}</span>
        </div>
        <p class="border-b border-rule px-3 py-2 text-meta text-fg-2">Independent endless-successor runs grouped by setup. Unlike paired experiments, seeds are not matched, so compare them with care.</p>
        <ResourceState v-if="!loaded && resource.state.value !== 'empty'" class="p-3" :state="resource.state.value" title="Endless-run cohorts unavailable" :message="`${resource.error.value || 'Aggregate statistics'} will recover automatically.`" :rows="3">
          <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="retry">Retry</button></template>
        </ResourceState>
        <div v-else-if="endless.length" class="overflow-x-auto">
          <table class="w-full min-w-[640px] border-collapse text-left">
            <thead>
              <tr class="h-9 border-b border-rule-strong bg-raised text-meta text-fg-2">
                <th class="px-3 font-medium">Cohort</th>
                <th class="w-24 px-3 text-right font-medium">Attempts</th>
                <th class="w-24 px-3 text-right font-medium">Best badges</th>
                <th class="w-24 px-3 text-right font-medium">≥1 badge</th>
                <th class="w-24 px-3 text-right font-medium">Wins</th>
                <th class="w-28 px-3 text-right font-medium">Retry failures</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="cohort in endless" :key="String(cohort.key)" class="h-14 border-b border-rule last:border-b-0">
                <td class="max-w-0 px-3">
                  <div class="truncate font-semibold" :title="String(cohort.goal || cohort.planner || '')">{{ cohort.goal || cohort.planner || 'Endless run' }}</div>
                  <div class="truncate font-mono text-meta text-fg-2" :title="String(cohort.key)">{{ cohort.key }}</div>
                </td>
                <td class="px-3 text-right font-mono">{{ nfmt(cohort.completed_attempts) }}</td>
                <td class="px-3 text-right font-mono">{{ nfmt(cohort.best_badges) }}</td>
                <td class="px-3 text-right font-mono">{{ pct(cohort.at_least_one_badge, cohort.usable_progress_runs) }}</td>
                <td class="px-3 text-right font-mono">{{ pct(cohort.goal_wins, cohort.goal_tracked_runs) }}</td>
                <td class="px-3 text-right font-mono">{{ nfmt(cohort.retryable_failure_attempts) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else class="px-3 py-8 text-center text-fg-2">No endless-run cohorts recorded yet.</p>
      </section>
    </template>

    <template v-else>
      <ResourceState v-if="resource.state.value === 'stale'" state="stale" title="Showing the last loaded statistics" :message="`Refresh failed: ${resource.error.value}`">
        <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="retry">Retry</button></template>
      </ResourceState>
      <ResourceState v-if="!loaded" :state="resource.state.value" title="Couldn’t load analytics" :message="`${resource.error.value || 'Farm statistics are unavailable'}. Runs and Live keep working; this page retries automatically.`" :rows="6">
        <template #actions><button type="button" class="ui-btn ui-btn-sm" @click="retry">Retry</button></template>
      </ResourceState>

      <template v-else>
        <!-- Health strip -->
        <dl class="flex flex-wrap gap-x-6 gap-y-2 border border-rule-strong bg-surface px-3 py-2" aria-label="Farm health">
          <div v-for="item in health" :key="item.label" class="flex items-baseline gap-2" :title="item.title">
            <dt class="ui-label">{{ item.label }}</dt>
            <dd class="m-0 font-mono font-medium">{{ item.value }}</dd>
          </div>
        </dl>

        <div class="grid grid-cols-1 gap-3 xl:grid-cols-2">
          <section class="ui-bay" aria-labelledby="outcomes-title">
            <div class="ui-bay-header">
              <h2 id="outcomes-title">Run outcomes</h2>
              <span class="text-meta font-normal text-fg-2">{{ nfmt(stats.settled_runs) }} settled runs</span>
            </div>
            <template v-if="outcomeRows.length">
              <div class="px-3 pt-3 pb-2">
                <div class="flex h-3 gap-0.5" role="img" :aria-label="`Run outcomes: ${outcomeRows.map((row) => `${row.status.label} ${row.count}`).join(', ')}`">
                  <span
                    v-for="row in outcomeRows"
                    :key="row.name"
                    :class="SEGMENT_FILL[row.status.tone]"
                    class="min-w-0.5 first:rounded-l-[3px] last:rounded-r-[3px]"
                    :style="{ flexGrow: row.count }"
                    :title="`${row.status.label}: ${nfmt(row.count)} · ${row.share.toFixed(1)}%`"
                  />
                </div>
              </div>
              <table class="w-full border-collapse text-left">
                <thead>
                  <tr class="h-9 border-y border-b-rule-strong border-t-rule bg-raised text-meta text-fg-2">
                    <th class="px-3 font-medium">Outcome</th>
                    <th class="w-24 px-3 text-right font-medium">Runs</th>
                    <th class="w-24 px-3 text-right font-medium">Share</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="row in outcomeRows" :key="row.name" class="h-10 border-b border-rule last:border-b-0">
                    <td class="px-3"><StatusPill :status="row.status" /></td>
                    <td class="px-3 text-right font-mono">{{ nfmt(row.count) }}</td>
                    <td class="px-3 text-right font-mono text-fg-2">{{ row.share.toFixed(1) }}%</td>
                  </tr>
                </tbody>
              </table>
            </template>
            <p v-else class="px-3 py-8 text-center text-fg-2">No settled run outcomes yet.</p>
          </section>

          <section class="ui-bay" aria-labelledby="badges-title">
            <div class="ui-bay-header">
              <h2 id="badges-title">Badge distribution</h2>
              <span class="text-meta font-normal text-fg-2">{{ nfmt(stats.usable_progress_runs) }} runs with progress</span>
            </div>
            <ul v-if="badgeRows.length" class="space-y-2 p-3" aria-label="Final badge counts">
              <li v-for="row in badgeRows" :key="String(row.badges)" class="grid grid-cols-[6rem_minmax(0,1fr)_7.5rem] items-center gap-3" :title="`${nfmt(row.count)} runs ended with ${row.badges} badge${num(row.badges) === 1 ? '' : 's'}`">
                <span class="text-fg-2">{{ row.badges }} badge{{ num(row.badges) === 1 ? '' : 's' }}</span>
                <span class="h-3"><span class="block h-full rounded-r-[3px] bg-fg-3" :style="{ width: `${Math.max(1, 100 * num(row.count) / maxBadgeCount)}%` }" /></span>
                <span class="text-right font-mono text-meta">{{ nfmt(row.count) }} <span class="text-fg-2">· {{ pct(row.count, stats.usable_progress_runs) }}</span></span>
              </li>
            </ul>
            <p v-else class="px-3 py-8 text-center text-fg-2">No badge data yet.</p>
          </section>

          <section class="ui-bay xl:col-span-2" aria-labelledby="llm-title">
            <div class="ui-bay-header">
              <h2 id="llm-title">LLM workload</h2>
              <span class="text-meta font-normal text-fg-2">{{ nfmt(llm.calls) }} calls</span>
            </div>
            <div class="grid grid-cols-1 gap-px bg-rule lg:grid-cols-2">
              <div class="bg-surface px-3 pb-3">
                <h3 class="mt-3 mb-1 text-title font-semibold">Calls and reliability</h3>
                <dl class="ui-kv">
                  <template v-for="item in workload" :key="item[0]">
                    <dt>{{ item[0] }}</dt><dd class="font-mono">{{ item[1] }}</dd>
                  </template>
                </dl>
              </div>
              <div class="bg-surface min-w-0 px-3 pb-3">
                <h3 class="mt-3 mb-1 flex flex-wrap items-baseline justify-between gap-2 text-title font-semibold">
                  If this ran on hosted APIs
                  <span class="text-meta font-normal text-fg-2">Live OpenRouter list rates</span>
                </h3>
                <p class="mb-2 text-meta text-fg-2">Estimated cost for the recorded token volume, or enter your own input/output budget.</p>
                <div class="mb-2 grid grid-cols-2 gap-2">
                  <label class="min-w-0 text-meta text-fg-2">Input tokens
                    <input v-model="comparisonPromptTokens" type="number" min="0" step="1" inputmode="numeric" class="ui-input mt-1 h-9 w-full min-w-0 font-mono text-fg" :placeholder="String(num(llm.prompt_tokens))" />
                  </label>
                  <label class="min-w-0 text-meta text-fg-2">Output tokens
                    <input v-model="comparisonCompletionTokens" type="number" min="0" step="1" inputmode="numeric" class="ui-input mt-1 h-9 w-full min-w-0 font-mono text-fg" :placeholder="String(num(llm.completion_tokens))" />
                  </label>
                </div>
                <div class="mb-2 flex flex-wrap items-end gap-2">
                  <label class="min-w-28 flex-1 text-meta text-fg-2">Find model
                    <input v-model="modelSearch" type="search" placeholder="Search all models" class="ui-input mt-1 h-9 w-full min-w-0 text-fg" />
                  </label>
                  <label class="text-meta text-fg-2">Sort by
                    <select v-model="modelSort" class="ui-input mt-1 h-9 w-full text-fg">
                      <option value="popular">Popular</option>
                      <option value="newest">Newest</option>
                      <option value="cheapest">Lowest estimated cost</option>
                      <option value="context">Largest context</option>
                    </select>
                  </label>
                  <button type="button" class="ui-btn ui-btn-sm h-9" :disabled="catalogLoading" @click="refreshHostedApiPrices(true)">
                    <ArrowPathIcon class="size-4" :class="{ 'motion-safe:animate-spin': catalogLoading }" aria-hidden="true" /> Refresh
                  </button>
                </div>
                <p v-if="catalogError" class="mb-2 text-meta text-warn" role="status">{{ catalogError }} <a href="https://openrouter.ai/models" target="_blank" rel="noopener noreferrer" class="underline">Browse on OpenRouter</a></p>
                <p v-if="catalogLoading && !catalogPrices.length" class="py-4 text-meta text-fg-2" role="status">Loading the live model catalog…</p>
                <div v-if="visibleHostedEstimates.length" class="overflow-x-auto">
                  <table class="w-full min-w-[460px] border-collapse text-left">
                    <thead><tr class="h-8 border-b border-rule-strong text-meta text-fg-2"><th class="font-medium">Model</th><th class="text-right font-medium">List price</th><th class="text-right font-medium">Equivalent</th></tr></thead>
                    <tbody>
                      <tr v-for="estimate in visibleHostedEstimates" :key="estimate.id" class="h-12 border-b border-rule">
                        <td class="max-w-0 pr-2">
                          <div class="truncate" :title="estimate.id">{{ estimate.model }}</div>
                          <div class="truncate text-meta text-fg-2">{{ estimate.provider }} · {{ contextLabel(estimate.contextLength) }}</div>
                        </td>
                        <td class="whitespace-nowrap text-right font-mono text-meta text-fg-2">{{ pricePerMillion(estimate.inputPerMillion) }}/M in · {{ pricePerMillion(estimate.outputPerMillion) }}/M out</td>
                        <td class="whitespace-nowrap text-right font-mono font-semibold">{{ usdEstimate(estimate.costUsd) }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <p v-else-if="!catalogLoading && !catalogError" class="py-4 text-meta text-fg-2">No matching priced text models.</p>
                <div v-if="filteredHostedEstimates.length" class="mt-2 flex flex-wrap items-center justify-between gap-2 text-meta text-fg-2">
                  <span>Showing {{ visibleHostedEstimates.length }} of {{ filteredHostedEstimates.length }} models</span>
                  <button v-if="visibleModelCount < filteredHostedEstimates.length" type="button" class="ui-btn ui-btn-sm" @click="visibleModelCount += 10">Show more</button>
                </div>
                <p class="mt-2 text-meta text-fg-2">
                  <template v-if="catalogFetchedAt">Fetched {{ new Date(catalogFetchedAt).toLocaleString() }}{{ catalogStale ? ' · stale' : '' }}. </template>
                  Prices are OpenRouter standard uncached text-token rates, not guaranteed direct-provider prices or exact bills; routing, cache discounts and extra charges can differ.
                  <template v-if="!hasTokenSpend">No farm token usage recorded yet; use the inputs above to estimate a workload. </template>
                  Local inference incurs no hosted API fees, but hardware and electricity are excluded.
                </p>
              </div>
              <div class="bg-surface">
                <h3 class="px-3 pt-3 pb-1 text-title font-semibold">Routing profiles</h3>
                <div v-if="profiles.length" class="overflow-x-auto">
                  <table class="w-full border-collapse text-left">
                    <thead>
                      <tr class="h-9 border-b border-rule-strong bg-raised text-meta text-fg-2">
                        <th class="px-3 font-medium">Profile</th>
                        <th class="px-3 text-right font-medium">Runs</th>
                        <th class="px-3 text-right font-medium">Calls</th>
                        <th class="px-3 text-right font-medium">Avg</th>
                        <th class="px-3 text-right font-medium">Rejected</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="profile in profiles" :key="String(profile.profile)" class="h-10 border-b border-rule last:border-b-0">
                        <td class="px-3">{{ profileLabel(profile.profile) }}</td>
                        <td class="px-3 text-right font-mono">{{ nfmt(profile.runs) }}</td>
                        <td class="px-3 text-right font-mono">{{ nfmt(profile.calls) }}</td>
                        <td class="px-3 text-right font-mono">{{ seconds(profile.avg_seconds) }}</td>
                        <td class="px-3 text-right font-mono">{{ nfmt(profile.rejected) }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <p v-else class="px-3 pb-6 text-fg-2">No routing-profile telemetry yet.</p>
              </div>
              <div class="bg-surface">
                <h3 class="px-3 pt-3 pb-1 text-title font-semibold">Latest serving models</h3>
                <table v-if="models.length" class="w-full table-fixed border-collapse text-left">
                  <thead>
                    <tr class="h-9 border-b border-rule-strong bg-raised text-meta text-fg-2">
                      <th class="px-3 font-medium">Model</th>
                      <th class="w-24 px-3 text-right font-medium">Runs</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="model in models" :key="String(model.name)" class="h-14 border-b border-rule last:border-b-0">
                      <td class="px-3">
                        <div class="truncate" :title="String(model.name || '')">{{ model.name }}</div>
                        <div class="truncate text-meta text-fg-2">{{ model.backend || 'Backend not reported' }}</div>
                      </td>
                      <td class="px-3 text-right font-mono">{{ nfmt(model.runs) }}</td>
                    </tr>
                  </tbody>
                </table>
                <p v-else class="px-3 pb-6 text-fg-2">No serving-model snapshots yet.</p>
              </div>
            </div>
          </section>

          <LeaderboardPanel class="xl:col-span-2" />
        </div>
      </template>
    </template>
  </div>
</template>
