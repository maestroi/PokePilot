<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { browserPokemonAssetUrl } from '../pokemonAssetProxy'
import { itemSpriteUrl, itemToken, itemVisualKind } from '../pokemonAssets'

const props = withDefaults(defineProps<{
  name: string
  size?: number
}>(), {
  size: 30
})

const failed = ref(false)
const src = computed(() => browserPokemonAssetUrl(itemSpriteUrl(props.name)))
const kind = computed(() => itemVisualKind(props.name))
const token = computed(() => itemToken(props.name))
const boxStyle = computed(() => ({ width: `${props.size}px`, height: `${props.size}px` }))

watch(src, () => { failed.value = false })
</script>

<template>
  <span
    :class="['item-icon inline-grid shrink-0 place-items-center overflow-hidden rounded-md border', `kind-${kind}`]"
    :style="boxStyle"
    aria-hidden="true"
  >
    <img
      v-if="src && !failed"
      :src="src"
      alt=""
      class="h-full w-full object-contain p-0.5 [image-rendering:pixelated]"
      loading="lazy"
      decoding="async"
      @error="failed = true"
    />
    <span v-else class="font-mono text-[7px] font-black tracking-[-0.04em]">{{ token }}</span>
  </span>
</template>

<style scoped>
.item-icon {
  border-color: rgba(148, 163, 184, 0.14);
  background: rgba(2, 6, 23, 0.38);
  color: rgb(148 163 184);
  box-shadow: inset 0 1px rgba(255, 255, 255, 0.035);
}

.kind-ball {
  border-color: rgba(248, 113, 113, 0.22);
  background: rgba(127, 29, 29, 0.12);
  color: rgb(252 165 165);
}

.kind-tm {
  border-color: rgba(96, 165, 250, 0.24);
  background: rgba(30, 64, 175, 0.14);
  color: rgb(147 197 253);
}

.kind-hm {
  border-color: rgba(45, 212, 191, 0.24);
  background: rgba(15, 118, 110, 0.14);
  color: rgb(94 234 212);
}

.kind-key {
  border-color: rgba(250, 204, 21, 0.2);
  background: rgba(113, 63, 18, 0.12);
  color: rgb(253 224 71);
}
</style>
