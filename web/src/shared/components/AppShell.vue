<script setup lang="ts">
import { NTag } from 'naive-ui'

defineProps<{
  eyebrow: string
  title: string
  subtitle: string
  mode: 'private' | 'public'
}>()
</script>

<template>
  <div class="poke-app">
    <main class="poke-shell">
      <header class="poke-shell__header">
        <div class="poke-shell__identity">
          <span class="poke-shell__eyebrow">{{ eyebrow }}</span>
          <h1 class="poke-shell__title">{{ title }}</h1>
          <p class="poke-shell__subtitle">{{ subtitle }}</p>
        </div>
        <NTag :bordered="false" size="small" :type="mode === 'private' ? 'info' : 'success'">
          {{ mode === 'private' ? 'Private operator' : 'Public read-only' }}
        </NTag>
      </header>

      <slot />
    </main>
  </div>
</template>
