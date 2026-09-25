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
  if (pct <= 20) return props.theme.battle.hpDanger || '#d84018'
  if (pct <= 50) return props.theme.battle.hpWarn || '#d8b838'
  return props.theme.battle.hpHealthy || '#58a838'
}

function actorLabel(actor: RenderBattleActor | undefined): string {
  return actor?.name || 'Unknown Pokémon'
}

function levelLabel(actor: RenderBattleActor | undefined): string {
  return actor?.level ? `Lv${actor.level}` : ''
}

function movePP(move: RenderBattleMove): string {
  if (move.max_pp) return `PP ${move.pp || 0}/${move.max_pp}`
  return `PP ${move.pp || 0}`
}
</script>

<template>
  <div
    class="absolute inset-0 overflow-hidden font-mono"
    :style="{ background: theme.battle.background, color: theme.ui.text }"
    aria-label="Gold and Silver battle view"
  >
    <div class="absolute inset-x-0 top-0 flex items-start justify-between gap-5 p-4 sm:p-6">
      <section class="gen2-panel min-w-44 p-3 sm:min-w-56" :style="{ background: theme.battle.panel || theme.ui.panel }">
        <div class="flex items-center justify-between gap-3">
          <strong class="truncate text-xs uppercase tracking-wide">{{ actorLabel(opponent) }}</strong>
          <span class="shrink-0 text-[10px] font-bold">{{ levelLabel(opponent) }}</span>
        </div>
        <div class="mt-2 grid grid-cols-[auto_1fr] items-center gap-2">
          <span class="text-[9px] font-bold">HP:</span>
          <div class="h-2 border-2 border-[#202020] bg-[#202020]">
            <div class="h-full transition-[width] duration-150" :style="{ width: hpPercent(opponent) + '%', background: hpColor(opponent) }" />
          </div>
        </div>
        <div class="mt-1 text-right text-[9px]">{{ opponent?.status || '' }}</div>
      </section>

      <PokemonSprite
        v-if="opponent"
        :name="opponent.appearance || opponent.name || ''"
        :size="136"
        :fainted="opponent.defeated"
        class="battle-pokemon"
      />
    </div>

    <div class="absolute inset-x-0 bottom-0 grid gap-3 p-4 sm:grid-cols-[minmax(0,1fr)_minmax(17rem,.85fr)] sm:p-6">
      <div class="flex items-end gap-3">
        <PokemonSprite
          v-if="player"
          :name="player.appearance || player.name || ''"
          :size="148"
          :fainted="player.defeated"
          back
          class="battle-pokemon"
        />
        <section class="gen2-panel mb-2 min-w-0 flex-1 p-3" :style="{ background: theme.battle.panel || theme.ui.panel }">
          <div class="flex items-center justify-between gap-3">
            <strong class="truncate text-xs uppercase tracking-wide">{{ actorLabel(player) }}</strong>
            <span class="shrink-0 text-[10px] font-bold">{{ levelLabel(player) }}</span>
          </div>
          <div class="mt-2 grid grid-cols-[auto_1fr] items-center gap-2">
            <span class="text-[9px] font-bold">HP:</span>
            <div class="h-2 border-2 border-[#202020] bg-[#202020]">
              <div class="h-full transition-[width] duration-150" :style="{ width: hpPercent(player) + '%', background: hpColor(player) }" />
            </div>
          </div>
          <div class="mt-1 flex justify-between text-[9px]">
            <span>{{ player?.status || '' }}</span>
            <span>{{ player?.hp || 0 }} / {{ player?.max_hp || 0 }}</span>
          </div>
        </section>
      </div>

      <section class="gen2-panel grid content-end gap-2 p-3" :style="{ background: theme.battle.panel || theme.ui.panel }">
        <div class="flex items-center justify-between text-[9px] font-bold uppercase">
          <span>{{ state.battle?.kind || 'battle' }}</span>
          <span>{{ state.battle?.phase || 'fight' }}</span>
        </div>
        <div v-if="moves.length" class="grid grid-cols-2 border-l-2 border-t-2 border-[#202020]">
          <div
            v-for="move in moves"
            :key="move.id || move.name"
            class="border-r-2 border-b-2 border-[#202020] px-2 py-2"
            :class="move.disabled ? 'opacity-40' : ''"
          >
            <div class="truncate text-[11px] font-bold uppercase">{{ move.name || move.id || 'Move' }}</div>
            <div class="mt-1 text-[9px]">{{ movePP(move) }}</div>
          </div>
        </div>
        <div v-else class="border-2 border-[#202020] px-3 py-4 text-center text-[10px]">
          Waiting for battle actions…
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.gen2-panel {
  border: 3px solid #202020;
  box-shadow: inset 0 0 0 2px #f8f8d8, 4px 4px 0 rgba(0, 0, 0, .28);
}

.battle-pokemon {
  border: 0;
  background: transparent;
  filter: drop-shadow(4px 5px 0 rgba(0,0,0,.18));
}
</style>
