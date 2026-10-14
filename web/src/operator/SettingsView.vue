<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowPathIcon } from '@heroicons/vue/20/solid'
import { getBroadcastControl } from '../shared/api/broadcast'
import Panel from '../shared/components/Panel.vue'
import ResourceState from '../shared/components/ResourceState.vue'
import StatusBadge from '../shared/components/StatusBadge.vue'
import { usePollingResource } from '../shared/composables/usePollingResource'
import { operatorHref } from '../shared/urls'
import { DEFAULT_RENDER_THEME_ID, publicRenderThemeOptions, renderThemeOptions, resolveRenderTheme } from '../shared/renderTheme'
import LLMDeploymentsPanel from './LLMDeploymentsPanel.vue'
import { readRunLaunchDefaults, saveRunLaunchDefaults, type RunLaunchDefaults } from './runLaunchDefaults'

type SettingsSection = 'models' | 'defaults' | 'destinations' | 'renderer'
const props = defineProps<{ section?: string }>()
const sections: { id: SettingsSection; label: string }[] = [
  { id: 'models', label: 'Models' },
  { id: 'defaults', label: 'Run defaults' },
  { id: 'destinations', label: 'Destinations' },
  { id: 'renderer', label: 'Renderer & themes' }
]
const activeSection = computed<SettingsSection>(() => sections.some((item) => item.id === props.section) ? props.section as SettingsSection : 'models')
const defaults = ref<RunLaunchDefaults>(readRunLaunchDefaults())
const defaultsStatus = ref('')
const rendererMode = ref<'modern' | 'classic'>(window.localStorage.getItem('pokepilot.operator.renderer') === 'classic' ? 'classic' : 'modern')
const currentTheme = window.localStorage.getItem('pokepilot.operator.theme') || DEFAULT_RENDER_THEME_ID
const themeID = ref(resolveRenderTheme(currentTheme).theme.id)
const rendererStatus = ref('')
const allThemes = renderThemeOptions()
const publicThemes = publicRenderThemeOptions()
const localOnlyThemes = computed(() => allThemes.filter((theme) => !publicThemes.some((candidate) => candidate.id === theme.id)))
const destinations = usePollingResource(
  (signal) => getBroadcastControl(signal),
  { intervalMs: 30000, isEmpty: (config) => config.destinations.length === 0 }
)

function operatorSearch(): string { return window.location.search }
function saveDefaults(): void {
  defaultsStatus.value = ''
  try {
    saveRunLaunchDefaults(defaults.value)
    defaultsStatus.value = 'Saved for new runs launched in this browser. Existing runs are unchanged.'
  } catch (cause) {
    defaultsStatus.value = cause instanceof Error ? 'Could not save: ' + cause.message : 'Could not save run defaults.'
  }
}
function saveRenderer(): void {
  rendererStatus.value = ''
  try {
    const resolved = resolveRenderTheme(themeID.value)
    window.localStorage.setItem('pokepilot.operator.renderer', rendererMode.value)
    window.localStorage.setItem('pokepilot.operator.theme', resolved.theme.id)
    themeID.value = resolved.theme.id
    rendererStatus.value = 'Saved for this browser. The next Live view uses these render preferences.'
  } catch (cause) {
    rendererStatus.value = cause instanceof Error ? 'Could not save: ' + cause.message : 'Could not save renderer preferences.'
  }
}
function refreshDestinations(): void { void destinations.retry() }
</script>

<template>
  <div class="space-y-3">
    <nav class="flex flex-wrap gap-1 border-b border-[var(--poke-border)] pb-2" aria-label="Settings sections">
      <a
        v-for="item in sections"
        :key="item.id"
        :href="operatorHref('settings', { section: item.id }, operatorSearch())"
        :aria-current="activeSection === item.id ? 'page' : undefined"
        :class="[
          activeSection === item.id ? 'bg-white/10 text-white ring-1 ring-white/10' : 'text-[var(--poke-muted)] hover:bg-white/5 hover:text-white',
          'rounded-sm px-3 py-2 text-xs font-semibold focus-visible:outline-2 focus-visible:outline-cyan-300'
        ]"
      >{{ item.label }}</a>
    </nav>

    <LLMDeploymentsPanel v-if="activeSection === 'models'" />

    <Panel v-else-if="activeSection === 'defaults'" title="New run defaults" description="Initial values for the existing New run form. Each run can override these before launch." compact>
      <form class="space-y-3" @submit.prevent="saveDefaults">
        <p class="text-xs text-slate-400">These are browser-local preferences, not server-side worker policy. They do not change queued or running work.</p>
        <div class="grid gap-3 sm:grid-cols-3">
          <label class="block">
            <span class="text-[11px] font-semibold text-slate-300">Play style</span>
            <select v-model="defaults.playStyle" class="mt-1 block w-full rounded-sm border border-white/10 bg-[#0e1520] px-2.5 py-2 text-xs text-white">
              <option value="adventure">Adventure</option><option value="speedrun">Speedrun</option><option value="completionist">Completionist</option><option value="team_builder">Team Builder</option>
            </select>
          </label>
          <label class="block">
            <span class="text-[11px] font-semibold text-slate-300">Play speed</span>
            <select v-model.number="defaults.fps" class="mt-1 block w-full rounded-sm border border-white/10 bg-[#0e1520] px-2.5 py-2 text-xs text-white">
              <option :value="60">1×</option><option :value="120">2×</option><option :value="240">4×</option><option :value="480">8×</option><option :value="0">Max · uncapped</option>
            </select>
          </label>
          <label class="block">
            <span class="text-[11px] font-semibold text-slate-300">Recovery</span>
            <select v-model="defaults.recoveryProfile" class="mt-1 block w-full rounded-sm border border-white/10 bg-[#0e1520] px-2.5 py-2 text-xs text-white">
              <option value="resilient">Resilient</option><option value="strict">Strict</option>
            </select>
          </label>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <button type="submit" class="rounded-sm bg-[var(--poke-cyan)] px-3 py-2 text-xs font-bold text-[#101820] hover:brightness-110 focus-visible:outline-2 focus-visible:outline-white">Save defaults</button>
          <a href="#new-run" class="text-xs text-cyan-200 hover:underline">Open New run</a>
        </div>
        <p v-if="defaultsStatus" class="text-xs text-slate-300" role="status">{{ defaultsStatus }}</p>
      </form>
    </Panel>

    <Panel v-else-if="activeSection === 'destinations'" title="Broadcast destinations" description="Read-only availability from the replay host. Broadcast start/stop lives in Media." compact>
      <ResourceState :state="destinations.state.value" title="Destinations unavailable" :message="destinations.error.value || 'No destinations reported by the replay host.'" :rows="4">
        <template #actions>
          <button type="button" class="rounded-sm bg-white/10 px-3 py-1.5 text-xs text-white" @click="refreshDestinations">Retry</button>
        </template>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <p class="text-xs text-slate-400">Destination credentials and stream keys are configured on the server; this API does not allow editing them in the browser.</p>
          <button type="button" class="inline-flex items-center gap-1 rounded-sm px-2.5 py-1.5 text-xs text-white ring-1 ring-white/10 hover:bg-white/10 focus-visible:outline-2 focus-visible:outline-cyan-300" @click="refreshDestinations"><ArrowPathIcon class="size-3.5" aria-hidden="true" />Refresh</button>
        </div>
        <ul v-if="destinations.data.value?.destinations?.length" class="mt-3 divide-y divide-white/10">
          <li v-for="destination in destinations.data.value.destinations" :key="destination.provider" class="flex flex-wrap items-start justify-between gap-3 py-3 first:pt-0 last:pb-0">
            <div class="min-w-0">
              <strong class="text-sm text-white">{{ destination.label }}</strong>
              <span class="ml-2 font-mono text-[10px] text-slate-500">{{ destination.provider }}</span>
              <p v-if="destination.host" class="mt-1 break-all font-mono text-[11px] text-slate-500">{{ destination.host }}</p>
              <p v-if="destination.reason" class="mt-1 text-xs text-amber-200">{{ destination.reason }}</p>
            </div>
            <StatusBadge :tone="destination.configured ? 'success' : 'warning'">{{ destination.configured ? 'Configured' : 'Unavailable' }}</StatusBadge>
          </li>
        </ul>
        <a href="#media/broadcasts" class="mt-4 inline-block text-xs font-semibold text-cyan-200 hover:underline">Open Media / Broadcasts →</a>
      </ResourceState>
    </Panel>

    <Panel v-else title="Renderer & theme preferences" description="Select your private Live renderer and inspect installed themes. Public distribution is independently gated." compact>
      <form class="grid gap-3 sm:grid-cols-2" @submit.prevent="saveRenderer">
        <label class="block">
          <span class="text-[11px] font-semibold text-slate-300">Live renderer</span>
          <select v-model="rendererMode" class="mt-1 block w-full rounded-sm border border-white/10 bg-[#0e1520] px-2.5 py-2 text-xs text-white">
            <option value="modern">Modern semantic renderer</option><option value="classic">Classic frame</option>
          </select>
        </label>
        <label class="block">
          <span class="text-[11px] font-semibold text-slate-300">Modern theme</span>
          <select v-model="themeID" class="mt-1 block w-full rounded-sm border border-white/10 bg-[#0e1520] px-2.5 py-2 text-xs text-white">
            <option v-for="theme in allThemes" :key="theme.id" :value="theme.id">{{ theme.name }} · v{{ theme.version }}</option>
          </select>
        </label>
        <div class="sm:col-span-2">
          <button type="submit" class="rounded-sm bg-[var(--poke-cyan)] px-3 py-2 text-xs font-bold text-[#101820] hover:brightness-110 focus-visible:outline-2 focus-visible:outline-white">Save renderer</button>
          <p v-if="rendererStatus" class="mt-2 text-xs text-slate-300" role="status">{{ rendererStatus }}</p>
        </div>
      </form>
      <div class="mt-4 grid gap-3 border-t border-white/8 pt-3 lg:grid-cols-2">
        <section class="rounded-sm border border-white/8 bg-black/10 p-3">
          <h3 class="text-xs font-semibold text-white">Public-safe · {{ publicThemes.length }}</h3>
          <ul class="mt-2 divide-y divide-white/8">
            <li v-for="theme in publicThemes" :key="theme.id" class="py-2 text-xs text-slate-300">
              <strong>{{ theme.name }}</strong><span class="ml-2 font-mono text-[10px] text-slate-500">v{{ theme.version }}</span>
              <p class="mt-1 text-[11px] text-slate-500">{{ theme.description || theme.id }}</p>
            </li>
          </ul>
        </section>
        <section class="rounded-sm border border-white/8 bg-black/10 p-3">
          <h3 class="text-xs font-semibold text-white">Operator only · {{ localOnlyThemes.length }}</h3>
          <ul v-if="localOnlyThemes.length" class="mt-2 divide-y divide-white/8">
            <li v-for="theme in localOnlyThemes" :key="theme.id" class="py-2 text-xs text-slate-300">
              <strong>{{ theme.name }}</strong><span class="ml-2 font-mono text-[10px] text-slate-500">v{{ theme.version }}</span>
              <p class="mt-1 text-[11px] text-slate-500">{{ theme.description || theme.id }}</p>
            </li>
          </ul>
          <p v-else class="mt-2 text-xs text-slate-500">All installed themes are public-safe.</p>
        </section>
      </div>
    </Panel>
  </div>
</template>
