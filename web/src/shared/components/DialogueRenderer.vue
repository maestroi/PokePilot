<script setup lang="ts">
import { computed } from 'vue'
import type { RenderState } from '../api/renderstate'
import type { ResolvedRenderTheme } from '../renderTheme'
import { hasOverworldSurface } from '../semanticRenderer'
import OverworldRenderer from './OverworldRenderer.vue'

const props = defineProps<{ state: RenderState; theme: ResolvedRenderTheme }>()
const hasWorld = computed(() => hasOverworldSurface(props.state))
</script>

<template>
  <div class="absolute inset-0 overflow-hidden" :style="{ background: theme.effects.background }">
    <OverworldRenderer v-if="hasWorld" :state="state" :theme="theme" />
    <div v-else class="absolute inset-0 bg-[#508040]" />
    <div class="absolute inset-x-0 bottom-0 p-3 sm:p-5">
      <div class="gen2-textbox mx-auto max-w-4xl px-5 py-4 font-mono sm:px-6" :style="{ background: theme.ui.panel, color: theme.ui.text }">
        <div v-if="state.dialogue?.speaker" class="mb-2 text-[10px] font-bold uppercase tracking-[.08em]" :style="{ color: theme.ui.accent }">
          {{ state.dialogue.speaker }}
        </div>
        <div class="min-h-12 text-sm font-bold leading-6 sm:text-base">
          {{ state.dialogue?.text || '…' }}
        </div>
        <div class="mt-1 text-right text-xs" aria-hidden="true">▼</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.gen2-textbox {
  border: 4px solid #202020;
  box-shadow: inset 0 0 0 2px #b8b898, 3px 3px 0 rgba(0,0,0,.25);
}
</style>
