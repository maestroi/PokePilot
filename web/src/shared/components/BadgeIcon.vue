<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { browserPokemonAssetUrl } from '../pokemonAssetProxy'
import { badgeName, badgeSpriteUrl } from '../pokemonProgressAssets'

const props = withDefaults(defineProps<{
  name: string
  size?: number
}>(), {
  size: 28
})

const failed = ref(false)
const src = computed(() => browserPokemonAssetUrl(badgeSpriteUrl(props.name)))
const label = computed(() => badgeName(props.name) || props.name || 'Badge')
const boxStyle = computed(() => ({ width: `${props.size}px`, height: `${props.size}px` }))

watch(src, () => { failed.value = false })
</script>

<template>
  <span
    class="badge-icon inline-grid shrink-0 place-items-center"
    :style="boxStyle"
    :title="label"
  >
    <img
      v-if="src && !failed"
      :src="src"
      :alt="label"
      class="h-full w-full object-contain [image-rendering:pixelated] drop-shadow-[0_2px_4px_rgba(0,0,0,.45)]"
      loading="lazy"
      decoding="async"
      @error="failed = true"
    />
    <span v-else class="badge-fallback grid h-[78%] w-[78%] place-items-center rounded-full font-mono text-[8px] font-black">
      {{ label.slice(0, 1).toUpperCase() }}
    </span>
  </span>
</template>

<style scoped>
.badge-fallback {
  border: 1px solid rgba(250, 204, 21, 0.36);
  background: radial-gradient(circle at 35% 30%, rgb(254 240 138), rgb(161 98 7));
  color: rgb(66 32 6);
  box-shadow: 0 0 8px rgba(250, 204, 21, 0.18);
}
</style>
