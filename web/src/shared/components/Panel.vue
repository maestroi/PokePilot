<script setup lang="ts">
withDefaults(defineProps<{
  title: string
  description?: string
  compact?: boolean
}>(), {
  description: '',
  compact: false
})
</script>

<template>
  <section class="poke-panel overflow-hidden rounded-lg">
    <header class="flex items-start justify-between gap-4 border-b border-white/10 px-4 py-3 sm:px-5">
      <div class="min-w-0">
        <h2 class="text-sm font-semibold text-white">{{ title }}</h2>
        <p v-if="description" class="mt-0.5 text-xs leading-5 text-slate-500">{{ description }}</p>
      </div>
      <div class="shrink-0">
        <slot name="actions" />
      </div>
    </header>
    <div :class="compact ? 'p-3 sm:p-4' : 'p-4 sm:p-5'">
      <slot />
    </div>
  </section>
</template>
