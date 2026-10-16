<script setup lang="ts">
import StatusPill from '../shared/components/StatusPill.vue'
import type { RunOutputs } from './mediaQuery'

// One run's outputs (runOutputs): a row per output it can have, then a single
// line for everything the host can't make for it.
defineProps<{ outputs: RunOutputs }>()
</script>

<template>
  <div>
    <dl class="ui-kv">
      <template v-for="row in outputs.rows" :key="row.id">
        <dt>{{ row.label }}</dt>
        <dd>
          <StatusPill :status="row.status" />
          <span v-if="row.detail" class="block text-meta text-fg-2">{{ row.detail }}</span>
        </dd>
      </template>
    </dl>
    <p v-if="outputs.unavailable.length" class="mt-2 max-w-[72ch] text-meta text-fg-2">
      <span aria-hidden="true">■</span> Not available: {{ outputs.unavailable.join(' ') }}
    </p>
  </div>
</template>
