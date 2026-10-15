<script setup lang="ts">
import { Menu, MenuButton, MenuItem, MenuItems } from '@headlessui/vue'

// Row menu for a failure group: read actions first, destructive last after a
// separator (DESIGN §4.9).
defineProps<{ groupKey: string; latestRun: string; dismissable: boolean }>()
const emit = defineEmits<{ live: []; copy: []; dismiss: []; cleanup: [] }>()
</script>

<template>
  <Menu as="div" class="relative inline-block text-left">
    <MenuButton class="ui-btn ui-btn-sm ui-icon-btn max-md:size-10" :aria-label="`More actions for failure group ${groupKey}`">
      <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><circle cx="3" cy="8" r="1.4" /><circle cx="8" cy="8" r="1.4" /><circle cx="13" cy="8" r="1.4" /></svg>
    </MenuButton>
    <MenuItems class="absolute right-0 z-30 mt-1 w-56 rounded-md border border-rule-strong bg-raised p-1 shadow-[0_8px_24px_rgb(0_0_0/45%)] focus:outline-none">
      <MenuItem v-if="latestRun" v-slot="{ active }">
        <button type="button" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5 text-left" @click="emit('live')">Open latest run in Live</button>
      </MenuItem>
      <MenuItem v-slot="{ active }">
        <button type="button" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5 text-left" @click="emit('copy')">Copy group key</button>
      </MenuItem>
      <div class="my-1 h-px bg-rule" role="separator" />
      <MenuItem v-if="dismissable" v-slot="{ active }">
        <button type="button" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5 text-left" @click="emit('dismiss')">Dismiss…</button>
      </MenuItem>
      <MenuItem v-slot="{ active }">
        <button type="button" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5 text-left text-danger" @click="emit('cleanup')">Delete matching runs…</button>
      </MenuItem>
    </MenuItems>
  </Menu>
</template>
