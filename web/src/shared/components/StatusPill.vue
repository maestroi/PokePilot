<script setup lang="ts">
import { toneClass, type StatusPresentation } from '../status'

defineProps<{ status: StatusPresentation }>()
</script>

<template>
  <span :class="['ui-pill', toneClass(status.tone)]">
    <span :class="status.live ? 'motion-safe:animate-pulse' : ''" aria-hidden="true">{{ status.glyph }}</span>
    {{ status.label }}
  </span>
</template>
