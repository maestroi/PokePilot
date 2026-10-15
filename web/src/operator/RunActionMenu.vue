<script setup lang="ts">
import { Menu, MenuButton, MenuItem, MenuItems } from '@headlessui/vue'

// One run menu for table rows and the detail pane: read actions first,
// destructive last after a separator (DESIGN §4.9).
defineProps<{ runId: string; mediaHref: string; up?: boolean }>()
const emit = defineEmits<{ inspect: []; copy: []; delete: [] }>()
</script>

<template>
  <Menu as="div" class="relative inline-block text-left">
    <MenuButton class="ui-btn ui-btn-sm ui-icon-btn" :aria-label="`More actions for ${runId}`">
      <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><circle cx="3" cy="8" r="1.4" /><circle cx="8" cy="8" r="1.4" /><circle cx="13" cy="8" r="1.4" /></svg>
    </MenuButton>
    <MenuItems
      :class="up ? 'bottom-full mb-1' : 'mt-1'"
      class="absolute right-0 z-30 w-52 rounded-md border border-rule-strong bg-raised p-1 shadow-[0_8px_24px_rgb(0_0_0/45%)] focus:outline-none"
    >
      <MenuItem v-slot="{ active }">
        <button type="button" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5 text-left" @click="emit('inspect')">Inspect in Live</button>
      </MenuItem>
      <MenuItem v-slot="{ active }">
        <a :href="mediaHref" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5">Open in Media</a>
      </MenuItem>
      <MenuItem v-slot="{ active }">
        <button type="button" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5 text-left" @click="emit('copy')">Copy run ID</button>
      </MenuItem>
      <div class="my-1 h-px bg-rule" role="separator" />
      <MenuItem v-slot="{ active }">
        <button type="button" :class="active ? 'bg-selected' : ''" class="flex h-8 w-full items-center rounded-[3px] px-2.5 text-left text-danger" @click="emit('delete')">Delete run…</button>
      </MenuItem>
    </MenuItems>
  </Menu>
</template>
