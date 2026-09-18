<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { LinkIcon } from '@heroicons/vue/20/solid'
import { getSpectatorSnapshot } from '../shared/api/spectator-client'
import type { SpectatorRun } from '../shared/api/spectator'
import AppShell from '../shared/components/AppShell.vue'
import SemanticMap from '../shared/components/SemanticMap.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { MAP_CATALOG, mapEntry, resolveMapQuery } from '../shared/mapCatalog'

interface MapWarp {
  x: number
  y: number
  dest: number
}

interface MapPayload {
  id?: number
  width?: number
  height?: number
  cells?: string | string[]
  warps?: MapWarp[]
  connections?: string[]
  fallback?: boolean
}

type WorldView = 'explorer' | 'debug'

const initialParams = new URLSearchParams(window.location.search)
const initialEntry = resolveMapQuery(initialParams.get('map')) || MAP_CATALOG[0]
const selectedMap = ref(initialEntry.id)
const search = ref(initialEntry.name)
const xInput = ref(initialParams.get('x') || '')
const yInput = ref(initialParams.get('y') || '')
const runID = ref(initialParams.get('run') || '')
const followAgent = ref(initialParams.get('follow') === '1')
const viewMode = ref<WorldView>(initialParams.get('debug') === '1' ? 'debug' : 'explorer')
const showWarps = ref(true)
const showSprites = ref(true)
const showTrail = ref(true)
const mapAsset = ref<MapPayload | null>(null)
const mapError = ref('')
const loadingMap = ref(false)
const copyState = ref('')
const mapHistory = ref<number[]>([])
let mapSerial = 0
let initialRunCentered = initialParams.has('map') || !runID.value

const spectator = usePollingResource(
  (signal) => getSpectatorSnapshot(signal),
  { intervalMs: 3000, isEmpty: () => false }
)

const liveRuns = computed(() => (spectator.data.value?.runs || []).filter((run) => run.status !== 'done' && run.status !== 'queued'))
const selectedRun = computed<SpectatorRun | null>(() => {
  if (!runID.value) return null
  return (spectator.data.value?.runs || []).find((run) => run.run_id === runID.value) || null
})
const selectedEntry = computed(() => mapEntry(selectedMap.value))
const mapID = computed(() => selectedMap.value.toString(16).padStart(2, '0').toUpperCase())
const isRunMap = computed(() => Boolean(selectedRun.value) && Number(selectedRun.value?.map) === selectedMap.value)
const target = computed<[number, number] | null>(() => {
  if (xInput.value.trim() === '' || yInput.value.trim() === '') return null
  const x = Number(xInput.value)
  const y = Number(yInput.value)
  if (!Number.isInteger(x) || !Number.isInteger(y) || x < 0 || y < 0) return null
  return [x, y]
})
const overlayTrail = computed<[number, number][]>(() => {
  if (viewMode.value === 'debug' && target.value) return [target.value]
  return isRunMap.value && showTrail.value ? selectedRun.value?.trail || [] : []
})
const overlaySprites = computed(() => isRunMap.value && showSprites.value ? selectedRun.value?.sprites || [] : [])
const filteredMaps = computed(() => {
  const needle = search.value.trim().toLowerCase()
  if (!needle) return MAP_CATALOG.slice(0, 24)
  return MAP_CATALOG.filter((entry) => {
    return entry.name.toLowerCase().includes(needle) || entry.label.toLowerCase().includes(needle) || entry.hex.toLowerCase().includes(needle)
  }).slice(0, 24)
})
const targetCell = computed(() => {
  const point = target.value
  const data = mapAsset.value
  if (!point || !data) return ''
  const [x, y] = point
  const width = Number(data.width || 0)
  const height = Number(data.height || 0)
  if (x < 0 || y < 0 || x >= width || y >= height) return 'outside map'
  let cell = '#'
  if (typeof data.cells === 'string') cell = data.cells[y * width + x] || '#'
  else if (Array.isArray(data.cells)) cell = data.cells[y]?.[x] || '#'
  return cell === '#' ? 'solid' : cell === 'W' ? 'warp' : cell === '~' ? 'water' : cell === 'g' ? 'grass' : 'walkable'
})

watch(selectedMap, () => {
  void loadMap()
  syncURL()
}, { immediate: true })
watch([xInput, yInput, viewMode, runID, followAgent], syncURL)
watch(selectedRun, (run) => {
  if (!run || !Number.isFinite(Number(run.map))) return
  if (followAgent.value || !initialRunCentered) {
    chooseMap(Number(run.map), true)
    initialRunCentered = true
  }
})

async function loadMap(): Promise<void> {
  const serial = ++mapSerial
  loadingMap.value = true
  mapError.value = ''
  try {
    const response = await fetch(`/maps/${selectedMap.value.toString(16).padStart(2, '0')}.json`, { cache: 'no-store' })
    if (!response.ok) throw new Error(`map request failed (${response.status})`)
    const payload = await response.json() as MapPayload
    if (serial !== mapSerial) return
    mapAsset.value = payload
  } catch (cause) {
    if (serial !== mapSerial) return
    mapAsset.value = null
    mapError.value = cause instanceof Error ? cause.message : 'Map unavailable'
  } finally {
    if (serial === mapSerial) loadingMap.value = false
  }
}

function chooseMap(id: number, keepFollowing = false): void {
  if (!keepFollowing) followAgent.value = false
  selectedMap.value = Math.max(0, Math.min(255, Math.trunc(id)))
  search.value = mapEntry(selectedMap.value)?.name || `0x${selectedMap.value.toString(16).padStart(2, '0').toUpperCase()}`
}

function applySearch(): void {
  mapHistory.value = []
  const exact = resolveMapQuery(search.value)
  if (exact) {
    chooseMap(exact.id)
    return
  }
  const first = filteredMaps.value[0]
  if (first) chooseMap(first.id)
}

function followWarp(destination: number): void {
  if (Number(destination) === 0xff) {
    const previous = mapHistory.value.pop()
    if (previous == null) return
    chooseMap(previous)
  } else {
    if (Number(destination) !== selectedMap.value) mapHistory.value.push(selectedMap.value)
    chooseMap(destination)
  }
  xInput.value = ''
  yInput.value = ''
}

function warpDestinationLabel(destination: number): string {
  if (Number(destination) === 0xff) return 'Return to previous map'
  return mapEntry(destination)?.label || ('0x' + Number(destination).toString(16).padStart(2, '0').toUpperCase())
}

function followRun(run: SpectatorRun): void {
  mapHistory.value = []
  runID.value = run.run_id
  followAgent.value = false
  initialRunCentered = true
  chooseMap(Number(run.map || 0), true)
  xInput.value = ''
  yInput.value = ''
}

function jumpToAgent(): void {
  mapHistory.value = []
  const run = selectedRun.value
  if (!run || !Number.isFinite(Number(run.map))) return
  chooseMap(Number(run.map), true)
  xInput.value = ''
  yInput.value = ''
}

function toggleFollowAgent(): void {
  followAgent.value = !followAgent.value
  if (followAgent.value) jumpToAgent()
}

function clearRun(): void {
  runID.value = ''
  followAgent.value = false
}

function clearTarget(): void {
  xInput.value = ''
  yInput.value = ''
}

function syncURL(): void {
  const url = new URL(window.location.href)
  const entry = mapEntry(selectedMap.value)
  url.searchParams.set('map', entry?.name || `0x${mapID.value}`)
  if (xInput.value.trim() !== '') url.searchParams.set('x', xInput.value.trim())
  else url.searchParams.delete('x')
  if (yInput.value.trim() !== '') url.searchParams.set('y', yInput.value.trim())
  else url.searchParams.delete('y')
  if (viewMode.value === 'debug') url.searchParams.set('debug', '1')
  else url.searchParams.delete('debug')
  if (runID.value) url.searchParams.set('run', runID.value)
  else url.searchParams.delete('run')
  if (followAgent.value && runID.value) url.searchParams.set('follow', '1')
  else url.searchParams.delete('follow')
  history.replaceState(null, '', url)
}

async function copyLink(): Promise<void> {
  try {
    await navigator.clipboard.writeText(window.location.href)
    copyState.value = 'Copied'
  } catch {
    copyState.value = 'Copy failed'
  }
  window.setTimeout(() => { copyState.value = '' }, 1400)
}
</script>

<template>
  <AppShell
    eyebrow="Public world data"
    title="World explorer"
    subtitle="Browse Kanto freely, open connected places, or attach a live agent without giving up control of the map."
    mode="public"
  >
    <template #summary>
      <span><strong class="text-white">{{ MAP_CATALOG.length }}</strong> named maps</span>
      <span v-if="liveRuns.length"><strong class="text-white">{{ liveRuns.length }}</strong> live</span>
    </template>

    <template #actions>
      <div class="flex items-center gap-1 rounded-md bg-black/25 p-0.5 ring-1 ring-white/10" aria-label="World explorer mode">
        <button
          type="button"
          :class="[
            viewMode === 'explorer' ? 'bg-emerald-300/15 text-emerald-100 ring-1 ring-emerald-300/20' : 'text-slate-500 hover:text-white',
            'rounded px-2.5 py-1.5 text-[10px] font-semibold'
          ]"
          @click="viewMode = 'explorer'"
        >
          Explorer
        </button>
        <button
          type="button"
          :class="[
            viewMode === 'debug' ? 'bg-cyan-300/15 text-cyan-100 ring-1 ring-cyan-300/20' : 'text-slate-500 hover:text-white',
            'rounded px-2.5 py-1.5 text-[10px] font-semibold'
          ]"
          @click="viewMode = 'debug'"
        >
          Debug
        </button>
      </div>
      <button type="button" class="inline-flex items-center gap-1.5 rounded-md bg-white/8 px-2.5 py-1.5 text-xs font-semibold text-slate-200 ring-1 ring-white/10 hover:bg-white/12" @click="copyLink">
        <LinkIcon class="size-3.5" aria-hidden="true" />
        {{ copyState || 'Share' }}
      </button>
    </template>

    <div v-if="viewMode === 'explorer'" class="mx-auto max-w-[112rem] space-y-3">
      <section class="rounded-xl border border-white/10 bg-[#0d141e] p-3 shadow-sm shadow-black/20">
        <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <div class="min-w-0 flex-1">
            <form class="flex max-w-2xl gap-2" @submit.prevent="applySearch">
              <div class="relative min-w-0 flex-1">
                <input
                  v-model="search"
                  class="w-full rounded-lg border border-white/10 bg-black/25 px-3 py-2.5 text-sm text-white outline-none placeholder:text-slate-600 focus:border-emerald-300/40"
                  placeholder="Search Pallet Town, Route 3, Pokémon Tower…"
                  autocomplete="off"
                />
              </div>
              <button type="submit" class="rounded-lg bg-emerald-300/12 px-4 py-2.5 text-xs font-bold text-emerald-100 ring-1 ring-emerald-300/20 hover:bg-emerald-300/18">
                Open
              </button>
            </form>
            <div class="mt-2 flex max-w-3xl gap-1.5 overflow-x-auto pb-1">
              <button
                v-for="entry in filteredMaps.slice(0, 8)"
                :key="entry.id"
                type="button"
                :class="[
                  entry.id === selectedMap ? 'bg-emerald-300/12 text-emerald-100 ring-emerald-300/25' : 'bg-black/15 text-slate-400 ring-white/8 hover:bg-white/6 hover:text-white',
                  'shrink-0 rounded-full px-2.5 py-1 text-[10px] font-medium ring-1'
                ]"
                @click="chooseMap(entry.id)"
              >
                {{ entry.label }}
              </button>
            </div>
          </div>

          <div v-if="selectedRun" class="flex flex-wrap items-center gap-2 rounded-lg border border-emerald-300/15 bg-emerald-300/5 px-3 py-2">
            <span class="size-2 rounded-full bg-emerald-300 shadow-[0_0_8px_rgba(110,231,183,.55)]" />
            <div class="min-w-0">
              <div class="max-w-72 truncate text-[10px] font-semibold text-slate-200">{{ selectedRun.goal || selectedRun.run_id }}</div>
              <div class="mt-0.5 text-[9px] text-slate-500">
                Agent on {{ mapEntry(Number(selectedRun.map || 0))?.label || 'unknown map' }}
              </div>
            </div>
            <button type="button" class="rounded bg-white/7 px-2 py-1 text-[10px] font-semibold text-slate-300 ring-1 ring-white/10 hover:bg-white/12" @click="jumpToAgent">
              Jump to agent
            </button>
            <button
              type="button"
              :class="[
                followAgent ? 'bg-cyan-300/15 text-cyan-100 ring-cyan-300/25' : 'bg-white/7 text-slate-300 ring-white/10 hover:bg-white/12',
                'rounded px-2 py-1 text-[10px] font-semibold ring-1'
              ]"
              @click="toggleFollowAgent"
            >
              {{ followAgent ? 'Following' : 'Follow agent' }}
            </button>
            <button type="button" class="px-1 text-[10px] font-semibold text-slate-500 hover:text-white" @click="clearRun">Detach</button>
          </div>

          <div v-else-if="liveRuns.length" class="flex flex-wrap items-center gap-1.5">
            <span class="mr-1 text-[9px] font-bold tracking-[0.1em] text-slate-600 uppercase">Live now</span>
            <button
              v-for="run in liveRuns.slice(0, 4)"
              :key="run.run_id"
              type="button"
              class="rounded-full bg-emerald-300/7 px-2.5 py-1 text-[10px] font-semibold text-emerald-100 ring-1 ring-emerald-300/15 hover:bg-emerald-300/12"
              @click="followRun(run)"
            >
              {{ run.goal || 'Agent' }}
            </button>
          </div>
        </div>
      </section>

      <section class="h-[calc(100vh-13rem)] min-h-[42rem] overflow-hidden rounded-2xl border border-white/10 bg-[#07100c] shadow-2xl shadow-black/25">
        <SemanticMap
          :map="selectedMap"
          :x="selectedRun?.x"
          :y="selectedRun?.y"
          :trail="overlayTrail"
          :sprites="overlaySprites"
          :show-player="isRunMap"
          :show-trail="Boolean(overlayTrail.length)"
          :show-sprites="Boolean(overlaySprites.length)"
          :show-warps="showWarps"
          show-pois
          :agent-map="selectedRun?.map"
          :agent-x="selectedRun?.x"
          :agent-y="selectedRun?.y"
          :show-agent-marker="Boolean(selectedRun)"
          appearance="explorer"
          :show-debug-toggle="false"
          interactive
          @warp-select="followWarp"
        />
      </section>
    </div>

    <div v-else class="mx-auto grid max-w-[112rem] gap-3 xl:grid-cols-[20rem_minmax(0,1fr)_22rem]">
      <aside class="space-y-3">
        <section class="rounded-xl border border-white/10 bg-[#0d141e] p-3">
          <div class="text-[10px] font-bold tracking-[0.1em] text-slate-500 uppercase">Open map</div>
          <form class="mt-2 flex gap-1.5" @submit.prevent="applySearch">
            <input v-model="search" class="min-w-0 flex-1 rounded-md border border-white/10 bg-black/25 px-2.5 py-2 font-mono text-xs text-white outline-none focus:border-cyan-300/40" placeholder="ROUTE_13, 0x18, 24…" autocomplete="off" />
            <button type="submit" class="rounded-md bg-cyan-300/10 px-2.5 py-2 text-xs font-bold text-cyan-100 ring-1 ring-cyan-300/20 hover:bg-cyan-300/15">Go</button>
          </form>
          <div class="mt-2 max-h-72 space-y-1 overflow-auto pr-1">
            <button
              v-for="entry in filteredMaps"
              :key="entry.id"
              type="button"
              :class="[
                entry.id === selectedMap ? 'bg-cyan-300/10 text-cyan-100 ring-cyan-300/25' : 'bg-black/15 text-slate-400 ring-white/7 hover:bg-white/5 hover:text-white',
                'flex w-full items-center justify-between gap-2 rounded px-2 py-1.5 text-left text-[10px] ring-1'
              ]"
              @click="chooseMap(entry.id)"
            >
              <span class="truncate">{{ entry.label }}</span>
              <span class="shrink-0 font-mono text-slate-600">0x{{ entry.hex }}</span>
            </button>
          </div>
        </section>

        <section class="rounded-xl border border-white/10 bg-[#0d141e] p-3">
          <div class="flex items-center justify-between gap-3">
            <div class="text-[10px] font-bold tracking-[0.1em] text-slate-500 uppercase">Target coordinate</div>
            <button v-if="target" type="button" class="text-[10px] font-semibold text-slate-500 hover:text-white" @click="clearTarget">Clear</button>
          </div>
          <div class="mt-2 grid grid-cols-2 gap-2">
            <label class="text-[9px] font-semibold text-slate-600 uppercase">X
              <input v-model="xInput" inputmode="numeric" class="mt-1 w-full rounded-md border border-white/10 bg-black/25 px-2 py-1.5 font-mono text-xs text-white outline-none focus:border-cyan-300/40" placeholder="49" />
            </label>
            <label class="text-[9px] font-semibold text-slate-600 uppercase">Y
              <input v-model="yInput" inputmode="numeric" class="mt-1 w-full rounded-md border border-white/10 bg-black/25 px-2 py-1.5 font-mono text-xs text-white outline-none focus:border-cyan-300/40" placeholder="8" />
            </label>
          </div>
          <div v-if="target" class="mt-2 rounded bg-black/20 px-2 py-1.5 font-mono text-[10px] text-slate-400 ring-1 ring-white/7">
            @ {{ target[0] }},{{ target[1] }} · {{ targetCell || 'loading' }}
          </div>
          <p class="mt-2 text-[10px] leading-4 text-slate-600">Debug coordinates remain shareable and never appear in normal Explorer mode.</p>
        </section>

        <section v-if="liveRuns.length" class="rounded-xl border border-white/10 bg-[#0d141e] p-3">
          <div class="flex items-center justify-between gap-3">
            <div class="text-[10px] font-bold tracking-[0.1em] text-slate-500 uppercase">Live overlay</div>
            <button v-if="runID" type="button" class="text-[10px] font-semibold text-slate-500 hover:text-white" @click="clearRun">Detach</button>
          </div>
          <div class="mt-2 space-y-1">
            <button v-for="run in liveRuns" :key="run.run_id" type="button" class="w-full rounded bg-black/15 px-2 py-2 text-left ring-1 ring-white/7 hover:bg-white/5" @click="followRun(run)">
              <div class="flex items-center justify-between gap-2">
                <span class="truncate text-[10px] font-semibold text-slate-300">{{ run.goal || run.run_id }}</span>
                <span v-if="run.run_id === runID" class="size-1.5 shrink-0 rounded-full bg-emerald-300" />
              </div>
              <div class="mt-0.5 font-mono text-[9px] text-slate-600">map 0x{{ Number(run.map || 0).toString(16).padStart(2, '0').toUpperCase() }} @ {{ run.x }},{{ run.y }}</div>
            </button>
          </div>
          <div v-if="selectedRun" class="mt-2 flex gap-1.5">
            <button type="button" class="rounded bg-white/7 px-2 py-1 text-[9px] text-slate-300 ring-1 ring-white/10" @click="jumpToAgent">Jump</button>
            <button type="button" :class="[followAgent ? 'bg-cyan-300/15 text-cyan-100 ring-cyan-300/20' : 'bg-white/7 text-slate-300 ring-white/10', 'rounded px-2 py-1 text-[9px] ring-1']" @click="toggleFollowAgent">
              {{ followAgent ? 'Following' : 'Follow' }}
            </button>
          </div>
        </section>
      </aside>

      <section class="min-h-[68vh] overflow-hidden rounded-xl border border-white/10 bg-[#080d14]">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-white/10 bg-[#0d141e] px-3 py-2.5">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <strong class="truncate text-sm text-white">{{ selectedEntry?.label || ('Map 0x' + mapID) }}</strong>
              <span class="rounded bg-black/30 px-1.5 py-0.5 font-mono text-[9px] text-slate-400 ring-1 ring-white/10">0x{{ mapID }}</span>
              <span v-if="isRunMap" class="rounded bg-emerald-300/10 px-1.5 py-0.5 text-[9px] font-bold text-emerald-200 ring-1 ring-emerald-300/20">LIVE RUN</span>
            </div>
            <p class="mt-1 truncate font-mono text-[10px] text-slate-600">{{ selectedEntry?.name || 'unnamed map id' }}</p>
          </div>
        </div>

        <div class="h-[calc(68vh-3rem)] min-h-[34rem]">
          <SemanticMap
            :map="selectedMap"
            :x="selectedRun?.x"
            :y="selectedRun?.y"
            :trail="overlayTrail"
            :sprites="overlaySprites"
            :show-player="isRunMap"
            :show-trail="Boolean(overlayTrail.length)"
            :show-sprites="Boolean(overlaySprites.length)"
            :show-warps="showWarps"
            :show-pois="false"
            :agent-map="selectedRun?.map"
            :agent-x="selectedRun?.x"
            :agent-y="selectedRun?.y"
            :show-agent-marker="Boolean(selectedRun)"
            debug
            appearance="semantic"
            :show-debug-toggle="false"
            interactive
            @warp-select="followWarp"
          />
        </div>
      </section>

      <aside class="space-y-3">
        <section class="rounded-xl border border-white/10 bg-[#0d141e] p-3">
          <div class="text-[10px] font-bold tracking-[0.1em] text-slate-500 uppercase">Map data</div>
          <div v-if="loadingMap" class="mt-3 text-xs text-slate-500">Loading map data…</div>
          <div v-else-if="mapError" class="mt-3 text-xs text-amber-300">{{ mapError }}</div>
          <template v-else-if="mapAsset">
            <div v-if="mapAsset.fallback" class="mt-2 rounded-md border border-amber-300/20 bg-amber-300/8 px-2.5 py-2 text-[10px] leading-4 text-amber-100">
              No exported ROM geometry exists for this ID. Do not use the fallback grid to diagnose collision.
            </div>
            <dl class="mt-3 grid grid-cols-2 gap-2 text-xs">
              <div class="rounded bg-black/20 px-2.5 py-2 ring-1 ring-white/7"><dt class="text-[9px] text-slate-600 uppercase">Size</dt><dd class="mt-1 font-mono text-slate-300">{{ mapAsset.width }}×{{ mapAsset.height }}</dd></div>
              <div class="rounded bg-black/20 px-2.5 py-2 ring-1 ring-white/7"><dt class="text-[9px] text-slate-600 uppercase">Warps</dt><dd class="mt-1 font-mono text-slate-300">{{ mapAsset.warps?.length || 0 }}</dd></div>
            </dl>
            <div class="mt-3">
              <div class="text-[9px] font-semibold text-slate-600 uppercase">Connections</div>
              <div v-if="mapAsset.connections?.length" class="mt-1.5 flex flex-wrap gap-1">
                <span v-for="connection in mapAsset.connections" :key="connection" class="rounded bg-white/5 px-1.5 py-1 text-[9px] text-slate-400 ring-1 ring-white/8">{{ connection }}</span>
              </div>
              <p v-else class="mt-1 text-[10px] text-slate-600">No edge connections in the exported header.</p>
            </div>
          </template>
        </section>

        <section class="rounded-xl border border-white/10 bg-[#0d141e] p-3">
          <div class="flex items-center justify-between gap-3">
            <div class="text-[10px] font-bold tracking-[0.1em] text-slate-500 uppercase">Warps</div>
            <label class="inline-flex items-center gap-1 text-[9px] text-slate-500"><input v-model="showWarps" type="checkbox" class="size-3 accent-purple-300" /> show</label>
          </div>
          <div v-if="mapAsset?.warps?.length" class="mt-2 max-h-80 space-y-1 overflow-auto pr-1">
            <button v-for="(warp, index) in mapAsset.warps" :key="warp.x + '-' + warp.y + '-' + index" type="button" class="flex w-full items-center justify-between gap-2 rounded bg-black/15 px-2 py-1.5 text-left ring-1 ring-white/7 hover:bg-white/5" @click="followWarp(warp.dest)">
              <span class="font-mono text-[10px] text-slate-400">{{ warp.x }},{{ warp.y }}</span>
              <span class="min-w-0 truncate text-right text-[10px] text-purple-200">{{ Number(warp.dest) === 0xff ? '↩' : '→' }} {{ warpDestinationLabel(warp.dest) }}</span>
            </button>
          </div>
          <p v-else class="mt-2 text-[10px] text-slate-600">No warps exported for this map.</p>
        </section>

        <section class="rounded-xl border border-white/10 bg-[#0d141e] p-3 text-[10px] leading-5 text-slate-500">
          <strong class="text-slate-300">Developer view</strong>
          <p class="mt-1">Exact semantic collision, map IDs, target coordinates and raw topology live here. Explorer intentionally hides them.</p>
        </section>
      </aside>
    </div>
  </AppShell>
</template>