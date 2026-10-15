<script setup lang="ts">
import { ref } from 'vue'
import type { DashboardRun, ProgrammingEntry, ProgrammingSnapshot } from '../shared/api/types'
import StatusPill from '../shared/components/StatusPill.vue'
import { presentRunStatus } from '../shared/status'
import { isPlayStyleRun, playStyleLabel } from '../shared/playstyle'
import { placeName, railFacts, tileLabel } from './operations'

defineProps<{
  active: DashboardRun[]
  recent: DashboardRun[]
  selectedId: string
  programming: ProgrammingSnapshot | null
  programmingError: string
}>()

const emit = defineEmits<{
  select: [runID: string]
  'toggle-programming': []
  'programming-action': [entry: ProgrammingEntry, action: 'skip' | 'retry']
}>()

const recentOpen = ref(true)
const list = ref<HTMLElement | null>(null)

function where(run: DashboardRun): string {
  return placeName(run) || tileLabel(run)
}

function rowTag(run: DashboardRun): string {
  if (run.planner === 'llm' && run.purpose === 'debug_coverage') return 'Debug coverage'
  return isPlayStyleRun(run) ? playStyleLabel(run) : ''
}

// Arrow keys move between rows; Enter/Space selects (native button).
function onKey(event: KeyboardEvent): void {
  if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return
  const rows = [...(list.value?.querySelectorAll<HTMLButtonElement>('button[data-run]') ?? [])]
  const index = rows.indexOf(document.activeElement as HTMLButtonElement)
  if (index < 0) return
  event.preventDefault()
  rows[Math.max(0, Math.min(rows.length - 1, index + (event.key === 'ArrowDown' ? 1 : -1)))]?.focus()
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col bg-surface">
    <nav ref="list" aria-label="Runs" class="min-h-0 flex-1 overflow-y-auto" @keydown="onKey">
      <h2 class="flex items-center justify-between px-3 pt-3 pb-1.5">
        <span class="ui-label">Active</span><span class="font-mono text-meta text-fg-2">{{ active.length }}</span>
      </h2>
      <p v-if="!active.length" class="px-3 pb-3 text-meta text-fg-3">No active runs.</p>
      <ul>
        <li v-for="run in active" :key="run.run_id">
          <button
            type="button"
            data-run
            :aria-current="run.run_id === selectedId ? 'true' : undefined"
            :class="run.run_id === selectedId ? 'border-l-accent bg-selected' : 'border-l-transparent hover:bg-raised'"
            class="grid min-h-[72px] w-full grid-cols-[64px_minmax(0,1fr)] items-center gap-2.5 border-b border-l-2 border-b-rule py-2 pr-3 pl-2.5 text-left [outline-offset:-2px]"
            @click="emit('select', run.run_id)"
          >
            <span class="grid h-[58px] w-16 place-items-center overflow-hidden bg-well">
              <img
                v-if="run.status !== 'queued'"
                :src="`/frame?run=${encodeURIComponent(run.run_id)}`"
                :alt="`${run.run_id} at ${where(run)}`"
                class="h-full w-full object-contain [image-rendering:pixelated]"
              />
              <span v-else class="text-meta text-fg-3">○ Queued</span>
            </span>
            <span class="flex min-w-0 flex-col gap-1">
              <span class="truncate font-mono text-meta font-medium" :title="run.run_id">{{ run.run_id }}</span>
              <span><StatusPill :status="presentRunStatus(run)" /></span>
              <span class="truncate text-meta text-fg-2">
                {{ run.status === 'queued' || run.status === 'leased' ? 'Waiting for a worker' : where(run) }}<template v-if="rowTag(run)"> · {{ rowTag(run) }}</template>
              </span>
            </span>
          </button>
        </li>
      </ul>

      <h2 class="mt-2">
        <button
          type="button"
          class="flex w-full items-center justify-between px-3 py-1.5 text-left"
          :aria-expanded="recentOpen"
          @click="recentOpen = !recentOpen"
        >
          <span class="ui-label">{{ recentOpen ? '▾' : '▸' }} Recent</span>
          <span class="font-mono text-meta text-fg-2">{{ recent.length }}</span>
        </button>
      </h2>
      <ul v-if="recentOpen">
        <li v-for="run in recent" :key="run.run_id">
          <button
            type="button"
            data-run
            :aria-current="run.run_id === selectedId ? 'true' : undefined"
            :class="run.run_id === selectedId ? 'border-l-accent bg-selected' : 'border-l-transparent hover:bg-raised'"
            class="grid min-h-[72px] w-full grid-cols-[64px_minmax(0,1fr)] items-center gap-2.5 border-b border-l-2 border-b-rule py-2 pr-3 pl-2.5 text-left [outline-offset:-2px]"
            @click="emit('select', run.run_id)"
          >
            <span class="grid h-[58px] w-16 place-items-center overflow-hidden bg-well">
              <img
                :src="`/frame?run=${encodeURIComponent(run.run_id)}`"
                :alt="`${run.run_id} final frame`"
                class="h-full w-full object-contain opacity-60 grayscale [image-rendering:pixelated]"
              />
            </span>
            <span class="flex min-w-0 flex-col gap-1">
              <span class="truncate font-mono text-meta font-medium" :title="run.run_id">{{ run.run_id }}</span>
              <span><StatusPill :status="presentRunStatus(run)" /></span>
              <span class="truncate text-meta text-fg-2" :title="railFacts(run)">{{ railFacts(run) }}</span>
            </span>
          </button>
        </li>
      </ul>
    </nav>

    <section v-if="programming" aria-label="Programming" class="shrink-0 space-y-2 border-t border-rule-strong bg-raised p-3">
      <div class="flex items-center justify-between">
        <h2 class="text-title font-semibold">Programming</h2>
        <span :class="programming.paused ? 'ui-tone-warn' : 'ui-tone-ok'" class="ui-pill">{{ programming.paused ? '‖ Paused' : '● Running' }}</span>
      </div>
      <div>
        <p class="ui-label">Live now</p>
        <p class="truncate font-medium">{{ programming.live_now?.challenge_name || 'No programmed challenge live' }}</p>
      </div>
      <div>
        <p class="ui-label">Up next</p>
        <div class="flex items-center justify-between gap-2">
          <p class="min-w-0 truncate font-medium">{{ programming.up_next?.challenge_name || 'Queue empty' }}</p>
          <template v-if="programming.up_next">
            <button
              v-if="programming.up_next.state === 'blocked' || programming.up_next.state === 'failed'"
              type="button"
              class="ui-btn ui-btn-sm ui-btn-quiet"
              @click="emit('programming-action', programming.up_next!, 'retry')"
            >Retry</button>
            <button v-else type="button" class="ui-btn ui-btn-sm ui-btn-quiet" @click="emit('programming-action', programming.up_next!, 'skip')">Skip</button>
          </template>
        </div>
      </div>
      <button type="button" class="ui-btn ui-btn-sm w-full" @click="emit('toggle-programming')">
        {{ programming.paused ? '▶ Resume queue' : '‖ Pause queue' }}
      </button>
      <p v-if="programmingError" class="text-meta text-danger" role="alert">✕ {{ programmingError }}</p>
    </section>
  </div>
</template>
