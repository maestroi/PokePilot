<script setup lang="ts">
import { computed } from 'vue'
import type { RenderBattleActor, RenderBattleMove, RenderState } from '../api/renderstate'
import type { ResolvedRenderTheme } from '../renderTheme'
import PokemonSprite from './PokemonSprite.vue'

const props = defineProps<{ state: RenderState; theme: ResolvedRenderTheme }>()

const actors = computed(() => props.state.battle?.actors || [])
const player = computed(() => actors.value.find((actor) => actor.role === 'player') || actors.value[0])
const opponent = computed(() => actors.value.find((actor) => actor.role === 'opponent') || actors.value[1])
const moves = computed(() => props.state.battle?.moves || [])

function hpPercent(actor: RenderBattleActor | undefined): number {
  if (!actor || !actor.max_hp || actor.max_hp <= 0) return 0
  return Math.max(0, Math.min(100, Math.round((Number(actor.hp || 0) / actor.max_hp) * 100)))
}

function hpColor(actor: RenderBattleActor | undefined): string {
  const pct = hpPercent(actor)
  if (pct <= 20) return props.theme.battle.hpDanger || '#fb7185'
  if (pct <= 50) return props.theme.battle.hpWarn || '#fbbf24'
  return props.theme.battle.hpHealthy || '#34d399'
}

function actorLabel(actor: RenderBattleActor | undefined): string {
  return actor?.name || 'Unknown Pokémon'
}

function levelLabel(actor: RenderBattleActor | undefined): string {
  return actor?.level ? `Lv ${actor.level}` : ''
}

function movePP(move: RenderBattleMove): string {
  if (move.max_pp) return `${move.pp || 0}/${move.max_pp} PP`
  return `${move.pp || 0} PP`
}
</script>

<template>
  <div
    class="absolute inset-0 overflow-hidden"
    :style="{
      background: theme.battle.background || 'linear-gradient(180deg,#d9f0dd 0%,#9bc3ab 48%,#355568 100%)',
      color: theme.ui.text
    }"
    aria-label="Modern battle view"
  >
    <div class="absolute inset-0 opacity-35" :style="{ background: 'radial-gradient(circle at 50% 35%, ' + theme.ui.accent + '55, transparent 42%)' }" />

    <div class="absolute inset-x-0 top-0 flex items-start justify-between p-5 sm:p-7">
      <div
        class="min-w-44 rounded-2xl border px-4 py-3 shadow-2xl backdrop-blur-md sm:min-w-56"
        :style="{ background: theme.battle.panel || theme.ui.panel, borderColor: theme.battle.opponentAccent || theme.ui.accent }"
      >
        <div class="flex items-center justify-between gap-3">
          <div class="truncate text-sm font-black tracking-wide text-white">{{ actorLabel(opponent) }}</div>
          <div class="shrink-0 font-mono text-[10px] font-bold text-white/60">{{ levelLabel(opponent) }}</div>
        </div>
        <div class="mt-2 h-2 overflow-hidden rounded-full bg-black/35">
          <div class="h-full rounded-full transition-[width] duration-200" :style="{ width: hpPercent(opponent) + '%', background: hpColor(opponent) }" />
        </div>
        <div class="mt-1.5 flex items-center justify-between text-[10px] font-semibold text-white/55">
          <span>{{ opponent?.status || 'healthy' }}</span>
          <span>{{ opponent?.hp || 0 }}/{{ opponent?.max_hp || 0 }}</span>
        </div>
      </div>

      <PokemonSprite
        v-if="opponent"
        :name="opponent.appearance || opponent.name || ''"
        :size="128"
        :fainted="opponent.defeated"
        class="battle-pokemon battle-opponent"
      />
    </div>

    <div class="absolute inset-x-0 bottom-0 grid gap-4 p-5 sm:grid-cols-[minmax(0,1fr)_minmax(16rem,.8fr)] sm:p-7">
      <div class="flex items-end gap-4">
        <PokemonSprite
          v-if="player"
          :name="player.appearance || player.name || ''"
          :size="144"
          :fainted="player.defeated"
          class="battle-pokemon battle-player"
        />
        <div
          class="mb-2 min-w-0 flex-1 rounded-2xl border px-4 py-3 shadow-2xl backdrop-blur-md"
          :style="{ background: theme.battle.panel || theme.ui.panel, borderColor: theme.battle.playerAccent || theme.ui.accent }"
        >
          <div class="flex items-center justify-between gap-3">
            <div class="truncate text-sm font-black tracking-wide text-white">{{ actorLabel(player) }}</div>
            <div class="shrink-0 font-mono text-[10px] font-bold text-white/60">{{ levelLabel(player) }}</div>
          </div>
          <div class="mt-2 h-2 overflow-hidden rounded-full bg-black/35">
            <div class="h-full rounded-full transition-[width] duration-200" :style="{ width: hpPercent(player) + '%', background: hpColor(player) }" />
          </div>
          <div class="mt-1.5 flex items-center justify-between text-[10px] font-semibold text-white/55">
            <span>{{ player?.status || 'healthy' }}</span>
            <span>{{ player?.hp || 0 }}/{{ player?.max_hp || 0 }}</span>
          </div>
        </div>
      </div>

      <div
        class="grid content-end gap-2 rounded-2xl border p-3 shadow-2xl backdrop-blur-md"
        :style="{ background: theme.battle.panel || theme.ui.panel, borderColor: 'rgba(255,255,255,.12)' }"
      >
        <div class="mb-1 flex items-center justify-between px-1 text-[9px] font-black uppercase tracking-[.12em] text-white/45">
          <span>{{ state.battle?.kind || 'battle' }}</span>
          <span>{{ state.battle?.phase || 'action' }}</span>
        </div>
        <div v-if="moves.length" class="grid grid-cols-2 gap-2">
          <div
            v-for="move in moves"
            :key="move.id || move.name"
            class="rounded-xl border px-3 py-2.5"
            :class="move.disabled ? 'opacity-40' : ''"
            :style="{ background: 'rgba(255,255,255,.045)', borderColor: 'rgba(255,255,255,.09)' }"
          >
            <div class="truncate text-xs font-black text-white">{{ move.name || move.id || 'Move' }}</div>
            <div class="mt-1 text-[9px] font-semibold text-white/45">{{ movePP(move) }}<span v-if="move.disabled"> · disabled</span></div>
          </div>
        </div>
        <div v-else class="rounded-xl bg-black/15 px-3 py-4 text-center text-xs font-semibold text-white/45">
          Waiting for battle actions…
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.battle-pokemon {
  border: 0;
  background: transparent;
  filter: drop-shadow(0 18px 20px rgba(0,0,0,.24));
}
.battle-opponent {
  transform: scaleX(-1);
}
</style>
