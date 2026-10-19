import type {
  DashboardRun, RunSpec, ReplayStatus, StudioAssessment, StudioEditStatus, StudioPackage,
  StudioPublicationCandidate, StudioThumbnailList, YouTubePublishStatus
} from '../shared/api/types'

// One browser-driven E2E coordinator. All expensive work executes on existing
// farm/replay workers; switching tabs stops this coordinator, not those jobs.
// Stage receipts are evidence from real APIs, never optimistic confirmations.
export type FullE2EStageState = 'working' | 'ready' | 'failed'
export interface FullE2EStage {
  id: string
  label: string
  state: FullE2EStageState
  detail?: string
}
export interface FullE2EOutput {
  format: 'episode' | 'short'
  planHash: string
  packageHash: string
  candidateID: string
  state: string
  privacy: 'private'
  videoID?: string
  videoURL?: string
}

type Format = 'episode' | 'short'
type Hash = string

export interface FullE2EAPI {
  createRun(spec: RunSpec): Promise<unknown>
  getRun(id: string): Promise<DashboardRun>
  renderBroadcast(id: string): Promise<ReplayStatus>
  broadcastStatus(id: string): Promise<ReplayStatus>
  assess(id: string): Promise<{ assessment: StudioAssessment }>
  createPlan(id: string, assessmentHash: string, format: Format): Promise<{ plan: { hash: Hash; profile: Format } }>
  renderEdit(id: string, hash: string): Promise<StudioEditStatus>
  editStatus(id: string, hash: string): Promise<StudioEditStatus>
  createPackage(id: string, assessmentHash: string, planHash: string): Promise<{ package: StudioPackage }>
  renderThumbnails(id: string, hash: string): Promise<StudioThumbnailList>
  selectThumbnail(id: string, hash: string, candidateID: string): Promise<unknown>
  createCandidate(id: string, proposal: {
    assessment_hash: string
    package_hash?: string
    format: 'episode' | 'short' | 'archive'
    title_id?: string
    thumbnail_id?: string
    privacy?: 'private'
    not_before_unix?: number
  }): Promise<StudioPublicationCandidate>
  publicationAction(id: string, candidateID: string, action: 'approve' | 'publish'): Promise<StudioPublicationCandidate>
  publications(id: string): Promise<{ candidates: StudioPublicationCandidate[] }>
  youtubeStatus(id: string): Promise<YouTubePublishStatus>
}

export interface FullE2EOptions {
  runID: string
  fresh: boolean
  starter: string
  seed: number
  deployment: string
  uploadPrivate: boolean
  signal: AbortSignal
  pollMS?: number
  timeoutMS?: number
  onStage: (stage: FullE2EStage) => void
  onOutput: (output: FullE2EOutput) => void
}

function hash(value: string | undefined, what: string): string {
  if (!value || !/^[a-f0-9]{64}$/.test(value)) throw new Error(`Missing or invalid ${what} hash`)
  return value
}

export function isVerifiedChampion(run: DashboardRun | null | undefined): boolean {
  if (!run || run.game !== 'pokemon-red' || run.status !== 'done' || run.reason !== 'done') return false
  if (!['elite-four', 'beat the elite four and champion.', 'beat the elite four and champion'].includes((run.goal || '').toLowerCase().trim())) return false
  const hall = (run.player?.milestones || []).some(m => /hall[ _]of[ _]fame/i.test(m))
  return run.stats?.goal_complete === true || hall
}

function abortCheck(signal: AbortSignal): void {
  if (signal.aborted) throw new Error('Stopped coordinating in this browser. The server-side run and render jobs were not cancelled.')
}

async function pause(milliseconds: number, signal: AbortSignal): Promise<void> {
  abortCheck(signal)
  await new Promise<void>((resolve, reject) => {
    const timer = setTimeout(() => { signal.removeEventListener('abort', onAbort); resolve() }, milliseconds)
    function onAbort(): void {
      clearTimeout(timer)
      signal.removeEventListener('abort', onAbort)
      reject(new Error('Stopped coordinating in this browser. Server-side work is unaffected.'))
    }
    signal.addEventListener('abort', onAbort, { once: true })
  })
}

export async function runFullE2E(api: FullE2EAPI, options: FullE2EOptions): Promise<FullE2EOutput[]> {
  const { runID: id, signal } = options
  if (!/^[-_a-zA-Z0-9]{3,100}$/.test(id)) throw new Error('Choose a valid run ID')
  const startedAt = Date.now()
  const deadline = startedAt + (options.timeoutMS ?? 48 * 60 * 60 * 1000)
  const interval = options.pollMS ?? 10000
  if (interval <= 0 || deadline <= startedAt) throw new Error('Invalid test interval or timeout')
  const outputs: FullE2EOutput[] = []
  const progress = (stage: string, label: string, state: FullE2EStageState, detail = '') =>
    options.onStage({ id: stage, label, state, detail })
  let stage = 'game'
  let label = 'Complete the full campaign'
  const mark = (name: string, title: string, detail = '') => { stage = name; label = title; progress(name, title, 'working', detail) }
  const finished = (detail = '') => progress(stage, label, 'ready', detail)
  const checked = async <T>(fn: () => Promise<T>): Promise<T> => { abortCheck(signal); const value = await fn(); abortCheck(signal); return value }
  const until = async <T>(fn: () => Promise<T>, valid: (value: T) => boolean, invalid: (value: T) => string | null): Promise<T> => {
    while (Date.now() < deadline) {
      const value = await checked(fn)
      const failure = invalid(value)
      if (failure) throw new Error(failure)
      if (valid(value)) return value
      await pause(Math.min(interval, Math.max(1, deadline - Date.now())), signal)
    }
    throw new Error('Timed out waiting for ' + label)
  }
  try {
    mark('game', 'Complete the full campaign', options.fresh ? 'Launching Pokémon Red' : 'Checking the selected run')
    if (options.fresh) {
      await checked(() => api.createRun({
        run_id: id, seed: options.seed, game: 'pokemon-red', planner: 'llm',
        starter: options.starter, dest: '', goal: 'elite-four', llm_profile: '',
        llm_deployment: options.deployment || undefined, play_style: 'adventure',
        purpose: 'full_e2e', risk_tolerance: 'balanced', wild_encounters: 'planner',
        reasoning_effort: '', fps: 0, max_rounds: 0, max_frames: 0,
        recovery_profile: 'resilient', endless: false, random_seed: false
      }))
    }
    const run = await until(() => api.getRun(id), r => r.status === 'done',
      r => r.status === 'done' && !isVerifiedChampion(r)
        ? 'Run finished without verified Hall of Fame completion (' + (r.reason || 'unknown reason') + ')' : null)
    if (!isVerifiedChampion(run)) throw new Error('Missing verified Hall of Fame evidence')
    finished('Hall of Fame goal completed')
    if (options.uploadPrivate) {
      mark('uploader', 'Check private uploader configuration')
      const config = await checked(() => api.youtubeStatus(id))
      if (!config.configured) throw new Error('YouTube uploader is not configured on the replay server')
      finished('Server-side credentials available')
    }
    mark('source', 'Render broadcast replay')
    const initial = await checked(() => api.broadcastStatus(id))
    if (initial.state !== 'ready') await checked(() => api.renderBroadcast(id))
    await until(() => api.broadcastStatus(id), s => s.state === 'ready' && Number(s.size || 0) > 0,
      s => ['error', 'failed', 'cancelled'].includes(s.state) ? (s.error || 'Broadcast render failed') : null)
    finished('Broadcast MP4 available')

    mark('producer', 'Assess recorded story')
    const assessment = (await checked(() => api.assess(id))).assessment
    const assessmentHash = hash(assessment.hash, 'assessment')
    finished('Producer decision: ' + assessment.publication_class)
    if (assessment.publication_class === 'do_not_publish' || assessment.publication_class === 'archive_only') {
      await checked(() => api.createCandidate(id, { assessment_hash: assessmentHash, format: 'archive' }))
      throw new Error('Producer classified this run as ' + assessment.publication_class + '. No upload is allowed.')
    }
    if (!['full_episode', 'short_only'].includes(assessment.publication_class)) throw new Error('Unknown producer verdict')
    if (assessment.publication_class !== 'full_episode') throw new Error('Full episode + Short requires full_episode producer approval; got short_only')

    for (const fmt of ['episode', 'short'] as const) {
      mark(fmt + '-edit', fmt === 'episode' ? 'Render full episode' : 'Render portrait Short')
      const plan = (await checked(() => api.createPlan(id, assessmentHash, fmt))).plan
      const planHash = hash(plan.hash, fmt + ' edit')
      let edit = await checked(() => api.editStatus(id, planHash))
      if (edit.state !== 'ready') {
        await checked(() => api.renderEdit(id, planHash))
        edit = await until(() => api.editStatus(id, planHash), s => s.state === 'ready',
          s => ['error', 'failed', 'cancelled'].includes(s.state) ? (s.error || 'Edit rendering failed') : null)
      }
      if (edit.state !== 'ready') throw new Error(fmt + ' not ready')
      finished('Edited ' + fmt + ' ready')
      mark(fmt + '-package', fmt === 'episode' ? 'Package full episode' : 'Package portrait Short')
      const pkg = (await checked(() => api.createPackage(id, assessmentHash, planHash))).package
      const packageHash = hash(pkg.hash, fmt + ' package')
      const titles = fmt === 'episode' ? pkg.titles : pkg.shorts
      if (!titles?.[0]?.id) throw new Error(fmt + ' has no event-backed title candidate')
      let thumbnailID: string | undefined
      if (fmt === 'episode') {
        const thumbs = await checked(() => api.renderThumbnails(id, packageHash))
        const first = thumbs.manifest?.candidates?.[0]?.id
        thumbnailID = hash(first, 'episode thumbnail')
        await checked(() => api.selectThumbnail(id, packageHash, thumbnailID!))
      }
      finished('Metadata' + (thumbnailID ? ' and thumbnail' : '') + ' ready')
      mark(fmt + '-candidate', 'Queue private ' + fmt + ' candidate')
      const candidate = await checked(() => api.createCandidate(id, {
        assessment_hash: assessmentHash, package_hash: packageHash,
        format: fmt, title_id: titles[0].id,
        thumbnail_id: thumbnailID, privacy: 'private', not_before_unix: 0
      }))
      const candidateID = hash(candidate.id, fmt + ' candidate')
      if (candidate.proposal.privacy !== 'private' || candidate.proposal.format !== fmt) throw new Error('Candidate identity or privacy mismatch')
      if (!['queued', 'approved', 'published'].includes(candidate.state)) throw new Error('Quality gate: ' + (candidate.reason || candidate.state))
      const output: FullE2EOutput = { format: fmt, planHash, packageHash, candidateID, state: candidate.state, privacy: 'private' }
      outputs.push(output)
      options.onOutput({ ...output })
      finished('Quality gate passed')
    }
    if (!options.uploadPrivate) return outputs

    for (const output of outputs) {
      const fmt = output.format
      mark(fmt + '-upload', 'Upload private ' + fmt + ' to YouTube', 'Explicit opt-in; may wait for channel pacing')
      let candidate = (await checked(() => api.publications(id))).candidates.find(c => c.id === output.candidateID)
      if (!candidate) throw new Error('Candidate missing from durable publication queue')
      if (candidate.proposal.privacy !== 'private') throw new Error('Refusing non-private candidate')
      if (candidate.state === 'queued') candidate = await checked(() => api.publicationAction(id, output.candidateID, 'approve'))
      if (candidate.state === 'approved') {
        // #803 throttles different videos; the channel may require a pause
        // between the episode and Short. Never override that server rule.
        while (Date.now() < deadline) {
          try {
            candidate = await checked(() => api.publicationAction(id, output.candidateID, 'publish'))
            break
          } catch (err) {
            if (!(err instanceof Error) || !err.message.includes('minimum upload interval')) throw err
            await pause(Math.min(interval, Math.max(1, deadline - Date.now())), signal)
          }
        }
      }
      candidate = await until(async () => {
        const items = (await api.publications(id)).candidates
        const found = items.find(c => c.id === output.candidateID)
        if (!found) throw new Error('Candidate disappeared from publication queue')
        return found
      }, c => c.state === 'published', c =>
        ['failed', 'blocked', 'skipped'].includes(c.state) ? (c.reason || 'Publication failed') : null)
      if (candidate.proposal.privacy !== 'private' || !candidate.provider_id) throw new Error('Published candidate lacks private intent or provider video ID')
      output.state = candidate.state
      output.videoID = candidate.provider_id
      output.videoURL = candidate.provider_url
      options.onOutput({ ...output })
      finished('YouTube video ID recorded: ' + candidate.provider_id)
    }
    return outputs
  } catch (cause) {
    progress(stage, label, 'failed', cause instanceof Error ? cause.message : 'Unknown failure')
    throw cause
  }
}
