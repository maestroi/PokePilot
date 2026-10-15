<script setup lang="ts">
import { computed } from 'vue'
import { ClipboardDocumentIcon, XMarkIcon } from '@heroicons/vue/20/solid'
import type { TriageGroup } from '../shared/api/types'
import StatusPill from '../shared/components/StatusPill.vue'
import { presentFailureGroup } from '../shared/status'
import { operatorHref } from '../shared/urls'
import { canDismiss, exampleRuns, groupTitle, investigateBlocker, issueURL, solverSummary } from './failuresQuery'
import { isResolvedGroup } from './runCleanup'
import { formatInteger, timeAgo } from './runs'

const props = defineProps<{
  groupKey: string
  group: TriageGroup | null
  loading: boolean
  investigating: boolean
  investigateError: string
  cleanupBusy: boolean
  dismissBusy: boolean
}>()
const emit = defineEmits<{
  close: []
  copy: [value: string, noun: string]
  live: [runID: string]
  investigate: [group: TriageGroup]
  dismiss: [group: TriageGroup]
  cleanup: [group: TriageGroup]
}>()

const status = computed(() => props.group ? presentFailureGroup(props.group, isResolvedGroup(props.group)) : null)
const runs = computed(() => props.group ? exampleRuns(props.group) : [])
const blocker = computed(() => props.group ? investigateBlocker(props.group) : '')
const issue = computed(() => props.group?.issue)

function liveHref(runID: string): string {
  return operatorHref('live', { run: runID, failure: props.groupKey, job: '', output: '' }, window.location.search)
}
function when(seconds?: number): string {
  return seconds ? new Date(seconds * 1000).toISOString() : ''
}
</script>

<template>
  <section class="flex h-full min-h-0 flex-col bg-surface" aria-labelledby="failure-pane-title">
    <header class="flex min-h-12 shrink-0 items-center gap-2 border-b border-rule bg-raised px-3 py-2">
      <h2 id="failure-pane-title" class="min-w-0 truncate font-mono text-title font-medium" :title="groupKey">
        <span class="sr-only">Failure group </span>{{ groupKey }}
      </h2>
      <button type="button" class="ui-btn ui-btn-sm ui-icon-btn shrink-0" aria-label="Copy group key" title="Copy group key" @click="emit('copy', groupKey, 'Group key')">
        <ClipboardDocumentIcon class="size-4" aria-hidden="true" />
      </button>
      <StatusPill v-if="status" :status="status" class="ml-auto" />
      <button type="button" class="ui-btn ui-btn-sm ui-icon-btn shrink-0" :class="status ? '' : 'ml-auto'" aria-label="Close failure details" title="Close (Esc)" @click="emit('close')">
        <XMarkIcon class="size-4" aria-hidden="true" />
      </button>
    </header>

    <div v-if="!group && loading" class="flex-1 space-y-3 p-4" aria-busy="true" aria-label="Loading failure group">
      <div v-for="n in 6" :key="n" class="h-4 bg-raised motion-safe:animate-pulse" :class="n % 3 === 0 ? 'w-2/3' : 'w-full'" />
    </div>

    <div v-else-if="!group" class="flex flex-1 flex-col items-start gap-3 p-4" role="status">
      <p class="text-title">Failure group not found. It may have been resolved or removed.</p>
      <button type="button" class="ui-btn" @click="emit('close')">Close</button>
    </div>

    <template v-else>
      <div class="min-h-0 flex-1 overflow-y-auto px-3 pb-4">
        <h3 class="mt-3 mb-1 text-title font-semibold">Failure</h3>
        <p class="max-w-[72ch] break-words whitespace-pre-wrap">{{ groupTitle(group) }}</p>
        <dl class="ui-kv mt-2">
          <dt>Occurrences</dt><dd class="font-mono">{{ formatInteger(Number(group.count || 0)) }}</dd>
          <template v-if="group.fingerprint">
            <dt>Fingerprint</dt>
            <dd class="flex min-w-0 items-start gap-1">
              <span class="min-w-0 font-mono text-meta break-all">{{ group.fingerprint }}</span>
              <button type="button" class="ui-btn ui-btn-sm ui-icon-btn size-6 shrink-0" aria-label="Copy fingerprint" @click="emit('copy', String(group.fingerprint), 'Fingerprint')">
                <ClipboardDocumentIcon class="size-4" aria-hidden="true" />
              </button>
            </dd>
          </template>
        </dl>
        <details v-if="group.example || group.detail" class="mt-2 max-w-[72ch] text-fg-2">
          <summary class="cursor-pointer text-meta">Details</summary>
          <p class="mt-1 font-mono text-meta break-words whitespace-pre-wrap">{{ group.example || group.detail }}</p>
        </details>

        <h3 class="mt-5 mb-1 text-title font-semibold">Issue</h3>
        <dl v-if="issue" class="ui-kv">
          <dt>Issue</dt>
          <dd>
            <a v-if="issueURL(group)" :href="issueURL(group)" target="_blank" rel="noopener" class="text-accent hover:underline">#{{ issue.issue_number }} ↗</a>
            <span v-else>#{{ issue.issue_number }}</span>
          </dd>
          <dt>Status</dt><dd>{{ [issue.status, issue.resolution].filter(Boolean).join(' · ') || '—' }}</dd>
          <template v-if="issue.verification_state">
            <dt>Verification</dt>
            <dd>{{ issue.verification_state }}<span v-if="issue.verification_required_runs" class="text-fg-2"> · needs {{ issue.verification_required_runs }} clean runs</span></dd>
          </template>
          <template v-if="issue.circuit_open">
            <dt>Quarantine</dt><dd>Circuit open · {{ issue.circuit_count || 0 }} hits</dd>
          </template>
          <template v-if="issue.updated_at">
            <dt>Updated</dt><dd :title="when(issue.updated_at)">{{ timeAgo(issue.updated_at) }}</dd>
          </template>
          <template v-if="solverSummary(group)">
            <dt>Solver</dt><dd>{{ solverSummary(group) }}</dd>
          </template>
        </dl>
        <p v-else class="text-fg-2">No linked issue yet.</p>
        <details v-if="issue?.solver_attempts?.length" class="mt-2">
          <summary class="cursor-pointer text-meta text-fg-2">All solver attempts ({{ issue.solver_attempts.length }})</summary>
          <ol class="mt-1 space-y-2">
            <li v-for="attempt in [...issue.solver_attempts].reverse()" :key="attempt.id" class="border-l-2 border-rule pl-2">
              <div><span class="font-medium">{{ attempt.model || attempt.backend }}</span> <span class="text-fg-2">· {{ attempt.state.replace(/_/g, ' ') }}</span></div>
              <p v-if="attempt.note" class="max-w-[72ch] text-meta break-words text-fg-2">{{ attempt.note }}</p>
              <a v-if="attempt.pr_url" :href="attempt.pr_url" target="_blank" rel="noopener" class="text-meta text-accent hover:underline">PR #{{ attempt.pr_number }} ↗</a>
            </li>
          </ol>
        </details>

        <h3 class="mt-5 mb-1 text-title font-semibold">Example runs <span class="font-normal text-fg-2">{{ runs.length }}</span></h3>
        <ul v-if="runs.length" class="divide-y divide-rule border-y border-rule">
          <li v-for="runID in runs" :key="runID" class="flex items-center gap-1 py-1.5">
            <span class="min-w-0 flex-1 font-mono break-all">{{ runID }}</span>
            <button type="button" class="ui-btn ui-btn-sm ui-icon-btn shrink-0 max-md:size-10" :aria-label="`Copy run ID ${runID}`" @click="emit('copy', runID, 'Run ID')">
              <ClipboardDocumentIcon class="size-4" aria-hidden="true" />
            </button>
            <a :href="liveHref(runID)" class="ui-btn ui-btn-sm ui-btn-quiet shrink-0 text-accent max-md:h-10" @click.prevent="emit('live', runID)">Open in Live</a>
          </li>
        </ul>
        <p v-else class="text-fg-2">No example runs recorded.</p>
      </div>

      <footer class="shrink-0 space-y-2 border-t border-rule bg-raised px-3 py-2">
        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="ui-btn ui-btn-primary max-md:h-10"
            :disabled="Boolean(blocker) || investigating"
            :aria-busy="investigating"
            :aria-describedby="blocker ? 'investigate-reason' : undefined"
            @click="emit('investigate', group)"
          >{{ investigating ? 'Starting…' : 'Investigate with AI' }}</button>
          <a v-if="issueURL(group)" :href="issueURL(group)" target="_blank" rel="noopener" class="ui-btn max-md:h-10">Open issue ↗</a>
          <span v-if="blocker" id="investigate-reason" class="text-meta text-fg-2">{{ blocker }}</span>
        </div>
        <p v-if="investigateError" class="text-meta text-danger" role="alert">✕ {{ investigateError }}</p>
        <div class="flex flex-wrap gap-2">
          <button v-if="canDismiss(group)" type="button" class="ui-btn ui-btn-sm ui-btn-quiet max-md:h-10" :disabled="dismissBusy" @click="emit('dismiss', group)">Dismiss…</button>
          <button type="button" class="ui-btn ui-btn-sm ui-btn-danger max-md:h-10" :disabled="cleanupBusy" @click="emit('cleanup', group)">Delete matching runs…</button>
        </div>
      </footer>
    </template>
  </section>
</template>
