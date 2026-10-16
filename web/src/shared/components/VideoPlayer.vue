<script lang="ts">
import { ref } from 'vue'

// One playback speed for every player in the console, remembered per browser.
const RATES = [1, 2, 4, 8, 16]
const KEY = 'pokepilot.replayPlaybackRate'
function stored(): number {
  try {
    const value = Number(window.localStorage.getItem(KEY))
    return RATES.includes(value) ? value : 1
  } catch {
    return 1
  }
}
const rate = ref(stored())
</script>

<script setup lang="ts">
import { watch } from 'vue'

// A <video> with a speed picker overlaid top-right. Attributes and listeners
// go to the <video>; the nearest positioned ancestor anchors the picker.
defineOptions({ inheritAttrs: false })
const video = ref<HTMLVideoElement | null>(null)
defineExpose({ video })

function apply(): void {
  if (video.value) video.value.playbackRate = rate.value
}
watch(rate, (value) => {
  try { window.localStorage.setItem(KEY, String(value)) } catch { /* storage off: the speed just isn't remembered */ }
  apply()
})
</script>

<template>
  <video ref="video" v-bind="$attrs" @loadedmetadata="apply" @canplay="apply" />
  <label class="absolute top-2 right-2 flex h-7 items-center gap-1.5 rounded-[3px] border border-control bg-well/90 px-2">
    <span class="text-meta text-fg-2">Speed</span>
    <select v-model.number="rate" class="bg-transparent text-meta text-fg">
      <option v-for="value in RATES" :key="value" :value="value" class="bg-raised">{{ value }}×</option>
    </select>
  </label>
</template>
