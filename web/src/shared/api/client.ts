import type { BuildProvenance } from './build'
import type {
  DashboardQuery,
  DashboardRun,
  DashboardSnapshot,
  ExperimentList,
  ExperimentRequest,
  ExperimentView,
  ModelRegistrySnapshot,
  ReplayStatus,
  RunArtifact,
  RunSpec,
  TriageGroup
} from './types'

export class ApiError extends Error {
  readonly status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

function queryString(params: DashboardQuery): string {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === '' || value === false) continue
    query.set(key, value === true ? '1' : String(value))
  }
  const encoded = query.toString()
  return encoded ? `?${encoded}` : ''
}

async function errorDetail(response: Response): Promise<string> {
  let detail = `${response.status} ${response.statusText}`.trim()
  try {
    const body = await response.json() as { error?: string }
    if (body.error) detail = body.error
  } catch {
    // Keep the status text when the response is not JSON.
  }
  return detail || 'Request failed'
}

async function requestJSON<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    cache: 'no-store',
    headers: {
      Accept: 'application/json',
      ...(init.body ? { 'Content-Type': 'application/json' } : {}),
      ...(init.headers || {})
    },
    ...init
  })
  if (!response.ok) throw new ApiError(await errorDetail(response), response.status)
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

function newRunID(): string {
  const values = new Uint32Array(2)
  crypto.getRandomValues(values)
  return `run-${values[0].toString(36)}${values[1].toString(36)}`
}

export function getBuildProvenance(signal?: AbortSignal): Promise<BuildProvenance> {
  return requestJSON<BuildProvenance>('/v1/build', { signal })
}

export function getDashboard(params: DashboardQuery = {}, signal?: AbortSignal): Promise<DashboardSnapshot> {
  return requestJSON<DashboardSnapshot>(`/v1/dashboard${queryString(params)}`, { signal })
}

export function getStats(signal?: AbortSignal): Promise<Record<string, unknown>> {
  return requestJSON<Record<string, unknown>>('/v1/stats', { signal })
}

export async function getTriage(signal?: AbortSignal): Promise<TriageGroup[]> {
  const value = await requestJSON<TriageGroup[] | { groups?: TriageGroup[] }>('/v1/triage', { signal })
  return Array.isArray(value) ? value : value.groups || []
}

export function createRun(spec: RunSpec, signal?: AbortSignal): Promise<Record<string, unknown>> {
  if (!spec.run_id.trim()) spec.run_id = newRunID()
  return requestJSON<Record<string, unknown>>('/v1/specs', {
    method: 'POST',
    body: JSON.stringify(spec),
    signal
  })
}

export async function getModels(signal?: AbortSignal): Promise<ModelRegistrySnapshot> {
  const value = await requestJSON<ModelRegistrySnapshot | { deployments?: ModelRegistrySnapshot['deployments'] }>('/v1/models', { signal })
  return { deployments: value.deployments || [], hosts: 'hosts' in value ? value.hosts : undefined }
}

export async function getExperiments(signal?: AbortSignal): Promise<ExperimentView[]> {
  const value = await requestJSON<ExperimentList | ExperimentView[]>('/v1/experiments', { signal })
  return Array.isArray(value) ? value : value.experiments || []
}

export function getExperiment(id: string, signal?: AbortSignal): Promise<ExperimentView> {
  return requestJSON<ExperimentView>(`/v1/experiments/${encodeURIComponent(id)}`, { signal })
}

export function createExperiment(request: ExperimentRequest, signal?: AbortSignal): Promise<ExperimentView> {
  return requestJSON<ExperimentView>('/v1/experiments', {
    method: 'POST',
    body: JSON.stringify(request),
    signal
  })
}

export function investigateTriage(key: string, signal?: AbortSignal): Promise<Record<string, unknown>> {
  return requestJSON<Record<string, unknown>>(`/v1/triage/${encodeURIComponent(key)}/investigate`, {
    method: 'POST',
    body: '{}',
    signal
  })
}

export function cancelRun(runID: string, signal?: AbortSignal): Promise<Record<string, unknown>> {
  return requestJSON<Record<string, unknown>>(`/v1/runs/${encodeURIComponent(runID)}/cancel`, {
    method: 'POST',
    body: '{}',
    signal
  })
}

export function forceEndWorker(addr: string, signal?: AbortSignal): Promise<Record<string, unknown>> {
  return requestJSON<Record<string, unknown>>(`/v1/workers/${encodeURIComponent(addr)}/force-end`, {
    method: 'POST',
    body: '{}',
    signal
  })
}

export async function deleteRun(runID: string, signal?: AbortSignal): Promise<void> {
  await requestJSON<Record<string, unknown>>(`/v1/runs/${encodeURIComponent(runID)}`, {
    method: 'DELETE',
    signal
  })
}

export async function getRun(runID: string, signal?: AbortSignal): Promise<DashboardRun> {
  const value = await requestJSON<DashboardRun | { run?: DashboardRun }>(`/v1/runs/${encodeURIComponent(runID)}`, { signal })
  const wrapped = value as { run?: DashboardRun }
  return wrapped.run ?? (value as DashboardRun)
}

export function getRunDebug(runID: string, signal?: AbortSignal): Promise<Record<string, unknown>> {
  return requestJSON<Record<string, unknown>>(`/v1/runs/${encodeURIComponent(runID)}/debug`, { signal })
}

export async function getRunArtifacts(runID: string, signal?: AbortSignal): Promise<RunArtifact[]> {
  const value = await requestJSON<RunArtifact[] | { artifacts?: RunArtifact[] }>(`/v1/runs/${encodeURIComponent(runID)}/artifacts`, { signal })
  return Array.isArray(value) ? value : value.artifacts || []
}

export async function getRunCheckpoints(runID: string, signal?: AbortSignal): Promise<unknown[]> {
  const value = await requestJSON<unknown[] | { checkpoints?: unknown[] }>(`/v1/runs/${encodeURIComponent(runID)}/checkpoints`, { signal })
  return Array.isArray(value) ? value : value.checkpoints || []
}

export function getReplayStatus(runID: string, signal?: AbortSignal): Promise<ReplayStatus> {
  return requestJSON<ReplayStatus>(`/v1/runs/${encodeURIComponent(runID)}/replay/status`, { signal })
}

export function renderReplay(runID: string, signal?: AbortSignal): Promise<ReplayStatus> {
  return requestJSON<ReplayStatus>(`/v1/runs/${encodeURIComponent(runID)}/replay/render`, {
    method: 'POST',
    body: '{}',
    signal
  })
}

export function artifactContentURL(runID: string, name: string): string {
  return `/v1/runs/${encodeURIComponent(runID)}/artifacts/${encodeURIComponent(name)}/content`
}

export function replayVideoURL(runID: string): string {
  return `/v1/runs/${encodeURIComponent(runID)}/replay/video`
}
