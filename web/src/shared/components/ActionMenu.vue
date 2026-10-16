<script setup lang="ts">
import { Menu, MenuButton, MenuItem, MenuItems } from '@headlessui/vue'

// Row "⋯" menu (DESIGN §4.9). Order items read → state change → destructive;
// `separator` draws a rule above an item. `checked` renders a menuitemcheckbox.
export type ActionMenuItem = {
  label: string
  select?: () => void
  href?: string
  external?: boolean
  danger?: boolean
  separator?: boolean
  checked?: boolean
}
defineProps<{ label: string; items: ActionMenuItem[] }>()
</script>

<template>
  <Menu as="div" class="relative inline-block text-left">
    <MenuButton class="ui-btn ui-btn-sm ui-icon-btn max-md:size-10" :aria-label="label">
      <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true"><circle cx="3" cy="8" r="1.4" /><circle cx="8" cy="8" r="1.4" /><circle cx="13" cy="8" r="1.4" /></svg>
    </MenuButton>
    <MenuItems class="absolute right-0 z-30 mt-1 w-60 rounded-md border border-rule-strong bg-raised p-1 shadow-[0_8px_24px_rgb(0_0_0/45%)] focus:outline-none">
      <template v-for="item in items" :key="item.label">
        <div v-if="item.separator" class="my-1 h-px bg-rule" role="separator" />
        <MenuItem v-slot="{ active }">
          <a
            v-if="item.href"
            :href="item.href"
            :target="item.external ? '_blank' : undefined"
            :rel="item.external ? 'noopener' : undefined"
            :class="[active ? 'bg-selected' : '', item.danger ? 'text-danger' : '']"
            class="flex h-8 w-full items-center rounded-[3px] px-2.5"
          >{{ item.label }}<span v-if="item.external" class="ml-1" aria-hidden="true">↗</span></a>
          <button
            v-else
            type="button"
            :role="item.checked === undefined ? undefined : 'menuitemcheckbox'"
            :aria-checked="item.checked === undefined ? undefined : item.checked"
            :class="[active ? 'bg-selected' : '', item.danger ? 'text-danger' : '']"
            class="flex h-8 w-full items-center gap-2 rounded-[3px] px-2.5 text-left"
            @click="item.select?.()"
          >
            <span class="min-w-0 flex-1 truncate">{{ item.label }}</span>
            <span v-if="item.checked" class="text-accent" aria-hidden="true">✓</span>
          </button>
        </MenuItem>
      </template>
    </MenuItems>
  </Menu>
</template>
