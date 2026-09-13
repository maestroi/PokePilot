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
  <section class="poke-panel overflow-hidden rounded-sm">
    <header class="flex items-center justify-between gap-3 border-b border-white/10 px-3 py-2">
      <div class="min-w-0">
        <h2 class="text-[13px] font-semibold text-white">{{ title }}</h2>
        <p v-if="description" class="mt-0.5 text-[11px] leading-4 text-slate-500">{{ description }}</p>
      </div>
      <div class="shrink-0">
        <slot name="actions" />
      </div>
    </header>
    <div :class="compact ? 'p-2.5' : 'p-3'">
      <slot />
    </div>
  </section>
</template>
