<script setup lang="ts">
import { computed } from 'vue'
import PokemonSprite from './PokemonSprite.vue'

const props = defineProps<{
  name: string
  level: number
  hp: number
  maxHp: number
  status?: string
  lead?: boolean
}>()

const hpPercent = computed(() => {
  if (props.maxHp <= 0) return 0
  return Math.max(0, Math.min(100, 100 * props.hp / props.maxHp))
})
const hpTone = computed(() => hpPercent.value <= 20 ? 'hp-low' : hpPercent.value <= 50 ? 'hp-mid' : 'hp-high')
const fainted = computed(() => props.hp <= 0)
const normalizedStatus = computed(() => String(props.status || '').trim().toLowerCase())
const statusLabel = computed(() => {
  if (fainted.value) return 'FNT'
  switch (normalizedStatus.value) {
    case 'poison':
    case 'poisoned':
    case 'psn': return 'PSN'
    case 'paralyze':
    case 'paralyzed':
    case 'par': return 'PAR'
    case 'sleep':
    case 'asleep':
    case 'slp': return 'SLP'
    case 'burn':
    case 'burned':
    case 'brn': return 'BRN'
    case 'freeze':
    case 'frozen':
    case 'frz': return 'FRZ'
    default: return normalizedStatus.value && normalizedStatus.value !== 'healthy' ? normalizedStatus.value.toUpperCase().slice(0, 3) : ''
  }
})
</script>

<template>
  <article class="pokemon-party-card relative overflow-hidden rounded-lg border bg-black/15 p-2.5">
    <div class="absolute inset-y-0 left-0 w-1 bg-[var(--mode-accent)] opacity-60" />
    <div class="flex min-w-0 gap-3 pl-1">
      <PokemonSprite :name="name" :size="58" :fainted="fainted" />
      <div class="min-w-0 flex-1 py-0.5">
        <div class="flex min-w-0 items-start justify-between gap-2">
          <div class="min-w-0">
            <div class="flex min-w-0 items-center gap-1.5">
              <strong class="truncate text-sm text-white">{{ name || 'Unknown' }}</strong>
              <span v-if="lead" class="rounded bg-[var(--mode-soft)] px-1.5 py-0.5 text-[7px] font-black tracking-[0.08em] text-[var(--mode-accent)] ring-1 ring-[var(--mode-border)]">LEAD</span>
            </div>
            <span class="mt-0.5 block font-mono text-[10px] text-slate-500">Lv {{ level }}</span>
          </div>
          <span v-if="statusLabel" class="status-chip shrink-0">{{ statusLabel }}</span>
        </div>

        <div class="mt-2 flex items-center gap-2">
          <span class="font-mono text-[8px] font-bold tracking-[0.08em] text-slate-600">HP</span>
          <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-white/8 ring-1 ring-white/5">
            <div :class="['h-full rounded-full transition-[width]', hpTone]" :style="{ width: `${hpPercent}%` }" />
          </div>
        </div>
        <div class="mt-1 flex items-center justify-between gap-2 font-mono text-[10px]">
          <span class="text-slate-400">{{ hp }}/{{ maxHp }}</span>
          <span :class="fainted ? 'text-red-300' : 'text-slate-600'">{{ fainted ? 'fainted' : statusLabel ? 'status' : 'healthy' }}</span>
        </div>
      </div>
    </div>
  </article>
</template>

<style scoped>
.pokemon-party-card {
  border-color: var(--mode-border);
  background:
    radial-gradient(circle at 0% 50%, var(--mode-soft), transparent 9rem),
    rgba(0, 0, 0, 0.15);
}

.hp-high { background: rgb(52 211 153); }
.hp-mid { background: rgb(250 204 21); }
.hp-low { background: rgb(248 113 113); }

.status-chip {
  border: 1px solid rgba(251, 191, 36, 0.22);
  border-radius: 0.25rem;
  background: rgba(120, 53, 15, 0.2);
  padding: 0.15rem 0.3rem;
  color: rgb(253 230 138);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 0.5rem;
  font-weight: 800;
  letter-spacing: 0.06em;
}
</style>
