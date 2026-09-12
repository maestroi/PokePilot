<script setup lang="ts">
import { NAlert, NEmpty, NSkeleton, NSpace } from 'naive-ui'

type ResourceState = 'loading' | 'ready' | 'refreshing' | 'stale' | 'empty' | 'error'

withDefaults(defineProps<{
  state: ResourceState
  title?: string
  message?: string
  rows?: number
}>(), {
  title: '',
  message: '',
  rows: 3
})
</script>

<template>
  <NSpace v-if="state === 'loading'" vertical :size="10" aria-label="Loading">
    <NSkeleton v-for="row in rows" :key="row" text :repeat="1" />
  </NSpace>

  <NAlert v-else-if="state === 'error'" type="error" :title="title || 'Unable to load'">
    {{ message || 'This section could not be loaded. Other parts of the page can keep working.' }}
  </NAlert>

  <NAlert v-else-if="state === 'stale'" type="warning" :title="title || 'Showing last known state'">
    {{ message || 'The latest refresh failed. Existing data is being kept visible while the connection recovers.' }}
  </NAlert>

  <NEmpty v-else-if="state === 'empty'" :description="message || title || 'Nothing to show yet'" />

  <div v-else class="resource-state__content" :aria-busy="state === 'refreshing'">
    <slot />
    <div v-if="state === 'refreshing'" class="resource-state__refresh" aria-live="polite">
      Refreshing…
    </div>
  </div>
</template>

<style scoped>
.resource-state__content {
  position: relative;
}

.resource-state__refresh {
  position: absolute;
  top: 0;
  right: 0;
  color: #718599;
  font: 600 10px/1.2 "SFMono-Regular", Consolas, monospace;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}
</style>
