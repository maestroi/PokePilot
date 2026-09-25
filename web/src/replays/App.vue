<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { getSpectatorSemanticReplay, getSpectatorSnapshot, spectatorReplayVideoURL } from '../shared/api/spectator-client'
import type { RenderState } from '../shared/api/renderstate'
import type { SpectatorRun, SpectatorSnapshot } from '../shared/api/spectator'
import ModernSceneRenderer from '../shared/components/ModernSceneRenderer.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { canRenderModernScene } from '../shared/semanticRenderer'
import {
  semanticReplaySampleAtMS,
  semanticReplayStateAtMS,
  type SemanticReplayTimeline
} from '../shared/semanticReplay'
import { DEFAULT_RENDER_THEME_ID, renderThemeOptions, resolveRenderTheme } from '../shared/renderTheme'
import {
  formatReplayDuration,
  replayResult,
  replayRuns,
  replayRuntimeSeconds,
  replaySearchText,
  sortReplays,
  type ReplaySort
} from './model'

const query = ref('')
const modelFilter = ref('')
const styleFilter = ref('')
const resultFilter = ref('')
const sort = ref<ReplaySort>('recent')
const selectedRunID = ref(runIDFromPath())
const copyState = ref('')
const playbackRate = ref(1)
const videoRef = ref<HTMLVideoElement | null>(null)
type RendererMode = 'modern' | 'classic'
type SemanticReplayStatus = 'idle' | 'loading' | 'ready' | 'error'

const rendererMode = ref<RendererMode>(window.localStorage.getItem('pokepilot.spectator.renderer') === 'classic' ? 'classic' : 'modern')
const themeOptions = renderThemeOptions()
const storedThemeID = window.localStorage.getItem('pokepilot.spectator.theme') || DEFAULT_RENDER_THEME_ID
const initialTheme = resolveRenderTheme(storedThemeID)
const selectedThemeID = ref(initialTheme.theme.id)
const themeNotice = ref(initialTheme.diagnostics.join(' '))
const activeTheme = computed(() => resolveRenderTheme(selectedThemeID.value).theme)

const semanticTimeline = shallowRef<SemanticReplayTimeline | null>(null)
const semanticState = shallowRef<RenderState | null>(null)
const semanticStatus = ref<SemanticReplayStatus>('idle')
const semanticError = ref('')
const currentTime = ref(0)
const videoDuration = ref(0)
const playing = ref(false)
let semanticAbort: AbortController | null = null
let playbackRAF = 0
let lastSemanticSampleAt = -1

const { data: snapshot, state, error, retry } = usePollingResource<SpectatorSnapshot>(
  (signal) => getSpectatorSnapshot(signal),
  { intervalMs: 10000, isEmpty: (value) => replayRuns(value.runs).length === 0 }
)

const allReplays = computed(() => replayRuns(snapshot.value?.runs ?? []))
const models = computed(() => uniqueValues(allReplays.value.map((run) => run.llm_profile)))
const styles = computed(() => uniqueValues(allReplays.value.map((run) => run.play_style)))
const results = computed(() => uniqueValues(allReplays.value.map((run) => replayResult(run))))

const filteredReplays = computed(() => {
  const needle = query.value.trim().toLowerCase()
  const filtered = allReplays.value.filter((run) => {
    if (modelFilter.value && run.llm_profile !== modelFilter.value) return false
    if (styleFilter.value && run.play_style !== styleFilter.value) return false
    if (resultFilter.value && replayResult(run) !== resultFilter.value) return false
    if (needle && !replaySearchText(run).includes(needle)) return false
    return true
  })
  return sortReplays(filtered, sort.value)
})

const selectedRun = computed(() => {
  if (selectedRunID.value) {
    const selected = allReplays.value.find((run) => run.run_id === selectedRunID.value)
    if (selected) return selected
  }
  return filteredReplays.value[0] || allReplays.value[0] || null
})

const selectedVideoURL = computed(() => selectedRun.value ? spectatorReplayVideoURL(selectedRun.value.run_id) : '')
const showModern = computed(() =>
  rendererMode.value === 'modern' &&
  semanticStatus.value === 'ready' &&
  semanticState.value !== null &&
  canRenderModernScene(semanticState.value)
)
const modernFallbackLabel = computed(() => {
  if (rendererMode.value !== 'modern' || showModern.value) return ''
  if (semanticStatus.value === 'loading') return 'Loading modern replay · classic fallback'
  if (semanticStatus.value === 'error') return 'Modern replay unavailable · classic fallback'
  if (semanticState.value) return `Modern unsupported for ${semanticState.value.scene} · classic fallback`
  return ''
})

watch(selectedRun, (run) => {
  if (!run || selectedRunID.value) return
  selectedRunID.value = run.run_id
}, { immediate: true })

watch(
  () => selectedRun.value?.run_id || '',
  (runID) => { void loadSemanticReplay(runID) },
  { immediate: true }
)

watch(playbackRate, (rate) => {
  if (videoRef.value) videoRef.value.playbackRate = rate
})

function uniqueValues(values: Array<string | undefined>): string[] {
  return [...new Set(values.map((value) => value?.trim()).filter((value): value is string => Boolean(value)))].sort()
}

function runIDFromPath(): string {
  const prefix = '/replays/'
  if (!window.location.pathname.startsWith(prefix)) return ''
  const raw = window.location.pathname.slice(prefix.length).split('/')[0]
  try { return decodeURIComponent(raw || '') } catch { return '' }
}

function selectReplay(run: SpectatorRun): void {
  selectedRunID.value = run.run_id
  history.pushState(null, '', `/replays/${encodeURIComponent(run.run_id)}`)
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

function clearSelection(): void {
  selectedRunID.value = ''
  history.pushState(null, '', '/replays')
}

function syncPath(): void {
  selectedRunID.value = runIDFromPath()
}

function formatDate(value?: number): string {
  if (!value) return 'Date unavailable'
  return new Date(value * 1000).toLocaleString([], {
    year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
  })
}

function badgeCount(run: SpectatorRun): number {
  return run.player?.badges?.length || 0
}

function dexLabel(run: SpectatorRun): string {
  const owned = Number(run.player?.dex_owned || 0)
  const total = Number(run.player?.dex_total || 0)
  return total > 0 ? `${owned}/${total}` : owned ? String(owned) : '—'
}

function replayTitle(run: SpectatorRun): string {
  return run.stats?.goal_summary || run.goal || `${run.starter || 'Pokémon Red'} run`
}

function resetFilters(): void {
  query.value = ''
  modelFilter.value = ''
  styleFilter.value = ''
  resultFilter.value = ''
  sort.value = 'recent'
}

function setRendererMode(mode: RendererMode): void {
  rendererMode.value = mode
  window.localStorage.setItem('pokepilot.spectator.renderer', mode)
  syncPlaybackState()
}

function setTheme(themeID: string): void {
  const resolved = resolveRenderTheme(themeID)
  selectedThemeID.value = resolved.theme.id
  themeNotice.value = resolved.diagnostics.join(' ')
  rendererMode.value = 'modern'
  window.localStorage.setItem('pokepilot.spectator.renderer', 'modern')
  window.localStorage.setItem('pokepilot.spectator.theme', resolved.theme.id)
}

function onThemeSelect(event: Event): void {
  const target = event.target as HTMLSelectElement | null
  if (target) setTheme(target.value)
}

async function loadSemanticReplay(runID: string): Promise<void> {
  semanticAbort?.abort()
  semanticAbort = null
  semanticTimeline.value = null
  semanticState.value = null
  semanticError.value = ''
  semanticStatus.value = runID ? 'loading' : 'idle'
  lastSemanticSampleAt = -1
  if (!runID) return

  const controller = new AbortController()
  semanticAbort = controller
  try {
    const timeline = await getSpectatorSemanticReplay(runID, controller.signal)
    if (controller.signal.aborted || selectedRun.value?.run_id !== runID) return
    semanticTimeline.value = timeline
    semanticStatus.value = 'ready'
    syncPlaybackState(true)
  } catch (cause) {
    if (controller.signal.aborted) return
    semanticStatus.value = 'error'
    semanticError.value = cause instanceof Error ? cause.message : 'Semantic replay unavailable'
  } finally {
    if (semanticAbort === controller) semanticAbort = null
  }
}

function syncPlaybackState(force = false): void {
  const video = videoRef.value
  if (video) {
    currentTime.value = Number.isFinite(video.currentTime) ? video.currentTime : 0
    if (Number.isFinite(video.duration) && video.duration > 0) videoDuration.value = video.duration
  }
  const timeline = semanticTimeline.value
  const atMS = Math.max(0, currentTime.value * 1000)
  const sample = semanticReplaySampleAtMS(timeline, atMS)
  if (!sample) {
    semanticState.value = null
    lastSemanticSampleAt = -1
    return
  }
  if (!force && sample.at_ms === lastSemanticSampleAt) return
  lastSemanticSampleAt = sample.at_ms
  semanticState.value = semanticReplayStateAtMS(timeline, atMS)
}

function stopPlaybackClock(): void {
  if (playbackRAF) cancelAnimationFrame(playbackRAF)
  playbackRAF = 0
}

function playbackTick(): void {
  syncPlaybackState()
  const video = videoRef.value
  if (video && !video.paused && !video.ended) {
    playbackRAF = requestAnimationFrame(playbackTick)
  } else {
    playbackRAF = 0
  }
}

function startPlaybackClock(): void {
  stopPlaybackClock()
  playbackRAF = requestAnimationFrame(playbackTick)
}

function onCanPlay(): void {
  if (videoRef.value) videoRef.value.playbackRate = playbackRate.value
  syncPlaybackState(true)
}

function onLoadedMetadata(): void {
  if (videoRef.value && Number.isFinite(videoRef.value.duration)) videoDuration.value = videoRef.value.duration
  syncPlaybackState(true)
}

function onPlay(): void {
  playing.value = true
  startPlaybackClock()
}

function onPause(): void {
  playing.value = false
  stopPlaybackClock()
  syncPlaybackState(true)
}

function onEnded(): void {
  playing.value = false
  stopPlaybackClock()
  syncPlaybackState(true)
}

function onTimeUpdate(): void {
  syncPlaybackState()
}

function togglePlayback(): void {
  const video = videoRef.value
  if (!video) return
  if (video.paused) void video.play()
  else video.pause()
}

function seekPlayback(event: Event): void {
  const target = event.target as HTMLInputElement | null
  const video = videoRef.value
  if (!target || !video) return
  const next = Number(target.value)
  if (!Number.isFinite(next)) return
  video.currentTime = next
  currentTime.value = next
  syncPlaybackState(true)
}

function formatPlaybackTime(seconds: number): string {
  const safe = Math.max(0, Number.isFinite(seconds) ? seconds : 0)
  const total = Math.floor(safe)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const secs = total % 60
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, '0')}:${String(secs).padStart(2, '0')}`
    : `${minutes}:${String(secs).padStart(2, '0')}`
}

async function copyReplayLink(): Promise<void> {
  const run = selectedRun.value
  if (!run) return
  const url = new URL(`/replays/${encodeURIComponent(run.run_id)}`, window.location.origin)
  try {
    await navigator.clipboard.writeText(url.toString())
    copyState.value = 'Copied'
  } catch {
    copyState.value = 'Copy failed'
  }
  window.setTimeout(() => { copyState.value = '' }, 1400)
}

onMounted(() => window.addEventListener('popstate', syncPath))
onBeforeUnmount(() => {
  window.removeEventListener('popstate', syncPath)
  semanticAbort?.abort()
  stopPlaybackClock()
})
</script>

<template>
  <main class="replay-shell">
    <header class="topbar">
      <div>
        <p class="eyebrow">RomPilot public archive</p>
        <h1>Replay library</h1>
        <p class="lede">Watch curated finished runs without digging through the operator console.</p>
      </div>
      <nav class="nav-actions" aria-label="Replay navigation">
        <a class="button secondary" href="/">Back to live</a>
        <button class="button secondary" type="button" @click="retry">Refresh</button>
      </nav>
    </header>

    <section v-if="selectedRun" class="viewer-card">
      <div class="viewer-copy">
        <div class="viewer-heading">
          <div>
            <div class="chips">
              <span class="chip accent">{{ replayResult(selectedRun) }}</span>
              <span v-if="selectedRun.llm_profile" class="chip">{{ selectedRun.llm_profile }}</span>
              <span v-if="selectedRun.play_style" class="chip">{{ selectedRun.play_style }}</span>
            </div>
            <h2>{{ replayTitle(selectedRun) }}</h2>
            <p class="run-id">{{ selectedRun.run_id }}</p>
          </div>
          <button class="button secondary" type="button" @click="copyReplayLink">{{ copyState || 'Copy link' }}</button>
        </div>

        <div class="player-wrap">
          <video
            ref="videoRef"
            :key="selectedRun.run_id"
            :class="['replay-player', { 'semantic-video-clock': showModern }]"
            :src="selectedVideoURL"
            :controls="!showModern"
            playsinline
            preload="metadata"
            @canplay="onCanPlay"
            @loadedmetadata="onLoadedMetadata"
            @timeupdate="onTimeUpdate"
            @seeked="() => syncPlaybackState(true)"
            @play="onPlay"
            @pause="onPause"
            @ended="onEnded"
          />
          <ModernSceneRenderer
            v-if="showModern && semanticState"
            :state="semanticState"
            :theme="activeTheme"
          />

          <div class="renderer-controls" aria-label="Replay renderer controls">
            <div class="renderer-switch">
              <button
                type="button"
                :class="{ active: rendererMode === 'modern' }"
                @click="setRendererMode('modern')"
              >Modern</button>
              <button
                type="button"
                :class="{ active: rendererMode === 'classic' }"
                @click="setRendererMode('classic')"
              >Classic</button>
            </div>
            <label v-if="rendererMode === 'modern'" class="theme-control">Theme
              <select :value="selectedThemeID" @change="onThemeSelect">
                <option v-for="theme in themeOptions" :key="theme.id" :value="theme.id">{{ theme.name }}</option>
              </select>
            </label>
          </div>

          <div v-if="showModern" class="semantic-transport">
            <button type="button" @click="togglePlayback">{{ playing ? 'Pause' : 'Play' }}</button>
            <input
              type="range"
              min="0"
              :max="Math.max(videoDuration, currentTime, 0.01)"
              step="0.05"
              :value="currentTime"
              aria-label="Replay position"
              @input="seekPlayback"
            >
            <span>{{ formatPlaybackTime(currentTime) }} / {{ formatPlaybackTime(videoDuration) }}</span>
            <label>Speed
              <select v-model.number="playbackRate">
                <option :value="1">1×</option>
                <option :value="2">2×</option>
                <option :value="4">4×</option>
                <option :value="8">8×</option>
                <option :value="16">16×</option>
              </select>
            </label>
          </div>

          <label v-if="!showModern" class="speed-control">Speed
            <select v-model.number="playbackRate">
              <option :value="1">1×</option>
              <option :value="2">2×</option>
              <option :value="4">4×</option>
              <option :value="8">8×</option>
              <option :value="16">16×</option>
            </select>
          </label>
          <div v-if="modernFallbackLabel" class="renderer-notice">{{ modernFallbackLabel }}</div>
          <div v-if="themeNotice && rendererMode === 'modern'" class="theme-notice">{{ themeNotice }}</div>
        </div>
      </div>

      <aside class="viewer-stats">
        <div><span>Runtime</span><strong>{{ formatReplayDuration(replayRuntimeSeconds(selectedRun)) }}</strong></div>
        <div><span>Finished</span><strong>{{ formatDate(selectedRun.ended_at) }}</strong></div>
        <div><span>Badges</span><strong>{{ badgeCount(selectedRun) }}</strong></div>
        <div><span>Pokédex</span><strong>{{ dexLabel(selectedRun) }}</strong></div>
        <div><span>Starter</span><strong>{{ selectedRun.starter || '—' }}</strong></div>
        <div><span>Model calls</span><strong>{{ selectedRun.stats?.calls || 0 }}</strong></div>
      </aside>
    </section>

    <section class="library-card">
      <div class="library-head">
        <div>
          <p class="eyebrow">Past runs</p>
          <h2>Browse replays</h2>
          <p>{{ filteredReplays.length }} of {{ allReplays.length }} public replays</p>
        </div>
        <button v-if="selectedRunID" class="text-button" type="button" @click="clearSelection">Clear selected replay</button>
      </div>

      <div class="filters">
        <label class="search-field">Search
          <input v-model="query" type="search" placeholder="Run, goal, Pokémon, model…">
        </label>
        <label>Model
          <select v-model="modelFilter">
            <option value="">All models</option>
            <option v-for="model in models" :key="model" :value="model">{{ model }}</option>
          </select>
        </label>
        <label>Mode
          <select v-model="styleFilter">
            <option value="">All modes</option>
            <option v-for="style in styles" :key="style" :value="style">{{ style }}</option>
          </select>
        </label>
        <label>Result
          <select v-model="resultFilter">
            <option value="">All results</option>
            <option v-for="result in results" :key="result" :value="result">{{ result }}</option>
          </select>
        </label>
        <label>Sort
          <select v-model="sort">
            <option value="recent">Newest</option>
            <option value="runtime">Fastest runtime</option>
            <option value="badges">Most badges</option>
          </select>
        </label>
        <button class="button secondary reset" type="button" @click="resetFilters">Reset</button>
      </div>

      <div v-if="state === 'loading' && !snapshot" class="empty-state">Loading public replays…</div>
      <div v-else-if="error && !snapshot" class="empty-state error-state">
        Replay library is temporarily unavailable.<br><small>{{ error }}</small>
      </div>
      <div v-else-if="filteredReplays.length === 0" class="empty-state">
        No public replays match these filters.
      </div>
      <div v-else class="replay-grid">
        <button
          v-for="run in filteredReplays"
          :key="run.run_id"
          class="replay-card"
          :class="{ selected: selectedRun?.run_id === run.run_id }"
          type="button"
          @click="selectReplay(run)"
        >
          <div class="card-top">
            <span class="result-pill">{{ replayResult(run) }}</span>
            <span>{{ formatDate(run.ended_at) }}</span>
          </div>
          <strong class="card-title">{{ replayTitle(run) }}</strong>
          <span class="card-run">{{ run.run_id }}</span>
          <div class="card-tags">
            <span v-if="run.llm_profile">{{ run.llm_profile }}</span>
            <span v-if="run.play_style">{{ run.play_style }}</span>
            <span v-if="run.starter">{{ run.starter }}</span>
          </div>
          <div class="card-metrics">
            <div><b>{{ formatReplayDuration(replayRuntimeSeconds(run)) }}</b><small>runtime</small></div>
            <div><b>{{ badgeCount(run) }}</b><small>badges</small></div>
            <div><b>{{ dexLabel(run) }}</b><small>dex</small></div>
          </div>
        </button>
      </div>
    </section>
  </main>
</template>

<style scoped>
.replay-shell { width: min(1480px, 100%); margin: 0 auto; padding: 28px; color: #f6f8ff; }
.topbar, .library-head, .viewer-heading, .nav-actions, .chips, .card-top, .card-tags { display: flex; align-items: center; gap: 10px; }
.topbar { justify-content: space-between; align-items: flex-start; margin-bottom: 20px; }
.eyebrow { margin: 0 0 5px; color: #94a4c4; font-size: 11px; font-weight: 850; letter-spacing: .14em; text-transform: uppercase; }
h1 { margin: 0; font-size: clamp(30px, 5vw, 54px); line-height: .98; letter-spacing: -.04em; }
h2 { margin: 0; font-size: clamp(20px, 3vw, 30px); letter-spacing: -.025em; }
.lede, .library-head p { margin: 8px 0 0; color: #9eacc9; }
.button, .text-button { border: 1px solid rgba(255,255,255,.12); background: rgba(255,255,255,.05); color: inherit; border-radius: 999px; padding: 9px 13px; font: inherit; font-weight: 750; cursor: pointer; text-decoration: none; }
.button:hover, .text-button:hover { border-color: rgba(255,255,255,.28); background: rgba(255,255,255,.08); }
.text-button { border: 0; background: transparent; color: #aab8d3; padding-inline: 0; }
.viewer-card, .library-card { border: 1px solid rgba(255,255,255,.09); border-radius: 24px; background: rgba(15,23,42,.88); box-shadow: 0 24px 80px rgba(0,0,0,.24); }
.viewer-card { display: grid; grid-template-columns: minmax(0, 1fr) 240px; gap: 16px; padding: 18px; margin-bottom: 18px; }
.viewer-heading { justify-content: space-between; align-items: flex-start; margin-bottom: 14px; }
.viewer-heading h2 { margin-top: 8px; }
.run-id, .card-run { color: #8797b8; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; overflow-wrap: anywhere; }
.chip, .result-pill, .card-tags span { display: inline-flex; border: 1px solid rgba(255,255,255,.1); border-radius: 999px; padding: 4px 8px; background: rgba(255,255,255,.035); color: #afbbd4; font-size: 11px; font-weight: 800; text-transform: uppercase; letter-spacing: .06em; }
.chip.accent, .result-pill { color: #ffd84a; border-color: rgba(255,216,74,.3); background: rgba(255,216,74,.07); }
.player-wrap { position: relative; min-height: 320px; border: 1px solid rgba(255,255,255,.08); border-radius: 18px; overflow: hidden; background: #04070d; display: grid; place-items: center; }
.replay-player { width: 100%; max-height: 650px; aspect-ratio: 160 / 144; object-fit: contain; background: #04070d; }
.semantic-video-clock { position: absolute; inset: 0; width: 100%; height: 100%; opacity: 0; pointer-events: none; }
.speed-control { position: absolute; top: 12px; right: 12px; display: flex; align-items: center; gap: 6px; padding: 6px 9px; border: 1px solid rgba(255,255,255,.12); border-radius: 999px; background: rgba(4,7,13,.86); color: #aab8d3; font-size: 11px; font-weight: 750; }
.speed-control select, .theme-control select, .semantic-transport select { border: 0; background: #111a2d; color: white; border-radius: 6px; padding: 2px 4px; }
.renderer-controls { position: absolute; z-index: 5; top: 12px; left: 12px; display: flex; align-items: center; gap: 8px; }
.renderer-switch { display: flex; overflow: hidden; border: 1px solid rgba(255,255,255,.12); border-radius: 999px; background: rgba(4,7,13,.86); }
.renderer-switch button { border: 0; background: transparent; color: #8999b8; padding: 6px 9px; font: inherit; font-size: 10px; font-weight: 850; text-transform: uppercase; letter-spacing: .06em; cursor: pointer; }
.renderer-switch button.active { background: rgba(103,232,249,.15); color: #cffafe; }
.theme-control { display: flex; align-items: center; gap: 5px; padding: 5px 8px; border: 1px solid rgba(255,255,255,.12); border-radius: 999px; background: rgba(4,7,13,.86); color: #aab8d3; font-size: 10px; font-weight: 800; }
.semantic-transport { position: absolute; z-index: 6; right: 12px; bottom: 12px; left: 12px; display: grid; grid-template-columns: auto minmax(80px,1fr) auto auto; align-items: center; gap: 9px; padding: 8px 10px; border: 1px solid rgba(255,255,255,.12); border-radius: 12px; background: rgba(4,7,13,.88); color: #b7c3dc; font-size: 11px; backdrop-filter: blur(8px); }
.semantic-transport button { border: 0; border-radius: 8px; background: rgba(255,255,255,.1); color: white; padding: 5px 9px; font: inherit; font-weight: 800; cursor: pointer; }
.semantic-transport input[type="range"] { width: 100%; }
.semantic-transport label { display: flex; align-items: center; gap: 5px; font-weight: 800; }
.renderer-notice, .theme-notice { position: absolute; z-index: 7; right: 12px; max-width: min(320px,70%); padding: 5px 8px; border-radius: 8px; background: rgba(4,7,13,.88); font-size: 10px; }
.renderer-notice { bottom: 12px; color: #ffd67a; }
.theme-notice { top: 52px; color: #ffd67a; }
.viewer-stats { display: grid; align-content: start; gap: 8px; }
.viewer-stats div { padding: 12px; border: 1px solid rgba(255,255,255,.08); border-radius: 14px; background: rgba(255,255,255,.025); }
.viewer-stats span, .card-metrics small { display: block; color: #8393b4; font-size: 10px; font-weight: 800; text-transform: uppercase; letter-spacing: .08em; }
.viewer-stats strong { display: block; margin-top: 4px; overflow-wrap: anywhere; }
.library-card { padding: 18px; }
.library-head { justify-content: space-between; align-items: flex-start; margin-bottom: 14px; }
.filters { display: grid; grid-template-columns: minmax(220px, 1.5fr) repeat(4, minmax(140px, .6fr)) auto; gap: 9px; align-items: end; margin-bottom: 16px; }
.filters label { display: grid; gap: 5px; color: #91a1c0; font-size: 11px; font-weight: 800; text-transform: uppercase; letter-spacing: .07em; }
.filters input, .filters select { width: 100%; border: 1px solid rgba(255,255,255,.1); border-radius: 11px; background: rgba(5,9,18,.72); color: #f5f7ff; padding: 10px 11px; font: inherit; text-transform: none; letter-spacing: normal; }
.reset { border-radius: 11px; padding-block: 10px; }
.replay-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 11px; }
.replay-card { display: block; width: 100%; min-width: 0; border: 1px solid rgba(255,255,255,.09); border-radius: 18px; background: rgba(255,255,255,.025); color: inherit; text-align: left; padding: 14px; cursor: pointer; transition: transform .15s ease, border-color .15s ease, background .15s ease; }
.replay-card:hover { transform: translateY(-2px); border-color: rgba(255,255,255,.22); background: rgba(255,255,255,.045); }
.replay-card.selected { border-color: rgba(255,216,74,.5); background: rgba(255,216,74,.06); }
.card-top { justify-content: space-between; color: #7f8eab; font-size: 10px; }
.card-title { display: block; margin: 12px 0 4px; font-size: 16px; line-height: 1.25; }
.card-run { display: block; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.card-tags { flex-wrap: wrap; margin: 12px 0; }
.card-tags span { text-transform: none; letter-spacing: 0; font-weight: 700; }
.card-metrics { display: grid; grid-template-columns: repeat(3, 1fr); gap: 7px; }
.card-metrics div { padding: 8px; border-radius: 11px; background: rgba(0,0,0,.14); }
.card-metrics b { display: block; font-size: 14px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.empty-state { padding: 46px 18px; text-align: center; color: #94a4c4; border: 1px dashed rgba(255,255,255,.1); border-radius: 16px; }
.error-state { color: #ff9ca4; }
@media (max-width: 1050px) { .viewer-card { grid-template-columns: 1fr; } .viewer-stats { grid-template-columns: repeat(3, 1fr); } .filters { grid-template-columns: repeat(3, 1fr); } .search-field { grid-column: 1 / -1; } .replay-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 680px) { .replay-shell { padding: 14px; } .topbar, .viewer-heading, .library-head { flex-direction: column; } .nav-actions { width: 100%; } .viewer-stats, .filters, .replay-grid { grid-template-columns: 1fr; } .search-field { grid-column: auto; } .player-wrap { min-height: 240px; } }
</style>
