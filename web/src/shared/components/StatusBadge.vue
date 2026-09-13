<script setup lang="ts">
withDefaults(defineProps<{
  tone?: 'neutral' | 'info' | 'success' | 'warning' | 'danger'
}>(), {
  tone: 'neutral'
})

const tones = {
  neutral: 'border-white/10 bg-white/5 text-slate-300',
  info: 'border-cyan-300/20 bg-cyan-300/10 text-cyan-200',
  success: 'border-emerald-300/20 bg-emerald-300/10 text-emerald-200',
  warning: 'border-amber-300/20 bg-amber-300/10 text-amber-200',
  danger: 'border-rose-300/20 bg-rose-300/10 text-rose-200'
}
</script>

<template>
  <span :class="[tones[tone], 'inline-flex items-center rounded-full border px-1.5 py-px text-[9px] font-bold tracking-[0.04em] uppercase']">
    <slot />
  </span>
</template>
