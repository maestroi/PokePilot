<script setup lang="ts">
import { computed } from 'vue'
import { SignalIcon, SparklesIcon } from '@heroicons/vue/20/solid'

type LoaderMode = 'page' | 'frame'
type LoaderState = 'idle' | 'loading' | 'ready' | 'error'

const props = withDefaults(defineProps<{
  mode?: LoaderMode
  state?: LoaderState
}>(), {
  mode: 'frame',
  state: 'loading'
})

const title = computed(() => {
  if (props.mode === 'page') return 'Tuning into RomPilot'
  if (props.state === 'error') return 'Reconnecting live frame'
  return 'Syncing live frame'
})

const detail = computed(() => {
  if (props.mode === 'page') return 'Connecting to the public run feed and finding the best live broadcast.'
  if (props.state === 'error') return 'The run is still live. Waiting for the broadcaster to send the next frame.'
  return 'Run state is online. The first game frame is arriving now.'
})
</script>

<template>
  <div
    :class="['broadcast-loader', `broadcast-loader-${mode}`]"
    role="status"
    aria-live="polite"
    aria-busy="true"
  >
    <div class="broadcast-loader-grid" aria-hidden="true" />
    <div class="broadcast-loader-scan" aria-hidden="true" />
    <div class="broadcast-loader-glow broadcast-loader-glow-a" aria-hidden="true" />
    <div class="broadcast-loader-glow broadcast-loader-glow-b" aria-hidden="true" />

    <span class="broadcast-loader-corner corner-tl" aria-hidden="true" />
    <span class="broadcast-loader-corner corner-tr" aria-hidden="true" />
    <span class="broadcast-loader-corner corner-bl" aria-hidden="true" />
    <span class="broadcast-loader-corner corner-br" aria-hidden="true" />

    <div class="broadcast-loader-content">
      <div class="broadcast-loader-core" aria-hidden="true">
        <span class="core-ring core-ring-a" />
        <span class="core-ring core-ring-b" />
        <span class="core-orbit orbit-a"><i /></span>
        <span class="core-orbit orbit-b"><i /></span>
        <span class="core-center">
          <SparklesIcon class="size-5" />
        </span>
      </div>

      <div class="mt-5 inline-flex items-center gap-2 rounded-full border border-cyan-300/15 bg-cyan-300/7 px-2.5 py-1 text-[9px] font-black tracking-[0.14em] text-cyan-200 uppercase">
        <SignalIcon class="size-3.5" aria-hidden="true" />
        RomPilot live link
      </div>

      <h2 class="mt-3 text-base font-black tracking-tight text-white sm:text-lg">{{ title }}</h2>
      <p class="mx-auto mt-1.5 max-w-md text-xs leading-5 text-slate-400">{{ detail }}</p>

      <div class="broadcast-loader-rail mt-5" aria-hidden="true">
        <span />
      </div>

      <div class="broadcast-loader-steps mt-4" aria-hidden="true">
        <span><i /> Public feed</span>
        <span><i /> Run state</span>
        <span><i /> Frame stream</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.broadcast-loader {
  position: relative;
  display: grid;
  min-height: 18rem;
  place-items: center;
  overflow: hidden;
  border: 1px solid rgba(103, 232, 249, 0.12);
  border-radius: 1rem;
  background:
    radial-gradient(circle at 50% 42%, rgba(34, 211, 238, 0.09), transparent 24rem),
    linear-gradient(180deg, #07101b 0%, #040812 100%);
  box-shadow: inset 0 0 4rem rgba(15, 23, 42, 0.72);
  isolation: isolate;
}

.broadcast-loader-page {
  min-height: min(72vh, 44rem);
  border-radius: 1.5rem;
}

.broadcast-loader-grid {
  position: absolute;
  inset: -30%;
  background-image:
    linear-gradient(rgba(125, 211, 252, 0.055) 1px, transparent 1px),
    linear-gradient(90deg, rgba(125, 211, 252, 0.055) 1px, transparent 1px);
  background-size: 32px 32px;
  opacity: 0.55;
  transform: perspective(520px) rotateX(62deg) translateY(16%);
  transform-origin: center 70%;
  animation: loader-grid-drift 5s linear infinite;
  mask-image: linear-gradient(to bottom, transparent 8%, black 42%, transparent 92%);
}

.broadcast-loader-scan {
  position: absolute;
  inset-inline: 0;
  top: -18%;
  height: 18%;
  background: linear-gradient(to bottom, transparent, rgba(103, 232, 249, 0.08), rgba(125, 211, 252, 0.22), transparent);
  filter: blur(1px);
  animation: loader-scan 2.8s ease-in-out infinite;
}

.broadcast-loader-glow {
  position: absolute;
  border-radius: 9999px;
  filter: blur(48px);
  opacity: 0.4;
}

.broadcast-loader-glow-a {
  left: 8%;
  top: 18%;
  width: 11rem;
  height: 11rem;
  background: rgba(34, 211, 238, 0.13);
  animation: loader-glow 4s ease-in-out infinite;
}

.broadcast-loader-glow-b {
  right: 6%;
  bottom: 12%;
  width: 13rem;
  height: 13rem;
  background: rgba(139, 92, 246, 0.12);
  animation: loader-glow 4s ease-in-out 1.2s infinite;
}

.broadcast-loader-content {
  position: relative;
  z-index: 2;
  width: min(100%, 34rem);
  padding: 2.5rem 1.5rem;
  text-align: center;
}

.broadcast-loader-core {
  position: relative;
  width: 5.5rem;
  height: 5.5rem;
  margin-inline: auto;
}

.core-ring,
.core-orbit,
.core-center {
  position: absolute;
  inset: 0;
  border-radius: 9999px;
}

.core-ring {
  border: 1px solid rgba(103, 232, 249, 0.22);
}

.core-ring-a {
  inset: 0.55rem;
  border-color: rgba(103, 232, 249, 0.28);
  animation: loader-pulse-ring 2.1s ease-in-out infinite;
}

.core-ring-b {
  inset: 1.1rem;
  border-color: rgba(167, 139, 250, 0.24);
  animation: loader-pulse-ring 2.1s ease-in-out 0.7s infinite;
}

.core-orbit {
  border: 1px dashed rgba(148, 163, 184, 0.2);
  animation: loader-orbit 5.2s linear infinite;
}

.orbit-b {
  inset: 0.35rem;
  animation-duration: 3.8s;
  animation-direction: reverse;
}

.core-orbit i {
  position: absolute;
  left: 50%;
  top: -0.18rem;
  width: 0.42rem;
  height: 0.42rem;
  border-radius: 9999px;
  background: rgb(103 232 249);
  box-shadow: 0 0 14px rgba(103, 232, 249, 0.95);
}

.orbit-b i {
  background: rgb(167 139 250);
  box-shadow: 0 0 14px rgba(167, 139, 250, 0.9);
}

.core-center {
  inset: 1.55rem;
  display: grid;
  place-items: center;
  color: rgb(165 243 252);
  border: 1px solid rgba(103, 232, 249, 0.28);
  background: radial-gradient(circle, rgba(34, 211, 238, 0.18), rgba(15, 23, 42, 0.78));
  box-shadow: 0 0 30px rgba(34, 211, 238, 0.14);
  animation: loader-core-breathe 1.8s ease-in-out infinite;
}

.broadcast-loader-rail {
  position: relative;
  width: min(17rem, 74%);
  height: 0.2rem;
  margin-inline: auto;
  overflow: hidden;
  border-radius: 9999px;
  background: rgba(255, 255, 255, 0.06);
}

.broadcast-loader-rail span {
  position: absolute;
  inset-block: 0;
  width: 36%;
  border-radius: inherit;
  background: linear-gradient(90deg, transparent, rgb(103 232 249), rgb(129 140 248), transparent);
  box-shadow: 0 0 18px rgba(103, 232, 249, 0.38);
  animation: loader-rail 1.7s ease-in-out infinite;
}

.broadcast-loader-steps {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 0.45rem 1rem;
  font-family: ui-monospace, "SFMono-Regular", Consolas, monospace;
  font-size: 0.58rem;
  letter-spacing: 0.08em;
  color: rgb(100 116 139);
  text-transform: uppercase;
}

.broadcast-loader-steps span {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
}

.broadcast-loader-steps i {
  width: 0.28rem;
  height: 0.28rem;
  border-radius: 9999px;
  background: rgb(103 232 249);
  opacity: 0.28;
  animation: loader-step 1.8s ease-in-out infinite;
}

.broadcast-loader-steps span:nth-child(2) i { animation-delay: 0.3s; }
.broadcast-loader-steps span:nth-child(3) i { animation-delay: 0.6s; }

.broadcast-loader-corner {
  position: absolute;
  z-index: 1;
  width: 1.5rem;
  height: 1.5rem;
  opacity: 0.42;
}

.corner-tl { left: 0.75rem; top: 0.75rem; border-left: 1px solid rgb(103 232 249); border-top: 1px solid rgb(103 232 249); }
.corner-tr { right: 0.75rem; top: 0.75rem; border-right: 1px solid rgb(103 232 249); border-top: 1px solid rgb(103 232 249); }
.corner-bl { left: 0.75rem; bottom: 0.75rem; border-left: 1px solid rgb(103 232 249); border-bottom: 1px solid rgb(103 232 249); }
.corner-br { right: 0.75rem; bottom: 0.75rem; border-right: 1px solid rgb(103 232 249); border-bottom: 1px solid rgb(103 232 249); }

@keyframes loader-grid-drift {
  from { background-position: 0 0, 0 0; }
  to { background-position: 0 32px, 32px 0; }
}

@keyframes loader-scan {
  0%, 12% { transform: translateY(-10%); opacity: 0; }
  28% { opacity: 1; }
  88% { opacity: 0.85; }
  100% { transform: translateY(720%); opacity: 0; }
}

@keyframes loader-orbit {
  to { transform: rotate(360deg); }
}

@keyframes loader-pulse-ring {
  0%, 100% { opacity: 0.35; transform: scale(0.96); }
  50% { opacity: 1; transform: scale(1.06); }
}

@keyframes loader-core-breathe {
  0%, 100% { transform: scale(0.94); }
  50% { transform: scale(1.04); }
}

@keyframes loader-rail {
  0% { left: -38%; opacity: 0; }
  16% { opacity: 1; }
  84% { opacity: 1; }
  100% { left: 102%; opacity: 0; }
}

@keyframes loader-step {
  0%, 100% { opacity: 0.2; transform: scale(0.82); }
  45% { opacity: 1; transform: scale(1.2); }
}

@keyframes loader-glow {
  0%, 100% { transform: scale(0.92); opacity: 0.28; }
  50% { transform: scale(1.08); opacity: 0.5; }
}

@media (max-width: 639px) {
  .broadcast-loader-page {
    min-height: 62vh;
  }

  .broadcast-loader-content {
    padding: 2rem 1rem;
  }

  .broadcast-loader-steps {
    gap-inline: 0.7rem;
  }
}

@media (prefers-reduced-motion: reduce) {
  .broadcast-loader-grid,
  .broadcast-loader-scan,
  .broadcast-loader-glow,
  .core-ring,
  .core-orbit,
  .core-center,
  .broadcast-loader-rail span,
  .broadcast-loader-steps i {
    animation: none !important;
  }

  .broadcast-loader-scan {
    display: none;
  }

  .broadcast-loader-rail span {
    left: 32%;
  }

  .broadcast-loader-steps i {
    opacity: 0.75;
  }
}
</style>
