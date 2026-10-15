<script setup lang="ts">
import { Dialog, DialogPanel, DialogTitle, TransitionChild, TransitionRoot } from '@headlessui/vue'

withDefaults(defineProps<{
  open: boolean
  title: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  busy?: boolean
  danger?: boolean
}>(), {
  confirmLabel: 'Confirm',
  cancelLabel: 'Cancel',
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
    <Dialog class="operator-dialog relative z-[70]" @close="busy ? undefined : emit('close')">
      <TransitionChild
        as="template"
        enter="ease-out duration-150"
        enter-from="opacity-0"
        enter-to="opacity-100"
        leave="ease-in duration-100"
        leave-from="opacity-100"
        leave-to="opacity-0"
      >
        <div class="fixed inset-0 bg-black/70" />
      </TransitionChild>

      <div class="fixed inset-0 z-[70] grid place-items-center overflow-y-auto p-4">
        <TransitionChild
          as="template"
          enter="ease-out duration-150 motion-reduce:duration-0"
          enter-from="opacity-0"
          enter-to="opacity-100"
          leave="ease-in duration-100 motion-reduce:duration-0"
          leave-from="opacity-100"
          leave-to="opacity-0"
        >
          <DialogPanel class="w-full max-w-[480px] rounded-md border border-rule-strong bg-raised p-5 text-fg shadow-[0_8px_24px_rgb(0_0_0/45%)]">
            <DialogTitle class="flex items-center gap-2 text-[14px] leading-5 font-semibold">
              <span :class="danger ? 'text-danger' : 'text-warn'" aria-hidden="true">▲</span>
              {{ title }}
            </DialogTitle>
            <p class="mt-2 text-[13px] leading-5 break-words whitespace-pre-wrap text-fg-2">{{ message }}</p>

            <div class="mt-5 flex justify-end gap-2">
              <button
                type="button"
                class="h-8 rounded-[3px] border border-control px-3 text-[13px] font-medium hover:bg-selected focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent disabled:opacity-50"
                :disabled="busy"
                @click="emit('close')"
              >
                {{ cancelLabel }}
              </button>
              <button
                type="button"
                :class="[
                  danger ? 'border-danger-solid bg-danger-solid text-white hover:bg-[#c7535a]' : 'border-accent bg-accent text-on-accent hover:bg-[#a6c6e8]',
                  'h-8 rounded-[3px] border px-3 text-[13px] font-semibold focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent disabled:cursor-wait disabled:opacity-60'
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
