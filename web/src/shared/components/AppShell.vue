<script setup lang="ts">
import { computed } from 'vue'
import { Disclosure, DisclosureButton, DisclosurePanel } from '@headlessui/vue'
import { Bars3Icon, XMarkIcon } from '@heroicons/vue/24/outline'
import type { PublicCapability } from '../publicCapabilities'
import type { AppNavItem } from '../types'

const props = withDefaults(defineProps<{
  eyebrow: string
  title: string
  subtitle: string
  mode: 'private' | 'public'
  navigation?: AppNavItem[]
  publicCapabilities?: PublicCapability[]
  showIntro?: boolean
}>(), {
  navigation: () => [],
  publicCapabilities: () => ['live', 'replay'] as PublicCapability[],
  showIntro: true
})

const publicNavigation = computed<AppNavItem[]>(() => {
  if (props.mode !== 'public') return []
  const path = window.location.pathname
  const capabilities = new Set(props.publicCapabilities)
  const items: AppNavItem[] = []
  if (capabilities.has('live')) {
    items.push({ name: 'Watch', href: '/', current: path === '/' || path.startsWith('/runs/') })
  }
  if (capabilities.has('worldMap')) {
    items.push({ name: 'Explore', href: '/explore', current: path === '/explore' || path === '/explore/' || path === '/world' || path === '/world/' })
  }
  if (capabilities.has('replay')) {
    items.push({ name: 'Replays', href: '/replays', current: path === '/replays' || path.startsWith('/replays/') })
  }
  return items
})

const effectiveNavigation = computed(() => props.navigation.length ? props.navigation : publicNavigation.value)
</script>

<template>
  <div
    :class="[
      'min-h-screen text-[var(--poke-text)]',
      mode === 'public'
        ? 'public-shell bg-[radial-gradient(circle_at_15%_-10%,rgba(239,68,68,.13),transparent_24rem),radial-gradient(circle_at_82%_2%,rgba(250,204,21,.08),transparent_25rem),radial-gradient(circle_at_50%_100%,rgba(67,56,202,.13),transparent_35rem),#070a16]'
        : 'bg-transparent'
    ]"
  >
    <Disclosure
      as="nav"
      :class="[
        'sticky top-0 z-40 border-b',
        mode === 'public'
          ? 'border-[#3b4167]/55 bg-[#080b18]/92 shadow-lg shadow-black/25 backdrop-blur-xl'
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
            v-if="mode === 'public'"
            class="public-pokeball shrink-0"
            aria-hidden="true"
          ><span /></span>
          <span
            v-else
            class="size-2 shrink-0 rounded-sm bg-[var(--poke-cyan)]"
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
                ? (mode === 'public' ? 'border-amber-300 text-white' : 'border-[var(--poke-cyan)] text-white')
                : (mode === 'public' ? 'border-transparent text-slate-400 hover:bg-white/5 hover:text-amber-100' : 'border-transparent text-[var(--poke-muted)] hover:bg-[var(--poke-panel)] hover:text-white'),
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


<style scoped>
.public-shell {
  font-family: ui-rounded, "Avenir Next", "Segoe UI", system-ui, sans-serif;
}

.public-pokeball {
  position: relative;
  display: inline-block;
  width: 1.75rem;
  height: 1.75rem;
  overflow: hidden;
  border: 2px solid #f8fafc;
  border-radius: 9999px;
  background: linear-gradient(to bottom, #e84a4a 0 45%, #202641 45% 55%, #f5f1df 55% 100%);
  box-shadow: 0 0 0 1px rgb(0 0 0 / 45%), 0 0 20px rgb(239 68 68 / 18%);
}

.public-pokeball::before {
  position: absolute;
  top: 50%;
  left: 0;
  width: 100%;
  height: 2px;
  background: #11152a;
  content: "";
  transform: translateY(-50%);
}

.public-pokeball > span {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 0.5rem;
  height: 0.5rem;
  border: 2px solid #11152a;
  border-radius: 9999px;
  background: #f8fafc;
  content: "";
  transform: translate(-50%, -50%);
}
</style>
