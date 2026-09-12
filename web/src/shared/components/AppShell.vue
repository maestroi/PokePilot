<script setup lang="ts">
import { ref } from 'vue'
import { Dialog, DialogPanel, TransitionChild, TransitionRoot } from '@headlessui/vue'
import { Bars3Icon, XMarkIcon } from '@heroicons/vue/24/outline'

export type AppNavItem = {
  name: string
  href: string
  current?: boolean
  badge?: string | number
}

withDefaults(defineProps<{
  eyebrow: string
  title: string
  subtitle: string
  mode: 'private' | 'public'
  navigation?: AppNavItem[]
}>(), {
  navigation: () => []
})

const sidebarOpen = ref(false)
</script>

<template>
  <div class="min-h-screen bg-transparent text-white">
    <template v-if="mode === 'private'">
      <TransitionRoot as="template" :show="sidebarOpen">
        <Dialog class="relative z-50 lg:hidden" @close="sidebarOpen = false">
          <TransitionChild
            as="template"
            enter="transition-opacity ease-linear duration-200"
            enter-from="opacity-0"
            enter-to="opacity-100"
            leave="transition-opacity ease-linear duration-200"
            leave-from="opacity-100"
            leave-to="opacity-0"
          >
            <div class="fixed inset-0 bg-black/75" />
          </TransitionChild>

          <div class="fixed inset-0 flex">
            <TransitionChild
              as="template"
              enter="transition ease-in-out duration-200 transform"
              enter-from="-translate-x-full"
              enter-to="translate-x-0"
              leave="transition ease-in-out duration-200 transform"
              leave-from="translate-x-0"
              leave-to="-translate-x-full"
            >
              <DialogPanel class="relative mr-14 flex w-full max-w-72 flex-1">
                <div class="flex grow flex-col overflow-y-auto border-r border-white/10 bg-[#090e16] px-5 pb-5">
                  <div class="flex h-16 shrink-0 items-center gap-3 border-b border-white/8">
                    <span class="size-2.5 rounded-sm bg-cyan-300 shadow-[0_0_18px_rgba(85,215,255,0.45)]" aria-hidden="true" />
                    <div>
                      <strong class="block text-sm tracking-wide text-white">PokéPilot</strong>
                      <span class="block text-[10px] font-medium tracking-[0.14em] text-slate-500 uppercase">Operator</span>
                    </div>
                  </div>
                  <nav class="mt-5 flex flex-1 flex-col">
                    <ul role="list" class="space-y-1">
                      <li v-for="item in navigation" :key="item.name">
                        <a
                          :href="item.href"
                          :class="[
                            item.current
                              ? 'bg-white/7 text-white ring-1 ring-white/8'
                              : 'text-slate-400 hover:bg-white/5 hover:text-white',
                            'flex items-center justify-between rounded-md px-3 py-2 text-sm font-semibold transition-colors'
                          ]"
                          @click="sidebarOpen = false"
                        >
                          <span>{{ item.name }}</span>
                          <span v-if="item.badge !== undefined" class="rounded-full bg-white/8 px-2 py-0.5 text-[11px] font-medium text-slate-300">{{ item.badge }}</span>
                        </a>
                      </li>
                    </ul>
                  </nav>
                </div>
                <button type="button" class="absolute top-4 left-full ml-3 rounded-md p-2 text-slate-300 hover:bg-white/10 hover:text-white" @click="sidebarOpen = false">
                  <span class="sr-only">Close navigation</span>
                  <XMarkIcon class="size-6" aria-hidden="true" />
                </button>
              </DialogPanel>
            </TransitionChild>
          </div>
        </Dialog>
      </TransitionRoot>

      <aside class="fixed inset-y-0 left-0 z-40 hidden w-64 border-r border-white/10 bg-[#090e16] lg:flex lg:flex-col">
        <div class="flex h-16 shrink-0 items-center gap-3 border-b border-white/8 px-5">
          <span class="size-2.5 rounded-sm bg-cyan-300 shadow-[0_0_18px_rgba(85,215,255,0.45)]" aria-hidden="true" />
          <div>
            <strong class="block text-sm tracking-wide text-white">PokéPilot</strong>
            <span class="block text-[10px] font-medium tracking-[0.14em] text-slate-500 uppercase">Operator</span>
          </div>
        </div>
        <nav class="flex flex-1 flex-col overflow-y-auto px-3 py-5">
          <ul role="list" class="space-y-1">
            <li v-for="item in navigation" :key="item.name">
              <a
                :href="item.href"
                :class="[
                  item.current
                    ? 'bg-white/7 text-white ring-1 ring-white/8'
                    : 'text-slate-400 hover:bg-white/5 hover:text-white',
                  'flex items-center justify-between rounded-md px-3 py-2 text-sm font-semibold transition-colors'
                ]"
              >
                <span>{{ item.name }}</span>
                <span v-if="item.badge !== undefined" class="rounded-full bg-white/8 px-2 py-0.5 text-[11px] font-medium text-slate-300">{{ item.badge }}</span>
              </a>
            </li>
          </ul>
        </nav>
      </aside>
    </template>

    <div :class="mode === 'private' ? 'lg:pl-64' : ''">
      <header class="sticky top-0 z-30 border-b border-white/10 bg-[#070b11]/90 backdrop-blur">
        <div class="flex h-16 items-center gap-4 px-4 sm:px-6 lg:px-8">
          <button v-if="mode === 'private'" type="button" class="-m-2 p-2 text-slate-400 hover:text-white lg:hidden" @click="sidebarOpen = true">
            <span class="sr-only">Open navigation</span>
            <Bars3Icon class="size-6" aria-hidden="true" />
          </button>

          <div v-if="mode === 'public'" class="flex items-center gap-3">
            <span class="size-2.5 rounded-sm bg-cyan-300 shadow-[0_0_18px_rgba(85,215,255,0.45)]" aria-hidden="true" />
            <strong class="text-sm tracking-wide text-white">PokéPilot</strong>
          </div>

          <div class="min-w-0 flex-1">
            <span class="poke-kicker block">{{ eyebrow }}</span>
            <div class="flex min-w-0 items-baseline gap-3">
              <h1 class="truncate text-base font-semibold text-white sm:text-lg">{{ title }}</h1>
              <span
                :class="[
                  mode === 'private' ? 'border-cyan-300/20 bg-cyan-300/8 text-cyan-200' : 'border-emerald-300/20 bg-emerald-300/8 text-emerald-200',
                  'hidden rounded-full border px-2 py-0.5 text-[10px] font-semibold tracking-[0.08em] uppercase sm:inline'
                ]"
              >
                {{ mode === 'private' ? 'Private' : 'Read-only' }}
              </span>
            </div>
          </div>

          <div class="flex shrink-0 items-center gap-2">
            <slot name="actions" />
          </div>
        </div>
      </header>

      <main class="px-4 py-5 sm:px-6 lg:px-8 lg:py-7">
        <div class="mb-5 max-w-4xl">
          <p class="text-sm leading-6 text-slate-400">{{ subtitle }}</p>
        </div>
        <slot />
      </main>
    </div>
  </div>
</template>
