import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import {
  actStudioPublication, assessStudioRun, createEditedStudioPackage, createStudioEditPlan,
  createStudioPublication, getStudioAssessment, getStudioEditPlan, getStudioEditStatus,
  getStudioPackage, getStudioPublications, getStudioThumbnails, renderStudioEdit,
  renderStudioThumbnails, selectStudioThumbnail
} from '../shared/api/client'
import type {
  StudioAssessment, StudioEditStatus, StudioPackage, StudioPackageSummary,
  StudioPublicationCandidate, StudioThumbnailList
} from '../shared/api/types'

export type StudioFormat = 'episode' | 'short'
export type PublicationAction = 'approve' | 'reject' | 'publish' | 'retry-thumbnail'

// One run's publishing state, shared by every Studio section so they all work
// on the same package: the producer verdict, the open package with its edit
// and thumbnails, and the run's publication candidates. Opens the newest stored
// package on its own; assessing is explicit because it may call a model.
export function useStudio(runID: Ref<string>, packages: Ref<StudioPackageSummary[] | null>, onPackagesChanged: () => void) {
  const assessment = ref<StudioAssessment | null>(null)
  const pkg = ref<StudioPackage | null>(null)
  const profile = ref<StudioFormat>('episode')
  const edit = ref<StudioEditStatus | null>(null)
  const thumbnails = ref<StudioThumbnailList | null>(null)
  const candidates = ref<StudioPublicationCandidate[]>([])
  const busy = ref(false)
  const error = ref('')
  const message = ref('')
  // Which section's action the error/message belongs to, so it shows beside the button pressed.
  const feedbackAt = ref<'pipeline' | 'publication'>('pipeline')
  let generation = 0

  const verdict = computed(() => assessment.value?.publication_class || '')
  const selectedThumbnail = computed(() => thumbnails.value?.selected_id || '')
  // Newest candidate made from the open package (or the run's archive decision).
  const publication = computed(() => [...candidates.value]
    .filter((c) => pkg.value ? c.proposal.package_hash === pkg.value.hash : c.proposal.format === 'archive')
    .sort((a, b) => Number(b.updated_at_unix || 0) - Number(a.updated_at_unix || 0))[0] || null)

  function friendly(cause: unknown): string {
    return cause instanceof Error ? cause.message : 'Studio service unavailable'
  }

  // Runs one action at a time. A run switch bumps `generation`, so a late
  // reply never lands on the next run.
  async function guarded(task: (stale: () => boolean) => Promise<void>, at: 'pipeline' | 'publication' = 'pipeline'): Promise<void> {
    if (!runID.value || busy.value) return
    const id = ++generation
    const stale = () => id !== generation
    busy.value = true
    feedbackAt.value = at
    error.value = ''
    message.value = ''
    try {
      await task(stale)
    } catch (cause) {
      if (!stale()) error.value = friendly(cause)
    } finally {
      if (!stale()) busy.value = false
    }
  }

  async function loadAssessment(hash: string): Promise<void> {
    const run = runID.value
    try {
      const next = await getStudioAssessment(run, hash)
      if (run === runID.value) assessment.value = next
    } catch { /* the verdict step then offers to assess again */ }
  }

  async function refreshEdit(): Promise<void> {
    const run = runID.value
    const plan = pkg.value?.plan_hash
    if (!plan) return
    try {
      const next = await getStudioEditStatus(run, plan)
      if (run === runID.value && plan === pkg.value?.plan_hash) edit.value = next
    } catch (cause) {
      if (run === runID.value) edit.value = { state: 'error', plan_hash: plan, error: friendly(cause) }
    }
  }

  async function refreshCandidates(): Promise<void> {
    const run = runID.value
    if (!run) return
    try {
      const data = await getStudioPublications(run)
      if (run !== runID.value) return
      candidates.value = data.candidates || []
      // No package to read the verdict from: an archive decision still has one.
      if (!assessment.value && !pkg.value && candidates.value[0]) void loadAssessment(candidates.value[0].proposal.assessment_hash)
    } catch { /* the publication section shows the queue as empty */ }
  }

  async function show(run: string, next: StudioPackage, stale: () => boolean): Promise<void> {
    pkg.value = next
    edit.value = null
    thumbnails.value = null
    const [plan, images] = await Promise.allSettled([
      next.plan_hash ? getStudioEditPlan(run, next.plan_hash) : Promise.reject(new Error('no plan')),
      getStudioThumbnails(run, next.hash)
    ])
    if (stale()) return
    if (plan.status === 'fulfilled') profile.value = plan.value.profile
    if (images.status === 'fulfilled') thumbnails.value = images.value
    if (assessment.value?.hash !== next.assessment_hash) await loadAssessment(next.assessment_hash)
    await refreshEdit()
  }

  function open(hash: string): Promise<void> {
    return guarded(async (stale) => {
      const run = runID.value
      const next = await getStudioPackage(run, hash)
      if (!stale()) await show(run, next, stale)
    })
  }

  function assess(): Promise<void> {
    return guarded(async (stale) => {
      const result = await assessStudioRun(runID.value)
      if (!stale()) assessment.value = result.assessment
    })
  }

  // Assess (unless a verdict is loaded), then cut an edit plan in the format
  // the verdict allows, render it from the replay video and package it.
  function prepare(format: StudioFormat): Promise<void> {
    return guarded(async (stale) => {
      const run = runID.value
      const current = assessment.value || (await assessStudioRun(run)).assessment
      if (stale()) return
      assessment.value = current
      if (current.publication_class === 'archive_only' || current.publication_class === 'do_not_publish') {
        await createStudioPublication(run, { assessment_hash: current.hash, format: 'archive' })
        message.value = 'The producer kept this run off YouTube. The decision is recorded; nothing will upload.'
        await refreshCandidates()
        return
      }
      const chosen: StudioFormat = current.publication_class === 'short_only' ? 'short' : format
      const plan = await createStudioEditPlan(run, current.hash, chosen)
      if (stale()) return
      let hint = ''
      try {
        const status = await renderStudioEdit(run, plan.plan.hash)
        hint = status.state === 'ready' ? 'The edited video is ready.' : 'The edited video is rendering.'
      } catch {
        hint = 'The edited video can’t render until the replay video is rendered.'
      }
      const created = await createEditedStudioPackage(run, current.hash, plan.plan.hash)
      if (stale()) return
      profile.value = chosen
      await show(run, created.package, stale)
      onPackagesChanged()
      message.value = `${chosen === 'short' ? 'Short' : 'Episode'} package prepared. ${hint}`
    })
  }

  function retryEdit(): Promise<void> {
    return guarded(async () => {
      const plan = pkg.value?.plan_hash
      if (!plan) return
      edit.value = await renderStudioEdit(runID.value, plan)
    })
  }

  function renderThumbnails(variants: number, generatedArt: boolean): Promise<void> {
    return guarded(async (stale) => {
      if (!pkg.value) return
      const result = await renderStudioThumbnails(runID.value, pkg.value.hash, variants, generatedArt)
      if (stale()) return
      thumbnails.value = result
      const fallbacks = result.manifest.candidates.filter((c) => c.artwork_status === 'fallback' || c.artwork_status === 'unconfigured')
      message.value = fallbacks.length
        ? 'Thumbnails ready. Generated artwork was unavailable for some concepts; those use the built-in illustration.'
        : `${result.manifest.candidates.length} thumbnails ready to review.`
      onPackagesChanged()
    })
  }

  function pickThumbnail(candidateID: string): Promise<void> {
    return guarded(async (stale) => {
      if (!pkg.value) return
      await selectStudioThumbnail(runID.value, pkg.value.hash, candidateID)
      const next = await getStudioThumbnails(runID.value, pkg.value.hash)
      if (stale()) return
      thumbnails.value = next
      onPackagesChanged()
    })
  }

  // A queued candidate's choices are fixed (its id is derived from them), so
  // changing them means skipping the unapproved one and queueing anew.
  function queue(proposal: { titleID: string; privacy: 'private' | 'unlisted' | 'public'; notBeforeUnix: number }, replace?: StudioPublicationCandidate): Promise<void> {
    return guarded(async () => {
      if (!pkg.value) return
      if (replace) await actStudioPublication(runID.value, replace.id, 'reject')
      const queued = await createStudioPublication(runID.value, {
        assessment_hash: pkg.value.assessment_hash,
        package_hash: pkg.value.hash,
        format: profile.value,
        title_id: proposal.titleID,
        thumbnail_id: selectedThumbnail.value || undefined,
        privacy: proposal.privacy,
        not_before_unix: proposal.notBeforeUnix
      })
      message.value = queued.state === 'queued' ? 'Queued. It waits for your approval.' : `Queued as “${queued.state}”. ${queued.reason || ''}`.trim()
      await refreshCandidates()
    }, 'publication')
  }

  function act(candidate: StudioPublicationCandidate, action: PublicationAction): Promise<void> {
    return guarded(async () => {
      const result = await actStudioPublication(runID.value, candidate.id, action)
      message.value = ({
        approve: 'Approved. Publishing is still a separate step.',
        publish: 'Upload started.',
        reject: 'Skipped. The run itself is unchanged.',
        'retry-thumbnail': 'Thumbnail attached without uploading the video again.'
      } as Record<PublicationAction, string>)[action] + (result.reason ? ` ${result.reason}` : '')
      await refreshCandidates()
    }, 'publication')
  }

  watch(runID, () => {
    generation++
    busy.value = false
    assessment.value = null
    pkg.value = null
    edit.value = null
    thumbnails.value = null
    candidates.value = []
    error.value = ''
    message.value = ''
    void refreshCandidates()
  }, { immediate: true })
  // Open the newest stored package once the run's list arrives.
  watch(packages, (list) => {
    if (!pkg.value && !busy.value && list?.[0]) void open(list[0].hash)
  }, { immediate: true })
  // Edits render and uploads run in the background; follow them.
  const timer = window.setInterval(() => {
    if (busy.value || !runID.value) return
    void refreshCandidates()
    if (edit.value?.state === 'generating') void refreshEdit()
  }, 10000)
  onBeforeUnmount(() => window.clearInterval(timer))

  return {
    assessment, verdict, pkg, profile, edit, thumbnails, selectedThumbnail, candidates, publication,
    busy, error, message, feedbackAt,
    assess, prepare, open, retryEdit, renderThumbnails, pickThumbnail, queue, act, refreshCandidates
  }
}

export type Studio = ReturnType<typeof useStudio>
