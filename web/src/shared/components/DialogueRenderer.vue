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
    <div v-else class="absolute inset-0 opacity-50" :style="{ background: 'radial-gradient(circle at 50% 35%, ' + theme.ui.accent + '55, transparent 45%)' }" />
    <div class="absolute inset-0 bg-black/20" />
    <div class="absolute inset-x-0 bottom-0 p-4 sm:p-6">
      <div
        class="mx-auto max-w-4xl rounded-2xl border px-5 py-4 shadow-2xl backdrop-blur-xl sm:px-6 sm:py-5"
        :style="{ background: theme.ui.panel, borderColor: theme.ui.accent + '55', color: theme.ui.text }"
      >
        <div v-if="state.dialogue?.speaker" class="mb-2 text-[10px] font-black uppercase tracking-[.14em]" :style="{ color: theme.ui.accent }">
          {{ state.dialogue.speaker }}
        </div>
        <div class="text-base font-semibold leading-7 text-white sm:text-lg">
          {{ state.dialogue?.text || '…' }}
        </div>
        <div class="mt-3 flex items-center justify-end gap-1.5" aria-hidden="true">
          <span class="size-1.5 rounded-full opacity-40" :style="{ background: theme.ui.accent }" />
          <span class="size-1.5 rounded-full opacity-65" :style="{ background: theme.ui.accent }" />
          <span class="size-1.5 animate-pulse rounded-full" :style="{ background: theme.ui.accent }" />
        </div>
      </div>
    </div>
  </div>
</template>
