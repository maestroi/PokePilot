<script setup lang="ts">
import { computed } from 'vue'
import type { RenderState } from '../api/renderstate'
import type { ResolvedRenderTheme } from '../renderTheme'
import { modernSceneKind } from '../semanticRenderer'
import BattleRenderer from './BattleRenderer.vue'
import DialogueRenderer from './DialogueRenderer.vue'
import MenuRenderer from './MenuRenderer.vue'
import OverworldRenderer from './OverworldRenderer.vue'

const props = defineProps<{ state: RenderState; theme: ResolvedRenderTheme }>()
const kind = computed(() => modernSceneKind(props.state))
</script>

<template>
  <Transition name="modern-scene" mode="out-in">
    <div :key="kind" class="absolute inset-0">
      <BattleRenderer v-if="kind === 'battle'" :state="state" :theme="theme" />
      <DialogueRenderer v-else-if="kind === 'dialogue'" :state="state" :theme="theme" />
      <MenuRenderer v-else-if="kind === 'menu'" :state="state" :theme="theme" />
      <OverworldRenderer v-else :state="state" :theme="theme" />
    </div>
  </Transition>
</template>

<style scoped>
.modern-scene-enter-active,
.modern-scene-leave-active {
  transition: opacity 120ms ease;
}
.modern-scene-enter-from,
.modern-scene-leave-to {
  opacity: 0;
}
</style>
