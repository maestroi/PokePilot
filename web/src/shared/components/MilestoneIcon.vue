<script setup lang="ts">
import { computed } from 'vue'
import BadgeIcon from './BadgeIcon.vue'
import ItemIcon from './ItemIcon.vue'
import { milestoneBadgeName, milestoneItemName, milestoneVisualKind } from '../pokemonProgressAssets'

const props = withDefaults(defineProps<{
  name: string
  size?: number
}>(), {
  size: 24
})

const badge = computed(() => milestoneBadgeName(props.name))
const item = computed(() => milestoneItemName(props.name))
const kind = computed(() => milestoneVisualKind(props.name))
const boxStyle = computed(() => ({ width: `${props.size}px`, height: `${props.size}px` }))
</script>

<template>
  <BadgeIcon v-if="badge" :name="badge" :size="size" />
  <ItemIcon v-else-if="item" :name="item" :size="size" />
  <span
    v-else
    :class="['story-icon inline-grid shrink-0 place-items-center rounded-full', `kind-${kind}`]"
    :style="boxStyle"
    aria-hidden="true"
  >
    <span class="story-center" />
  </span>
</template>

<style scoped>
.story-icon {
  border: 1px solid rgba(148, 163, 184, 0.28);
  background: linear-gradient(to bottom, rgb(185 28 28) 0 44%, rgb(30 41 59) 44% 56%, rgb(226 232 240) 56% 100%);
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
}

.story-center {
  width: 31%;
  aspect-ratio: 1;
  border: 1px solid rgb(71 85 105);
  border-radius: 9999px;
  background: rgb(226 232 240);
}
</style>
