<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowPathIcon, ArrowTopRightOnSquareIcon, MagnifyingGlassIcon } from '@heroicons/vue/20/solid'
import { getTriage, investigateTriage } from '../shared/api/client'
import type { TriageGroup } from '../shared/api/types'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'

const investigating = ref(new Set<string>())
const actionError = ref('')

const resource = usePollingResource(
  (signal) => getTriage(signal),
  { intervalMs: 5000, isEmpty: (groups) => groups.length === 0 }
)

function isResolved(group: TriageGroup): boolean {
  const status = String(group.issue?.status || '').toLowerCase()
  const resolution = String(group.issue?.resolution || '').toLowerCase()
  return status === 'resolved' || status === 'fixed' || resolution === 'fixed'
}

const openGroups = computed(() => resource.data.value?.filter((group) => !isResolved(group)) ?? [])
const resolvedGroups = computed(() => resource.data.value?.filter(isResolved) ?? [])

function issueURL(group: TriageGroup): string {
  const raw = group.issue?.issue_url || ''
  if (!raw) return ''
  try {
    const url = new URL(raw)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.toString() : ''
  } catch {
    return ''
  }
}

function groupTitle(group: TriageGroup): string {
  return group.pattern || group.detail || group.fingerprint || group.key
}

function exampleRuns(group: TriageGroup): string[] {
  const candidates = group.runs || group.examples || []
  return candidates.slice(0, 5).map(String)
}

function inspectURL(runID: string): string {
  const url = new URL('/', window.location.origin)
  url.searchParams.set('run', runID)
  url.hash = 'live'
  return url.toString()
}

async function investigate(group: TriageGroup): Promise<void> {
  if (investigating.value.has(group.key)) return
  actionError.value = ''
  investigating.value = new Set([...investigating.value, group.key])
  try {
    await investigateTriage(group.key)
    await resource.retry()
  } catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : 'Investigation request failed'
  } finally {
    const next = new Set(investigating.value)
    next.delete(group.key)
    investigating.value = next
  }
}

function retry(): void {
  void resource.retry()
}
</script>

<template>
  <div class="space-y-3">
    <Panel title="Failure triage" description="Actionable failures grouped by their normalized fingerprint, not one row per failed attempt." compact>
      <template #actions>
        <button type="button" class="inline-flex items-center gap-1.5 rounded-md bg-white/8 px-2.5 py-1.5 text-xs font-semibold text-slate-200 ring-1 ring-white/10 hover:bg-white/12" @click="retry">
          <ArrowPathIcon class="size-3.5" aria-hidden="true" />
          Refresh
        </button>
      </template>

      <ResourceState
        :state="resource.state.value"
        :title="resource.state.value === 'empty' ? 'No failure groups' : 'Failure triage unavailable'"
        :message="resource.error.value || 'New actionable failure groups will appear here.'"
        :rows="6"
      >
        <template #actions>
          <button type="button" class="rounded-md bg-white/10 px-2.5 py-1.5 text-xs font-semibold text-white ring-1 ring-white/10 hover:bg-white/15" @click="retry">Retry now</button>
        </template>

        <div v-if="actionError" class="mb-3 border-l-4 border-rose-400 bg-rose-400/10 p-3 text-sm text-rose-100" role="alert">{{ actionError }}</div>

        <div v-if="openGroups.length" class="divide-y divide-white/8">
          <article v-for="group in openGroups" :key="group.key" class="py-4 first:pt-0 last:pb-0">
            <div class="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2">
                  <StatusBadge tone="danger">{{ Number(group.count || 0) }} occurrence{{ Number(group.count || 0) === 1 ? '' : 's' }}</StatusBadge>
                  <StatusBadge v-if="group.issue?.issue_number" :tone="group.issue?.stale ? 'warning' : 'info'">Issue #{{ group.issue.issue_number }}</StatusBadge>
                  <span class="font-mono text-[10px] text-slate-600">{{ group.key }}</span>
                </div>
                <h3 class="mt-2 break-words text-sm font-semibold leading-6 text-slate-100">{{ groupTitle(group) }}</h3>

                <div v-if="exampleRuns(group).length" class="mt-3 flex flex-wrap gap-1.5">
                  <a
                    v-for="runID in exampleRuns(group)"
                    :key="runID"
                    :href="inspectURL(runID)"
                    class="rounded-md bg-black/20 px-2 py-1 font-mono text-[11px] text-cyan-200 ring-1 ring-white/8 hover:bg-white/8"
                    :title="runID"
                  >
                    {{ runID.length > 18 ? `${runID.slice(0, 18)}…` : runID }}
                  </a>
                </div>
              </div>

              <div class="flex shrink-0 flex-wrap gap-2">
                <a
                  v-if="issueURL(group)"
                  :href="issueURL(group)"
                  target="_blank"
                  rel="noopener"
                  class="inline-flex items-center gap-1.5 rounded-md bg-white/6 px-2.5 py-1.5 text-xs font-semibold text-slate-300 ring-1 ring-white/8 hover:bg-white/10 hover:text-white"
                >
                  Open issue
                  <ArrowTopRightOnSquareIcon class="size-3.5" aria-hidden="true" />
                </a>
                <button
                  type="button"
                  :disabled="investigating.has(group.key)"
                  class="inline-flex items-center gap-1.5 rounded-md bg-cyan-500 px-2.5 py-1.5 text-xs font-semibold text-white hover:bg-cyan-400 disabled:cursor-wait disabled:opacity-60"
                  @click="investigate(group)"
                >
                  <MagnifyingGlassIcon class="size-3.5" aria-hidden="true" />
                  {{ investigating.has(group.key) ? 'Starting…' : 'Investigate' }}
                </button>
              </div>
            </div>
          </article>
        </div>

        <p v-else class="py-8 text-center text-sm text-slate-500">No open failure groups.</p>
      </ResourceState>
    </Panel>

    <Panel v-if="resolvedGroups.length" title="Resolved history" description="Groups whose linked issue is already marked fixed/resolved." compact>
      <div class="divide-y divide-white/8">
        <div v-for="group in resolvedGroups.slice(0, 20)" :key="group.key" class="flex items-center justify-between gap-4 py-2.5 first:pt-0 last:pb-0">
          <div class="min-w-0">
            <p class="truncate text-xs text-slate-400" :title="groupTitle(group)">{{ groupTitle(group) }}</p>
            <p class="mt-0.5 font-mono text-[10px] text-slate-600">{{ group.key }}</p>
          </div>
          <StatusBadge tone="success">resolved</StatusBadge>
        </div>
      </div>
    </Panel>
  </div>
</template>
