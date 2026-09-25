<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { browserPokemonAssetUrl } from '../pokemonAssetProxy'
import { pokemonBackSpriteUrl, pokemonDexNumber, pokemonSpriteUrl } from '../pokemonAssets'

const props = withDefaults(defineProps<{
  name: string
  size?: number
  fainted?: boolean
  back?: boolean
}>(), {
  size: 56,
  fainted: false,
  back: false
})

const failed = ref(false)
const src = computed(() => browserPokemonAssetUrl(props.back ? pokemonBackSpriteUrl(props.name) : pokemonSpriteUrl(props.name)))
const dex = computed(() => pokemonDexNumber(props.name))
const boxStyle = computed(() => ({ width: `${props.size}px`, height: `${props.size}px` }))

watch(src, () => { failed.value = false })
</script>

<template>
  <span
    class="pokemon-sprite relative inline-grid shrink-0 place-items-center overflow-hidden"
    :class="{ 'opacity-45 grayscale': fainted }"
    :style="boxStyle"
    :title="dex ? `${name} · #${String(dex).padStart(3, '0')}` : name"
  >
    <img
      v-if="src && !failed"
      :src="src"
      :alt="`${name} sprite`"
      class="h-full w-full object-contain [image-rendering:pixelated]"
      loading="lazy"
      decoding="async"
      @error="failed = true"
    />
    <span v-else class="pokeball-fallback" aria-hidden="true">
      <span class="pokeball-center" />
    </span>
  </span>
</template>

<style scoped>
.pokemon-sprite {
  image-rendering: pixelated;
}

.pokeball-fallback {
  position: relative;
  width: 58%;
  aspect-ratio: 1;
  border: 2px solid #202020;
  border-radius: 9999px;
  background: linear-gradient(to bottom, #d84018 0 45%, #202020 45% 55%, #f8f8d8 55% 100%);
}

.pokeball-center {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 28%;
  aspect-ratio: 1;
  transform: translate(-50%, -50%);
  border: 2px solid #202020;
  border-radius: 9999px;
  background: #f8f8d8;
}
</style>
