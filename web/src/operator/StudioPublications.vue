<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import {
  actStudioPublication, assessStudioRun, createEditedStudioPackage,
  createStudioEditPlan, createStudioPublication, getStudioPackage,
  getStudioPublications, getStudioThumbnails, renderStudioEdit
} from '../shared/api/client'
import type {
  StudioPackage, StudioPackageSummary, StudioPublicationCandidate,
  StudioPublicationFormat, StudioPublicationState
} from '../shared/api/types'
import { middleTruncate } from './runs'

const props = defineProps<{ runID: string; packages: StudioPackageSummary[] }>()
const emit = defineEmits<{ changed: [] }>()
const candidates = ref<StudioPublicationCandidate[]>([])
const pkg = ref<StudioPackage | null>(null)
const selectedThumbnail = ref('')
const packageHash = ref('')
const format = ref<StudioPublicationFormat>('episode')
const titleID = ref('')
const privacy = ref<'private'|'unlisted'|'public'>('private')
const notBefore = ref('')
const busy = ref(false)
const error = ref('')
const message = ref('')
let generation = 0

const titleOptions = computed(() => format.value === 'short' ? (pkg.value?.shorts || []) : (pkg.value?.titles || []))
const currentPackageReady = computed(() => Boolean(pkg.value?.plan_hash))
function friendly(errorValue:unknown):string {
  return errorValue instanceof Error ? errorValue.message : 'Publication service unavailable'
}
async function refresh():Promise<void> {
  if(!props.runID)return
  const run=props.runID
  try {
    const data=await getStudioPublications(run)
    if(run===props.runID)candidates.value=data.candidates
  } catch(cause) {
    if(run===props.runID)error.value=friendly(cause)
  }
}
watch(() => props.runID, () => {
  generation++
  pkg.value=null;packageHash.value='';candidates.value=[];selectedThumbnail.value=''
  titleID.value='';error.value='';message.value=''
  void refresh()
},{immediate:true})
watch(format,() => {
  titleID.value=titleOptions.value[0]?.id || ''
})
const interval=window.setInterval(() => {if(!busy.value)void refresh()},10000)
onUnmounted(() => window.clearInterval(interval))

async function openPackage(hash:string):Promise<void> {
  if(!props.runID||busy.value)return
  const run=props.runID,id=++generation
  busy.value=true;error.value='';message.value=''
  try {
    const data=await getStudioPackage(run,hash)
    if(id!==generation)return
    pkg.value=data;packageHash.value=hash
    titleID.value=(format.value==='short' ? data.shorts : data.titles)?.[0]?.id||''
    selectedThumbnail.value=''
    try {
      const images=await getStudioThumbnails(run,hash)
      if(id!==generation)return
      selectedThumbnail.value=images.selected_id||''
    }catch{ /* thumbnail review is optional for Shorts; mandatory for episodes */ }
  }catch(cause){if(id===generation)error.value=friendly(cause)}
  finally{if(id===generation)busy.value=false}
}
async function prepare():Promise<void> {
  if(!props.runID||busy.value)return
  const run=props.runID,id=++generation
  busy.value=true;error.value='';message.value=''
  try {
    const assessed=await assessStudioRun(run)
    if(id!==generation)return
    const publicationClass=assessed.assessment.publication
    if(publicationClass==='archive_only'||publicationClass==='do_not_publish'){
      await createStudioPublication(run,{assessment_hash:assessed.assessment.hash,format:'archive'})
      message.value='Run is archive-only / do-not-publish. The decision is recorded; no upload will occur.'
      await refresh()
      return
    }
    if(format.value==='episode'&&publicationClass==='short_only'){
      error.value='The producer approved only a Short. Select Short and prepare again.'
      return
    }
    const profile=format.value==='short'?'short':'episode'
    const plan=await createStudioEditPlan(run,assessed.assessment.hash,profile)
    if(id!==generation)return
    let renderHint=''
    try {
      const st=await renderStudioEdit(run,plan.plan.hash)
      renderHint=st.state==='ready'?'The edited video is ready.':'Editing is queued. You can return and approve once rendering finishes.'
    }catch{
      renderHint='The edited video is not ready. Render the source broadcast replay first and retry the edit rendering.'
    }
    const created=await createEditedStudioPackage(run,assessed.assessment.hash,plan.plan.hash)
    if(id!==generation)return
    emit('changed')
    pkg.value=created.package
    packageHash.value=created.package.hash
    titleID.value=(format.value==='short'?created.package.shorts:created.package.titles)?.[0]?.id||''
    selectedThumbnail.value=''
    message.value=`A verified ${profile} package is prepared. ${renderHint}`
  }catch(cause){if(id===generation)error.value=friendly(cause)}
  finally{if(id===generation)busy.value=false}
}
async function queue():Promise<void> {
  if(!props.runID||!pkg.value||busy.value)return
  const run=props.runID
  busy.value=true;error.value='';message.value=''
  try {
    const notBeforeUnix=notBefore.value ? Math.floor(new Date(notBefore.value).getTime()/1000) : 0
    const queued=await createStudioPublication(run,{
      assessment_hash:pkg.value.assessment_hash,
      package_hash:pkg.value.hash,
      format:format.value,
      title_id:titleID.value,
      thumbnail_id:selectedThumbnail.value||undefined,
      privacy:privacy.value,
      not_before_unix:Number.isFinite(notBeforeUnix)?notBeforeUnix:0
    })
    message.value=queued.state==='queued'?'Candidate awaits explicit approval.':
      `Candidate state: ${queued.state}. ${queued.reason||'Check prerequisites and retry approval when ready.'}`
    await refresh()
  }catch(cause){error.value=friendly(cause)}
  finally{busy.value=false}
}
async function act(candidate:StudioPublicationCandidate,action:'approve'|'reject'|'publish'|'retry-thumbnail'):Promise<void> {
  if(!props.runID||busy.value)return
  busy.value=true;error.value='';message.value=''
  try {
    const result=await actStudioPublication(props.runID,candidate.id,action)
    message.value=action==='retry-thumbnail'?'Thumbnail attached without uploading the video again.':
      action==='publish'?'Upload started. This uses the existing resumable YouTube publisher.':
      action==='approve'?'Candidate approved. Publishing still requires a separate action.':
        'Candidate skipped. This does not alter the original run.'
    if(result.reason)message.value+=` ${result.reason}`
    await refresh()
  }catch(cause){error.value=friendly(cause)}
  finally{busy.value=false}
}
function canApprove(c:StudioPublicationCandidate):boolean{
 return !c.approved&&['queued','blocked','rendering','failed'].includes(c.state)
}
function canReject(c:StudioPublicationCandidate):boolean{
 return c.state!=='publishing'&&c.state!=='published'&&c.state!=='skipped'
}
function canPublish(c:StudioPublicationCandidate):boolean{
 return c.approved&&(c.state==='approved'||c.state==='failed')
}
function stateLabel(state:StudioPublicationState):string{
 return ({
   queued:'Awaiting approval',blocked:'Quality gate blocked',rendering:'Awaiting edited video',
   approved:'Approved · not uploaded',publishing:'Uploading',published:'Published',
   failed:'Upload failed',skipped:'Not publishing'
 } as Record<StudioPublicationState,string>)[state]||state
}
</script>

<template>
  <section class="ui-bay min-w-0 xl:col-span-2" aria-labelledby="studio-publications-title">
    <div class="ui-bay-header">
      <h2 id="studio-publications-title">Publication queue</h2>
      <span class="text-meta text-fg-2">Human approval required · YouTube</span>
    </div>
    <div class="space-y-4 p-3">
      <p class="max-w-[76ch] text-fg-2">
        Only editorially approved runs can publish. This queue checks event evidence, duplicate story value,
        an edited MP4, selected title and episode thumbnail. Nothing uploads until you approve and separately publish.
      </p>
      <template v-if="runID">
        <div class="flex flex-wrap items-end gap-3">
          <label class="flex flex-col gap-1 text-meta" for="publication-format">
            Output format
            <select id="publication-format" v-model="format" class="ui-input h-10" :disabled="busy">
              <option value="episode">Full episode · landscape</option>
              <option value="short">Short / clip · portrait</option>
            </select>
          </label>
          <button class="ui-btn h-10" type="button" :disabled="busy" @click="prepare">
            {{ busy ? 'Working…' : 'Prepare edited output + package' }}
          </button>
        </div>
        <p class="text-meta text-fg-2">
          Preparing an edit uses the existing recorded replay and producer assessment. If the broadcast MP4
          is missing, render it in Media Library first. A Short needs its own portrait edit and content package.
        </p>
        <div v-if="packages.length" class="flex flex-wrap items-center gap-2">
          <span class="text-meta text-fg-2">Saved packages:</span>
          <button v-for="entry in packages" :key="entry.hash" type="button" class="ui-btn ui-btn-sm"
            :class="entry.hash===packageHash?'ui-btn-primary':''" :disabled="busy"
            @click="openPackage(entry.hash)">{{ middleTruncate(entry.hash,8,5) }} {{ entry.thumbnail_selected?'· thumbnail selected':'' }}</button>
        </div>
        <p v-if="error" class="border border-danger/40 bg-danger/10 px-3 py-2 text-danger" role="alert">{{ error }}</p>
        <p v-if="message" class="text-meta text-fg-2" role="status">{{ message }}</p>
        <div v-if="pkg" class="space-y-3 border-t border-rule pt-3">
          <p class="text-meta text-fg-2">Source package: <span class="font-mono">{{ middleTruncate(pkg.hash,12,8) }}</span>
            · {{ currentPackageReady?'Edited plan attached':'No edit plan: prepare an edited package before queuing.' }}</p>
          <div class="flex flex-wrap items-end gap-3">
            <label class="flex min-w-60 flex-1 flex-col gap-1 text-meta" for="studio-pub-title">
              Verified {{ format==='short'?'Short':'episode' }} title
              <select id="studio-pub-title" v-model="titleID" class="ui-input h-10 min-w-0 w-full" :disabled="busy">
                <option v-for="title in titleOptions" :key="title.id" :value="title.id">{{ title.title }}</option>
              </select>
            </label>
            <label class="flex flex-col gap-1 text-meta" for="studio-pub-privacy">
              YouTube privacy
              <select id="studio-pub-privacy" v-model="privacy" class="ui-input h-10" :disabled="busy">
                <option value="private">Private</option><option value="unlisted">Unlisted</option><option value="public">Public</option>
              </select>
            </label>
            <label class="flex flex-col gap-1 text-meta" for="studio-pub-schedule">
              Not before (optional)
              <input id="studio-pub-schedule" v-model="notBefore" type="datetime-local" class="ui-input h-10" :disabled="busy" />
            </label>
          </div>
          <p class="text-meta text-fg-2">
            Thumbnail: <strong>{{ selectedThumbnail ? 'selected in Thumbnail studio' : 'not selected' }}</strong>.
            {{ format==='episode'?'An episode cannot pass quality gates without a rendered, selected thumbnail.':'Shorts may proceed without one.' }}
            <button type="button" class="ui-btn ui-btn-sm ml-2" :disabled="busy" @click="openPackage(pkg.hash)">Refresh assets</button>
          </p>
          <button class="ui-btn ui-btn-primary h-10" type="button"
            :disabled="busy||!currentPackageReady||!titleID" @click="queue">Queue for review</button>
        </div>
        <div class="space-y-2 border-t border-rule pt-3">
          <div class="flex items-center justify-between gap-2">
            <h3 class="font-semibold">Publication candidates</h3>
            <button type="button" class="ui-btn ui-btn-sm" :disabled="busy" @click="refresh()">Refresh status</button>
          </div>
          <p v-if="!candidates.length" class="text-meta text-fg-2">No queued publications. Archive-only runs need no upload.</p>
          <article v-for="candidate in candidates" :key="candidate.id" class="space-y-2 border border-rule bg-surface p-3">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-semibold capitalize">{{ candidate.proposal.format }}</span>
              <span class="ui-pill" :class="candidate.state==='published'?'ui-tone-ok':'ui-tone-neutral'">{{ stateLabel(candidate.state) }}</span>
              <span class="font-mono text-meta text-fg-3" :title="candidate.id">{{ middleTruncate(candidate.id,8,5) }}</span>
              <span class="text-meta text-fg-2">{{ candidate.proposal.privacy }}</span>
              <span v-if="candidate.proposal.not_before_unix" class="text-meta text-fg-2">
                After {{ new Date(candidate.proposal.not_before_unix*1000).toLocaleString() }}
              </span>
            </div>
            <p v-if="candidate.reason" class="text-meta text-fg-2" role="status">{{ candidate.reason }}</p>
            <p v-if="candidate.thumbnail_status==='failed'" class="text-meta text-warn">Video uploaded; thumbnail transfer failed. You can retry the thumbnail without reuploading the video.</p>
            <div class="flex flex-wrap gap-2">
              <button v-if="canApprove(candidate)" type="button" class="ui-btn ui-btn-sm" :disabled="busy" @click="act(candidate,'approve')">Approve after gates</button>
              <button v-if="canPublish(candidate)" type="button" class="ui-btn ui-btn-sm ui-btn-primary" :disabled="busy" @click="act(candidate,'publish')">
                {{ candidate.state==='failed'?'Retry resumable upload':'Publish to YouTube' }}
              </button>
              <button v-if="candidate.state==='published' && candidate.thumbnail_status==='failed'" type="button" class="ui-btn ui-btn-sm" :disabled="busy" @click="act(candidate,'retry-thumbnail')">Retry thumbnail only</button>
              <button v-if="canReject(candidate)" type="button" class="ui-btn ui-btn-sm" :disabled="busy" @click="act(candidate,'reject')">Skip</button>
              <a v-if="candidate.provider_url" :href="candidate.provider_url" target="_blank" rel="noopener noreferrer"
                class="ui-btn ui-btn-sm">Open published video</a>
            </div>
          </article>
        </div>
      </template>
      <p v-else class="text-fg-2">Select a completed run above to review publications.</p>
    </div>
  </section>
</template>
