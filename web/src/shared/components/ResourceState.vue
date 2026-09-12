<script setup lang="ts">
import { ExclamationTriangleIcon, SignalSlashIcon } from '@heroicons/vue/20/solid'
import type { ResourceState } from '../resource'

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
  <div v-if="state === 'loading'" class="space-y-3" aria-label="Loading" aria-busy="true">
    <div v-for="row in rows" :key="row" class="animate-pulse">
      <div class="h-3 rounded-sm bg-white/10" :class="row === rows ? 'w-2/3' : 'w-full'" />
    </div>
  </div>

  <div v-else-if="state === 'error'" class="border-l-4 border-rose-400 bg-rose-400/10 p-4" role="alert">
    <div class="flex gap-3">
      <ExclamationTriangleIcon class="mt-0.5 size-5 shrink-0 text-rose-300" aria-hidden="true" />
      <div>
        <h3 class="text-sm font-semibold text-rose-100">{{ title || 'Unable to load' }}</h3>
        <p class="mt-1 text-sm leading-5 text-rose-200/75">
          {{ message || 'This section could not be loaded. Other parts of the page can keep working.' }}
        </p>
        <div class="mt-3"><slot name="actions" /></div>
      </div>
    </div>
  </div>

  <div v-else-if="state === 'empty'" class="rounded-lg border border-dashed border-white/15 px-6 py-10 text-center">
    <div class="mx-auto flex size-10 items-center justify-center rounded-lg bg-white/5 ring-1 ring-white/10">
      <SignalSlashIcon class="size-5 text-slate-500" aria-hidden="true" />
    </div>
    <h3 class="mt-3 text-sm font-semibold text-white">{{ title || 'Nothing to show yet' }}</h3>
    <p v-if="message" class="mx-auto mt-1 max-w-lg text-sm leading-5 text-slate-500">{{ message }}</p>
    <div class="mt-3"><slot name="actions" /></div>
  </div>

  <div v-else class="relative" :aria-busy="state === 'refreshing'">
    <div v-if="state === 'stale'" class="mb-4 border-l-4 border-amber-400 bg-amber-400/10 p-3" role="status">
      <div class="flex gap-3">
        <ExclamationTriangleIcon class="mt-0.5 size-5 shrink-0 text-amber-300" aria-hidden="true" />
        <div>
          <h3 class="text-sm font-semibold text-amber-100">{{ title || 'Showing last known state' }}</h3>
          <p class="mt-0.5 text-sm leading-5 text-amber-100/70">
            {{ message || 'The latest refresh failed. Existing data stays visible while the connection recovers.' }}
          </p>
          <div class="mt-2"><slot name="actions" /></div>
        </div>
      </div>
    </div>

    <slot />

    <div v-if="state === 'refreshing'" class="absolute -top-1 right-0 flex items-center gap-1.5 text-[10px] font-semibold tracking-[0.08em] text-slate-500 uppercase" aria-live="polite">
      <span class="size-1.5 animate-pulse rounded-full bg-cyan-300" />
      Refreshing
    </div>
  </div>
</template>
