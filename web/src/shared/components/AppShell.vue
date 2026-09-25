<script setup lang="ts">
import { computed } from 'vue'
import { Disclosure, DisclosureButton, DisclosurePanel } from '@headlessui/vue'
import { Bars3Icon, XMarkIcon } from '@heroicons/vue/24/outline'
import type { AppNavItem } from '../types'

const props = withDefaults(defineProps<{
  eyebrow: string
  title: string
  subtitle: string
  mode: 'private' | 'public'
  navigation?: AppNavItem[]
  showIntro?: boolean
}>(), {
  navigation: () => [],
  showIntro: true
})

const publicNavigation = computed<AppNavItem[]>(() => {
  if (props.mode !== 'public') return []
  const path = window.location.pathname
  return [
    { name: 'Watch', href: '/', current: path === '/' || path.startsWith('/runs/') },
    { name: 'Explore', href: '/explore', current: path === '/explore' || path === '/explore/' || path === '/world' || path === '/world/' },
    { name: 'Replays', href: '/replays', current: path === '/replays' || path.startsWith('/replays/') }
  ]
})

const effectiveNavigation = computed(() => props.navigation.length ? props.navigation : publicNavigation.value)
</script>

<template>
  <div
    :class="[
      'min-h-screen text-[var(--poke-text)]',
      mode === 'public'
        ? 'bg-[radial-gradient(circle_at_18%_-8%,rgba(37,99,235,.16),transparent_28rem),radial-gradient(circle_at_82%_0%,rgba(124,58,237,.12),transparent_30rem),#040812]'
        : 'bg-transparent'
    ]"
  >
    <Disclosure
      as="nav"
      :class="[
        'sticky top-0 z-40 border-b',
        mode === 'public'
          ? 'border-white/8 bg-[#050914]/90 shadow-lg shadow-black/20 backdrop-blur-xl'
          : 'border-[var(--poke-border)] bg-[#0f141c]'
      ]"
      v-slot="{ open }"
    >
      <div :class="['flex items-center gap-2 px-2 sm:px-3', mode === 'public' ? 'h-14' : 'h-11']">
        <div
          :class="[
            'flex min-w-0 items-center gap-2',
            mode === 'public' ? 'pr-4 sm:min-w-[12rem]' : 'border-r border-[var(--poke-border)] pr-3 sm:w-[13.125rem]'
          ]"
        >
          <span
            :class="[
              'shrink-0',
              mode === 'public'
                ? 'size-7 rounded-lg bg-gradient-to-br from-cyan-300 via-blue-500 to-violet-500 shadow-[0_0_24px_rgba(59,130,246,.28)] ring-1 ring-white/15'
                : 'size-2 rounded-sm bg-[var(--poke-cyan)]'
            ]"
            aria-hidden="true"
          />
          <div class="min-w-0 leading-tight">
            <strong :class="['block truncate text-white', mode === 'public' ? 'text-base font-black tracking-tight' : 'text-[15px]']">
              {{ mode === 'private' ? 'RomPilot Admin' : 'RomPilot' }}
            </strong>
            <span :class="['hidden text-[9px] tracking-[0.04em] uppercase sm:block', mode === 'public' ? 'text-cyan-200/55' : 'text-[var(--poke-muted)]']">
              {{ mode === 'private' ? 'Control plane' : 'Autonomous game runs' }}
            </span>
          </div>
        </div>

        <div v-if="effectiveNavigation.length" class="hidden min-w-0 flex-1 items-stretch self-stretch sm:flex">
          <a
            v-for="item in effectiveNavigation"
            :key="item.name"
            :href="item.href"
            :aria-current="item.current ? 'page' : undefined"
            :class="[
              item.current
                ? (mode === 'public' ? 'border-cyan-300 text-white' : 'border-[var(--poke-cyan)] text-white')
                : (mode === 'public' ? 'border-transparent text-slate-400 hover:bg-white/5 hover:text-white' : 'border-transparent text-[var(--poke-muted)] hover:bg-[var(--poke-panel)] hover:text-white'),
              'inline-flex items-center border-b-2 px-3 text-[13px] font-semibold transition-colors'
            ]"
          >
            {{ item.name }}
            <span v-if="item.badge !== undefined" class="ml-1.5 rounded-full bg-white/10 px-1.5 py-px text-[10px] font-medium text-slate-300">{{ item.badge }}</span>
          </a>
        </div>

        <div v-else class="min-w-0 flex-1">
          <span class="poke-kicker block">{{ eyebrow }}</span>
          <h1 class="truncate text-sm font-semibold text-white">{{ title }}</h1>
        </div>

        <div class="ml-auto hidden min-w-0 items-center gap-2.5 text-[11px] text-[var(--poke-muted)] lg:flex">
          <slot name="summary" />
        </div>

        <div class="flex shrink-0 items-center gap-1.5">
          <slot name="actions" />
          <DisclosureButton
            v-if="effectiveNavigation.length"
            class="inline-flex items-center justify-center rounded-sm p-1.5 text-[var(--poke-muted)] hover:bg-white/5 hover:text-white sm:hidden"
          >
            <span class="sr-only">Open navigation</span>
            <Bars3Icon v-if="!open" class="size-5" aria-hidden="true" />
            <XMarkIcon v-else class="size-5" aria-hidden="true" />
          </DisclosureButton>
        </div>
      </div>

      <DisclosurePanel v-if="effectiveNavigation.length" class="border-t border-[var(--poke-border)] sm:hidden">
        <a
          v-for="item in effectiveNavigation"
          :key="item.name"
          :href="item.href"
          :aria-current="item.current ? 'page' : undefined"
          :class="[
            item.current
              ? 'border-[var(--poke-cyan)] bg-[var(--poke-panel)] text-white'
              : 'border-transparent text-[var(--poke-muted)] hover:bg-white/5 hover:text-white',
            'block border-l-2 px-3 py-2 text-sm font-semibold'
          ]"
        >
          {{ item.name }}
        </a>
        <div class="border-t border-[var(--poke-border)] px-3 py-2 text-[11px] text-[var(--poke-muted)]">
          <slot name="summary" />
        </div>
      </DisclosurePanel>
    </Disclosure>

    <main :class="mode === 'public' ? 'px-2.5 py-3 sm:px-4 sm:py-4 xl:px-5' : 'px-2.5 py-2 sm:px-3'">
      <div v-if="showIntro && (title || subtitle)" class="mb-3 max-w-4xl">
        <span v-if="eyebrow && effectiveNavigation.length" class="poke-kicker block">{{ eyebrow }}</span>
        <h1 v-if="effectiveNavigation.length" class="text-xl font-semibold text-white">{{ title }}</h1>
        <p v-if="subtitle" class="mt-1 max-w-[70ch] text-[13px] text-[var(--poke-muted)]">{{ subtitle }}</p>
      </div>
      <slot />
    </main>
  </div>
</template>
