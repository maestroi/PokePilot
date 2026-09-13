<script setup lang="ts">
import { Dialog, DialogPanel, DialogTitle, TransitionChild, TransitionRoot } from '@headlessui/vue'
import { ExclamationTriangleIcon } from '@heroicons/vue/24/outline'

withDefaults(defineProps<{
  open: boolean
  title: string
  message: string
  confirmLabel?: string
  busy?: boolean
  danger?: boolean
}>(), {
  confirmLabel: 'Confirm',
  busy: false,
  danger: false
})

const emit = defineEmits<{
  confirm: []
  close: []
}>()
</script>

<template>
  <TransitionRoot as="template" :show="open">
    <Dialog class="relative z-[70]" @close="busy ? undefined : emit('close')">
      <TransitionChild
        as="template"
        enter="ease-out duration-150"
        enter-from="opacity-0"
        enter-to="opacity-100"
        leave="ease-in duration-100"
        leave-from="opacity-100"
        leave-to="opacity-0"
      >
        <div class="fixed inset-0 bg-black/75 backdrop-blur-[1px]" />
      </TransitionChild>

      <div class="fixed inset-0 z-[70] grid place-items-center overflow-y-auto p-4">
        <TransitionChild
          as="template"
          enter="ease-out duration-150"
          enter-from="opacity-0 translate-y-2 scale-95"
          enter-to="opacity-100 translate-y-0 scale-100"
          leave="ease-in duration-100"
          leave-from="opacity-100 translate-y-0 scale-100"
          leave-to="opacity-0 translate-y-2 scale-95"
        >
          <DialogPanel class="w-full max-w-md rounded-lg border border-white/10 bg-[#0b111a] p-5 shadow-2xl shadow-black/50">
            <div class="flex gap-3">
              <div :class="[danger ? 'bg-rose-400/10 text-rose-300' : 'bg-amber-400/10 text-amber-300', 'flex size-10 shrink-0 items-center justify-center rounded-full']">
                <ExclamationTriangleIcon class="size-5" aria-hidden="true" />
              </div>
              <div class="min-w-0 flex-1">
                <DialogTitle class="text-sm font-semibold text-white">{{ title }}</DialogTitle>
                <p class="mt-2 text-sm leading-6 break-words whitespace-pre-wrap text-slate-400">{{ message }}</p>
              </div>
            </div>

            <div class="mt-5 flex justify-end gap-2">
              <button
                type="button"
                class="rounded-md bg-white/8 px-3 py-2 text-xs font-semibold text-slate-200 ring-1 ring-white/10 hover:bg-white/12 disabled:opacity-50"
                :disabled="busy"
                @click="emit('close')"
              >
                Cancel
              </button>
              <button
                type="button"
                :class="[
                  danger ? 'bg-rose-500 hover:bg-rose-400' : 'bg-cyan-500 hover:bg-cyan-400',
                  'rounded-md px-3 py-2 text-xs font-semibold text-white shadow-sm disabled:cursor-wait disabled:opacity-60'
                ]"
                :disabled="busy"
                @click="emit('confirm')"
              >
                {{ busy ? 'Working…' : confirmLabel }}
              </button>
            </div>
          </DialogPanel>
        </TransitionChild>
      </div>
    </Dialog>
  </TransitionRoot>
</template>
