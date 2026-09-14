<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { pokemonDexNumber, pokemonSpriteUrl } from '../pokemonAssets'

const props = withDefaults(defineProps<{
  name: string
  size?: number
  fainted?: boolean
}>(), {
  size: 56,
  fainted: false
})

const failed = ref(false)
const src = computed(() => pokemonSpriteUrl(props.name))
const dex = computed(() => pokemonDexNumber(props.name))
const boxStyle = computed(() => ({ width: `${props.size}px`, height: `${props.size}px` }))

watch(src, () => { failed.value = false })
</script>

<template>
  <span
    class="pokemon-sprite relative inline-grid shrink-0 place-items-center overflow-hidden rounded-lg border border-white/8 bg-black/20"
    :class="{ 'opacity-45 grayscale': fainted }"
    :style="boxStyle"
    :title="dex ? `${name} · #${String(dex).padStart(3, '0')}` : name"
  >
    <img
      v-if="src && !failed"
      :src="src"
      :alt="`${name} sprite`"
      class="h-full w-full object-contain p-0.5 [image-rendering:pixelated]"
      loading="lazy"
      decoding="async"
      @error="failed = true"
    />
    <span v-else class="pokeball-fallback" aria-hidden="true">
      <span class="pokeball-center" />
    </span>
    <span v-if="dex" class="absolute right-1 bottom-0.5 font-mono text-[7px] font-bold text-white/30">#{{ String(dex).padStart(3, '0') }}</span>
  </span>
</template>

<style scoped>
.pokemon-sprite {
  box-shadow: inset 0 1px rgba(255, 255, 255, 0.04);
}

.pokeball-fallback {
  position: relative;
  width: 58%;
  aspect-ratio: 1;
  border: 2px solid rgb(71 85 105);
  border-radius: 9999px;
  background: linear-gradient(to bottom, rgb(127 29 29) 0 45%, rgb(30 41 59) 45% 55%, rgb(226 232 240) 55% 100%);
  opacity: 0.72;
}

.pokeball-center {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 28%;
  aspect-ratio: 1;
  transform: translate(-50%, -50%);
  border: 2px solid rgb(71 85 105);
  border-radius: 9999px;
  background: rgb(226 232 240);
}
</style>
