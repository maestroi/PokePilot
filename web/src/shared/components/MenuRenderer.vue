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
const selectionLabel = computed(() => entries.value.length ? `${Math.min(entries.value.length, selected.value + 1)} / ${entries.value.length}` : '')
</script>

<template>
  <div class="absolute inset-0 overflow-hidden" :style="{ background: theme.effects.background }">
    <OverworldRenderer v-if="hasWorld" :state="state" :theme="theme" />
    <div class="absolute inset-0 bg-black/45 backdrop-blur-[1px]" />
    <div class="absolute inset-0 grid place-items-center p-6">
      <div
        class="w-full max-w-lg rounded-2xl border p-5 shadow-2xl backdrop-blur-xl"
        :style="{ background: theme.ui.panel, borderColor: theme.ui.accent + '55', color: theme.ui.text }"
      >
        <div class="flex items-center justify-between gap-4">
          <div class="text-[10px] font-black uppercase tracking-[.14em]" :style="{ color: theme.ui.accent }">Menu</div>
          <div v-if="selectionLabel" class="font-mono text-[10px] font-bold text-white/45">{{ selectionLabel }}</div>
        </div>
        <div class="mt-3 text-sm font-semibold leading-6 text-white/90">
          {{ state.menu?.title || 'Choose an option' }}
        </div>
        <div v-if="entries.length" class="mt-4 grid gap-1.5">
          <div
            v-for="(entry, index) in entries"
            :key="entry.id || index"
            class="flex items-center gap-2 rounded-lg px-3 py-2 text-xs font-semibold"
            :class="index === selected ? 'bg-white/10 text-white' : 'text-white/35'"
          >
            <span class="w-3 text-center" :style="{ color: index === selected ? theme.ui.accent : 'transparent' }">▶</span>
            <span>{{ entry.label || `Option ${index + 1}` }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
