<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { CheckCircleIcon, ClockIcon, UserGroupIcon } from '@heroicons/vue/20/solid'

interface VoteCandidate {
  challenge_id: string
  challenge_version: number
  challenge_name: string
  votes: number
}

interface AudienceVote {
  id: string
  status: string
  opened_at: number
  closes_at?: number
  tie_policy: string
  candidates: VoteCandidate[]
  winner_id?: string
  queue_entry_id?: string
  source_totals?: Record<string, number>
}

const vote = ref<AudienceVote | null>(null)
const message = ref('')
const pending = ref('')
const votedFor = ref('')
let timer = 0

const totalVotes = computed(() =>
  (vote.value?.candidates || []).reduce((sum, candidate) => sum + Number(candidate.votes || 0), 0)
)

const closesLabel = computed(() => {
  const closesAt = Number(vote.value?.closes_at || 0)
  if (!closesAt) return 'Operator closes the vote'
  const seconds = Math.max(0, closesAt - Math.floor(Date.now() / 1000))
  if (seconds < 60) return seconds + 's left'
  const minutes = Math.ceil(seconds / 60)
  return minutes + 'm left'
})

function candidatePercent(candidate: VoteCandidate): number {
  if (totalVotes.value <= 0) return 0
  return Math.round(100 * Number(candidate.votes || 0) / totalVotes.value)
}

async function refresh(): Promise<void> {
  try {
    const response = await fetch('/v1/watch/vote', {
      method: 'GET',
      headers: { Accept: 'application/json' },
      cache: 'no-store'
    })
    if (!response.ok) throw new Error('Vote feed unavailable')
    const payload = await response.json() as { active?: AudienceVote | null }
    vote.value = payload.active || null
  } catch {
  }
}

async function cast(candidate: VoteCandidate): Promise<void> {
  const active = vote.value
  if (!active || pending.value || votedFor.value) return
  pending.value = candidate.challenge_id
  message.value = ''
  try {
    const response = await fetch('/v1/watch/vote/' + encodeURIComponent(active.id) + '/ballots', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ candidate_id: candidate.challenge_id })
    })
    const payload = await response.json().catch(() => ({})) as {
      accepted?: boolean
      vote?: AudienceVote
      error?: string
    }
    if (!response.ok) {
      if (response.status === 409) {
        message.value = 'This browser already voted in this round.'
        votedFor.value = 'recorded'
        await refresh()
        return
      }
      throw new Error(payload.error || 'Vote was not accepted')
    }
    if (payload.vote) vote.value = payload.vote
    votedFor.value = candidate.challenge_id
    message.value = payload.accepted === false ? 'Your vote was already recorded.' : 'Vote recorded.'
  } catch (error) {
    message.value = error instanceof Error ? error.message : 'Vote failed'
  } finally {
    pending.value = ''
  }
}

onMounted(() => {
  void refresh()
  timer = window.setInterval(() => { void refresh() }, 3000)
})

onUnmounted(() => {
  if (timer) window.clearInterval(timer)
})
</script>

<template>
  <section
    v-if="vote"
    class="audience-vote overflow-hidden rounded-2xl border border-violet-300/18 bg-violet-300/[0.035] shadow-xl shadow-violet-950/10"
    aria-live="polite"
  >
    <div class="flex flex-wrap items-start justify-between gap-3 border-b border-white/8 px-4 py-4 sm:px-5">
      <div>
        <div class="flex items-center gap-2 text-[10px] font-black tracking-[0.12em] text-violet-200 uppercase">
          <UserGroupIcon class="size-4" aria-hidden="true" />
          Audience chooses what runs next
        </div>
        <h2 class="mt-1.5 text-base font-black text-white">Vote for the next challenge</h2>
        <p class="mt-1 text-xs text-slate-500">Only validated challenge-catalog entries can appear here.</p>
      </div>
      <div class="flex items-center gap-3 text-[10px] text-slate-500">
        <span class="inline-flex items-center gap-1.5">
          <ClockIcon class="size-3.5 text-violet-300" aria-hidden="true" />
          {{ closesLabel }}
        </span>
        <span class="font-mono">{{ totalVotes }} vote{{ totalVotes === 1 ? '' : 's' }}</span>
      </div>
    </div>

    <div class="grid gap-2 p-3 sm:grid-cols-2 sm:p-4 lg:grid-cols-4">
      <button
        v-for="candidate in vote.candidates"
        :key="candidate.challenge_id + ':' + candidate.challenge_version"
        type="button"
        :disabled="Boolean(pending || votedFor)"
        class="vote-candidate group relative overflow-hidden rounded-xl border border-white/9 bg-black/15 p-3 text-left transition enabled:hover:-translate-y-0.5 enabled:hover:border-violet-300/30 enabled:hover:bg-violet-300/[0.055] disabled:cursor-default disabled:opacity-80"
        @click="cast(candidate)"
      >
        <div class="relative z-10">
          <div class="flex items-start justify-between gap-2">
            <strong class="line-clamp-2 text-xs font-extrabold leading-5 text-white">{{ candidate.challenge_name }}</strong>
            <span class="shrink-0 font-mono text-[10px] text-violet-200">{{ candidatePercent(candidate) }}%</span>
          </div>
          <div class="mt-1 text-[9px] text-slate-600">{{ candidate.challenge_id }} · v{{ candidate.challenge_version }}</div>
          <div class="mt-3 flex items-center justify-between gap-2">
            <span class="font-mono text-[10px] text-slate-400">{{ candidate.votes }} vote{{ candidate.votes === 1 ? '' : 's' }}</span>
            <span
              v-if="votedFor === candidate.challenge_id"
              class="inline-flex items-center gap-1 text-[9px] font-bold text-emerald-200"
            >
              <CheckCircleIcon class="size-3.5" aria-hidden="true" />
              Your vote
            </span>
            <span v-else-if="pending === candidate.challenge_id" class="text-[9px] font-bold text-violet-200">Sending…</span>
            <span v-else-if="!votedFor" class="text-[9px] font-bold text-violet-200 opacity-70 transition group-hover:opacity-100">Vote →</span>
          </div>
        </div>
        <div
          class="absolute inset-x-0 bottom-0 h-0.5 bg-violet-300/60 transition-[width]"
          :style="{ width: candidatePercent(candidate) + '%' }"
        />
      </button>
    </div>

    <div v-if="message" class="border-t border-white/8 px-4 py-2.5 text-[10px] text-slate-400 sm:px-5">
      {{ message }}
    </div>
  </section>
</template>
