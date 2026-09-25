<script setup lang="ts">
import { computed } from 'vue'
import type { RenderState } from '../api/renderstate'
import type { ResolvedRenderTheme } from '../renderTheme'
import { hasOverworldSurface } from '../semanticRenderer'
import OverworldRenderer from './OverworldRenderer.vue'

const props = defineProps<{ state: RenderState; theme: ResolvedRenderTheme }>()
const hasWorld = computed(() => hasOverworldSurface(props.state))
const selected = computed(() => Number(props.state.menu?.cursor ?? 0))
const entries = computed(() => props.state.menu?.entries || [])
const selectionLabel = computed(() => entries.value.length ? `${Math.min(entries.value.length, selected.value + 1)}/${entries.value.length}` : '')
</script>

<template>
  <div class="absolute inset-0 overflow-hidden" :style="{ background: theme.effects.background }">
    <OverworldRenderer v-if="hasWorld" :state="state" :theme="theme" />
    <div class="absolute inset-0 bg-black/20" />
    <div class="absolute right-4 top-4 w-[min(22rem,calc(100%-2rem))] sm:right-6 sm:top-6">
      <div class="gen2-menu p-3 font-mono" :style="{ background: theme.ui.panel, color: theme.ui.text }">
        <div class="flex items-center justify-between gap-4 border-b-2 border-[#202020] pb-2 text-[10px] font-bold uppercase">
          <span>{{ state.menu?.title || 'Menu' }}</span>
          <span v-if="selectionLabel">{{ selectionLabel }}</span>
        </div>
        <div v-if="entries.length" class="mt-2 grid gap-1">
          <div
            v-for="(entry, index) in entries"
            :key="entry.id || index"
            class="grid grid-cols-[1rem_1fr] items-center gap-1 px-1 py-1 text-xs font-bold"
            :class="entry.disabled ? 'opacity-40' : ''"
          >
            <span>{{ index === selected ? '▶' : '' }}</span>
            <span>{{ entry.label || `Option ${index + 1}` }}</span>
          </div>
        </div>
        <div v-else class="px-1 py-3 text-xs font-bold">Choose an option</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.gen2-menu {
  border: 4px solid #202020;
  box-shadow: inset 0 0 0 2px #b8b898, 3px 3px 0 rgba(0,0,0,.25);
}
</style>
